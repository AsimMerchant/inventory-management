package web

import (
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// Step one of merging Record an order into Record money. The money screen now
// carries product lines, and a line with an agreed quantity of 1 or more makes
// the order itself. Nobody at the desk ever sees an order form.

func orders(t *testing.T, e *env, key []byte) []register.FinanceOrder {
	t.Helper()
	var out []register.FinanceOrder
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		out = append(out, f.Orders...)
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// reopenValid proves the file on disk still loads. ValidateFinance runs on
// every decrypt, so an order this screen writes badly stops the whole vault
// opening, not just one screen.
func reopenValid(t *testing.T, e *env) {
	t.Helper()
	reopened, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadFinance(key, func(f *register.FinanceData) {
		if err := register.ValidateFinance(f); err != nil {
			t.Errorf("the reopened vault is invalid: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMoneyEntryMakesTheOrderItAgrees(t *testing.T) {
	type line struct{ product, quantity, basis string }
	for _, tc := range []struct {
		name      string
		lines     []line
		agreed    string
		wantOrder bool
		wantLines int    // order lines, and movement OrderLineIDs
		wantRefs  int    // product snapshots on the movement
		wantPaise int64  // 0 means no agreed total
		wantKind  string // "" means no kind stored at all
	}{
		{
			name:      "quantities and a total make a whole order",
			lines:     []line{{"Chairs", "500", "rent"}, {"Round tables", "40", "purchase"}},
			agreed:    "12000.50",
			wantOrder: true, wantLines: 2, wantRefs: 2,
			wantPaise: 1200050, wantKind: "exact",
		},
		{
			// The case that would break every later decrypt if the kind were
			// set without a total: ValidateFinance refuses a kind with no
			// amount behind it.
			name:      "quantities with no total store no kind at all",
			lines:     []line{{"Chairs", "500", "rent"}},
			wantOrder: true, wantLines: 1, wantRefs: 1,
			wantKind: "",
		},
		{
			// MOV-0018: money for goods where no number was ever agreed.
			name:      "products with no quantity stay a plain movement",
			lines:     []line{{"Chairs", "", ""}, {"Round tables", "", ""}},
			wantOrder: false, wantRefs: 2,
		},
		{
			name:      "a quantity on one line of several still makes one order",
			lines:     []line{{"Chairs", "500", "rent"}, {"Round tables", "", ""}},
			wantOrder: true, wantLines: 1, wantRefs: 2,
		},
		{
			name:      "no products at all is money on its own",
			wantOrder: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), orderNow)
			admin, key, adminID := financeAdmin(t, e)

			form := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
			for i, l := range tc.lines {
				moneyLine(form, 0, i, productIDNamed(t, e, l.product), l.quantity, l.basis)
			}
			if tc.agreed != "" {
				form.Set("agreedTotal-0", tc.agreed)
			}
			if status, body := admin.post(t, "/finance/movements/new", form); status != 303 {
				t.Fatalf("the money entry = %d: %s", status, body)
			}

			m := movements(t, e, key)[0]
			all := orders(t, e, key)
			if !tc.wantOrder {
				if len(all) != 0 {
					t.Fatalf("%d orders made, want none", len(all))
				}
				if m.OrderID != "" || len(m.OrderLineIDs) != 0 {
					t.Errorf("the movement points at %q %v", m.OrderID, m.OrderLineIDs)
				}
			} else {
				if len(all) != 1 {
					t.Fatalf("%d orders made, want 1", len(all))
				}
				o := all[0]
				if m.OrderID != o.ID {
					t.Errorf("the movement points at %q, the order is %s", m.OrderID, o.ID)
				}
				if len(o.Lines) != tc.wantLines || len(m.OrderLineIDs) != tc.wantLines {
					t.Fatalf("%d order lines and %d linked, want %d", len(o.Lines), len(m.OrderLineIDs), tc.wantLines)
				}
				for i, l := range o.Lines {
					if m.OrderLineIDs[i] != l.ID {
						t.Errorf("line %d is %s but the movement names %s", i, l.ID, m.OrderLineIDs[i])
					}
				}
				if register.FinanceValueText(mustFinance(t, e, key), o.PartyID) != "Sharma Events" {
					t.Error("the order was made against the wrong party")
				}
				if o.PartyID != m.PartyID {
					t.Error("the order and the money name different parties")
				}
				if !o.OrderedAt.Equal(m.OccurredAt) {
					t.Errorf("the order is dated %v and the money %v", o.OrderedAt, m.OccurredAt)
				}
				if o.Status != "open" || o.CreatedByID != adminID {
					t.Errorf("the order is %+v", o)
				}
				switch {
				case tc.wantPaise == 0 && o.AgreedPaise != nil:
					t.Errorf("an agreed total of %d appeared from nowhere", *o.AgreedPaise)
				case tc.wantPaise != 0 && (o.AgreedPaise == nil || *o.AgreedPaise != tc.wantPaise):
					t.Errorf("the agreed total is %v, want %d paise", o.AgreedPaise, tc.wantPaise)
				}
				if o.AgreedKind != tc.wantKind {
					t.Errorf("the total kind is %q, want %q", o.AgreedKind, tc.wantKind)
				}
			}
			if len(m.Products) != tc.wantRefs {
				t.Errorf("the movement carries %d product snapshots, want %d", len(m.Products), tc.wantRefs)
			}
			reopenValid(t, e)
		})
	}
}

func TestMoneyProductLinesAreRefusedInPlainWords(t *testing.T) {
	chairsName, tablesName := "Chairs", "Round tables"
	type line struct{ product, quantity, basis string }
	for _, tc := range []struct {
		name      string
		lines     []line
		agreed    string
		want      string
		withOrder bool
	}{
		{
			name:  "a quantity with no rent or purchase tick",
			lines: []line{{chairsName, "500", ""}},
			want:  "Say whether each product with an agreed quantity is rented or bought.",
		},
		{
			name:  "a quantity that is not a whole number",
			lines: []line{{chairsName, "five hundred", "rent"}},
			want:  "Type the agreed quantity as a whole number of 1 or more, or leave it empty.",
		},
		{
			name:  "a quantity of nought",
			lines: []line{{chairsName, "0", "rent"}},
			want:  "Type the agreed quantity as a whole number of 1 or more, or leave it empty.",
		},
		{
			name:  "the same product twice",
			lines: []line{{chairsName, "10", "rent"}, {chairsName, "5", "purchase"}},
			want:  "Pick each product from the list, and each one only once.",
		},
		{
			name:   "an agreed total with nothing agreed",
			lines:  []line{{tablesName, "", ""}},
			agreed: "5000",
			want:   "Type the agreed quantity of at least one product, or leave the agreed total empty.",
		},
		{
			name:   "an agreed total that is not money",
			lines:  []line{{tablesName, "40", "rent"}},
			agreed: "about five thousand",
			want:   register.MoneyRefusal,
		},
		{
			// Both halves are on screen at once. Taking the order and dropping
			// the typed quantity would lose what somebody agreed.
			name:      "an order chosen and products typed as well",
			withOrder: true,
			lines:     []line{{chairsName, "500", "rent"}},
			want:      "Fill in the products above, or choose an order already recorded — not both.",
		},
		{
			name:      "an order chosen and an agreed total typed as well",
			withOrder: true,
			agreed:    "5000",
			want:      "Fill in the products above, or choose an order already recorded — not both.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), orderNow)
			admin, key, _ := financeAdmin(t, e)
			existing := 0
			if tc.withOrder {
				// One order already recorded, the thing the dropdown is for.
				seed := moneyForm("out", "1000", "Sharma Events", "Advance", "Cash")
				moneyLine(seed, 0, 0, productIDNamed(t, e, tablesName), "40", "rent")
				if status, body := admin.post(t, "/finance/movements/new", seed); status != 303 {
					t.Fatalf("the seed entry = %d: %s", status, body)
				}
				existing = 1
			}
			before := mustReadFile(t, e.path)

			form := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
			if tc.withOrder {
				form.Set("orderId", orders(t, e, key)[0].ID)
			}
			for i, l := range tc.lines {
				moneyLine(form, 0, i, productIDNamed(t, e, l.product), l.quantity, l.basis)
			}
			if tc.agreed != "" {
				form.Set("agreedTotal-0", tc.agreed)
			}
			status, body := admin.post(t, "/finance/movements/new", form)
			if status != 200 || !strings.Contains(body, tc.want) {
				t.Fatalf("gave %d without %q", status, tc.want)
			}
			if len(movements(t, e, key)) != existing || len(orders(t, e, key)) != existing {
				t.Error("a refused entry wrote something")
			}
			if string(mustReadFile(t, e.path)) != string(before) {
				t.Error("a refused entry changed the register file")
			}
		})
	}
}

// TestSecondPaymentPointsAtTheOrderAlreadyThere is why the order dropdown
// stays: an instalment against an agreement already recorded must attach to it
// rather than mint a second one.
func TestSecondPaymentPointsAtTheOrderAlreadyThere(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	first := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(first, 0, 0, productIDNamed(t, e, "Chairs"), "500", "rent")
	first.Set("agreedTotal-0", "12000")
	if status, body := admin.post(t, "/finance/movements/new", first); status != 303 {
		t.Fatalf("the first payment = %d: %s", status, body)
	}
	made := orders(t, e, key)
	if len(made) != 1 {
		t.Fatalf("%d orders after the first payment", len(made))
	}
	orderID := made[0].ID

	// The order the money screen made is offered on the screen itself.
	status, page := admin.get(t, "/finance/movements/new")
	if status != 200 || !strings.Contains(page, `value="`+orderID+`"`) {
		t.Fatalf("the money form does not offer %s", orderID)
	}

	balance := moneyForm("out", "7000", "Sharma Events", "Balance", "Bank transfer")
	balance.Set("orderId", orderID)
	if status, body := admin.post(t, "/finance/movements/new", balance); status != 303 {
		t.Fatalf("the balance payment = %d: %s", status, body)
	}

	if again := orders(t, e, key); len(again) != 1 {
		t.Fatalf("%d orders after the second payment, want 1", len(again))
	}
	all := movements(t, e, key)
	if len(all) != 2 || all[1].OrderID != orderID {
		t.Fatalf("the second payment points at %q", all[1].OrderID)
	}
	var got register.MoneyTotals
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		var err error
		got, err = register.OrderTotals(f, orderID)
		if err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if got.PaidPaise != 1200000 {
		t.Errorf("the two payments against the order total %d paise", got.PaidPaise)
	}
	reopenValid(t, e)
}

// TestCorrectingAnAutoOrderedEntryMakesNoSecondOrder guards the correction
// path: reopening an entry that already made an order must not make another.
func TestCorrectingAnAutoOrderedEntryMakesNoSecondOrder(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)

	form := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(form, 0, 0, productIDNamed(t, e, "Chairs"), "500", "rent")
	id := saveMoney(t, e, admin, key, form)
	orderID := movementByID(t, e, key, id).OrderID
	if orderID == "" {
		t.Fatal("the entry made no order")
	}

	status, edit := admin.get(t, "/finance/movements/"+id+"/edit")
	if status != 200 {
		t.Fatalf("the correction form = %d", status)
	}
	// It is corrected through the order's own tick boxes, and nothing is
	// pre-filled into the product lines, so a correction cannot agree the same
	// quantity twice.
	if !strings.Contains(edit, `name="lineIds-0"`) {
		t.Error("the correction form does not offer the order's products")
	}
	if !strings.Contains(edit, `name="qty-0-0" min="1" value=""`) {
		t.Error("the correction form pre-filled the product lines from the order")
	}
	// Typing into them anyway is refused rather than silently dropped.
	both := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	both.Set("orderId", orderID)
	moneyLine(both, 0, 0, productIDNamed(t, e, "Round tables"), "40", "purchase")
	if status, body := admin.post(t, "/finance/movements/"+id+"/edit", both); status != 200 ||
		!strings.Contains(body, "not both") {
		t.Errorf("an order plus typed products gave %d", status)
	}
	if all := orders(t, e, key); len(all) != 1 {
		t.Fatalf("the refused correction left %d orders", len(all))
	}

	fix := moneyForm("out", "5500", "Sharma Events", "Advance", "Cash")
	fix.Set("orderId", orderID)
	if status, body := admin.post(t, "/finance/movements/"+id+"/edit", fix); status != 303 {
		t.Fatalf("the correction = %d: %s", status, body)
	}
	if all := orders(t, e, key); len(all) != 1 {
		t.Fatalf("%d orders after the correction, want 1", len(all))
	}
	if m := movementByID(t, e, key, id); m.AmountPaise != 550000 || m.OrderID != orderID {
		t.Errorf("the corrected entry is %+v", m)
	}
	reopenValid(t, e)
}

// TestMoneyProductLineWorksWithoutJavaScript posts what a browser with script
// switched off sends: the hidden input arrives empty and the <noscript> select
// of the same name carries the answer.
func TestMoneyProductLineWorksWithoutJavaScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")

	status, page := admin.get(t, "/finance/movements/new")
	if status != 200 {
		t.Fatalf("the money form = %d", status)
	}
	noscript := page[strings.Index(page, "<noscript>"):]
	if !strings.Contains(noscript, `<select class="inp" name="product-0-0">`) {
		t.Fatal("the product line has no no-script fallback")
	}

	form := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(form, 0, 0, "", "500", "rent")
	// Hidden input first and empty, select second and chosen: the order the
	// browser submits them in.
	form["product-0-0"] = []string{"", chairs}
	if status, body := admin.post(t, "/finance/movements/new", form); status != 303 {
		t.Fatalf("the no-script entry = %d: %s", status, body)
	}

	all := orders(t, e, key)
	if len(all) != 1 || len(all[0].Lines) != 1 {
		t.Fatalf("the no-script entry made %d orders", len(all))
	}
	if all[0].Lines[0].ProductID != chairs || all[0].Lines[0].ExpectedQuantity != 500 {
		t.Errorf("the no-script line is %+v", all[0].Lines[0])
	}
	reopenValid(t, e)
}

