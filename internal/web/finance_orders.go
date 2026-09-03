package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// errOrderRefused is the sentinel a refused order transaction returns. The
// reason itself is put on the draft so the person sees it in words.
var errOrderRefused = errors.New("order refused")

// orderLine is one product row of the order form, held as typed so a refusal
// hands the whole draft back untouched.
type orderLine struct {
	ProductID   string
	ProductName string
	Quantity    string
	Basis       string
	KindID      string                     // the kind picked off the list, when the basis is "other"
	NewKind     string                     // a kind typed for the first time
	Kinds       []register.AcquisitionKind // the shared list, read from the open register
	Picker      pickerData
	Index       int
	Removable   bool
	Locked      bool   // a ledger entry points at this line
	LineID      string // set when correcting an existing order
}

// orderDraft is the order form: everything typed, plus everything the screen
// needs to redraw itself.
type orderDraft struct {
	CSRF       string
	Action     string
	Heading    string
	Submit     string
	OrderID    string
	Error      string
	Notice     string
	Party      valuePicker
	Lines      []orderLine
	OrderedAt  string
	Agreed     string
	AgreedKind string
	Remarks    string
}

// readOrderDraft reads the form back exactly as it was typed. Nothing is
// resolved here: resolution happens inside the write.
func (s *Server) readOrderDraft(r *http.Request) orderDraft {
	_ = r.ParseForm()
	d := orderDraft{
		OrderedAt:  strings.TrimSpace(r.FormValue("orderedAt")),
		Agreed:     strings.TrimSpace(r.FormValue("agreedTotal")),
		AgreedKind: r.FormValue("agreedKind"),
		Remarks:    register.CleanName(r.FormValue("remarks")),
	}
	d.Party.PickedID = r.FormValue("partyId")
	d.Party.PickedText = register.CleanName(r.FormValue("partyName"))
	if typed := register.CleanName(r.FormValue("partyNameNew")); typed != "" {
		d.Party.PickedID, d.Party.PickedText = "", typed
	} else if picked := r.FormValue("partyIdChoice"); picked != "" {
		d.Party.PickedID, d.Party.PickedText = picked, ""
	}

	ids, names := r.Form["productId"], r.Form["productName"]
	quantities, bases := r.Form["quantity"], r.Form["basis"]
	kindIDs, newKinds := r.Form["kindId"], r.Form["newKind"]
	lineIDs := r.Form["lineId"]
	count := len(names)
	for _, n := range []int{len(ids), len(quantities), len(bases), len(lineIDs)} {
		if n > count {
			count = n
		}
	}
	at := func(v []string, i int) string {
		if i < len(v) {
			return v[i]
		}
		return ""
	}
	for i := 0; i < count; i++ {
		basis := at(bases, i)
		if indexed, ok := r.Form[rowField("basis", i)]; ok {
			basis = at(indexed, 0)
		}
		d.Lines = append(d.Lines, orderLine{
			ProductID:   at(ids, i),
			ProductName: register.CleanName(at(names, i)),
			Quantity:    strings.TrimSpace(at(quantities, i)),
			Basis:       basis,
			KindID:      at(kindIDs, i),
			NewKind:     register.CleanName(at(newKinds, i)),
			LineID:      at(lineIDs, i),
		})
	}
	if len(d.Lines) == 0 {
		d.Lines = []orderLine{{}}
	}
	return d
}

// query re-encodes the draft so a redirect can hand it straight back. The field
// names are the form's own, so a redirect and a post are the same shape.
func (d orderDraft) query() url.Values {
	q := url.Values{}
	q.Set("partyId", d.Party.PickedID)
	q.Set("partyName", d.Party.PickedText)
	for _, l := range d.Lines {
		q.Add("productId", l.ProductID)
		q.Add("productName", l.ProductName)
		q.Add("quantity", l.Quantity)
		q.Add("basis", l.Basis)
		q.Add("kindId", l.KindID)
		q.Add("newKind", l.NewKind)
		q.Add("lineId", l.LineID)
	}
	q.Set("orderedAt", d.OrderedAt)
	q.Set("agreedTotal", d.Agreed)
	q.Set("agreedKind", d.AgreedKind)
	q.Set("remarks", d.Remarks)
	return q
}

