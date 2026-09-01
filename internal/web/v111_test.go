package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

func TestEveryRenderedScreenHasGlobalDeskControls(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	for _, path := range []string{"/shift", "/stock", "/out", "/inwards", "/suppliers", "/log", "/inward/new", "/issue/new", "/return/new", "/entry/INW-0001/edit", "/product/PRD-0001/edit", "/not-a-route"} {
		resp, body := e.get(path)
		if path == "/not-a-route" && resp.StatusCode != 404 {
			t.Fatalf("%s status=%d", path, resp.StatusCode)
		}
		assertContains(t, body, `href="/stock">Dashboard</a>`)
		assertContains(t, body, `href="/shift">Change person</a>`)
	}
}

func TestGlobalDeskControlsWorkWithoutJavaScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/stock")
	beforeScript := strings.Split(body, "<script")[0]
	for _, want := range []string{`<a href="/stock">Dashboard</a>`, `<a href="/shift">Change person</a>`} {
		if !strings.Contains(beforeScript, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestLogLoadsProductPickerScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/log")
	for _, want := range []string{"Which product", `data-mode="log"`, `name="productName"`, `name="productId"`, `<script src="/static/picker.js"></script>`} {
		assertContains(t, body, want)
	}
}

func TestLogProductFilterRequiresSelectedID(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, all := e.get("/log?day=all&productName=Chairs")
	_, bad := e.get("/log?day=all&productId=PRD-9999&productName=Chairs")
	if all != bad {
		t.Fatal("typed or unknown product changed filter")
	}
	_, chairs := e.get("/log?day=all&productId=PRD-0001")
	if chairs == all {
		t.Fatal("selected ID did not filter")
	}
}

func TestIssueFormShowsOptionalChallan(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	_, body := e.get("/issue/new")
	for _, want := range []string{"Challan no. (optional)", "Type it from the paper challan.", `name="challanNo"`, `autocomplete="off"`} {
		assertContains(t, body, want)
	}
	if strings.Index(body, "name=\"quantity\"") > strings.Index(body, "name=\"challanNo\"") || strings.Index(body, "name=\"challanNo\"") > strings.Index(body, "name=\"takerName\"") {
		t.Fatal("challan field order is wrong")
	}
}

func TestIssueStoresCleanedOptionalChallan(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	form := walkthroughIssue()
	form.Set("challanNo", "  CH-452   A ")
	resp, _ := e.post("/issue/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	issues := e.saved().Issues
	if got := issues[len(issues)-1].ChallanNo; got != "CH-452 A" {
		t.Fatalf("challan=%q", got)
	}
}

func TestIssueNeverReusesPreviousChallan(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	form := walkthroughIssue()
	form.Set("challanNo", "452")
	_, _ = e.post("/issue/new", form)
	_, body := e.get("/issue/new?productId=PRD-0001&takerName=Ravi+Menon")
	assertContains(t, body, `name="challanNo" autocomplete="off" value=""`)
}

func TestIssueRefusalKeepsOnlyPostedChallan(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	form := walkthroughIssue()
	form.Set("quantity", "9999")
	form.Set("challanNo", "CH-999")
	resp, body := e.post("/issue/new", form)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	assertContains(t, body, `value="CH-999"`)
	_, clean := e.get("/issue/new")
	assertNotContains(t, clean, `value="CH-999"`)
}

func TestFixIssueChallanAddsChangesAndRemoves(t *testing.T) {
	r := anitaOnDuty()
	id := r.Issues[0].ID
	e := newTestServer(t, r, twoEighteen)
	for _, value := range []string{"CH-452", "452", ""} {
		resp, _ := e.post("/entry/"+id+"/edit", url.Values{"challanNo": {value}})
		if resp.StatusCode != 303 {
			t.Fatalf("%q status=%d", value, resp.StatusCode)
		}
	}
	var got register.Issue
	for _, is := range e.saved().Issues {
		if is.ID == id {
			got = is
		}
	}
	if len(got.Changes) != 3 {
		t.Fatalf("changes=%#v", got.Changes)
	}
	wants := []string{"Added a challan no.: CH-452", "Changed the challan no. from CH-452 to 452", "Removed the challan no. that said: 452"}
	for i, c := range got.Changes {
		if phrase := changePhrase(c, "chairs"); phrase != wants[i] {
			t.Fatalf("phrase %d=%q", i, phrase)
		}
	}
}

func TestReturnSearchByChallanShowsEveryOutstandingMatch(t *testing.T) {
	r := anitaOnDuty()
	base := twoEighteen
	r.Issues = append(r.Issues,
		register.Issue{ID: "ISS-0101", ProductID: "PRD-0001", Quantity: 20, ChallanNo: "452", TakerName: "Ravi Menon", TakerMobile: "1", AdditionalTakers: []register.IssueRecipient{{Name: "Amit Sharma", Mobile: "2"}}, IssuedAt: base, RecordedAt: base, PersonInchargeName: "Anita Rao"},
		register.Issue{ID: "ISS-0102", ProductID: "PRD-0005", Quantity: 5, ChallanNo: "CH-452", TakerName: "Meera", TakerMobile: "3", IssuedAt: base.Add(time.Minute), RecordedAt: base.Add(time.Minute), PersonInchargeName: "Anita Rao"})
	e := newTestServer(t, r, base)
	resp, body := e.get("/return/new?challan=45")
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	for _, want := range []string{"Outstanding entries with this challan", "Challan 452", "20 chairs out", "Ravi Menon and Amit Sharma", "Challan CH-452", "5 charcoal sacks out", "Pick this holding"} {
		assertContains(t, body, want)
	}
}

func TestReturnSearchByChallanNoMatch(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	before := string(mustJSON(t, e.saved()))
	resp, body := e.get("/return/new?challan=none")
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, "No outstanding issue matches that challan number.")
	after := string(mustJSON(t, e.saved()))
	if before != after {
		t.Fatal("search wrote")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProductEditScreenFromDashboard(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, stock := e.get("/stock")
	assertContains(t, stock, `href="/product/PRD-0001/edit">Fix product`)
	_, body := e.get("/product/PRD-0001/edit")
	for _, want := range []string{"Fix a product", "Product name", "Save product name", "Delete this product", "Delete Chairs?", "Received entries: 3", "Issue entries: 4", "Return entries: 1", "Currently out: 275 chairs", "The history will stay in Who did what.", "Why are you deleting this product?", "Yes, delete Chairs and its entries"} {
		assertContains(t, body, want)
	}
}

func TestRenameChairsToFoldingChairs(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	resp, _ := e.post("/product/PRD-0001/edit", url.Values{"name": {"Folding chairs"}})
	if resp.StatusCode != 303 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	_, stock := e.get(resp.Header.Get("Location"))
	assertContains(t, stock, "Renamed Chairs to Folding chairs.")
	assertContains(t, stock, "Folding chairs")
	_, log := e.get("/log?day=all")
	assertContains(t, log, "Changed the product name from Chairs to Folding chairs")
}

func TestDeleteProductRequiresReason(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	before := mustJSON(t, e.saved())
	resp, body := e.post("/product/PRD-0001/delete", url.Values{})
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, "Write why this product is being deleted.")
	if string(before) != string(mustJSON(t, e.saved())) {
		t.Fatal("refusal wrote")
	}
}

func TestDeleteProductAndHistoryAtomically(t *testing.T) {
	r := register.WalkthroughT3()
	impact, _ := register.ProductDeletionImpact(r, "PRD-0001")
	e := newTestServer(t, r, sixOhFive)
	resp, _ := e.post("/product/PRD-0001/delete", url.Values{"impactVersion": {impact.Version}, "reason": {"Goods never arrived."}})
	if resp.StatusCode != 303 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	saved := e.saved()
	if _, ok := register.ProductByID(saved, "PRD-0001"); ok {
		t.Fatal("product still live")
	}
	if len(register.Validate(saved)) != 0 {
		t.Fatal("invalid")
	}
	_, stock := e.get(resp.Header.Get("Location"))
	assertContains(t, stock, "Deleted Chairs and all of its entries. The history is still in Who did what.")
	assertNotContains(t, stock, `<td class="nm">Chairs</td>`)
	_, log := e.get("/log?day=all&productId=PRD-0001")
	assertContains(t, log, "Deleted this product: Chairs")
	assertContains(t, log, "Deleted — Goods never arrived.")
}

func TestLogPickerCanSelectDeletedProduct(t *testing.T) {
	r := register.WalkthroughT3()
	_ = register.DeleteProductCascade(r, "PRD-0001", "Anita Rao", sixOhFive, "Wrong product")
	e := newTestServer(t, r, sixOhFive)
	resp, err := e.client().Get(e.URL + "/api/products?mode=log&q=chair")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rows []suggestion
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Label != "Chairs — deleted" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestDashboardStillNeedsACurrentShift(t *testing.T) {
	r := register.WalkthroughT0()
	r.OnDutyStaffID = ""
	r.ShiftStartedAt = nil
	e := newTestServer(t, r, sixOhFive)
	resp, _ := e.get("/stock")
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/shift" {
		t.Fatalf("status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, body := e.get("/shift")
	assertContains(t, body, "Dashboard")
}

func TestChangingPersonAffectsFutureActionsOnly(t *testing.T) {
	r := register.WalkthroughT1()
	old := r.Inwards[len(r.Inwards)-1]
	e := newTestServer(t, r, twoEighteen)
	resp, _ := e.post("/shift/start", url.Values{"staffId": {"STF-0002"}})
	if resp.StatusCode != 303 {
		t.Fatal(resp.StatusCode)
	}
	form := walkthroughIssue()
	resp, _ = e.post("/issue/new", form)
	if resp.StatusCode != 303 {
		t.Fatal(resp.StatusCode)
	}
	saved := e.saved()
	if saved.Inwards[len(saved.Inwards)-1].RecordedBy != old.RecordedBy || saved.Issues[len(saved.Issues)-1].PersonInchargeName != "Anita Rao" {
		t.Fatal("history was restamped or future action used old actor")
	}
}

func TestChangingPersonCreatesNoLogRow(t *testing.T) {
	r := register.WalkthroughT1()
	before := register.LogEntries(r)
	e := newTestServer(t, r, twoEighteen)
	_, _ = e.post("/shift/start", url.Values{"staffId": {"STF-0002"}})
	after := register.LogEntries(e.saved())
	if string(mustJSON(t, before)) != string(mustJSON(t, after)) {
		t.Fatal("shift created or changed log rows")
	}
}

func TestLogProductPickerSuggestsTypedPrefix(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), twoEighteen)
	resp, err := e.client().Get(e.URL + "/api/products?mode=log&q=ch")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rows []suggestion
	_ = json.NewDecoder(resp.Body).Decode(&rows)
	if len(rows) != 2 || rows[0].Label != "Chairs — 390 on hand" || rows[1].Label != "Charcoal sacks — 12 on hand" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestLogProductPickerHasNoScriptFallback(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), twoEighteen)
	_, body := e.get("/log?productId=PRD-0001")
	fallback := between(body, "<noscript>", "</noscript>")
	if strings.Count(fallback, "<option") != 6 {
		t.Fatalf("options=%d", strings.Count(fallback, "<option"))
	}
	if !strings.Contains(fallback, `value="PRD-0001" selected`) {
		t.Fatal("selected product absent")
	}
}

func TestLogProductFilterKeepsOtherFilters(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/log?day=2026-09-03&kind=went_out&q=98861&productId=PRD-0001")
	for _, want := range []string{"ISS-0003", "ISS-0008", "day=2026-09-03", "kind=went_out", "q=98861"} {
		assertContains(t, body, want)
	}
}

func TestIssueWithoutChallanRemainsCompatible(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	resp, _ := e.post("/issue/new", walkthroughIssue())
	if resp.StatusCode != 303 {
		t.Fatal(resp.StatusCode)
	}
	saved := e.saved()
	if saved.Issues[len(saved.Issues)-1].ChallanNo != "" {
		t.Fatal("challan not empty")
	}
	raw := mustJSON(t, saved.Issues[len(saved.Issues)-1])
	if strings.Contains(string(raw), "challanNo") {
		t.Fatal("empty challan marshalled")
	}
}

func TestIssueAllowsSameChallanForProductsAndGroups(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	forms := []url.Values{walkthroughIssue(), walkthroughIssue()}
	forms[0].Set("challanNo", "452")
	forms[1].Set("challanNo", "452")
	forms[1].Set("productId", "PRD-0005")
	forms[1].Set("quantity", "5")
	forms[1].Set("takerName", "Meera")
	for _, form := range forms {
		resp, _ := e.post("/issue/new", form)
		if resp.StatusCode != 303 {
			t.Fatalf("status=%d", resp.StatusCode)
		}
	}
	issues := e.saved().Issues
	if issues[len(issues)-2].ChallanNo != "452" || issues[len(issues)-1].ChallanNo != "452" {
		t.Fatal("duplicate challan refused")
	}
}

func TestOutAndReturnLinesShowChallanOnlyWhenPresent(t *testing.T) {
	r := register.WalkthroughT3()
	r.Issues[0].ChallanNo = "452"
	e := newTestServer(t, r, sixOhFive)
	_, out := e.get("/out")
	assertContains(t, out, "Challan no. 452")
	if strings.Contains(out, "Challan no. </div>") {
		t.Fatal("empty placeholder")
	}
	_, ret := e.get("/return/new?q=99860")
	assertContains(t, ret, "Challan no. 452")
}

func TestReturnChallanSearchTakesPrecedenceOverPersonQuery(t *testing.T) {
	r := anitaOnDuty()
	r.Issues = append(r.Issues, register.Issue{ID: "ISS-0999", ProductID: "PRD-0001", Quantity: 5, ChallanNo: "452", TakerName: "Ravi Only", IssuedAt: twoEighteen, RecordedAt: twoEighteen})
	e := newTestServer(t, r, twoEighteen)
	_, body := e.get("/return/new?q=Meera&challan=452")
	assertContains(t, body, "Ravi Only")
	assertNotContains(t, body, "Nobody by that name")
}

func TestReturnChallanMatchSelectsExistingHoldingProduct(t *testing.T) {
	r := anitaOnDuty()
	r.Issues = append(r.Issues, register.Issue{ID: "ISS-0999", ProductID: "PRD-0001", Quantity: 20, ChallanNo: "452", TakerName: "Ravi Only", IssuedAt: twoEighteen, RecordedAt: twoEighteen})
	e := newTestServer(t, r, twoEighteen)
	_, body := e.get("/return/new?challan=452&holdingIssueId=ISS-0999&productId=PRD-0001")
	assertContains(t, body, "Chairs coming back")
	assertContains(t, body, `name="quantity" min="1" max="20" step="1"
             value="20"`)
}

func challanBoundaryRegister() *register.Register {
	return &register.Register{SchemaVersion: 2, Products: []register.Product{{ID: "PRD-1", Name: "Chairs"}}, Inwards: []register.Inward{{ID: "INW-1", ProductID: "PRD-1", Quantity: 100}}, Issues: []register.Issue{{ID: "ISS-1", ProductID: "PRD-1", Quantity: 20, ChallanNo: "452", TakerName: "Ravi", AdditionalTakers: []register.IssueRecipient{{Name: "Amit"}}, IssuedAt: twoEighteen}, {ID: "ISS-2", ProductID: "PRD-1", Quantity: 7, ChallanNo: "452", TakerName: "Meera", IssuedAt: twoEighteen.Add(time.Minute)}}}
}

func TestReturnByChallanCannotCrossHoldingBoundary(t *testing.T) {
	r := challanBoundaryRegister()
	plan, refusal := planReturnForHolding(r, "ISS-1", []string{"ISS-1"}, "15", "Ravi", "expected", "later")
	if refusal != "" {
		t.Fatal(refusal)
	}
	r.Returns = append(r.Returns, register.Return{ID: "RET-1", ProductID: plan.ProductID, Allocations: plan.Allocations})
	if register.OutstandingOnIssue(r, "ISS-1") != 5 || register.OutstandingOnIssue(r, "ISS-2") != 7 {
		t.Fatal("return crossed holding")
	}
}

func TestReturnByChallanRefusesStaleHolding(t *testing.T) {
	r := challanBoundaryRegister()
	r.Issues[0].Deleted = &register.Deletion{Reason: "gone"}
	_, refusal := planReturnForHolding(r, "ISS-1", []string{"ISS-1"}, "1", "Ravi", "expected", "later")
	if refusal != "That holding has changed. Pick it again from the list." {
		t.Fatalf("refusal=%q", refusal)
	}
}

func TestRenameProductRefusesBlankAndDuplicate(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	for _, name := range []string{" ", " round   tables "} {
		resp, body := e.post("/product/PRD-0001/edit", url.Values{"name": {name}})
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
		if register.CleanName(name) == "" {
			assertContains(t, body, "Type the product's name.")
		} else {
			assertContains(t, body, "Round tables is already on the list. Pick a different name.")
		}
	}
}

func TestRenameProductNearDuplicateNeedsConfirmation(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	resp, body := e.post("/product/PRD-0004/edit", url.Values{"name": {"Round stools"}})
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, "Round tables is already on the list. Rename this product to Round stools anyway?")
	resp, _ = e.post("/product/PRD-0004/edit", url.Values{"name": {"Round stools"}, "confirm": {"yes"}})
	if resp.StatusCode != 303 {
		t.Fatal(resp.StatusCode)
	}
}

