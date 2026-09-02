package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// errMoneyRefused is the sentinel a refused money transaction returns. The
// reason itself goes on the draft so the person reads it in words.
var errMoneyRefused = errors.New("money refused")

// moneyRow is one row of the Record money form, held exactly as typed so a
// refusal hands the whole page back untouched.
type moneyRow struct {
	Index       int
	Direction   string
	Amount      string
	OccurredAt  string
	Party       valuePicker
	Purpose     valuePicker
	Mode        valuePicker
	OrderID     string
	LineIDs     []string
	ProductIDs  []string
	Settlement  string
	Reference   string
	Remarks     string
	Orders      []moneyOrderChoice
	Lines       []moneyLineChoice
	Products    []suggestion
	Settlements []moneySettlementChoice
	Removable   bool
}

type moneyOrderChoice struct {
	ID, Label string
	Selected  bool
}

type moneyLineChoice struct {
	ID, Label string
	Selected  bool
}

type moneySettlementChoice struct {
	Value, Label string
	Selected     bool
}

// moneyDraft is the whole Record money form. Several rows are how one
// settlement is split by purpose: a deposit, the rent, the freight and the
// labour are four events that happen to be paid at once.
type moneyDraft struct {
	CSRF     string
	Action   string
	Heading  string
	Submit   string
	MoveID   string
	Error    string
	Notice   string
	Rows     []moneyRow
	Editing  bool
	Products []suggestion
}

// readMoneyDraft reads the form back as typed. Nothing is resolved here:
// resolution happens inside the write.
func (s *Server) readMoneyDraft(r *http.Request) moneyDraft {
	_ = r.ParseForm()
	at := func(v []string, i int) string {
		if i < len(v) {
			return v[i]
		}
		return ""
	}
	directions := r.Form["direction"]
	amounts := r.Form["amount"]
	occurred := r.Form["occurredAt"]
	partyIDs, partyNames := r.Form["partyId"], r.Form["partyName"]
	purposeIDs, purposeNames := r.Form["purposeId"], r.Form["purposeName"]
	modeIDs, modeNames := r.Form["modeId"], r.Form["modeName"]
	partyChoices, partyNew := r.Form["partyIdChoice"], r.Form["partyNameNew"]
	purposeChoices, purposeNew := r.Form["purposeIdChoice"], r.Form["purposeNameNew"]
	modeChoices, modeNew := r.Form["modeIdChoice"], r.Form["modeNameNew"]
	orders := r.Form["orderId"]
	settlements := r.Form["settlement"]
	references, remarks := r.Form["reference"], r.Form["remarks"]

	count := len(amounts)
	for _, n := range []int{len(directions), len(occurred), len(partyNames), len(orders)} {
		if n > count {
			count = n
		}
	}
	if count == 0 {
		count = 1
	}

	d := moneyDraft{}
	for i := 0; i < count; i++ {
		direction := at(directions, i)
		if indexed, ok := r.Form[rowField("direction", i)]; ok {
			direction = at(indexed, 0)
		}
		row := moneyRow{
			Index:      i,
			Direction:  direction,
			Amount:     strings.TrimSpace(at(amounts, i)),
			OccurredAt: strings.TrimSpace(at(occurred, i)),
			OrderID:    at(orders, i),
			Settlement: at(settlements, i),
			Reference:  register.CleanName(at(references, i)),
			Remarks:    register.CleanName(at(remarks, i)),
			// Per-row multi-selects are named with the row index, because a
			// repeated name cannot say which row a checked box belongs to.
			LineIDs:    r.Form[rowField("lineIds", i)],
			ProductIDs: r.Form[rowField("productIds", i)],
		}
		row.Party.PickedID, row.Party.PickedText = at(partyIDs, i), register.CleanName(at(partyNames, i))
		row.Purpose.PickedID, row.Purpose.PickedText = at(purposeIDs, i), register.CleanName(at(purposeNames, i))
		row.Mode.PickedID, row.Mode.PickedText = at(modeIDs, i), register.CleanName(at(modeNames, i))
		applyNoScriptValue(&row.Party, at(partyChoices, i), at(partyNew, i))
		applyNoScriptValue(&row.Purpose, at(purposeChoices, i), at(purposeNew, i))
		applyNoScriptValue(&row.Mode, at(modeChoices, i), at(modeNew, i))
		d.Rows = append(d.Rows, row)
	}
	return d
}

