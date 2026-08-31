package register

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// No function in this file may reach for the clock. Every function that can depend
// on the time takes now as a parameter, so every figure can be proved by a test.

// ErrUnknownProduct is returned when a product ID names nothing in the register.
var ErrUnknownProduct = errors.New("no such product")

// ErrUnknownIssue is returned when an issue ID names nothing in the register.
var ErrUnknownIssue = errors.New("no such issue")

// LiveInwards returns the inwards that are not tombstoned.
func LiveInwards(r *Register) []Inward {
	live := make([]Inward, 0, len(r.Inwards))
	for i := 0; i < len(r.Inwards); i++ {
		if r.Inwards[i].Deleted == nil {
			live = append(live, r.Inwards[i])
		}
	}
	return live
}

// LiveIssues returns the issues that are not tombstoned.
func LiveIssues(r *Register) []Issue {
	live := make([]Issue, 0, len(r.Issues))
	for i := 0; i < len(r.Issues); i++ {
		if r.Issues[i].Deleted == nil {
			live = append(live, r.Issues[i])
		}
	}
	return live
}

// LiveReturns returns the returns that are not tombstoned.
func LiveReturns(r *Register) []Return {
	live := make([]Return, 0, len(r.Returns))
	for i := 0; i < len(r.Returns); i++ {
		if r.Returns[i].Deleted == nil {
			live = append(live, r.Returns[i])
		}
	}
	return live
}

// CameIn is how many of a product were ever received.
func CameIn(r *Register, productID string) int {
	total := 0
	for _, in := range LiveInwards(r) {
		if in.ProductID == productID {
			total += in.Quantity
		}
	}
	return total
}

// Returned is how many of a product have come back.
func Returned(r *Register, productID string) int {
	total := 0
	for _, re := range LiveReturns(r) {
		if re.ProductID == productID {
			total += re.Quantity()
		}
	}
	return total
}

// OutWithPeople is how many of a product are with people right now. A shortfall
// never enters this figure: five chairs short are five chairs still out.
func OutWithPeople(r *Register, productID string) int {
	issued := 0
	for _, is := range LiveIssues(r) {
		if is.ProductID == productID {
			issued += is.Quantity
		}
	}
	return issued - Returned(r, productID)
}

// OnHand is how many of a product are in the store.
func OnHand(r *Register, productID string) int {
	return CameIn(r, productID) - OutWithPeople(r, productID)
}

// StockRow is one line of the stock table.
type StockRow struct {
	ProductID string
	Name      string
	Basis     Basis // Rent if any inward for the product is rent, else Purchase
	CameIn    int
	Out       int
	OnHand    int
}

