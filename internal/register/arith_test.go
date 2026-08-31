package register

import (
	"errors"
	"testing"
	"time"
)

const (
	chairs = "PRD-0001"
	tables = "PRD-0002"
	drums  = "PRD-0003"
	boards = "PRD-0004"
	sacks  = "PRD-0005"
)

var (
	t0Clock = MustTime("2026-09-03T10:00:00+05:30")
	t1Clock = MustTime("2026-09-03T10:42:00+05:30")
	t4Clock = MustTime("2026-09-03T18:10:00+05:30")
)

// ---------- on hand ----------

func TestOnHandAtT0(t *testing.T) {
	r := WalkthroughT0()
	cases := []struct {
		product string
		want    int
	}{
		{chairs, 390},
		{tables, 48},
		{drums, 35},
		{boards, 0},
		{sacks, 12},
	}
	for _, c := range cases {
		if got := OnHand(r, c.product); got != c.want {
			t.Errorf("OnHand(%s) = %d, want %d", c.product, got, c.want)
		}
	}
}

func TestOnHandAfterInward(t *testing.T) {
	r := WalkthroughT1()
	if got := CameIn(r, chairs); got != 1200 {
		t.Errorf("CameIn = %d, want 1200", got)
	}
	if got := OutWithPeople(r, chairs); got != 310 {
		t.Errorf("OutWithPeople = %d, want 310", got)
	}
	if got := OnHand(r, chairs); got != 890 {
		t.Errorf("OnHand = %d, want 890", got)
	}
}

func TestOnHandAfterIssue(t *testing.T) {
	r := WalkthroughT2()
	if got := OnHand(r, chairs); got != 880 {
		t.Errorf("OnHand = %d, want 880", got)
	}
	if got := OutWithPeople(r, chairs); got != 320 {
		t.Errorf("OutWithPeople = %d, want 320", got)
	}
}

func TestOnHandAfterPartialReturn(t *testing.T) {
	r := WalkthroughT3()
	if got := CameIn(r, chairs); got != 1200 {
		t.Errorf("CameIn = %d, want 1200", got)
	}
	if got := Returned(r, chairs); got != 45 {
		t.Errorf("Returned = %d, want 45", got)
	}
	if got := OutWithPeople(r, chairs); got != 275 {
		t.Errorf("OutWithPeople = %d, want 275", got)
	}
	if got := OnHand(r, chairs); got != 925 {
		t.Errorf("OnHand = %d, want 925", got)
	}
}

func TestOnHandMixedSequence(t *testing.T) {
	steps := []struct {
		name string
		reg  *Register
		want int
	}{
		{"after INW-0007", WalkthroughT1(), 890},
		{"after ISS-0008", WalkthroughT2(), 880},
		{"after RET-0001", WalkthroughT3(), 925},
	}
	for _, s := range steps {
		if got := OnHand(s.reg, chairs); got != s.want {
			t.Errorf("%s: OnHand = %d, want %d", s.name, got, s.want)
		}
	}

	r := WalkthroughT3()
	r.Returns = append(r.Returns, Return{
		ID: "RET-0002", ProductID: chairs,
		Allocations:  []Allocation{{IssueID: "ISS-0008", Quantity: 5}},
		ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
		TakenBackBy: "Imran Sheikh",
		ReturnedAt:  t4Clock, RecordedAt: t4Clock,
	})
	if got := OnHand(r, chairs); got != 930 {
		t.Errorf("after the last 5 come back: OnHand = %d, want 930", got)
	}
	for _, line := range OutstandingForPerson(r, "Ravi Menon", "98861 40023") {
		if line.ProductID == chairs {
			t.Errorf("Ravi still holds %d chairs on %s", line.Out, line.IssueID)
		}
	}
}

func TestOnHandUnknownProduct(t *testing.T) {
	r := WalkthroughT0()
	if got := OnHand(r, "PRD-9999"); got != 0 {
		t.Errorf("OnHand = %d, want 0", got)
	}
	if got := CameIn(r, "PRD-9999"); got != 0 {
		t.Errorf("CameIn = %d, want 0", got)
	}
	if got := OutWithPeople(r, "PRD-9999"); got != 0 {
		t.Errorf("OutWithPeople = %d, want 0", got)
	}
	if got := Returned(r, "PRD-9999"); got != 0 {
		t.Errorf("Returned = %d, want 0", got)
	}
}

func TestOnHandNeverNegativeAcrossFixture(t *testing.T) {
	points := map[string]*Register{
		"T0": WalkthroughT0(), "T1": WalkthroughT1(),
		"T2": WalkthroughT2(), "T3": WalkthroughT3(),
	}
	for name, r := range points {
		for _, p := range r.Products {
			if got := OnHand(r, p.ID); got < 0 {
				t.Errorf("%s: OnHand(%s) = %d", name, p.Name, got)
			}
			if got := OutWithPeople(r, p.ID); got < 0 {
				t.Errorf("%s: OutWithPeople(%s) = %d", name, p.Name, got)
			}
		}
	}
}

// ---------- stock rows ----------

