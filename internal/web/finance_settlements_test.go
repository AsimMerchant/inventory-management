package web

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// settleForm is one supplier return or sale as the screen submits it.
func settleForm(party, productID string, quantity int, at string) url.Values {
	return url.Values{
		"partyName": {party}, "productId": {productID},
		"quantity": {strconv.Itoa(quantity)}, "at": {at},
	}
}

// receive records an ordinary inward at the desk, exactly as spec 07 does.
func receive(t *testing.T, e *env, productID string, quantity int, basis, supplier, on string) {
	t.Helper()
	resp, body := e.post("/inward/new", url.Values{
		"productId": {productID}, "quantity": {strconv.Itoa(quantity)},
		"basis": {basis}, "supplier": {supplier}, "receivedOn": {on},
	})
	if resp.StatusCode != 303 {
		t.Fatalf("receiving %d = %d: %s", quantity, resp.StatusCode, body)
	}
}

func stockOf(t *testing.T, e *env, productID string) int {
	t.Helper()
	n := 0
	e.st.Read(func(reg *register.Register) { n = register.OnHand(reg, productID) })
	return n
}

func settlements(t *testing.T, e *env, key []byte) (returns []register.SupplierReturn, sales []register.StockSale) {
	t.Helper()
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		returns = append(returns, f.SupplierReturns...)
		sales = append(sales, f.Sales...)
	}); err != nil {
		t.Fatal(err)
	}
	return
}

// emptyStock is a register with products and a live shift but nothing received,
// so each test states its own receipts.
func emptyStock() *register.Register {
	r := register.WalkthroughT0()
	r.Inwards = []register.Inward{}
	r.Issues = []register.Issue{}
	r.Returns = []register.Return{}
	return r
}

func TestSupplierReturnAndSaleMoveStockButNoMoney(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tents := "" // created deliberately through the finance route below

	if status, _ := admin.post(t, "/finance/product/new", url.Values{
		"line": {"0"}, "partyName": {"Sharma Events"},
		"productId": {""}, "productName": {"Tents"},
		"quantity": {"100"}, "basis": {"rent"}, "orderedAt": {"2026-09-03T10:00"},
	}); status != 303 {
		t.Fatal("creating Tents failed")
	}
	tents = productIDNamed(t, e, "Tents")

	receive(t, e, tents, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, chairs, 60, "purchase", "Gupta Traders", "2026-09-03")

	// Return 30 tents to the supplier they came from.
	if status, body := admin.post(t, "/finance/supplier-returns/new",
		settleForm("Sharma Events", tents, 30, "2026-09-03T15:00")); status != 303 {
		t.Fatalf("supplier return = %d: %s", status, body)
	}
	// Sell 20 purchased chairs.
	if status, body := admin.post(t, "/finance/sales/new",
		settleForm("Patel Decorators", chairs, 20, "2026-09-03T16:00")); status != 303 {
		t.Fatalf("sale = %d: %s", status, body)
	}

	if got := stockOf(t, e, tents); got != 70 {
		t.Errorf("Tents on hand is %d, want 70", got)
	}
	if got := stockOf(t, e, chairs); got != 40 {
		t.Errorf("Chairs on hand is %d, want 40", got)
	}
	// Neither created any money by itself.
	if got := totals(t, e, key); got.PaidPaise != 0 || got.ReceivedPaise != 0 {
		t.Errorf("a physical settlement created money: %+v", got)
	}

	// The public record says nothing about which kind of exit it was, who it
	// was with, or for how much.
	raw := string(mustReadFile(t, e.path))
	// The inward's own supplier is ordinary public inventory data and is meant
	// to be readable. What must not appear is anything that says a settlement
	// happened, with whom, or for how much.
	if !strings.Contains(raw, "Sharma Events") {
		t.Error("the inward supplier should stay readable public data")
	}
	for _, secret := range []string{
		"supplierReturns", "\"sales\"", "Patel Decorators",
		"amountPaise", "partyId", "buyerPartyId", "SRN-", "SAL-",
	} {
		if strings.Contains(raw, secret) {
			t.Errorf("%q is readable in the register file", secret)
		}
	}
	var disposals []register.InventoryDisposal
	e.st.Read(func(reg *register.Register) { disposals = append(disposals, reg.Disposals...) })
	if len(disposals) != 2 {
		t.Fatalf("%d stock removals recorded", len(disposals))
	}
	for _, d := range disposals {
		if d.ID == "" || d.ProductID == "" || d.Quantity < 1 || len(d.Sources) == 0 {
			t.Errorf("a stock removal is incomplete: %+v", d)
		}
	}
	// Two removals of different kinds are indistinguishable in public.
	if disposals[0].ProductID == disposals[1].ProductID {
		t.Fatal("the two removals should be for different products")
	}
}

