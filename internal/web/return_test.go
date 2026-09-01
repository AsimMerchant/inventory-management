package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// imranOnDuty is the T2 register at six in the evening: Ravi is holding 50
// chairs across two issues and 2 round tables, and Imran is at the desk.
func imranOnDuty() *register.Register {
	reg := register.WalkthroughT2()
	reg.OnDutyStaffID = "STF-0003"
	return reg
}

// walkthroughReturn is the 45 of 50 that come back at 6:05 pm.
func walkthroughReturn() url.Values {
	return url.Values{
		"q":              {"98861"},
		"productId":      {"PRD-0001"},
		"issueIds":       {"ISS-0003", "ISS-0008"},
		"quantity":       {"45"},
		"returnerName":   {"Ravi Menon"},
		"returnerMobile": {"98861 40023"},
		"returnedAt":     {"2026-09-03T18:05"},
		"disposition":    {"wont_return"},
		"remark":         {"5 chairs broke during setup near the stage. Ravi informed."},
	}
}

func jointReturnRegister() *register.Register {
	reg := anitaOnDuty()
	// This fixture is the stakeholder's exact scenario: Ravi's one solo issue
	// and one joint issue, without the walkthrough's earlier Ravi holdings.
	reg.Issues = nil
	reg.Returns = nil
	at := time.Date(2026, time.September, 3, 15, 0, 0, 0, register.IST)
	reg.Issues = append(reg.Issues,
		register.Issue{ID: "ISS-9001", ProductID: "PRD-0001", Quantity: 3, TakerName: "Ravi Menon", TakerDepartment: "Catering", TakerMobile: "98861 40023", IssuedAt: at, RecordedAt: at},
		register.Issue{ID: "ISS-9002", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", TakerDepartment: "Catering", TakerMobile: "98861 40023", IssuedAt: at.Add(time.Minute), RecordedAt: at.Add(time.Minute), AdditionalTakers: []register.IssueRecipient{{Name: "Amit Sharma", Department: "Setup", Mobile: "97740 11298"}, {Name: "Suresh Patel", Department: "Logistics", Mobile: "90080 77001"}}},
	)
	return reg
}

func TestReturnSearchFindsGroupByEveryMember(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	for _, query := range []string{"Ravi", "Amit", "Suresh", "9774011298"} {
		_, body := e.get("/return/new?q=" + url.QueryEscape(query))
		assertContains(t, body, "Ravi Menon, Amit Sharma and Suresh Patel - holding together")
		assertContains(t, body, "30 out")
	}
	_, ravi := e.get("/return/new?q=98861")
	assertContains(t, ravi, "Ravi Menon - holding alone")
	if got := strings.Count(ravi, "Ravi Menon - holding alone"); got != 1 {
		t.Fatalf("solo context heading shown %d times, want once", got)
	}
	if got := strings.Count(ravi, "Ravi Menon, Amit Sharma and Suresh Patel - holding together"); got != 1 {
		t.Fatalf("joint context heading shown %d times, want once", got)
	}
}

func TestReturnSoloOnlyKeepsLegacyRowsWithoutContextHeadings(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)
	_, body := e.get("/return/new?q=98861")
	assertNotContains(t, body, "holding alone")
	assertNotContains(t, body, "holding together")
}

