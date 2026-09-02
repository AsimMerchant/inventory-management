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

// orderNow is a moment inside WalkthroughT0's live shift, so the ordinary desk
// screens are reachable in the same test as the protected ones.
var orderNow = register.MustTime("2026-09-03T11:00:00+05:30")

func TestAuthenticatedFinanceProductPickerWorksBeforeInventoryShift(t *testing.T) {
	e := newTestServer(t, nil, orderNow)
	admin, _, _ := financeAdmin(t, e)
	if status, body := admin.get(t, "/api/products?mode=all&q=Tent"); status != http.StatusOK || body != "[]\n" {
		t.Fatalf("authenticated pre-shift product picker = %d: %s", status, body)
	}
	public := financeClient(t)
	resp, _ := financeRequest(t, public, http.MethodGet, e.URL+"/api/products?mode=all&q=Tent", nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/shift" {
		t.Fatalf("public pre-shift product picker = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestBrowserShapedOrderKeepsIndependentBasisPerLine(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	form := orderForm("Sharma Events", "PRD-0001", "100", "")
	form["productId"] = []string{"PRD-0001", "PRD-0002"}
	form["productName"] = []string{"Chairs", "Round tables"}
	form["quantity"] = []string{"100", "50"}
	form.Set("basis-0", "rent")
	form.Set("basis-1", "purchase")
	if status, body := admin.post(t, "/finance/orders/new", form); status != http.StatusSeeOther {
		t.Fatalf("browser-shaped mixed order = %d: %s", status, body)
	}
	f := mustFinance(t, e, key)
	if len(f.Orders) != 1 || len(f.Orders[0].Lines) != 2 || f.Orders[0].Lines[0].Basis != register.Basis("rent") || f.Orders[0].Lines[1].Basis != register.Basis("purchase") {
		t.Fatalf("mixed bases = %+v", f.Orders)
	}
}

// onHand is every product's stock, which an order must never change.
func onHand(e *env) map[string]int {
	out := map[string]int{}
	e.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.Deleted == nil {
				out[p.ID] = register.OnHand(reg, p.ID)
			}
		}
	})
	return out
}

func sameStock(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// orderForm is the smallest complete order post: one party, one product line.
func orderForm(party, productID, quantity, basis string) url.Values {
	return url.Values{
		"partyName": {party},
		"productId": {productID}, "productName": {""},
		"quantity": {quantity}, "basis": {basis},
		"orderedAt": {"2026-09-03T11:00"},
	}
}

func onlyOrder(t *testing.T, e *env, key []byte) register.FinanceOrder {
	t.Helper()
	var out []register.FinanceOrder
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		out = append(out, f.Orders...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("%d orders stored, want 1", len(out))
	}
	return out[0]
}

func productIDNamed(t *testing.T, e *env, name string) string {
	t.Helper()
	id := ""
	e.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.Deleted == nil && p.Name == name {
				id = p.ID
			}
		}
	})
	if id == "" {
		t.Fatalf("no product called %q", name)
	}
	return id
}

