package register

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FinanceValueKind names one of the three reusable suggestion lists. Every
// party, purpose and payment mode a financial user types becomes a shared
// suggestion for every other financial user, so the lists are data, not code.
type FinanceValueKind string

const (
	FinanceParty   FinanceValueKind = "party"
	FinancePurpose FinanceValueKind = "purpose"
	FinanceMode    FinanceValueKind = "mode"
)

// FinanceChange is one audited field correction. It mirrors Change on the
// inventory side but carries the immutable account identity as well as the
// name, because a financial actor must stay identifiable after a rename.
type FinanceChange struct {
	At          time.Time `json:"at"`
	ByAccountID string    `json:"byAccountId"`
	ByName      string    `json:"byName"`
	ByMobile    string    `json:"byMobile"`
	Field       string    `json:"field"`
	Label       string    `json:"label"`
	From        string    `json:"from"`
	To          string    `json:"to"`
}

// FinanceReusableValue is one remembered party, purpose or payment mode.
// A merged value keeps its row so old records still resolve through it.
type FinanceReusableValue struct {
	ID           string           `json:"id"` // PTY-0001 | PUR-0001 | PMD-0001
	Kind         FinanceValueKind `json:"kind"`
	Value        string           `json:"value"`
	CreatedAt    time.Time        `json:"createdAt"`
	CreatedByID  string           `json:"createdById"`
	Changes      []FinanceChange  `json:"changes,omitempty"`
	MergedIntoID string           `json:"mergedIntoId,omitempty"`
}

// FinanceOrderLine is one product on an order: intent, never inventory.
// ProductNameSnapshot is the historical label and is never rewritten by a
// later rename of the live product.
type FinanceOrderLine struct {
	ID                  string `json:"id"` // OLN-0001, unique across the vault
	ProductID           string `json:"productId"`
	ProductNameSnapshot string `json:"productNameSnapshot"`
	ExpectedQuantity    int    `json:"expectedQuantity"`
	Basis               Basis  `json:"basis"`
	KindID              string `json:"kindId,omitempty"` // AKD-0001, only when Basis is Other
}

