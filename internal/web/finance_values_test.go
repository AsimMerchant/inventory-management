package web

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// financeAdmin sets up the vault and returns a logged-in admin browser.
func financeAdmin(t *testing.T, e *env) (*testClient, []byte, string) {
	t.Helper()
	key, admin, _, err := e.st.InitializeFinance("Asha Mehta", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.ConfirmFinanceRecovery(key, admin, financeNow); err != nil {
		t.Fatal(err)
	}
	return financeLogin(t, e, "9886140023", "correct horse"), key, admin
}

// financeUser authorizes and activates an ordinary financial account and
// returns a logged-in browser for it.
func financeUser(t *testing.T, e *env, key []byte, admin, name, mobile, password string) (*testClient, string) {
	t.Helper()
	id, code, err := e.st.AuthorizeFinanceAccount(key, admin, name, mobile, register.FinanceUser, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.ActivateFinance(mobile, code, password, financeNow); err != nil {
		t.Fatal(err)
	}
	return financeLogin(t, e, mobile, password), id
}

// testClient is a logged-in finance browser plus the CSRF token its session
// needs on every POST.
type testClient struct {
	e    *env
	c    *http.Client
	csrf string
}

func (tc *testClient) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, body := financeRequest(t, tc.c, "GET", tc.e.URL+path, nil)
	return resp.StatusCode, body
}

func (tc *testClient) post(t *testing.T, path string, form url.Values) (int, string) {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set("csrf", tc.csrf)
	resp, body := financeRequest(t, tc.c, "POST", tc.e.URL+path, form)
	return resp.StatusCode, body
}

func financeLogin(t *testing.T, e *env, mobile, password string) *testClient {
	t.Helper()
	c := financeClient(t)
	_, body := financeRequest(t, c, "GET", e.URL+"/finance/login", nil)
	csrf := csrfFrom(t, body)
	resp, body := financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{
		"csrf": {csrf}, "mobile": {mobile}, "password": {password},
	})
	if resp.StatusCode != 303 {
		t.Fatalf("login for %s = %d: %s", mobile, resp.StatusCode, body)
	}
	_, body = financeRequest(t, c, "GET", e.URL+"/finance", nil)
	return &testClient{e: e, c: c, csrf: csrfFrom(t, body)}
}

func valueIDByText(t *testing.T, e *env, key []byte, kind register.FinanceValueKind, text string) string {
	t.Helper()
	id := ""
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if v, ok := register.FindFinanceValueByText(f, kind, text); ok {
			id = v.ID
		}
	}); err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatalf("no %s value called %q", kind, text)
	}
	return id
}

func partyIDByText(t *testing.T, e *env, text string) string {
	t.Helper()
	id := ""
	e.st.Read(func(r *register.Register) {
		if p, ok := register.FindPartyByText(r, text); ok {
			id = p.ID
		}
	})
	if id == "" {
		t.Fatalf("no party called %q", text)
	}
	return id
}

func partyText(t *testing.T, e *env, id string) string {
	t.Helper()
	text := ""
	e.st.Read(func(r *register.Register) { text = register.PartyText(r, id) })
	return text
}

