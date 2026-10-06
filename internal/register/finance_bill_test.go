package register

import (
	"errors"
	"testing"
	"time"
)

func validBillFixture(t *testing.T) (*Register, *FinanceData, FinanceBill) {
	t.Helper()
	at := time.Date(2026, 9, 3, 12, 40, 0, 0, IST)
	r := &Register{SchemaVersion: SchemaVersion}
	partyID, err := AddParty(r, "Sharma Tent House")
	if err != nil {
		t.Fatal(err)
	}
	challanID, err := AddSupplierChallan(r, partyID, "STH/4390")
	if err != nil {
		t.Fatal(err)
	}
	f := financeSeed()
	purposeID, err := AddFinanceValue(f, FinancePurpose, "Rent", "FAC-0001", at)
	if err != nil {
		t.Fatal(err)
	}
	f.Orders = append(f.Orders, FinanceOrder{
		ID: "ORD-0001", PartyID: partyID, OrderedAt: at, Status: "open", CreatedAt: at, CreatedByID: "FAC-0001",
		Lines: []FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Chairs", ExpectedQuantity: 500, Basis: Rent}},
	})
	b := FinanceBill{
		ID: "BIL-0001", AmountPaise: 2000000, OccurredAt: at, PartyID: partyID,
		OrderID: "ORD-0001", Products: []FinanceProductRef{{ProductID: "PRD-0001", ProductName: "Chairs"}},
		PurposeID: purposeID, SupplierChallanID: challanID, RecordedAt: at, RecordedByID: "FAC-0001",
	}
	return r, f, b
}

func TestHistoricalAgreementNeverBecomesBill(t *testing.T) {
	f := financeSeed()
	estimated, exact := int64(2500000), int64(3100000)
	at := time.Date(2026, 9, 3, 12, 40, 0, 0, IST)
	f.Orders = []FinanceOrder{
		{ID: "ORD-0001", PartyID: "PRT-0001", Lines: []FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 50, Basis: Rent}}, AgreedPaise: &estimated, AgreedKind: "estimated", OrderedAt: at, Status: "open", CreatedAt: at, CreatedByID: "FAC-0001"},
		{ID: "ORD-0002", PartyID: "PRT-0001", Lines: []FinanceOrderLine{{ID: "OLN-0002", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 50, Basis: Rent}}, AgreedPaise: &exact, AgreedKind: "exact", OrderedAt: at, Status: "open", CreatedAt: at, CreatedByID: "FAC-0001"},
	}
	if len(f.Bills) != 0 {
		t.Fatalf("agreements created %d bills", len(f.Bills))
	}
	got, err := TotalBills(f, nil)
	if err != nil || got != 0 {
		t.Fatalf("TotalBills = %d, %v; want 0", got, err)
	}
	if *f.Orders[0].AgreedPaise != estimated || f.Orders[0].AgreedKind != "estimated" || *f.Orders[1].AgreedPaise != exact || f.Orders[1].AgreedKind != "exact" {
		t.Fatal("historical agreements changed")
	}
}

func TestBillTotalsSkipVoidsAndRefuseOverflow(t *testing.T) {
	f := financeSeed()
	f.Bills = []FinanceBill{{ID: "BIL-0001", AmountPaise: 2000000}, {ID: "BIL-0002", AmountPaise: 600000, Voided: &FinanceVoid{}}}
	if got, err := TotalBills(f, nil); err != nil || got != 2000000 {
		t.Fatalf("TotalBills = %d, %v", got, err)
	}
	f.Bills = []FinanceBill{{ID: "BIL-0001", AmountPaise: 1<<63 - 1}, {ID: "BIL-0002", AmountPaise: 1}}
	if _, err := TotalBills(f, nil); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestBillValidationAndCrossReferences(t *testing.T) {
	r, f, bill := validBillFixture(t)
	f.Bills = []FinanceBill{bill}
	f.Movements = []MoneyMovement{{
		ID: "MOV-0001", Direction: MoneyOut, AmountPaise: 500000, OccurredAt: bill.OccurredAt,
		PartyID: bill.PartyID, PurposeID: bill.PurposeID, ModeID: "PMD-0001",
		SupplierChallanID: bill.SupplierChallanID, RecordedAt: bill.RecordedAt, RecordedByID: "FAC-0001",
	}}
	if err := ValidateFinance(f); err != nil {
		t.Fatalf("valid finance refused: %v", err)
	}
	if err := ValidatePartyReferences(r, f); err != nil {
		t.Fatalf("valid combined references refused: %v", err)
	}
	if !PartyIsUsed(r, f, bill.PartyID) || !FinanceValueIsUsed(f, bill.PurposeID) || !FinanceLineIsReferenced(f, "OLN-0001") {
		t.Fatal("bill use guards missed a reference")
	}

	t.Run("wrong party challan", func(t *testing.T) {
		other, _ := AddParty(r, "Gupta Electricals")
		wrong, _ := AddSupplierChallan(r, other, "GE/118")
		copy := *f
		copy.Bills = append([]FinanceBill{}, f.Bills...)
		copy.Bills[0].SupplierChallanID = wrong
		if err := ValidatePartyReferences(r, &copy); err == nil {
			t.Fatal("wrong-party bill challan accepted")
		}
	})

	t.Run("product snapshot differs from order", func(t *testing.T) {
		copy := *f
		copy.Bills = append([]FinanceBill{}, f.Bills...)
		copy.Bills[0].Products = []FinanceProductRef{{ProductID: "PRD-0001", ProductName: "Tables"}}
		if err := ValidateFinance(&copy); err == nil {
			t.Fatal("bill snapshot mismatch accepted")
		}
	})
}

func TestBillLookupOrderingAndNextID(t *testing.T) {
	f := financeSeed()
	at := time.Date(2026, 9, 3, 12, 40, 0, 0, IST)
	f.Bills = []FinanceBill{{ID: "BIL-0002", OccurredAt: at}, {ID: "BIL-0001", OccurredAt: at.Add(time.Minute)}}
	if got := f.NextID("BIL"); got != "BIL-0003" {
		t.Fatalf("NextID(BIL) = %q", got)
	}
	if got, ok := BillByID(f, "BIL-0002"); !ok || got.ID != "BIL-0002" {
		t.Fatal("BillByID missed bill")
	}
	if got := SortedBills(f.Bills); got[0].ID != "BIL-0001" {
		t.Fatalf("descending order = %+v", got)
	}
	if got := AscendingBills(f.Bills); got[0].ID != "BIL-0002" {
		t.Fatalf("ascending order = %+v", got)
	}
}
