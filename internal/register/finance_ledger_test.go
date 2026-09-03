package register

import (
	"strings"
	"testing"
	"time"
)

func money(f *FinanceData, id string, dir MoneyDirection, paise int64, at time.Time) MoneyMovement {
	party, _ := AddFinanceValue(f, FinanceParty, "Sharma Events", "FAC-0001", at)
	purpose, _ := AddFinanceValue(f, FinancePurpose, "Rent", "FAC-0001", at)
	mode, _ := FindFinanceValueByText(f, FinanceMode, "Cash")
	return MoneyMovement{
		ID: id, Direction: dir, AmountPaise: paise, OccurredAt: at,
		PartyID: party, PurposeID: purpose, ModeID: mode.ID,
		RecordedAt: at, RecordedByID: "FAC-0001",
	}
}

func TestAddAndSubPaiseRefuseRatherThanWrap(t *testing.T) {
	const max = int64(1<<63 - 1)
	if got, err := AddPaise(max-1, 1); err != nil || got != max {
		t.Fatalf("AddPaise to the limit gave %d, %v", got, err)
	}
	if _, err := AddPaise(max, 1); err == nil {
		t.Fatal("AddPaise wrapped round")
	}
	if err := func() error { _, err := AddPaise(max, 1); return err }(); err.Error() != ErrMoneyOverflow.Error() {
		t.Fatalf("the refusal reads %q", err)
	}
	if _, err := SubPaise(-(1 << 63), 1); err == nil {
		t.Fatal("SubPaise wrapped round")
	}
	if got, err := SubPaise(1000, 400); err != nil || got != 600 {
		t.Fatalf("SubPaise gave %d, %v", got, err)
	}
}