// fill puts the pickers and the defaults on a draft ready to be drawn.
func (s *Server) fill(d *orderDraft, r *http.Request) {
	sess := financeSessionOf(r)
	if d.OrderedAt == "" {
		d.OrderedAt = s.now().Format("2006-01-02T15:04")
	}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		d.Party = pickerFor(f, register.FinanceParty, "Supplier or other party",
			"partyId", "partyName", d.Party.PickedID, d.Party.PickedText, "")
	})
	s.st.Read(func(reg *register.Register) {
		kinds := register.LiveAcquisitionKinds(reg)
		for i := range d.Lines {
			l := &d.Lines[i]
			l.Index = i
			l.Removable = len(d.Lines) > 1
			l.Kinds = kinds
			name := l.ProductName
			if l.ProductID != "" {
				if p, ok := register.ProductByID(reg, l.ProductID); ok {
					name = p.Name
				}
			}
			// An order is often the first anybody hears of a product, so it
			// can be added here — one deliberate press, the same guard against
			// near-duplicates the desk gets.
			l.Picker = pickerData{
				Endpoint:    "/finance/api/products",
				Label:       "Product",
				Mode:        "all",
				PickedID:    l.ProductID,
				PickedName:  name,
				Products:    matchProductsMode(reg, "", "all"),
				AllowNew:    true,
				NewEndpoint: "/finance/products/new",
				CSRF:        financeSessionOf(r).csrf,
			}
		}
	})
}

func (s *Server) renderOrderForm(w http.ResponseWriter, r *http.Request, d orderDraft) {
	sess := financeSessionOf(r)
	d.CSRF = sess.csrf
	s.fill(&d, r)
	title := d.Heading
	if title == "" {
		title = "Record an order"
		d.Heading = title
	}
	if d.Submit == "" {
		d.Submit = "Save order"
	}
	if d.Action == "" {
		d.Action = "/finance/orders/new"
	}
	s.financePage(w, r, title, "finance-order-form.html", d)
}

// financeOrderNew is the order form and its save. A refused save redraws the
// same page from the same draft, so nothing is ever typed twice.
func (s *Server) financeOrderNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		d := s.readOrderDraft(r)
		if added := r.URL.Query().Get("added"); added != "" {
			d.Notice = added + " added to the product list."
		}
		s.renderOrderForm(w, r, d)
		return
	}

	d := s.readOrderDraft(r)
	if r.FormValue("addLine") != "" {
		d.Lines = append(d.Lines, orderLine{})
		s.renderOrderForm(w, r, d)
		return
	}
	if at := r.FormValue("removeLine"); at != "" {
		if i, err := strconv.Atoi(at); err == nil && i >= 0 && i < len(d.Lines) && len(d.Lines) > 1 {
			d.Lines = append(d.Lines[:i], d.Lines[i+1:]...)
		}
		s.renderOrderForm(w, r, d)
		return
	}

	sess := financeSessionOf(r)
	now := s.now()
	orderedAt, agreed, refusal := orderFields(d)
	if refusal != "" {
		d.Error = refusal
		s.renderOrderForm(w, r, d)
		return
	}

	newID := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		partyID, err := resolveValue(f, register.FinanceParty, d.Party.PickedID, d.Party.PickedText, sess.accountID, now)
		if err != nil {
			refusal = "Say who this order is with."
			return errOrderRefused
		}
		lines, text := buildLines(reg, f, d, sess.accountID, now)
		if text != "" {
			refusal = text
			return errOrderRefused
		}
		newID = f.NextID("ORD")
		order := register.FinanceOrder{
			ID: newID, PartyID: partyID, Lines: lines, OrderedAt: orderedAt,
			Status: "open", Remarks: d.Remarks, CreatedAt: now, CreatedByID: sess.accountID,
		}
		if agreed != nil {
			order.AgreedPaise, order.AgreedKind = agreed, d.AgreedKind
		}
		f.Orders = append(f.Orders, order)
		f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "order_created", "order", newID,
			"Order recorded", "", orderSummary(reg, f, order)))
		return nil
	})
	switch {
	case errors.Is(err, errOrderRefused):
		d.Error = refusal
		s.renderOrderForm(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderOrderForm(w, r, d)
	default:
		http.Redirect(w, r, "/finance/orders/"+newID+"?saved=1", http.StatusSeeOther)
	}
}