func TestReturn20FromGroup(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	form := url.Values{"q": {"98861"}, "productId": {"PRD-0001"}, "holdingIssueId": {"ISS-9002"}, "issueIds": {"ISS-9002"}, "quantity": {"20"}, "returnerName": {"Amit Sharma"}, "returnerMobile": {"97740 11298"}, "returnedAt": {"2026-09-03T18:05"}, "disposition": {"expected"}, "remark": {"Ten chairs are coming later."}}
	resp, _ := e.post("/return/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	reg := e.saved()
	re := reg.Returns[len(reg.Returns)-1]
	if len(re.Allocations) != 1 || re.Allocations[0].IssueID != "ISS-9002" || register.OutstandingOnIssue(reg, "ISS-9001") != 3 || register.OutstandingOnIssue(reg, "ISS-9002") != 10 {
		t.Fatalf("return = %#v", re)
	}
}

func TestReturn3FromRaviAlone(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	form := url.Values{"q": {"98861"}, "productId": {"PRD-0001"}, "holdingIssueId": {"ISS-9001"}, "issueIds": {"ISS-9001"}, "quantity": {"3"}, "returnerName": {"Ravi Menon"}, "returnerMobile": {"98861 40023"}, "returnedAt": {"2026-09-03T18:05"}}
	resp, _ := e.post("/return/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	reg := e.saved()
	if register.OutstandingOnIssue(reg, "ISS-9001") != 0 || register.OutstandingOnIssue(reg, "ISS-9002") != 30 {
		t.Fatalf("holdings crossed: %#v", reg.Returns[len(reg.Returns)-1])
	}
}

func TestOutShowsJointQuantityOnce(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	_, body := e.get("/out")
	assertContains(t, body, "Ravi Menon - holding alone - 3 out")
	assertContains(t, body, "Ravi Menon, Amit Sharma and Suresh Patel - holding together - 30 out")
	assertNotContains(t, body, "holding together ·")
	if got := strings.Count(body, "30 taken, 0 back"); got != 1 {
		t.Fatalf("group quantity shown %d times", got)
	}
}

func TestReturnRefusesStaleGroup(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	if err := e.st.Update(func(reg *register.Register) error {
		reg.Returns = append(reg.Returns, register.Return{ID: "RET-9001", ProductID: "PRD-0001", Allocations: []register.Allocation{{IssueID: "ISS-9002", Quantity: 30}}, ReturnerName: "Amit Sharma", ReturnedAt: sixOhFive, RecordedAt: sixOhFive})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"q": {"98861"}, "productId": {"PRD-0001"}, "holdingIssueId": {"ISS-9002"}, "issueIds": {"ISS-9002"}, "quantity": {"1"}, "returnerName": {"Ravi Menon"}, "returnedAt": {"2026-09-03T18:05"}}
	before := len(e.saved().Returns)
	resp, body := e.post("/return/new", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	assertContains(t, body, "That holding has changed. Pick it again from the list.")
	if reg := e.saved(); len(reg.Returns) != before || len(register.Validate(reg)) != 0 {
		t.Fatalf("stale return changed register: %#v", reg.Returns)
	}
}

func TestShortReturnStaysAgainstWholeGroup(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), sixOhFive)
	_, hint := e.get("/return/new?q=9774011298&holdingIssueId=ISS-9002&productId=PRD-0001&quantity=25")
	assertContains(t, hint, "5 chairs missing. Ravi Menon, Amit Sharma and Suresh Patel still have them.")
	form := url.Values{"q": {"9774011298"}, "productId": {"PRD-0001"}, "holdingIssueId": {"ISS-9002"}, "issueIds": {"ISS-9002"}, "quantity": {"25"}, "returnerName": {"Meera Pillai"}, "returnedAt": {"2026-09-03T18:05"}, "disposition": {"expected"}, "remark": {"Five chairs are coming later."}}
	resp, _ := e.post("/return/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	reg := e.saved()
	if got := register.OutstandingOnIssue(reg, "ISS-9002"); got != 5 {
		t.Fatalf("group outstanding = %d", got)
	}
}

func TestReturnFormRendersWalkthroughLabels(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	resp, body := e.get("/return/new")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /return/new returned %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{
		"Someone is returning",
		"Thursday, 3 September · 6:05 pm",
		"Find the person",
		"Search by name, mobile or department.",
	} {
		assertContains(t, body, want)
	}

	_, picked := e.get("/return/new?q=98861&productId=PRD-0001")
	assertContains(t, picked, "Taken back by")
	assertContains(t, picked, `Imran Sheikh <span class="sm">(you)</span>`)
	assertContains(t, picked, "Cancel")
}

func TestReturnPickerHasNoNewPersonRow(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=Meera")
	assertNotContains(t, body, "+ New person named")
	assertContains(t, body, "Nobody by that name is holding anything.")
}

// twoRaviKumarsHoldingChairs is the T2 register with two men of one name, told
// apart only by their mobiles.
func twoRaviKumarsHoldingChairs() *register.Register {
	reg := imranOnDuty()
	at := func(hour, min int) time.Time {
		return time.Date(2026, time.September, 3, hour, min, 0, 0, register.IST)
	}
	reg.Issues = append(reg.Issues,
		register.Issue{
			ID: "ISS-0009", ProductID: "PRD-0001", Quantity: 8,
			TakerName: "Ravi Kumar", TakerDepartment: "Catering", TakerMobile: "98861 99999",
			PersonInchargeName: "Anita Rao", PersonInchargeMobile: "99001 34562",
			IssuedAt: at(11, 0), RecordedAt: at(11, 0),
		},
		register.Issue{
			ID: "ISS-0010", ProductID: "PRD-0001", Quantity: 6,
			TakerName: "Ravi Kumar", TakerDepartment: "Security", TakerMobile: "97740 11298",
			PersonInchargeName: "Anita Rao", PersonInchargeMobile: "99001 34562",
			IssuedAt: at(11, 5), RecordedAt: at(11, 5),
		},
	)
	return reg
}

func TestReturnPickerSeparatesTwoPeopleWithOneName(t *testing.T) {
	e := newTestServer(t, twoRaviKumarsHoldingChairs(), sixOhFive)

	_, body := e.get("/return/new?q=" + url.QueryEscape("Ravi Kumar"))
	assertContains(t, body, "Ravi Kumar · 97740 11298 · Security")
	assertContains(t, body, "Ravi Kumar · 98861 99999 · Catering")

	_, one := e.get("/return/new?q=" + url.QueryEscape("97740 11298"))
	assertContains(t, one, "Ravi Kumar · Security · 97740 11298 — still holding")
	assertContains(t, one, "6 out")
	assertNotContains(t, one, "8 out")
}

func TestFindByMobileFragmentShowsRavisLines(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	rows := askPeople(t, e, "q=98861")
	if len(rows) != 1 || rows[0].Name != "Ravi Menon" {
		t.Fatalf("the finder answered %+v, want Ravi Menon alone", peopleLabels(rows))
	}

	_, body := e.get("/return/new?q=98861")
	assertContains(t, body, "Ravi Menon · Catering · 98861 40023 — still holding")

	want := []string{
		"Issued 9:40 am by Suresh Kumar · 40 taken, 0 back",
		"Issued 2:18 pm by Anita Rao · 10 taken, 0 back",
		"Issued 9:40 am by Suresh Kumar · 2 taken, 0 back",
	}
	assertInOrder(t, body, want...)
	assertInOrder(t, body, "40 out", "10 out", "2 out")
}

// assertInOrder insists the sentences appear on the page in the order given.
func assertInOrder(t *testing.T, body string, want ...string) {
	t.Helper()
	from := 0
	for _, w := range want {
		at := strings.Index(body[from:], w)
		if at < 0 {
			t.Fatalf("the page does not contain %q after the previous line", w)
		}
		from += at + len(w)
	}
}

func TestPickingChairsSelectsBothChairLines(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=98861&productId=PRD-0001")
	if n := strings.Count(body, `class="outrow pick"`); n != 2 {
		t.Errorf("%d rows are picked, want the two chair lines", n)
	}
	if n := strings.Count(body, `class="outrow"`); n != 1 {
		t.Errorf("%d rows are left unpicked, want the round tables alone", n)
	}
	assertContains(t, body, "Chairs coming back")
	assertContains(t, body, `value="50"`)
	assertContains(t, body, `max="50"`)
	assertContains(t, body, "Take back 50 chairs")
}

func TestPickingTablesDefaultsToTwo(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=98861&productId=PRD-0002")
	assertContains(t, body, "Round tables coming back")
	assertContains(t, body, `value="2"`)
	assertContains(t, body, `max="2"`)
	assertContains(t, body, "Take back 2 round tables")
}

// shortfallStrings are every sentence the shortfall block puts on the screen.
var shortfallStrings = []string{
	"5 chairs missing. Ravi Menon still has them.",
	"5 chairs are missing. Say what happened before you save.",
	"What happened to the 5 chairs?",
	"Still expected back",
	"Won't come back — broken or lost",
	"Write it the way you would say it.",
}

func TestShortfallHintAtFortyFive(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=98861&productId=PRD-0001&quantity=45")
	for _, want := range shortfallStrings {
		assertContains(t, body, want)
	}
}

func TestNoShortfallBlockOnFullReturn(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=98861&productId=PRD-0001&quantity=50")
	for _, unwanted := range shortfallStrings {
		assertNotContains(t, body, unwanted)
	}
}

func TestReturn45Of50(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	resp, _ := e.post("/return/new", walkthroughReturn())
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/stock?saved=RET-0001" {
		t.Errorf("the save went to %q, want /stock?saved=RET-0001", got)
	}

	saved := e.saved()
	if len(saved.Returns) != 1 {
		t.Fatalf("the register holds %d returns, want 1", len(saved.Returns))
	}
	got := saved.Returns[0]
	want := register.WalkthroughT3().Returns[0]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the saved return is\n%+v\nwant\n%+v", got, want)
	}
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 925 {
		t.Errorf("chairs read %d on hand, want 925", onHand)
	}

	lines := register.OutstandingForPerson(saved, "Ravi Menon", "98861 40023")
	if len(lines) != 2 {
		t.Fatalf("Ravi is holding %d lines, want 2", len(lines))
	}
	if lines[0].ProductName != "Round tables" || lines[0].Out != 2 {
		t.Errorf("the first line is %d %s, want 2 Round tables", lines[0].Out, lines[0].ProductName)
	}
	if lines[1].IssueID != "ISS-0008" || lines[1].Out != 5 {
		t.Errorf("the second line is %d on %s, want 5 on ISS-0008", lines[1].Out, lines[1].IssueID)
	}

	_, body := e.get("/stock?saved=RET-0001")
	assertContains(t, body, "Took back 45 chairs. Chairs: 925 on hand.")
	assertContains(t, body, "Ravi Menon still has 5 chairs.")
}

func TestReturnAllocatesOldestFirst(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)
	e.post("/return/new", walkthroughReturn())

	saved := e.saved()
	if out := register.OutstandingOnIssue(saved, "ISS-0003"); out != 0 {
		t.Errorf("the 9:40 am issue has %d out, want 0", out)
	}
	if out := register.OutstandingOnIssue(saved, "ISS-0008"); out != 5 {
		t.Errorf("the 2:18 pm issue has %d out, want 5", out)
	}
	assertAllocationsAddUp(t, saved)
}

