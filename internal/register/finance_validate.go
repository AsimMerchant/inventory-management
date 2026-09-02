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
	return nil
}

func (f *FinanceData) NextID(prefix string) string {
	highest := 0
	var ids []string
	if prefix == "FAC" {
		for _, a := range f.Accounts {
			ids = append(ids, a.ID)
		}
	} else if prefix == "FAE" {
		for _, a := range f.Audit {
			ids = append(ids, a.ID)
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
