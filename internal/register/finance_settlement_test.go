package register

import (
	"strings"
	"testing"
	"time"
)

var settleAt = time.Date(2026, 9, 5, 10, 0, 0, 0, IST)

// stockFor builds a register holding just the inwards a settlement test needs.
func stockFor(inwards ...Inward) *Register {
	r := &Register{
		SchemaVersion: SchemaVersion,
		Products: []Product{
			{ID: "PRD-0001", Name: "Tents", CreatedAt: settleAt, CreatedBy: "Suresh Kumar"},
			{ID: "PRD-0002", Name: "Chairs", CreatedAt: settleAt, CreatedBy: "Suresh Kumar"},
		},
		Staff:     []Staff{{ID: "STF-0001", Name: "Suresh Kumar", Mobile: "98450 22117", CreatedAt: settleAt}},
		Inwards:   []Inward{},
		Issues:    []Issue{},
		Returns:   []Return{},
		Disposals: []InventoryDisposal{},
	}
	r.Inwards = append(r.Inwards, inwards...)
	return r
}

func inward(id, productID string, quantity int, basis Basis, supplier, receivedOn string) Inward {
	return Inward{
		ID: id, ProductID: productID, Quantity: quantity, Basis: basis,
		Supplier: supplier, ReceivedOn: receivedOn,
		ReceivedBy: "Suresh Kumar", RecordedBy: "Suresh Kumar",
		RecordedAt: settleAt,
	}
}

// partiesFor makes the finance side with one party per name given.
func partiesFor(names ...string) (*FinanceData, map[string]string) {
	f := financeSeed()
	ids := map[string]string{}
	for _, n := range names {
		id, err := AddFinanceValue(f, FinanceParty, n, "FAC-0001", settleAt)
		if err != nil {
			panic(err)
		}
		ids[n] = id
	}
	return f, ids
}

// disposeSupplierReturn records a return the way the store will, so the test
// exercises the same pairing the real write has to satisfy.
func disposeSupplierReturn(t *testing.T, r *Register, f *FinanceData, partyID, productID string, quantity int, at time.Time) string {
	t.Helper()
	sources, err := AllocateSupplierReturn(r, f, partyID, productID, quantity)
	if err != nil {
		t.Fatalf("allocating %d: %v", quantity, err)
	}
	p, _ := ProductByID(r, productID)
	d := InventoryDisposal{
		ID: r.NextID("DSP"), ProductID: productID, Quantity: quantity,
		Sources: sources, RecordedAt: at,
	}
	r.Disposals = append(r.Disposals, d)
	id := f.NextID("SRN")
	f.SupplierReturns = append(f.SupplierReturns, SupplierReturn{
		ID: id, DisposalID: d.ID, PartyID: partyID,
		Product: FinanceProductRef{ProductID: productID, ProductName: p.Name},
		Sources: sources, ReturnedAt: at, RecordedAt: at, RecordedByID: "FAC-0001",
	})
	if err := ValidatePairing(r, f); err != nil {
		t.Fatalf("the return did not pair: %v", err)
	}
	return id
}

func disposeSale(t *testing.T, r *Register, f *FinanceData, partyID, productID string, quantity int, at time.Time) string {
	t.Helper()
	sources, err := AllocateStockSale(r, f, productID, quantity)
	if err != nil {
		t.Fatalf("allocating a sale of %d: %v", quantity, err)
	}
	p, _ := ProductByID(r, productID)
	d := InventoryDisposal{
		ID: r.NextID("DSP"), ProductID: productID, Quantity: quantity,
		Sources: sources, RecordedAt: at,
	}
	r.Disposals = append(r.Disposals, d)
	id := f.NextID("SAL")
	f.Sales = append(f.Sales, StockSale{
		ID: id, DisposalID: d.ID, BuyerPartyID: partyID,
		Product: FinanceProductRef{ProductID: productID, ProductName: p.Name},
		Sources: sources, SoldAt: at, RecordedAt: at, RecordedByID: "FAC-0001",
	})
	if err := ValidatePairing(r, f); err != nil {
		t.Fatalf("the sale did not pair: %v", err)
	}
	return id
}