// orderFields validates everything that does not need the register open.
func orderFields(d orderDraft) (time.Time, *int64, string) {
	orderedAt, err := time.ParseInLocation("2006-01-02T15:04", d.OrderedAt, time.Local)
	if err != nil {
		return time.Time{}, nil, "Pick the date and time of the order."
	}
	if d.Agreed == "" {
		if d.AgreedKind != "" {
			return time.Time{}, nil, "Leave the estimate or exact choice empty when there is no agreed total."
		}
		return orderedAt, nil, ""
	}
	paise, err := register.ParseRupees(d.Agreed)
	if err != nil {
		return time.Time{}, nil, register.MoneyRefusal
	}
	if d.AgreedKind != "estimated" && d.AgreedKind != "exact" {
		return time.Time{}, nil, "Say whether the agreed total is an estimate or exact."
	}
	return orderedAt, &paise, ""
}

// buildLines resolves each typed line against the live catalogue inside the
// transaction. A product name typed but never picked is refused: the product
// invariant is the same here as on the inventory side.
func buildLines(reg *register.Register, f *register.FinanceData, d orderDraft, actorID string, now time.Time) ([]register.FinanceOrderLine, string) {
	lines := []register.FinanceOrderLine{}
	seen := map[string]bool{}
	// NextID counts the lines already stored, so several new lines in one save
	// each need their own number without the store seeing them yet.
	spare := 0
	next := func() string {
		spare++
		n, _ := strconv.Atoi(strings.TrimPrefix(f.NextID("OLN"), "OLN-"))
		return "OLN-" + pad4(n+spare-1)
	}
	for _, l := range d.Lines {
		if l.ProductID == "" && l.ProductName == "" && l.Quantity == "" {
			continue
		}
		p, ok := register.ProductByID(reg, l.ProductID)
		if !ok {
			return nil, "Pick each product from the list."
		}
		quantity, err := strconv.Atoi(l.Quantity)
		if err != nil || quantity < 1 {
			return nil, "Type how many of each product were ordered."
		}
		basis := register.Basis(l.Basis)
		if basis != register.Rent && basis != register.Purchase && basis != register.Other {
			return nil, "Say how each product came in."
		}
		kindID := ""
		if basis == register.Other {
			kindID = l.KindID
			if l.NewKind != "" {
				added, err := register.AddAcquisitionKind(reg, l.NewKind, financeAccountName(f, actorID), now)
				if err != nil {
					return nil, kindRefusal
				}
				kindID = added
			}
			if _, ok := register.ResolveAcquisitionKind(reg, kindID); !ok {
				return nil, kindRefusal
			}
		}
		if seen[p.ID+string(basis)+kindID] {
			return nil, "That product is already on this order on the same terms."
		}
		seen[p.ID+string(basis)+kindID] = true
		id := l.LineID
		if id == "" {
			id = next()
		}
		lines = append(lines, register.FinanceOrderLine{
			ID: id, ProductID: p.ID, ProductNameSnapshot: p.Name,
			ExpectedQuantity: quantity, Basis: basis, KindID: kindID,
		})
	}
	if len(lines) == 0 {
		return nil, "Put at least one product on the order."
	}
	return lines, ""
}

// orderSummary is the one-line description an audit row carries.
func orderSummary(reg *register.Register, f *register.FinanceData, o register.FinanceOrder) string {
	parts := []string{register.FinanceValueText(f, o.PartyID)}
	parts = append(parts, lineText(reg, o.Lines))
	if o.AgreedPaise != nil {
		parts = append(parts, register.FormatRupees(*o.AgreedPaise))
	}
	return strings.Join(parts, " · ")
}