// StockRows is the stock table, sorted by name A to Z. A product with no
// inwards still gets a row, with three zeros.
func StockRows(r *Register) []StockRow {
	rent := map[string]bool{}
	for _, in := range LiveInwards(r) {
		if in.Basis == Rent {
			rent[in.ProductID] = true
		}
	}

	rows := make([]StockRow, 0, len(r.Products))
	for _, p := range r.Products {
		basis := Purchase
		if rent[p.ID] {
			basis = Rent
		}
		rows = append(rows, StockRow{
			ProductID: p.ID,
			Name:      p.Name,
			Basis:     basis,
			CameIn:    CameIn(r, p.ID),
			Out:       OutWithPeople(r, p.ID),
			OnHand:    OnHand(r, p.ID),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := FoldKey(rows[i].Name), FoldKey(rows[j].Name)
		if a != b {
			return a < b
		}
		return rows[i].ProductID < rows[j].ProductID
	})
	return rows
}

// OutstandingOnIssue is how many of an issue line are still out.
func OutstandingOnIssue(r *Register, issueID string) int {
	out := 0
	for _, is := range LiveIssues(r) {
		if is.ID == issueID {
			out = is.Quantity
		}
	}
	return out - allocatedTo(r, issueID)
}

func allocatedTo(r *Register, issueID string) int {
	total := 0
	for _, re := range LiveReturns(r) {
		for _, a := range re.Allocations {
			if a.IssueID == issueID {
				total += a.Quantity
			}
		}
	}
	return total
}

// PersonID is how the program tells one person from another: a full name and a
// mobile number together. Two people called Ravi Kumar with different mobiles
// are two people. Where no mobile was recorded, the name alone is the key.
type PersonID struct {
	NameKey   string
	MobileKey string
}

// PersonOf is the key for a name and a mobile as they were typed.
func PersonOf(name, mobile string) PersonID {
	return PersonID{NameKey: FoldKey(name), MobileKey: MobileKey(mobile)}
}

// OutstandingLine is one issue a person has not fully handed back.
type OutstandingLine struct {
	IssueID     string
	ProductID   string
	ProductName string
	Taken       int
	Back        int
	Out         int // Taken - Back, always > 0
	IssuedAt    time.Time
	IssuedBy    string // Issue.PersonInchargeName
}

// OutstandingForPerson is what one person is still holding, oldest first.
func OutstandingForPerson(r *Register, name, mobile string) []OutstandingLine {
	want := PersonOf(name, mobile)
	names := productNames(r)

	var lines []OutstandingLine
	for _, is := range LiveIssues(r) {
		if PersonOf(is.TakerName, is.TakerMobile) != want {
			continue
		}
		back := allocatedTo(r, is.ID)
		if is.Quantity-back <= 0 {
			continue
		}
		lines = append(lines, OutstandingLine{
			IssueID:     is.ID,
			ProductID:   is.ProductID,
			ProductName: names[is.ProductID],
			Taken:       is.Quantity,
			Back:        back,
			Out:         is.Quantity - back,
			IssuedAt:    is.IssuedAt,
			IssuedBy:    is.PersonInchargeName,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		if !lines[i].IssuedAt.Equal(lines[j].IssuedAt) {
			return lines[i].IssuedAt.Before(lines[j].IssuedAt)
		}
		return lines[i].IssueID < lines[j].IssueID
	})
	return lines
}

func productNames(r *Register) map[string]string {
	names := make(map[string]string, len(r.Products))
	for _, p := range r.Products {
		names[p.ID] = p.Name
	}
	return names
}

// PersonSummary is everything one person is holding.
type PersonSummary struct {
	ID         PersonID
	Name       string // most recent spelling used on an issue
	Department string // from the most recent issue
	Mobile     string // from the most recent issue, shown exactly as typed
	TotalOut   int
	Lines      []OutstandingLine
}

// PeopleHolding is everybody holding something, by name A to Z, ties broken by
// mobile so two people of the same name have a stable order.
func PeopleHolding(r *Register) []PersonSummary {
	latest := map[PersonID]Issue{}
	var order []PersonID
	for _, is := range LiveIssues(r) {
		id := PersonOf(is.TakerName, is.TakerMobile)
		prev, seen := latest[id]
		if !seen {
			order = append(order, id)
			latest[id] = is
			continue
		}
		if is.IssuedAt.After(prev.IssuedAt) || (is.IssuedAt.Equal(prev.IssuedAt) && is.ID > prev.ID) {
			latest[id] = is
		}
	}

	var people []PersonSummary
	for _, id := range order {
		newest := latest[id]
		lines := OutstandingForPerson(r, newest.TakerName, newest.TakerMobile)
		total := 0
		for _, l := range lines {
			total += l.Out
		}
		if total == 0 {
			continue
		}
		people = append(people, PersonSummary{
			ID:         id,
			Name:       newest.TakerName,
			Department: newest.TakerDepartment,
			Mobile:     newest.TakerMobile,
			TotalOut:   total,
			Lines:      lines,
		})
	}
	sort.Slice(people, func(i, j int) bool {
		a, b := FoldKey(people[i].Name), FoldKey(people[j].Name)
		if a != b {
			return a < b
		}
		return people[i].ID.MobileKey < people[j].ID.MobileKey
	})
	return people
}

// FindPeople is the one person-finder in the program. One typed string finds a
// person by part of a name, part of a mobile or part of a department, so nobody
// has to know which box to use. An empty query returns everybody holding something.
func FindPeople(r *Register, query string) []PersonSummary {
	people := PeopleHolding(r)
	text := FoldKey(query)
	if text == "" {
		return people
	}
	digits := MobileKey(query)

	var found []PersonSummary
	for _, p := range people {
		switch {
		case strings.Contains(FoldKey(p.Name), text),
			strings.Contains(FoldKey(p.Department), text),
			digits != "" && strings.Contains(MobileKey(p.Mobile), digits):
			found = append(found, p)
		}
	}
	return found
}

// Tiles are the four counts on the home screen.
type Tiles struct {
	Products       int
	OutRightNow    int
	PeopleHolding  int
	OutOverTwoDays int
}

// TileCounts counts the home screen tiles. OutOverTwoDays counts lines, not
// items, and counts a line if any quantity on it is still out.
func TileCounts(r *Register, now time.Time) Tiles {
	t := Tiles{Products: len(r.Products), PeopleHolding: len(PeopleHolding(r))}
	for _, p := range r.Products {
		t.OutRightNow += OutWithPeople(r, p.ID)
	}
	cutoff := now.Add(-48 * time.Hour)
	for _, is := range LiveIssues(r) {
		if is.IssuedAt.Before(cutoff) && OutstandingOnIssue(r, is.ID) > 0 {
			t.OutOverTwoDays++
		}
	}
	return t
}

// SupplierRow is one line of the suppliers record: what came in from whom.
// There is no debt in this program, so there is no field that could hold one.
type SupplierRow struct {
	Supplier     string // "" means no supplier was recorded on those inwards
	ProductID    string
	ProductName  string
	OnRent       bool
	CameIn       int
	WontComeBack int
}

// SupplierRows is the suppliers record: rent rows first by supplier then
// product, then the bought rows by product. WontComeBack is a note on the
// record, not a debt: it is shown on every rent row for that product.
func SupplierRows(r *Register) []SupplierRow {
	type key struct {
		supplier  string
		productID string
		onRent    bool
	}
	names := productNames(r)
	totals := map[key]int{}
	var order []key
	for _, in := range LiveInwards(r) {
		k := key{supplier: in.Supplier, productID: in.ProductID, onRent: in.Basis == Rent}
		if _, seen := totals[k]; !seen {
			order = append(order, k)
		}
		totals[k] += in.Quantity
	}

	lost := map[string]int{}
	for _, re := range LiveReturns(r) {
		if re.ShortDisposition == WontComeBack {
			lost[re.ProductID] += re.ShortQuantity
		}
	}

	rows := make([]SupplierRow, 0, len(order))
	for _, k := range order {
		row := SupplierRow{
			Supplier:    k.supplier,
			ProductID:   k.productID,
			ProductName: names[k.productID],
			OnRent:      k.onRent,
			CameIn:      totals[k],
		}
		if k.onRent {
			row.WontComeBack = lost[k.productID]
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.OnRent != b.OnRent {
			return a.OnRent
		}
		if a.OnRent {
			if s1, s2 := FoldKey(a.Supplier), FoldKey(b.Supplier); s1 != s2 {
				return s1 < s2
			}
			return FoldKey(a.ProductName) < FoldKey(b.ProductName)
		}
		if p1, p2 := FoldKey(a.ProductName), FoldKey(b.ProductName); p1 != p2 {
			return p1 < p2
		}
		return FoldKey(a.Supplier) < FoldKey(b.Supplier)
	})
	return rows
}

// ProblemKind says which invariant a Problem breaks.
type ProblemKind int

const (
	NegativeOnHand ProblemKind = iota
	NegativeOut
	OverAllocatedIssue
)

// Problem is one broken invariant, with the two numbers that disagree.
type Problem struct {
	ProductID   string
	ProductName string
	Kind        ProblemKind
	IssueID     string // set only for OverAllocatedIssue
	Have, Want  int
}

// Validate is the one invariant checker. It returns an empty slice for every
// register this program can produce through the three entry flows; the
// correction screens apply a change to a copy and ask this function whether it
// is allowed, rather than re-deriving each field's limit by hand.
func Validate(r *Register) []Problem {
	names := productNames(r)
	problems := []Problem{}

	for _, p := range r.Products {
		if onHand := OnHand(r, p.ID); onHand < 0 {
			problems = append(problems, Problem{
				ProductID: p.ID, ProductName: p.Name,
				Kind: NegativeOnHand, Have: onHand, Want: 0,
			})
		}
		if out := OutWithPeople(r, p.ID); out < 0 {
			problems = append(problems, Problem{
				ProductID: p.ID, ProductName: p.Name,
				Kind: NegativeOut, Have: out, Want: 0,
			})
		}
	}

	for _, is := range LiveIssues(r) {
		if allocated := allocatedTo(r, is.ID); allocated > is.Quantity {
			problems = append(problems, Problem{
				ProductID: is.ProductID, ProductName: names[is.ProductID],
				Kind: OverAllocatedIssue, IssueID: is.ID,
				Have: allocated, Want: is.Quantity,
			})
		}
	}
	return problems
}

// QuantityError is a refused quantity. The sentence shown to the person at the
// desk is built by the flow that raised it, not here.
type QuantityError struct {
	Field       string // "issue" | "return"
	Asked       int
	Allowed     int
	ProductName string
}

func (e QuantityError) Error() string {
	return fmt.Sprintf("%s of %d %s refused: %d is the most", e.Field, e.Asked, e.ProductName, e.Allowed)
}

// CheckIssue refuses an issue of a product that does not exist, of nothing, or
// of more than is on hand. now is not read today; it is a parameter because no
// function in this package may reach for the clock itself.
func CheckIssue(r *Register, productID string, qty int, now time.Time) error {
	name, ok := productNames(r)[productID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownProduct, productID)
	}
	allowed := OnHand(r, productID)
	if qty < 1 || qty > allowed {
		return QuantityError{Field: "issue", Asked: qty, Allowed: allowed, ProductName: name}
	}
	return nil
}

// CheckReturn refuses a return against an unknown issue, of nothing, or of more
// than those issues still have out.
func CheckReturn(r *Register, issueIDs []string, qty int) error {
	names := productNames(r)
	productName := ""
	allowed := 0
	for _, id := range issueIDs {
		found := false
		for _, is := range LiveIssues(r) {
			if is.ID == id {
				found = true
				if productName == "" {
					productName = names[is.ProductID]
				}
			}
		}
		if !found {
			return fmt.Errorf("%w: %s", ErrUnknownIssue, id)
		}
		allowed += OutstandingOnIssue(r, id)
	}
	if qty < 1 || qty > allowed {
		return QuantityError{Field: "return", Asked: qty, Allowed: allowed, ProductName: productName}
	}
	return nil
}

// AllocatedFromLiveReturns is how much of one issue live returns have put back
// against it. A deleted return holds nothing, so the issue it once blocked can
// be deleted after it. The correction screens need this figure by name: see the
// note on the delete guard in internal/web/corrections.go.
func AllocatedFromLiveReturns(r *Register, issueID string) int {
	return allocatedTo(r, issueID)
}