func TestOnHandSubtractsPublicDisposalsWithoutFinanceUnlock(t *testing.T) {
	r := stockFor(
		inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"),
		inward("INW-0002", "PRD-0002", 60, Purchase, "Gupta Traders", "2026-09-01"),
	)
	f, ids := partiesFor("Sharma Events", "Gupta Traders")

	if got := OnHand(r, "PRD-0001"); got != 100 {
		t.Fatalf("before any disposal Tents are %d", got)
	}
	disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 30, settleAt)
	disposeSale(t, r, f, ids["Gupta Traders"], "PRD-0002", 20, settleAt)

	// The public register alone knows enough to be right about stock.
	if got := OnHand(r, "PRD-0001"); got != 70 {
		t.Errorf("Tents on hand is %d, want 70", got)
	}
	if got := OnHand(r, "PRD-0002"); got != 40 {
		t.Errorf("Chairs on hand is %d, want 40", got)
	}
	if got := Disposed(r, "PRD-0001"); got != 30 {
		t.Errorf("Tents disposed is %d", got)
	}
	if problems := Validate(r); len(problems) != 0 {
		t.Errorf("the register is invalid: %v", problems)
	}

	// And the public record says nothing about which kind of exit it was.
	for _, d := range r.Disposals {
		if d.ProductID == "" || d.Quantity == 0 || len(d.Sources) == 0 {
			t.Errorf("a disposal is incomplete: %+v", d)
		}
	}
}

func TestSupplierReturnLimitedByOnHandAndSupplierRentalReceipts(t *testing.T) {
	r := stockFor(
		inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"),
		inward("INW-0002", "PRD-0001", 40, Rent, "Gupta Traders", "2026-09-02"),
	)
	// 80 went out with people and never came back, so only 60 are here.
	r.Issues = append(r.Issues, Issue{
		ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 80,
		TakerName: "Ravi Menon", TakerDepartment: "Setup", TakerMobile: "98861 40023",
		IssuedAt: settleAt, PersonInchargeName: "Suresh Kumar",
		PersonInchargeMobile: "98450 22117", RecordedAt: settleAt,
	})
	f, ids := partiesFor("Sharma Events", "Gupta Traders")

	if got := OnHand(r, "PRD-0001"); got != 60 {
		t.Fatalf("on hand is %d, want 60", got)
	}
	// Sharma sent 100 but only 60 are in the store, so 60 is the limit.
	if got := SupplierReturnAvailable(r, f, ids["Sharma Events"], "PRD-0001"); got != 60 {
		t.Fatalf("Sharma may return %d, want 60", got)
	}

	disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 50, settleAt)

	// 10 left in the store. Sharma still has 50 unreturned but the store cap
	// bites; Gupta has 40 unreturned and the same 10 cap.
	if got := OnHand(r, "PRD-0001"); got != 10 {
		t.Fatalf("after returning 50, on hand is %d", got)
	}
	if got := SupplierReturnAvailable(r, f, ids["Sharma Events"], "PRD-0001"); got != 10 {
		t.Errorf("Sharma may now return %d, want 10", got)
	}
	if got := SupplierReturnAvailable(r, f, ids["Gupta Traders"], "PRD-0001"); got != 10 {
		t.Errorf("Gupta may return %d, want 10", got)
	}
	// The same physical stock is not offered twice: returning ten to one
	// leaves nothing for the other.
	disposeSupplierReturn(t, r, f, ids["Gupta Traders"], "PRD-0001", 10, settleAt)
	if got := SupplierReturnAvailable(r, f, ids["Sharma Events"], "PRD-0001"); got != 0 {
		t.Errorf("after Gupta took the last ten, Sharma may return %d", got)
	}
	if _, err := AllocateSupplierReturn(r, f, ids["Sharma Events"], "PRD-0001", 1); err == nil {
		t.Error("a return beyond the cap was allowed")
	}
}