// lineText renders the product lines the way a correction records them. A
// typed kind is written out in the word somebody chose, not as "other": these
// strings are history and have to go on saying what they said.
func lineText(reg *register.Register, lines []register.FinanceOrderLine) string {
	var out []string
	for _, l := range lines {
		out = append(out, strconv.Itoa(l.ExpectedQuantity)+" "+l.ProductNameSnapshot+" — "+strings.ToLower(register.BasisWord(reg, l.Basis, l.KindID)))
	}
	return strings.Join(out, "; ")
}

func financeAuditFor(f *register.FinanceData, actorID string, at time.Time, kind, entityType, entityID, summary, before, after string) register.FinanceAuditEvent {
	var name, mobile string
	for _, a := range f.Accounts {
		if a.ID == actorID {
			name, mobile = a.DisplayName, a.Mobile
		}
	}
	return register.FinanceAuditEvent{
		ID: f.NextID("FAE"), At: at, ByAccountID: actorID, ByName: name, ByMobile: mobile,
		Kind: kind, EntityType: entityType, EntityID: entityID,
		Summary: summary, Before: before, After: after,
	}
}

// pad4 numbers an ID the way NextID does.
func pad4(n int) string {
	out := strconv.Itoa(n)
	for len(out) < 4 {
		out = "0" + out
	}
	return out
}

// orderView is one order as a screen shows it: values resolved to what they now
// say, product names shown as they were at the time.
type orderView struct {
	ID, Party, Status, Remarks string
	OrderedAt, CreatedAt       time.Time
	CreatedBy                  string
	Lines                      []orderLineView
	TotalLabel, Total          string
	Changes                    []register.FinanceChange
	Cancelled                  bool
}

type orderLineView struct {
	ID, Product, NowCalled, Basis string
	Quantity                      int
	Locked                        bool
}

// viewOrder renders one stored order. The snapshot is the historical label; a
// live rename is shown beside it rather than replacing it.
func viewOrder(reg *register.Register, f *register.FinanceData, o register.FinanceOrder) orderView {
	v := orderView{
		ID: o.ID, Party: register.FinanceValueText(f, o.PartyID), Status: o.Status,
		Remarks: o.Remarks, OrderedAt: o.OrderedAt, CreatedAt: o.CreatedAt,
		Changes: o.Changes, Cancelled: o.Status == "cancelled",
	}
	for _, a := range f.Accounts {
		if a.ID == o.CreatedByID {
			v.CreatedBy = a.DisplayName
		}
	}
	for _, l := range o.Lines {
		row := orderLineView{
			ID: l.ID, Product: l.ProductNameSnapshot, Quantity: l.ExpectedQuantity,
			Basis:  strings.ToLower(register.BasisWord(reg, l.Basis, l.KindID)),
			Locked: register.FinanceLineIsReferenced(f, l.ID),
		}
		if p, ok := register.ProductByID(reg, l.ProductID); ok && p.Name != l.ProductNameSnapshot {
			row.NowCalled = "now called " + p.Name
		}
		v.Lines = append(v.Lines, row)
	}
	if o.AgreedPaise != nil {
		v.Total = register.FormatRupees(*o.AgreedPaise)
		v.TotalLabel = "Agreed total"
		if o.AgreedKind == "estimated" {
			v.TotalLabel = "Estimated total"
		}
	}
	return v
}

type ordersData struct {
	CSRF   string
	Orders []orderView
}

// financeOrders lists every order, newest first.
func (s *Server) financeOrders(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	data := ordersData{CSRF: sess.csrf}
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		for _, o := range register.SortedFinanceOrders(f) {
			data.Orders = append(data.Orders, viewOrder(reg, f, o))
		}
	})
	s.financePage(w, r, "Orders", "finance-orders.html", data)
}

type orderDetailData struct {
	CSRF, Notice, Error string
	Order               orderView
}

// financeOrderDetail is one order with its corrections and the cancel form.
func (s *Server) financeOrderDetail(w http.ResponseWriter, r *http.Request) {
	s.renderOrderDetail(w, r, r.PathValue("id"), "")
}