func TestOrderWithMultipleProductsAndMixedBasis(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	reels := productIDNamed(t, e, "Round tables")

	before := onHand(e)
	var inwards, issues, returns int
	e.st.Read(func(reg *register.Register) {
		inwards, issues, returns = len(reg.Inwards), len(reg.Issues), len(reg.Returns)
	})
	status, body := admin.post(t, "/finance/orders/new", url.Values{
		"partyName": {"Sharma Events"},
		"productId": {chairs, reels}, "productName": {"", ""},
		"quantity": {"100", "50"}, "basis": {"rent", "purchase"},
		"orderedAt":   {"2026-09-03T11:00"},
		"agreedTotal": {"25000"}, "agreedKind": {"estimated"},
	})
	if status != 303 {
		t.Fatalf("save=%d %s", status, body)
	}

	o := onlyOrder(t, e, key)
	if o.ID != "ORD-0001" || o.Status != "open" || o.CreatedByID != adminID {
		t.Fatalf("order is %+v", o)
	}
	if len(o.Lines) != 2 {
		t.Fatalf("%d lines", len(o.Lines))
	}
	if o.Lines[0].ProductID != chairs || o.Lines[0].ExpectedQuantity != 100 || o.Lines[0].Basis != register.Rent {
		t.Errorf("first line is %+v", o.Lines[0])
	}
	if o.Lines[1].ProductID != reels || o.Lines[1].ExpectedQuantity != 50 || o.Lines[1].Basis != register.Purchase {
		t.Errorf("second line is %+v", o.Lines[1])
	}
	if o.Lines[0].ID == o.Lines[1].ID {
		t.Errorf("both lines got the id %s", o.Lines[0].ID)
	}
	if o.Lines[0].ProductNameSnapshot != "Chairs" || o.Lines[1].ProductNameSnapshot != "Round tables" {
		t.Errorf("snapshots are %q and %q", o.Lines[0].ProductNameSnapshot, o.Lines[1].ProductNameSnapshot)
	}
	if o.AgreedPaise == nil || *o.AgreedPaise != 2500000 || o.AgreedKind != "estimated" {
		t.Errorf("total is %v %q", o.AgreedPaise, o.AgreedKind)
	}

	// One party, created as part of the save.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		parties := register.LiveFinanceValues(f, register.FinanceParty)
		if len(parties) != 1 || parties[0].Value != "Sharma Events" || parties[0].ID != o.PartyID {
			t.Errorf("parties are %+v", parties)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// An order is intent. Nothing physical moved.
	if got := onHand(e); !sameStock(before, got) {
		t.Errorf("on-hand changed from %v to %v", before, got)
	}
	e.st.Read(func(reg *register.Register) {
		if len(reg.Inwards) != inwards || len(reg.Issues) != issues || len(reg.Returns) != returns {
			t.Error("an order touched the inventory records")
		}
	})

	// It survives a restart, still encrypted.
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadFinance(key2, func(f *register.FinanceData) {
		if len(f.Orders) != 1 || f.Orders[0].ID != "ORD-0001" {
			t.Errorf("after restart the orders are %+v", f.Orders)
		}
	}); err != nil {
		t.Fatal(err)
	}
	raw := string(mustReadFile(t, e.path))
	for _, secret := range []string{"Sharma Events", "2500000", "ORD-0001"} {
		if strings.Contains(raw, secret) {
			t.Errorf("%q is readable in the register file", secret)
		}
	}
}

func TestOrderAgreedTotalOptionalEstimatedOrExact(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")

	// Blank total.
	if status, body := admin.post(t, "/finance/orders/new", orderForm("Sharma Events", chairs, "10", "rent")); status != 303 {
		t.Fatalf("blank total refused: %d %s", status, body)
	}
	o := onlyOrder(t, e, key)
	if o.AgreedPaise != nil || o.AgreedKind != "" {
		t.Fatalf("blank total stored as %v %q", o.AgreedPaise, o.AgreedKind)
	}

	// Estimated and exact, and the rupee forms the spec names.
	amounts := map[string]int64{"5000": 500000, "5000.5": 500050, "5000.50": 500050}
	for typed, want := range amounts {
		f := orderForm("Sharma Events", chairs, "10", "rent")
		f.Set("agreedTotal", typed)
		f.Set("agreedKind", "exact")
		if status, body := admin.post(t, "/finance/orders/new", f); status != 303 {
			t.Fatalf("%s refused: %d %s", typed, status, body)
		}
		var last register.FinanceOrder
		_ = e.st.ReadFinance(key, func(d *register.FinanceData) { last = d.Orders[len(d.Orders)-1] })
		if last.AgreedPaise == nil || *last.AgreedPaise != want || last.AgreedKind != "exact" {
			t.Errorf("%s stored as %v", typed, last.AgreedPaise)
		}
	}

	// Everything a person could type that is not an amount.
	for _, typed := range []string{"0", "-5", "5,000", "5e3", "5.005", "abc", "99999999999999999999"} {
		f := orderForm("Sharma Events", chairs, "10", "rent")
		f.Set("agreedTotal", typed)
		f.Set("agreedKind", "exact")
		status, body := admin.post(t, "/finance/orders/new", f)
		if status != 200 || !strings.Contains(body, register.MoneyRefusal) {
			t.Errorf("%q gave %d without the refusal", typed, status)
		}
	}

	// A total with no estimate/exact choice is refused.
	f := orderForm("Sharma Events", chairs, "10", "rent")
	f.Set("agreedTotal", "5000")
	if status, body := admin.post(t, "/finance/orders/new", f); status != 200 || !strings.Contains(body, "estimate or exact") {
		t.Errorf("a total without a kind gave %d", status)
	}
}

func TestFinanceOrderCanDeliberatelyCreateInventoryProduct(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	status, body := admin.post(t, "/finance/product/new", url.Values{
		"line": {"0"}, "partyName": {"Sharma Events"},
		"productId": {""}, "productName": {"Tents"},
		"quantity": {"20"}, "basis": {"rent"},
		"orderedAt": {"2026-09-03T11:00"},
	})
	if status != 303 {
		t.Fatalf("creating Tents = %d %s", status, body)
	}
	tents := productIDNamed(t, e, "Tents")

	// Attributed to the authenticated financial person, and audited inside the
	// vault with the immutable account identity.
	e.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.ID == tents && p.CreatedBy != "Asha Mehta" {
				t.Errorf("Tents recorded by %q", p.CreatedBy)
			}
		}
	})
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		found := false
		for _, a := range f.Audit {
			if a.Kind == "product_created" && a.EntityID == tents {
				found = true
				if a.ByMobile != "9886140023" {
					t.Errorf("audit mobile is %q", a.ByMobile)
				}
			}
		}
		if !found {
			t.Error("no product_created audit event")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// It is an ordinary product now: zero on hand, on Stock, in the inward picker.
	e.st.Read(func(reg *register.Register) {
		if got := register.OnHand(reg, tents); got != 0 {
			t.Errorf("Tents starts at %d on hand", got)
		}
	})
	for _, path := range []string{"/stock", "/inward/new", "/api/products?mode=all&q=ten"} {
		_, body := e.get(path)
		if !strings.Contains(body, "Tents") {
			t.Errorf("%s does not offer Tents", path)
		}
		for _, secret := range []string{"Sharma Events", "Agreed total", "ORD-"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s leaked %q", path, secret)
			}
		}
	}
}