func TestTotalMoneyCountsOnlyLiveMovements(t *testing.T) {
	f := financeSeed()
	at := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	f.Movements = []MoneyMovement{
		money(f, "MOV-0001", MoneyOut, 1000000, at),
		money(f, "MOV-0002", MoneyIn, 400000, at),
		money(f, "MOV-0003", MoneyOut, 250000, at),
	}
	// A voided row is excluded from every total but stays on the list.
	f.Movements[2].Voided = &FinanceVoid{
		At: at, ByAccountID: "FAC-0001", ByName: "Asha Patel",
		ByMobile: "9820011111", Reason: "Typed twice",
	}

	totals, err := TotalMoney(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if totals.PaidPaise != 1000000 || totals.ReceivedPaise != 400000 || totals.NetPaise != 600000 {
		t.Fatalf("totals are %+v", totals)
	}
	if len(f.Movements) != 3 {
		t.Fatal("a voided row was removed from the list")
	}
	if err := ValidateFinance(f); err != nil {
		t.Fatalf("a voided movement was refused: %v", err)
	}

	// An overflowing total is refused, never wrapped.
	f.Movements = append(f.Movements, money(f, "MOV-0004", MoneyOut, 1<<62, at), money(f, "MOV-0005", MoneyOut, 1<<62, at))
	if _, err := TotalMoney(f, nil); err == nil {
		t.Fatal("an overflowing total was returned")
	}
}

func TestOrderTotalsSumOnlyThatOrder(t *testing.T) {
	f := financeSeed()
	at := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	party, _ := AddFinanceValue(f, FinanceParty, "Sharma Events", "FAC-0001", at)
	f.Orders = []FinanceOrder{{
		ID: "ORD-0001", PartyID: party, OrderedAt: at, Status: "open",
		CreatedAt: at, CreatedByID: "FAC-0001",
		Lines: []FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001",
			ProductNameSnapshot: "Chairs", ExpectedQuantity: 100, Basis: Rent}},
	}}
	a := money(f, "MOV-0001", MoneyOut, 1000000, at)
	a.OrderID = "ORD-0001"
	b := money(f, "MOV-0002", MoneyIn, 1000000, at)
	b.OrderID = "ORD-0001"
	f.Movements = []MoneyMovement{a, b, money(f, "MOV-0003", MoneyOut, 700000, at)}

	totals, err := OrderTotals(f, "ORD-0001")
	if err != nil {
		t.Fatal(err)
	}
	// Paid and refunded in full: the order nets to nothing, and neither row
	// was deleted to get there.
	if totals.PaidPaise != 1000000 || totals.ReceivedPaise != 1000000 || totals.NetPaise != 0 {
		t.Fatalf("order totals are %+v", totals)
	}
	if err := ValidateFinance(f); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFinanceHoldsTheMovementInvariants(t *testing.T) {
	at := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	build := func() (*FinanceData, *MoneyMovement) {
		f := financeSeed()
		party, _ := AddFinanceValue(f, FinanceParty, "Sharma Events", "FAC-0001", at)
		f.Orders = []FinanceOrder{{
			ID: "ORD-0001", PartyID: party, OrderedAt: at, Status: "open",
			CreatedAt: at, CreatedByID: "FAC-0001",
			Lines: []FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001",
				ProductNameSnapshot: "Chairs", ExpectedQuantity: 100, Basis: Rent}},
		}}
		m := money(f, "MOV-0001", MoneyOut, 500000, at)
		f.Movements = []MoneyMovement{m}
		return f, &f.Movements[0]
	}

	f, _ := build()
	if err := ValidateFinance(f); err != nil {
		t.Fatalf("a plain movement was refused: %v", err)
	}

	for name, breakIt := range map[string]func(*FinanceData, *MoneyMovement){
		"zero amount":      func(_ *FinanceData, m *MoneyMovement) { m.AmountPaise = 0 },
		"negative amount":  func(_ *FinanceData, m *MoneyMovement) { m.AmountPaise = -1 },
		"no direction":     func(_ *FinanceData, m *MoneyMovement) { m.Direction = "" },
		"unknown purpose":  func(_ *FinanceData, m *MoneyMovement) { m.PurposeID = "PUR-9999" },
		"unknown mode":     func(_ *FinanceData, m *MoneyMovement) { m.ModeID = "PMD-9999" },
		"unknown recorder": func(_ *FinanceData, m *MoneyMovement) { m.RecordedByID = "FAC-9999" },
		"unknown order":    func(_ *FinanceData, m *MoneyMovement) { m.OrderID = "ORD-9999" },
		"lines with no order": func(_ *FinanceData, m *MoneyMovement) {
			m.OrderLineIDs = []string{"OLN-0001"}
		},
		"line off the order": func(_ *FinanceData, m *MoneyMovement) {
			m.OrderID, m.OrderLineIDs = "ORD-0001", []string{"OLN-0002"}
		},
		"repeated line": func(_ *FinanceData, m *MoneyMovement) {
			m.OrderID, m.OrderLineIDs = "ORD-0001", []string{"OLN-0001", "OLN-0001"}
		},
		"blank product snapshot": func(_ *FinanceData, m *MoneyMovement) {
			m.Products = []FinanceProductRef{{ProductID: "PRD-0001"}}
		},
		"void with no reason": func(_ *FinanceData, m *MoneyMovement) {
			m.Voided = &FinanceVoid{At: at, ByAccountID: "FAC-0001", ByName: "Asha Patel", ByMobile: "9820011111"}
		},
	} {
		f, m := build()
		breakIt(f, m)
		if err := ValidateFinance(f); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// Duplicated IDs are refused.
	f, _ = build()
	f.Movements = append(f.Movements, f.Movements[0])
	if err := ValidateFinance(f); err == nil {
		t.Error("a duplicated movement id was accepted")
	}
}

func TestFinanceValueIsUsedCoversMovements(t *testing.T) {
	f := financeSeed()
	at := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	m := money(f, "MOV-0001", MoneyOut, 500000, at)
	f.Movements = []MoneyMovement{m}
	for _, id := range []string{m.PartyID, m.PurposeID, m.ModeID} {
		if !FinanceValueIsUsed(f, id) {
			t.Errorf("%s is used by a payment but reads as unused", id)
		}
	}
	// Voiding does not free the values: the row still says what it said.
	f.Movements[0].Voided = &FinanceVoid{At: at, ByAccountID: "FAC-0001",
		ByName: "Asha Patel", ByMobile: "9820011111", Reason: "Typed twice"}
	if !FinanceValueIsUsed(f, m.PurposeID) {
		t.Error("voiding a payment freed its purpose for deletion")
	}
	unused, _ := FindFinanceValueByText(f, FinanceMode, "Cheque")
	if FinanceValueIsUsed(f, unused.ID) {
		t.Error("an untouched mode reads as used")
	}
}