func TestOnHandSubtractsPublicDisposalsAfterRestartWithoutLogin(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, chairs, 60, "purchase", "Gupta Traders", "2026-09-03")
	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 30, "2026-09-03T15:00"))
	admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 20, "2026-09-03T16:00"))
	_ = key

	// Reopen with nobody logged in at all.
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Read(func(reg *register.Register) {
		if got := register.OnHand(reg, tables); got != 70 {
			t.Errorf("after restart Round tables are %d, want 70", got)
		}
		if got := register.OnHand(reg, chairs); got != 40 {
			t.Errorf("after restart Chairs are %d, want 40", got)
		}
		// The issue guard uses the same number.
		if err := register.CheckIssue(reg, tables, 71, orderNow); err == nil {
			t.Error("issuing more than is left was allowed")
		}
		if err := register.CheckIssue(reg, tables, 70, orderNow); err != nil {
			t.Errorf("issuing exactly what is left was refused: %v", err)
		}
	})

	// And the ordinary Stock screen shows it without any protected detail.
	_, body := e.get("/stock")
	for _, secret := range []string{"Sharma Events", "Patel Decorators", "₹", "Returned to supplier", "Sold"} {
		if strings.Contains(body, secret) {
			t.Errorf("the Stock screen leaked %q", secret)
		}
	}
}

func TestSupplierReturnRefusesMoreThanTheTwoCaps(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, tables, 40, "rent", "Gupta Traders", "2026-09-03")

	// 80 go out with people and do not come back, leaving 60 in the store.
	resp, body := e.post("/issue/new", url.Values{
		"productId": {tables}, "quantity": {"80"},
		"takerName": {"Ravi Menon"}, "takerDepartment": {"Setup"},
		"takerMobile": {"98861 40023"}, "issuedAt": {"2026-09-03T12:00"},
	})
	if resp.StatusCode != 303 {
		t.Fatalf("issuing = %d: %s", resp.StatusCode, body)
	}
	if got := stockOf(t, e, tables); got != 60 {
		t.Fatalf("on hand is %d, want 60", got)
	}

	// Sharma sent 100 but only 60 are here: 61 is refused with the exact
	// number a person can act on.
	status, body := admin.post(t, "/finance/supplier-returns/new",
		settleForm("Sharma Events", tables, 61, "2026-09-03T15:00"))
	if status != 200 || !strings.Contains(body, "Only 60 Round tables can be returned to this supplier.") {
		t.Fatalf("61 gave %d: %s", status, body)
	}
	if returns, _ := settlements(t, e, key); len(returns) != 0 {
		t.Fatal("a refused return was written")
	}

	// 50 is fine; then only the shared 10 remain, for either supplier.
	if status, body := admin.post(t, "/finance/supplier-returns/new",
		settleForm("Sharma Events", tables, 50, "2026-09-03T15:00")); status != 303 {
		t.Fatalf("50 gave %d: %s", status, body)
	}
	status, body = admin.post(t, "/finance/supplier-returns/new",
		settleForm("Gupta Traders", tables, 11, "2026-09-03T15:30"))
	if status != 200 || !strings.Contains(body, "Only 10 Round tables can be returned to this supplier.") {
		t.Errorf("Gupta's 11 gave %d: %s", status, body)
	}

	// A supplier who sent nothing may return nothing.
	status, _ = admin.post(t, "/finance/supplier-returns/new",
		settleForm("Verma Sound", tables, 1, "2026-09-03T15:30"))
	if status != 200 {
		t.Error("a supplier who sent nothing was allowed to take stock back")
	}
}

