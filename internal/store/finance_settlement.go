package store

import (
	"fmt"
	"time"

	"storeregister/internal/register"
)

// SettlementDraft is one physical exit as a screen submitted it. Quantity is
// what the person typed; the sources are worked out again inside the write.
type SettlementDraft struct {
	Kind string // supplier_return | sale
	// PartyID is what the picker selected; PartyText is what was typed when
	// nothing was selected. Both travel together so the party is resolved or
	// created inside the settlement's own transaction: resolving it in a
	// separate write would save the file even when the settlement is then
	// refused.
	PartyID   string
	PartyText string
	ProductID string
	Quantity  int
	At        time.Time
	Reference string
	Remarks   string
}

// resolveParty finds or creates the party inside the caller's transaction.
func resolveParty(reg *register.Register, d SettlementDraft) (register.Party, error) {
	if d.PartyID != "" {
		if p, ok := register.ResolveParty(reg, d.PartyID); ok {
			return p, nil
		}
	}
	id, err := register.AddParty(reg, d.PartyText)
	if err != nil {
		return register.Party{}, ErrSettlementRefused
	}
	p, _ := register.PartyByID(reg, id)
	return p, nil
}

// RecordSettlement writes the protected settlement and its neutral public
// stock removal in one encrypted save. Both availability caps are worked out
// again in here, under the same lock, so two people cannot both spend the last
// of the stock.
func (s *Store) RecordSettlement(vaultKey []byte, actorID string, d SettlementDraft, now time.Time) (string, error) {
	var id string
	err := s.UpdateFinance(vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		p, ok := register.ProductByID(reg, d.ProductID)
		if !ok {
			return ErrSettlementRefused
		}
		party, err := resolveParty(reg, d)
		if err != nil {
			return err
		}

		var sources []register.DisposalAllocation
		switch d.Kind {
		case "supplier_return":
			sources, err = register.AllocateSupplierReturn(reg, f, party.ID, p.ID, d.Quantity)
		case "sale":
			sources, err = register.AllocateStockSale(reg, f, p.ID, d.Quantity)
		default:
			return ErrSettlementRefused
		}
		if err != nil {
			return ErrNotEnough
		}

		disposal := register.InventoryDisposal{
			ID: reg.NextID("DSP"), ProductID: p.ID, Quantity: d.Quantity,
			Sources: sources, RecordedAt: now,
		}
		reg.Disposals = append(reg.Disposals, disposal)
		ref := register.FinanceProductRef{ProductID: p.ID, ProductName: p.Name}

		if d.Kind == "supplier_return" {
			id = f.NextID("SRN")
			f.SupplierReturns = append(f.SupplierReturns, register.SupplierReturn{
				ID: id, DisposalID: disposal.ID, PartyID: party.ID, Product: ref,
				Sources: sources, ReturnedAt: d.At, Reference: d.Reference, Remarks: d.Remarks,
				RecordedAt: now, RecordedByID: actorID,
			})
			f.Audit = append(f.Audit, FinanceAudit(f, actorID, now, "supplier_return_created",
				"supplier_return", id, settlementSummary(party.Name, d.Quantity, p.Name), "", ""))
		} else {
			id = f.NextID("SAL")
			f.Sales = append(f.Sales, register.StockSale{
				ID: id, DisposalID: disposal.ID, BuyerPartyID: party.ID, Product: ref,
				Sources: sources, SoldAt: d.At, Reference: d.Reference, Remarks: d.Remarks,
				RecordedAt: now, RecordedByID: actorID,
			})
			f.Audit = append(f.Audit, FinanceAudit(f, actorID, now, "sale_created",
				"sale", id, settlementSummary(party.Name, d.Quantity, p.Name), "", ""))
		}
		return nil
	})
	return id, err
}

func settlementSummary(party string, quantity int, product string) string {
	return party + " · " + itoaStore(quantity) + " " + product
}

func itoaStore(n int) string { return fmt.Sprintf("%d", n) }

// ErrSettlementRefused is a settlement the register cannot make sense of.
var ErrSettlementRefused = fmt.Errorf("that settlement cannot be recorded")

// ErrNotEnough is a settlement asking for more than the two caps allow.
var ErrNotEnough = fmt.Errorf("not enough stock")

// EditSettlement corrects one settlement. It reallocates from scratch and
// rewrites the paired public removal, so the two halves never drift apart.
func (s *Store) EditSettlement(vaultKey []byte, actorID, kind, id string, d SettlementDraft, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		party, err := resolveParty(reg, d)
		if err != nil {
			return err
		}

		var (
			productID  string
			disposalID string
			before     []register.FinanceChange
			voided     bool
		)
		switch kind {
		case "supplier_return":
			for _, x := range f.SupplierReturns {
				if x.ID == id {
					productID, disposalID, before, voided = x.Product.ProductID, x.DisposalID, x.Changes, x.Voided != nil
				}
			}
		case "sale":
			for _, x := range f.Sales {
				if x.ID == id {
					productID, disposalID, before, voided = x.Product.ProductID, x.DisposalID, x.Changes, x.Voided != nil
				}
			}
		default:
			return ErrSettlementRefused
		}
		_ = before
		if productID == "" {
			return ErrSettlementRefused
		}
		if voided {
			return ErrSettlementRefused
		}

		// Reallocate against everything except this settlement's own current
		// removal, so a correction is measured as if it had not happened.
		var sources []register.DisposalAllocation
		if kind == "supplier_return" {
			sources, err = register.ReallocateSupplierReturn(reg, f, party.ID, productID, d.Quantity, id)
		} else {
			sources, err = register.ReallocateStockSale(reg, f, productID, d.Quantity, id)
		}
		if err != nil {
			return ErrNotEnough
		}

		for i := range reg.Disposals {
			if reg.Disposals[i].ID == disposalID {
				reg.Disposals[i].Quantity = d.Quantity
				reg.Disposals[i].Sources = sources
			}
		}
		applySettlementEdit(reg, f, kind, id, party.ID, sources, d, actorID, now)
		return nil
	})
}

