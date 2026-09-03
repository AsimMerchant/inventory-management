package register

import (
	"testing"
	"time"
)

func TestValidatePartyReferencesCoversInventoryAndFinance(t *testing.T) {
	build := func() (*Register, *FinanceData) {
		r := &Register{SchemaVersion: SchemaVersion}
		id, err := AddParty(r, "Sharma Events")
		if err != nil {
			t.Fatal(err)
		}
		r.Inwards = []Inward{{ID: "INW-0001", PartyID: id, Supplier: "Sharma Events"}}
		f := financeSeed()
		f.Orders = []FinanceOrder{{ID: "ORD-0001", PartyID: id}}
		f.Movements = []MoneyMovement{{ID: "MOV-0001", PartyID: id}}
		f.SupplierReturns = []SupplierReturn{{ID: "SRN-0001", PartyID: id}}
		f.Sales = []StockSale{{ID: "SAL-0001", BuyerPartyID: id}}
		return r, f
	}

	if r, f := build(); ValidatePartyReferences(r, f) != nil {
		t.Fatalf("valid shared references were refused: %v", ValidatePartyReferences(r, f))
	}
	for name, breakIt := range map[string]func(*Register, *FinanceData){
		"delivery":        func(r *Register, _ *FinanceData) { r.Inwards[0].PartyID = "PRT-9999" },
		"order":           func(_ *Register, f *FinanceData) { f.Orders[0].PartyID = "PRT-9999" },
		"money":           func(_ *Register, f *FinanceData) { f.Movements[0].PartyID = "PUR-0001" },
		"supplier return": func(_ *Register, f *FinanceData) { f.SupplierReturns[0].PartyID = "" },
		"sale":            func(_ *Register, f *FinanceData) { f.Sales[0].BuyerPartyID = "PRT-9999" },
	} {
		r, f := build()
		breakIt(r, f)
		if err := ValidatePartyReferences(r, f); err == nil {
			t.Errorf("unknown %s party was accepted", name)
		}
	}
}

func TestLinkInwardPartiesKeepsDistinctSpellingsUntilHumanMerge(t *testing.T) {
	at := time.Date(2026, 9, 3, 12, 0, 0, 0, IST)
	r := &Register{Inwards: []Inward{
		{ID: "INW-0001", Supplier: "Sharma Events", RecordedBy: "Suresh", RecordedAt: at},
		{ID: "INW-0002", Supplier: " sharma events ", RecordedBy: "Suresh", RecordedAt: at},
		{ID: "INW-0003", Supplier: "Sharma Tents", RecordedBy: "Suresh", RecordedAt: at},
	}}
	LinkInwardParties(r)
	if len(r.Parties) != 2 {
		t.Fatalf("migration made %d parties, want two distinct spellings", len(r.Parties))
	}
	if r.Inwards[0].PartyID != r.Inwards[1].PartyID || r.Inwards[2].PartyID == r.Inwards[0].PartyID {
		t.Fatalf("delivery links are %+v", r.Inwards)
	}
}