func TestStockRowsAtT0(t *testing.T) {
	rows := StockRows(WalkthroughT0())
	wantOrder := []string{"Chairs", "Charcoal sacks", "Extension boards", "Round tables", "Water drums (20L)"}
	if len(rows) != len(wantOrder) {
		t.Fatalf("got %d rows, want %d", len(rows), len(wantOrder))
	}
	for i, name := range wantOrder {
		if rows[i].Name != name {
			t.Errorf("row %d is %q, want %q", i, rows[i].Name, name)
		}
	}
	if got := rows[0]; got.CameIn != 700 || got.Out != 310 || got.OnHand != 390 || got.Basis != Rent {
		t.Errorf("Chairs row = %+v, want 700/310/390 on rent", got)
	}
	if got := rows[4]; got.CameIn != 40 || got.Out != 5 || got.OnHand != 35 || got.Basis != Purchase {
		t.Errorf("Water drums row = %+v, want 40/5/35 purchased", got)
	}
	if rows[2].OnHand != 0 {
		t.Errorf("Extension boards on hand = %d, want 0", rows[2].OnHand)
	}
}

func TestStockRowsIncludeProductWithNoStock(t *testing.T) {
	r := WalkthroughT0()
	r.Products = append(r.Products, Product{
		ID: "PRD-0006", Name: "Gas cylinders",
		CreatedAt: MustTime("2026-09-03T10:00:00+05:30"), CreatedBy: "Suresh Kumar",
	})
	rows := StockRows(r)
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6", len(rows))
	}
	if rows[3].Name != "Gas cylinders" {
		t.Fatalf("row 3 is %q, want Gas cylinders between Extension boards and Round tables", rows[3].Name)
	}
	if rows[3].CameIn != 0 || rows[3].Out != 0 || rows[3].OnHand != 0 {
		t.Errorf("Gas cylinders row = %+v, want three zeros", rows[3])
	}
}

// ---------- issue refusals ----------

func quantityErr(t *testing.T, err error) QuantityError {
	t.Helper()
	var qe QuantityError
	if !errors.As(err, &qe) {
		t.Fatalf("error %v is not a QuantityError", err)
	}
	return qe
}

func TestIssueRefusedOverOnHand(t *testing.T) {
	err := CheckIssue(WalkthroughT1(), chairs, 900, t1Clock)
	qe := quantityErr(t, err)
	if qe.Asked != 900 || qe.Allowed != 890 || qe.ProductName != "Chairs" || qe.Field != "issue" {
		t.Errorf("QuantityError = %+v, want asked 900 of 890 chairs", qe)
	}
	if qe.Error() == "" {
		t.Error("QuantityError has no message")
	}
}

func TestIssueAllowedAtExactlyOnHand(t *testing.T) {
	r := WalkthroughT1()
	if err := CheckIssue(r, chairs, 890, t1Clock); err != nil {
		t.Errorf("890 chairs refused: %v", err)
	}
	if err := CheckIssue(r, chairs, 891, t1Clock); err == nil {
		t.Error("891 chairs allowed")
	}
}

func TestIssueRefusedWhenNoneLeft(t *testing.T) {
	r := WalkthroughT0()
	for _, qty := range []int{1, 2, 25} {
		qe := quantityErr(t, CheckIssue(r, boards, qty, t0Clock))
		if qe.Allowed != 0 {
			t.Errorf("Allowed = %d, want 0", qe.Allowed)
		}
	}
}

func TestIssueRefusedForZeroAndNegative(t *testing.T) {
	r := WalkthroughT1()
	for _, qty := range []int{0, -5} {
		if err := CheckIssue(r, chairs, qty, t1Clock); err == nil {
			t.Errorf("%d chairs allowed", qty)
		}
	}
}

func TestIssueRefusedForUnknownProduct(t *testing.T) {
	err := CheckIssue(WalkthroughT1(), "PRD-9999", 1, t1Clock)
	if !errors.Is(err, ErrUnknownProduct) {
		t.Errorf("error = %v, want ErrUnknownProduct", err)
	}
}

// ---------- outstanding per person ----------