func TestRenameProductRechecksAtSaveTime(t *testing.T) {
	r := register.WalkthroughT3()
	r.Products[1].Name = "Folding chairs"
	e := newTestServer(t, r, sixOhFive)
	resp, _ := e.post("/product/PRD-0001/edit", url.Values{"name": {"Folding chairs"}})
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if e.saved().Products[0].Name != "Chairs" {
		t.Fatal("duplicate rename saved")
	}
}

func TestDeleteProductShowsStrongImpactSummary(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/product/PRD-0001/edit")
	for _, want := range []string{"Delete Chairs?", "Received entries: 3", "Issue entries: 4", "Return entries: 1", "Currently out: 275 chairs", `name="impactVersion" value="`} {
		assertContains(t, body, want)
	}
}

func TestDeleteProductRefusesStaleImpact(t *testing.T) {
	r := register.WalkthroughT3()
	impact, _ := register.ProductDeletionImpact(r, "PRD-0001")
	r.Issues = append(r.Issues, register.Issue{ID: "ISS-0999", ProductID: "PRD-0001", Quantity: 1, TakerName: "Ravi", IssuedAt: sixOhFive})
	e := newTestServer(t, r, sixOhFive)
	resp, body := e.post("/product/PRD-0001/delete", url.Values{"impactVersion": {impact.Version}, "reason": {"wrong"}})
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, "This product changed. Check the numbers and confirm again.")
	if _, ok := register.ProductByID(e.saved(), "PRD-0001"); !ok {
		t.Fatal("stale delete succeeded")
	}
}