func TestSaleRefusesMoreThanPurchasedOrOnHand(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")

	receive(t, e, chairs, 100, "purchase", "Gupta Traders", "2026-09-03")
	receive(t, e, chairs, 50, "rent", "Sharma Events", "2026-09-03")
	e.post("/issue/new", url.Values{
		"productId": {chairs}, "quantity": {"90"},
		"takerName": {"Ravi Menon"}, "takerDepartment": {"Setup"},
		"takerMobile": {"98861 40023"}, "issuedAt": {"2026-09-03T12:00"},
	})
	if got := stockOf(t, e, chairs); got != 60 {
		t.Fatalf("on hand is %d, want 60", got)
	}

	// 100 were bought but only 60 are here.
	status, body := admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 61, "2026-09-03T16:00"))
	if status != 200 || !strings.Contains(body, "Only 60 Chairs can be sold.") {
		t.Fatalf("61 gave %d: %s", status, body)
	}
	if _, sales := settlements(t, e, key); len(sales) != 0 {
		t.Fatal("a refused sale was written")
	}
	if status, body := admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 40, "2026-09-03T16:00")); status != 303 {
		t.Fatalf("40 gave %d: %s", status, body)
	}
	// 60 purchased are unsold but only 20 are physically here.
	status, body = admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 21, "2026-09-03T17:00"))
	if status != 200 || !strings.Contains(body, "Only 20 Chairs can be sold.") {
		t.Errorf("21 gave %d: %s", status, body)
	}
}

func TestPhysicalSettlementRechecksInsideAtomicUpdate(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	second, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 50, "rent", "Sharma Events", "2026-09-03")

	// Two people try to return 40 each when only 50 are allowed.
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, c := range []*testClient{admin, second} {
		wg.Add(1)
		go func(i int, c *testClient) {
			defer wg.Done()
			codes[i], _ = c.post(t, "/finance/supplier-returns/new",
				settleForm("Sharma Events", tables, 40, "2026-09-03T15:00"))
		}(i, c)
	}
	wg.Wait()

	accepted := 0
	for _, code := range codes {
		if code == 303 {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d of two concurrent 40-returns were accepted: %v", accepted, codes)
	}
	if got := stockOf(t, e, tables); got != 10 {
		t.Errorf("on hand is %d, want 10", got)
	}

	// Everything still adds up, and still does after a restart.
	e.st.Read(func(reg *register.Register) {
		if problems := register.Validate(reg); len(problems) != 0 {
			t.Errorf("the register is invalid: %v", problems)
		}
		if register.OnHand(reg, tables) < 0 {
			t.Error("on hand went negative")
		}
	})
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	// ReadBoth, not Read nested inside ReadFinance: those take the same
	// non-reentrant lock and nesting them hangs the whole run.
	if err := reopened.ReadBoth(key2, func(reg *register.Register, f *register.FinanceData) {
		if len(f.SupplierReturns) != 1 {
			t.Errorf("%d returns after restart", len(f.SupplierReturns))
		}
		if err := register.ValidatePairing(reg, f); err != nil {
			t.Errorf("after restart the two halves disagree: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDepositRefundAndSaleProceedsAreSeparateFromStock(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")
	chairs := productIDNamed(t, e, "Chairs")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, chairs, 60, "purchase", "Gupta Traders", "2026-09-03")

	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 40, "2026-09-03T15:00"))
	if got := stockOf(t, e, tables); got != 60 {
		t.Fatalf("after returning 40 the stock is %d", got)
	}
	// The deposit coming back is money only.
	deposit := moneyForm("in", "5000", "Sharma Events", "Deposit refund", "Bank transfer")
	if status, _ := admin.post(t, "/finance/movements/new", deposit); status != 303 {
		t.Fatal("the deposit refund was refused")
	}
	if got := stockOf(t, e, tables); got != 60 {
		t.Errorf("a deposit refund moved stock to %d", got)
	}

	// A sale, then its proceeds in two instalments.
	admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 50, "2026-09-03T16:00"))
	if got := stockOf(t, e, chairs); got != 10 {
		t.Fatalf("after selling 50 the stock is %d", got)
	}
	for _, amount := range []string{"4000", "6000"} {
		form := moneyForm("in", amount, "Patel Decorators", "Sale proceeds", "UPI")
		if status, _ := admin.post(t, "/finance/movements/new", form); status != 303 {
			t.Fatalf("the %s instalment was refused", amount)
		}
	}
	// Stock fell once, by fifty; the money arrived twice.
	if got := stockOf(t, e, chairs); got != 10 {
		t.Errorf("the instalments moved stock to %d", got)
	}
	if got := totals(t, e, key); got.ReceivedPaise != 1500000 {
		t.Errorf("received total is %d, want 1500000", got.ReceivedPaise)
	}
	if all := movements(t, e, key); len(all) != 3 {
		t.Errorf("%d money rows, want 3", len(all))
	}
}

