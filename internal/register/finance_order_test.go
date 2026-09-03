package register

import (
	"strings"
	"testing"
	"time"
)

func financeSeed() *FinanceData {
	at := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	f := &FinanceData{
		Accounts: []FinanceAccount{{
			ID: "FAC-0001", DisplayName: "Asha Patel", Mobile: "9820011111",
			Role: FinanceAdmin, Status: "active", CreatedAt: at, CreatedByID: "FAC-0001",
		}},
		Orders:          []FinanceOrder{},
		ReusableValues:  []FinanceReusableValue{},
		Movements:       []MoneyMovement{},
		SupplierReturns: []SupplierReturn{},
		Sales:           []StockSale{},
		Audit:           []FinanceAuditEvent{},
	}
	for _, mode := range InitialPaymentModes {
		if _, err := AddFinanceValue(f, FinanceMode, mode, "FAC-0001", at); err != nil {
			panic(err)
		}
	}
	return f
}

// The rupee parser is the only place money enters the program, so it is the
// only place a wrong answer can be introduced. No float appears in it.
func TestParseRupeesAcceptsAndRefuses(t *testing.T) {
	good := map[string]int64{
		"5000":                 500000,
		"5000.5":               500050,
		"5000.50":              500050,
		"0.01":                 1,
		"  2500.75  ":          250075,
		"25000":                2500000,
		"92233720368547758.07": 1<<63 - 1,
	}
	for in, want := range good {
		got, err := ParseRupees(in)
		if err != nil {
			t.Fatalf("ParseRupees(%q) refused: %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseRupees(%q) = %d, want %d", in, got, want)
		}
	}

	bad := []string{
		"", "  ", "0", "0.00", "-5", "+5", "5,000", "5e3", "5.005", "5.",
		".5", "abc", "5 000", "₹5000", "5.0.0",
		"92233720368547758.08", "92233720368547759", "99999999999999999999",
	}
	for _, in := range bad {
		if got, err := ParseRupees(in); err == nil {
			t.Fatalf("ParseRupees(%q) accepted %d, want a refusal", in, got)
		} else if err.Error() != MoneyRefusal {
			t.Fatalf("ParseRupees(%q) said %q, want the one refusal wording", in, err)
		}
	}
}

func TestFormatRupeesAlwaysTwoDecimals(t *testing.T) {
	cases := map[int64]string{
		1: "₹0.01", 50: "₹0.50", 100: "₹1.00",
		500050: "₹5,000.50", 2500000: "₹25,000.00",
		// Indian grouping: three digits, then twos.
		1234567890: "₹1,23,45,678.90",
	}
	for paise, want := range cases {
		if got := FormatRupees(paise); got != want {
			t.Fatalf("FormatRupees(%d) = %q, want %q", paise, got, want)
		}
	}
}

func TestInitialPaymentModesAreCreatedInOrder(t *testing.T) {
	f := financeSeed()
	live := LiveFinanceValues(f, FinanceMode)
	if len(live) != 5 {
		t.Fatalf("got %d modes, want 5", len(live))
	}
	// Stored in the spec's order; listed alphabetically by fold.
	var stored []string
	for _, v := range f.ReusableValues {
		stored = append(stored, v.Value)
	}
	if strings.Join(stored, ",") != "Cash,UPI,Bank transfer,Cheque,Card" {
		t.Fatalf("modes stored as %v", stored)
	}
	if f.ReusableValues[0].ID != "PMD-0001" || f.ReusableValues[4].ID != "PMD-0005" {
		t.Fatalf("mode ids are %s..%s", f.ReusableValues[0].ID, f.ReusableValues[4].ID)
	}
	if len(LiveFinanceValues(f, FinanceParty)) != 0 || len(LiveFinanceValues(f, FinancePurpose)) != 0 {
		t.Fatalf("parties and purposes must start empty")
	}
}

