package web

import (
	"net/url"
	"strings"
	"sync"
	"testing"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// moneyForm is the smallest complete Record money post.
func moneyForm(direction, amount, party, purpose, mode string) url.Values {
	return url.Values{
		"direction": {direction}, "amount": {amount},
		"occurredAt":  {"2026-09-03T11:00"},
		"partyName":   {party},
		"purposeName": {purpose},
		"modeName":    {mode},
	}
}

func movements(t *testing.T, e *env, key []byte) []register.MoneyMovement {
	t.Helper()
	var out []register.MoneyMovement
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		out = append(out, f.Movements...)
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func totals(t *testing.T, e *env, key []byte) register.MoneyTotals {
	t.Helper()
	var got register.MoneyTotals
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		var err error
		got, err = register.TotalMoney(f, nil)
		if err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestMoneyMovementStoresExactPaiseAndAuthenticatedActor(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)

	beforeStock := onHand(e)
	form := moneyForm("out", "5000", "Sharma Events", "Deposit", "Online")
	form.Set("occurredAt", "2026-09-01T09:15")
	if status, body := admin.post(t, "/finance/movements/new", form); status != 303 {
		t.Fatalf("save = %d: %s", status, body)
	}

	all := movements(t, e, key)
	if len(all) != 1 {
		t.Fatalf("%d movements stored", len(all))
	}
	m := all[0]
	if m.ID != "MOV-0001" || m.Direction != register.MoneyOut || m.AmountPaise != 500000 {
		t.Fatalf("movement is %+v", m)
	}
	// When it happened and when it was typed are two different facts.
	if m.OccurredAt.Format("2006-01-02T15:04") != "2026-09-01T09:15" {
		t.Errorf("occurred at %v", m.OccurredAt)
	}
	if !m.RecordedAt.Equal(orderNow) || m.RecordedByID != adminID {
		t.Errorf("recorded %v by %s", m.RecordedAt, m.RecordedByID)
	}

	// The audit carries the account id and mobile, which a later rename cannot
	// take away.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		found := false
		for _, a := range f.Audit {
			if a.Kind == "movement_created" && a.EntityID == "MOV-0001" {
				found = true
				if a.ByAccountID != adminID || a.ByName != "Asha Mehta" || a.ByMobile != "9886140023" {
					t.Errorf("audit actor is %+v", a)
				}
				if !strings.Contains(a.Summary, "₹5,000.00") || !strings.Contains(a.Summary, "Money paid") {
					t.Errorf("audit summary is %q", a.Summary)
				}
			}
		}
		if !found {
			t.Error("no movement_created audit event")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Money never moves stock.
	if got := onHand(e); !sameStock(beforeStock, got) {
		t.Error("recording money changed stock")
	}
	if got := totals(t, e, key); got.PaidPaise != 500000 || got.ReceivedPaise != 0 || got.NetPaise != 500000 {
		t.Errorf("totals are %+v", got)
	}

	// Nothing readable in the file.
	raw := string(mustReadFile(t, e.path))
	for _, secret := range []string{"Sharma Events", "500000", "Deposit", "MOV-0001"} {
		if strings.Contains(raw, secret) {
			t.Errorf("%q is readable in the register file", secret)
		}
	}
}

func TestSplitAndBroadPaymentsAreIndependentRows(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	// The stakeholder's ₹14,000 settlement, entered as four amounts at once.
	batch := url.Values{
		"direction":   {"out", "out", "out", "out"},
		"amount":      {"5000", "5000", "2000", "2000"},
		"occurredAt":  {"2026-09-03T11:00", "2026-09-03T11:00", "2026-09-03T11:00", "2026-09-03T11:00"},
		"partyName":   {"Sharma Events", "Sharma Events", "Freight Movers", "Labour contractor"},
		"purposeName": {"Deposit", "Rent", "Freight", "Unloading labour"},
		"modeName":    {"Cash", "Cash", "Cash", "Cash"},
		"orderId":     {"", "", "", ""},
		"reference":   {"INV/88", "INV/88", "INV/88", "INV/88"},
	}
	if status, body := admin.post(t, "/finance/movements/new", batch); status != 303 {
		t.Fatalf("batch save = %d: %s", status, body)
	}

	all := movements(t, e, key)
	if len(all) != 4 {
		t.Fatalf("%d movements stored, want 4", len(all))
	}
	var ids []string
	var sum int64
	for _, m := range all {
		ids = append(ids, m.ID)
		sum += m.AmountPaise
	}
	if strings.Join(ids, ",") != "MOV-0001,MOV-0002,MOV-0003,MOV-0004" {
		t.Errorf("ids are %v", ids)
	}
	if sum != 1400000 {
		t.Errorf("the four rows total %d paise", sum)
	}
	// Form order is kept, so the ids match what the person typed where.
	if all[0].AmountPaise != 500000 || all[2].AmountPaise != 200000 {
		t.Errorf("the rows were reordered: %d then %d", all[0].AmountPaise, all[2].AmountPaise)
	}

	// The same settlement entered broadly stays one event. The server never
	// splits or combines an amount by itself.
	broad := moneyForm("out", "14000", "Sharma Events", "Whole settlement", "Cash")
	if status, _ := admin.post(t, "/finance/movements/new", broad); status != 303 {
		t.Fatal("the broad row was refused")
	}
	all = movements(t, e, key)
	if len(all) != 5 || all[4].AmountPaise != 1400000 {
		t.Fatalf("the broad row became %d rows", len(all)-4)
	}
	if got := totals(t, e, key); got.PaidPaise != 2800000 {
		t.Errorf("paid total is %d", got.PaidPaise)
	}

	// It survives a restart.
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadFinance(key2, func(f *register.FinanceData) {
		if len(f.Movements) != 5 {
			t.Errorf("after restart there are %d movements", len(f.Movements))
		}
		if err := register.ValidateFinance(f); err != nil {
			t.Errorf("the reopened vault is invalid: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMovementReusableSuggestionsAreMandatory(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	user, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")

	// Each of the three is mandatory, and its own refusal.
	for name, form := range map[string]url.Values{
		"no party":   moneyForm("out", "500", "", "Freight", "Cash"),
		"no purpose": moneyForm("out", "500", "Sharma Events", "", "Cash"),
		"no mode":    moneyForm("out", "500", "Sharma Events", "Freight", ""),
	} {
		status, body := admin.post(t, "/finance/movements/new", form)
		if status != 200 {
			t.Errorf("%s gave %d", name, status)
		}
		if !strings.Contains(body, "Say who") && !strings.Contains(body, "Say what") && !strings.Contains(body, "Say how") {
			t.Errorf("%s did not say what was missing", name)
		}
	}
	// A refusal leaves no half-created suggestion behind.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(register.LiveFinanceValues(f, register.FinanceParty)) != 0 {
			t.Error("a refused row created a party")
		}
		if len(register.LiveFinanceValues(f, register.FinancePurpose)) != 0 {
			t.Error("a refused row created a purpose")
		}
		if len(f.Movements) != 0 {
			t.Error("a refused row was written")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// New values created with a movement are immediately another user's
	// suggestions.
	form := moneyForm("out", "2000", "Sharma Events", "Unloading labour", "Online")
	if status, body := admin.post(t, "/finance/movements/new", form); status != 303 {
		t.Fatalf("save = %d: %s", status, body)
	}
	if got := suggestValues(t, user, "party", "sharma"); strings.Join(got, ",") != "Sharma Events" {
		t.Errorf("the other user is suggested parties %v", got)
	}
	if got := suggestValues(t, user, "purpose", "unl"); strings.Join(got, ",") != "Unloading labour" {
		t.Errorf("the other user is suggested purposes %v", got)
	}
	if got := suggestValues(t, user, "mode", "onl"); strings.Join(got, ",") != "Online" {
		t.Errorf("the other user is suggested modes %v", got)
	}
}

func TestMovementMayCoverWholeOrderOrSeveralProducts(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")

	orderID := saveOrder(t, e, admin, key, url.Values{
		"partyName": {"Sharma Events"},
		"productId": {chairs, tables}, "productName": {"", ""},
		"quantity": {"100", "50"}, "basis": {"rent", "purchase"},
		"orderedAt": {"2026-09-03T10:00"},
	})
	o := orderByID(t, e, key, orderID)

	// No line ids means the whole order: every line's snapshot is frozen.
	whole := moneyForm("out", "10000", "Sharma Events", "Advance", "Cash")
	whole.Set("orderId", orderID)
	if status, body := admin.post(t, "/finance/movements/new", whole); status != 303 {
		t.Fatalf("whole-order payment = %d: %s", status, body)
	}
	m := movements(t, e, key)[0]
	if m.OrderID != orderID || len(m.OrderLineIDs) != 0 {
		t.Errorf("the whole-order row is %+v", m)
	}
	if len(m.Products) != 2 || m.Products[0].ProductName != "Chairs" || m.Products[1].ProductName != "Round tables" {
		t.Errorf("the whole-order snapshots are %+v", m.Products)
	}

	// A different payee for one line of the same order is normal: freight goes
	// to a transporter, not the supplier.
	part := moneyForm("out", "2000", "Freight Movers", "Freight", "Cash")
	part.Set("orderId", orderID)
	part["lineIds-0"] = []string{o.Lines[0].ID}
	if status, body := admin.post(t, "/finance/movements/new", part); status != 303 {
		t.Fatalf("part-order payment = %d: %s", status, body)
	}
	m = movements(t, e, key)[1]
	if len(m.OrderLineIDs) != 1 || m.OrderLineIDs[0] != o.Lines[0].ID {
		t.Errorf("the line reference is %v", m.OrderLineIDs)
	}
	if len(m.Products) != 1 || m.Products[0].ProductName != "Chairs" {
		t.Errorf("the part-order snapshots are %+v", m.Products)
	}
	if got := register.FinanceValueText(mustFinance(t, e, key), m.PartyID); got != "Freight Movers" {
		t.Errorf("the payee is %q", got)
	}

	before := mustReadFile(t, e.path)
	// An unknown order, and a line belonging to no order, are both refused
	// without writing anything.
	bad := moneyForm("out", "500", "Sharma Events", "Advance", "Cash")
	bad.Set("orderId", "ORD-9999")
	if status, _ := admin.post(t, "/finance/movements/new", bad); status != 200 {
		t.Error("an unknown order was accepted")
	}
	stray := moneyForm("out", "500", "Sharma Events", "Advance", "Cash")
	stray["lineIds-0"] = []string{o.Lines[0].ID}
	if status, body := admin.post(t, "/finance/movements/new", stray); status != 200 ||
		!strings.Contains(body, "Pick the order those products belong to.") {
		t.Errorf("a line with no order gave %d", status)
	}
	wrongLine := moneyForm("out", "500", "Sharma Events", "Advance", "Cash")
	wrongLine.Set("orderId", orderID)
	wrongLine["lineIds-0"] = []string{"OLN-9999"}
	if status, _ := admin.post(t, "/finance/movements/new", wrongLine); status != 200 {
		t.Error("a line off the order was accepted")
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a refused movement changed the register file")
	}
}

func mustFinance(t *testing.T, e *env, key []byte) *register.FinanceData {
	t.Helper()
	var out *register.FinanceData
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) { out = f }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStandaloneMovementAndProductAdjustment(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")
	beforeStock := onHand(e)

	// A labour expense with no order and no product at all.
	plain := moneyForm("out", "1500", "Labour contractor", "Unloading labour", "Cash")
	if status, body := admin.post(t, "/finance/movements/new", plain); status != 303 {
		t.Fatalf("standalone expense = %d: %s", status, body)
	}
	m := movements(t, e, key)[0]
	if m.OrderID != "" || len(m.Products) != 0 {
		t.Errorf("the standalone row is %+v", m)
	}

	// A settlement in goods rather than cash is still an exact amount with a
	// custom mode, and still moves no stock by itself.
	adjust := moneyForm("in", "3000", "Sharma Events", "Damage settlement", "Product adjustment")
	adjust["productIds-0"] = []string{chairs, tables}
	adjust.Set("remarks", "Twelve broken chairs kept against the rent")
	if status, body := admin.post(t, "/finance/movements/new", adjust); status != 303 {
		t.Fatalf("product adjustment = %d: %s", status, body)
	}
	m = movements(t, e, key)[1]
	if m.AmountPaise != 300000 || m.Direction != register.MoneyIn {
		t.Errorf("the adjustment is %+v", m)
	}
	if len(m.Products) != 2 {
		t.Errorf("the adjustment covers %d products", len(m.Products))
	}
	if m.Remarks == "" {
		t.Error("the adjustment lost its explanation")
	}
	if got := onHand(e); !sameStock(beforeStock, got) {
		t.Error("a product adjustment moved stock")
	}
}

func TestCancelRefundKeepsBothDirections(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	orderID := saveOrder(t, e, admin, key, orderForm("Sharma Events", chairs, "100", "rent"))

	paid := moneyForm("out", "10000", "Sharma Events", "Advance", "Bank transfer")
	paid.Set("orderId", orderID)
	if status, _ := admin.post(t, "/finance/movements/new", paid); status != 303 {
		t.Fatal("the payment was refused")
	}
	// The goods never arrive, so the order is cancelled.
	if status, _ := admin.post(t, "/finance/orders/"+orderID+"/cancel",
		url.Values{"reason": {"Supplier could not deliver"}}); status != 303 {
		t.Fatal("the cancellation was refused")
	}
	// The money comes back as its own incoming row. Deleting the payment is
	// not an available action.
	refund := moneyForm("in", "10000", "Sharma Events", "Refund", "Bank transfer")
	refund.Set("orderId", orderID)
	if status, _ := admin.post(t, "/finance/movements/new", refund); status != 303 {
		t.Fatal("the refund was refused")
	}

	all := movements(t, e, key)
	if len(all) != 2 || all[0].Direction != register.MoneyOut || all[1].Direction != register.MoneyIn {
		t.Fatalf("the two rows are %+v", all)
	}
	var order register.MoneyTotals
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		var err error
		order, err = register.OrderTotals(f, orderID)
		if err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if order.PaidPaise != 1000000 || order.ReceivedPaise != 1000000 || order.NetPaise != 0 {
		t.Fatalf("the order nets %+v", order)
	}
	if orderByID(t, e, key, orderID).Status != "cancelled" {
		t.Error("the order is not cancelled")
	}
}

func TestConcurrentMovementPostsKeepAllRowsAndTotals(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	second, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")

	const rows = 20
	var wg sync.WaitGroup
	codes := make([]int, rows)
	for i := 0; i < rows; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := admin
			if i%2 == 1 {
				c = second
			}
			codes[i], _ = c.post(t, "/finance/movements/new",
				moneyForm("out", "500", "Freight Movers", "Freight", "Cash"))
		}(i)
	}
	wg.Wait()

	saved := 0
	for _, code := range codes {
		if code == 303 {
			saved++
		}
	}
	if saved != rows {
		t.Fatalf("only %d of %d concurrent rows saved: %v", saved, rows, codes)
	}

	all := movements(t, e, key)
	if len(all) != rows {
		t.Fatalf("%d rows stored for %d saves", len(all), rows)
	}
	seen := map[string]bool{}
	for _, m := range all {
		if seen[m.ID] {
			t.Fatalf("%s was used twice", m.ID)
		}
		seen[m.ID] = true
	}
	if got := totals(t, e, key); got.PaidPaise != int64(rows)*50000 {
		t.Errorf("paid total is %d, want %d", got.PaidPaise, rows*50000)
	}
	// One party, one purpose, whatever the ordering of the twenty posts.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if n := len(register.LiveFinanceValues(f, register.FinanceParty)); n != 1 {
			t.Errorf("%d parties for one payee", n)
		}
		if n := len(register.LiveFinanceValues(f, register.FinancePurpose)); n != 1 {
			t.Errorf("%d purposes for one purpose", n)
		}
	}); err != nil {
		t.Fatal(err)
	}

	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadFinance(key2, func(f *register.FinanceData) {
		if err := register.ValidateFinance(f); err != nil {
			t.Errorf("the reopened vault is invalid: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
