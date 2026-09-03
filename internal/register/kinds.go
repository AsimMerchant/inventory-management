package register

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// AcquisitionKind is one word somebody typed for how goods arrived when they
// were neither rented nor bought: donated, sponsored, borrowed. The list is
// one shared vocabulary. A word typed on the money screen is offered at the
// delivery desk afterwards, and a word typed at the desk is offered on the
// money screen.
//
// A merged kind keeps its row so records saved with it still resolve through
// to the wording they should now show, exactly as a merged financial value does.
type AcquisitionKind struct {
	ID           string    `json:"id"` // AKD-0001
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"createdAt"`
	CreatedBy    string    `json:"createdBy"` // the on-duty name, or the financial account's name
	Changes      []Change  `json:"changes,omitempty"`
	MergedIntoID string    `json:"mergedIntoId,omitempty"`
}

// ErrKindUsed is the refusal when a kind a record still points at would be
// deleted. Used words are corrected or merged, never erased.
var ErrKindUsed = errors.New("This kind has been used. Rename it or merge it instead.")

// LiveAcquisitionKinds are the unmerged kinds, alphabetical by fold.
func LiveAcquisitionKinds(r *Register) []AcquisitionKind {
	out := []AcquisitionKind{}
	for _, k := range r.AcquisitionKinds {
		if k.MergedIntoID == "" {
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return FoldKey(out[i].Name) < FoldKey(out[j].Name) })
	return out
}

// AcquisitionKindByID returns the kind with this ID whether or not it is merged.
func AcquisitionKindByID(r *Register, id string) (AcquisitionKind, bool) {
	for _, k := range r.AcquisitionKinds {
		if k.ID == id {
			return k, true
		}
	}
	return AcquisitionKind{}, false
}

// ResolveAcquisitionKind follows a merge chain to the kind a record should now
// show. A chain longer than the whole list is a cycle and resolves to nothing.
func ResolveAcquisitionKind(r *Register, id string) (AcquisitionKind, bool) {
	seen := 0
	for id != "" {
		k, ok := AcquisitionKindByID(r, id)
		if !ok {
			return AcquisitionKind{}, false
		}
		if k.MergedIntoID == "" {
			return k, true
		}
		id = k.MergedIntoID
		seen++
		if seen > len(r.AcquisitionKinds) {
			return AcquisitionKind{}, false
		}
	}
	return AcquisitionKind{}, false
}

// AcquisitionKindText is the word to show for an ID, "" when it resolves to
// nothing. Nothing anywhere refuses to load a record whose kind has gone: the
// screen falls back to the plain word "Other" through BasisWord.
func AcquisitionKindText(r *Register, id string) string {
	if k, ok := ResolveAcquisitionKind(r, id); ok {
		return k.Name
	}
	return ""
}

// FindAcquisitionKindByText returns the live kind whose folded word is exactly
// the folded query. An exact match always reuses; it never adds a second row
// saying the same thing.
func FindAcquisitionKindByText(r *Register, text string) (AcquisitionKind, bool) {
	key := FoldKey(text)
	if key == "" {
		return AcquisitionKind{}, false
	}
	for _, k := range LiveAcquisitionKinds(r) {
		if FoldKey(k.Name) == key {
			return k, true
		}
	}
	return AcquisitionKind{}, false
}

// AddAcquisitionKind appends a kind and returns its ID, reusing an exact
// folded match instead of duplicating it. Call it inside the write
// transaction: that is what stops two simultaneous saves creating two rows
// that say the same word.
func AddAcquisitionKind(r *Register, text, by string, at time.Time) (string, error) {
	text = CleanName(text)
	if text == "" {
		return "", errors.New("the kind is blank")
	}
	if existing, ok := FindAcquisitionKindByText(r, text); ok {
		return existing.ID, nil
	}
	id := r.NextID("AKD")
	r.AcquisitionKinds = append(r.AcquisitionKinds, AcquisitionKind{
		ID: id, Name: text, CreatedAt: at, CreatedBy: by,
	})
	return id, nil
}

// AcquisitionKindIsUsed reports whether anything still points at this kind: a
// delivery, an order line, or another kind merged into it. A deleted delivery
// counts, because its history has to keep saying what it said. f may be nil,
// which is the desk with nobody logged in.
func AcquisitionKindIsUsed(r *Register, f *FinanceData, id string) bool {
	if id == "" {
		return false
	}
	for _, in := range r.Inwards {
		if in.KindID == id {
			return true
		}
	}
	for _, k := range r.AcquisitionKinds {
		if k.MergedIntoID == id {
			return true
		}
	}
	if f != nil {
		for _, o := range f.Orders {
			for _, l := range o.Lines {
				if l.KindID == id {
					return true
				}
			}
		}
	}
	return false
}

// BasisWord is how one acquisition reads on screen and in a correction note:
// "Rent", "Purchase", or the typed word. A kind that has gone missing reads as
// the plain word "Other" rather than stopping anything.
func BasisWord(r *Register, b Basis, kindID string) string {
	switch b {
	case Rent:
		return "Rent"
	case Other:
		if word := AcquisitionKindText(r, kindID); word != "" {
			return word
		}
		return "Other"
	default:
		return "Purchase"
	}
}

// CanGoBack says whether goods received on this basis may be sent back to a
// supplier. Rent always can; every typed kind can too, because a borrowed
// thing may well go back and the program has no business deciding otherwise.
func CanGoBack(b Basis) bool { return b == Rent || b == Other }

// CanBeSold says whether goods received on this basis may be sold. Purchase
// always can; every typed kind can too, because a donated thing may well be
// sold. Both doors draw on the same stock, so goods sold can no longer go back.
func CanBeSold(b Basis) bool { return b == Purchase || b == Other }

// KindWord is the kind's word folded for comparison, used to group and sort
// rows that share one typed kind.
func kindSortKey(r *Register, b Basis, kindID string) string {
	return strings.ToLower(BasisWord(r, b, kindID))
}