// assertAllocationsAddUp is acceptance criterion 4: every stored return's
// allocations sum to what came back on it.
func assertAllocationsAddUp(t *testing.T, reg *register.Register) {
	t.Helper()
	for _, re := range reg.Returns {
		total := 0
		for _, a := range re.Allocations {
			if a.Quantity < 1 {
				t.Errorf("%s stores an allocation of %d", re.ID, a.Quantity)
			}
			total += a.Quantity
		}
		if total != re.Quantity() {
			t.Errorf("%s allocates %d but came back as %d", re.ID, total, re.Quantity())
		}
	}
}

func TestFullReturnOf50(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("quantity", "50")
	form.Del("disposition")
	form.Del("remark")
	if resp, _ := e.post("/return/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the full return was refused")
	}

	saved := e.saved()
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 930 {
		t.Errorf("chairs read %d on hand, want 930", onHand)
	}
	lines := register.OutstandingForPerson(saved, "Ravi Menon", "98861 40023")
	if len(lines) != 1 || lines[0].ProductName != "Round tables" {
		t.Errorf("Ravi is holding %+v, want the 2 round tables alone", lines)
	}
	got := saved.Returns[0]
	if got.ShortQuantity != 0 || got.ShortDisposition != "" || got.Remark != "" {
		t.Errorf("the return notes a shortfall of %d %q %q, want none",
			got.ShortQuantity, got.ShortDisposition, got.Remark)
	}
	assertAllocationsAddUp(t, saved)
}