func sameLines(t *testing.T, got []OutstandingLine, want []OutstandingLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.IssueID != w.IssueID || g.ProductName != w.ProductName || g.Taken != w.Taken ||
			g.Back != w.Back || g.Out != w.Out || g.IssuedBy != w.IssuedBy {
			t.Errorf("line %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestRaviOutstandingBeforeReturn(t *testing.T) {
	lines := OutstandingForPerson(WalkthroughT2(), "Ravi Menon", "98861 40023")
	sameLines(t, lines, []OutstandingLine{
		{IssueID: "ISS-0003", ProductName: "Chairs", Taken: 40, Back: 0, Out: 40, IssuedBy: "Suresh Kumar"},
		{IssueID: "ISS-0005", ProductName: "Round tables", Taken: 2, Back: 0, Out: 2, IssuedBy: "Suresh Kumar"},
		{IssueID: "ISS-0008", ProductName: "Chairs", Taken: 10, Back: 0, Out: 10, IssuedBy: "Anita Rao"},
	})
	if !lines[0].IssuedAt.Equal(MustTime("2026-09-03T09:40:00+05:30")) {
		t.Errorf("ISS-0003 issued at %v", lines[0].IssuedAt)
	}
	if !lines[2].IssuedAt.Equal(MustTime("2026-09-03T14:18:00+05:30")) {
		t.Errorf("ISS-0008 issued at %v", lines[2].IssuedAt)
	}
	total := 0
	for _, l := range lines {
		if l.ProductID == chairs {
			total += l.Out
		}
	}
	if total != 50 {
		t.Errorf("Ravi holds %d chairs, want 50", total)
	}
}

func TestRaviOutstandingAfterPartialReturn(t *testing.T) {
	lines := OutstandingForPerson(WalkthroughT3(), "Ravi Menon", "98861 40023")
	sameLines(t, lines, []OutstandingLine{
		{IssueID: "ISS-0005", ProductName: "Round tables", Taken: 2, Back: 0, Out: 2, IssuedBy: "Suresh Kumar"},
		{IssueID: "ISS-0008", ProductName: "Chairs", Taken: 10, Back: 5, Out: 5, IssuedBy: "Anita Rao"},
	})
}

func TestPersonMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	r := WalkthroughT2()
	want := OutstandingForPerson(r, "Ravi Menon", "98861 40023")
	spellings := []struct{ name, mobile string }{
		{"  ravi   menon ", "98861 40023"},
		{"Ravi Menon", "9886140023"},
		{"RAVI MENON", "98861-40023"},
		{"Ravi Menon", "98861 40023"},
	}
	for _, s := range spellings {
		sameLines(t, OutstandingForPerson(r, s.name, s.mobile), want)
	}
	if len(want) != 3 {
		t.Fatalf("Ravi has %d lines, want 3", len(want))
	}
}

// twoRaviKumars adds two different people who happen to share a name.
func twoRaviKumars(r *Register) *Register {
	r.Issues = append(r.Issues,
		issue("ISS-0009", chairs, 6, "Ravi Kumar", "Logistics", "90011 22334",
			"Anita Rao", "99001 34562", MustTime("2026-09-03T15:00:00+05:30")),
		issue("ISS-0010", chairs, 4, "Ravi Kumar", "Logistics", "93400 55118",
			"Anita Rao", "99001 34562", MustTime("2026-09-03T15:05:00+05:30")),
	)
	return r
}

func TestTwoPeopleWithTheSameNameAreTwoPeople(t *testing.T) {
	r := twoRaviKumars(WalkthroughT2())
	var kumars []PersonSummary
	for _, p := range PeopleHolding(r) {
		if p.Name == "Ravi Kumar" {
			kumars = append(kumars, p)
		}
	}
	if len(kumars) != 2 {
		t.Fatalf("got %d Ravi Kumars, want 2", len(kumars))
	}
	if kumars[0].TotalOut != 6 || kumars[1].TotalOut != 4 {
		t.Errorf("totals are %d and %d, want 6 and 4", kumars[0].TotalOut, kumars[1].TotalOut)
	}
	lines := OutstandingForPerson(r, "Ravi Kumar", "90011 22334")
	if len(lines) != 1 || lines[0].Out != 6 {
		t.Errorf("lines = %+v, want one line of 6", lines)
	}
}

func TestPersonWithNoMobileIsKeyedOnNameAlone(t *testing.T) {
	r := WalkthroughT2()
	r.Issues = append(r.Issues,
		issue("ISS-0009", chairs, 3, "Meera Pillai", "Reception", "",
			"Anita Rao", "99001 34562", MustTime("2026-09-03T15:10:00+05:30")),
		issue("ISS-0010", chairs, 7, "Meera Pillai", "Reception", "99450 71222",
			"Anita Rao", "99001 34562", MustTime("2026-09-03T15:20:00+05:30")),
	)

	var meeras []PersonSummary
	for _, p := range PeopleHolding(r) {
		if p.Name == "Meera Pillai" {
			meeras = append(meeras, p)
		}
	}
	if len(meeras) != 2 {
		t.Fatalf("got %d Meera Pillais, want 2 unmerged rows", len(meeras))
	}
	found := false
	for _, m := range meeras {
		if m.Mobile == "" && m.TotalOut == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("no Meera Pillai with no mobile holding 3: %+v", meeras)
	}
	lines := OutstandingForPerson(r, "Meera Pillai", "")
	if len(lines) != 1 || lines[0].Out != 3 {
		t.Errorf("lines = %+v, want one line of 3", lines)
	}
}

func TestPeopleHoldingAtT0(t *testing.T) {
	people := PeopleHolding(WalkthroughT0())
	want := []struct {
		name string
		out  int
	}{
		{"Farida Begum", 5},
		{"Joseph D'Cruz", 145},
		{"Lakshmi Iyer", 160},
		{"Ravi Menon", 42},
	}
	if len(people) != len(want) {
		t.Fatalf("got %d people, want %d", len(people), len(want))
	}
	for i, w := range want {
		if people[i].Name != w.name || people[i].TotalOut != w.out {
			t.Errorf("person %d = %s holding %d, want %s holding %d",
				i, people[i].Name, people[i].TotalOut, w.name, w.out)
		}
	}
	ravi := people[3]
	if ravi.Department != "Catering" || ravi.Mobile != "98861 40023" {
		t.Errorf("Ravi = %+v, want Catering / 98861 40023", ravi)
	}
	if len(ravi.Lines) != 2 {
		t.Errorf("Ravi has %d lines, want 2", len(ravi.Lines))
	}
}

func TestPeopleHoldingSortsSameNameByMobile(t *testing.T) {
	r := twoRaviKumars(WalkthroughT2())
	for run := 0; run < 3; run++ {
		people := PeopleHolding(r)
		first := -1
		for i, p := range people {
			if p.Name == "Ravi Kumar" {
				if first == -1 {
					first = i
				} else if i != first+1 {
					t.Fatalf("the two Ravi Kumars are not adjacent: %d and %d", first, i)
				}
			}
		}
		if people[first].ID.MobileKey != "9001122334" || people[first+1].ID.MobileKey != "9340055118" {
			t.Fatalf("mobiles out of order: %s then %s",
				people[first].ID.MobileKey, people[first+1].ID.MobileKey)
		}
	}
}

func TestFindPeopleByMobileFragment(t *testing.T) {
	found := FindPeople(WalkthroughT2(), "98861")
	if len(found) != 1 {
		t.Fatalf("got %d people, want 1: %+v", len(found), found)
	}
	if found[0].Name != "Ravi Menon" || found[0].TotalOut != 52 || len(found[0].Lines) != 3 {
		t.Errorf("found %+v, want Ravi Menon holding 52 across 3 lines", found[0])
	}
}

func TestFindPeopleByMobileIgnoresSpacing(t *testing.T) {
	r := WalkthroughT2()
	for _, query := range []string{"98861 40023", "9886140023", "8861400"} {
		found := FindPeople(r, query)
		if len(found) != 1 || found[0].Name != "Ravi Menon" {
			t.Errorf("FindPeople(%q) = %+v, want Ravi Menon alone", query, found)
		}
	}
}

func TestFindPeopleByName(t *testing.T) {
	r := WalkthroughT2()
	for _, query := range []string{"Ravi Menon", "ravi"} {
		found := FindPeople(r, query)
		if len(found) != 1 || found[0].Name != "Ravi Menon" {
			t.Errorf("FindPeople(%q) = %+v, want Ravi Menon alone", query, found)
		}
	}

	both := FindPeople(twoRaviKumars(WalkthroughT2()), "Ravi")
	if len(both) != 3 {
		t.Fatalf("got %d people, want 3", len(both))
	}
	if both[0].Name != "Ravi Kumar" || both[1].Name != "Ravi Kumar" || both[2].Name != "Ravi Menon" {
		t.Errorf("names are %s, %s, %s", both[0].Name, both[1].Name, both[2].Name)
	}
}

func TestFindPeopleByDepartment(t *testing.T) {
	r := WalkthroughT2()
	if found := FindPeople(r, "cater"); len(found) != 1 || found[0].Name != "Ravi Menon" {
		t.Errorf("FindPeople(cater) = %+v, want Ravi Menon alone", found)
	}
	if found := FindPeople(r, "Stage"); len(found) != 1 || found[0].Name != "Joseph D'Cruz" {
		t.Errorf("FindPeople(Stage) = %+v, want Joseph D'Cruz alone", found)
	}
}

func TestFindPeopleWithAnEmptyQueryReturnsEverybody(t *testing.T) {
	r := WalkthroughT2()
	if got, want := len(FindPeople(r, "   ")), len(PeopleHolding(r)); got != want {
		t.Errorf("an empty query found %d people, want %d", got, want)
	}
}

func TestFindPeopleExcludesFullyReturned(t *testing.T) {
	r := WalkthroughT3()
	r.Returns = append(r.Returns,
		Return{
			ID: "RET-0002", ProductID: chairs,
			Allocations:  []Allocation{{IssueID: "ISS-0008", Quantity: 5}},
			ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
			TakenBackBy: "Imran Sheikh", ReturnedAt: t4Clock, RecordedAt: t4Clock,
		},
		Return{
			ID: "RET-0003", ProductID: tables,
			Allocations:  []Allocation{{IssueID: "ISS-0005", Quantity: 2}},
			ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
			TakenBackBy: "Imran Sheikh", ReturnedAt: t4Clock, RecordedAt: t4Clock,
		},
	)
	if found := FindPeople(r, "98861"); len(found) != 0 {
		t.Errorf("FindPeople still lists %+v", found)
	}
}

// ---------- return refusals ----------

func TestReturnRefusedOverOutstanding(t *testing.T) {
	qe := quantityErr(t, CheckReturn(WalkthroughT2(), []string{"ISS-0003", "ISS-0008"}, 51))
	if qe.Asked != 51 || qe.Allowed != 50 || qe.Field != "return" {
		t.Errorf("QuantityError = %+v, want asked 51 of 50", qe)
	}
}

func TestReturnAllowedAtExactlyOutstanding(t *testing.T) {
	if err := CheckReturn(WalkthroughT2(), []string{"ISS-0003", "ISS-0008"}, 50); err != nil {
		t.Errorf("50 refused: %v", err)
	}
}

func TestPartialReturnThenOverReturnRefused(t *testing.T) {
	r := WalkthroughT3()
	qe := quantityErr(t, CheckReturn(r, []string{"ISS-0008"}, 6))
	if qe.Allowed != 5 {
		t.Errorf("Allowed = %d, want 5", qe.Allowed)
	}
	if err := CheckReturn(r, []string{"ISS-0008"}, 5); err != nil {
		t.Errorf("5 refused: %v", err)
	}
}

func TestReturnRefusedForZero(t *testing.T) {
	if err := CheckReturn(WalkthroughT3(), []string{"ISS-0008"}, 0); err == nil {
		t.Error("a return of nothing was allowed")
	}
}

func TestReturnAgainstFullySettledIssueRefused(t *testing.T) {
	qe := quantityErr(t, CheckReturn(WalkthroughT3(), []string{"ISS-0003"}, 1))
	if qe.Allowed != 0 {
		t.Errorf("Allowed = %d, want 0", qe.Allowed)
	}
}

func TestReturnRefusedForUnknownIssue(t *testing.T) {
	err := CheckReturn(WalkthroughT3(), []string{"ISS-9999"}, 1)
	if !errors.Is(err, ErrUnknownIssue) {
		t.Errorf("error = %v, want ErrUnknownIssue", err)
	}
}

func TestOutstandingOnIssueAcrossTwoReturns(t *testing.T) {
	at := MustTime("2026-09-03T10:00:00+05:30")
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{{ID: chairs, Name: "Chairs", CreatedAt: at}},
		Inwards:       []Inward{{ID: "INW-0001", ProductID: chairs, Quantity: 50, Basis: Rent}},
		Issues: []Issue{issue("ISS-0001", chairs, 50, "Ravi Menon", "Catering", "98861 40023",
			"Suresh Kumar", "98450 22117", at)},
		Returns: []Return{},
	}
	if got := OutstandingOnIssue(r, "ISS-0001"); got != 50 {
		t.Errorf("outstanding = %d, want 50", got)
	}
	r.Returns = append(r.Returns, Return{ID: "RET-0001", ProductID: chairs,
		Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 20}}})
	if got := OutstandingOnIssue(r, "ISS-0001"); got != 30 {
		t.Errorf("outstanding = %d, want 30", got)
	}
	r.Returns = append(r.Returns, Return{ID: "RET-0002", ProductID: chairs,
		Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 30}}})
	if got := OutstandingOnIssue(r, "ISS-0001"); got != 0 {
		t.Errorf("outstanding = %d, want 0", got)
	}
	if err := CheckReturn(r, []string{"ISS-0001"}, 1); err == nil {
		t.Error("one more chair was allowed back")
	}
	if got := OutstandingOnIssue(r, "ISS-9999"); got != 0 {
		t.Errorf("outstanding on an unknown issue = %d, want 0", got)
	}
}