func TestMatchFinanceValuesOrdersPrefixFirst(t *testing.T) {
	f := financeSeed()
	at := time.Now()
	for _, v := range []string{"Cashew supplier", "Advance cash", "Bank draft"} {
		if _, err := AddFinanceValue(f, FinanceMode, v, "FAC-0001", at); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, v := range MatchFinanceValues(f, FinanceMode, "ca") {
		got = append(got, v.Value)
	}
	// Prefix matches alphabetically, then substring matches alphabetically.
	want := "Card,Cash,Cashew supplier,Advance cash"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}

	var ba []string
	for _, v := range MatchFinanceValues(f, FinanceMode, "ba") {
		ba = append(ba, v.Value)
	}
	if strings.Join(ba, ",") != "Bank draft,Bank transfer" {
		t.Fatalf("typing ba gave %v", ba)
	}
}

func TestAddFinanceValueReusesExactFoldedMatch(t *testing.T) {
	f := financeSeed()
	at := time.Now()
	before := len(f.ReusableValues)
	id, err := AddFinanceValue(f, FinanceMode, "  cash  ", "FAC-0001", at)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ReusableValues) != before {
		t.Fatalf("a case-fold match created a second row")
	}
	if got := FinanceValueText(f, id); got != "Cash" {
		t.Fatalf("resolved to %q, want Cash", got)
	}
	if _, err := AddFinanceValue(f, FinanceParty, "   ", "FAC-0001", at); err == nil {
		t.Fatalf("a blank value was accepted")
	}
}

func TestResolveFinanceValueFollowsMergeChain(t *testing.T) {
	f := financeSeed()
	at := time.Now()
	online, _ := AddFinanceValue(f, FinanceMode, "Online payment", "FAC-0001", at)
	middle, _ := AddFinanceValue(f, FinanceMode, "Online", "FAC-0001", at)
	bank, _ := FindFinanceValueByText(f, FinanceMode, "Bank transfer")

	for i := range f.ReusableValues {
		switch f.ReusableValues[i].ID {
		case online:
			f.ReusableValues[i].MergedIntoID = middle
		case middle:
			f.ReusableValues[i].MergedIntoID = bank.ID
		}
	}
	if got := FinanceValueText(f, online); got != "Bank transfer" {
		t.Fatalf("merged value shows %q, want Bank transfer", got)
	}
	if err := ValidateFinance(f); err != nil {
		t.Fatalf("a valid merge chain was refused: %v", err)
	}
	// A cycle must never be storable.
	for i := range f.ReusableValues {
		if f.ReusableValues[i].ID == bank.ID {
			f.ReusableValues[i].MergedIntoID = online
		}
	}
	if err := ValidateFinance(f); err == nil {
		t.Fatalf("a merge cycle was accepted")
	}
}

func financeOrder(f *FinanceData, lines ...FinanceOrderLine) FinanceOrder {
	party, _ := AddFinanceValue(f, FinanceParty, "Sharma Events", "FAC-0001", time.Now())
	at := time.Date(2026, 9, 2, 11, 0, 0, 0, time.Local)
	return FinanceOrder{
		ID: f.NextID("ORD"), PartyID: party, Lines: lines, OrderedAt: at,
		Status: "open", CreatedAt: at, CreatedByID: "FAC-0001",
	}
}

