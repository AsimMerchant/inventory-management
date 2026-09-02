package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// settlementDraft is the Return rented goods or Record a sale form, held as
// typed so a refusal hands the whole page back.
type settlementDraft struct {
	CSRF        string
	Kind        string
	Heading     string
	Submit      string
	PartyLabel  string
	Action      string
	ID          string
	Error       string
	Party       valuePicker
	ProductID   string
	ProductName string
	Quantity    string
	At          string
	Reference   string
	Remarks     string
	Available   int
	Picker      pickerData
	Editing     bool
}

func (s *Server) readSettlementDraft(r *http.Request, kind string) settlementDraft {
	_ = r.ParseForm()
	d := settlementDraft{
		Kind: kind,
		// formProductID, not FormValue: the picker submits a hidden field and
		// its <noscript> fallback submits a select of the same name, so with no
		// script both arrive and the empty one is first.
		ProductID: formProductID(r),
		Quantity:  strings.TrimSpace(r.FormValue("quantity")),
		At:        strings.TrimSpace(r.FormValue("at")),
		Reference: register.CleanName(r.FormValue("reference")),
		Remarks:   register.CleanName(r.FormValue("remarks")),
	}
	d.Party.PickedID = r.FormValue("partyId")
	d.Party.PickedText = register.CleanName(r.FormValue("partyName"))
	if typed := register.CleanName(r.FormValue("partyNameNew")); typed != "" {
		d.Party.PickedID, d.Party.PickedText = "", typed
	} else if picked := r.FormValue("partyIdChoice"); picked != "" {
		d.Party.PickedID, d.Party.PickedText = picked, ""
	}
	return d
}

// fillSettlement works out what may still be returned or sold, and offers only
// the products with something available.
func (s *Server) fillSettlement(d *settlementDraft, r *http.Request) {
	sess := financeSessionOf(r)
	if d.At == "" {
		d.At = s.now().Format("2006-01-02T15:04")
	}
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		if p, ok := register.ProductByID(reg, d.ProductID); ok {
			d.ProductName = p.Name
		}
		label := "Supplier"
		if d.Kind == "sale" {
			label = "Buyer or other party"
		}
		d.PartyLabel = label
		d.Party = pickerFor(f, register.FinanceParty, label,
			"partyId", "partyName", d.Party.PickedID, d.Party.PickedText, "")

		// A refused settlement creates no party, so the typed name is all
		// there is to go on. Resolve it by text too, or the refusal would say
		// "Only 0" about a supplier who has plenty here.
		partyID := ""
		if v, ok := register.ResolveFinanceValue(f, d.Party.PickedID); ok {
			partyID = v.ID
		} else if v, ok := register.FindFinanceValueByText(f, register.FinanceParty, d.Party.PickedText); ok {
			partyID = v.ID
		}
		// Type to find the product, like every other screen. The suggestions
		// carry how many may still go back or be sold, because that number is
		// the reason the person is on this screen at all.
		mode := "return"
		if d.Kind == "sale" {
			mode = "sale"
		}
		d.Picker = pickerData{
			Label: "Product", Mode: mode, Endpoint: "/finance/api/products",
			PickedID:  d.ProductID,
			CountInto: "[data-available]",
		}
		// What may go back depends on which supplier sent it. What may be sold
		// does not depend on who is buying, so the sale screen must not tie the
		// two together: it would throw away a product the buyer never affected.
		if d.Kind != "sale" {
			d.Picker.PartyFrom = `[data-values][data-kind="party"] [data-values-text]`
		}
		for _, p := range reg.Products {
			if p.Deleted != nil {
				continue
			}
			available, word := 0, "available"
			switch {
			case d.Kind == "sale":
				available = register.PurchasedAvailableToSellExcluding(reg, f, p.ID, d.ID)
				if available <= 0 && p.ID != d.ProductID {
					continue
				}
			case partyID != "":
				available = register.SupplierReturnAvailableExcluding(reg, f, partyID, p.ID, d.ID)
			case d.Party.PickedText != "":
				// Typed but not on the protected list yet: the goods may still
				// be here under that name on the inwards.
				available = register.SupplierReturnAvailableByName(reg, f, d.Party.PickedText, p.ID)
			default:
				// No supplier named. The list is what is in the store, the same
				// as the picker shows, so the two fields can be filled in
				// either order.
				available, word = register.OnHand(reg, p.ID), "on hand"
			}
			if d.Kind != "sale" && register.OnHand(reg, p.ID) <= 0 && p.ID != d.ProductID {
				continue
			}
			row := suggestion{
				ID: p.ID, Name: p.Name, OnHand: available,
				Label: p.Name + " — " + strconv.Itoa(available) + " " + word,
			}
			// These same rows are the picker's no-script fallback.
			d.Picker.Products = append(d.Picker.Products, row)
			if p.ID == d.ProductID {
				d.Available = available
				d.Picker.PickedName = p.Name
			}
		}
	})
}

