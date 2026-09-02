package store

import (
	"errors"
	"fmt"
	"time"

	"storeregister/internal/register"
)

// ErrNotAdmin is the refusal when a financial user who is not an administrator
// reaches list maintenance. Creating a value while recording something is open
// to everybody; correcting the shared lists is not.
var ErrNotAdmin = errors.New("only an administrator may do this")

// FinanceActor is who an audited financial write is attributed to. The account
// ID and mobile are immutable, so a later rename never orphans the history.
func financeActor(data *register.FinanceData, actorID string) (register.FinanceAccount, bool) {
	for _, a := range data.Accounts {
		if a.ID == actorID {
			return a, true
		}
	}
	return register.FinanceAccount{}, false
}

// FinanceAudit builds one audit row for any entity type.
func FinanceAudit(data *register.FinanceData, actorID string, at time.Time, kind, entityType, entityID, summary, before, after string) register.FinanceAuditEvent {
	actor, _ := financeActor(data, actorID)
	return register.FinanceAuditEvent{
		ID: data.NextID("FAE"), At: at,
		ByAccountID: actorID, ByName: actor.DisplayName, ByMobile: actor.Mobile,
		Kind: kind, EntityType: entityType, EntityID: entityID,
		Summary: summary, Before: before, After: after,
	}
}

// FinanceChangeBy builds one audited field correction attributed to actorID.
func FinanceChangeBy(data *register.FinanceData, actorID string, at time.Time, field, label, from, to string) register.FinanceChange {
	actor, _ := financeActor(data, actorID)
	return register.FinanceChange{
		At: at, ByAccountID: actorID, ByName: actor.DisplayName, ByMobile: actor.Mobile,
		Field: field, Label: label, From: from, To: to,
	}
}

func requireAdmin(data *register.FinanceData, actorID string) error {
	a, ok := financeActor(data, actorID)
	if !ok || a.Status != "active" || a.Role != register.FinanceAdmin {
		return ErrNotAdmin
	}
	return nil
}

// RenameFinanceValue corrects one shared spelling. Existing records keep the ID
// they were saved with and simply start showing the new text.
func (s *Store) RenameFinanceValue(vaultKey []byte, actorID, valueID, newText string, now time.Time) error {
	newText = register.CleanName(newText)
	if newText == "" {
		return fmt.Errorf("type the new wording")
	}
	return s.UpdateFinance(vaultKey, func(_ *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range data.ReusableValues {
			if data.ReusableValues[i].ID == valueID && data.ReusableValues[i].MergedIntoID == "" {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that value is not on the list")
		}
		v := &data.ReusableValues[at]
		if v.Value == newText {
			return nil
		}
		if existing, ok := register.FindFinanceValueByText(data, v.Kind, newText); ok && existing.ID != v.ID {
			return fmt.Errorf("%s is already on the list", existing.Value)
		}
		label := register.ValueKindLabel(v.Kind)
		old := v.Value
		v.Value = newText
		v.Changes = append(v.Changes, FinanceChangeBy(data, actorID, now, "value", label, old, newText))
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "value_renamed", "reusableValue", v.ID,
			label+" corrected", old, newText))
		return nil
	})
}

// MergeFinanceValue points one value at another. Nothing is rewritten: the
// records that used the old value follow the merge to the new wording.
func (s *Store) MergeFinanceValue(vaultKey []byte, actorID, valueID, targetID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(_ *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		if valueID == targetID {
			return fmt.Errorf("pick a different value to keep")
		}
		at := -1
		for i := range data.ReusableValues {
			if data.ReusableValues[i].ID == valueID && data.ReusableValues[i].MergedIntoID == "" {
				at = i
			}
		}
		target, ok := register.FinanceValueByID(data, targetID)
		if at < 0 || !ok || target.MergedIntoID != "" || target.Kind != data.ReusableValues[at].Kind {
			return fmt.Errorf("that value is not on the list")
		}
		v := &data.ReusableValues[at]
		label := register.ValueKindLabel(v.Kind)
		v.MergedIntoID = targetID
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "value_merged", "reusableValue", v.ID,
			label+" merged", v.Value, target.Value))
		return nil
	})
}

// DeleteFinanceValue removes a typo nothing points at. A value any record uses
// is never erased: it is renamed or merged, so old records keep their meaning.
func (s *Store) DeleteFinanceValue(vaultKey []byte, actorID, valueID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(_ *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range data.ReusableValues {
			if data.ReusableValues[i].ID == valueID {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that value is not on the list")
		}
		v := data.ReusableValues[at]
		if register.FinanceValueIsUsed(data, v.ID) || mergedInto(data, v.ID) {
			return register.ErrValueUsed
		}
		data.ReusableValues = append(data.ReusableValues[:at], data.ReusableValues[at+1:]...)
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "value_deleted", "reusableValue", v.ID,
			register.ValueKindLabel(v.Kind)+" removed", v.Value, ""))
		return nil
	})
}

// mergedInto reports whether another value points here. A merge target is in
// use even when no order names it directly.
func mergedInto(data *register.FinanceData, id string) bool {
	for _, v := range data.ReusableValues {
		if v.MergedIntoID == id {
			return true
		}
	}
	return false
}