func TestSharedPartyRouteHonorsBothAccessPathsAndNoLeakBoundary(t *testing.T) {
	reg := emptyRegister()
	reg.OnDutyStaffID = ""
	reg.ShiftStartedAt = nil
	e := newTestServer(t, reg, financeNow)
	admin, key, adminID := financeAdmin(t, e)

	// Reproduce a legacy protected party after the transaction's import step,
	// so it exists only in the encrypted reusable-value list until the next
	// successful financial write.
	partyID := ""
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		var err error
		partyID, err = register.AddFinanceValue(f, register.FinanceParty, "Sharma Events", adminID, financeNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.st.Read(func(r *register.Register) {
		if len(r.Parties) != 0 || r.OnDutyStaffID != "" || r.ShiftStartedAt != nil {
			t.Fatalf("legacy setup changed the public desk: %+v", r.Parties)
		}
	})

	checkResponse := func(body string) {
		t.Helper()
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatalf("party suggestions are not JSON: %v (%s)", err, body)
		}
		if len(rows) != 1 {
			t.Fatalf("party suggestions=%s", body)
		}
		allowed := map[string]bool{"id": true, "value": true, "label": true}
		for field := range rows[0] {
			if !allowed[field] {
				t.Errorf("party response leaked field %q", field)
			}
		}
		if len(rows[0]) != len(allowed) {
			t.Errorf("party response fields=%v, want exactly id/value/label", rows[0])
		}
		var id, value, label string
		_ = json.Unmarshal(rows[0]["id"], &id)
		_ = json.Unmarshal(rows[0]["value"], &value)
		_ = json.Unmarshal(rows[0]["label"], &label)
		if id != partyID || value != "Sharma Events" || label != "Sharma Events" {
			t.Errorf("party suggestion=%s", body)
		}
	}

	status, body := admin.get(t, "/api/parties?q=sharma")
	if status != http.StatusOK {
		t.Fatalf("authenticated party route=%d: %s", status, body)
	}
	checkResponse(body)
	// ReadBoth is read-only: neither the migration nor an inventory shift was
	// persisted merely because an authenticated picker was drawn.
	e.st.Read(func(r *register.Register) {
		if len(r.Parties) != 0 || r.OnDutyStaffID != "" || r.ShiftStartedAt != nil {
			t.Error("authenticated shared-party read changed public state")
		}
	})
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if v, ok := register.FinanceValueByID(f, partyID); !ok || v.Kind != register.FinanceParty {
			t.Error("authenticated shared-party read removed the protected legacy row")
		}
	}); err != nil {
		t.Fatal(err)
	}

	resp, _ := e.get("/api/parties?q=sharma")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/shift" {
		t.Errorf("off-duty anonymous party route=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}

	if err := e.st.UpdateFinance(key, func(*register.Register, *register.FinanceData) error { return nil }); err != nil {
		t.Fatal(err)
	}
	e.st.Read(func(r *register.Register) {
		if p, ok := register.PartyByID(r, partyID); !ok || p.Name != "Sharma Events" {
			t.Errorf("persistent migration produced %+v", p)
		}
	})
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if _, ok := register.FinanceValueByID(f, partyID); ok {
			t.Error("persistent migration left the protected party duplicate")
		}
	}); err != nil {
		t.Fatal(err)
	}
	started := financeNow.Add(time.Hour)
	if err := e.st.Update(func(r *register.Register) error {
		r.OnDutyStaffID = "STF-0001"
		r.ShiftStartedAt = &started
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resp, body = e.get("/api/parties?q=sharma")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("on-duty anonymous party route=%d: %s", resp.StatusCode, body)
	}
	checkResponse(body)
	resp, body = e.get("/inward/new")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("on-duty inward form=%d: %s", resp.StatusCode, body)
	}
	assertContains(t, body, `value="`+partyID+`"`)
	assertContains(t, body, "Sharma Events")
}

func suggestValues(t *testing.T, tc *testClient, kind, q string) []string {
	t.Helper()
	path := "/finance/api/values?kind=" + kind + "&q=" + url.QueryEscape(q)
	if kind == "party" {
		path = "/api/parties?q=" + url.QueryEscape(q)
	}
	status, body := tc.get(t, path)
	if status != 200 {
		t.Fatalf("suggestions for %q = %d", q, status)
	}
	var rows []valueSuggestion
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("suggestions were not JSON: %v (%s)", err, body)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Value)
	}
	return out
}