func (s *Server) renderSettlement(w http.ResponseWriter, r *http.Request, d settlementDraft) {
	sess := financeSessionOf(r)
	d.CSRF = sess.csrf
	if d.Heading == "" {
		d.Heading = settlementHeading(d.Kind)
	}
	if d.Submit == "" {
		d.Submit = settlementSubmit(d.Kind)
	}
	if d.Action == "" {
		d.Action = settlementNewPath(d.Kind)
	}
	s.fillSettlement(&d, r)
	s.financePage(w, r, d.Heading, "finance-settlement-form.html", d)
}

func settlementHeading(kind string) string {
	if kind == "sale" {
		return "Record a sale"
	}
	return "Return rented goods"
}

func settlementSubmit(kind string) string {
	if kind == "sale" {
		return "Save sale"
	}
	return "Save supplier return"
}

func settlementNewPath(kind string) string {
	if kind == "sale" {
		return "/finance/sales/new"
	}
	return "/finance/supplier-returns/new"
}

func (s *Server) financeSupplierReturnNew(w http.ResponseWriter, r *http.Request) {
	s.settlementNew(w, r, "supplier_return")
}

func (s *Server) financeSaleNew(w http.ResponseWriter, r *http.Request) {
	s.settlementNew(w, r, "sale")
}

// settlementNew is the form and its save. The two caps are worked out again
// inside the write, so two people cannot both spend the last of the stock.
func (s *Server) settlementNew(w http.ResponseWriter, r *http.Request, kind string) {
	if r.Method != http.MethodPost {
		s.renderSettlement(w, r, settlementDraft{Kind: kind})
		return
	}

	d := s.readSettlementDraft(r, kind)
	sess := financeSessionOf(r)
	now := s.now()

	quantity, at, productName, refusal := s.settlementFields(r, &d)
	if refusal != "" {
		d.Error = refusal
		s.renderSettlement(w, r, d)
		return
	}

	if d.Party.PickedID == "" && d.Party.PickedText == "" {
		d.Error = settlementPartyRefusal(kind)
		s.renderSettlement(w, r, d)
		return
	}

	// The party is resolved or created inside the settlement's own write, so a
	// refused settlement leaves nothing behind at all.
	id, err := s.st.RecordSettlement(sess.vaultKey, sess.accountID, store.SettlementDraft{
		Kind: kind, PartyID: d.Party.PickedID, PartyText: d.Party.PickedText,
		ProductID: d.ProductID, Quantity: quantity,
		At: at, Reference: d.Reference, Remarks: d.Remarks,
	}, now)

	switch {
	case errors.Is(err, store.ErrNotEnough):
		s.fillSettlement(&d, r)
		d.Error = tooMany(kind, d.Available, productName)
		s.renderSettlement(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderSettlement(w, r, d)
	default:
		http.Redirect(w, r, "/finance/settlements?saved="+id, http.StatusSeeOther)
	}
}

// settlementFields checks everything that does not need the write lock.
func (s *Server) settlementFields(r *http.Request, d *settlementDraft) (int, time.Time, string, string) {
	quantity, err := strconv.Atoi(d.Quantity)
	if err != nil || quantity < 1 {
		return 0, time.Time{}, "", quantityRefusal(d.Kind)
	}
	at, err := time.ParseInLocation("2006-01-02T15:04", d.At, time.Local)
	if err != nil {
		return 0, time.Time{}, "", timeRefusal(d.Kind)
	}
	name := ""
	s.st.Read(func(reg *register.Register) {
		if p, ok := register.ProductByID(reg, d.ProductID); ok {
			name = p.Name
		}
	})
	if name == "" {
		return 0, time.Time{}, "", "Pick the product from the list."
	}
	return quantity, at, name, ""
}

func quantityRefusal(kind string) string {
	if kind == "sale" {
		return "Type how many were sold."
	}
	return "Type how many were returned to the supplier."
}

func timeRefusal(kind string) string {
	if kind == "sale" {
		return "Pick the date and time they were sold."
	}
	return "Pick the date and time they went back."
}

func settlementPartyRefusal(kind string) string {
	if kind == "sale" {
		return "Say who bought them."
	}
	return "Say which supplier they went back to."
}

func tooMany(kind string, allowed int, product string) string {
	if kind == "sale" {
		return "Only " + strconv.Itoa(allowed) + " " + product + " can be sold."
	}
	return "Only " + strconv.Itoa(allowed) + " " + product + " can be returned to this supplier."
}

type settlementView struct {
	Kind, KindLabel, ID, Party, Product string
	Quantity                            int
	At, RecordedAt                      time.Time
	RecordedBy, Reference, Remarks      string
	Changes                             []register.FinanceChange
	Voided                              *register.FinanceVoid
	VoidLine                            string
	ProductGone                         bool
}

type settlementsData struct {
	CSRF, Notice, Error string
	Rows                []settlementView
}

// financeSettlements is every physical exit, newest first.
func (s *Server) financeSettlements(w http.ResponseWriter, r *http.Request) {
	s.renderSettlements(w, r, "")
}

func (s *Server) renderSettlements(w http.ResponseWriter, r *http.Request, problem string) {
	sess := financeSessionOf(r)
	data := settlementsData{CSRF: sess.csrf, Error: problem}
	q := r.URL.Query()
	switch {
	case q.Get("saved") != "":
		data.Notice = "Supplier return saved."
		if strings.HasPrefix(q.Get("saved"), "SAL-") {
			data.Notice = "Sale saved."
		}
	case q.Get("corrected") != "":
		data.Notice = "Correction saved."
	case q.Get("voided") != "":
		data.Notice = "Voided."
	}
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		for _, row := range register.SettlementRows(reg, f) {
			v := settlementView{
				Kind: row.Kind, KindLabel: settlementKindLabel(row.Kind), ID: row.ID,
				Party:   register.FinanceValueText(f, row.PartyID),
				Product: row.Product.ProductName, Quantity: row.Quantity,
				At: row.At, RecordedAt: row.RecordedAt,
				Reference: row.Reference, Remarks: row.Remarks,
				Changes: row.Changes, Voided: row.Voided, ProductGone: row.ProductGone,
			}
			for _, a := range f.Accounts {
				if a.ID == row.RecordedBy {
					v.RecordedBy = a.DisplayName
				}
			}
			if row.Voided != nil {
				v.VoidLine = "Voided — " + row.Voided.Reason
			}
			data.Rows = append(data.Rows, v)
		}
	})
	s.financePage(w, r, "Stock returned or sold", "finance-settlements.html", data)
}

