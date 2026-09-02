package register

import (
	"fmt"
	"strconv"
	"strings"
)

func ValidateFinance(f *FinanceData) error {
	if f == nil {
		return fmt.Errorf("finance data is missing")
	}
	seenIDs := make(map[string]bool)
	seenMobiles := make(map[string]bool)
	for _, account := range f.Accounts {
		if account.ID == "" || seenIDs[account.ID] {
			return fmt.Errorf("financial account id is blank or duplicated")
		}
		seenIDs[account.ID] = true
		if CleanName(account.DisplayName) == "" || account.DisplayName != CleanName(account.DisplayName) {
			return fmt.Errorf("financial account name is invalid")
		}
		mobile := MobileKey(account.Mobile)
		if mobile == "" || seenMobiles[mobile] {
			return fmt.Errorf("financial account mobile is blank or duplicated")
		}
		seenMobiles[mobile] = true
		if account.Role != FinanceUser && account.Role != FinanceAdmin {
			return fmt.Errorf("financial account role is invalid")
		}
		if account.Status != "pending" && account.Status != "active" && account.Status != "disabled" {
			return fmt.Errorf("financial account status is invalid")
		}
	}
	for _, account := range f.Accounts {
		if !seenIDs[account.CreatedByID] {
			return fmt.Errorf("financial account creator is unknown")
		}
	}
	auditIDs := make(map[string]bool)
	for _, event := range f.Audit {
		if event.ID == "" || auditIDs[event.ID] {
			return fmt.Errorf("financial audit id is blank or duplicated")
		}
		auditIDs[event.ID] = true
		if !seenIDs[event.ByAccountID] || event.ByName == "" || MobileKey(event.ByMobile) == "" {
			return fmt.Errorf("financial audit actor is invalid")
		}
		if event.Kind == "" || event.EntityType == "" || event.EntityID == "" || event.Summary == "" {
			return fmt.Errorf("financial audit event is incomplete")
		}
	}
	if f.Accounts == nil {
		return fmt.Errorf("financial accounts must be a list")
	}
	if f.Audit == nil {
		return fmt.Errorf("financial audit must be a list")
	}
	if f.Orders == nil {
		return fmt.Errorf("financial orders must be a list")
	}
	if f.ReusableValues == nil {
		return fmt.Errorf("financial reusable values must be a list")
	}
	if err := validateReusableValues(f, seenIDs); err != nil {
		return err
	}
	return validateOrders(f, seenIDs)
}

// validateReusableValues holds the two rules the lists rest on: one live
// spelling per kind, and a merge chain that always ends somewhere.
func validateReusableValues(f *FinanceData, accountIDs map[string]bool) error {
	ids := make(map[string]bool)
	folded := make(map[string]bool)
	for _, v := range f.ReusableValues {
		if v.ID == "" || ids[v.ID] {
			return fmt.Errorf("reusable value id is blank or duplicated")
		}
		ids[v.ID] = true
		prefix := ValueKindPrefix(v.Kind)
		if prefix == "" || !strings.HasPrefix(v.ID, prefix+"-") {
			return fmt.Errorf("reusable value kind and id do not agree")
		}
		if CleanName(v.Value) == "" || v.Value != CleanName(v.Value) {
			return fmt.Errorf("reusable value text is invalid")
		}
		if !accountIDs[v.CreatedByID] {
			return fmt.Errorf("reusable value creator is unknown")
		}
		if v.MergedIntoID == "" {
			key := string(v.Kind) + "\x00" + FoldKey(v.Value)
			if folded[key] {
				return fmt.Errorf("reusable value is duplicated")
			}
			folded[key] = true
		}
		if err := validateFinanceChanges(v.Changes, accountIDs); err != nil {
			return err
		}
	}
	for _, v := range f.ReusableValues {
		if v.MergedIntoID == "" {
			continue
		}
		target, ok := FinanceValueByID(f, v.MergedIntoID)
		if !ok || target.Kind != v.Kind || target.ID == v.ID {
			return fmt.Errorf("reusable value merge target is invalid")
		}
		if _, ok := ResolveFinanceValue(f, v.ID); !ok {
			return fmt.Errorf("reusable value merge does not end anywhere")
		}
	}
	return nil
}