// TestMoneyFormLoadsThePickerScript is the whole class of defect a Go test
// otherwise sails past: picker markup on a page that never loads picker.js is
// a box that does nothing at the desk.
func TestMoneyFormLoadsThePickerScript(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	status, page := admin.get(t, "/finance/movements/new")
	if status != 200 {
		t.Fatalf("the money form = %d", status)
	}
	if !strings.Contains(page, "data-picker ") {
		t.Fatal("the money form draws no product picker")
	}
	if !strings.Contains(page, `<script src="/static/picker.js"></script>`) {
		t.Error("the money form has picker boxes and never loads picker.js")
	}
	// And the file is really served, not merely named.
	if resp, body := e.get("/static/picker.js"); resp.StatusCode != 200 || !strings.Contains(body, "data-picker") {
		t.Errorf("/static/picker.js answered %d", resp.StatusCode)
	}
}

// TestSplitMoneyEntryMakesOneOrderPerRow is the shape the desk actually uses:
// one settlement typed as several amounts. Each amount agrees its own goods
// with its own party.
func TestSplitMoneyEntryMakesOneOrderPerRow(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")
	tables := productIDNamed(t, e, "Round tables")

	form := url.Values{
		"direction-0": {"out"}, "direction-1": {"out"},
		"amount":      {"5000", "2000"},
		"occurredAt":  {"2026-09-03T11:00", "2026-09-03T11:05"},
		"partyName":   {"Sharma Events", "Bala Transport"},
		"purposeName": {"Rent", "Freight"},
		"modeName":    {"Cash", "Cash"},
	}
	moneyLine(form, 0, 0, chairs, "500", "rent")
	moneyLine(form, 1, 0, tables, "40", "purchase")
	form.Set("agreedTotal-0", "12000")
	if status, body := admin.post(t, "/finance/movements/new", form); status != 303 {
		t.Fatalf("the split entry = %d: %s", status, body)
	}

	all := orders(t, e, key)
	if len(all) != 2 {
		t.Fatalf("%d orders for two amounts, want 2", len(all))
	}
	f := mustFinance(t, e, key)
	if register.FinanceValueText(f, all[0].PartyID) != "Sharma Events" ||
		register.FinanceValueText(f, all[1].PartyID) != "Bala Transport" {
		t.Errorf("the two orders are against %q and %q",
			register.FinanceValueText(f, all[0].PartyID), register.FinanceValueText(f, all[1].PartyID))
	}
	if all[0].ID == all[1].ID {
		t.Fatalf("both orders were numbered %s", all[0].ID)
	}
	// Line IDs are unique across the whole vault. Two orders in one save is
	// exactly where a shared counter goes wrong.
	if all[0].Lines[0].ID == all[1].Lines[0].ID {
		t.Errorf("both order lines were numbered %s", all[0].Lines[0].ID)
	}
	// The agreed total belongs to the row it was typed on and nowhere else.
	if all[0].AgreedPaise == nil || *all[0].AgreedPaise != 1200000 {
		t.Errorf("the first order's total is %v", all[0].AgreedPaise)
	}
	if all[1].AgreedPaise != nil {
		t.Errorf("the second order picked up a total of %d", *all[1].AgreedPaise)
	}

	moves := movements(t, e, key)
	if len(moves) != 2 || moves[0].OrderID != all[0].ID || moves[1].OrderID != all[1].ID {
		t.Errorf("the two amounts point at %q and %q", moves[0].OrderID, moves[1].OrderID)
	}
	reopenValid(t, e)
}

