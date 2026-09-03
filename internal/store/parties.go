package store

import (
	"fmt"
	"time"

	"storeregister/internal/register"
)

// The shared list of suppliers and other parties used to live in two places at
// once: the desk typed a supplier as free text on a delivery, and the ledger
// kept a tidy protected list of the same real names inside the vault. One
// supplier, two vocabularies, and only the ledger's was safe from drift.
//
// The list is now one list, in the open register. This file holds the two
// migrations that get an existing file there, and the three administrator
// actions on the list afterwards.

func importVaultParties(reg *register.Register, f *register.FinanceData) {
	kept := make([]register.FinanceReusableValue, 0, len(f.ReusableValues))
	for _, v := range f.ReusableValues {
		if v.Kind != register.FinanceParty {
			kept = append(kept, v)
			continue
		}
		if _, exists := register.PartyByID(reg, v.ID); exists {
			continue
		}
		party := register.Party{ID: v.ID, Name: v.Value, MergedIntoID: v.MergedIntoID}
		for _, c := range v.Changes {
			if c.Field == "value" && register.CleanName(c.From) != "" && register.FoldKey(c.From) != register.FoldKey(party.Name) {
				party.PreviousNames = appendUniquePartyName(party.PreviousNames, c.From)
			}
		}
		if party.MergedIntoID == "" {
			if existing, ok := register.FindPartyByText(reg, v.Value); ok {
				party.MergedIntoID = existing.ID
			}
		}
		reg.Parties = append(reg.Parties, party)
	}
	f.ReusableValues = kept
}

func appendUniquePartyName(names []string, name string) []string {
	name = register.CleanName(name)
	for _, existing := range names {
		if register.FoldKey(existing) == register.FoldKey(name) {
			return names
		}
	}
	return append(names, name)
}

// liveParty is the one lookup the three actions below share: the row must be
// on the list and must not already be merged into another.
func livePartyIndex(reg *register.Register, id string) int {
	at := -1
	for i := range reg.Parties {
		if reg.Parties[i].ID == id && reg.Parties[i].MergedIntoID == "" {
			at = i
		}
	}
	return at
}

// RenameParty corrects one spelling. Deliveries and money records keep the ID
// they were saved with and simply start showing the new name — on the desk's
// screens as well as the ledger's, because there is only one list now.
func (s *Store) RenameParty(vaultKey []byte, actorID, partyID, newText string, now time.Time) error {
	newText = register.CleanName(newText)
	if newText == "" {
		return fmt.Errorf("type the new wording")
	}
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := livePartyIndex(reg, partyID)
		if at < 0 {
			return fmt.Errorf("that name is not on the list")
		}
		p := &reg.Parties[at]
		if p.Name == newText {
			return nil
		}
		if existing, ok := register.FindPartyByText(reg, newText); ok && existing.ID != p.ID {
			return fmt.Errorf("%s is already on the list", existing.Name)
		}
		old := p.Name
		p.Name = newText
		p.PreviousNames = appendUniquePartyName(p.PreviousNames, old)
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "party_renamed", "party", p.ID,
			"Supplier or other party corrected", old, newText))
		return nil
	})
}

// MergeParty points one name at another. Nothing is rewritten: the deliveries
// and the money records that used the old name follow the merge to the new one.
func (s *Store) MergeParty(vaultKey []byte, actorID, partyID, targetID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		if partyID == targetID {
			return fmt.Errorf("pick a different name to keep")
		}
		at := livePartyIndex(reg, partyID)
		target, ok := register.PartyByID(reg, targetID)
		if at < 0 || !ok || target.MergedIntoID != "" {
			return fmt.Errorf("that name is not on the list")
		}
		p := &reg.Parties[at]
		p.MergedIntoID = targetID
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "party_merged", "party", p.ID,
			"Supplier or other party merged", p.Name, target.Name))
		return nil
	})
}

// DeleteParty removes a typo nothing points at. A name any delivery, order,
// payment, supplier return or sale uses is never erased: it is renamed or
// merged, so old records keep their meaning.
func (s *Store) DeleteParty(vaultKey []byte, actorID, partyID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range reg.Parties {
			if reg.Parties[i].ID == partyID {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that name is not on the list")
		}
		p := reg.Parties[at]
		if register.PartyIsUsed(reg, data, p.ID) {
			return register.ErrPartyUsed
		}
		reg.Parties = append(reg.Parties[:at], reg.Parties[at+1:]...)
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "party_deleted", "party", p.ID,
			"Supplier or other party removed", p.Name, ""))
		return nil
	})
}