// refusedReturn posts a doctored return and insists on a 200, a sentence a
// person can act on, and nothing written.
func refusedReturn(t *testing.T, e *env, form url.Values, want string) string {
	t.Helper()
	before := len(e.saved().Returns)
	resp, body := e.post("/return/new", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the refusal returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, want)
	for _, unwanted := range []string{"invalid", "error", "nil", "panic"} {
		assertNotContains(t, body, unwanted)
	}
	if after := len(e.saved().Returns); after != before {
		t.Errorf("the register holds %d returns, want the original %d", after, before)
	}
	return body
}

func TestReturnRefuses51Of50(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("quantity", "51")
	refusedReturn(t, e, form, "Ravi Menon has 50 chairs. You cannot take back more than 50.")

	if onHand := register.OnHand(e.saved(), "PRD-0001"); onHand != 880 {
		t.Errorf("chairs read %d on hand, want 880", register.OnHand(e.saved(), "PRD-0001"))
	}
}

func TestReturnRefusesZeroAndGarbage(t *testing.T) {
	for _, quantity := range []string{"0", "-1", "abc", "4.5"} {
		t.Run(quantity, func(t *testing.T) {
			e := newTestServer(t, imranOnDuty(), sixOhFive)
			form := walkthroughReturn()
			form.Set("quantity", quantity)
			refusedReturn(t, e, form, "Type how many chairs are coming back.")
		})
	}
}