// ---------- shortfall does not move stock ----------

func TestShortfallStaysOutstanding(t *testing.T) {
	wont := WalkthroughT3()
	expected := WalkthroughT3()
	expected.Returns[0].ShortDisposition = ExpectedBack

	for name, r := range map[string]*Register{"wont_return": wont, "expected": expected} {
		if got := OnHand(r, chairs); got != 925 {
			t.Errorf("%s: OnHand = %d, want 925", name, got)
		}
		held := 0
		for _, l := range OutstandingForPerson(r, "Ravi Menon", "98861 40023") {
			if l.ProductID == chairs {
				held += l.Out
			}
		}
		if held != 5 {
			t.Errorf("%s: Ravi holds %d chairs, want 5", name, held)
		}
		row := findSupplierRow(t, SupplierRows(r), "Sharma Tent House", "Chairs", true)
		if row.CameIn != 890 {
			t.Errorf("%s: Sharma chairs came in %d, want 890", name, row.CameIn)
		}
	}

	wontRow := findSupplierRow(t, SupplierRows(wont), "Sharma Tent House", "Chairs", true)
	expRow := findSupplierRow(t, SupplierRows(expected), "Sharma Tent House", "Chairs", true)
	if wontRow.WontComeBack != 5 || expRow.WontComeBack != 0 {
		t.Errorf("WontComeBack = %d and %d, want 5 and 0", wontRow.WontComeBack, expRow.WontComeBack)
	}
}