func (s *Server) renderOrderDetail(w http.ResponseWriter, r *http.Request, id, problem string) {
	sess := financeSessionOf(r)
	data := orderDetailData{CSRF: sess.csrf, Error: problem}
	if r.URL.Query().Get("saved") != "" {
		data.Notice = "Order saved."
	}
	if r.URL.Query().Get("cancelled") != "" {
		data.Notice = "Order cancelled."
	}
	if r.URL.Query().Get("corrected") != "" {
		data.Notice = "Order corrected."
	}
	found := false
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		if o, ok := register.FinanceOrderByID(f, id); ok {
			data.Order, found = viewOrder(reg, f, o), true
		}
	})
	if !found {
		s.financeNotFound(w, r)
		return
	}
	s.financePage(w, r, "Order", "finance-order.html", data)
}

// financeProductNew is the second, deliberate route that may append a product,
// and it applies exactly the guards the ordinary one does. The order draft
// travels with the form, so the person comes back to their half-typed order
// with the new product already picked.
func (s *Server) financeProductNew(w http.ResponseWriter, r *http.Request) {
	d := s.readOrderDraft(r)
	at, err := strconv.Atoi(r.FormValue("line"))
	if err != nil || at < 0 || at >= len(d.Lines) {
		s.financeNotFound(w, r)
		return
	}
	name := d.Lines[at].ProductName
	if name == "" {
		d.Error = "Type the new product's name."
		s.renderOrderForm(w, r, d)
		return
	}

	sess := financeSessionOf(r)
	now := s.now()
	confirmed := r.FormValue("confirm") == "yes"

	var existing, nearMatch, newID string
	err = s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		for _, p := range reg.Products {
			if p.Deleted == nil && register.FoldKey(p.Name) == register.FoldKey(name) {
				existing = p.Name
				return errOrderRefused
			}
		}
		if !confirmed {
			if nearMatch = nearDuplicate(reg, name); nearMatch != "" {
				return errOrderRefused
			}
		}
		who := ""
		for _, a := range f.Accounts {
			if a.ID == sess.accountID {
				who = a.DisplayName
			}
		}
		newID = reg.NextID("PRD")
		reg.Products = append(reg.Products, register.Product{
			ID: newID, Name: name, CreatedAt: now, CreatedBy: who,
		})
		f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "product_created", "product", newID,
			"Product added while recording an order", "", name))
		return nil
	})

	switch {
	case existing != "":
		d.Error = existing + " is already on the list. Pick it."
		s.renderOrderForm(w, r, d)
	case nearMatch != "":
		s.financePage(w, r, "Add a product", "finance-product-confirm.html", struct {
			CSRF, Typed, Existing, Line string
			Draft                       url.Values
		}{sess.csrf, name, nearMatch, strconv.Itoa(at), d.query()})
	case err != nil:
		d.Error = saveFailed
		s.renderOrderForm(w, r, d)
	default:
		d.Lines[at].ProductID = newID
		q := d.query()
		q.Set("added", name)
		http.Redirect(w, r, "/finance/orders/new?"+q.Encode(), http.StatusSeeOther)
	}
}