func TestFinanceProductCreationKeepsDuplicateGuards(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	baseline := 0
	e.st.Read(func(reg *register.Register) { baseline = len(reg.Products) })

	draft := func(name string) url.Values {
		return url.Values{
			"line": {"0"}, "partyName": {"Sharma Events"},
			"productId": {""}, "productName": {name},
			"quantity": {"20"}, "basis": {"rent"},
			"orderedAt": {"2026-09-03T11:00"},
		}
	}

	// A case-fold duplicate is refused outright, with the ordinary wording.
	status, body := admin.post(t, "/finance/product/new", draft("chairs"))
	if status != 200 || !strings.Contains(body, "Chairs is already on the list. Pick it.") {
		t.Fatalf("duplicate gave %d: %s", status, body)
	}

	// A four-character near duplicate asks once.
	status, body = admin.post(t, "/finance/product/new", draft("Chair"))
	if status != 200 || !strings.Contains(body, "is already on the list. Is") {
		t.Fatalf("near duplicate gave %d: %s", status, body)
	}
	countProducts := func() int {
		n := 0
		e.st.Read(func(reg *register.Register) { n = len(reg.Products) })
		return n
	}
	if countProducts() != baseline {
		t.Fatal("a refused creation added a product")
	}

	// Confirming once creates the separate product.
	confirmed := draft("Chair")
	confirmed.Set("confirm", "yes")
	if status, body := admin.post(t, "/finance/product/new", confirmed); status != 303 {
		t.Fatalf("confirmed creation gave %d: %s", status, body)
	}
	if countProducts() != baseline+1 {
		t.Fatal("the confirmed product was not created")
	}

	// A hand-built post claiming confirmation cannot bypass the fold guard,
	// because the check runs again inside the write.
	bypass := draft("CHAIRS")
	bypass.Set("confirm", "yes")
	if status, body := admin.post(t, "/finance/product/new", bypass); status != 200 || !strings.Contains(body, "already on the list") {
		t.Fatalf("the fold guard was bypassed: %d %s", status, body)
	}
	if countProducts() != baseline+1 {
		t.Fatal("a bypassed post created a product")
	}
}

