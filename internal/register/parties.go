package register

import (
	"errors"
	"sort"
	"strings"
)

// Party is one supplier or other party the store deals with: whoever the goods
// came from, and whoever money is paid to. The list is one shared vocabulary.
// A name picked at the delivery desk is offered on the money screens
// afterwards, and a name typed on a money screen is offered at the desk.
//
// It sits in the open register rather than the vault because the desk is never
// logged in and cannot read a list it has to pick from. A name on its own is
// not financial data: no amount, no purpose, no payment mode and no link to
// any money record leaves the vault with it.
//
// A merged party keeps its row so records saved with it still resolve through
// to the name they should now show.
type Party struct {
	// ID is PRT-0001 for a party this list allocated. Parties imported from
	// the vault's old protected list keep their original PTY-0001 identifier,
	// so that not one reference inside the vault had to be rewritten.
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	PreviousNames []string `json:"previousNames,omitempty"`
	MergedIntoID  string   `json:"mergedIntoId,omitempty"`
}

// ErrPartyUsed is the refusal when a party a record still points at would be
// deleted. Used names are corrected or merged, never erased.
var ErrPartyUsed = errors.New("This name has been used. Rename it or merge it instead.")

// IsPartyID says whether an identifier belongs to the shared party list. Both
// prefixes are live: PRT for names this list allocated, PTY for names imported
// from the vault's old list.
func IsPartyID(id string) bool {
	return strings.HasPrefix(id, "PRT-") || strings.HasPrefix(id, "PTY-")
}

// LiveParties are the unmerged parties, alphabetical by fold.
func LiveParties(r *Register) []Party {
	out := []Party{}
	for _, p := range r.Parties {
		if p.MergedIntoID == "" {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return FoldKey(out[i].Name) < FoldKey(out[j].Name) })
	return out
}

// PartyByID returns the party with this ID whether or not it is merged.
func PartyByID(r *Register, id string) (Party, bool) {
	for _, p := range r.Parties {
		if p.ID == id {
			return p, true
		}
	}
	return Party{}, false
}

// ResolveParty follows a merge chain to the party a record should now show.
// A chain longer than the whole list is a cycle and resolves to nothing.
func ResolveParty(r *Register, id string) (Party, bool) {
	seen := 0
	for id != "" {
		p, ok := PartyByID(r, id)
		if !ok {
			return Party{}, false
		}
		if p.MergedIntoID == "" {
			return p, true
		}
		id = p.MergedIntoID
		seen++
		if seen > len(r.Parties) {
			return Party{}, false
		}
	}
	return Party{}, false
}

// PartyText is the name to show for an ID, "" when it resolves to nothing.
// Nothing anywhere refuses to load a record whose party has gone: the screen
// falls back to the name that was stored with the record.
func PartyText(r *Register, id string) string {
	if p, ok := ResolveParty(r, id); ok {
		return p.Name
	}
	return ""
}

// ResolvedPartyID is the ID a record's party now points at, or the ID itself
// when it resolves to nothing. It is what two records are compared on.
func ResolvedPartyID(r *Register, id string) string {
	if p, ok := ResolveParty(r, id); ok {
		return p.ID
	}
	return id
}

// FindPartyByText returns the live party whose folded name is exactly the
// folded query. An exact match always reuses; it never adds a second row
// saying the same thing.
func FindPartyByText(r *Register, text string) (Party, bool) {
	key := FoldKey(text)
	if key == "" {
		return Party{}, false
	}
	for _, p := range LiveParties(r) {
		if FoldKey(p.Name) == key {
			return p, true
		}
	}
	return Party{}, false
}

// MatchParties is the typeahead: case-insensitive substring, names beginning
// with the query first, alphabetical by fold within each group. The caller
// caps the list; the uncapped list is the no-script fallback.
func MatchParties(r *Register, query string) []Party {
	q := FoldKey(query)
	rows := []Party{}
	for _, p := range LiveParties(r) {
		if q != "" && !strings.Contains(FoldKey(p.Name), q) {
			continue
		}
		rows = append(rows, p)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		fi, fj := FoldKey(rows[i].Name), FoldKey(rows[j].Name)
		pi := q != "" && strings.HasPrefix(fi, q)
		pj := q != "" && strings.HasPrefix(fj, q)
		if pi != pj {
			return pi
		}
		return fi < fj
	})
	return rows
}

// AddParty appends a party and returns its ID, reusing an exact folded match
// instead of duplicating it. Call it inside the write transaction: that is
// what stops two screens saving one name at once leaving two rows.
func AddParty(r *Register, text string) (string, error) {
	text = CleanName(text)
	if text == "" {
		return "", errors.New("the name is blank")
	}
	if existing, ok := FindPartyByText(r, text); ok {
		return existing.ID, nil
	}
	id := r.NextID("PRT")
	r.Parties = append(r.Parties, Party{ID: id, Name: text})
	return id, nil
}