// financeOrderCancel marks an order cancelled. It never voids a payment,
// deletes a product or moves stock: a cancelled order stays on the screen and
// can still receive a refund recorded separately.
func (s *Server) financeOrderCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reason := register.CleanName(r.FormValue("reason"))
	if r.FormValue("confirm") != "yes" {
		if _, ok := s.draftFromOrder(r, id); !ok {
			s.financeNotFound(w, r)
			return
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Cancel this order?",
			Warning: "The order will stay in the ledger. Existing payments, receipts and stock entries will not change. Record any refund separately.",
			Action:  "/finance/orders/" + id + "/cancel", Button: "Yes, cancel this order",
			AskReason: true, ReasonLabel: "Why is this order cancelled?", Reason: reason,
		})
		return
	}
	if reason == "" {
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Cancel this order?",
			Warning: "The order will stay in the ledger. Existing payments, receipts and stock entries will not change. Record any refund separately.",
			Action:  "/finance/orders/" + id + "/cancel", Button: "Yes, cancel this order",
			AskReason: true, ReasonLabel: "Why is this order cancelled?", Error: "Say why this order is cancelled.",
		})
		return
	}
	sess := financeSessionOf(r)
	now := s.now()

	refusal := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(_ *register.Register, f *register.FinanceData) error {
		for i := range f.Orders {
			if f.Orders[i].ID != id {
				continue
			}
			if f.Orders[i].Status == "cancelled" {
				refusal = "This order is already cancelled."
				return errOrderRefused
			}
			f.Orders[i].Status = "cancelled"
			f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "order_cancelled", "order", id,
				reason, "open", "cancelled"))
			return nil
		}
		refusal = "That order is not on the list."
		return errOrderRefused
	})
	switch {
	case errors.Is(err, errOrderRefused):
		s.renderOrderDetail(w, r, id, refusal)
	case err != nil:
		s.renderOrderDetail(w, r, id, saveFailed)
	default:
		http.Redirect(w, r, "/finance/orders/"+id+"?cancelled=1", http.StatusSeeOther)
	}
}

// financeOrderEdit corrects one order. Every changed field is recorded, in the
// order the spec lists, and nothing is silently deleted.
func (s *Server) financeOrderEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := financeSessionOf(r)

	if r.Method != http.MethodPost {
		d, ok := s.draftFromOrder(r, id)
		if !ok {
			s.financeNotFound(w, r)
			return
		}
		s.renderOrderForm(w, r, d)
		return
	}

	d := s.readOrderDraft(r)
	d.OrderID, d.Heading, d.Submit = id, "Fix this order", "Save the correction"
	d.Action = "/finance/orders/" + id + "/edit"

	if r.FormValue("addLine") != "" {
		d.Lines = append(d.Lines, orderLine{})
		s.renderOrderForm(w, r, d)
		return
	}
	if at := r.FormValue("removeLine"); at != "" {
		if i, err := strconv.Atoi(at); err == nil && i >= 0 && i < len(d.Lines) && len(d.Lines) > 1 {
			d.Lines = append(d.Lines[:i], d.Lines[i+1:]...)
		}
		s.renderOrderForm(w, r, d)
		return
	}

	now := s.now()
	orderedAt, agreed, refusal := orderFields(d)
	if refusal != "" {
		d.Error = refusal
		s.renderOrderForm(w, r, d)
		return
	}

	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		at := -1
		for i := range f.Orders {
			if f.Orders[i].ID == id {
				at = i
			}
		}
		if at < 0 {
			refusal = "That order is not on the list."
			return errOrderRefused
		}
		before := f.Orders[at]

		partyID, err := resolveValue(f, register.FinanceParty, d.Party.PickedID, d.Party.PickedText, sess.accountID, now)
		if err != nil {
			refusal = "Say who this order is with."
			return errOrderRefused
		}
		lines, text := buildLines(reg, f, d, sess.accountID, now)
		if text != "" {
			refusal = text
			return errOrderRefused
		}
		if text := lineRefusal(f, before.Lines, lines); text != "" {
			refusal = text
			return errOrderRefused
		}

		after := before
		after.PartyID, after.Lines, after.OrderedAt = partyID, lines, orderedAt
		after.AgreedPaise, after.AgreedKind, after.Remarks = agreed, d.AgreedKind, d.Remarks
		if agreed == nil {
			after.AgreedKind = ""
		}

		changes := orderChanges(reg, f, before, after, sess.accountID, now)
		if len(changes) == 0 {
			return nil
		}
		after.Changes = append(append([]register.FinanceChange{}, before.Changes...), changes...)
		f.Orders[at] = after
		f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "order_edited", "order", id,
			"Order corrected", orderSummary(reg, f, before), orderSummary(reg, f, after)))
		return nil
	})
	switch {
	case errors.Is(err, errOrderRefused):
		d.Error = refusal
		s.renderOrderForm(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderOrderForm(w, r, d)
	default:
		http.Redirect(w, r, "/finance/orders/"+id+"?corrected=1", http.StatusSeeOther)
	}
}

