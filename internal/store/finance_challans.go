package store

import (
	"fmt"
	"time"

	"storeregister/internal/register"
)

func (s *Store) RenameSupplierChallan(vaultKey []byte, actorID, challanID, newNumber string, now time.Time) error {
	newNumber = register.CleanName(newNumber)
	if newNumber == "" {
		return fmt.Errorf("Type a different challan number for this supplier.")
	}
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range reg.SupplierChallans {
			if reg.SupplierChallans[i].ID == challanID {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that challan is not on the list")
		}
		row := &reg.SupplierChallans[at]
		if row.Number == newNumber {
			return nil
		}
		party, ok := register.ResolveParty(reg, row.PartyID)
		if !ok {
			return fmt.Errorf("that challan's supplier is not on the list")
		}
		for _, other := range reg.SupplierChallans {
			otherParty, resolves := register.ResolveParty(reg, other.PartyID)
			if other.ID != row.ID && resolves && otherParty.ID == party.ID && register.FoldKey(other.Number) == register.FoldKey(newNumber) {
				return fmt.Errorf("Type a different challan number for this supplier.")
			}
		}
		old := row.Number
		row.Number = newNumber
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "supplier_challan_renamed", "supplier_challan", row.ID,
			"Supplier challan corrected", old, newNumber))
		return nil
	})
}

func (s *Store) DeleteSupplierChallan(vaultKey []byte, actorID, challanID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, data *register.FinanceData) error {
		if err := requireAdmin(data, actorID); err != nil {
			return err
		}
		at := -1
		for i := range reg.SupplierChallans {
			if reg.SupplierChallans[i].ID == challanID {
				at = i
			}
		}
		if at < 0 {
			return fmt.Errorf("that challan is not on the list")
		}
		row := reg.SupplierChallans[at]
		if register.SupplierChallanIsUsed(data, row.ID) {
			return register.ErrSupplierChallanUsed
		}
		reg.SupplierChallans = append(reg.SupplierChallans[:at], reg.SupplierChallans[at+1:]...)
		data.Audit = append(data.Audit, FinanceAudit(data, actorID, now, "supplier_challan_deleted", "supplier_challan", row.ID,
			"Unused supplier challan removed", row.Number, ""))
		return nil
	})
}