// FinanceOrder is what was ordered from one party, before anything arrives.
// No quantity here ever reaches stock arithmetic.
type FinanceOrder struct {
	ID          string             `json:"id"` // ORD-0001
	PartyID     string             `json:"partyId"`
	Lines       []FinanceOrderLine `json:"lines"`
	AgreedPaise *int64             `json:"agreedPaise,omitempty"`
	AgreedKind  string             `json:"agreedKind,omitempty"` // estimated | exact
	OrderedAt   time.Time          `json:"orderedAt"`
	Status      string             `json:"status"` // open | cancelled
	Remarks     string             `json:"remarks,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	CreatedByID string             `json:"createdById"`
	Changes     []FinanceChange    `json:"changes,omitempty"`
}

// ValueKindPrefix is the ID prefix each list uses.
func ValueKindPrefix(kind FinanceValueKind) string {
	switch kind {
	case FinanceParty:
		return "PTY"
	case FinancePurpose:
		return "PUR"
	case FinanceMode:
		return "PMD"
	}
	return ""
}

// ValueKindLabel is the wording a correction shows for each list.
func ValueKindLabel(kind FinanceValueKind) string {
	switch kind {
	case FinanceParty:
		return "Party"
	case FinancePurpose:
		return "Purpose"
	case FinanceMode:
		return "Payment mode"
	}
	return ""
}

// InitialPaymentModes is the list every new vault starts with, in this order.
var InitialPaymentModes = []string{"Cash", "UPI", "Bank transfer", "Cheque", "Card"}

// ErrValueUsed is the refusal when an admin tries to delete a value that a
// record still points at. Used values are corrected or merged, never erased.
var ErrValueUsed = errors.New("This value has been used. Rename it or merge it instead.")

// ErrLineUsed is the refusal when an order line a ledger entry points at would
// be removed or repointed.
var ErrLineUsed = errors.New("This product is already used by a ledger entry.")

// FinanceValueByID returns the value with this ID whether or not it is merged.
func FinanceValueByID(f *FinanceData, id string) (FinanceReusableValue, bool) {
	for _, v := range f.ReusableValues {
		if v.ID == id {
			return v, true
		}
	}
	return FinanceReusableValue{}, false
}

// ResolveFinanceValue follows a merge chain to the value records should now
// display. A chain longer than the whole list is a cycle and resolves to
// nothing, which ValidateFinance refuses to store in the first place.
func ResolveFinanceValue(f *FinanceData, id string) (FinanceReusableValue, bool) {
	seen := 0
	for id != "" {
		v, ok := FinanceValueByID(f, id)
		if !ok {
			return FinanceReusableValue{}, false
		}
		if v.MergedIntoID == "" {
			return v, true
		}
		id = v.MergedIntoID
		seen++
		if seen > len(f.ReusableValues) {
			return FinanceReusableValue{}, false
		}
	}
	return FinanceReusableValue{}, false
}

// FinanceValueText is the display text for an ID, "" when it resolves to
// nothing. Records store the ID they were saved with; the screen shows where
// that ID now points.
func FinanceValueText(f *FinanceData, id string) string {
	if v, ok := ResolveFinanceValue(f, id); ok {
		return v.Value
	}
	return ""
}

// LiveFinanceValues are the unmerged values of one kind, alphabetical by fold.
func LiveFinanceValues(f *FinanceData, kind FinanceValueKind) []FinanceReusableValue {
	out := []FinanceReusableValue{}
	for _, v := range f.ReusableValues {
		if v.Kind == kind && v.MergedIntoID == "" {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return FoldKey(out[i].Value) < FoldKey(out[j].Value) })
	return out
}

// MatchFinanceValues is the typeahead: case-insensitive substring, values
// beginning with the query first, alphabetical by fold within each group.
// The caller caps the list; the uncapped list is the no-script fallback.
func MatchFinanceValues(f *FinanceData, kind FinanceValueKind, query string) []FinanceReusableValue {
	q := FoldKey(query)
	rows := []FinanceReusableValue{}
	for _, v := range LiveFinanceValues(f, kind) {
		if q != "" && !strings.Contains(FoldKey(v.Value), q) {
			continue
		}
		rows = append(rows, v)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		fi, fj := FoldKey(rows[i].Value), FoldKey(rows[j].Value)
		pi := q != "" && strings.HasPrefix(fi, q)
		pj := q != "" && strings.HasPrefix(fj, q)
		if pi != pj {
			return pi
		}
		return fi < fj
	})
	return rows
}

// FindFinanceValueByText returns the live value of this kind whose folded text
// is exactly the folded query. An exact match always reuses; it never adds a
// second row saying the same thing.
func FindFinanceValueByText(f *FinanceData, kind FinanceValueKind, text string) (FinanceReusableValue, bool) {
	key := FoldKey(text)
	if key == "" {
		return FinanceReusableValue{}, false
	}
	for _, v := range LiveFinanceValues(f, kind) {
		if FoldKey(v.Value) == key {
			return v, true
		}
	}
	return FinanceReusableValue{}, false
}

// AddFinanceValue appends a new reusable value and returns its ID. It reuses an
// exact folded match instead of duplicating it, so the caller may call this
// unconditionally inside the transaction.
func AddFinanceValue(f *FinanceData, kind FinanceValueKind, text, byAccountID string, at time.Time) (string, error) {
	text = CleanName(text)
	if text == "" {
		return "", errors.New("the value is blank")
	}
	if existing, ok := FindFinanceValueByText(f, kind, text); ok {
		return existing.ID, nil
	}
	id := f.NextID(ValueKindPrefix(kind))
	f.ReusableValues = append(f.ReusableValues, FinanceReusableValue{
		ID: id, Kind: kind, Value: text, CreatedAt: at, CreatedByID: byAccountID,
	})
	return id, nil
}

// FinanceValueIsUsed reports whether any current order or settlement field
// points at this value. Audit and correction text mentioning the old wording
// does not count: that is history, not a live reference.
func FinanceValueIsUsed(f *FinanceData, id string) bool {
	for _, o := range f.Orders {
		if o.PartyID == id {
			return true
		}
	}
	// A voided movement still points at its values: its history has to keep
	// saying what it said.
	for _, m := range f.Movements {
		if m.PartyID == id || m.PurposeID == id || m.ModeID == id {
			return true
		}
	}
	return false
}

// FinanceLineIsReferenced reports whether a ledger entry points at this order
// line. A voided movement does not count: it is a row kept for the record and
// no longer claims anything, so the line it once named is free again.
func FinanceLineIsReferenced(f *FinanceData, lineID string) bool {
	for _, m := range f.Movements {
		if !m.Live() {
			continue
		}
		for _, id := range m.OrderLineIDs {
			if id == lineID {
				return true
			}
		}
	}
	return false
}

// FinanceOrderByID finds one order.
func FinanceOrderByID(f *FinanceData, id string) (FinanceOrder, bool) {
	for _, o := range f.Orders {
		if o.ID == id {
			return o, true
		}
	}
	return FinanceOrder{}, false
}

// SortedFinanceOrders are all orders newest OrderedAt first, ID as tie-break.
func SortedFinanceOrders(f *FinanceData) []FinanceOrder {
	out := append([]FinanceOrder{}, f.Orders...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OrderedAt.Equal(out[j].OrderedAt) {
			return out[i].OrderedAt.After(out[j].OrderedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// MoneyRefusal is the one thing a person is told when an amount will not parse.
const MoneyRefusal = "Type the agreed total in rupees, using up to two decimal places."

// ParseRupees turns typed rupees into positive int64 paise without any float
// arithmetic. Zero, signs, commas, exponents, more than two decimal places and
// anything that overflows are refused: money that is nearly right is worse than
// money that is refused.
func ParseRupees(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New(MoneyRefusal)
	}
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || !allDigits(whole) {
		return 0, errors.New(MoneyRefusal)
	}
	if hasDot {
		if len(frac) == 0 || len(frac) > 2 || !allDigits(frac) {
			return 0, errors.New(MoneyRefusal)
		}
	}
	rupees, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errors.New(MoneyRefusal)
	}
	paise := int64(0)
	switch len(frac) {
	case 0:
	case 1:
		paise = int64(frac[0]-'0') * 10
	case 2:
		paise = int64(frac[0]-'0')*10 + int64(frac[1]-'0')
	}
	// 92233720368547758.07 is the largest amount int64 paise can hold. Reject
	// anything above it before the multiply wraps round to a small positive.
	const maxRupees = (1<<63 - 1) / 100
	if rupees > maxRupees {
		return 0, errors.New(MoneyRefusal)
	}
	total := rupees * 100
	if total > (1<<63-1)-paise {
		return 0, errors.New(MoneyRefusal)
	}
	total += paise
	if total <= 0 {
		return 0, errors.New(MoneyRefusal)
	}
	return total, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatRupees renders paise as ₹ with exactly two decimals and Indian digit
// grouping, using integers only. Specs 19 and 21 write amounts as ₹5,000.00, so
// the grouping is part of the contract, not decoration.
func FormatRupees(paise int64) string {
	sign := ""
	if paise < 0 {
		sign = "-"
		paise = -paise
	}
	return "₹" + sign + groupIndian(paise/100) + "." + twoDigits(paise%100)
}

// groupIndian puts a comma after the last three digits and then after every two:
// 12345678 becomes 1,23,45,678.
func groupIndian(n int64) string {
	d := strconv.FormatInt(n, 10)
	if len(d) <= 3 {
		return d
	}
	head, tail := d[:len(d)-3], d[len(d)-3:]
	var parts []string
	for len(head) > 2 {
		parts = append([]string{head[len(head)-2:]}, parts...)
		head = head[:len(head)-2]
	}
	parts = append([]string{head}, parts...)
	return strings.Join(parts, ",") + "," + tail
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