// ---------- tiles ----------

func TestTilesAtT0(t *testing.T) {
	got := TileCounts(WalkthroughT0(), t0Clock)
	want := Tiles{Products: 5, OutRightNow: 352, PeopleHolding: 4, OutOverTwoDays: 3}
	if got != want {
		t.Errorf("tiles = %+v, want %+v", got, want)
	}
}

func TestOutOverTwoDaysBoundary(t *testing.T) {
	r := WalkthroughT0()
	onTheLine := MustTime("2026-09-03T09:45:00+05:30")
	if got := TileCounts(r, onTheLine).OutOverTwoDays; got != 2 {
		t.Errorf("at the boundary: %d, want 2", got)
	}
	if got := TileCounts(r, onTheLine.Add(time.Minute)).OutOverTwoDays; got != 3 {
		t.Errorf("one minute later: %d, want 3", got)
	}
}

func TestOutOverTwoDaysIgnoresReturnedLines(t *testing.T) {
	if got := TileCounts(WalkthroughT3(), t4Clock).OutOverTwoDays; got != 3 {
		t.Errorf("OutOverTwoDays = %d, want 3", got)
	}
}

// ---------- suppliers ----------

func findSupplierRow(t *testing.T, rows []SupplierRow, supplier, product string, onRent bool) SupplierRow {
	t.Helper()
	for _, r := range rows {
		if r.Supplier == supplier && r.ProductName == product && r.OnRent == onRent {
			return r
		}
	}
	t.Fatalf("no row for %q / %q (rent %v) in %+v", supplier, product, onRent, rows)
	return SupplierRow{}
}