// validateOrders holds the order invariants. Every one of them is something a
// screen would otherwise have to re-check on the way out.
func validateOrders(f *FinanceData, accountIDs map[string]bool) error {
	orderIDs := make(map[string]bool)
	lineIDs := make(map[string]bool)
	for _, o := range f.Orders {
		if o.ID == "" || orderIDs[o.ID] {
			return fmt.Errorf("financial order id is blank or duplicated")
		}
		orderIDs[o.ID] = true
		if _, ok := ResolveFinanceValue(f, o.PartyID); !ok {
			return fmt.Errorf("financial order party is unknown")
		}
		if !accountIDs[o.CreatedByID] {
			return fmt.Errorf("financial order creator is unknown")
		}
		if o.Status != "open" && o.Status != "cancelled" {
			return fmt.Errorf("financial order status is invalid")
		}
		if o.AgreedPaise == nil {
			if o.AgreedKind != "" {
				return fmt.Errorf("financial order has a total kind but no total")
			}
		} else {
			if *o.AgreedPaise <= 0 {
				return fmt.Errorf("financial order total must be positive")
			}
			if o.AgreedKind != "estimated" && o.AgreedKind != "exact" {
				return fmt.Errorf("financial order total kind is invalid")
			}
		}
		if len(o.Lines) == 0 {
			return fmt.Errorf("financial order has no products")
		}
		pairs := make(map[string]bool)
		for _, l := range o.Lines {
			if l.ID == "" || lineIDs[l.ID] {
				return fmt.Errorf("financial order line id is blank or duplicated")
			}
			lineIDs[l.ID] = true
			if l.ProductID == "" || CleanName(l.ProductNameSnapshot) == "" {
				return fmt.Errorf("financial order line product is invalid")
			}
			if l.ExpectedQuantity < 1 {
				return fmt.Errorf("financial order line quantity must be at least 1")
			}
			if l.Basis != Rent && l.Basis != Purchase {
				return fmt.Errorf("financial order line must be rent or purchase")
			}
			pair := l.ProductID + "\x00" + string(l.Basis)
			if pairs[pair] {
				return fmt.Errorf("financial order repeats one product on the same basis")
			}
			pairs[pair] = true
		}
		if err := validateFinanceChanges(o.Changes, accountIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateFinanceChanges(changes []FinanceChange, accountIDs map[string]bool) error {
	for _, c := range changes {
		if !accountIDs[c.ByAccountID] || c.ByName == "" || MobileKey(c.ByMobile) == "" {
			return fmt.Errorf("financial correction actor is invalid")
		}
		if c.Field == "" || c.Label == "" {
			return fmt.Errorf("financial correction is incomplete")
		}
	}
	return nil
}

func (f *FinanceData) NextID(prefix string) string {
	highest := 0
	var ids []string
	switch prefix {
	case "FAC":
		for _, a := range f.Accounts {
			ids = append(ids, a.ID)
		}
	case "FAE":
		for _, a := range f.Audit {
			ids = append(ids, a.ID)
		}
	case "ORD":
		for _, o := range f.Orders {
			ids = append(ids, o.ID)
		}
	case "OLN":
		// Line IDs are unique across the whole vault, not within one order,
		// so a movement may name a line without also naming its order.
		for _, o := range f.Orders {
			for _, l := range o.Lines {
				ids = append(ids, l.ID)
			}
		}
	case "PTY", "PUR", "PMD":
		for _, v := range f.ReusableValues {
			ids = append(ids, v.ID)
		}
	}
	for _, id := range ids {
		rest, ok := strings.CutPrefix(id, prefix+"-")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(rest)
		if err == nil && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, highest+1)
}
