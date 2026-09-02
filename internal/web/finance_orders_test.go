package web

import (
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