func TestSupplierRowsAtT3(t *testing.T) {
	rows := SupplierRows(WalkthroughT3())
	want := []SupplierRow{
		{Supplier: "Gupta Electricals", ProductName: "Extension boards", OnRent: true, CameIn: 25, WontComeBack: 0},
		{Supplier: "Sharma Tent House", ProductName: "Chairs", OnRent: true, CameIn: 890, WontComeBack: 5},
		{Supplier: "Sharma Tent House", ProductName: "Round tables", OnRent: true, CameIn: 60, WontComeBack: 0},
		{Supplier: "", ProductName: "Chairs", OnRent: false, CameIn: 310, WontComeBack: 0},
		{Supplier: "", ProductName: "Charcoal sacks", OnRent: false, CameIn: 12, WontComeBack: 0},
		{Supplier: "", ProductName: "Water drums (20L)", OnRent: false, CameIn: 40, WontComeBack: 0},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		g := rows[i]
		if g.Supplier != w.Supplier || g.ProductName != w.ProductName || g.OnRent != w.OnRent ||
			g.CameIn != w.CameIn || g.WontComeBack != w.WontComeBack {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestSupplierRowsAtT0(t *testing.T) {
	rows := SupplierRows(WalkthroughT0())
	row := findSupplierRow(t, rows, "Sharma Tent House", "Chairs", true)
	if row.CameIn != 390 {
		t.Errorf("Sharma chairs came in %d, want 390", row.CameIn)
	}
	for _, r := range rows {
		if r.WontComeBack != 0 {
			t.Errorf("row %+v carries a note before any return was recorded", r)
		}
	}
}

func TestSameSupplierBothBasesGetsTwoRows(t *testing.T) {
	r := WalkthroughT3()
	r.Inwards = append(r.Inwards, Inward{
		ID: "INW-0008", ProductID: chairs, Quantity: 20, ReceivedOn: "2026-09-03",
		Basis: Purchase, Supplier: "Sharma Tent House",
	})
	rows := SupplierRows(r)
	rent := findSupplierRow(t, rows, "Sharma Tent House", "Chairs", true)
	bought := findSupplierRow(t, rows, "Sharma Tent House", "Chairs", false)
	if rent.CameIn != 890 || bought.CameIn != 20 {
		t.Errorf("rows read %d on rent and %d bought, want 890 and 20", rent.CameIn, bought.CameIn)
	}
	if bought.WontComeBack != 0 {
		t.Errorf("the bought row carries a note of %d", bought.WontComeBack)
	}
}

func TestWontComeBackShowsOnEveryRentRowOfThatProduct(t *testing.T) {
	r := WalkthroughT3()
	r.Inwards = append(r.Inwards, Inward{
		ID: "INW-0008", ProductID: chairs, Quantity: 100, ReceivedOn: "2026-09-03",
		Basis: Rent, Supplier: "Gupta Electricals",
	})
	rows := SupplierRows(r)
	sharma := findSupplierRow(t, rows, "Sharma Tent House", "Chairs", true)
	gupta := findSupplierRow(t, rows, "Gupta Electricals", "Chairs", true)
	if sharma.WontComeBack != 5 || gupta.WontComeBack != 5 {
		t.Errorf("notes read %d and %d, want 5 on both", sharma.WontComeBack, gupta.WontComeBack)
	}
	if sharma.CameIn != 890 || gupta.CameIn != 100 {
		t.Errorf("came in %d and %d, want 890 and 100", sharma.CameIn, gupta.CameIn)
	}
}

func TestGuptaElectricalsRow(t *testing.T) {
	at := MustTime("2026-09-02T09:00:00+05:30")
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{{ID: boards, Name: "Extension boards", CreatedAt: at}},
		Inwards: []Inward{{
			ID: "INW-0001", ProductID: boards, Quantity: 25, ReceivedOn: "2026-09-02",
			Basis: Rent, Supplier: "Gupta Electricals", ChallanNo: "GE/118",
		}},
		Issues: []Issue{issue("ISS-0001", boards, 25, "Joseph D'Cruz", "Stage & Sound", "90350 66471",
			"Imran Sheikh", "90080 77213", at)},
		Returns: []Return{{
			ID: "RET-0001", ProductID: boards,
			Allocations:   []Allocation{{IssueID: "ISS-0001", Quantity: 22}},
			ReturnerName:  "Joseph D'Cruz",
			ShortQuantity: 3, ShortDisposition: WontComeBack,
			Remark: "3 boards burnt out at the stage panel.",
		}},
	}
	row := findSupplierRow(t, SupplierRows(r), "Gupta Electricals", "Extension boards", true)
	if row.CameIn != 25 || row.WontComeBack != 3 {
		t.Errorf("row = %+v, want 25 in and 3 not coming back", row)
	}
	if got := OnHand(r, boards); got != 22 {
		t.Errorf("OnHand = %d, want 22", got)
	}
	if got := OutWithPeople(r, boards); got != 3 {
		t.Errorf("OutWithPeople = %d, want 3", got)
	}
}

func TestPurchasedStockCarriesNoNote(t *testing.T) {
	r := WalkthroughT3()
	r.Returns = append(r.Returns, Return{
		ID: "RET-0002", ProductID: drums,
		Allocations:   []Allocation{{IssueID: "ISS-0006", Quantity: 3}},
		ReturnerName:  "Farida Begum",
		ShortQuantity: 2, ShortDisposition: WontComeBack,
		Remark: "2 drums cracked.",
	})
	row := findSupplierRow(t, SupplierRows(r), "", "Water drums (20L)", false)
	if row.OnRent || row.WontComeBack != 0 {
		t.Errorf("row = %+v, want bought and no note", row)
	}
}

// ---------- deleted records ----------

func tombstone() *Deletion {
	return &Deletion{
		At: MustTime("2026-09-03T19:00:00+05:30"), By: "Suresh Kumar",
		Reason: "Entered twice by mistake.",
	}
}

func TestDeletedInwardVanishesFromEveryNumber(t *testing.T) {
	r := WalkthroughT0()
	r.Inwards[1].Deleted = tombstone() // INW-0002, 310 purchased chairs

	if got := CameIn(r, chairs); got != 390 {
		t.Errorf("CameIn = %d, want 390", got)
	}
	row := StockRows(r)[0]
	if row.Name != "Chairs" || row.CameIn != 390 || row.Out != 310 || row.OnHand != 80 {
		t.Errorf("Chairs row = %+v, want 390/310/80", row)
	}
	tiles := TileCounts(r, t0Clock)
	if tiles.Products != 5 || tiles.OutRightNow != 352 {
		t.Errorf("tiles = %+v, want 5 products and 352 out", tiles)
	}
	for _, s := range SupplierRows(r) {
		if s.Supplier == "" && s.ProductName == "Chairs" {
			t.Errorf("the deleted inward still has a supplier row: %+v", s)
		}
	}
}

func TestDeletedIssueVanishesFromEveryNumber(t *testing.T) {
	r := WalkthroughT0()
	r.Issues[0].Deleted = tombstone() // ISS-0001, 150 chairs to Lakshmi Iyer

	if got := OutWithPeople(r, chairs); got != 160 {
		t.Errorf("OutWithPeople = %d, want 160", got)
	}
	if got := OnHand(r, chairs); got != 540 {
		t.Errorf("OnHand = %d, want 540", got)
	}
	people := PeopleHolding(r)
	if len(people) != 4 {
		t.Fatalf("got %d people, want 4", len(people))
	}
	var lakshmi PersonSummary
	for _, p := range people {
		if p.Name == "Lakshmi Iyer" {
			lakshmi = p
		}
	}
	if lakshmi.TotalOut != 10 || len(lakshmi.Lines) != 1 || lakshmi.Lines[0].ProductName != "Round tables" {
		t.Errorf("Lakshmi = %+v, want 10 round tables on one line", lakshmi)
	}
	found := FindPeople(r, "Lakshmi")
	if len(found) != 1 || len(found[0].Lines) != 1 || found[0].Lines[0].ProductName != "Round tables" {
		t.Errorf("FindPeople(Lakshmi) = %+v, want her one round tables line", found)
	}
	tiles := TileCounts(r, t0Clock)
	if tiles.OutRightNow != 202 || tiles.PeopleHolding != 4 || tiles.OutOverTwoDays != 2 {
		t.Errorf("tiles = %+v, want 202 out, 4 people, 2 aged lines", tiles)
	}
}

func TestDeletedReturnVanishesFromEveryNumber(t *testing.T) {
	r := WalkthroughT3()
	r.Returns[0].Deleted = tombstone() // RET-0001

	if got := Returned(r, chairs); got != 0 {
		t.Errorf("Returned = %d, want 0", got)
	}
	if got := OnHand(r, chairs); got != 880 {
		t.Errorf("OnHand = %d, want 880", got)
	}
	held := 0
	for _, l := range OutstandingForPerson(r, "Ravi Menon", "98861 40023") {
		if l.ProductID == chairs {
			held += l.Out
		}
	}
	if held != 50 {
		t.Errorf("Ravi holds %d chairs, want 50", held)
	}
	if got := OutstandingOnIssue(r, "ISS-0003"); got != 40 {
		t.Errorf("ISS-0003 outstanding = %d, want 40", got)
	}
	row := findSupplierRow(t, SupplierRows(r), "Sharma Tent House", "Chairs", true)
	if row.WontComeBack != 0 {
		t.Errorf("WontComeBack = %d, want 0: the note dies with the record", row.WontComeBack)
	}
}

func TestDeletedRecordsStayInTheFile(t *testing.T) {
	withInward := WalkthroughT0()
	withInward.Inwards[1].Deleted = tombstone()
	if len(withInward.Inwards) != 6 {
		t.Errorf("%d inwards, want 6", len(withInward.Inwards))
	}

	withIssue := WalkthroughT0()
	withIssue.Issues[0].Deleted = tombstone()
	if len(withIssue.Issues) != 7 {
		t.Errorf("%d issues, want 7", len(withIssue.Issues))
	}

	withReturn := WalkthroughT3()
	withReturn.Returns[0].Deleted = tombstone()
	if len(withReturn.Returns) != 1 {
		t.Errorf("%d returns, want 1", len(withReturn.Returns))
	}

	for _, d := range []*Deletion{
		withInward.Inwards[1].Deleted, withIssue.Issues[0].Deleted, withReturn.Returns[0].Deleted,
	} {
		if d.By != "Suresh Kumar" || d.Reason != "Entered twice by mistake." {
			t.Errorf("tombstone = %+v, want it readable", d)
		}
	}
}

// ---------- validate ----------

func TestValidateIsCleanForEveryFixtureTimepoint(t *testing.T) {
	points := map[string]*Register{
		"T0": WalkthroughT0(), "T1": WalkthroughT1(),
		"T2": WalkthroughT2(), "T3": WalkthroughT3(),
	}
	for name, r := range points {
		if problems := Validate(r); len(problems) != 0 {
			t.Errorf("%s: %+v", name, problems)
		}
	}
}

func TestValidateCatchesNegativeOnHand(t *testing.T) {
	at := MustTime("2026-09-03T10:00:00+05:30")
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{{ID: chairs, Name: "Chairs", CreatedAt: at}},
		Inwards:       []Inward{{ID: "INW-0001", ProductID: chairs, Quantity: 10, Basis: Rent}},
		Issues: []Issue{
			issue("ISS-0001", chairs, 10, "Ravi Menon", "Catering", "98861 40023", "Suresh Kumar", "98450 22117", at),
			issue("ISS-0002", chairs, 10, "Ravi Menon", "Catering", "98861 40023", "Suresh Kumar", "98450 22117", at),
		},
		Returns: []Return{{
			ID: "RET-0001", ProductID: chairs,
			Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 10}},
			Deleted:     tombstone(),
		}},
	}
	problems := Validate(r)
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1: %+v", len(problems), problems)
	}
	p := problems[0]
	if p.Kind != NegativeOnHand || p.ProductName != "Chairs" || p.Have != -10 || p.Want != 0 {
		t.Errorf("problem = %+v, want Chairs on hand -10", p)
	}
}