func TestSettlementCorrectionReallocatesAndAuditsAtomically(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, tables, 60, "rent", "Gupta Traders", "2026-09-03")
	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 50, "2026-09-03T15:00"))
	returns, _ := settlements(t, e, key)
	id := returns[0].ID
	if got := stockOf(t, e, tables); got != 110 {
		t.Fatalf("after returning 50 the stock is %d", got)
	}

	// Reduce it to 30 and correct the time and remark at once.
	edit := settleForm("Sharma Events", tables, 30, "2026-09-03T17:30")
	edit.Set("remarks", "Counted again at the gate")
	if status, body := admin.post(t, "/finance/settlements/supplier_return/"+id+"/edit", edit); status != 303 {
		t.Fatalf("correction = %d: %s", status, body)
	}
	// Twenty came back to the store.
	if got := stockOf(t, e, tables); got != 130 {
		t.Errorf("after reducing to 30 the stock is %d, want 130", got)
	}

	returns, _ = settlements(t, e, key)
	x := returns[0]
	if x.Quantity() != 30 {
		t.Errorf("the return is now %d", x.Quantity())
	}
	var fields []string
	for _, c := range x.Changes {
		fields = append(fields, c.Field)
		if c.ByAccountID != adminID || c.ByMobile != "9886140023" {
			t.Errorf("change %s is attributed to %+v", c.Field, c)
		}
	}
	if strings.Join(fields, ",") != "quantity,returnedAt,remarks" {
		t.Fatalf("changes are %v", fields)
	}
	byField := map[string]register.FinanceChange{}
	for _, c := range x.Changes {
		byField[c.Field] = c
	}
	if byField["quantity"].Label != "How many" ||
		byField["quantity"].From != "50 Round tables" || byField["quantity"].To != "30 Round tables" {
		t.Errorf("the quantity change is %+v", byField["quantity"])
	}
	if byField["returnedAt"].Label != "Date and time returned" {
		t.Errorf("the time change is %+v", byField["returnedAt"])
	}

	// The public and protected halves still agree.
	if err := e.st.ReadBoth(key, func(reg *register.Register, f *register.FinanceData) {
		if err := register.ValidatePairing(reg, f); err != nil {
			t.Errorf("the two halves disagree: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// A correction beyond the cap is refused and changes nothing.
	before := mustReadFile(t, e.path)
	if status, _ := admin.post(t, "/finance/settlements/supplier_return/"+id+"/edit",
		settleForm("Sharma Events", tables, 500, "2026-09-03T17:30")); status != 200 {
		t.Error("a correction beyond the cap was accepted")
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a refused correction changed the register file")
	}
}

func TestSettlementVoidRestoresStockButKeepsHistory(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")

	receive(t, e, chairs, 60, "purchase", "Gupta Traders", "2026-09-03")
	admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 5, "2026-09-03T16:00"))
	_, sales := settlements(t, e, key)
	id := sales[0].ID
	if got := stockOf(t, e, chairs); got != 55 {
		t.Fatalf("after selling 5 the stock is %d", got)
	}

	// A void needs a reason.
	if status, body := admin.post(t, "/finance/settlements/sale/"+id+"/void", nil); status != 200 ||
		!strings.Contains(body, "Say why you are voiding this record.") {
		t.Fatalf("a blank reason gave %d", status)
	}
	if status, body := admin.post(t, "/finance/settlements/sale/"+id+"/void",
		url.Values{"reason": {"These were never sold"}}); status != 303 {
		t.Fatalf("void = %d: %s", status, body)
	}

	// The stock comes back and the row stays.
	if got := stockOf(t, e, chairs); got != 60 {
		t.Errorf("after voiding the stock is %d, want 60", got)
	}
	_, sales = settlements(t, e, key)
	if len(sales) != 1 || sales[0].Voided == nil || sales[0].Voided.Reason != "These were never sold" {
		t.Fatalf("the void is %+v", sales)
	}
	// The public removal is switched off, not deleted.
	e.st.Read(func(reg *register.Register) {
		if len(reg.Disposals) != 1 || reg.Disposals[0].InactiveAt == nil {
			t.Errorf("the stock removal is %+v", reg.Disposals)
		}
	})
	// It stays visible, and in the activity list.
	_, body := admin.get(t, "/finance/settlements")
	if !strings.Contains(body, "Voided — These were never sold") {
		t.Error("the settlements screen does not show the void")
	}
	_, activity := admin.get(t, "/finance/audit")
	if !strings.Contains(activity, "These were never sold") {
		t.Error("the activity list does not show the void")
	}
	// It cannot be voided or corrected again.
	if status, _ := admin.post(t, "/finance/settlements/sale/"+id+"/void", url.Values{"reason": {"again"}}); status != 200 {
		t.Error("a second void was accepted")
	}
}

func TestInwardCorrectionCannotStrandDisposalAllocation(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	var inwardID string
	e.st.Read(func(reg *register.Register) { inwardID = reg.Inwards[0].ID })
	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 60, "2026-09-03T15:00"))

	edit := func(v url.Values) (int, string) {
		form := url.Values{
			"quantity": {"100"}, "basis": {"rent"}, "supplier": {"Sharma Events"},
			"receivedOn": {"2026-09-03"}, "receivedBy": {"Suresh Kumar"},
		}
		for k, vals := range v {
			form[k] = vals
		}
		resp, body := e.post("/entry/"+inwardID+"/edit", form)
		return resp.StatusCode, body
	}

	before := mustReadFile(t, e.path)
	// Below what already left, a different basis and a different supplier are
	// all refused with the one neutral sentence.
	for name, change := range map[string]url.Values{
		"below the allocated amount": {"quantity": {"59"}},
		"a different basis":          {"basis": {"purchase"}},
		"a different supplier":       {"supplier": {"Gupta Traders"}},
	} {
		status, body := edit(change)
		if status != 200 || !strings.Contains(body, register.ErrStrandedDisposal) {
			t.Errorf("%s gave %d without the refusal", name, status)
		}
	}
	// Deleting it is refused too.
	resp, body := e.post("/entry/"+inwardID+"/delete", url.Values{"reason": {"Typed twice"}})
	if resp.StatusCode != 200 || !strings.Contains(body, register.ErrStrandedDisposal) {
		t.Errorf("deleting gave %d without the refusal: %s", resp.StatusCode, body)
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a refused inward correction changed the register file")
	}

	// Raising it, and lowering it no further than what is allocated, are fine.
	if status, body := edit(url.Values{"quantity": {"120"}}); status != 303 {
		t.Errorf("raising the quantity gave %d: %s", status, body)
	}
	if status, body := edit(url.Values{"quantity": {"60"}}); status != 303 {
		t.Errorf("lowering to exactly the allocated amount gave %d: %s", status, body)
	}

	// Void the return, and the formerly blocked correction goes through.
	returns, _ := settlements(t, e, key)
	if status, _ := admin.post(t, "/finance/settlements/supplier_return/"+returns[0].ID+"/void",
		url.Values{"reason": {"Never actually went back"}}); status != 303 {
		t.Fatal("the void was refused")
	}
	if status, body := edit(url.Values{"quantity": {"10"}}); status != 303 {
		t.Errorf("after voiding, the correction gave %d: %s", status, body)
	}
}