func TestShortReturnWithoutDispositionRefused(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("disposition", "")
	refusedReturn(t, e, form, "Tap one: the 5 chairs are coming back later, or they are gone.")
}

func TestShortReturnWithoutRemarkRefused(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("remark", "   ")
	refusedReturn(t, e, form, "Write what happened to the 5 chairs.")
}

func TestShortReturnExpectedBackKeepsSameNumbers(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("disposition", "expected")
	if resp, _ := e.post("/return/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the short return was refused")
	}

	saved := e.saved()
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 925 {
		t.Errorf("chairs read %d on hand, want 925", onHand)
	}
	lines := register.OutstandingForPerson(saved, "Ravi Menon", "98861 40023")
	chairs := 0
	for _, l := range lines {
		if l.ProductID == "PRD-0001" {
			chairs += l.Out
		}
	}
	if chairs != 5 {
		t.Errorf("Ravi is holding %d chairs, want the 5 that did not come back", chairs)
	}
	for _, row := range register.SupplierRows(saved) {
		if row.Supplier != "Sharma Tent House" || row.ProductID != "PRD-0001" {
			continue
		}
		if row.CameIn != 890 {
			t.Errorf("Sharma sent %d chairs, want 890", row.CameIn)
		}
		if row.WontComeBack != 0 {
			t.Errorf("the row notes %d broken or lost, want none", row.WontComeBack)
		}
	}
}

func TestReturnRefusesNoSelection(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Del("issueIds")
	refusedReturn(t, e, form, "Tap the row for the thing they are bringing back.")
}

func TestReturnRefusesMixedProducts(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form["issueIds"] = []string{"ISS-0003", "ISS-0005"}
	refusedReturn(t, e, form, "Tap the row for the thing they are bringing back.")
}

func TestReturnRefusesEmptyReturner(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("returnerName", "  ")
	refusedReturn(t, e, form, "Who is handing it back? Type their name.")
}

func TestReturnByADifferentPerson(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	form := walkthroughReturn()
	form.Set("returnerName", "Suresh Kumar")
	form.Set("returnerMobile", "98450 22117")
	if resp, _ := e.post("/return/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the return was refused")
	}

	saved := e.saved()
	if got := saved.Returns[0].ReturnerName; got != "Suresh Kumar" {
		t.Errorf("the return was handed in by %q, want Suresh Kumar", got)
	}
	if got := saved.Issues[2].TakerName; got != "Ravi Menon" {
		t.Errorf("the issue is now against %q, want Ravi Menon", got)
	}

	_, body := e.get("/stock?saved=RET-0001")
	assertContains(t, body, "Ravi Menon still has 5 chairs.")
	assertNotContains(t, body, "Suresh Kumar still has")

	// The five that did not come back are Ravi's, whoever carries the next lot
	// to the desk.
	again := walkthroughReturn()
	again["issueIds"] = []string{"ISS-0008"}
	again.Set("quantity", "6")
	again.Set("returnerName", "Suresh Kumar")
	body = refusedReturn(t, e, again, "Ravi Menon has 5 chairs. You cannot take back more than 5.")
	assertNotContains(t, body, "out with Suresh Kumar")
}