func TestOrderConcurrentNewValueResolutionDoesNotDuplicate(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	second, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")
	chairs := productIDNamed(t, e, "Chairs")

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, c := range []*testClient{admin, second} {
		wg.Add(1)
		go func(i int, c *testClient) {
			defer wg.Done()
			codes[i], _ = c.post(t, "/finance/orders/new", orderForm("Sharma Events", chairs, "10", "rent"))
		}(i, c)
	}
	wg.Wait()

	saved := 0
	for _, code := range codes {
		if code == 303 {
			saved++
		}
	}
	if saved == 0 {
		t.Fatalf("neither order saved: %v", codes)
	}

	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		parties := register.LiveFinanceValues(f, register.FinanceParty)
		if len(parties) != 1 {
			t.Fatalf("%d parties for one supplier: %+v", len(parties), parties)
		}
		if len(f.Orders) != saved {
			t.Fatalf("%d orders stored for %d saves", len(f.Orders), saved)
		}
		for _, o := range f.Orders {
			if o.PartyID != parties[0].ID {
				t.Errorf("%s points at %s, not the one party", o.ID, o.PartyID)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	// The vault still decrypts and validates after a restart.
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

func TestInventoryReceivesPartialOrderWithoutOrderKnowledge(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	// Finance orders 100 Tents, creating the product deliberately.
	if status, _ := admin.post(t, "/finance/product/new", url.Values{
		"line": {"0"}, "partyName": {"Sharma Events"},
		"productId": {""}, "productName": {"Tents"},
		"quantity": {"100"}, "basis": {"rent"},
		"orderedAt": {"2026-09-03T11:00"},
	}); status != 303 {
		t.Fatal("creating Tents failed")
	}
	tents := productIDNamed(t, e, "Tents")
	if status, body := admin.post(t, "/finance/orders/new", orderForm("Sharma Events", tents, "100", "rent")); status != 303 {
		t.Fatalf("order=%d %s", status, body)
	}

	// The desk receives 70, then 30, knowing nothing about the order.
	for _, quantity := range []string{"70", "30"} {
		resp, body := e.post("/inward/new", url.Values{
			"productId": {tents}, "quantity": {quantity}, "basis": {"rent"},
			"supplier": {"Sharma Events"}, "receivedOn": {"2026-09-03"},
		})
		if resp.StatusCode != 303 {
			t.Fatalf("receiving %s gave %d: %s", quantity, resp.StatusCode, body)
		}
	}
	e.st.Read(func(reg *register.Register) {
		if got := register.OnHand(reg, tents); got != 100 {
			t.Errorf("on hand is %d, want 100", got)
		}
		tentInwards := 0
		for _, in := range reg.Inwards {
			if in.ProductID == tents && in.Deleted == nil {
				tentInwards++
			}
		}
		if tentInwards != 2 {
			t.Errorf("%d Tents inward entries, want 2", tentInwards)
		}
	})

	// The order is untouched and still open.
	o := onlyOrder(t, e, key)
	if o.Status != "open" || o.Lines[0].ExpectedQuantity != 100 || len(o.Changes) != 0 {
		t.Errorf("the order changed: %+v", o)
	}

	// No ordinary page mentions the order, its party list or any money.
	for _, path := range []string{"/stock", "/inwards", "/suppliers", "/log?day=all", "/inward/new"} {
		_, body := e.get(path)
		for _, secret := range []string{"ORD-0001", "Expected quantity", "Agreed total", "₹", "/finance/orders"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s leaked %q", path, secret)
			}
		}
	}
	// The protected party list never populates the inward supplier suggestions.
	_, inward := e.get("/inward/new")
	if strings.Contains(inward, "/finance/api/values") {
		t.Error("the inward form reaches the protected value list")
	}
}

// saveOrder records one order and returns its ID.
func saveOrder(t *testing.T, e *env, admin *testClient, key []byte, form url.Values) string {
	t.Helper()
	if status, body := admin.post(t, "/finance/orders/new", form); status != 303 {
		t.Fatalf("order save = %d: %s", status, body)
	}
	id := ""
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) { id = f.Orders[len(f.Orders)-1].ID })
	return id
}