func TestSupplierReturnAllocatesOldestEligibleInwards(t *testing.T) {
	r := stockFor(
		inward("INW-0001", "PRD-0001", 70, Rent, "Sharma Events", "2026-09-01"),
		inward("INW-0002", "PRD-0001", 30, Rent, "Sharma Events", "2026-09-03"),
		inward("INW-0003", "PRD-0001", 50, Purchase, "Sharma Events", "2026-09-01"),
		inward("INW-0004", "PRD-0001", 40, Rent, "", "2026-09-01"),
		inward("INW-0005", "PRD-0001", 25, Rent, "Gupta Traders", "2026-09-01"),
	)
	f, ids := partiesFor("Sharma Events", "Gupta Traders")

	sources, err := AllocateSupplierReturn(r, f, ids["Sharma Events"], "PRD-0001", 90)
	if err != nil {
		t.Fatal(err)
	}
	// Oldest of Sharma's own rented receipts first: 70 then 20.
	if len(sources) != 2 ||
		sources[0] != (DisposalAllocation{InwardID: "INW-0001", Quantity: 70}) ||
		sources[1] != (DisposalAllocation{InwardID: "INW-0002", Quantity: 20}) {
		t.Fatalf("allocated %+v", sources)
	}
	// The purchase, the blank supplier and Gupta's rows are untouched.
	for _, a := range sources {
		if a.InwardID == "INW-0003" || a.InwardID == "INW-0004" || a.InwardID == "INW-0005" {
			t.Errorf("allocation reached %s", a.InwardID)
		}
	}

	// Correcting the party's spelling keeps the old inwards attributed to it.
	for i := range f.ReusableValues {
		if f.ReusableValues[i].ID == ids["Sharma Events"] {
			f.ReusableValues[i].Changes = append(f.ReusableValues[i].Changes, FinanceChange{
				At: settleAt, ByAccountID: "FAC-0001", ByName: "Asha Patel", ByMobile: "9820011111",
				Field: "value", Label: "Party", From: "Sharma Events", To: "Sharma Tent House",
			})
			f.ReusableValues[i].Value = "Sharma Tent House"
		}
	}
	after, err := AllocateSupplierReturn(r, f, ids["Sharma Events"], "PRD-0001", 90)
	if err != nil {
		t.Fatalf("after the rename the return was refused: %v", err)
	}
	if len(after) != 2 || after[0].InwardID != "INW-0001" {
		t.Errorf("after the rename the sources are %+v", after)
	}

	// A merged-away spelling still resolves to the surviving party.
	old, err := AddFinanceValue(f, FinanceParty, "Sharma Tents", "FAC-0001", settleAt)
	if err != nil {
		t.Fatal(err)
	}
	for i := range f.ReusableValues {
		if f.ReusableValues[i].ID == old {
			f.ReusableValues[i].MergedIntoID = ids["Sharma Events"]
		}
	}
	if got := SupplierReturnAvailable(r, f, old, "PRD-0001"); got != 100 {
		t.Errorf("through the merged spelling the available amount is %d, want 100", got)
	}
}