func TestProductCascadeTombstonesDisposalsButKeepsFinancialHistory(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 40, "2026-09-03T15:00"))

	// The confirmation counts the stock removals.
	_, body := e.get("/product/" + tables + "/edit")
	if !strings.Contains(body, "Stock removal entries: 1") {
		t.Errorf("the deletion confirmation does not count the removal: %s", body)
	}
	var version string
	e.st.Read(func(reg *register.Register) {
		impact, _ := register.ProductDeletionImpact(reg, tables)
		version = impact.Version
		if impact.DisposalEntries != 1 {
			t.Errorf("the impact counts %d removals", impact.DisposalEntries)
		}
	})

	resp, body := e.post("/product/"+tables+"/delete", url.Values{
		"reason": {"Added twice by mistake"}, "impactVersion": {version},
	})
	if resp.StatusCode != 303 {
		t.Fatalf("deleting = %d: %s", resp.StatusCode, body)
	}

	// The working register forgets it; the protected history does not.
	e.st.Read(func(reg *register.Register) {
		if _, ok := register.ProductByID(reg, tables); ok {
			t.Error("the product is still live")
		}
		if len(reg.Disposals) != 1 || reg.Disposals[0].InactiveAt == nil {
			t.Errorf("the removal is %+v", reg.Disposals)
		}
	})
	returns, _ := settlements(t, e, key)
	if len(returns) != 1 || returns[0].Voided != nil {
		t.Errorf("the cascade voided the protected return: %+v", returns)
	}
	if returns[0].Product.ProductName != "Round tables" {
		t.Errorf("the product snapshot is %q", returns[0].Product.ProductName)
	}
	_, settleBody := admin.get(t, "/finance/settlements")
	if !strings.Contains(settleBody, "The product was removed from the working register.") {
		t.Error("the settlements screen does not say the product went")
	}

	// And it all survives a restart.
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadFinance(key2, func(f *register.FinanceData) {
		if len(f.SupplierReturns) != 1 || f.SupplierReturns[0].Product.ProductName != "Round tables" {
			t.Errorf("after restart the history is %+v", f.SupplierReturns)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSupplierObligationsScreenUsesActualReceipts(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	// An order and an advance change nothing.
	orderID := saveOrder(t, e, admin, key, orderForm("Sharma Events", tables, "100", "rent"))
	advance := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	advance.Set("orderId", orderID)
	admin.post(t, "/finance/movements/new", advance)

	_, body := admin.get(t, "/finance/obligations")
	if strings.Contains(body, "100") {
		t.Error("an order created an obligation")
	}

	// 70 actually arrive; 20 go back.
	receive(t, e, tables, 70, "rent", "Sharma Events", "2026-09-03")
	_, body = admin.get(t, "/finance/obligations")
	if !strings.Contains(body, "Sharma Events") || !strings.Contains(body, ">70<") {
		t.Errorf("after receiving 70 the screen shows: %s", body)
	}
	admin.post(t, "/finance/supplier-returns/new", settleForm("Sharma Events", tables, 20, "2026-09-03T15:00"))
	_, body = admin.get(t, "/finance/obligations")
	if !strings.Contains(body, ">50<") {
		t.Errorf("after returning 20 the screen shows: %s", body)
	}
}

func TestInventoryDisposalContainsNoFinancialDetail(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")
	chairs := productIDNamed(t, e, "Chairs")

	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")
	receive(t, e, chairs, 60, "purchase", "Gupta Traders", "2026-09-03")
	admin.post(t, "/finance/supplier-returns/new", func() url.Values {
		v := settleForm("Sharma Events", tables, 40, "2026-09-03T15:00")
		v.Set("reference", "GATE/12")
		v.Set("remarks", "Loaded onto their truck")
		return v
	}())
	admin.post(t, "/finance/sales/new", settleForm("Patel Decorators", chairs, 20, "2026-09-03T16:00"))

	raw := string(mustReadFile(t, e.path))
	for _, secret := range []string{
		"Patel Decorators", "GATE/12", "Loaded onto their truck",
		"supplierReturns", "buyerPartyId", "partyId", "amountPaise",
	} {
		if strings.Contains(raw, secret) {
			t.Errorf("%q is readable in the register file", secret)
		}
	}
	// The projection carries only the contracted fields.
	var disposals []register.InventoryDisposal
	e.st.Read(func(reg *register.Register) { disposals = append(disposals, reg.Disposals...) })
	if len(disposals) != 2 {
		t.Fatalf("%d removals", len(disposals))
	}
	// Nothing in the public record distinguishes the two kinds of exit.
	shapeOf := func(d register.InventoryDisposal) string {
		return strings.Join([]string{
			boolText(d.ID != ""), boolText(d.ProductID != ""),
			boolText(d.Quantity > 0), boolText(len(d.Sources) > 0),
			boolText(!d.RecordedAt.IsZero()), boolText(d.InactiveAt == nil),
		}, ",")
	}
	if shapeOf(disposals[0]) != shapeOf(disposals[1]) {
		t.Error("a sale and a supplier return look different in the public record")
	}
}

func boolText(b bool) string {
	if b {
		return "y"
	}
	return "n"
}
