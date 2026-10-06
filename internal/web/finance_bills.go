package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"storeregister/internal/register"
)

type billEditData struct {
	CSRF, Error, Action, Amount, OccurredAt, Reference, Remarks string
	Party, Purpose                                              valuePicker
	Challan                                                     supplierChallanPicker
	Products                                                    string
}

func (s *Server) billEditDraft(r *http.Request, id string) (billEditData, bool) {
	sess := financeSessionOf(r)
	d := billEditData{CSRF: sess.csrf, Action: "/finance/bills/" + id + "/edit"}
	found := false
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		bill, ok := register.BillByID(f, id)
		if !ok {
			return
		}
		found = true
		d.Amount, d.OccurredAt = rupeeInput(bill.AmountPaise), bill.OccurredAt.Format("2006-01-02T15:04")
		d.Reference, d.Remarks, d.Products = bill.Reference, bill.Remarks, productRefText(bill.Products)
		d.Party = partyPicker(reg, "Supplier", "partyId", "partyName", bill.PartyID, "")
		d.Purpose = pickerFor(f, register.FinancePurpose, "Purpose", "purposeId", "purposeName", bill.PurposeID, "", "")
		d.Challan.PickedID = bill.SupplierChallanID
		if row, _, ok := register.ResolveSupplierChallan(reg, bill.SupplierChallanID); ok {
			d.Challan.PickedText = row.Number
		}
		for _, row := range register.MatchSupplierChallans(reg, bill.PartyID, "") {
			d.Challan.Options = append(d.Challan.Options, valueOption{ID: row.ID, Value: row.Number})
		}
	})
	return d, found
}

func (s *Server) renderBillEdit(w http.ResponseWriter, r *http.Request, d billEditData) {
	s.financePage(w, r, "Fix this bill", "finance-bill-form.html", d)
}

func (s *Server) financeBillEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, ok := s.billEditDraft(r, id)
	if !ok {
		s.financeNotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		s.renderBillEdit(w, r, d)
		return
	}
	d.Amount = strings.TrimSpace(r.FormValue("amount"))
	d.OccurredAt = strings.TrimSpace(r.FormValue("occurredAt"))
	d.Reference, d.Remarks = register.CleanName(r.FormValue("reference")), register.CleanName(r.FormValue("remarks"))
	d.Party.PickedID, d.Party.PickedText = readPartyPicker(r, "partyId", "partyName")
	d.Purpose.PickedID, d.Purpose.PickedText = readPickerPair(r, "purposeId", "purposeName")
	d.Challan.PickedID = firstValue(r, "supplierChallanId")
	d.Challan.PickedText = register.CleanName(firstValue(r, "supplierChallanNo"))
	if typed := register.CleanName(r.FormValue("supplierChallanNoNew")); typed != "" {
		d.Challan.PickedID, d.Challan.PickedText = "", typed
	} else if choice := r.FormValue("supplierChallanIdChoice"); choice != "" {
		d.Challan.PickedID = choice
	}

	sess, now := financeSessionOf(r), s.now()
	refusal := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		at := -1
		for i := range f.Bills {
			if f.Bills[i].ID == id {
				at = i
			}
		}
		if at < 0 || f.Bills[at].Voided != nil {
			refusal = "This bill was voided. Record a new one instead."
			return errMoneyRefused
		}
		amount, err := register.ParseRupees(d.Amount)
		if err != nil {
			refusal = "Type each amount in rupees, using up to two decimal places."
			return errMoneyRefused
		}
		occurred, err := time.ParseInLocation("2006-01-02T15:04", d.OccurredAt, time.Local)
		if err != nil {
			refusal = "Pick the date and time of the entry."
			return errMoneyRefused
		}
		party, _, err := resolvePartyID(reg, d.Party.PickedID, d.Party.PickedText)
		if err != nil || party == "" {
			refusal = "Say which supplier this is for."
			return errMoneyRefused
		}
		purpose, err := resolveValue(f, register.FinancePurpose, d.Purpose.PickedID, d.Purpose.PickedText, sess.accountID, now)
		if err != nil {
			refusal = "Say what this entry is for."
			return errMoneyRefused
		}
		challan, err := resolveSupplierChallanID(reg, party, d.Challan.PickedID, d.Challan.PickedText)
		if err != nil {
			refusal = "Pick this supplier's challan from the list, or type a new number."
			return errMoneyRefused
		}
		before, after := f.Bills[at], f.Bills[at]
		after.AmountPaise, after.OccurredAt, after.PartyID, after.PurposeID = amount, occurred, party, purpose
		after.SupplierChallanID, after.Reference, after.Remarks = challan, d.Reference, d.Remarks
		changes := billChanges(reg, f, before, after, sess.accountID, now)
		if len(changes) == 0 {
			return nil
		}
		after.Changes = append(append([]register.FinanceChange{}, before.Changes...), changes...)
		f.Bills[at] = after
		f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "bill_edited", "bill", id, "Bill corrected", billSummary(reg, f, before), billSummary(reg, f, after)))
		if _, err := register.TotalBills(f, nil); err != nil {
			refusal = register.ErrMoneyOverflow.Error()
			return errMoneyRefused
		}
		return nil
	})
	if errors.Is(err, errMoneyRefused) {
		d.Error = refusal
		s.renderBillEdit(w, r, d)
	} else if err != nil {
		d.Error = saveFailed
		s.renderBillEdit(w, r, d)
	} else {
		http.Redirect(w, r, "/finance/journal?corrected="+id, http.StatusSeeOther)
	}
}