func TestSaleLimitedByOnHandAndPurchasedReceipts(t *testing.T) {
	r := stockFor(
		inward("INW-0001", "PRD-0002", 100, Purchase, "Gupta Traders", "2026-09-01"),
		inward("INW-0002", "PRD-0002", 50, Rent, "Sharma Events", "2026-09-01"),
	)
	// 90 are out with people, so 60 are in the store.
	r.Issues = append(r.Issues, Issue{
		ID: "ISS-0001", ProductID: "PRD-0002", Quantity: 90,
		TakerName: "Ravi Menon", TakerDepartment: "Setup", TakerMobile: "98861 40023",
		IssuedAt: settleAt, PersonInchargeName: "Suresh Kumar",
		PersonInchargeMobile: "98450 22117", RecordedAt: settleAt,
	})
	f, ids := partiesFor("Patel Decorators")

	if got := OnHand(r, "PRD-0002"); got != 60 {
		t.Fatalf("on hand is %d, want 60", got)
	}
	// 100 were bought but only 60 are here.
	if got := PurchasedAvailableToSell(r, f, "PRD-0002"); got != 60 {
		t.Fatalf("may sell %d, want 60", got)
	}

	disposeSale(t, r, f, ids["Patel Decorators"], "PRD-0002", 40, settleAt)

	// 60 purchased remain unsold, but only 20 are physically here.
	if got := OnHand(r, "PRD-0002"); got != 20 {
		t.Fatalf("after selling 40, on hand is %d", got)
	}
	if got := PurchasedAvailableToSell(r, f, "PRD-0002"); got != 20 {
		t.Errorf("may now sell %d, want 20", got)
	}
	// A sale never reaches the rented receipts.
	sources, err := AllocateStockSale(r, f, "PRD-0002", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range sources {
		if a.InwardID == "INW-0002" {
			t.Error("a sale allocated a rented receipt")
		}
	}
}

func TestSupplierObligationUsesActualReceiptsNotOrderOrMoney(t *testing.T) {
	r := stockFor()
	f, ids := partiesFor("Sharma Events")

	// An order and an advance payment change nothing: this counts goods.
	f.Orders = append(f.Orders, FinanceOrder{
		ID: "ORD-0001", PartyID: ids["Sharma Events"], OrderedAt: settleAt, Status: "open",
		CreatedAt: settleAt, CreatedByID: "FAC-0001",
		Lines: []FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001",
			ProductNameSnapshot: "Tents", ExpectedQuantity: 100, Basis: Rent}},
	})
	advance := money(f, "MOV-0001", MoneyOut, 500000, settleAt)
	advance.OrderID = "ORD-0001"
	f.Movements = append(f.Movements, advance)
	if rows := SupplierObligations(r, f); len(rows) != 0 {
		t.Fatalf("an order created obligations: %+v", rows)
	}

	// 70 actually arrive.
	r.Inwards = append(r.Inwards, inward("INW-0001", "PRD-0001", 70, Rent, "Sharma Events", "2026-09-02"))
	rows := SupplierObligations(r, f)
	if len(rows) != 1 || rows[0].Received != 70 || rows[0].Returned != 0 || rows[0].Remaining != 70 {
		t.Fatalf("after receiving 70 the obligation is %+v", rows)
	}
	if rows[0].PartyID != ids["Sharma Events"] || rows[0].ProductName != "Tents" {
		t.Errorf("the obligation names %+v", rows[0])
	}

	// 20 go back.
	disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 20, settleAt)
	rows = SupplierObligations(r, f)
	if len(rows) != 1 || rows[0].Received != 70 || rows[0].Returned != 20 || rows[0].Remaining != 50 {
		t.Fatalf("after returning 20 the obligation is %+v", rows)
	}

	// A deposit coming back is money, not goods.
	refund := money(f, "MOV-0002", MoneyIn, 500000, settleAt)
	f.Movements = append(f.Movements, refund)
	if again := SupplierObligations(r, f); again[0].Remaining != 50 {
		t.Errorf("a refund changed the obligation to %d", again[0].Remaining)
	}

	// A supplier nobody has added to the protected list still gets a row.
	r.Inwards = append(r.Inwards, inward("INW-0002", "PRD-0002", 15, Rent, "Verma Sound", "2026-09-02"))
	rows = SupplierObligations(r, f)
	found := false
	for _, row := range rows {
		if row.PartyName == "Verma Sound" {
			found = true
			if row.PartyID != "" {
				t.Errorf("an unknown supplier was given the party id %s", row.PartyID)
			}
		}
	}
	if !found {
		t.Error("a supplier with no finance party got no row")
	}
}