func TestDeletedNameMayBeCreatedAgain(t *testing.T) {
	r := register.WalkthroughT3()
	_ = register.DeleteProductCascade(r, "PRD-0001", "Anita", sixOhFive, "wrong")
	e := newTestServer(t, r, sixOhFive)
	resp, _ := e.post("/product/new", url.Values{"name": {"Chairs"}, "return": {"/stock"}})
	if resp.StatusCode != 303 {
		t.Fatal(resp.StatusCode)
	}
	live := 0
	for _, p := range e.saved().Products {
		if p.Deleted == nil && p.Name == "Chairs" {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("live=%d", live)
	}
}

func TestDeletedProductCannotBeManagedAgain(t *testing.T) {
	r := register.WalkthroughT3()
	impact, _ := register.ProductDeletionImpact(r, "PRD-0001")
	e := newTestServer(t, r, sixOhFive)
	_, _ = e.post("/product/PRD-0001/delete", url.Values{"impactVersion": {impact.Version}, "reason": {"wrong"}})
	resp, body := e.get("/product/PRD-0001/edit")
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, "This product was deleted. Its history is still in Who did what.")
	assertNotContains(t, body, "Save product name")
}

func TestProductLifecycleLogIsComplete(t *testing.T) {
	r := register.WalkthroughT3()
	_ = register.RenameProduct(r, "PRD-0001", "Folding chairs", "Anita", sixOhFive)
	_ = register.DeleteProductCascade(r, "PRD-0001", "Anita", sixOhFive.Add(time.Minute), "Wrong product")
	entries := register.LogEntries(r)
	corrected, deleted := 0, 0
	for _, e := range entries {
		if e.ProductID == "PRD-0001" && e.Kind == register.LogCorrected {
			corrected++
		}
		if e.ProductID == "PRD-0001" && e.Kind == register.LogDeleted {
			deleted++
		}
	}
	if corrected != 1 || deleted != 9 {
		t.Fatalf("corrected=%d deleted=%d", corrected, deleted)
	}
	for _, e := range entries {
		if e.ProductID == "PRD-0001" && e.ProductName != "Folding chairs" {
			t.Fatalf("historical row %s lost renamed product: %q", e.RecordID, e.ProductName)
		}
	}
}

