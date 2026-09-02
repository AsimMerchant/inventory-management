package web

import (
	"errors"
	"net/http"
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
	Index      int
	Direction  string
	Amount     string
	OccurredAt string
	Party      valuePicker
	Purpose    valuePicker
	Mode       valuePicker
	OrderID    string
	LineIDs    []string
	ProductIDs []string
	Reference  string
	Remarks    string
	Orders     []moneyOrderChoice
	Lines      []moneyLineChoice
	Products   []suggestion
	Removable  bool
}

type moneyOrderChoice struct {
	ID, Label string
	Selected  bool
}

type moneyLineChoice struct {
	ID, Label string
	Selected  bool
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
	orders := r.Form["orderId"]
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
		row := moneyRow{
			Index:      i,
			Direction:  at(directions, i),
			Amount:     strings.TrimSpace(at(amounts, i)),
			OccurredAt: strings.TrimSpace(at(occurred, i)),
			OrderID:    at(orders, i),
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
		d.Rows = append(d.Rows, row)
	}
	return d
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
		}
	})
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
			OrderID: last.OrderID, Reference: last.Reference,
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
		return register.MoneyMovement{}, "Say whether the money was paid or received."
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
		return register.MoneyMovement{}, "Say how the money was paid."
	}

	m := register.MoneyMovement{
		Direction: direction, AmountPaise: amount, OccurredAt: occurredAt,
		PartyID: partyID, PurposeID: purposeID, ModeID: modeID,
		Reference: row.Reference, Remarks: row.Remarks,
		RecordedAt: now, RecordedByID: actorID,
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