func applyNoScriptValue(p *valuePicker, choice, typed string) {
	if typed = register.CleanName(typed); typed != "" {
		p.PickedID, p.PickedText = "", typed
	} else if choice != "" {
		p.PickedID, p.PickedText = choice, ""
	}
}

func rowField(name string, i int) string {
	return name + "-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	out := ""
	for i > 0 {
		out = string(rune('0'+i%10)) + out
		i /= 10
	}
	return out
}

// fillMoney puts the pickers, order choices and defaults on a draft ready to
// be drawn.
func (s *Server) fillMoney(d *moneyDraft, r *http.Request) {
	sess := financeSessionOf(r)
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		d.Products = matchProductsMode(reg, "", "all")
		for i := range d.Rows {
			row := &d.Rows[i]
			row.Index = i
			row.Removable = len(d.Rows) > 1
			if row.OccurredAt == "" {
				row.OccurredAt = s.now().Format("2006-01-02T15:04")
			}
			if row.Direction == "" {
				row.Direction = string(register.MoneyOut)
			}
			row.Party = pickerFor(f, register.FinanceParty, "Supplier or other party",
				"partyId", "partyName", row.Party.PickedID, row.Party.PickedText, "")
			row.Purpose = pickerFor(f, register.FinancePurpose, "Purpose",
				"purposeId", "purposeName", row.Purpose.PickedID, row.Purpose.PickedText, "")
			row.Mode = pickerFor(f, register.FinanceMode, "Payment mode",
				"modeId", "modeName", row.Mode.PickedID, row.Mode.PickedText, "")

			row.Orders = []moneyOrderChoice{}
			for _, o := range register.SortedFinanceOrders(f) {
				row.Orders = append(row.Orders, moneyOrderChoice{
					ID: o.ID, Selected: o.ID == row.OrderID,
					Label: register.FinanceValueText(f, o.PartyID) + " · " + lineText(o.Lines),
				})
			}
			row.Lines = []moneyLineChoice{}
			if o, ok := register.FinanceOrderByID(f, row.OrderID); ok {
				for _, l := range o.Lines {
					row.Lines = append(row.Lines, moneyLineChoice{
						ID: l.ID, Label: l.ProductNameSnapshot,
						Selected: containsID(row.LineIDs, l.ID),
					})
				}
			}
			row.Products = d.Products
			row.Settlements = settlementChoices(f, row.Settlement)
		}
	})
}

func settlementChoices(f *register.FinanceData, selected string) []moneySettlementChoice {
	var out []moneySettlementChoice
	for _, x := range f.SupplierReturns {
		value := "supplier_return:" + x.ID
		out = append(out, moneySettlementChoice{Value: value, Selected: value == selected,
			Label: "Supplier return " + x.ID + " — " + strconv.Itoa(x.Quantity()) + " " + x.Product.ProductName + " — " + register.FinanceValueText(f, x.PartyID)})
	}
	for _, x := range f.Sales {
		value := "sale:" + x.ID
		out = append(out, moneySettlementChoice{Value: value, Selected: value == selected,
			Label: "Sale " + x.ID + " — " + strconv.Itoa(x.Quantity()) + " " + x.Product.ProductName + " — " + register.FinanceValueText(f, x.BuyerPartyID)})
	}
	return out
}

func containsID(all []string, want string) bool {
	for _, v := range all {
		if v == want {
			return true
		}
	}
	return false
}

func (s *Server) renderMoneyForm(w http.ResponseWriter, r *http.Request, d moneyDraft) {
	sess := financeSessionOf(r)
	d.CSRF = sess.csrf
	if d.Heading == "" {
		d.Heading = "Record money"
	}
	if d.Submit == "" {
		d.Submit = "Save transaction"
	}
	if d.Action == "" {
		d.Action = "/finance/movements/new"
	}
	s.fillMoney(&d, r)
	s.financePage(w, r, d.Heading, "finance-money-form.html", d)
}

