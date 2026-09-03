package store

import (
	"fmt"
	"time"

	"storeregister/internal/register"
)

// The three administrator actions on the shared acquisition-kinds list. They
// are the same three the party, purpose and payment-mode lists already have,
// and they are written the same way. The one difference is where the list
// lives: the kinds are plain register data, because the delivery desk reads
// them and is never logged in. The audit row still goes in the vault, because
// only an authorized person can get here at all.
//
// UpdateFinance hands the callback both halves under one lock and saves them
// in one write, so the admin check and the change to the open list happen in
// the same transaction.

// RenameAcquisitionKind corrects one spelling. Deliveries keep the ID they
// were saved with and simply start showing the new word.
func (s *Store) RenameAcquisitionKind(vaultKey []byte, actorID, kindID, newText string, now time.Time) error {
	newText = register.CleanName(newText)
	if newText == "" {
		return fmt.Errorf("type the new wording")
	}
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range reg.AcquisitionKinds {
			if reg.AcquisitionKinds[i].ID == kindID && reg.AcquisitionKinds[i].MergedIntoID == "" {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that kind is not on the list")
		}
		k := &reg.AcquisitionKinds[at]
		if k.Name == newText {
			return nil
		}
		if existing, ok := register.FindAcquisitionKindByText(reg, newText); ok && existing.ID != k.ID {
			return fmt.Errorf("%s is already on the list", existing.Name)
		}
		actor, _ := financeActor(data, actorID)
		old := k.Name
		k.Name = newText
		k.Changes = append(k.Changes, register.Change{
			At: now, By: actor.DisplayName, Field: "kind", Label: "How it came in", From: old, To: newText,
		})
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "kind_renamed", "acquisitionKind", k.ID,
			"How goods came in corrected", old, newText))
		return nil
	})
}

// MergeAcquisitionKind points one kind at another. Nothing is rewritten: the
// deliveries that used the old word follow the merge to the new one.
func (s *Store) MergeAcquisitionKind(vaultKey []byte, actorID, kindID, targetID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		if kindID == targetID {
			return fmt.Errorf("pick a different kind to keep")
		}
		at := -1
		for i := range reg.AcquisitionKinds {
			if reg.AcquisitionKinds[i].ID == kindID && reg.AcquisitionKinds[i].MergedIntoID == "" {
				at = i
			}
		}
		target, ok := register.AcquisitionKindByID(reg, targetID)
		if at < 0 || !ok || target.MergedIntoID != "" {
			return fmt.Errorf("that kind is not on the list")
		}
		k := &reg.AcquisitionKinds[at]
		k.MergedIntoID = targetID
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "kind_merged", "acquisitionKind", k.ID,
			"How goods came in merged", k.Name, target.Name))
		return nil
	})
}

// DeleteAcquisitionKind removes a typo nothing points at. A kind any delivery
// or order line uses is never erased: it is renamed or merged, so old records
// keep their meaning.
func (s *Store) DeleteAcquisitionKind(vaultKey []byte, actorID, kindID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range reg.AcquisitionKinds {
			if reg.AcquisitionKinds[i].ID == kindID {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that kind is not on the list")
		}
		k := reg.AcquisitionKinds[at]
		if register.AcquisitionKindIsUsed(reg, data, k.ID) {
			return register.ErrKindUsed
		}
		reg.AcquisitionKinds = append(reg.AcquisitionKinds[:at], reg.AcquisitionKinds[at+1:]...)
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "kind_deleted", "acquisitionKind", k.ID,
			"How goods came in removed", k.Name, ""))
		return nil
	})
}