func TestValidatePairingRefusesEveryOrphanAndMismatch(t *testing.T) {
	r := stockFor(inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"))
	f, ids := partiesFor("Sharma Events")
	disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 30, settleAt)
	if err := ValidatePairing(r, f); err != nil {
		t.Fatalf("a sound pairing was refused: %v", err)
	}

	for name, breakIt := range map[string]func(*Register, *FinanceData){
		"quantity differs": func(r *Register, _ *FinanceData) { r.Disposals[0].Quantity = 29 },
		"product differs":  func(r *Register, _ *FinanceData) { r.Disposals[0].ProductID = "PRD-0002" },
		"sources differ":   func(r *Register, _ *FinanceData) { r.Disposals[0].Sources[0].Quantity = 29 },
		"time differs": func(r *Register, _ *FinanceData) {
			r.Disposals[0].RecordedAt = settleAt.Add(time.Minute)
		},
		"live settlement with an inactive removal": func(r *Register, _ *FinanceData) {
			at := settleAt
			r.Disposals[0].InactiveAt = &at
		},
		"voided settlement with an active removal": func(_ *Register, f *FinanceData) {
			f.SupplierReturns[0].Voided = &FinanceVoid{At: settleAt, ByAccountID: "FAC-0001",
				ByName: "Asha Patel", ByMobile: "9820011111", Reason: "Wrong supplier"}
		},
		"settlement naming no removal": func(_ *Register, f *FinanceData) {
			f.SupplierReturns[0].DisposalID = ""
		},
		"settlement naming an unknown removal": func(_ *Register, f *FinanceData) {
			f.SupplierReturns[0].DisposalID = "DSP-9999"
		},
		"an orphan removal": func(r *Register, _ *FinanceData) {
			r.Disposals = append(r.Disposals, InventoryDisposal{
				ID: "DSP-0009", ProductID: "PRD-0001", Quantity: 5,
				Sources: []DisposalAllocation{{InwardID: "INW-0001", Quantity: 5}}, RecordedAt: settleAt,
			})
		},
		"two settlements on one removal": func(_ *Register, f *FinanceData) {
			second := f.SupplierReturns[0]
			second.ID = "SRN-0002"
			f.SupplierReturns = append(f.SupplierReturns, second)
		},
	} {
		r := stockFor(inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"))
		f, ids := partiesFor("Sharma Events")
		disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 30, settleAt)
		breakIt(r, f)
		if err := ValidatePairing(r, f); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestSettlementRowsAreNewestFirstWhicheverKind(t *testing.T) {
	r := stockFor(
		inward("INW-0001", "PRD-0001", 100, Rent, "Sharma Events", "2026-09-01"),
		inward("INW-0002", "PRD-0002", 100, Purchase, "Gupta Traders", "2026-09-01"),
	)
	f, ids := partiesFor("Sharma Events", "Patel Decorators")
	disposeSupplierReturn(t, r, f, ids["Sharma Events"], "PRD-0001", 10, settleAt)
	disposeSale(t, r, f, ids["Patel Decorators"], "PRD-0002", 20, settleAt.Add(time.Hour))

	rows := SettlementRows(r, f)
	if len(rows) != 2 {
		t.Fatalf("%d settlement rows", len(rows))
	}
	var kinds []string
	for _, row := range rows {
		kinds = append(kinds, row.Kind)
	}
	if strings.Join(kinds, ",") != "sale,supplier_return" {
		t.Errorf("the order is %v", kinds)
	}
	if rows[0].Quantity != 20 || rows[1].Quantity != 10 {
		t.Errorf("quantities are %d and %d", rows[0].Quantity, rows[1].Quantity)
	}
}