func TestValidateCatchesNegativeOut(t *testing.T) {
	at := MustTime("2026-09-03T10:00:00+05:30")
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{{ID: chairs, Name: "Chairs", CreatedAt: at}},
		Inwards:       []Inward{{ID: "INW-0001", ProductID: chairs, Quantity: 100, Basis: Rent}},
		Issues: []Issue{issue("ISS-0001", chairs, 5, "Ravi Menon", "Catering", "98861 40023",
			"Suresh Kumar", "98450 22117", at)},
		Returns: []Return{
			{ID: "RET-0001", ProductID: chairs, Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 5}}},
			{ID: "RET-0002", ProductID: chairs, Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 5}}},
		},
	}
	found := false
	for _, p := range Validate(r) {
		if p.Kind == NegativeOut && p.ProductName == "Chairs" && p.Have == -5 {
			found = true
		}
	}
	if !found {
		t.Errorf("no NegativeOut problem in %+v", Validate(r))
	}
}

func TestValidateCatchesOverAllocatedIssue(t *testing.T) {
	at := MustTime("2026-09-03T10:00:00+05:30")
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{{ID: chairs, Name: "Chairs", CreatedAt: at}},
		Inwards:       []Inward{{ID: "INW-0001", ProductID: chairs, Quantity: 100, Basis: Rent}},
		Issues: []Issue{
			issue("ISS-0001", chairs, 40, "Ravi Menon", "Catering", "98861 40023", "Suresh Kumar", "98450 22117", at),
			issue("ISS-0002", chairs, 20, "Lakshmi Iyer", "Kitchen", "99860 11204", "Suresh Kumar", "98450 22117", at),
		},
		Returns: []Return{
			{ID: "RET-0001", ProductID: chairs, Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 30}}},
			{ID: "RET-0002", ProductID: chairs, Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 20}}},
		},
	}
	problems := Validate(r)
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1: %+v", len(problems), problems)
	}
	p := problems[0]
	if p.Kind != OverAllocatedIssue || p.IssueID != "ISS-0001" || p.Have != 50 || p.Want != 40 {
		t.Errorf("problem = %+v, want ISS-0001 over-allocated 50 against 40", p)
	}
}
