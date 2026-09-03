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
		if r.Inwards[i].Deleted == nil && productExists(r, r.Inwards[i].ProductID) {
			live = append(live, r.Inwards[i])
		}
	}
	return live
}

// LiveIssues returns the issues that are not tombstoned.
func LiveIssues(r *Register) []Issue {
	live := make([]Issue, 0, len(r.Issues))
	for i := 0; i < len(r.Issues); i++ {
		if r.Issues[i].Deleted == nil && productExists(r, r.Issues[i].ProductID) {
			live = append(live, r.Issues[i])
		}
	}
	return live
}

// LiveReturns returns the returns that are not tombstoned.
func LiveReturns(r *Register) []Return {
	live := make([]Return, 0, len(r.Returns))
	for i := 0; i < len(r.Returns); i++ {
		if r.Returns[i].Deleted == nil && productExists(r, r.Returns[i].ProductID) {
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

// LiveDisposals are the stock removals still in force.
func LiveDisposals(r *Register) []InventoryDisposal {
	out := []InventoryDisposal{}
	for _, d := range r.Disposals {
		if d.InactiveAt == nil {
			out = append(out, d)
		}
	}
	return out
}

// Disposed is how many of a product have left the store for good.
func Disposed(r *Register, productID string) int {
	total := 0
	for _, d := range LiveDisposals(r) {
		if d.ProductID == productID {
			total += d.Quantity
		}
	}
	return total
}

// OnHand is how many of a product are in the store.
func OnHand(r *Register, productID string) int {
	return CameIn(r, productID) - OutWithPeople(r, productID) - Disposed(r, productID)
}

// StockRow is one line of the stock table.
type StockRow struct {
	ProductID string
	Name      string
	// Basis is Rent if any inward for the product is rent; otherwise the
	// typed kind of the first such delivery if there is one; otherwise
	// Purchase. Stock is pooled, so a product that arrived two ways can only
	// show one word, and rent is shown first because those goods are owed.
	Basis  Basis
	KindID string // set only when Basis is Other
	CameIn int
	Out    int
	OnHand int
}

// sortedByID orders deliveries by their identifier so a choice made from
// several of them is the same every time the page is drawn.
func sortedByID(rows []Inward) []Inward {
	out := append([]Inward{}, rows...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// StockRows is the stock table, sorted by name A to Z. A product with no
// inwards still gets a row, with three zeros.
func StockRows(r *Register) []StockRow {
	rent := map[string]bool{}
	kind := map[string]string{}
	for _, in := range sortedByID(LiveInwards(r)) {
		if in.Basis == Rent {
			rent[in.ProductID] = true
		}
		if in.Basis == Other && in.KindID != "" {
			if _, seen := kind[in.ProductID]; !seen {
				kind[in.ProductID] = in.KindID
			}
		}
	}

	rows := make([]StockRow, 0, len(r.Products))
	for _, p := range r.Products {
		if p.Deleted != nil {
			continue
		}
		basis, kindID := Purchase, ""
		switch {
		case rent[p.ID]:
			basis = Rent
		case kind[p.ID] != "":
			basis, kindID = Other, kind[p.ID]
		}
		rows = append(rows, StockRow{
			ProductID: p.ID,
			Name:      p.Name,
			Basis:     basis,
			KindID:    kindID,
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
	ChallanNo   string
}

// OutstandingForPerson is what one person is still holding, oldest first.
func OutstandingForPerson(r *Register, name, mobile string) []OutstandingLine {
	want := PersonOf(name, mobile)
	names := productNames(r)

	var lines []OutstandingLine
	for _, is := range LiveIssues(r) {
		member := false
		for _, recipient := range RecipientsOf(is) {
			if PersonOf(recipient.Name, recipient.Mobile) == want {
				member = true
				break
			}
		}
		if !member {
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
			ChallanNo:   is.ChallanNo,
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

// JointHolding is one selectable responsibility context. Solo issues for the
// same person are combined as before; every multi-person issue stands alone.
type JointHolding struct {
	AnchorIssueID string
	Recipients    []IssueRecipient
	Label         string
	TotalOut      int
	Lines         []OutstandingLine
}

// JointHoldings returns every live outstanding holding without duplicating a
// joint quantity for each of its members.
func JointHoldings(r *Register) []JointHolding {
	names := productNames(r)
	soloAt := map[PersonID]int{}
	var holdings []JointHolding
	for _, is := range LiveIssues(r) {
		out := OutstandingOnIssue(r, is.ID)
		if out <= 0 {
			continue
		}
		line := OutstandingLine{IssueID: is.ID, ProductID: is.ProductID, ProductName: names[is.ProductID], Taken: is.Quantity, Back: is.Quantity - out, Out: out, IssuedAt: is.IssuedAt, IssuedBy: is.PersonInchargeName, ChallanNo: is.ChallanNo}
		recipients := RecipientsOf(is)
		if len(recipients) > 1 {
			holdings = append(holdings, JointHolding{AnchorIssueID: is.ID, Recipients: recipients, Label: RecipientLabel(is), TotalOut: out, Lines: []OutstandingLine{line}})
			continue
		}
		id := PersonOf(is.TakerName, is.TakerMobile)
		if at, ok := soloAt[id]; ok {
			holdings[at].TotalOut += out
			holdings[at].Lines = append(holdings[at].Lines, line)
			continue
		}
		soloAt[id] = len(holdings)
		holdings = append(holdings, JointHolding{AnchorIssueID: is.ID, Recipients: recipients, Label: RecipientLabel(is), TotalOut: out, Lines: []OutstandingLine{line}})
	}
	for i := range holdings {
		sort.Slice(holdings[i].Lines, func(a, b int) bool {
			if !holdings[i].Lines[a].IssuedAt.Equal(holdings[i].Lines[b].IssuedAt) {
				return holdings[i].Lines[a].IssuedAt.Before(holdings[i].Lines[b].IssuedAt)
			}
			return holdings[i].Lines[a].IssueID < holdings[i].Lines[b].IssueID
		})
		holdings[i].AnchorIssueID = holdings[i].Lines[0].IssueID
	}
	sort.Slice(holdings, func(i, j int) bool {
		if a, b := FoldKey(holdings[i].Label), FoldKey(holdings[j].Label); a != b {
			return a < b
		}
		if !holdings[i].Lines[0].IssuedAt.Equal(holdings[j].Lines[0].IssuedAt) {
			return holdings[i].Lines[0].IssuedAt.Before(holdings[j].Lines[0].IssuedAt)
		}
		return holdings[i].AnchorIssueID < holdings[j].AnchorIssueID
	})
	return holdings
}

// FindJointHoldings finds each holding once when any recipient matches.
func FindJointHoldings(r *Register, query string) []JointHolding {
	text, digits := FoldKey(query), MobileKey(query)
	var found []JointHolding
	for _, holding := range JointHoldings(r) {
		match := text == ""
		for _, recipient := range holding.Recipients {
			if strings.Contains(FoldKey(recipient.Name), text) || strings.Contains(FoldKey(recipient.Department), text) || (digits != "" && strings.Contains(MobileKey(recipient.Mobile), digits)) {
				match = true
			}
		}
		if match {
			found = append(found, holding)
		}
	}
	return found
}

// JointHoldingForIssue re-derives a selectable holding from its anchor.
func JointHoldingForIssue(r *Register, issueID string) (JointHolding, bool) {
	for _, holding := range JointHoldings(r) {
		if holding.AnchorIssueID == issueID {
			return holding, true
		}
	}
	return JointHolding{}, false
}

func productNames(r *Register) map[string]string {
	names := make(map[string]string, len(r.Products))
	for _, p := range r.Products {
		if p.Deleted != nil {
			continue
		}
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
	type latestRecipient struct {
		issue     Issue
		recipient IssueRecipient
	}
	latest := map[PersonID]latestRecipient{}
	var order []PersonID
	for _, is := range LiveIssues(r) {
		for _, recipient := range RecipientsOf(is) {
			id := PersonOf(recipient.Name, recipient.Mobile)
			prev, seen := latest[id]
			if !seen {
				order = append(order, id)
			}
			if !seen || is.IssuedAt.After(prev.issue.IssuedAt) || (is.IssuedAt.Equal(prev.issue.IssuedAt) && is.ID > prev.issue.ID) {
				latest[id] = latestRecipient{issue: is, recipient: recipient}
			}
		}
	}

	var people []PersonSummary
	for _, id := range order {
		newest := latest[id]
		lines := OutstandingForPerson(r, newest.recipient.Name, newest.recipient.Mobile)
		total := 0
		for _, l := range lines {
			total += l.Out
		}
		if total == 0 {
			continue
		}
		people = append(people, PersonSummary{
			ID:         id,
			Name:       newest.recipient.Name,
			Department: newest.recipient.Department,
			Mobile:     newest.recipient.Mobile,
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
	t := Tiles{PeopleHolding: len(PeopleHolding(r))}
	for _, p := range r.Products {
		if p.Deleted != nil {
			continue
		}
		t.Products++
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
	OnRent       bool  // Basis == Rent, kept because it is what the screen asks
	Basis        Basis //
	KindID       string
	CameIn       int
	WontComeBack int
}

// SupplierRows is the suppliers record: rent rows first by supplier then
// product, then each typed kind, then the bought rows by product. Two typed
// kinds never share a row: donated chairs and sponsored chairs from one
// supplier are two lines. WontComeBack is a note on the record, not a debt: it
// is shown on every rent row for that product.
func SupplierRows(r *Register) []SupplierRow {
	type key struct {
		supplier  string
		productID string
		basis     Basis
		kindID    string
	}
	names := productNames(r)
	totals := map[key]int{}
	var order []key
	for _, in := range LiveInwards(r) {
		k := key{supplier: in.Supplier, productID: in.ProductID, basis: in.Basis, kindID: in.KindID}
		if k.basis != Other {
			k.kindID = ""
		}
		if k.basis != Rent && k.basis != Other {
			k.basis = Purchase
		}
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
			OnRent:      k.basis == Rent,
			Basis:       k.basis,
			KindID:      k.kindID,
			CameIn:      totals[k],
		}
		if k.basis == Rent {
			row.WontComeBack = lost[k.productID]
		}
		rows = append(rows, row)
	}
	// Rent first because those goods are owed, then each typed kind together
	// under its own word, then what was bought.
	rank := func(row SupplierRow) int {
		switch row.Basis {
		case Rent:
			return 0
		case Other:
			return 1
		default:
			return 2
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if a.Basis == Rent {
			if s1, s2 := FoldKey(a.Supplier), FoldKey(b.Supplier); s1 != s2 {
				return s1 < s2
			}
			return FoldKey(a.ProductName) < FoldKey(b.ProductName)
		}
		if a.Basis == Other {
			if w1, w2 := kindSortKey(r, a.Basis, a.KindID), kindSortKey(r, b.Basis, b.KindID); w1 != w2 {
				return w1 < w2
			}
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
	// StrandedDisposal is stock that already left the store being attributed
	// to an inward that can no longer account for it.
	StrandedDisposal
)

// ErrStrandedDisposal is what the correction screens say when an inward
// cannot be changed because some of that stock has already gone. It is
// deliberately neutral: the public route cannot decrypt whether the protected
// allocation was a sale or a return to a supplier.
const ErrStrandedDisposal = "Some of this stock has already left the store. Fix that return or sale first."

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
		if p.Deleted != nil {
			continue
		}
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

	// Every live stock removal must still be attributable to inwards that can
	// account for it. An inward corrected downwards, deleted, or moved to
	// another basis or supplier would otherwise strand what already left.
	live := map[string]Inward{}
	for _, in := range LiveInwards(r) {
		live[in.ID] = in
	}
	taken := map[string]int{}
	for _, d := range LiveDisposals(r) {
		if _, ok := ProductByID(r, d.ProductID); !ok {
			problems = append(problems, Problem{
				ProductID: d.ProductID, ProductName: names[d.ProductID],
				Kind: StrandedDisposal, Have: d.Quantity, Want: 0,
			})
			continue
		}
		if d.Quantity < 1 || d.Quantity != disposalSum(d) {
			problems = append(problems, Problem{
				ProductID: d.ProductID, ProductName: names[d.ProductID],
				Kind: StrandedDisposal, Have: disposalSum(d), Want: d.Quantity,
			})
			continue
		}
		for _, a := range d.Sources {
			in, ok := live[a.InwardID]
			if !ok || in.ProductID != d.ProductID || a.Quantity < 1 {
				problems = append(problems, Problem{
					ProductID: d.ProductID, ProductName: names[d.ProductID],
					Kind: StrandedDisposal, Have: a.Quantity, Want: 0,
				})
				continue
			}
			taken[a.InwardID] += a.Quantity
		}
	}
	for id, total := range taken {
		if in := live[id]; total > in.Quantity {
			problems = append(problems, Problem{
				ProductID: in.ProductID, ProductName: names[in.ProductID],
				Kind: StrandedDisposal, Have: total, Want: in.Quantity,
			})
		}
	}
	return problems
}

func disposalSum(d InventoryDisposal) int {
	total := 0
	for _, a := range d.Sources {
		total += a.Quantity
	}
	return total
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
	p, ok := ProductByID(r, productID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownProduct, productID)
	}
	name := p.Name
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
