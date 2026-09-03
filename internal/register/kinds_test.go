package register

import (
	"testing"
	"time"
)

var kindAt = time.Date(2026, 9, 5, 9, 0, 0, 0, IST)

// kindInward is inward() with a typed kind on it.
func kindInward(id, productID string, quantity int, kindID, supplier, receivedOn string) Inward {
	in := inward(id, productID, quantity, Other, supplier, receivedOn)
	in.KindID = kindID
	return in
}

// withKinds adds the named words to a register and hands back their IDs.
func withKinds(t *testing.T, r *Register, words ...string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, w := range words {
		id, err := AddAcquisitionKind(r, w, "Suresh Kumar", kindAt)
		if err != nil {
			t.Fatalf("adding %q: %v", w, err)
		}
		ids[w] = id
	}
	return ids
}

func TestAcquisitionKindsAreAddedOnceAndNumbered(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated", "Sponsored")
	if ids["Donated"] != "AKD-0001" || ids["Sponsored"] != "AKD-0002" {
		t.Fatalf("ids = %v", ids)
	}
	// The same word again, in different letters, is the same row.
	again, err := AddAcquisitionKind(r, "donated", "Anita Rao", kindAt)
	if err != nil || again != ids["Donated"] {
		t.Fatalf("AddAcquisitionKind(donated) = %q, %v", again, err)
	}
	if len(r.AcquisitionKinds) != 2 {
		t.Fatalf("the list holds %d words", len(r.AcquisitionKinds))
	}
	if _, err := AddAcquisitionKind(r, "   ", "Anita Rao", kindAt); err == nil {
		t.Error("a blank word was accepted")
	}
}