// financeMoneyNew is the Record money form and its save. Every row submitted
// together is checked completely before any of them is written, so a settlement
// entered as four amounts is four events or none.
func (s *Server) financeMoneyNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.renderMoneyForm(w, r, moneyDraft{Rows: []moneyRow{{}}})
		return
	}

	d := s.readMoneyDraft(r)
	if r.FormValue("addRow") != "" {
		// Adding a row saves nothing. It copies the shared party, time, mode
		// and reference down, because one settlement split by purpose repeats
		// all four.
		last := d.Rows[len(d.Rows)-1]
		d.Rows = append(d.Rows, moneyRow{
			Direction: last.Direction, OccurredAt: last.OccurredAt,
			Party:   valuePicker{PickedID: last.Party.PickedID, PickedText: last.Party.PickedText},
			Mode:    valuePicker{PickedID: last.Mode.PickedID, PickedText: last.Mode.PickedText},
			OrderID: last.OrderID, Settlement: last.Settlement, Reference: last.Reference,
		})
		s.renderMoneyForm(w, r, d)
		return
	}
	if at := r.FormValue("removeRow"); at != "" {
		if i := atoiSafe(at); i >= 0 && i < len(d.Rows) && len(d.Rows) > 1 {
			d.Rows = append(d.Rows[:i], d.Rows[i+1:]...)
		}
		s.renderMoneyForm(w, r, d)
		return
	}

	sess := financeSessionOf(r)
	now := s.now()
	refusal := ""
	firstID, saved := "", 0

	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		built := []register.MoneyMovement{}
		for _, row := range d.Rows {
			m, text := buildMovement(reg, f, row, sess.accountID, now)
			if text != "" {
				refusal = text
				return errMoneyRefused
			}
			m.ID = nextMovementID(f, len(built))
			built = append(built, m)
		}
		if len(built) == 0 {
			refusal = "Type at least one amount."
			return errMoneyRefused
		}
		f.Movements = append(f.Movements, built...)
		for _, m := range built {
			f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "movement_created", "movement", m.ID,
				movementSummary(f, m), "", ""))
		}
		// Every total the ledger shows must still add up after this write.
		if _, err := register.TotalMoney(f, nil); err != nil {
			refusal = register.ErrMoneyOverflow.Error()
			return errMoneyRefused
		}
		firstID, saved = built[0].ID, len(built)
		return nil
	})

	switch {
	case errors.Is(err, errMoneyRefused):
		d.Error = refusal
		s.renderMoneyForm(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderMoneyForm(w, r, d)
	default:
		q := "?saved=" + firstID + "&n=" + itoa(saved)
		http.Redirect(w, r, "/finance/journal"+q, http.StatusSeeOther)
	}
}

// nextMovementID numbers a row that is being appended alongside others in the
// same save, since none of them is in the vault yet.
func nextMovementID(f *register.FinanceData, ahead int) string {
	base := f.NextID("MOV")
	n := atoiSafe(strings.TrimPrefix(base, "MOV-"))
	return "MOV-" + pad4(n+ahead)
}