func TestValidateFinanceHoldsTheOrderInvariants(t *testing.T) {
	line := func(id, product string, qty int, basis Basis) FinanceOrderLine {
		return FinanceOrderLine{ID: id, ProductID: product, ProductNameSnapshot: "Chairs", ExpectedQuantity: qty, Basis: basis}
	}

	good := financeSeed()
	good.Orders = append(good.Orders, financeOrder(good,
		line("OLN-0001", "PRD-0001", 100, Rent),
		line("OLN-0002", "PRD-0001", 50, Purchase),
	))
	if err := ValidateFinance(good); err != nil {
		t.Fatalf("one product on both bases was refused: %v", err)
	}

	refusals := map[string]func(*FinanceOrder){
		"no lines":              func(o *FinanceOrder) { o.Lines = nil },
		"zero quantity":         func(o *FinanceOrder) { o.Lines[0].ExpectedQuantity = 0 },
		"unknown basis":         func(o *FinanceOrder) { o.Lines[0].Basis = "borrowed" },
		"repeated product line": func(o *FinanceOrder) { o.Lines[1].Basis = Rent },
		"duplicate line id":     func(o *FinanceOrder) { o.Lines[1].ID = "OLN-0001" },
		"bad status":            func(o *FinanceOrder) { o.Status = "half" },
		"kind without total":    func(o *FinanceOrder) { o.AgreedKind = "exact" },
		"blank snapshot":        func(o *FinanceOrder) { o.Lines[0].ProductNameSnapshot = "" },
	}
	for name, breakIt := range refusals {
		f := financeSeed()
		o := financeOrder(f,
			line("OLN-0001", "PRD-0001", 100, Rent),
			line("OLN-0002", "PRD-0001", 50, Purchase),
		)
		breakIt(&o)
		f.Orders = append(f.Orders, o)
		if err := ValidateFinance(f); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}

	// A total needs a kind, and only a positive one is a total at all.
	f := financeSeed()
	o := financeOrder(f, line("OLN-0001", "PRD-0001", 100, Rent))
	zero := int64(0)
	o.AgreedPaise, o.AgreedKind = &zero, "exact"
	f.Orders = append(f.Orders, o)
	if err := ValidateFinance(f); err == nil {
		t.Fatalf("a zero total was accepted")
	}
}

func TestFinanceOrderLineIDsAreUniqueAcrossTheVault(t *testing.T) {
	f := financeSeed()
	f.Orders = append(f.Orders, financeOrder(f, FinanceOrderLine{
		ID: f.NextID("OLN"), ProductID: "PRD-0001", ProductNameSnapshot: "Chairs",
		ExpectedQuantity: 10, Basis: Rent,
	}))
	if f.Orders[0].Lines[0].ID != "OLN-0001" {
		t.Fatalf("first line id is %s", f.Orders[0].Lines[0].ID)
	}
	next := f.NextID("OLN")
	if next != "OLN-0002" {
		t.Fatalf("second order's first line id is %s, want OLN-0002", next)
	}
}

func TestSortedFinanceOrdersAreNewestFirst(t *testing.T) {
	f := financeSeed()
	mk := func(id string, day int) FinanceOrder {
		o := financeOrder(f, FinanceOrderLine{ID: "OLN-" + id, ProductID: "PRD-0001", ProductNameSnapshot: "Chairs", ExpectedQuantity: 1, Basis: Rent})
		o.ID, o.OrderedAt = "ORD-"+id, time.Date(2026, 9, day, 9, 0, 0, 0, time.Local)
		return o
	}
	f.Orders = []FinanceOrder{mk("0001", 1), mk("0003", 5), mk("0002", 5)}
	var got []string
	for _, o := range SortedFinanceOrders(f) {
		got = append(got, o.ID)
	}
	if strings.Join(got, ",") != "ORD-0002,ORD-0003,ORD-0001" {
		t.Fatalf("order is %v", got)
	}
}

// A money entry that names an order line locks that line: the order screen must
// not let it be taken away or pointed at a different product, because the money
// already recorded refers to it. Voiding the entry frees the line again.
func TestFinanceLineIsReferencedFollowsLiveMoney(t *testing.T) {
	f := &FinanceData{
		Movements: []MoneyMovement{
			{ID: "MOV-0001", OrderID: "ORD-0001", OrderLineIDs: []string{"OLN-0001"}},
			{ID: "MOV-0002", OrderID: "ORD-0001", OrderLineIDs: []string{"OLN-0002"},
				Voided: &FinanceVoid{Reason: "recorded twice"}},
		},
	}
	for _, tt := range []struct {
		name, line string
		want       bool
	}{
		{"a live entry names it", "OLN-0001", true},
		{"only a voided entry names it", "OLN-0002", false},
		{"nothing names it", "OLN-0003", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := FinanceLineIsReferenced(f, tt.line); got != tt.want {
				t.Errorf("FinanceLineIsReferenced(%s) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