func settlementKindLabel(kind string) string {
	if kind == "sale" {
		return "Sold"
	}
	return "Returned to supplier"
}

type obligationsData struct {
	Rows []register.SupplierObligation
}

// financeObligations is what each supplier actually sent and how much is left.
func (s *Server) financeObligations(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	data := obligationsData{}
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		data.Rows = register.SupplierObligations(reg, f)
	})
	s.financePage(w, r, "Rented goods still to return", "finance-obligations.html", data)
}

// financeSettlementEdit corrects one settlement. The product itself cannot
// change: a settlement against the wrong product is voided and re-entered.
func (s *Server) financeSettlementEdit(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	if kind != "supplier_return" && kind != "sale" {
		s.financeNotFound(w, r)
		return
	}
	sess := financeSessionOf(r)

	if r.Method != http.MethodPost {
		d, ok := s.draftFromSettlement(r, kind, id)
		if !ok {
			s.financeNotFound(w, r)
			return
		}
		s.renderSettlement(w, r, d)
		return
	}

	d := s.readSettlementDraft(r, kind)
	d.ID, d.Editing = id, true
	d.Action = "/finance/settlements/" + kind + "/" + id + "/edit"
	d.Heading, d.Submit = "Fix this record", "Save the correction"
	original, ok := s.draftFromSettlement(r, kind, id)
	if !ok {
		s.financeNotFound(w, r)
		return
	}
	if d.ProductID != "" && d.ProductID != original.ProductID {
		d.ProductID, d.ProductName = original.ProductID, original.ProductName
		d.Error = "The product cannot be changed here. Void this entry and record it again."
		s.renderSettlement(w, r, d)
		return
	}
	d.ProductID, d.ProductName = original.ProductID, original.ProductName

	now := s.now()
	quantity, at, productName, refusal := s.settlementFields(r, &d)
	if refusal != "" {
		d.Error = refusal
		s.renderSettlement(w, r, d)
		return
	}

	if d.Party.PickedID == "" && d.Party.PickedText == "" {
		d.Error = settlementPartyRefusal(kind)
		s.renderSettlement(w, r, d)
		return
	}

	err := s.st.EditSettlement(sess.vaultKey, sess.accountID, kind, id, store.SettlementDraft{
		Kind: kind, PartyID: d.Party.PickedID, PartyText: d.Party.PickedText,
		ProductID: d.ProductID, Quantity: quantity,
		At: at, Reference: d.Reference, Remarks: d.Remarks,
	}, now)

	switch {
	case errors.Is(err, store.ErrNotEnough):
		s.fillSettlement(&d, r)
		d.Error = tooMany(kind, d.Available, productName)
		s.renderSettlement(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderSettlement(w, r, d)
	default:
		http.Redirect(w, r, "/finance/settlements?corrected="+id, http.StatusSeeOther)
	}
}

func (s *Server) draftFromSettlement(r *http.Request, kind, id string) (settlementDraft, bool) {
	sess := financeSessionOf(r)
	d := settlementDraft{
		Kind: kind, ID: id, Editing: true,
		Heading: "Fix this record", Submit: "Save the correction",
		Action: "/finance/settlements/" + kind + "/" + id + "/edit",
	}
	found := false
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		if kind == "supplier_return" {
			for _, x := range f.SupplierReturns {
				if x.ID == id && x.Voided == nil {
					found = true
					d.Party.PickedID, d.ProductID, d.ProductName = x.PartyID, x.Product.ProductID, x.Product.ProductName
					d.Quantity = strconv.Itoa(x.Quantity())
					d.At = x.ReturnedAt.Format("2006-01-02T15:04")
					d.Reference, d.Remarks = x.Reference, x.Remarks
				}
			}
			return
		}
		for _, x := range f.Sales {
			if x.ID == id && x.Voided == nil {
				found = true
				d.Party.PickedID, d.ProductID, d.ProductName = x.BuyerPartyID, x.Product.ProductID, x.Product.ProductName
				d.Quantity = strconv.Itoa(x.Quantity())
				d.At = x.SoldAt.Format("2006-01-02T15:04")
				d.Reference, d.Remarks = x.Reference, x.Remarks
			}
		}
	})
	return d, found
}

