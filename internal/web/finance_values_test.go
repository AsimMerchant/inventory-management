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

func suggestValues(t *testing.T, tc *testClient, kind, q string) []string {
	t.Helper()
	status, body := tc.get(t, "/finance/api/values?kind="+kind+"&q="+url.QueryEscape(q))
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

	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		for kind, texts := range map[register.FinanceValueKind][]string{
			register.FinancePurpose: {"Frieght"},
			register.FinanceMode:    {"Online payment"},
			register.FinanceParty:   {"Sharm Events", "Sharma Events"},
		} {
			for _, text := range texts {
				if _, err := register.AddFinanceValue(f, kind, text, adminID, financeNow); err != nil {
					return err
				}
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
	if status, body := admin.post(t, "/finance/lists/"+online+"/merge", url.Values{"target": {bank}}); status != 303 {
		t.Fatalf("merge=%d %s", status, body)
	}

	// Delete a typo nothing points at.
	sharm := valueIDByText(t, e, key, register.FinanceParty, "Sharm Events")
	if status, body := admin.post(t, "/finance/lists/"+sharm+"/delete", nil); status != 303 {
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
		if _, ok := register.FinanceValueByID(f, sharm); ok {
			t.Error("the unused typo was not removed")
		}

		kinds := map[string]bool{}
		for _, a := range f.Audit {
			kinds[a.Kind] = true
			if a.EntityType == "reusableValue" && (a.ByAccountID != adminID || a.ByMobile != "9886140023") {
				t.Errorf("audit actor is %+v", a)
			}
		}
		for _, want := range []string{"value_renamed", "value_merged", "value_deleted"} {
			if !kinds[want] {
				t.Errorf("no %s audit event", want)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	// A value something points at is never erased. Bank transfer is now a merge
	// target, so it is in use even though no order names it.
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/lists/"+bank+"/delete", nil)
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
	user, userID := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")

	// An ordinary financial user sees and reuses the suggestions.
	if got := suggestValues(t, user, "mode", "ca"); strings.Join(got, ",") != "Card,Cash" {
		t.Fatalf("user was suggested %v", got)
	}
	// And may add a value as part of recording something.
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		_, err := register.AddFinanceValue(f, register.FinanceParty, "Sharma Events", userID, financeNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sharma := valueIDByText(t, e, key, register.FinanceParty, "Sharma Events")

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
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if got := register.FinanceValueText(f, sharma); got != "Sharma Events" {
			t.Errorf("the value now reads %q", got)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFinanceSuggestionOrderingSelectionAndNoScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)

	// Ten parties, so the eight-row cap and the ordering both have to bite.
	parties := []string{
		"Sharma Events", "Sharma Tent House", "Sharma Lights", "Sharma Sound",
		"Sharma Catering", "Sharma Decorators", "Sharma Transport", "Sharma Chairs",
		"New Sharma Supplies", "Anand Sharma & Sons",
	}
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		for _, p := range parties {
			if _, err := register.AddFinanceValue(f, register.FinanceParty, p, adminID, orderNow); err != nil {
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
	sharma := valueIDByText(t, e, key, register.FinanceParty, "Sharma Events")
	form := orderForm("", chairs, "10", "rent")
	form.Set("partyId", sharma)
	if status, body := admin.post(t, "/finance/orders/new", form); status != 303 {
		t.Fatalf("selecting by id = %d: %s", status, body)
	}

	// The server-only path: a genuinely new value typed with no ID, created as
	// part of the save. Freight and Product adjustment are the spec's examples.
	before := 0
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) { before = len(f.ReusableValues) })
	typed := orderForm("Freight Movers", chairs, "10", "rent")
	if status, body := admin.post(t, "/finance/orders/new", typed); status != 303 {
		t.Fatalf("typing a new party = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.ReusableValues) != before+1 {
			t.Errorf("the new party did not appear on the shared list")
		}
		if _, ok := register.FindFinanceValueByText(f, register.FinanceParty, "Freight Movers"); !ok {
			t.Error("Freight Movers was not created")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// A stale or unknown ID falls back to the typed text rather than filing the
	// order against the wrong party.
	stale := orderForm("Product adjustment", chairs, "10", "rent")
	stale.Set("partyId", "PTY-9999")
	if status, body := admin.post(t, "/finance/orders/new", stale); status != 303 {
		t.Fatalf("a stale id = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		last := f.Orders[len(f.Orders)-1]
		if got := register.FinanceValueText(f, last.PartyID); got != "Product adjustment" {
			t.Errorf("the order was filed against %q", got)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Neither an ID nor text is refused: the party is mandatory.
	blank := orderForm("", chairs, "10", "rent")
	if status, body := admin.post(t, "/finance/orders/new", blank); status != 200 ||
		!strings.Contains(body, "Say who this order is with.") {
		t.Errorf("a blank party gave %d", status)
	}
}