func TestBasisWordSaysWhatCameIn(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated")
	// A merged word shows where it now points; a missing one falls back to
	// the plain word rather than stopping anything.
	merged, _ := AddAcquisitionKind(r, "Donatd", "Suresh Kumar", kindAt)
	for i := range r.AcquisitionKinds {
		if r.AcquisitionKinds[i].ID == merged {
			r.AcquisitionKinds[i].MergedIntoID = ids["Donated"]
		}
	}
	cases := []struct {
		name   string
		basis  Basis
		kindID string
		want   string
	}{
		{"rent", Rent, "", "Rent"},
		{"purchase", Purchase, "", "Purchase"},
		{"typed word", Other, ids["Donated"], "Donated"},
		{"a merged word follows the merge", Other, merged, "Donated"},
		{"a word that is gone", Other, "AKD-9999", "Other"},
		{"other with no word at all", Other, "", "Other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := BasisWord(r, c.basis, c.kindID); got != c.want {
				t.Errorf("BasisWord = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAcquisitionKindIsUsed(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated", "Sponsored", "Borrowed", "Lent")
	r.Inwards = append(r.Inwards, kindInward("INW-0001", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"))
	// A deleted delivery still counts: its history has to keep its word.
	deleted := kindInward("INW-0002", "PRD-0002", 10, ids["Sponsored"], "Gupta Traders", "2026-09-01")
	deleted.Deleted = &Deletion{At: kindAt, By: "Suresh Kumar", Reason: "typed twice"}
	r.Inwards = append(r.Inwards, deleted)
	for i := range r.AcquisitionKinds {
		if r.AcquisitionKinds[i].Name == "Lent" {
			r.AcquisitionKinds[i].MergedIntoID = ids["Borrowed"]
		}
	}
	f := financeSeed()
	f.Orders = append(f.Orders, FinanceOrder{
		ID: "ORD-0001", PartyID: "PTY-0001", OrderedAt: kindAt, Status: "open",
		CreatedAt: kindAt, CreatedByID: "FAC-0001",
		Lines: []FinanceOrderLine{{
			ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Tents",
			ExpectedQuantity: 5, Basis: Other, KindID: ids["Sponsored"],
		}},
	})

	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"a delivery uses it", ids["Donated"], true},
		{"an order line uses it", ids["Sponsored"], true},
		{"another word is merged into it", ids["Borrowed"], true},
		{"the merged word itself", ids["Lent"], false},
		{"nothing at all", "AKD-9999", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AcquisitionKindIsUsed(r, f, c.id); got != c.want {
				t.Errorf("AcquisitionKindIsUsed = %v, want %v", got, c.want)
			}
			// The desk has no vault open and must reach the same answer
			// about the deliveries it can see.
			if c.id == ids["Donated"] && !AcquisitionKindIsUsed(r, nil, c.id) {
				t.Error("with no vault open a used word reads as unused")
			}
		})
	}
}

func TestStockRowShowsTheTypedWord(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated")
	cases := []struct {
		name    string
		inwards []Inward
		want    string
	}{
		{"donated only", []Inward{
			kindInward("INW-0001", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
		}, "Donated"},
		{"donated and bought", []Inward{
			kindInward("INW-0001", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
			inward("INW-0002", "PRD-0001", 20, Purchase, "Gupta Traders", "2026-09-02"),
		}, "Donated"},
		{"rent wins, because those goods are owed", []Inward{
			kindInward("INW-0001", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
			inward("INW-0002", "PRD-0001", 20, Rent, "Sharma Events", "2026-09-02"),
		}, "Rent"},
		{"bought only", []Inward{
			inward("INW-0001", "PRD-0001", 20, Purchase, "Gupta Traders", "2026-09-02"),
		}, "Purchase"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reg := stockFor(c.inwards...)
			reg.AcquisitionKinds = r.AcquisitionKinds
			var row StockRow
			for _, got := range StockRows(reg) {
				if got.ProductID == "PRD-0001" {
					row = got
				}
			}
			if got := BasisWord(reg, row.Basis, row.KindID); got != c.want {
				t.Errorf("the stock pill reads %q, want %q", got, c.want)
			}
		})
	}
}

func TestSuppliersGroupTypedKindsOnTheirOwnRows(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated", "Sponsored")
	r.Inwards = []Inward{
		inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"),
		kindInward("INW-0002", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
		kindInward("INW-0003", "PRD-0001", 30, ids["Sponsored"], "Sharma Events", "2026-09-01"),
		kindInward("INW-0004", "PRD-0001", 7, ids["Donated"], "Sharma Events", "2026-09-02"),
		inward("INW-0005", "PRD-0001", 20, Purchase, "Gupta Traders", "2026-09-02"),
	}
	rows := SupplierRows(r)
	type got struct {
		word     string
		quantity int
	}
	var seen []got
	for _, row := range rows {
		seen = append(seen, got{BasisWord(r, row.Basis, row.KindID), row.CameIn})
	}
	want := []got{{"Rent", 100}, {"Donated", 57}, {"Sponsored", 30}, {"Purchase", 20}}
	if len(seen) != len(want) {
		t.Fatalf("the suppliers view has %d rows: %v", len(seen), seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("row %d is %v, want %v", i, seen[i], want[i])
		}
	}
}

func TestTypedKindsGoOutOfBothDoors(t *testing.T) {
	ids := map[string]string{}
	build := func(t *testing.T) (*Register, *FinanceData, string) {
		t.Helper()
		r := stockFor()
		ids = withKinds(t, r, "Donated")
		r.Inwards = []Inward{
			inward("INW-0001", "PRD-0001", 40, Rent, "Sharma Events", "2026-09-01"),
			inward("INW-0002", "PRD-0002", 25, Purchase, "Gupta Traders", "2026-09-01"),
			kindInward("INW-0003", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
		}
		f, parties := partiesFor(r, "Sharma Events", "Gupta Traders")
		return r, f, parties["Sharma Events"]
	}

	t.Run("rent is offered on returns only", func(t *testing.T) {
		r, f, _ := build(t)
		r.Inwards = r.Inwards[:2]
		if got := SupplierReturnAvailable(r, f, "", "PRD-0001"); got != 40 {
			t.Errorf("%d rented tents may go back, want 40", got)
		}
		if got := PurchasedAvailableToSell(r, f, "PRD-0001"); got != 0 {
			t.Errorf("%d rented tents may be sold, want 0", got)
		}
	})

	t.Run("purchase is offered on sales only", func(t *testing.T) {
		r, f, _ := build(t)
		if got := PurchasedAvailableToSell(r, f, "PRD-0002"); got != 25 {
			t.Errorf("%d bought chairs may be sold, want 25", got)
		}
		if got := SupplierReturnAvailable(r, f, "", "PRD-0002"); got != 0 {
			t.Errorf("%d bought chairs may go back, want 0", got)
		}
	})

	t.Run("a typed kind is offered on both", func(t *testing.T) {
		r, f, _ := build(t)
		if got := SupplierReturnAvailable(r, f, "", "PRD-0001"); got != 90 {
			t.Errorf("%d tents may go back, want 90 — 40 rented and 50 donated", got)
		}
		if got := PurchasedAvailableToSell(r, f, "PRD-0001"); got != 50 {
			t.Errorf("%d tents may be sold, want the 50 donated ones", got)
		}
	})

	t.Run("both doors draw on one pool", func(t *testing.T) {
		r, f, party := build(t)
		disposeSale(t, r, f, party, "PRD-0001", 30, settleAt)
		if got := PurchasedAvailableToSell(r, f, "PRD-0001"); got != 20 {
			t.Errorf("%d donated tents may still be sold, want 20", got)
		}
		if got := SupplierReturnAvailable(r, f, "", "PRD-0001"); got != 60 {
			t.Errorf("%d tents may still go back, want 60 — the 30 sold are gone", got)
		}
	})
}

func TestObligationsStayWithRentOnly(t *testing.T) {
	r := stockFor()
	ids := withKinds(t, r, "Donated")
	r.Inwards = []Inward{
		inward("INW-0001", "PRD-0001", 40, Rent, "Sharma Events", "2026-09-01"),
		kindInward("INW-0002", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
	}
	f, _ := partiesFor(r, "Sharma Events")
	rows := SupplierObligations(r, f)
	if len(rows) != 1 || rows[0].Received != 40 {
		t.Fatalf("obligations = %+v, want the 40 rented tents only", rows)
	}
}

// TestReturnSettlesWhatIsOwedFirst is the number a supplier reads. Goods that
// went back must come off what is owed, and goods that were never owed must
// not.
func TestReturnSettlesWhatIsOwedFirst(t *testing.T) {
	build := func(t *testing.T) (*Register, *FinanceData, string) {
		t.Helper()
		r := stockFor()
		ids := withKinds(t, r, "Donated")
		r.Inwards = []Inward{
			inward("INW-0001", "PRD-0001", 40, Rent, "Sharma Events", "2026-09-01"),
			kindInward("INW-0002", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
		}
		f, parties := partiesFor(r, "Sharma Events")
		return r, f, parties["Sharma Events"]
	}

	t.Run("sending back what was rented clears the debt", func(t *testing.T) {
		r, f, party := build(t)
		disposeSupplierReturn(t, r, f, party, "PRD-0001", 40, settleAt)
		rows := SupplierObligations(r, f)
		if len(rows) != 1 || rows[0].Returned != 40 || rows[0].Remaining != 0 {
			t.Fatalf("after sending back 40 the supplier row is %+v", rows)
		}
	})

	t.Run("the older donated delivery does not get in the way", func(t *testing.T) {
		// The donated goods arrived first, so oldest-first alone would send
		// them back and leave the rented ones sitting as a debt.
		r := stockFor()
		ids := withKinds(t, r, "Donated")
		r.Inwards = []Inward{
			kindInward("INW-0001", "PRD-0001", 50, ids["Donated"], "Sharma Events", "2026-09-01"),
			inward("INW-0002", "PRD-0001", 40, Rent, "Sharma Events", "2026-09-02"),
		}
		f, parties := partiesFor(r, "Sharma Events")
		disposeSupplierReturn(t, r, f, parties["Sharma Events"], "PRD-0001", 40, settleAt)
		rows := SupplierObligations(r, f)
		if len(rows) != 1 || rows[0].Returned != 40 || rows[0].Remaining != 0 {
			t.Fatalf("after sending back 40 the supplier row is %+v", rows)
		}
	})

	t.Run("sending back what was given owes nothing either way", func(t *testing.T) {
		r, f, party := build(t)
		// Everything rented has already gone back, so this return can only
		// draw on the donated stock.
		disposeSupplierReturn(t, r, f, party, "PRD-0001", 40, settleAt)
		disposeSupplierReturn(t, r, f, party, "PRD-0001", 30, settleAt)
		rows := SupplierObligations(r, f)
		if len(rows) != 1 || rows[0].Returned != 40 || rows[0].Remaining != 0 {
			t.Fatalf("the donated return moved the debt: %+v", rows)
		}
	})
}

func TestOrderLineWithATypedKindPassesValidation(t *testing.T) {
	f := financeSeed()
	party, err := AddFinanceValue(f, FinanceParty, "Sharma Events", "FAC-0001", kindAt)
	if err != nil {
		t.Fatal(err)
	}
	f.Orders = append(f.Orders, FinanceOrder{
		ID: "ORD-0001", PartyID: party, OrderedAt: kindAt, Status: "open",
		CreatedAt: kindAt, CreatedByID: "FAC-0001",
		Lines: []FinanceOrderLine{
			{ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 5, Basis: Other, KindID: "AKD-0001"},
			// The same product under a different word is a different line.
			{ID: "OLN-0002", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 3, Basis: Other, KindID: "AKD-0002"},
			{ID: "OLN-0003", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 2, Basis: Rent},
		},
	})
	if err := ValidateFinance(f); err != nil {
		t.Fatalf("a typed kind stopped the vault loading: %v", err)
	}
}
