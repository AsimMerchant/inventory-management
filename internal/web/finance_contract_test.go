package web

import (
	"bytes"
	"html"
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
)

func TestSetupCodePageExplainsOneTimeTwentyFourHourUse(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	admin, _, _ := financeAdmin(t, e)
	status, body := admin.post(t, "/finance/accounts/new", url.Values{
		"name": {"Rohan Das"}, "mobile": {"9900134562"}, "role": {"user"},
	})
	if status != 200 {
		t.Fatalf("authorize = %d", status)
	}
	for _, want := range []string{
		"Give this setup code to Rohan Das",
		"This code works once and expires in 24 hours. Give it only to Rohan Das.",
		"choose Activate my account, and create their own password.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("setup-code page missing %q", want)
		}
	}
}

func TestRecoveryKeyReplacementWarnsAndConfirmsTwice(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	c := financeClient(t)
	old, csrf := setupFinanceWeb(t, e, c)
	financeRequest(t, c, "POST", e.URL+"/finance/setup/confirm", url.Values{"csrf": {csrf}, "saved": {"yes"}})
	_, first := financeRequest(t, c, "GET", e.URL+"/finance/recovery-key/new", nil)
	warning := "The current recovery key will stop working. You must save and confirm the new key before you can return to the financial ledger."
	if !strings.Contains(first, warning) || !strings.Contains(first, "Continue to replace key") {
		t.Fatal("first replacement warning is incomplete")
	}
	before := mustReadFile(t, e.path)
	status, second := financeRequest(t, c, "POST", e.URL+"/finance/recovery-key/new", url.Values{"csrf": {csrfFrom(t, first)}})
	if status.StatusCode != 200 || !strings.Contains(second, warning) || !strings.Contains(second, "Yes, replace recovery key") || !strings.Contains(second, "Current password") {
		t.Fatal("second replacement warning is incomplete")
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("first replacement POST wrote the store")
	}
	_, final := financeRequest(t, c, "POST", e.URL+"/finance/recovery-key/new", url.Values{
		"csrf": {csrfFrom(t, second)}, "confirm": {"yes"}, "password": {"correct horse"},
	})
	if !strings.Contains(final, "Save this recovery key") || strings.Contains(final, old) {
		t.Fatal("confirmed replacement did not show only the new recovery key")
	}
	if _, err := e.st.UnlockFinanceRecovery(old); err == nil {
		t.Fatal("old recovery key still works")
	}
}

func TestAuthorizationFormsAreReadableStructuredSections(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	public := financeClient(t)
	checks := []struct{ path, heading string }{
		{"/finance/setup", "Set up authorized access"},
		{"/finance/login", "Log in to authorized access"},
		{"/finance/activate", "Activate your account"},
		{"/finance/recover", "Recover authorized access"},
	}
	for _, tc := range checks {
		_, body := financeRequest(t, public, "GET", e.URL+tc.path, nil)
		if !strings.Contains(body, `class="auth-section"`) || !strings.Contains(body, "<legend>"+tc.heading+"</legend>") || !strings.Contains(body, `class="lab"`) {
			t.Errorf("%s is not a labelled auth section", tc.path)
		}
	}
	_, setup := financeRequest(t, public, "GET", e.URL+"/finance/setup", nil)
	_, refused := financeRequest(t, public, "POST", e.URL+"/finance/setup", url.Values{
		"csrf": {csrfFrom(t, setup)}, "name": {"Asha Mehta"}, "mobile": {"98861 40023"},
		"password": {"secret-one"}, "again": {"secret-two"},
	})
	if !strings.Contains(refused, `value="Asha Mehta"`) || !strings.Contains(refused, `value="98861 40023"`) || strings.Contains(refused, "secret-one") || strings.Contains(refused, "secret-two") {
		t.Fatal("setup refusal did not preserve only non-secret input")
	}
}

