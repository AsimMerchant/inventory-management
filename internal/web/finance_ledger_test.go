package web

import (
	"net/http"
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

// saveMoney records one movement and returns its ID.
func saveMoney(t *testing.T, e *env, c *testClient, key []byte, form url.Values) string {
	t.Helper()
	if status, body := c.post(t, "/finance/movements/new", form); status != 303 {
		t.Fatalf("money save = %d: %s", status, body)
	}
	all := movements(t, e, key)
	return all[len(all)-1].ID
}

func movementByID(t *testing.T, e *env, key []byte, id string) register.MoneyMovement {
	t.Helper()
	for _, m := range movements(t, e, key) {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no movement %s", id)
	return register.MoneyMovement{}
}

func TestMovementCorrectionKeepsEveryOriginalValue(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")

	form := moneyForm("out", "5000", "Sharm Events", "Deposit", "Cash")
	form["productIds-0"] = []string{chairs}
	form.Set("reference", "INV/88")
	form.Set("remarks", "Paid at the gate")
	id := saveMoney(t, e, admin, key, form)
	created := movementByID(t, e, key, id)

	// Correct every field the form holds at once.
	edit := moneyForm("in", "6500.50", "Sharma Events", "Refund", "Bank transfer")
	edit.Set("occurredAt", "2026-09-01T08:30")
	edit["productIds-0"] = []string{tables}
	edit.Set("reference", "")
	edit.Set("remarks", "Sent back the same evening")
	if status, body := admin.post(t, "/finance/movements/"+id+"/edit", edit); status != 303 {
		t.Fatalf("correction = %d: %s", status, body)
	}

	m := movementByID(t, e, key, id)
	// Who recorded it, and when, is never rewritten by a correction.
	if !m.RecordedAt.Equal(created.RecordedAt) || m.RecordedByID != created.RecordedByID {
		t.Error("the correction rewrote who recorded the transaction")
	}
	if m.AmountPaise != 650050 || m.Direction != register.MoneyIn {
		t.Errorf("the corrected row is %+v", m)
	}

	var got []string
	for _, c := range m.Changes {
		got = append(got, c.Field)
		if c.ByAccountID != adminID || c.ByMobile != "9886140023" || !c.At.Equal(orderNow) {
			t.Errorf("change %s is attributed to %+v", c.Field, c)
		}
	}
	want := "direction,amount,occurredAt,party,products,purpose,mode,reference,remarks"
	if strings.Join(got, ",") != want {
		t.Fatalf("changes are %v, want %s", got, want)
	}
	byField := map[string]register.FinanceChange{}
	for _, c := range m.Changes {
		byField[c.Field] = c
	}
	for field, wants := range map[string][3]string{
		"direction":  {"Money paid or received", "Money paid", "Money received"},
		"amount":     {"Amount", "₹5,000.00", "₹6,500.50"},
		"occurredAt": {"Date and time", "3 September 2026 · 11:00 am", "1 September 2026 · 8:30 am"},
		"party":      {"Supplier or other party", "Sharm Events", "Sharma Events"},
		"products":   {"Related products", "Chairs", "Round tables"},
		"purpose":    {"Purpose", "Deposit", "Refund"},
		"mode":       {"Payment mode", "Cash", "Bank transfer"},
		"reference":  {"Reference", "INV/88", "Blank"},
		"remarks":    {"Remarks", "Paid at the gate", "Sent back the same evening"},
	} {
		c := byField[field]
		if c.Label != wants[0] || c.From != wants[1] || c.To != wants[2] {
			t.Errorf("%s recorded as %q: %q → %q", field, c.Label, c.From, c.To)
		}
	}

	// One audit event, and the old and new summaries both kept.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		edits := 0
		for _, a := range f.Audit {
			if a.Kind == "movement_edited" && a.EntityID == id {
				edits++
				if !strings.Contains(a.Before, "₹5,000.00") || !strings.Contains(a.After, "₹6,500.50") {
					t.Errorf("the audit summary is %q → %q", a.Before, a.After)
				}
			}
		}
		if edits != 1 {
			t.Errorf("%d movement_edited events", edits)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Submitting the same values again records nothing.
	if status, _ := admin.post(t, "/finance/movements/"+id+"/edit", edit); status != 303 {
		t.Fatal("the no-op correction was refused")
	}
	if again := movementByID(t, e, key, id); len(again.Changes) != len(m.Changes) {
		t.Errorf("a no-op correction recorded %d more changes", len(again.Changes)-len(m.Changes))
	}
}

func TestVoidNeverDeletesAndOppositeMovementIsReversal(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	typo := saveMoney(t, e, admin, key, moneyForm("out", "50000", "Sharma Events", "Rent", "Cash"))
	real := saveMoney(t, e, admin, key, moneyForm("out", "5000", "Sharma Events", "Rent", "Cash"))

	// A void needs a reason.
	if status, body := admin.post(t, "/finance/movements/"+typo+"/void", nil); status != 200 ||
		!strings.Contains(body, "Say why you are voiding this transaction.") {
		t.Fatalf("a blank reason gave %d", status)
	}
	if movementByID(t, e, key, typo).Voided != nil {
		t.Fatal("a refused void marked the row")
	}

	if status, body := admin.post(t, "/finance/movements/"+typo+"/void",
		url.Values{"reason": {"Typed an extra zero"}}); status != 303 {
		t.Fatalf("void = %d: %s", status, body)
	}

	// The row stays. It is only excluded from the totals.
	all := movements(t, e, key)
	if len(all) != 2 {
		t.Fatalf("%d rows after voiding one", len(all))
	}
	v := movementByID(t, e, key, typo)
	if v.Voided == nil || v.Voided.Reason != "Typed an extra zero" || v.Voided.ByName != "Asha Mehta" {
		t.Fatalf("the void is %+v", v.Voided)
	}
	if got := totals(t, e, key); got.PaidPaise != 500000 {
		t.Errorf("the voided row still counts: paid is %d", got.PaidPaise)
	}

	// It cannot be voided or edited again.
	if status, _ := admin.post(t, "/finance/movements/"+typo+"/void", url.Values{"reason": {"again"}}); status != 200 {
		t.Error("a second void was accepted")
	}
	if status, body := admin.post(t, "/finance/movements/"+typo+"/edit",
		moneyForm("out", "1", "Sharma Events", "Rent", "Cash")); status != 200 ||
		!strings.Contains(body, "This transaction was voided.") {
		t.Errorf("a voided row was editable: %d", status)
	}

	// Real money coming back is its own incoming row; the payment stays live.
	refund := saveMoney(t, e, admin, key, moneyForm("in", "5000", "Sharma Events", "Refund", "Cash"))
	if movementByID(t, e, key, real).Voided != nil {
		t.Error("recording a refund voided the payment")
	}
	if got := totals(t, e, key); got.PaidPaise != 500000 || got.ReceivedPaise != 500000 || got.NetPaise != 0 {
		t.Errorf("after the refund the totals are %+v", got)
	}
	if refund == real {
		t.Error("the refund reused the payment's row")
	}

	// The voided row is still on the journal, marked.
	status, body := admin.get(t, "/finance/journal")
	if status != 200 {
		t.Fatalf("journal = %d", status)
	}
	if !strings.Contains(body, "Voided — Typed an extra zero") {
		t.Error("the journal does not show the void")
	}
	// And in the activity list.
	_, activity := admin.get(t, "/finance/audit")
	if !strings.Contains(activity, "Typed an extra zero") {
		t.Error("the activity list does not show the void")
	}
}

func TestJournalChronologicalAndDateFilters(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	for _, when := range []string{"2026-08-31T23:59", "2026-09-01T00:00", "2026-09-02T18:00"} {
		form := moneyForm("out", "1000", "Sharma Events", "Rent", "Cash")
		form.Set("occurredAt", when)
		saveMoney(t, e, admin, key, form)
	}

	rows := func(path string) []string {
		status, body := admin.get(t, path)
		if status != 200 {
			t.Fatalf("%s = %d", path, status)
		}
		var out []string
		for _, id := range []string{"MOV-0001", "MOV-0002", "MOV-0003"} {
			if strings.Contains(body, ">"+id+"<") || strings.Contains(body, "/finance/movements/"+id+"/edit") {
				out = append(out, id)
			}
		}
		return out
	}

	// One day takes exactly that local calendar day.
	if got := rows("/finance/journal?day=2026-09-01"); strings.Join(got, ",") != "MOV-0002" {
		t.Errorf("the single day kept %v", got)
	}
	// An inclusive range takes both ends.
	if got := rows("/finance/journal?from=2026-08-31&to=2026-09-01"); strings.Join(got, ",") != "MOV-0001,MOV-0002" {
		t.Errorf("the range kept %v", got)
	}
	// A day supplied with a range wins.
	if got := rows("/finance/journal?day=2026-09-02&from=2026-08-31&to=2026-08-31"); strings.Join(got, ",") != "MOV-0003" {
		t.Errorf("the day lost to the range: %v", got)
	}
	// No parameters is everything.
	if got := rows("/finance/journal"); len(got) != 3 {
		t.Errorf("the unfiltered journal kept %v", got)
	}

	// Half a range, an unparseable date and a backwards range all refuse with
	// the one wording, and change no bytes.
	before := mustReadFile(t, e.path)
	for _, path := range []string{
		"/finance/journal?from=2026-09-01",
		"/finance/journal?to=2026-09-01",
		"/finance/journal?day=the+first",
		"/finance/journal?from=2026-09-05&to=2026-09-01",
	} {
		status, body := admin.get(t, path)
		if status != 200 || !strings.Contains(body, register.JournalRefusal) {
			t.Errorf("%s gave %d without the refusal", path, status)
		}
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a refused filter changed the register file")
	}
}

func TestJournalExactTimeRangeIsInclusive(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	// The spec's own five movements around a 23:00-23:39 window.
	for _, minute := range []string{"22:59", "23:00", "23:20", "23:39", "23:40"} {
		form := moneyForm("out", "1000", "Sharma Events", "Rent", "Cash")
		form.Set("occurredAt", "2016-01-01T"+minute)
		saveMoney(t, e, admin, key, form)
	}

	kept := func(path string) []string {
		status, body := admin.get(t, path)
		if status != 200 {
			t.Fatalf("%s = %d", path, status)
		}
		var out []string
		for i, minute := range []string{"22:59", "23:00", "23:20", "23:39", "23:40"} {
			id := "MOV-000" + itoa(i+1)
			if strings.Contains(body, "/finance/movements/"+id+"/edit") {
				out = append(out, minute)
			}
		}
		return out
	}

	exact := "/finance/journal?fromTime=2016-01-01T23%3A00&toTime=2016-01-01T23%3A39"
	if got := kept(exact); strings.Join(got, ",") != "23:00,23:20,23:39" {
		t.Fatalf("the exact range kept %v", got)
	}
	// It wins over a day and a date range supplied at the same time.
	if got := kept(exact + "&day=2016-01-02&from=2016-01-05&to=2016-01-06"); strings.Join(got, ",") != "23:00,23:20,23:39" {
		t.Fatalf("the exact range lost to the other filters: %v", got)
	}

	before := mustReadFile(t, e.path)
	for _, path := range []string{
		"/finance/journal?fromTime=2016-01-01T23%3A00",
		"/finance/journal?toTime=2016-01-01T23%3A39",
		"/finance/journal?fromTime=noon&toTime=2016-01-01T23%3A39",
		"/finance/journal?fromTime=2016-01-01T23%3A39&toTime=2016-01-01T23%3A00",
	} {
		status, body := admin.get(t, path)
		if status != 200 || !strings.Contains(body, register.JournalRefusal) {
			t.Errorf("%s gave %d without the refusal", path, status)
		}
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a refused exact range changed the register file")
	}
}

func TestJournalIsProtectedAndPrintableWithoutJavaScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	for i, minute := range []string{"22:59", "23:00", "23:20", "23:39", "23:40"} {
		form := moneyForm("out", itoa((i+1)*1000), "Sharma Events", "Rent", "Cash")
		form.Set("occurredAt", "2016-01-01T"+minute)
		form.Set("reference", "REF/"+itoa(i+1))
		saveMoney(t, e, admin, key, form)
	}

	// Nobody without a session sees anything.
	public := e.client()
	for _, path := range []string{"/finance/journal", "/finance/journal/print", "/finance/audit"} {
		resp, body := financeRequest(t, public, "GET", e.URL+path, nil)
		if resp.StatusCode != 303 {
			t.Errorf("%s answered %d to a stranger", path, resp.StatusCode)
		}
		for _, secret := range []string{"Sharma Events", "₹", "REF/"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s leaked %q to a stranger", path, secret)
			}
		}
	}

	// The screen is server-rendered: filter controls are a plain GET form and
	// the print link is a plain link.
	status, body := admin.get(t, "/finance/journal")
	if status != 200 {
		t.Fatalf("journal = %d", status)
	}
	for _, want := range []string{
		`method="get"`, `name="day"`, `name="from"`, `name="to"`,
		`name="fromTime"`, `name="toTime"`,
		"One day", "Exact start", "Exact end", ">Show<", "Every date",
		`href="/finance/journal/print"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the journal has no %s", want)
		}
	}
	if !strings.Contains(body, "no-store") && resp(t, admin, "/finance/journal") != "no-store" {
		t.Error("the journal is cacheable")
	}

	// Filtering works with no JavaScript at all: it is a GET with query
	// parameters, exactly what the form submits.
	filtered := "/finance/journal/print?fromTime=2016-01-01T23%3A00&toTime=2016-01-01T23%3A39"
	status, printed := admin.get(t, filtered)
	if status != 200 {
		t.Fatalf("print = %d", status)
	}
	if !strings.Contains(printed, "Transaction journal") {
		t.Error("the print view has no heading")
	}
	if !strings.Contains(printed, "Printed ") {
		t.Error("the print view has no printed stamp")
	}
	if !strings.Contains(printed, "1 January 2016") {
		t.Error("the print view does not name the range")
	}
	// Exactly the three rows in the window, oldest first.
	for _, want := range []string{"₹2,000.00", "₹3,000.00", "₹4,000.00"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the print view is missing %s", want)
		}
	}
	for _, unwanted := range []string{"₹1,000.00", "₹5,000.00"} {
		if strings.Contains(printed, unwanted) {
			t.Errorf("the print view includes %s from outside the range", unwanted)
		}
	}
	first := strings.Index(printed, "₹2,000.00")
	last := strings.Index(printed, "₹4,000.00")
	if first > last {
		t.Error("the print view is not in ascending time order")
	}
	// No navigation or actions on the printed page.
	for _, unwanted := range []string{"Void this transaction", "Fix this transaction", "Every date"} {
		if strings.Contains(printed, unwanted) {
			t.Errorf("the print view carries the %q control", unwanted)
		}
	}
	// And the stylesheet that hides the rest, read off the running server so
	// this asserts what actually ships rather than what is on disk.
	_, css := e.get("/static/app.css")
	if !strings.Contains(css, "@media print") {
		t.Error("the served stylesheet has no print rules")
	}
	for _, hidden := range []string{".filters", ".actions", ".danger"} {
		if !strings.Contains(css, hidden) {
			t.Errorf("the print rules do not mention %s", hidden)
		}
	}
}

// resp returns one header from a protected GET.
func resp(t *testing.T, c *testClient, path string) string {
	t.Helper()
	r, err := http.NewRequest("GET", c.e.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	return out.Header.Get("Cache-Control")
}

func TestFinancialAuditVisibleButImmutable(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	user, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")
	chairs := productIDNamed(t, e, "Chairs")

	// One of each kind of event: account, list, order and money.
	orderID := saveOrder(t, e, admin, key, orderForm("Sharma Events", chairs, "100", "rent"))
	moveID := saveMoney(t, e, admin, key, moneyForm("out", "5000", "Sharma Events", "Deposit", "Cash"))
	sharma := valueIDByText(t, e, key, register.FinanceParty, "Sharma Events")
	if status, _ := admin.post(t, "/finance/lists/"+sharma+"/rename",
		url.Values{"value": {"Sharma Tent House"}}); status != 303 {
		t.Fatal("the rename was refused")
	}

	// Both an administrator and an ordinary financial user see the activity.
	for who, c := range map[string]*testClient{"admin": admin, "user": user} {
		status, body := c.get(t, "/finance/audit")
		if status != 200 {
			t.Fatalf("%s got %d from the activity list", who, status)
		}
		for _, want := range []string{
			"Authorized account created", "Shared value corrected",
			"Order recorded", "Money recorded",
			"Asha Mehta", "9886140023", orderID, moveID,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not see %q in the activity list", who, want)
			}
		}
	}

	// There is no route that changes it.
	before := mustReadFile(t, e.path)
	for _, path := range []string{"/finance/audit", "/finance/audit/FAE-0001/edit", "/finance/audit/FAE-0001/delete"} {
		status, _ := admin.post(t, path, url.Values{"anything": {"yes"}})
		if status == 200 || status == 303 {
			t.Errorf("POST %s answered %d", path, status)
		}
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a hand-built post changed the register file")
	}
}
