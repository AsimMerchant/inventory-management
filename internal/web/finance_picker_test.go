package web

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"storeregister/internal/register"
)

// newProduct creates a product at the desk, the way the catalogue is really
// filled, and returns its ID.
func newProduct(t *testing.T, e *env, name string) string {
	t.Helper()
	existing := ""
	e.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.Deleted == nil && p.Name == name {
				existing = p.ID
			}
		}
	})
	if existing != "" {
		return existing
	}
	resp, body := e.post("/product/new", url.Values{
		"name": {name}, "return": {"/inward/new"}, "confirm": {"yes"},
	})
	if resp.StatusCode != 303 {
		t.Fatalf("creating %q = %d: %s", name, resp.StatusCode, body)
	}
	return productIDNamed(t, e, name)
}

// suggestionsOf asks the protected picker route and decodes the answer.
func suggestionsOf(t *testing.T, admin *testClient, query string) []suggestion {
	t.Helper()
	status, body := admin.get(t, "/finance/api/products?"+query)
	if status != 200 {
		t.Fatalf("the picker route answered %d for %q", status, query)
	}
	var rows []suggestion
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, body)
	}
	return rows
}

func names(rows []suggestion) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// TestSettlementProductIsTypedNotScrolled covers the defect a real event finds
// and three fixtures never do. The supplier return screen used to render every
// returnable product as one <select>. With forty products from five suppliers
// that is a scrolling list nobody reads, and the number that decides how many
// may go back was not on it. The screen must behave like every other product
// box in the register: type a few letters, see what matches with its number,
// press the one you mean.
func TestSettlementProductIsTypedNotScrolled(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	// A supplier with more returnable products than the picker will ever show
	// at once, plus one from somebody else that must never appear.
	want := []string{
		"Chairs", "Chair covers", "Charcoal sacks", "Chafing dishes",
		"Carpet rolls", "Cable reels", "Ceiling fans", "Cooking vessels",
		"Tents", "Tent poles",
	}
	for i, name := range want {
		receive(t, e, newProduct(t, e, name), 100+i, "rent", "Sharma Tent House", "2026-09-03")
	}
	other := newProduct(t, e, "Chandeliers")
	receive(t, e, other, 50, "rent", "Gupta Traders", "2026-09-03")

	// The form points its picker at the protected route, and tells it where the
	// supplier is, because what may go back depends on who it came from.
	status, form := admin.get(t, "/finance/supplier-returns/new")
	if status != 200 {
		t.Fatalf("the form = %d", status)
	}
	if !strings.Contains(form, `data-endpoint="/finance/api/products"`) {
		t.Error("the settlement form has no type-to-find product picker")
	}
	if !strings.Contains(form, `data-mode="return"`) {
		t.Error("the picker does not ask for returnable products")
	}
	if !strings.Contains(form, "data-party-from=") {
		t.Error("the picker cannot tell the server which supplier was chosen")
	}
	// The markup alone is not the fix. Without its script the box is an inert
	// text field that finds nothing, which is exactly how this shipped once.
	if !strings.Contains(form, `src="/static/picker.js"`) {
		t.Error("the settlement form never loads the picker's script")
	}

	// Typing "ch" narrows forty products to the handful that match, with the
	// number on every row.
	rows := suggestionsOf(t, admin, "mode=return&party=Sharma+Tent+House&q=ch")
	if len(rows) == 0 {
		t.Fatal("typing ch found nothing")
	}
	if len(rows) > maxSuggestions {
		t.Errorf("the picker offered %d rows, more than a person reads", len(rows))
	}
	for _, r := range rows {
		if !strings.Contains(strings.ToLower(r.Name), "ch") {
			t.Errorf("%q does not match ch", r.Name)
		}
		if !strings.Contains(r.Label, strconv.Itoa(r.OnHand)+" available") {
			t.Errorf("the row for %q does not say how many may go back: %q", r.Name, r.Label)
		}
	}
	// Another supplier's goods are offered too, because a return is not
	// confined to the supplier who sent them: they are handed to whoever is
	// taking them back.
	if !containsName(rows, "Chandeliers") {
		t.Errorf("goods from another supplier are hidden: %v", names(rows))
	}
	// Names starting with the query come first: Chafing dishes before the rest.
	got := names(rows)
	if got[0] != "Chafing dishes" {
		t.Errorf("the closest match is not first: %v", got)
	}

	// The list is the same before a supplier is named, so the two fields can be
	// filled in either order and neither disturbs the other.
	early := suggestionsOf(t, admin, "mode=return&q=ch")
	if len(early) != len(rows) {
		t.Errorf("naming a supplier changed the list: %v then %v", names(early), names(rows))
	}

	// Somebody who does want one supplier's goods only can ask for that, and
	// then the others drop out. It is a tick they choose, never a step forced
	// on them.
	narrowed := suggestionsOf(t, admin, "mode=return&onlyParty=yes&party=Sharma+Tent+House&q=ch")
	if len(narrowed) == 0 {
		t.Fatal("narrowing to one supplier offered nothing")
	}
	if containsName(narrowed, "Chandeliers") {
		t.Errorf("narrowing to Sharma still offers Gupta's goods: %v", names(narrowed))
	}
	if len(narrowed) >= len(rows) {
		t.Errorf("narrowing did not shorten the list: %v vs %v", names(narrowed), names(rows))
	}

	// And the screen still saves.
	chairs := productIDNamed(t, e, "Chairs")
	status, body := admin.post(t, "/finance/supplier-returns/new", url.Values{
		"partyName": {"Sharma Tent House"}, "productId": {chairs},
		"quantity": {"40"}, "at": {"2026-09-06T10:00"},
	})
	if status != 303 {
		t.Fatalf("the return = %d: %s", status, body)
	}
	if got := stockOf(t, e, chairs); got != 60 {
		t.Errorf("stock is %d, want 60", got)
	}
}