func TestReusableListCombineAndDeleteRequireImpactConfirmation(t *testing.T) {
	// The full rename/combine/delete behavior test also asserts the mutations.
	// This test fixes the first-step contract and the changed-target recheck.
	e := newTestServer(t, nil, financeNow)
	admin, key, actor := financeAdmin(t, e)
	var source, target, changed string
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		var err error
		if source, err = register.AddFinanceValue(f, register.FinanceParty, "Sharm Events", actor, financeNow); err != nil {
			return err
		}
		if target, err = register.AddFinanceValue(f, register.FinanceParty, "Sharma Events", actor, financeNow); err != nil {
			return err
		}
		changed, err = register.AddFinanceValue(f, register.FinanceParty, "Sharma Event Hire", actor, financeNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/lists/"+source+"/merge", url.Values{"target": {target}})
	if status != 200 || !strings.Contains(body, "Combine Sharm Events into Sharma Events?") || !strings.Contains(body, "Yes, combine these values") {
		t.Fatal("combine consequence page is incomplete")
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("combine first step wrote")
	}
	status, body = admin.post(t, "/finance/lists/"+source+"/merge", url.Values{
		"target": {changed}, "confirmedTarget": {target}, "confirm": {"yes"},
	})
	if status != 200 || !strings.Contains(body, "Combine Sharm Events into Sharma Event Hire?") {
		t.Fatal("changed target did not receive fresh impact")
	}
}

func TestOrderCancellationWarnsBeforeSecondConfirmation(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	id := saveOrder(t, e, admin, key, orderForm("Sharma Events", productIDNamed(t, e, "Chairs"), "100", "rent"))
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/orders/"+id+"/cancel", nil)
	if status != 200 || !strings.Contains(body, "Existing payments, receipts and stock entries will not change. Record any refund separately.") || !strings.Contains(body, "Yes, cancel this order") {
		t.Fatal("order cancellation consequence page is incomplete")
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("cancellation first step wrote")
	}
}

func TestMovementVoidWarnsAndRequiresSecondConfirmation(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	id := saveMoney(t, e, admin, key, moneyForm("out", "5000", "Sharma Events", "Deposit", "Cash"))
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/movements/"+id+"/void", nil)
	if status != 200 || !strings.Contains(body, "record money in the opposite direction instead") || !strings.Contains(body, "Yes, void this transaction") {
		t.Fatal("movement void consequence page is incomplete")
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("movement void first step wrote")
	}
}

func TestJournalFilterPrecedenceDoesNotFallBack(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	id := saveMoney(t, e, admin, key, moneyForm("out", "9876", "Sharma Events", "Deposit", "Cash"))
	_ = id
	_, body := admin.get(t, "/finance/journal?fromTime=2016-01-01T23%3A00&day=2026-09-03")
	if !strings.Contains(body, "Choose both an exact start and exact end time.") || strings.Contains(body, "₹9,876.00") {
		t.Fatal("partial exact range fell back to a lower filter")
	}
	_, body = admin.get(t, "/finance/journal?day=bad&from=2026-09-01&to=2026-09-05")
	if !strings.Contains(body, register.JournalRefusal) || strings.Contains(body, "₹9,876.00") {
		t.Fatal("invalid day fell back to the date range")
	}
}

func TestJournalPrintKeepsMovementRecorderSnapshot(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	saveMoney(t, e, admin, key, moneyForm("out", "5000", "Sharma Events", "Deposit", "Cash"))
	if err := e.st.EditFinanceAccount(key, adminID, adminID, "Asha Updated", "9000000000", register.FinanceAdmin, orderNow); err != nil {
		t.Fatal(err)
	}
	_, printed := admin.get(t, "/finance/journal/print")
	if !strings.Contains(printed, "Recorded by Asha Mehta · 9886140023") || strings.Contains(printed, "Recorded by Asha Updated") {
		t.Fatal("print did not retain the movement creation identity")
	}
}

func TestSettlementVoidWarnsBeforeSecondConfirmation(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	receive(t, e, chairs, 10, "purchase", "Gupta Traders", "2026-09-03")
	admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 5, "2026-09-03T16:00"))
	_, sales := settlements(t, e, key)
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/settlements/sale/"+sales[0].ID+"/void", nil)
	if status != 200 || !strings.Contains(body, "Stock will be put back into the store totals") || !strings.Contains(body, "Any linked sale proceeds will not change.") || !strings.Contains(body, "Yes, void this stock movement") {
		t.Fatal("settlement void consequence page is incomplete")
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("settlement void first step wrote")
	}
}

func TestSupplierObligationsUsePlainHeadingAndColumns(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")
	receive(t, e, tables, 10, "rent", "Supplier not yet protected", "2026-09-03")
	_, body := admin.get(t, "/finance/obligations")
	for _, want := range []string{"Rented goods still to return", "Based only on goods actually received", "Supplier", "Product", "Received", "Returned to supplier", "Still to return", "Supplier not yet protected"} {
		if !strings.Contains(body, want) {
			t.Errorf("obligations missing %q", want)
		}
	}
}

func TestPlainLanguageContractsAndConfirmationSteps(t *testing.T) {
	// This compact gate checks response-level details shared by the focused
	// tests above: standalone print output and the paid/received mode refusal.
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	_, printed := admin.get(t, "/finance/journal/print")
	for _, unwanted := range []string{"Authorized people", "Reusable lists", "/finance/logout"} {
		if strings.Contains(printed, unwanted) {
			t.Errorf("print contains action %q", unwanted)
		}
	}
	if !strings.Contains(printed, `data-print`) || !strings.Contains(printed, `/static/print.js`) {
		t.Error("print view omitted its print control")
	}
	form := moneyForm("in", "5", "Sharma Events", "Refund", "")
	status, body := admin.post(t, "/finance/movements/new", form)
	if status != 200 || !strings.Contains(html.UnescapeString(body), "Say how the money was paid or received.") {
		t.Fatal("received-money mode refusal is unclear")
	}
}