func TestInitialPaymentModesAndMandatorySuggestions(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	admin, key, adminID := financeAdmin(t, e)

	// The five modes exist in the spec's order, attributed to the first account.
	var stored []string
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		for _, v := range f.ReusableValues {
			if v.Kind == register.FinanceMode {
				stored = append(stored, v.Value)
				if v.CreatedByID != adminID {
					t.Errorf("%s attributed to %s", v.Value, v.CreatedByID)
				}
				if !v.CreatedAt.Equal(financeNow) {
					t.Errorf("%s created at %v", v.Value, v.CreatedAt)
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(stored, ",") != "Cash,UPI,Bank transfer,Cheque,Card" {
		t.Fatalf("initial modes are %v", stored)
	}

	if got := suggestValues(t, admin, "mode", "ba"); strings.Join(got, ",") != "Bank transfer" {
		t.Fatalf("typing ba suggested %v", got)
	}

	// A value one person adds is immediately available to another.
	user, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		_, err := register.AddFinanceValue(f, register.FinanceMode, "Online", adminID, financeNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := suggestValues(t, user, "mode", "onl"); strings.Join(got, ",") != "Online" {
		t.Fatalf("the other user was suggested %v", got)
	}

	// Exact and case-fold reuse never makes a second row saying the same thing.
	before, after := 0, 0
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) { before = len(f.ReusableValues) })
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		for _, text := range []string{"Online", "  online  ", "ONLINE"} {
			if _, err := register.AddFinanceValue(f, register.FinanceMode, text, adminID, financeNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) { after = len(f.ReusableValues) })
	if before != after {
		t.Fatalf("reuse grew the list from %d to %d", before, after)
	}
}

func TestAdminRenamesMergesAndDeletesReusableTypos(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	admin, key, adminID := financeAdmin(t, e)

	if err := e.st.UpdateFinance(key, func(reg *register.Register, f *register.FinanceData) error {
		for kind, texts := range map[register.FinanceValueKind][]string{
			register.FinancePurpose: {"Frieght"},
			register.FinanceMode:    {"Online payment"},
		} {
			for _, text := range texts {
				if _, err := register.AddFinanceValue(f, kind, text, adminID, financeNow); err != nil {
					return err
				}
			}
		}
		for _, text := range []string{"Sharm Events", "Sharma Events"} {
			if _, err := register.AddParty(reg, text); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Well inside the 15-minute idle expiry, so the session survives the jump.
	later := financeNow.Add(5 * time.Minute)
	e.srv.now = func() time.Time { return later }

	// Rename a typo.
	frieght := valueIDByText(t, e, key, register.FinancePurpose, "Frieght")
	if status, body := admin.post(t, "/finance/lists/"+frieght+"/rename", url.Values{"value": {"Freight"}}); status != 303 {
		t.Fatalf("rename=%d %s", status, body)
	}

	// Merge a duplicate wording into the one being kept.
	online := valueIDByText(t, e, key, register.FinanceMode, "Online payment")
	bank := valueIDByText(t, e, key, register.FinanceMode, "Bank transfer")
	beforeConfirm := mustReadFile(t, e.path)
	if status, body := admin.post(t, "/finance/lists/"+online+"/merge", url.Values{"target": {bank}}); status != 200 ||
		!strings.Contains(body, "Combine Online payment into Bank transfer?") || !strings.Contains(body, "Yes, combine these values") {
		t.Fatalf("merge confirmation=%d %s", status, body)
	}
	if string(beforeConfirm) != string(mustReadFile(t, e.path)) {
		t.Fatal("first merge step changed the register")
	}
	if status, body := admin.post(t, "/finance/lists/"+online+"/merge", url.Values{"target": {bank}, "confirmedTarget": {bank}, "confirm": {"yes"}}); status != 303 {
		t.Fatalf("merge=%d %s", status, body)
	}

	// Delete a typo nothing points at.
	sharm := partyIDByText(t, e, "Sharm Events")
	beforeConfirm = mustReadFile(t, e.path)
	if status, body := admin.post(t, "/finance/lists/"+sharm+"/delete", nil); status != 200 ||
		!strings.Contains(body, "Delete unused supplier or other party “Sharm Events”?") || !strings.Contains(body, "Yes, delete this unused value") {
		t.Fatalf("delete confirmation=%d %s", status, body)
	}
	if string(beforeConfirm) != string(mustReadFile(t, e.path)) {
		t.Fatal("first delete step changed the register")
	}
	if status, body := admin.post(t, "/finance/lists/"+sharm+"/delete", url.Values{"confirm": {"yes"}}); status != 303 {
		t.Fatalf("delete=%d %s", status, body)
	}

	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if got := register.FinanceValueText(f, frieght); got != "Freight" {
			t.Errorf("renamed value reads %q", got)
		}
		v, _ := register.FinanceValueByID(f, frieght)
		if len(v.Changes) != 1 {
			t.Fatalf("%d changes recorded", len(v.Changes))
		}
		ch := v.Changes[0]
		if ch.From != "Frieght" || ch.To != "Freight" || ch.Label != "Purpose" || ch.Field != "value" {
			t.Errorf("change is %+v", ch)
		}
		if ch.ByAccountID != adminID || ch.ByName != "Asha Mehta" || ch.ByMobile != "9886140023" || !ch.At.Equal(later) {
			t.Errorf("change actor is %+v", ch)
		}

		if got := register.FinanceValueText(f, online); got != "Bank transfer" {
			t.Errorf("merged value reads %q", got)
		}
		kinds := map[string]bool{}
		for _, a := range f.Audit {
			kinds[a.Kind] = true
			if a.EntityType == "reusableValue" && (a.ByAccountID != adminID || a.ByMobile != "9886140023") {
				t.Errorf("audit actor is %+v", a)
			}
		}
		for _, want := range []string{"value_renamed", "value_merged", "party_deleted"} {
			if !kinds[want] {
				t.Errorf("no %s audit event", want)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	e.st.Read(func(reg *register.Register) {
		if _, ok := register.PartyByID(reg, sharm); ok {
			t.Error("the unused typo was not removed")
		}
	})

	// A value something points at is never erased. Bank transfer is now a merge
	// target, so it is in use even though no order names it.
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/lists/"+bank+"/delete", url.Values{"confirm": {"yes"}})
	if status != 200 {
		t.Fatalf("used delete=%d", status)
	}
	if !strings.Contains(body, "This value has been used. Rename it or merge it instead.") {
		t.Fatalf("wrong refusal wording: %s", body)
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Fatal("a refused delete changed the register file")
	}
}

func TestFinancialUserCannotManageReusableValues(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	_, key, adminID := financeAdmin(t, e)
	user, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")

	// An ordinary financial user sees and reuses the suggestions.
	if got := suggestValues(t, user, "mode", "ca"); strings.Join(got, ",") != "Card,Cash" {
		t.Fatalf("user was suggested %v", got)
	}
	// And may add a value as part of recording something.
	if err := e.st.UpdateFinance(key, func(reg *register.Register, f *register.FinanceData) error {
		_, err := register.AddParty(reg, "Sharma Events")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sharma := partyIDByText(t, e, "Sharma Events")

	// But every list-management route is refused and writes nothing.
	before := mustReadFile(t, e.path)
	if status, _ := user.get(t, "/finance/lists"); status != 403 {
		t.Fatalf("the lists screen answered %d", status)
	}
	for path, form := range map[string]url.Values{
		"/finance/lists/" + sharma + "/rename": {"value": {"Sharma Event Supplies"}},
		"/finance/lists/" + sharma + "/merge":  {"target": {valueIDByText(t, e, key, register.FinanceMode, "Cash")}},
		"/finance/lists/" + sharma + "/delete": nil,
	} {
		if status, _ := user.post(t, path, form); status != 403 {
			t.Errorf("%s answered %d, want 403", path, status)
		}
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Fatal("a refused list action changed the register file")
	}
	if got := partyText(t, e, sharma); got != "Sharma Events" {
		t.Errorf("the value now reads %q", got)
	}
}

func TestFinanceSuggestionOrderingSelectionAndNoScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	// Ten parties, so the eight-row cap and the ordering both have to bite.
	parties := []string{
		"Sharma Events", "Sharma Tent House", "Sharma Lights", "Sharma Sound",
		"Sharma Catering", "Sharma Decorators", "Sharma Transport", "Sharma Chairs",
		"New Sharma Supplies", "Anand Sharma & Sons",
	}
	if err := e.st.UpdateFinance(key, func(reg *register.Register, f *register.FinanceData) error {
		for _, p := range parties {
			if _, err := register.AddParty(reg, p); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got := suggestValues(t, admin, "party", "sharma")
	if len(got) != 8 {
		t.Fatalf("%d suggestions, want the eight-row cap: %v", len(got), got)
	}
	// The eight names starting with the query, alphabetically. The two that
	// merely contain it are pushed out by the cap.
	want := "Sharma Catering,Sharma Chairs,Sharma Decorators,Sharma Events," +
		"Sharma Lights,Sharma Sound,Sharma Tent House,Sharma Transport"
	if strings.Join(got, ",") != want {
		t.Fatalf("ordering is %v", got)
	}
	// With a narrower query the cap no longer bites, and the one name that
	// merely contains the query comes after the one that starts with it.
	narrow := suggestValues(t, admin, "party", "sharma s")
	if strings.Join(narrow, ",") != "Sharma Sound,New Sharma Supplies" {
		t.Fatalf("substring matches come out as %v", narrow)
	}

	// The picker markup carries a hidden ID beside the visible text, and a
	// no-script select that lists every live value.
	status, body := admin.get(t, "/finance/orders/new")
	if status != 200 {
		t.Fatalf("the order form = %d", status)
	}
	for _, want := range []string{
		`name="partyName"`, `name="partyId"`, `data-values-id`, `data-values-text`,
		"<noscript>", "Or add a new one", `src="/static/value-picker.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the party picker has no %s", want)
		}
	}
	// html/template escapes an ampersand, so compare against the unescaped body.
	plain := html.UnescapeString(body)
	for _, p := range parties {
		if !strings.Contains(plain, ">"+p+"<") {
			t.Errorf("the no-script list omits %q", p)
		}
	}

	chairs := productIDNamed(t, e, "Chairs")

	// The server-only path: an existing value chosen by ID, with no typed text.
	sharma := partyIDByText(t, e, "Sharma Events")
	form := orderForm("", chairs, "10", "rent")
	form.Set("partyId", sharma)
	if status, body := admin.post(t, "/finance/orders/new", form); status != 303 {
		t.Fatalf("selecting by id = %d: %s", status, body)
	}

	// The server-only path: a genuinely new value typed with no ID, created as
	// part of the save. Freight and Product adjustment are the spec's examples.
	before := 0
	e.st.Read(func(reg *register.Register) { before = len(reg.Parties) })
	typed := orderForm("Freight Movers", chairs, "10", "rent")
	if status, body := admin.post(t, "/finance/orders/new", typed); status != 303 {
		t.Fatalf("typing a new party = %d: %s", status, body)
	}
	e.st.Read(func(reg *register.Register) {
		if len(reg.Parties) != before+1 {
			t.Errorf("the new party did not appear on the shared list")
		}
		if _, ok := register.FindPartyByText(reg, "Freight Movers"); !ok {
			t.Error("Freight Movers was not created")
		}
	})

	// A stale or unknown ID falls back to the typed text rather than filing the
	// order against the wrong party.
	stale := orderForm("Product adjustment", chairs, "10", "rent")
	stale.Set("partyId", "PTY-9999")
	if status, body := admin.post(t, "/finance/orders/new", stale); status != 303 {
		t.Fatalf("a stale id = %d: %s", status, body)
	}
	lastPartyID := ""
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		lastPartyID = f.Orders[len(f.Orders)-1].PartyID
	}); err != nil {
		t.Fatal(err)
	}
	if got := partyText(t, e, lastPartyID); got != "Product adjustment" {
		t.Errorf("the order was filed against %q", got)
	}

	// Neither an ID nor text is refused: the party is mandatory.
	blank := orderForm("", chairs, "10", "rent")
	if status, body := admin.post(t, "/finance/orders/new", blank); status != 200 ||
		!strings.Contains(body, "Say who this order is with.") {
		t.Errorf("a blank party gave %d", status)
	}
}
