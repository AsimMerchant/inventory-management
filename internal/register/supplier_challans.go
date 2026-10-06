package register

import (
	"errors"
	"sort"
	"strings"
)

// SupplierChallan is a public suggestion only. The protected record that uses
// it remains in the encrypted vault.
type SupplierChallan struct {
	ID      string `json:"id"`
	PartyID string `json:"partyId"`
	Number  string `json:"number"`
}

var ErrSupplierChallanUsed = errors.New("This challan has been used. Rename it instead.")

func SupplierChallanByID(r *Register, id string) (SupplierChallan, bool) {
	for _, row := range r.SupplierChallans {
		if row.ID == id {
			return row, true
		}
	}
	return SupplierChallan{}, false
}

func ResolveSupplierChallan(r *Register, id string) (SupplierChallan, Party, bool) {
	row, ok := SupplierChallanByID(r, id)
	if !ok {
		return SupplierChallan{}, Party{}, false
	}
	party, ok := ResolveParty(r, row.PartyID)
	return row, party, ok
}

// MatchSupplierChallans returns deduplicated rows belonging to the selected
// resolved party. A prefix sorts before a substring, then text and stable ID.
func MatchSupplierChallans(r *Register, partyID, query string) []SupplierChallan {
	party, ok := ResolveParty(r, partyID)
	if !ok {
		return []SupplierChallan{}
	}
	q := FoldKey(query)
	byFold := make(map[string]SupplierChallan)
	for _, row := range r.SupplierChallans {
		rowParty, ok := ResolveParty(r, row.PartyID)
		if !ok || rowParty.ID != party.ID {
			continue
		}
		fold := FoldKey(row.Number)
		if q != "" && !strings.Contains(fold, q) {
			continue
		}
		if old, seen := byFold[fold]; !seen || row.ID < old.ID {
			byFold[fold] = row
		}
	}
	rows := make([]SupplierChallan, 0, len(byFold))
	for _, row := range byFold {
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		fi, fj := FoldKey(rows[i].Number), FoldKey(rows[j].Number)
		pi, pj := q != "" && strings.HasPrefix(fi, q), q != "" && strings.HasPrefix(fj, q)
		if pi != pj {
			return pi
		}
		if fi != fj {
			return fi < fj
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}

func AddSupplierChallan(r *Register, partyID, number string) (string, error) {
	number = CleanName(number)
	party, ok := ResolveParty(r, partyID)
	if !ok || number == "" {
		return "", errors.New("supplier challan is invalid")
	}
	for _, row := range MatchSupplierChallans(r, party.ID, number) {
		if FoldKey(row.Number) == FoldKey(number) {
			return row.ID, nil
		}
	}
	id := r.NextID("SCH")
	r.SupplierChallans = append(r.SupplierChallans, SupplierChallan{ID: id, PartyID: party.ID, Number: number})
	return id, nil
}

// ValidateSupplierChallans permits folded duplicates only when distinct
// historical party rows later resolve together through a merge.
func ValidateSupplierChallans(r *Register) error {
	ids := make(map[string]bool)
	foldsByStoredParty := make(map[string]bool)
	for _, row := range r.SupplierChallans {
		if !strings.HasPrefix(row.ID, "SCH-") || ids[row.ID] {
			return errors.New("supplier challan id is blank, invalid or duplicated")
		}
		ids[row.ID] = true
		if row.Number == "" || row.Number != CleanName(row.Number) {
			return errors.New("supplier challan number is invalid")
		}
		if _, ok := ResolveParty(r, row.PartyID); !ok {
			return errors.New("supplier challan party is unknown")
		}
		key := row.PartyID + "\x00" + FoldKey(row.Number)
		if foldsByStoredParty[key] {
			return errors.New("supplier challan number is duplicated")
		}
		foldsByStoredParty[key] = true
	}
	return nil
}

func SupplierChallanIsUsed(f *FinanceData, id string) bool {
	for _, bill := range f.Bills {
		if bill.SupplierChallanID == id {
			return true
		}
	}
	for _, movement := range f.Movements {
		if movement.SupplierChallanID == id {
			return true
		}
	}
	return false
}