// PartyIsUsed reports whether anything still points at this party: a delivery,
// an order, a money movement, a supplier return, a sale, or another party
// merged into it. A voided movement and a deleted delivery both count, because
// their history has to keep saying what it said. f may be nil, which is the
// desk with nobody logged in.
func PartyIsUsed(r *Register, f *FinanceData, id string) bool {
	if id == "" {
		return false
	}
	for _, in := range r.Inwards {
		if in.PartyID == id {
			return true
		}
	}
	for _, p := range r.Parties {
		if p.MergedIntoID == id {
			return true
		}
	}
	if f == nil {
		return false
	}
	for _, o := range f.Orders {
		if o.PartyID == id {
			return true
		}
	}
	for _, m := range f.Movements {
		if m.PartyID == id {
			return true
		}
	}
	for _, s := range f.SupplierReturns {
		if s.PartyID == id {
			return true
		}
	}
	for _, s := range f.Sales {
		if s.BuyerPartyID == id {
			return true
		}
	}
	return false
}

// ValidatePartyReferences checks the one invariant that spans the open party
// list and the encrypted ledger. It deliberately lives outside ValidateFinance:
// the vault must be decryptable before its old party rows can be migrated.
func ValidatePartyReferences(r *Register, f *FinanceData) error {
	ids := make(map[string]bool)
	folds := make(map[string]bool)
	for _, p := range r.Parties {
		if !IsPartyID(p.ID) || ids[p.ID] {
			return errors.New("party id is blank, invalid or duplicated")
		}
		ids[p.ID] = true
		if CleanName(p.Name) == "" || p.Name != CleanName(p.Name) {
			return errors.New("party name is invalid")
		}
		seenNames := map[string]bool{FoldKey(p.Name): true}
		for _, previous := range p.PreviousNames {
			if CleanName(previous) == "" || previous != CleanName(previous) || seenNames[FoldKey(previous)] {
				return errors.New("party previous name is invalid or duplicated")
			}
			seenNames[FoldKey(previous)] = true
		}
		if p.MergedIntoID == "" {
			key := FoldKey(p.Name)
			if folds[key] {
				return errors.New("party name is duplicated")
			}
			folds[key] = true
		}
	}
	for _, p := range r.Parties {
		if p.MergedIntoID != "" {
			target, ok := PartyByID(r, p.MergedIntoID)
			if !ok || target.ID == p.ID {
				return errors.New("party merge target is invalid")
			}
			if _, ok := ResolveParty(r, p.ID); !ok {
				return errors.New("party merge does not end anywhere")
			}
		}
	}
	for _, in := range r.Inwards {
		if in.PartyID != "" {
			if _, ok := ResolveParty(r, in.PartyID); !ok {
				return errors.New("delivery party is unknown")
			}
		}
	}
	if f == nil {
		return nil
	}
	check := func(id, kind string) error {
		if _, ok := ResolveParty(r, id); !ok {
			return errors.New(kind + " party is unknown")
		}
		return nil
	}
	for _, o := range f.Orders {
		if err := check(o.PartyID, "financial order"); err != nil {
			return err
		}
	}
	for _, m := range f.Movements {
		if err := check(m.PartyID, "money movement"); err != nil {
			return err
		}
	}
	for _, s := range f.SupplierReturns {
		if err := check(s.PartyID, "supplier return"); err != nil {
			return err
		}
	}
	for _, s := range f.Sales {
		if err := check(s.BuyerPartyID, "sale"); err != nil {
			return err
		}
	}
	return nil
}

// PartyAliases is every spelling that has ever meant this party: its current
// name, every wording it was corrected from, and the same for every party
// merged into it. A delivery recorded before a rename must still match.
func PartyAliases(r *Register, partyID string) map[string]bool {
	out := map[string]bool{}
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if depth > len(r.Parties)+1 {
			return
		}
		p, ok := PartyByID(r, id)
		if !ok {
			return
		}
		if key := FoldKey(p.Name); key != "" {
			out[key] = true
		}
		for _, previous := range p.PreviousNames {
			if key := FoldKey(previous); key != "" {
				out[key] = true
			}
		}
		// Anything merged into this one carried its own history here.
		for _, other := range r.Parties {
			if other.MergedIntoID == id {
				walk(other.ID, depth+1)
			}
		}
	}
	walk(ResolvedPartyID(r, partyID), 0)
	return out
}

// InwardPartyName is the name to show for one delivery: the shared list entry
// it points at, or the text it was saved with when that entry has gone. The
// text on the record is a snapshot and is never rewritten by a rename, so the
// file stays readable by hand.
func InwardPartyName(r *Register, in Inward) string {
	if name := PartyText(r, in.PartyID); name != "" {
		return name
	}
	return in.Supplier
}

// LinkInwardParties gives every delivery that names a supplier an entry on
// the shared list. It runs on every load, so a delivery typed into the file by
// hand joins the list too.
//
// Two spellings already in the file are two entries. Only an exact match,
// ignoring case and surrounding space, reuses one — the same rule that stops
// the list growing a second row every time a name is typed. Nothing here folds
// two names together because they look similar; a human merges those with the
// tool built for it.
func LinkInwardParties(reg *Register) {
	for i := range reg.Inwards {
		in := &reg.Inwards[i]
		if in.PartyID != "" || CleanName(in.Supplier) == "" {
			continue
		}
		id, err := AddParty(reg, in.Supplier)
		if err != nil {
			continue
		}
		in.PartyID = id
	}
}