func atoiSafe(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return -1
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// buildMovement resolves one typed row against the vault inside the write. It
// creates any genuinely new party, purpose or mode as part of the same save.
func buildMovement(reg *register.Register, f *register.FinanceData, row moneyRow, actorID string, now time.Time) (register.MoneyMovement, string) {
	direction := register.MoneyDirection(row.Direction)
	if direction != register.MoneyOut && direction != register.MoneyIn {
		return register.MoneyMovement{}, "Choose whether money was paid or received."
	}
	amount, err := register.ParseRupees(row.Amount)
	if err != nil {
		return register.MoneyMovement{}, "Type the amount in rupees, using up to two decimal places."
	}
	occurredAt, err := time.ParseInLocation("2006-01-02T15:04", row.OccurredAt, time.Local)
	if err != nil {
		return register.MoneyMovement{}, "Pick the date and time of the transaction."
	}

	partyID, err := resolveValue(f, register.FinanceParty, row.Party.PickedID, row.Party.PickedText, actorID, now)
	if err != nil {
		return register.MoneyMovement{}, "Say who the money went to or came from."
	}
	purposeID, err := resolveValue(f, register.FinancePurpose, row.Purpose.PickedID, row.Purpose.PickedText, actorID, now)
	if err != nil {
		return register.MoneyMovement{}, "Say what the money was for."
	}
	modeID, err := resolveValue(f, register.FinanceMode, row.Mode.PickedID, row.Mode.PickedText, actorID, now)
	if err != nil {
		return register.MoneyMovement{}, "Say how the money was paid or received."
	}

	m := register.MoneyMovement{
		Direction: direction, AmountPaise: amount, OccurredAt: occurredAt,
		PartyID: partyID, PurposeID: purposeID, ModeID: modeID,
		Reference: row.Reference, Remarks: row.Remarks,
		RecordedAt: now, RecordedByID: actorID,
	}
	if row.Settlement != "" {
		kind, id, ok := strings.Cut(row.Settlement, ":")
		if !ok || !financeSettlementExists(f, kind, id) {
			return register.MoneyMovement{}, "Pick the related stock return or sale from the list."
		}
		m.Settlements = []register.FinanceSettlementRef{{Kind: kind, ID: id}}
	}

	if row.OrderID != "" {
		order, ok := register.FinanceOrderByID(f, row.OrderID)
		if !ok {
			return register.MoneyMovement{}, "Pick the order from the list."
		}
		m.OrderID = order.ID
		byID := map[string]register.FinanceOrderLine{}
		for _, l := range order.Lines {
			byID[l.ID] = l
		}
		seen := map[string]bool{}
		for _, id := range row.LineIDs {
			l, ok := byID[id]
			if !ok || seen[id] {
				return register.MoneyMovement{}, "Pick the products from the list."
			}
			seen[id] = true
			m.OrderLineIDs = append(m.OrderLineIDs, id)
			m.Products = append(m.Products, register.FinanceProductRef{
				ProductID: l.ProductID, ProductName: l.ProductNameSnapshot,
			})
		}
		if len(m.OrderLineIDs) == 0 {
			// No lines chosen means the whole order.
			for _, l := range order.Lines {
				m.Products = append(m.Products, register.FinanceProductRef{
					ProductID: l.ProductID, ProductName: l.ProductNameSnapshot,
				})
			}
		}
		return m, ""
	}

	if len(row.LineIDs) != 0 {
		return register.MoneyMovement{}, "Pick the order those products belong to."
	}
	seen := map[string]bool{}
	for _, id := range row.ProductIDs {
		p, ok := register.ProductByID(reg, id)
		if !ok || seen[id] {
			return register.MoneyMovement{}, "Pick the products from the list."
		}
		seen[id] = true
		m.Products = append(m.Products, register.FinanceProductRef{ProductID: p.ID, ProductName: p.Name})
	}
	return m, ""
}

// movementSummary is the one line an audit row carries.
func movementSummary(f *register.FinanceData, m register.MoneyMovement) string {
	parts := []string{
		register.DirectionText(m.Direction),
		register.FormatRupees(m.AmountPaise),
		register.FinanceValueText(f, m.PartyID),
		register.FinanceValueText(f, m.PurposeID),
		register.FinanceValueText(f, m.ModeID),
	}
	return strings.Join(parts, " · ")
}

// draftFromMovement fills the correction form with what is stored.
func (s *Server) draftFromMovement(r *http.Request, id string) (moneyDraft, bool) {
	sess := financeSessionOf(r)
	d := moneyDraft{
		MoveID: id, Editing: true, Heading: "Fix this transaction",
		Submit: "Save the correction", Action: "/finance/movements/" + id + "/edit",
	}
	found := false
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		m, ok := register.MovementByID(f, id)
		if !ok {
			return
		}
		found = true
		row := moneyRow{
			Direction: string(m.Direction), Amount: rupeeInput(m.AmountPaise),
			OccurredAt: m.OccurredAt.Format("2006-01-02T15:04"),
			OrderID:    m.OrderID, LineIDs: append([]string{}, m.OrderLineIDs...),
			Reference: m.Reference, Remarks: m.Remarks,
		}
		if len(m.Settlements) != 0 {
			row.Settlement = m.Settlements[0].Kind + ":" + m.Settlements[0].ID
		}
		row.Party.PickedID = m.PartyID
		row.Purpose.PickedID = m.PurposeID
		row.Mode.PickedID = m.ModeID
		for _, p := range m.Products {
			row.ProductIDs = append(row.ProductIDs, p.ProductID)
		}
		d.Rows = []moneyRow{row}
	})
	return d, found
}

// rupeeInput turns stored paise back into something the amount box accepts.
func rupeeInput(paise int64) string {
	return strconv.FormatInt(paise/100, 10) + "." + pad2(paise%100)
}