func orderByID(t *testing.T, e *env, key []byte, id string) register.FinanceOrder {
	t.Helper()
	var o register.FinanceOrder
	ok := false
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) { o, ok = register.FinanceOrderByID(f, id) }); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("no order %s", id)
	}
	return o
}

// TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear covers everything the
// correction contract reaches today. The movement-linked half of the spec's
// scenario needs spec 19's money movements, which do not exist yet: the guard
// and its exact refusal are implemented and exercised through
// register.FinanceLineIsReferenced, which returns false until spec 19 fills it
// in. See HANDOFF.md, "Known gap: two spec-18 tests depend on spec 19".
func TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")

	form := orderForm("Sharma Events", chairs, "100", "rent")
	form.Set("agreedTotal", "25000")
	form.Set("agreedKind", "estimated")
	form.Set("remarks", "Delivery on the 4th")
	id := saveOrder(t, e, admin, key, form)
	created := orderByID(t, e, key, id)

	// Correct every field at once.
	edit := url.Values{
		"partyName": {"Sharma Tent House"},
		"lineId":    {created.Lines[0].ID, ""},
		"productId": {chairs, tables}, "productName": {"", ""},
		"quantity": {"120", "40"}, "basis": {"rent", "purchase"},
		"orderedAt":   {"2026-09-03T09:30"},
		"agreedTotal": {"31000.50"}, "agreedKind": {"exact"},
		"remarks": {""},
	}
	if status, body := admin.post(t, "/finance/orders/"+id+"/edit", edit); status != 303 {
		t.Fatalf("correction = %d: %s", status, body)
	}

	o := orderByID(t, e, key, id)
	// What was recorded when, and by whom, is never rewritten by a correction.
	if !o.CreatedAt.Equal(created.CreatedAt) || o.CreatedByID != created.CreatedByID {
		t.Errorf("the correction rewrote who recorded the order")
	}
	if o.Lines[0].ID != created.Lines[0].ID {
		t.Errorf("the kept line changed id")
	}

	// One change per changed field, in the spec's order.
	var got []string
	for _, c := range o.Changes {
		got = append(got, c.Field)
		if c.ByAccountID != adminID || c.ByMobile != "9886140023" || !c.At.Equal(orderNow) {
			t.Errorf("change %s is attributed to %+v", c.Field, c)
		}
	}
	want := "party,products,agreedTotal,agreedKind,orderedAt,remarks"
	if strings.Join(got, ",") != want {
		t.Fatalf("changes are %v, want %s", got, want)
	}
	byField := map[string]register.FinanceChange{}
	for _, c := range o.Changes {
		byField[c.Field] = c
	}
	for field, wants := range map[string][3]string{
		"party":       {"Supplier or other party", "Sharma Events", "Sharma Tent House"},
		"products":    {"Products ordered", "100 Chairs — rent", "120 Chairs — rent; 40 Round tables — purchase"},
		"agreedTotal": {"Agreed total", "₹25,000.00", "₹31,000.50"},
		"agreedKind":  {"Estimate or exact", "Estimate", "Exact"},
		"orderedAt":   {"Order date and time", "3 September 2026 · 11:00 am", "3 September 2026 · 9:30 am"},
		"remarks":     {"Remarks", "Delivery on the 4th", "Blank"},
	} {
		c := byField[field]
		if c.Label != wants[0] || c.From != wants[1] || c.To != wants[2] {
			t.Errorf("%s recorded as %q: %q → %q", field, c.Label, c.From, c.To)
		}
	}

	// One order_edited event, after the field changes.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		edits := 0
		for _, a := range f.Audit {
			if a.Kind == "order_edited" && a.EntityID == id {
				edits++
			}
		}
		if edits != 1 {
			t.Errorf("%d order_edited events", edits)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Submitting the same values again records nothing.
	if status, _ := admin.post(t, "/finance/orders/"+id+"/edit", edit); status != 303 {
		t.Fatal("the no-op correction was refused")
	}
	if again := orderByID(t, e, key, id); len(again.Changes) != len(o.Changes) {
		t.Errorf("a no-op correction recorded %d changes", len(again.Changes)-len(o.Changes))
	}

	// The refusal a movement-linked line gets is implemented and worded exactly.
	if register.ErrLineUsed.Error() != "This product is already used by a ledger entry." {
		t.Errorf("the refusal reads %q", register.ErrLineUsed)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if got := lineRefusal(f, o.Lines, nil); got != register.ErrLineUsed.Error() {
			// No movement exists yet, so no line is referenced and nothing is
			// refused. Spec 19 makes this assertion bite.
			if got != "" {
				t.Errorf("lineRefusal said %q", got)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCancelPaidUndeliveredOrderKeepsHistory covers the cancellation contract.
// Its "after a movement from spec 19" half is the named gap: no payment type
// exists to record yet. See HANDOFF.md.
func TestCancelPaidUndeliveredOrderKeepsHistory(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	id := saveOrder(t, e, admin, key, orderForm("Sharma Events", chairs, "100", "rent"))

	beforeProducts, beforeStock := 0, onHand(e)
	e.st.Read(func(reg *register.Register) { beforeProducts = len(reg.Products) })

	// A cancellation must say why.
	if status, body := admin.post(t, "/finance/orders/"+id+"/cancel", nil); status != 200 ||
		!strings.Contains(body, "Cancel this order?") {
		t.Fatalf("a blank reason gave %d", status)
	}
	if orderByID(t, e, key, id).Status != "open" {
		t.Fatal("a refused cancellation changed the order")
	}

	if status, body := admin.post(t, "/finance/orders/"+id+"/cancel",
		url.Values{"reason": {"Supplier could not deliver"}, "confirm": {"yes"}}); status != 303 {
		t.Fatalf("cancel = %d: %s", status, body)
	}

	o := orderByID(t, e, key, id)
	if o.Status != "cancelled" {
		t.Fatalf("status is %q", o.Status)
	}
	// The order and its products stay. Nothing is erased.
	if len(o.Lines) != 1 || o.Lines[0].ExpectedQuantity != 100 {
		t.Errorf("cancellation changed the lines: %+v", o.Lines)
	}
	after := 0
	e.st.Read(func(reg *register.Register) { after = len(reg.Products) })
	if after != beforeProducts {
		t.Error("cancelling an order deleted a product")
	}
	if got := onHand(e); !sameStock(beforeStock, got) {
		t.Error("cancelling an order moved stock")
	}

	// It is audited with the reason, and still visible on both screens.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		found := false
		for _, a := range f.Audit {
			if a.Kind == "order_cancelled" && a.EntityID == id {
				found = true
				if a.Summary != "Supplier could not deliver" || a.Before != "open" || a.After != "cancelled" {
					t.Errorf("cancellation audit is %+v", a)
				}
			}
		}
		if !found {
			t.Error("no order_cancelled audit event")
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/finance/orders", "/finance/orders/" + id} {
		status, body := admin.get(t, path)
		if status != 200 || !strings.Contains(body, "Cancelled") {
			t.Errorf("%s = %d and does not show the cancellation", path, status)
		}
	}

	// Cancelling twice is refused rather than recorded twice.
	if status, _ := admin.post(t, "/finance/orders/"+id+"/cancel", url.Values{"reason": {"again"}, "confirm": {"yes"}}); status != 200 {
		t.Error("a second cancellation was accepted")
	}
}

func TestOrderListAndDetailShowSnapshotsAndRenames(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	id := saveOrder(t, e, admin, key, orderForm("Sharma Events", chairs, "100", "rent"))

	// The inventory side renames the product. The order keeps its own label.
	if err := e.st.Update(func(reg *register.Register) error {
		return register.RenameProduct(reg, chairs, "Folding chairs", "Suresh Kumar", orderNow)
	}); err != nil {
		t.Fatal(err)
	}
	if got := orderByID(t, e, key, id).Lines[0].ProductNameSnapshot; got != "Chairs" {
		t.Fatalf("the snapshot was rewritten to %q", got)
	}
	status, body := admin.get(t, "/finance/orders/"+id)
	if status != 200 {
		t.Fatalf("detail = %d", status)
	}
	if !strings.Contains(body, "Chairs") || !strings.Contains(body, "now called Folding chairs") {
		t.Errorf("the detail page does not show the snapshot and the new name: %s", body)
	}
}