func TestLogPageSearchesChallanAndKeepsFilters(t *testing.T) {
	r := anitaOnDuty()
	r.Issues[0].ChallanNo = "CH-452"
	e := newTestServer(t, r, sixOhFive)
	_, body := e.get("/log?day=all&kind=went_out&productId=PRD-0001&challan=45")
	for _, want := range []string{"Challan no.", "Type any part of the challan number.", "Any challan", "challan=45", "Challan no. CH-452."} {
		assertContains(t, body, want)
	}
}

func TestChallanCorrectionUsesSharedChangePhrase(t *testing.T) {
	tests := []struct {
		c    register.Change
		want string
	}{
		{register.Change{Field: "challan", Label: "Challan no.", To: "CH-452"}, "Added a challan no.: CH-452"},
		{register.Change{Field: "challan", Label: "Challan no.", From: "452", To: "CH-452"}, "Changed the challan no. from 452 to CH-452"},
		{register.Change{Field: "challan", Label: "Challan no.", From: "CH-452"}, "Removed the challan no. that said: CH-452"},
	}
	for _, tc := range tests {
		if got := changePhrase(tc.c, "chairs"); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestIssueChallanSaveFailureRollsBack(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)
	before := mustJSON(t, e.saved())
	dir := filepath.Dir(e.path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	form := walkthroughIssue()
	form.Set("challanNo", "452")
	resp, _ := e.post("/issue/new", form)
	_ = os.Chmod(dir, 0700)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if string(before) != string(mustJSON(t, e.saved())) {
		t.Fatal("failed save changed register")
	}
}

func TestDeleteProductSaveFailureRollsEverythingBack(t *testing.T) {
	r := register.WalkthroughT3()
	impact, _ := register.ProductDeletionImpact(r, "PRD-0001")
	e := newTestServer(t, r, sixOhFive)
	before := mustJSON(t, e.saved())
	dir := filepath.Dir(e.path)
	_ = os.Chmod(dir, 0500)
	resp, _ := e.post("/product/PRD-0001/delete", url.Values{"impactVersion": {impact.Version}, "reason": {"wrong"}})
	_ = os.Chmod(dir, 0700)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if string(before) != string(mustJSON(t, e.saved())) {
		t.Fatal("failed delete changed register")
	}
}