// financeMoneyEdit corrects one movement. Nothing about who recorded it, or
// when, is ever rewritten: only what they meant to type.
func (s *Server) financeMoneyEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := financeSessionOf(r)

	if r.Method != http.MethodPost {
		d, ok := s.draftFromMovement(r, id)
		if !ok {
			s.financeNotFound(w, r)
			return
		}
		s.renderMoneyForm(w, r, d)
		return
	}

	d := s.readMoneyDraft(r)
	d.MoveID, d.Editing, d.Heading = id, true, "Fix this transaction"
	d.Submit, d.Action = "Save the correction", "/finance/movements/"+id+"/edit"
	if len(d.Rows) != 1 {
		d.Rows = d.Rows[:1]
	}

	now := s.now()
	refusal := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) error {
		at := -1
		for i := range f.Movements {
			if f.Movements[i].ID == id {
				at = i
			}
		}
		if at < 0 {
			refusal = "That transaction is not on the list."
			return errMoneyRefused
		}
		before := f.Movements[at]
		if before.Voided != nil {
			refusal = "This transaction was voided. Record a new one instead."
			return errMoneyRefused
		}

		built, text := buildMovement(reg, f, d.Rows[0], sess.accountID, now)
		if text != "" {
			refusal = text
			return errMoneyRefused
		}
		after := before
		after.Direction, after.AmountPaise, after.OccurredAt = built.Direction, built.AmountPaise, built.OccurredAt
		after.PartyID, after.PurposeID, after.ModeID = built.PartyID, built.PurposeID, built.ModeID
		after.OrderID, after.OrderLineIDs, after.Products = built.OrderID, built.OrderLineIDs, built.Products
		after.Settlements = built.Settlements
		after.Reference, after.Remarks = built.Reference, built.Remarks

		changes := movementChanges(f, before, after, sess.accountID, now)
		if len(changes) == 0 {
			return nil
		}
		after.Changes = append(append([]register.FinanceChange{}, before.Changes...), changes...)
		f.Movements[at] = after
		f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "movement_edited", "movement", id,
			"Transaction corrected", movementSummary(f, before), movementSummary(f, after)))
		if _, err := register.TotalMoney(f, nil); err != nil {
			refusal = register.ErrMoneyOverflow.Error()
			return errMoneyRefused
		}
		return nil
	})

	switch {
	case errors.Is(err, errMoneyRefused):
		d.Error = refusal
		s.renderMoneyForm(w, r, d)
	case err != nil:
		d.Error = saveFailed
		s.renderMoneyForm(w, r, d)
	default:
		http.Redirect(w, r, "/finance/journal?corrected="+id, http.StatusSeeOther)
	}
}

// movementChanges records one change per changed field, in the order the spec
// fixes. Submitting the same values again records nothing and saves nothing.
func movementChanges(f *register.FinanceData, before, after register.MoneyMovement, actorID string, at time.Time) []register.FinanceChange {
	var out []register.FinanceChange
	var name, mobile string
	for _, a := range f.Accounts {
		if a.ID == actorID {
			name, mobile = a.DisplayName, a.Mobile
		}
	}
	add := func(field, label, from, to string) {
		if from == to {
			return
		}
		out = append(out, register.FinanceChange{
			At: at, ByAccountID: actorID, ByName: name, ByMobile: mobile,
			Field: field, Label: label, From: from, To: to,
		})
	}
	add("direction", "Money paid or received",
		register.DirectionText(before.Direction), register.DirectionText(after.Direction))
	add("amount", "Amount", register.FormatRupees(before.AmountPaise), register.FormatRupees(after.AmountPaise))
	add("occurredAt", "Date and time",
		before.OccurredAt.Format("2 January 2006 · 3:04 pm"), after.OccurredAt.Format("2 January 2006 · 3:04 pm"))
	add("party", "Supplier or other party",
		register.FinanceValueText(f, before.PartyID), register.FinanceValueText(f, after.PartyID))
	add("order", "Related order", orderRefText(f, before), orderRefText(f, after))
	add("products", "Related products", productRefText(before.Products), productRefText(after.Products))
	add("settlements", "Related stock return or sale", settlementRefText(f, before.Settlements), settlementRefText(f, after.Settlements))
	add("purpose", "Purpose",
		register.FinanceValueText(f, before.PurposeID), register.FinanceValueText(f, after.PurposeID))
	add("mode", "Payment mode",
		register.FinanceValueText(f, before.ModeID), register.FinanceValueText(f, after.ModeID))
	add("reference", "Reference", blankOr(before.Reference), blankOr(after.Reference))
	add("remarks", "Remarks", blankOr(before.Remarks), blankOr(after.Remarks))
	return out
}