// financeSettlementVoid marks a settlement that should never have been
// recorded. The stock comes back and the protected history stays.
func (s *Server) financeSettlementVoid(w http.ResponseWriter, r *http.Request) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	sess := financeSessionOf(r)
	reason := register.CleanName(r.FormValue("reason"))
	if kind != "supplier_return" && kind != "sale" {
		s.financeNotFound(w, r)
		return
	}
	if r.FormValue("confirm") != "yes" {
		if _, ok := s.draftFromSettlement(r, kind, id); !ok {
			s.financeNotFound(w, r)
			return
		}
		extra := "Any linked money received will not change."
		if kind == "sale" {
			extra = "Any linked sale proceeds will not change."
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading:      "Void this stock movement?",
			Warning:      "Stock will be put back into the store totals. This entry will stay in Stock returned or sold and in Financial activity.",
			ExtraWarning: extra, Action: "/finance/settlements/" + kind + "/" + id + "/void",
			Button: "Yes, void this stock movement", AskReason: true,
			ReasonLabel: "Why are you voiding this record?", Reason: reason,
		})
		return
	}
	if reason == "" {
		extra := "Any linked money received will not change."
		if kind == "sale" {
			extra = "Any linked sale proceeds will not change."
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Void this stock movement?", Warning: "Stock will be put back into the store totals. This entry will stay in Stock returned or sold and in Financial activity.",
			ExtraWarning: extra, Action: "/finance/settlements/" + kind + "/" + id + "/void",
			Button: "Yes, void this stock movement", AskReason: true, ReasonLabel: "Why are you voiding this record?", Error: "Say why you are voiding this record.",
		})
		return
	}
	err := s.st.VoidSettlement(sess.vaultKey, sess.accountID, kind, id, reason, s.now())
	switch {
	case errors.Is(err, store.ErrSettlementRefused):
		s.renderSettlements(w, r, "That record cannot be voided.")
	case err != nil:
		s.renderSettlements(w, r, saveFailed)
	default:
		http.Redirect(w, r, "/finance/settlements?voided="+id, http.StatusSeeOther)
	}
}