// VoidSettlement marks a settlement that should never have been recorded and
// switches off its public stock removal, so the stock comes back.
func (s *Store) VoidSettlement(vaultKey []byte, actorID, kind, id, reason string, now time.Time) error {
	reason = register.CleanName(reason)
	if reason == "" {
		return ErrSettlementRefused
	}
	return s.UpdateFinance(vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		actor, _ := financeActor(f, actorID)
		void := &register.FinanceVoid{
			At: now, ByAccountID: actorID, ByName: actor.DisplayName,
			ByMobile: actor.Mobile, Reason: reason,
		}
		disposalID := ""
		switch kind {
		case "supplier_return":
			for i := range f.SupplierReturns {
				if f.SupplierReturns[i].ID == id {
					if f.SupplierReturns[i].Voided != nil {
						return ErrSettlementRefused
					}
					f.SupplierReturns[i].Voided = void
					disposalID = f.SupplierReturns[i].DisposalID
				}
			}
		case "sale":
			for i := range f.Sales {
				if f.Sales[i].ID == id {
					if f.Sales[i].Voided != nil {
						return ErrSettlementRefused
					}
					f.Sales[i].Voided = void
					disposalID = f.Sales[i].DisposalID
				}
			}
		default:
			return ErrSettlementRefused
		}
		if disposalID == "" {
			return ErrSettlementRefused
		}
		for i := range reg.Disposals {
			if reg.Disposals[i].ID == disposalID {
				at := now
				reg.Disposals[i].InactiveAt = &at
			}
		}
		f.Audit = append(f.Audit, FinanceAudit(f, actorID, now, kind+"_voided", kind, id, reason, "", "Voided"))
		return nil
	})
}

// applySettlementEdit writes the corrected values and records one audited
// change per changed field, in the order each spec fixes.
func applySettlementEdit(reg *register.Register, f *register.FinanceData, kind, id, partyID string,
	sources []register.DisposalAllocation, d SettlementDraft, actorID string, now time.Time) {

	change := func(field, label, from, to string) *register.FinanceChange {
		if from == to {
			return nil
		}
		c := FinanceChangeBy(f, actorID, now, field, label, from, to)
		return &c
	}
	quantityText := func(n int, product string) string { return itoaStore(n) + " " + product }
	timeText := func(t time.Time) string { return t.Format("2 January 2006 · 3:04 pm") }
	blank := func(s string) string {
		if s == "" {
			return "Blank"
		}
		return s
	}

	if kind == "supplier_return" {
		for i := range f.SupplierReturns {
			x := &f.SupplierReturns[i]
			if x.ID != id {
				continue
			}
			var added []register.FinanceChange
			for _, c := range []*register.FinanceChange{
				change("quantity", "How many", quantityText(x.Quantity(), x.Product.ProductName), quantityText(d.Quantity, x.Product.ProductName)),
				change("returnedAt", "Date and time returned", timeText(x.ReturnedAt), timeText(d.At)),
				change("party", "Supplier", register.PartyText(reg, x.PartyID), register.PartyText(reg, partyID)),
				change("reference", "Reference", blank(x.Reference), blank(d.Reference)),
				change("remarks", "Remarks", blank(x.Remarks), blank(d.Remarks)),
			} {
				if c != nil {
					added = append(added, *c)
				}
			}
			if len(added) == 0 {
				return
			}
			x.Sources, x.PartyID, x.ReturnedAt = sources, partyID, d.At
			x.Reference, x.Remarks = d.Reference, d.Remarks
			x.Changes = append(x.Changes, added...)
			f.Audit = append(f.Audit, FinanceAudit(f, actorID, now, "supplier_return_edited",
				"supplier_return", id, "Supplier return corrected", "", ""))
			return
		}
		return
	}

	for i := range f.Sales {
		x := &f.Sales[i]
		if x.ID != id {
			continue
		}
		var added []register.FinanceChange
		for _, c := range []*register.FinanceChange{
			change("quantity", "How many", quantityText(x.Quantity(), x.Product.ProductName), quantityText(d.Quantity, x.Product.ProductName)),
			change("soldAt", "Date and time sold", timeText(x.SoldAt), timeText(d.At)),
			change("party", "Buyer or other party", register.PartyText(reg, x.BuyerPartyID), register.PartyText(reg, partyID)),
			change("reference", "Reference", blank(x.Reference), blank(d.Reference)),
			change("remarks", "Remarks", blank(x.Remarks), blank(d.Remarks)),
		} {
			if c != nil {
				added = append(added, *c)
			}
		}
		if len(added) == 0 {
			return
		}
		x.Sources, x.BuyerPartyID, x.SoldAt = sources, partyID, d.At
		x.Reference, x.Remarks = d.Reference, d.Remarks
		x.Changes = append(x.Changes, added...)
		f.Audit = append(f.Audit, FinanceAudit(f, actorID, now, "sale_edited", "sale", id, "Sale corrected", "", ""))
		return
	}
}