// lineRefusal holds the one rule corrections must not break: a line a ledger
// entry points at may change how many were expected, and nothing else.
func lineRefusal(f *register.FinanceData, before, after []register.FinanceOrderLine) string {
	kept := map[string]register.FinanceOrderLine{}
	for _, l := range after {
		kept[l.ID] = l
	}
	for _, old := range before {
		if !register.FinanceLineIsReferenced(f, old.ID) {
			continue
		}
		now, ok := kept[old.ID]
		if !ok || now.ProductID != old.ProductID || now.Basis != old.Basis || now.KindID != old.KindID {
			return register.ErrLineUsed.Error()
		}
	}
	return ""
}

// orderChanges records one FinanceChange per changed field, in the order the
// spec fixes. Submitting the same values again records nothing.
func orderChanges(reg *register.Register, f *register.FinanceData, before, after register.FinanceOrder, actorID string, at time.Time) []register.FinanceChange {
	var out []register.FinanceChange
	add := func(field, label, from, to string) {
		if from == to {
			return
		}
		var name, mobile string
		for _, a := range f.Accounts {
			if a.ID == actorID {
				name, mobile = a.DisplayName, a.Mobile
			}
		}
		out = append(out, register.FinanceChange{
			At: at, ByAccountID: actorID, ByName: name, ByMobile: mobile,
			Field: field, Label: label, From: from, To: to,
		})
	}
	add("party", "Supplier or other party",
		register.FinanceValueText(f, before.PartyID), register.FinanceValueText(f, after.PartyID))
	add("products", "Products ordered", lineText(reg, before.Lines), lineText(reg, after.Lines))
	add("agreedTotal", "Agreed total", totalText(before.AgreedPaise), totalText(after.AgreedPaise))
	add("agreedKind", "Estimate or exact", kindText(before.AgreedKind), kindText(after.AgreedKind))
	add("orderedAt", "Order date and time",
		before.OrderedAt.Format("2 January 2006 · 3:04 pm"), after.OrderedAt.Format("2 January 2006 · 3:04 pm"))
	add("remarks", "Remarks", remarkText(before.Remarks), remarkText(after.Remarks))
	return out
}

func totalText(paise *int64) string {
	if paise == nil {
		return "Not entered"
	}
	return register.FormatRupees(*paise)
}

func kindText(kind string) string {
	switch kind {
	case "estimated":
		return "Estimate"
	case "exact":
		return "Exact"
	}
	return "Not entered"
}

func remarkText(s string) string {
	if s == "" {
		return "Blank"
	}
	return s
}

// draftFromOrder fills the correction form with what is stored.
func (s *Server) draftFromOrder(r *http.Request, id string) (orderDraft, bool) {
	sess := financeSessionOf(r)
	d := orderDraft{
		OrderID: id, Heading: "Fix this order", Submit: "Save the correction",
		Action: "/finance/orders/" + id + "/edit",
	}
	found := false
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		o, ok := register.FinanceOrderByID(f, id)
		if !ok {
			return
		}
		found = true
		d.Party.PickedID = o.PartyID
		d.Party.PickedText = register.FinanceValueText(f, o.PartyID)
		for _, l := range o.Lines {
			d.Lines = append(d.Lines, orderLine{
				ProductID: l.ProductID, ProductName: l.ProductNameSnapshot,
				Quantity: strconv.Itoa(l.ExpectedQuantity), Basis: string(l.Basis), KindID: l.KindID,
				LineID: l.ID, Locked: register.FinanceLineIsReferenced(f, l.ID),
			})
		}
		d.OrderedAt = o.OrderedAt.Format("2006-01-02T15:04")
		if o.AgreedPaise != nil {
			d.Agreed = strconv.FormatInt(*o.AgreedPaise/100, 10) + "." + pad2(*o.AgreedPaise%100)
		}
		d.AgreedKind, d.Remarks = o.AgreedKind, o.Remarks
	})
	return d, found
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