func readPickerPair(r *http.Request, idName, textName string) (string, string) {
	id, text := firstValue(r, idName), register.CleanName(firstValue(r, textName))
	if typed := register.CleanName(r.FormValue(textName + "New")); typed != "" {
		return "", typed
	}
	if choice := r.FormValue(idName + "Choice"); choice != "" {
		return choice, ""
	}
	return id, text
}

func billChanges(reg *register.Register, f *register.FinanceData, before, after register.FinanceBill, actorID string, at time.Time) []register.FinanceChange {
	var out []register.FinanceChange
	add := func(field, label, from, to string) {
		if from != to {
			out = append(out, financeChange(f, actorID, at, field, label, from, to))
		}
	}
	add("amount", "Billed", register.FormatRupees(before.AmountPaise), register.FormatRupees(after.AmountPaise))
	add("occurredAt", "Date and time", before.OccurredAt.Format("2 January 2006 · 3:04 pm"), after.OccurredAt.Format("2 January 2006 · 3:04 pm"))
	add("party", "Supplier", register.PartyText(reg, before.PartyID), register.PartyText(reg, after.PartyID))
	add("purpose", "Purpose", register.FinanceValueText(f, before.PurposeID), register.FinanceValueText(f, after.PurposeID))
	add("supplierChallan", "Challan no.", supplierChallanText(reg, before.SupplierChallanID), supplierChallanText(reg, after.SupplierChallanID))
	add("reference", "Reference", blankOr(before.Reference), blankOr(after.Reference))
	add("remarks", "Remarks", blankOr(before.Remarks), blankOr(after.Remarks))
	return out
}

func financeChange(f *register.FinanceData, actorID string, at time.Time, field, label, from, to string) register.FinanceChange {
	for _, actor := range f.Accounts {
		if actor.ID == actorID {
			return register.FinanceChange{At: at, ByAccountID: actorID, ByName: actor.DisplayName, ByMobile: actor.Mobile, Field: field, Label: label, From: from, To: to}
		}
	}
	return register.FinanceChange{At: at, ByAccountID: actorID, Field: field, Label: label, From: from, To: to}
}

func supplierChallanText(reg *register.Register, id string) string {
	if row, _, ok := register.ResolveSupplierChallan(reg, id); ok {
		return row.Number
	}
	return "Blank"
}

func (s *Server) financeBillVoid(w http.ResponseWriter, r *http.Request) {
	id, reason := r.PathValue("id"), register.CleanName(r.FormValue("reason"))
	sess := financeSessionOf(r)
	confirm := func(problem string) {
		s.renderFinanceConfirm(w, r, financeConfirmData{Heading: "Void this bill?", Warning: "Use this only if this bill entry was a mistake. It will stop counting in billed totals, but it will stay in the journal and financial activity. If the supplier cancelled or replaced a real bill, keep this bill and record what actually happened in Remarks.", Action: "/finance/bills/" + id + "/void", Button: "Yes, void this bill", AskReason: true, ReasonLabel: "Why are you voiding this bill?", Reason: reason, Error: problem})
	}
	if _, ok := registerBill(s, sess, id); !ok {
		s.financeNotFound(w, r)
		return
	}
	if r.FormValue("confirm") != "yes" {
		confirm("")
		return
	}
	if reason == "" {
		confirm("Say why you are voiding this bill.")
		return
	}
	refusal := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		for i := range f.Bills {
			if f.Bills[i].ID != id {
				continue
			}
			if f.Bills[i].Voided != nil {
				refusal = "This bill is already voided."
				return errMoneyRefused
			}
			change := financeChange(f, sess.accountID, s.now(), "", "", "", "")
			f.Bills[i].Voided = &register.FinanceVoid{At: s.now(), ByAccountID: sess.accountID, ByName: change.ByName, ByMobile: change.ByMobile, Reason: reason}
			f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, s.now(), "bill_voided", "bill", id, reason, billSummary(reg, f, f.Bills[i]), "Voided"))
			return nil
		}
		return errMoneyRefused
	})
	if errors.Is(err, errMoneyRefused) {
		s.renderJournal(w, r, refusal)
	} else if err != nil {
		s.renderJournal(w, r, saveFailed)
	} else {
		http.Redirect(w, r, "/finance/journal?voided="+id, http.StatusSeeOther)
	}
}

func registerBill(s *Server, sess *financeSession, id string) (register.FinanceBill, bool) {
	var bill register.FinanceBill
	var ok bool
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) { bill, ok = register.BillByID(f, id) })
	return bill, ok
}