// TestAddingAndRemovingProductLinesKeepsWhatWasTyped: the buttons redraw the
// page, and a redraw that loses typed values is the defect the row buttons
// were already written to avoid.
func TestAddingAndRemovingProductLinesKeepsWhatWasTyped(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)
	chairs := productIDNamed(t, e, "Chairs")

	form := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(form, 0, 0, chairs, "500", "rent")
	form.Set("remarks", "Half now, half on delivery")
	form.Set("agreedTotal-0", "12000")
	form.Set("addLine", "0")

	status, page := admin.post(t, "/finance/movements/new", form)
	if status != 200 {
		t.Fatalf("adding a product line = %d", status)
	}
	for _, want := range []string{
		`name="product-0-1"`,
		`value="Chairs" data-picker-text`,
		`name="qty-0-0" min="1" value="500"`,
		`name="basis-0-0" value="rent" checked`,
		`name="agreedTotal-0" value="12000"`,
		"Half now, half on delivery",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the redrawn page lost %s", want)
		}
	}

	// Taking the second line off leaves the first one exactly as typed.
	form.Del("addLine")
	moneyLine(form, 0, 1, "", "", "")
	form.Set("removeLine", "0-1")
	status, page = admin.post(t, "/finance/movements/new", form)
	if status != 200 {
		t.Fatalf("removing a product line = %d", status)
	}
	if strings.Contains(page, `name="product-0-1"`) {
		t.Error("the line was not taken off")
	}
	if !strings.Contains(page, `value="Chairs" data-picker-text`) ||
		!strings.Contains(page, `name="qty-0-0" min="1" value="500"`) {
		t.Error("removing a line lost the line that was kept")
	}
}
