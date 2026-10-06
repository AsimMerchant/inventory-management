package register

import (
	"sort"
	"time"
)

type FinanceBill struct {
	ID                string              `json:"id"`
	AmountPaise       int64               `json:"amountPaise"`
	OccurredAt        time.Time           `json:"occurredAt"`
	PartyID           string              `json:"partyId"`
	OrderID           string              `json:"orderId,omitempty"`
	Products          []FinanceProductRef `json:"products,omitempty"`
	PurposeID         string              `json:"purposeId"`
	SupplierChallanID string              `json:"supplierChallanId,omitempty"`
	Reference         string              `json:"reference,omitempty"`
	Remarks           string              `json:"remarks,omitempty"`
	RecordedAt        time.Time           `json:"recordedAt"`
	RecordedByID      string              `json:"recordedById"`
	Changes           []FinanceChange     `json:"changes,omitempty"`
	Voided            *FinanceVoid        `json:"voided,omitempty"`
}

func (b FinanceBill) Live() bool { return b.Voided == nil }

func BillByID(f *FinanceData, id string) (FinanceBill, bool) {
	for _, b := range f.Bills {
		if b.ID == id {
			return b, true
		}
	}
	return FinanceBill{}, false
}

func SortedBills(rows []FinanceBill) []FinanceBill {
	out := append([]FinanceBill{}, rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.After(out[j].OccurredAt)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

func AscendingBills(rows []FinanceBill) []FinanceBill {
	out := append([]FinanceBill{}, rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.Before(out[j].OccurredAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func TotalBills(f *FinanceData, keep func(FinanceBill) bool) (int64, error) {
	var total int64
	for _, bill := range f.Bills {
		if !bill.Live() || (keep != nil && !keep(bill)) {
			continue
		}
		var err error
		total, err = AddPaise(total, bill.AmountPaise)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