// TestSaleProductPickerOffersOnlyWhatWasBought is the other half: a sale is
// capped by what was purchased, so the picker must not offer rented goods.
func TestSaleProductPickerOffersOnlyWhatWasBought(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	bought := newProduct(t, e, "Steel thalis")
	rented := newProduct(t, e, "Stage platforms")
	receive(t, e, bought, 500, "purchase", "Patel Caterers Supply", "2026-09-03")
	receive(t, e, rented, 20, "rent", "Sharma Tent House", "2026-09-03")

	rows := suggestionsOf(t, admin, "mode=sale&q=st")
	if got := names(rows); len(got) != 1 || got[0] != "Steel thalis" {
		t.Errorf("the sale picker offered %v, want only Steel thalis", got)
	}
	if rows[0].Label != "Steel thalis — 500 available" {
		t.Errorf("the row reads %q", rows[0].Label)
	}
}

// TestMoneyProductsAreChosenOneAtATime covers the control the user found by
// asking how to use it. Related products was a multi-select: choosing more than
// one meant knowing to hold Ctrl, an ordinary click silently threw away
// everything already picked, and forty products sat in a four-row window. The
// replacement is the same promise as every other box: type, press, and see what
// you chose.
func TestMoneyProductsAreChosenOneAtATime(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := newProduct(t, e, "Chairs")
	tents := newProduct(t, e, "Tents")
	drums := newProduct(t, e, "Water drums (20L)")

	status, form := admin.get(t, "/finance/movements/new")
	if status != 200 {
		t.Fatalf("the money form = %d", status)
	}
	if !strings.Contains(form, `data-multi data-field="productIds-0"`) {
		t.Error("the money form has no type-to-find product control")
	}
	if !strings.Contains(form, `data-endpoint="/finance/api/products"`) {
		t.Error("the control does not ask the protected product route")
	}
	if !strings.Contains(form, `src="/static/multi-picker.js"`) {
		t.Error("the money form never loads the control's script")
	}
	// The old control must be gone from the page itself. It survives only
	// inside <noscript>, where a keyboard convention is the only thing left.
	visible := noscriptStripped(form)
	if strings.Contains(visible, `name="productIds-0" multiple`) {
		t.Error("the multi-select the user could not work out is still on screen")
	}

	// Several products on one entry still save, and all of them come back.
	status, body := admin.post(t, "/finance/movements/new", url.Values{
		"direction-0": {"out"}, "amount": {"3000"},
		"occurredAt": {"2026-09-03T11:00"},
		"partyName":  {"Bala Transport"}, "purposeName": {"Freight"},
		"modeName":     {"Cash"},
		"productIds-0": {chairs, tents, drums},
	})
	if status != 303 {
		t.Fatalf("the money entry = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Movements) != 1 {
			t.Fatalf("%d movements stored, want 1", len(f.Movements))
		}
		if got := len(f.Movements[0].Products); got != 3 {
			t.Fatalf("%d products on the entry, want 3", got)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Reopening the entry shows all three back as removable choices, not as a
	// selection hidden inside a scrolling box.
	_, list := admin.get(t, "/finance/journal")
	id := ""
	for _, part := range strings.Split(list, `"`) {
		if strings.HasPrefix(part, "/finance/movements/") && strings.HasSuffix(part, "/edit") {
			id = part
		}
	}
	if id == "" {
		t.Fatal("the journal offers no way to correct the entry")
	}
	status, edit := admin.get(t, id)
	if status != 200 {
		t.Fatalf("the correction form = %d", status)
	}
	for _, name := range []string{"Chairs", "Tents", "Water drums (20L)"} {
		if !strings.Contains(edit, `<span>`+name+`</span>`) {
			t.Errorf("%q is not shown as a chosen product on the correction form", name)
		}
	}
	if strings.Count(edit, "chosen-off") < 3 {
		t.Error("the chosen products cannot be taken off again")
	}
}

// noscriptStripped removes every <noscript> block, leaving what a person with
// script on actually sees.
func noscriptStripped(page string) string {
	for {
		i := strings.Index(page, "<noscript>")
		if i < 0 {
			return page
		}
		j := strings.Index(page[i:], "</noscript>")
		if j < 0 {
			return page[:i]
		}
		page = page[:i] + page[i+j+len("</noscript>"):]
	}
}

// TestSplitMoneyEntryKeepsEachRowsProducts covers the headline the release notes
// make: one payment split across several purposes, recorded together. Each
// amount has its own product control, and the chips on one row must not leak
// onto another or vanish when the page is redrawn to add a row.
func TestSplitMoneyEntryKeepsEachRowsProducts(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := newProduct(t, e, "Chairs")
	tents := newProduct(t, e, "Tents")

	// Adding a second amount redraws the page. The first row's chosen product
	// has to come back with it.
	status, page := admin.post(t, "/finance/movements/new", url.Values{
		"direction-0": {"out"}, "amount": {"4000"},
		"occurredAt": {"2026-09-03T11:00"},
		"partyName":  {"Bala Transport"}, "purposeName": {"Freight"},
		"modeName":     {"Cash"},
		"productIds-0": {chairs},
		"addRow":       {"yes"},
	})
	if status != 200 {
		t.Fatalf("adding a second amount = %d", status)
	}
	if !strings.Contains(page, `data-multi data-field="productIds-1"`) {
		t.Error("the second amount has no product control of its own")
	}
	if !strings.Contains(noscriptStripped(page), "<span>Chairs</span>") {
		t.Error("the first amount lost its product when the row was added")
	}

	// Saving both: each entry keeps only its own products.
	status, body := admin.post(t, "/finance/movements/new", url.Values{
		"direction-0": {"out"}, "direction-1": {"out"},
		"amount":       {"4000", "1500"},
		"occurredAt":   {"2026-09-03T11:00", "2026-09-03T11:05"},
		"partyName":    {"Bala Transport", "Bala Transport"},
		"purposeName":  {"Freight", "Unloading labour"},
		"modeName":     {"Cash", "Cash"},
		"productIds-0": {chairs},
		"productIds-1": {tents},
	})
	if status != 303 {
		t.Fatalf("the split entry = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Movements) != 2 {
			t.Fatalf("%d movements stored, want 2", len(f.Movements))
		}
		for _, m := range f.Movements {
			if len(m.Products) != 1 {
				t.Fatalf("an entry carries %d products, want 1", len(m.Products))
			}
		}
		got := []string{f.Movements[0].Products[0].ProductName, f.Movements[1].Products[0].ProductName}
		sort.Strings(got)
		if got[0] != "Chairs" || got[1] != "Tents" {
			t.Errorf("the two entries carry %v, want Chairs and Tents one each", got)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCorrectionCanTakeEveryProductOff is the path the old multi-select made
// impossible to reason about: emptying the list. Taking every chip off submits
// no productIds at all, and the entry must end up with none rather than
// silently keeping what it had.
func TestCorrectionCanTakeEveryProductOff(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := newProduct(t, e, "Chairs")
	tents := newProduct(t, e, "Tents")

	if status, body := admin.post(t, "/finance/movements/new", url.Values{
		"direction-0": {"out"}, "amount": {"1500"},
		"occurredAt": {"2026-09-03T11:00"},
		"partyName":  {"Bala Transport"}, "purposeName": {"Freight"},
		"modeName":     {"Cash"},
		"productIds-0": {chairs, tents},
	}); status != 303 {
		t.Fatalf("the entry = %d: %s", status, body)
	}
	id := ""
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) { id = f.Movements[0].ID }); err != nil {
		t.Fatal(err)
	}

	status, body := admin.post(t, "/finance/movements/"+id+"/edit", url.Values{
		"direction-0": {"out"}, "amount": {"1500"},
		"occurredAt": {"2026-09-03T11:00"},
		"partyName":  {"Bala Transport"}, "purposeName": {"Freight"},
		"modeName": {"Cash"},
		// No productIds at all: every chip was taken off.
	})
	if status != 303 {
		t.Fatalf("the correction = %d: %s", status, body)
	}
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		m := f.Movements[0]
		if len(m.Products) != 0 {
			t.Errorf("the entry still carries %d products after all were taken off", len(m.Products))
		}
		// And the audit says so, rather than the change happening silently.
		found := false
		for _, c := range m.Changes {
			if c.Field == "products" && c.From == "Chairs; Tents" {
				found = true
			}
		}
		if !found {
			t.Errorf("clearing the products was not recorded: %+v", m.Changes)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAvailabilityBoxIsKeptInStepWithThePicker is the defect the user found on
// the sale screen: the product was chosen and correct, and "Available to sell"
// sat there reading 0. The number is drawn by the server for whatever was
// picked last, so choosing a product in the browser has to update it. The
// picker carries the number on every row already; the box has to be told which
// element to write it into.
func TestAvailabilityBoxIsKeptInStepWithThePicker(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	barricades := newProduct(t, e, "Barricades")
	receive(t, e, barricades, 427, "purchase", "Gupta Traders", "2026-09-03")

	for _, path := range []string{"/finance/sales/new", "/finance/supplier-returns/new"} {
		status, form := admin.get(t, path)
		if status != 200 {
			t.Fatalf("%s = %d", path, status)
		}
		if !strings.Contains(form, "<output class=\"inp\" data-available>") {
			t.Errorf("%s: the availability box is not named, so nothing can fill it", path)
		}
		if !strings.Contains(form, `data-count-into="[data-available]"`) {
			t.Errorf("%s: the picker is not told where to put the number", path)
		}
	}

	// And the number the picker would write is the real one.
	rows := suggestionsOf(t, admin, "mode=sale&q=Barr")
	if len(rows) != 1 || rows[0].OnHand != 427 {
		t.Fatalf("the sale picker offered %+v, want Barricades with 427", rows)
	}
}

// TestSaleProductSurvivesChoosingTheBuyer is the user's report: filling the
// product first and the party second wiped the product out. What may be sold
// does not depend on who is buying — only a supplier return does — so the sale
// screen must not tie the two fields together at all.
func TestSaleProductSurvivesChoosingTheBuyer(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	_, sale := admin.get(t, "/finance/sales/new")
	if strings.Contains(sale, "data-party-from=") {
		t.Error("the sale screen ties the product to the buyer, so naming the buyer clears it")
	}
	if strings.Contains(sale, "data-wait=") {
		t.Error("the sale screen tells people to name a party first, which is not true")
	}

	// The return screen is the opposite: the list genuinely depends on the
	// supplier, so it says which field to fill in first rather than finding
	// nothing and looking broken.
	_, ret := admin.get(t, "/finance/supplier-returns/new")
	if !strings.Contains(ret, "data-party-from=") {
		t.Error("the return screen does not know which supplier the list depends on")
	}
	if strings.Contains(ret, "data-wait=") {
		t.Error("the return screen still tells people which field to fill first")
	}

	// And the server agrees: a sale ignores the party, a return does not.
	bought := newProduct(t, e, "Steel thalis")
	receive(t, e, bought, 500, "purchase", "Patel Caterers Supply", "2026-09-03")
	if rows := suggestionsOf(t, admin, "mode=sale&q=Steel"); len(rows) != 1 {
		t.Errorf("the sale list with no buyer named offered %v, want Steel thalis", names(rows))
	}
	if rows := suggestionsOf(t, admin, "mode=sale&party=Anybody+At+All&q=Steel"); len(rows) != 1 {
		t.Errorf("naming a buyer changed the sale list: %v", names(rows))
	}
}

// TestSettlementReadsTheProductWithNoScript covers a defect only a browser with
// JavaScript switched off can see. The picker submits a hidden productId and
// its <noscript> fallback submits a <select> of the same name, so with no
// script both arrive and the empty hidden one is first. r.FormValue takes the
// first, so the screen refused every save with "Pick the product from the
// list." while a product was plainly chosen on it. formProductID exists for
// exactly this and the settlement draft was not using it.
func TestSettlementReadsTheProductWithNoScript(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")
	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")

	// Exactly what a scriptless browser sends: the empty hidden field, then the
	// fallback select's real answer.
	status, body := admin.post(t, "/finance/supplier-returns/new", url.Values{
		"partyName": {"Sharma Events"},
		"productId": {"", tables},
		"quantity":  {"40"}, "at": {"2026-09-06T10:00"},
	})
	if status != 303 {
		t.Fatalf("the return = %d: %s", status, body)
	}
	if got := stockOf(t, e, tables); got != 60 {
		t.Errorf("stock is %d, want 60", got)
	}

	// And a genuinely empty answer is still refused, rather than the fix
	// silently accepting anything.
	status, body = admin.post(t, "/finance/supplier-returns/new", url.Values{
		"partyName": {"Sharma Events"},
		"productId": {"", ""},
		"quantity":  {"10"}, "at": {"2026-09-06T10:00"},
	})
	if status != 200 || !strings.Contains(body, "Pick the product from the list.") {
		t.Errorf("naming no product = %d, and did not refuse: %s", status, body)
	}
}

func containsName(rows []suggestion, name string) bool {
	for _, r := range rows {
		if r.Name == name {
			return true
		}
	}
	return false
}

// TestNewProductCanBeAddedFromTheFinancialScreens covers a claim the release
// notes already made and the program did not keep: adding a product that is not
// on the list yet, from inside the protected area. The desk's /product/new
// cannot be reused for it — that route needs somebody on duty, and nobody is on
// duty in the financial area, so it refuses silently.
func TestNewProductCanBeAddedFromTheFinancialScreens(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	// Nobody is at the desk: the state the financial area is really used in.
	if err := e.st.Update(func(reg *register.Register) error {
		reg.OnDutyStaffID, reg.ShiftStartedAt = "", nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The ordinary route cannot serve the financial area: with nobody on duty
	// it creates nothing, which is why the protected one exists.
	_, _ = e.post("/product/new", url.Values{"name": {"Diesel generator hire"}})
	for _, n := range liveProductNames(t, e) {
		if n == "Diesel generator hire" {
			t.Fatal("the desk route created a product with nobody on duty")
		}
	}

	status, body := admin.post(t, "/finance/products/new", url.Values{
		"name": {"Diesel generator hire"},
	})
	if status != 200 {
		t.Fatalf("adding a product = %d: %s", status, body)
	}
	var made productAnswer
	if err := json.Unmarshal([]byte(body), &made); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, body)
	}
	if made.ID == "" || made.Name != "Diesel generator hire" {
		t.Fatalf("the answer was %+v", made)
	}

	// It is an ordinary product: the desk sees it, at zero, and can receive it.
	if got := productIDNamed(t, e, "Diesel generator hire"); got != made.ID {
		t.Errorf("the product is %s on the register but %s in the answer", got, made.ID)
	}
	if got := stockOf(t, e, made.ID); got != 0 {
		t.Errorf("a brand-new product starts at %d, want 0", got)
	}

	// The product invariant holds. The same name again is refused outright.
	_, body = admin.post(t, "/finance/products/new", url.Values{"name": {"diesel generator hire"}})
	var again productAnswer
	_ = json.Unmarshal([]byte(body), &again)
	if again.ID != "" || !strings.Contains(again.Error, "already on the list") {
		t.Errorf("a duplicate name was accepted: %+v", again)
	}

	// A near-duplicate takes one deliberate confirmation, and writes nothing
	// until it gets one. A split product silently halves the on-hand count.
	before := len(liveProductNames(t, e))
	_, body = admin.post(t, "/finance/products/new", url.Values{"name": {"Diesel generator hires"}})
	var near productAnswer
	_ = json.Unmarshal([]byte(body), &near)
	if !near.NeedsConfirm || near.ID != "" {
		t.Fatalf("a near-duplicate was not queried: %+v", near)
	}
	if len(liveProductNames(t, e)) != before {
		t.Error("the unconfirmed near-duplicate was written anyway")
	}
	_, body = admin.post(t, "/finance/products/new", url.Values{
		"name": {"Diesel generator hires"}, "confirm": {"yes"},
	})
	var confirmed productAnswer
	_ = json.Unmarshal([]byte(body), &confirmed)
	if confirmed.ID == "" {
		t.Errorf("the confirmed near-duplicate was still refused: %+v", confirmed)
	}

	// A stranger cannot create products through it.
	plain := e.client()
	resp, err := plain.PostForm(e.URL+"/finance/products/new", url.Values{"name": {"Smuggled in"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("the protected product route answered a stranger")
	}
	for _, n := range liveProductNames(t, e) {
		if n == "Smuggled in" {
			t.Fatal("a stranger created a product")
		}
	}

	// And both screens actually offer it, with the token the write needs.
	for _, path := range []string{"/finance/orders/new", "/finance/movements/new"} {
		_, form := admin.get(t, path)
		if !strings.Contains(form, `data-new-endpoint="/finance/products/new"`) {
			t.Errorf("%s offers no way to add a product that is not on the list", path)
		}
		if !strings.Contains(form, "data-csrf=") {
			t.Errorf("%s cannot authorise the write", path)
		}
	}
}

func liveProductNames(t *testing.T, e *env) []string {
	t.Helper()
	out := []string{}
	e.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.Deleted == nil {
				out = append(out, p.Name)
			}
		}
	})
	return out
}