func TestMovementSortsDifferForScreenPrintAndActivity(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 9, 2, h, m, 0, 0, time.Local) }
	rows := []MoneyMovement{
		{ID: "MOV-0001", OccurredAt: at(10, 0)},
		{ID: "MOV-0003", OccurredAt: at(9, 0)},
		{ID: "MOV-0002", OccurredAt: at(10, 0)},
	}
	var screen, printed []string
	for _, m := range SortedMovements(rows) {
		screen = append(screen, m.ID)
	}
	for _, m := range AscendingMovements(rows) {
		printed = append(printed, m.ID)
	}
	if strings.Join(screen, ",") != "MOV-0002,MOV-0001,MOV-0003" {
		t.Errorf("the screen order is %v", screen)
	}
	if strings.Join(printed, ",") != "MOV-0003,MOV-0001,MOV-0002" {
		t.Errorf("the printed order is %v", printed)
	}

	f := &FinanceData{Audit: []FinanceAuditEvent{
		{ID: "FAE-0001", At: at(9, 0)},
		{ID: "FAE-0003", At: at(11, 0)},
		{ID: "FAE-0002", At: at(11, 0)},
	}}
	var activity []string
	for _, a := range SortedAudit(f) {
		activity = append(activity, a.ID)
	}
	if strings.Join(activity, ",") != "FAE-0003,FAE-0002,FAE-0001" {
		t.Errorf("the activity order is %v", activity)
	}
}

func TestJournalFilterPrecedenceAndInclusiveBoundaries(t *testing.T) {
	loc := time.Local
	// The spec's own example: five movements around a 23:00-23:39 window.
	rows := []MoneyMovement{}
	for i, minute := range []string{"22:59", "23:00", "23:20", "23:39", "23:40"} {
		t0, err := time.ParseInLocation("2006-01-02T15:04", "2016-01-01T"+minute, loc)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, MoneyMovement{ID: "MOV-000" + string(rune('1'+i)), OccurredAt: t0})
	}
	kept := func(f JournalFilter) string {
		var out []string
		for _, m := range rows {
			if f.Keep(m) {
				out = append(out, m.OccurredAt.Format("15:04"))
			}
		}
		return strings.Join(out, ",")
	}

	// An exact range includes both the minute typed at each end.
	exact, err := ParseJournalFilter("", "", "", "2016-01-01T23:00", "2016-01-01T23:39", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := kept(exact); got != "23:00,23:20,23:39" {
		t.Fatalf("the exact range kept %s", got)
	}

	// It wins over a day and a date range supplied at the same time.
	both, err := ParseJournalFilter("2016-01-02", "2016-01-05", "2016-01-06", "2016-01-01T23:00", "2016-01-01T23:39", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := kept(both); got != "23:00,23:20,23:39" {
		t.Fatalf("the exact range lost to the other filters: %s", got)
	}

	// A day takes the whole local calendar day.
	day, err := ParseJournalFilter("2016-01-01", "2016-01-05", "2016-01-06", "", "", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := kept(day); got != "22:59,23:00,23:20,23:39,23:40" {
		t.Fatalf("the day kept %s", got)
	}

	// A date range is inclusive at both ends.
	span, err := ParseJournalFilter("", "2015-12-31", "2016-01-01", "", "", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got := kept(span); got != "22:59,23:00,23:20,23:39,23:40" {
		t.Fatalf("the range kept %s", got)
	}

	// No parameters is every date.
	every, err := ParseJournalFilter("", "", "", "", "", loc)
	if err != nil || !every.Every || every.Label != "Every date" {
		t.Fatalf("the empty filter is %+v, %v", every, err)
	}

	// Half a range, an unparseable end and a backwards range are all refused
	// with the one wording.
	for name, args := range map[string][5]string{
		"half a date range":     {"", "2016-01-01", "", "", ""},
		"half an exact range":   {"", "", "", "2016-01-01T23:00", ""},
		"unparseable day":       {"the first", "", "", "", ""},
		"unparseable range":     {"", "2016-13-45", "2016-01-02", "", ""},
		"backwards range":       {"", "2016-01-05", "2016-01-01", "", ""},
		"backwards exact":       {"", "", "", "2016-01-01T23:39", "2016-01-01T23:00"},
		"unparseable exact end": {"", "", "", "2016-01-01T23:00", "not a time"},
	} {
		if _, err := ParseJournalFilter(args[0], args[1], args[2], args[3], args[4], loc); err == nil {
			t.Errorf("%s was accepted", name)
		} else if err.Error() != JournalRefusal {
			t.Errorf("%s said %q", name, err)
		}
	}
}

func TestDirectionTextIsTheWordingPeopleRead(t *testing.T) {
	if got := DirectionText(MoneyOut); got != "Money paid" {
		t.Errorf("out reads %q", got)
	}
	if got := DirectionText(MoneyIn); got != "Money received" {
		t.Errorf("in reads %q", got)
	}
}