func financeSettlementExists(f *register.FinanceData, kind, id string) bool {
	if kind == "supplier_return" {
		for _, x := range f.SupplierReturns {
			if x.ID == id {
				return true
			}
		}
	}
	if kind == "sale" {
		for _, x := range f.Sales {
			if x.ID == id {
				return true
			}
		}
	}
	return false
}

func settlementRefText(f *register.FinanceData, refs []register.FinanceSettlementRef) string {
	if len(refs) == 0 {
		return "Blank"
	}
	choices := settlementChoices(f, "")
	var out []string
	for _, ref := range refs {
		value := ref.Kind + ":" + ref.ID
		label := ref.ID
		for _, c := range choices {
			if c.Value == value {
				label = c.Label
			}
		}
		out = append(out, label)
	}
	return strings.Join(out, "; ")
}

func orderRefText(f *register.FinanceData, m register.MoneyMovement) string {
	if m.OrderID == "" {
		return "Blank"
	}
	o, ok := register.FinanceOrderByID(f, m.OrderID)
	if !ok {
		return m.OrderID
	}
	if len(m.OrderLineIDs) == 0 {
		return register.FinanceValueText(f, o.PartyID) + " · Whole order"
	}
	return register.FinanceValueText(f, o.PartyID)
}

// productRefText uses the snapshots, so a correction records what the row said
// at the time rather than what the catalogue says now.
func productRefText(refs []register.FinanceProductRef) string {
	if len(refs) == 0 {
		return "Blank"
	}
	var out []string
	for _, p := range refs {
		out = append(out, p.ProductName)
	}
	return strings.Join(out, "; ")
}

func blankOr(s string) string {
	if s == "" {
		return "Blank"
	}
	return s
}

// financeMoneyVoid marks an entry that should never have been recorded. It
// never removes the row: real money that came back is its own incoming row.
func (s *Server) financeMoneyVoid(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reason := register.CleanName(r.FormValue("reason"))
	sess := financeSessionOf(r)
	now := s.now()
	if r.FormValue("confirm") != "yes" {
		if _, ok := registerMovement(s, sess, id); !ok {
			s.financeNotFound(w, r)
			return
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Void this transaction?",
			Warning: "Use this only if this entry was a mistake. It will stop counting in totals, but it will stay in the journal and financial activity. If money really moved, record money in the opposite direction instead.",
			Action:  "/finance/movements/" + id + "/void", Button: "Yes, void this transaction",
			AskReason: true, ReasonLabel: "Why are you voiding this transaction?", Reason: reason,
		})
		return
	}

	if reason == "" {
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Void this transaction?",
			Warning: "Use this only if this entry was a mistake. It will stop counting in totals, but it will stay in the journal and financial activity. If money really moved, record money in the opposite direction instead.",
			Action:  "/finance/movements/" + id + "/void", Button: "Yes, void this transaction",
			AskReason: true, ReasonLabel: "Why are you voiding this transaction?", Error: "Say why you are voiding this transaction.",
		})
		return
	}
	refusal := ""
	err := s.st.UpdateFinance(sess.vaultKey, func(_ *register.Register, f *register.FinanceData) error {
		for i := range f.Movements {
			if f.Movements[i].ID != id {
				continue
			}
			if f.Movements[i].Voided != nil {
				refusal = "This transaction is already voided."
				return errMoneyRefused
			}
			var name, mobile string
			for _, a := range f.Accounts {
				if a.ID == sess.accountID {
					name, mobile = a.DisplayName, a.Mobile
				}
			}
			f.Movements[i].Voided = &register.FinanceVoid{
				At: now, ByAccountID: sess.accountID, ByName: name, ByMobile: mobile, Reason: reason,
			}
			f.Audit = append(f.Audit, financeAuditFor(f, sess.accountID, now, "movement_voided", "movement", id,
				reason, movementSummary(f, f.Movements[i]), "Voided"))
			return nil
		}
		refusal = "That transaction is not on the list."
		return errMoneyRefused
	})

	switch {
	case errors.Is(err, errMoneyRefused):
		s.renderJournal(w, r, refusal)
	case err != nil:
		s.renderJournal(w, r, saveFailed)
	default:
		http.Redirect(w, r, "/finance/journal?voided="+id, http.StatusSeeOther)
	}
}

func registerMovement(s *Server, sess *financeSession, id string) (register.MoneyMovement, bool) {
	var m register.MoneyMovement
	var ok bool
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) { m, ok = register.MovementByID(f, id) })
	return m, ok
}