func TestSecondPartialReturnAgainstSameIssue(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)
	if resp, _ := e.post("/return/new", walkthroughReturn()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the first return was refused")
	}

	second := walkthroughReturn()
	second["issueIds"] = []string{"ISS-0008"}
	second.Set("quantity", "3")
	second.Set("disposition", "expected")
	second.Set("remark", "2 chairs still with the catering tent.")
	if resp, _ := e.post("/return/new", second); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the second return was refused")
	}

	saved := e.saved()
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 928 {
		t.Errorf("chairs read %d on hand, want 928", onHand)
	}
	if out := register.OutstandingOnIssue(saved, "ISS-0008"); out != 2 {
		t.Errorf("the 2:18 pm issue has %d out, want 2", out)
	}
	assertAllocationsAddUp(t, saved)

	third := second
	refusedReturn(t, e, third, "Ravi Menon has 2 chairs. You cannot take back more than 2.")
}

func TestReturnRevalidatesAtSaveTime(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)

	_, body := e.get("/return/new?q=98861&productId=PRD-0001")
	assertContains(t, body, "Take back 50 chairs")

	// Another desk takes 48 of them back while this form sits open.
	if err := e.st.Update(func(reg *register.Register) error {
		reg.Returns = append(reg.Returns, register.Return{
			ID: "RET-0001", ProductID: "PRD-0001",
			Allocations: []register.Allocation{
				{IssueID: "ISS-0003", Quantity: 40}, {IssueID: "ISS-0008", Quantity: 8},
			},
			ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
			TakenBackBy: "Imran Sheikh", ReturnedAt: sixOhFive, RecordedAt: sixOhFive,
		})
		return nil
	}); err != nil {
		t.Fatalf("mutating the store: %v", err)
	}

	refusedReturn(t, e, walkthroughReturn(), "Ravi Menon has 2 chairs. You cannot take back more than 2.")
}

func TestReturnIsAtomicOnDisk(t *testing.T) {
	e := newTestServer(t, imranOnDuty(), sixOhFive)
	e.post("/return/new", walkthroughReturn())

	raw, err := os.ReadFile(e.path + ".bak")
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	var backup register.Register
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatalf("the backup does not parse: %v", err)
	}
	if len(backup.Returns) != 0 {
		t.Errorf("the backup holds %d returns, want 0", len(backup.Returns))
	}
	if n := len(e.saved().Returns); n != 1 {
		t.Errorf("the register holds %d returns, want 1", n)
	}
}

// TestOnlyProductRouteCreatesProducts is the test 06 deferred until all three
// entry flows existed.
func TestOnlyProductRouteCreatesProducts(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	if resp, _ := e.post("/inward/new", walkthroughInward()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the inward was refused")
	}
	if resp, _ := e.post("/issue/new", walkthroughIssue()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the issue was refused")
	}
	if resp, _ := e.post("/return/new", walkthroughReturn()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the return was refused")
	}
	if n := len(e.saved().Products); n != 5 {
		t.Fatalf("the register holds %d products, want 5", n)
	}

	before := len(e.saved().Inwards)
	form := walkthroughInward()
	form.Set("productId", "PRD-9999")
	resp, body := e.post("/inward/new", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the refusal returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "banner bad")
	assertContains(t, body, "Pick the product from the list.")

	saved := e.saved()
	if n := len(saved.Products); n != 5 {
		t.Errorf("the register holds %d products, want 5", n)
	}
	if n := len(saved.Inwards); n != before {
		t.Errorf("the register holds %d inwards, want %d", n, before)
	}
}
