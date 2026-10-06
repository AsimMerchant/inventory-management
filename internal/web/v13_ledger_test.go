package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
)

func recordSomethingForm() url.Values {
	return url.Values{
		"occurredAt":        {"2026-09-03T12:40"},
		"partyName":         {"Sharma Tent House"},
		"purposeName":       {"Rent"},
		"modeName":          {"Cash"},
		"billed":            {"20000"},
		"paid":              {"5000"},
		"received":          {"2000"},
		"supplierChallanNo": {"STH/4390"},
		"reference":         {"REF-7"},
		"remarks":           {"Event chairs"},
		"productName-0-0":   {""},
		"product-0-0":       {"PRD-0001"},
		"qty-0-0":           {"500"},
		"basis-0-0":         {"rent"},
	}
}

func TestRecordSomethingPurposeIsMandatorySharedAndReusable(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	status, body := admin.get(t, "/finance/movements/new")
	if status != http.StatusOK {
		t.Fatalf("GET = %d", status)
	}
	for _, want := range []string{"Record something", "Date and time", "Supplier", "Purpose", "Payment mode", "Challan no.", "What this is for", "Product", "How many", "How these came in", "Billed", "Paid", "Received", "Reference", "Remarks", "Add another amount", "Save"} {
		assertContains(t, body, want)
	}
	for _, unwanted := range []string{`name="direction`, `name="amount"`, "Agreed total", "Part of an earlier order?", "Related stock return or sale"} {
		assertNotContains(t, body, unwanted)
	}

	blank := recordSomethingForm()
	blank.Set("purposeName", "")
	if status, body := admin.post(t, "/finance/movements/new", blank); status != http.StatusOK || !strings.Contains(body, "Say what this entry is for.") {
		t.Fatalf("blank purpose = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Bills) != 0 || len(f.Movements) != 0 {
			t.Error("refusal wrote an event")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMixedEntryCreatesBillPaidReceivedAtomically(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	form := recordSomethingForm()
	status, body := admin.post(t, "/finance/movements/new", form)
	if status != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", status, body)
	}
	var r *register.Register
	e.st.Read(func(got *register.Register) { r = got })
	if len(r.SupplierChallans) != 1 || r.SupplierChallans[0].Number != "STH/4390" {
		t.Fatalf("challans = %+v", r.SupplierChallans)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Orders) != 1 || len(f.Bills) != 1 || len(f.Movements) != 2 {
			t.Fatalf("orders/bills/movements = %d/%d/%d", len(f.Orders), len(f.Bills), len(f.Movements))
		}
		if f.Orders[0].AgreedPaise != nil || f.Orders[0].AgreedKind != "" {
			t.Error("new order has a historical agreement")
		}
		b, paid, received := f.Bills[0], f.Movements[0], f.Movements[1]
		if b.PurposeID == "" || b.PurposeID != paid.PurposeID || b.PurposeID != received.PurposeID || b.SupplierChallanID != paid.SupplierChallanID || b.SupplierChallanID != received.SupplierChallanID {
			t.Fatal("events do not share purpose/challan")
		}
		if paid.Direction != register.MoneyOut || received.Direction != register.MoneyIn {
			t.Fatalf("directions = %s/%s", paid.Direction, received.Direction)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBillOnlyNeedsNoPaymentMode(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	form := recordSomethingForm()
	form.Set("paid", "")
	form.Set("received", "")
	form.Set("modeName", "")
	if status, body := admin.post(t, "/finance/movements/new", form); status != http.StatusSeeOther {
		t.Fatalf("bill-only = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Bills) != 1 || len(f.Movements) != 0 {
			t.Fatalf("bills/movements = %d/%d", len(f.Bills), len(f.Movements))
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBillCorrectionAndVoidKeepCompleteHistory(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	form := recordSomethingForm()
	form.Set("paid", "")
	form.Set("received", "")
	if status, body := admin.post(t, "/finance/movements/new", form); status != http.StatusSeeOther {
		t.Fatalf("seed = %d: %s", status, body)
	}
	edit := url.Values{
		"amount": {"21500"}, "occurredAt": {"2026-09-03T12:45"},
		"partyName": {"Sharma Tent House"}, "purposeName": {"Damage charge"},
		"supplierChallanNo": {"STH/4390-A"}, "reference": {"REF-8"}, "remarks": {"Corrected"},
	}
	if status, body := admin.post(t, "/finance/bills/BIL-0001/edit", edit); status != http.StatusSeeOther {
		t.Fatalf("edit = %d: %s", status, body)
	}
	if status, body := admin.post(t, "/finance/bills/BIL-0001/void", url.Values{}); status != http.StatusOK || !strings.Contains(body, "Void this bill?") {
		t.Fatalf("first void = %d: %s", status, body)
	}
	if status, body := admin.post(t, "/finance/bills/BIL-0001/void", url.Values{"confirm": {"yes"}, "reason": {"Entered twice"}}); status != http.StatusSeeOther {
		t.Fatalf("confirmed void = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		bill := f.Bills[0]
		if bill.AmountPaise != 2150000 || len(bill.Changes) != 6 || bill.Voided == nil || bill.Voided.Reason != "Entered twice" {
			t.Errorf("bill history = %+v", bill)
		}
		if total, _ := register.TotalBills(f, nil); total != 0 {
			t.Errorf("voided total = %d", total)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestJournalCombinesBillsAndMoneyAcrossFilters(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	if status, body := admin.post(t, "/finance/movements/new", recordSomethingForm()); status != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", status, body)
	}
	_, body := admin.get(t, "/finance/journal?day=2026-09-03")
	if strings.Index(body, "Billed ₹20,000.00") > strings.Index(body, "Paid ₹5,000.00") || strings.Index(body, "Paid ₹5,000.00") > strings.Index(body, "Received ₹2,000.00") {
		t.Fatalf("event order is wrong: %s", body)
	}
	for _, want := range []string{"Billed", "Paid", "Received", "STH/4390", "Fix this bill", "Review voiding this bill"} {
		assertContains(t, body, want)
	}
	_, printed := admin.get(t, "/finance/journal/print?day=2026-09-03")
	for _, want := range []string{"<th>Entry</th>", "<th>Challan no.</th>", "Billed ₹20,000.00", "Paid ₹5,000.00", "Received ₹2,000.00"} {
		if !strings.Contains(printed, want) {
			t.Errorf("print missing %q", want)
		}
	}
}

func TestSupplierChallanVocabularyIsPublicButUnlinked(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	form := recordSomethingForm()
	if status, body := admin.post(t, "/finance/movements/new", form); status != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", status, body)
	}
	var partyID, challanID string
	e.st.Read(func(r *register.Register) {
		partyID, challanID = r.SupplierChallans[0].PartyID, r.SupplierChallans[0].ID
	})
	resp, body := e.get("/api/supplier-challans?partyId=" + partyID + "&q=4390")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("api = %d", resp.StatusCode)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 3 || rows[0]["id"] != challanID || rows[0]["value"] != "STH/4390" || rows[0]["label"] != "STH/4390" {
		t.Fatalf("api rows = %#v", rows)
	}

	before := len(e.saved().SupplierChallans)
	inward := url.Values{
		"productId": {"PRD-0001"}, "productName": {"Chairs"}, "quantity": {"1"},
		"receivedOn": {"2026-09-03"}, "basis": {"rent"}, "partyId": {partyID},
		"supplierChallanIdChoice": {challanID},
	}
	if resp, body := e.post("/inward/new", inward); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("inward = %d: %s", resp.StatusCode, body)
	}
	saved := e.saved()
	if len(saved.SupplierChallans) != before || saved.Inwards[len(saved.Inwards)-1].ChallanNo != "STH/4390" {
		t.Fatalf("inward coupled vocabulary: %+v", saved.Inwards[len(saved.Inwards)-1])
	}
}
