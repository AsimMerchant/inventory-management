package web

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"storeregister/internal/register"
)

// movementView is one movement as a screen or a printed page shows it: values
// resolved to what they now say, product names as they were at the time.
type movementView struct {
	ID          string
	Direction   string
	Amount      string
	OccurredAt  time.Time
	Party       string
	Purpose     string
	Mode        string
	Order       string
	OrderID     string
	Products    string
	Reference   string
	Remarks     string
	RecordedAt  time.Time
	RecordedBy  string
	RecordedMob string
	Changes     []register.FinanceChange
	Voided      *register.FinanceVoid
	VoidLine    string
}

func viewMovement(f *register.FinanceData, m register.MoneyMovement) movementView {
	v := movementView{
		ID: m.ID, Direction: register.DirectionText(m.Direction),
		Amount: register.FormatRupees(m.AmountPaise), OccurredAt: m.OccurredAt,
		Party:   register.FinanceValueText(f, m.PartyID),
		Purpose: register.FinanceValueText(f, m.PurposeID),
		Mode:    register.FinanceValueText(f, m.ModeID),
		OrderID: m.OrderID, Reference: m.Reference, Remarks: m.Remarks,
		RecordedAt: m.RecordedAt, Changes: m.Changes, Voided: m.Voided,
	}
	if m.OrderID != "" {
		v.Order = orderRefText(f, m)
	}
	if len(m.Products) != 0 {
		v.Products = productRefText(m.Products)
	}
	for _, a := range f.Accounts {
		if a.ID == m.RecordedByID {
			v.RecordedBy, v.RecordedMob = a.DisplayName, a.Mobile
		}
	}
	if m.Voided != nil {
		v.VoidLine = "Voided — " + m.Voided.Reason
	}
	return v
}

type journalData struct {
	CSRF, Error, Notice string
	Heading             string
	FilterLabel         string
	Day, From, To       string
	FromTime, ToTime    string
	PrintLink           string
	PrintedAt           string
	Rows                []movementView
	Totals              totalsView
	Print               bool
}

type totalsView struct {
	Paid, Received, Net string
}

func money(paise int64) string { return register.FormatRupees(paise) }

// financeJournal is the protected list of every transaction.
func (s *Server) financeJournal(w http.ResponseWriter, r *http.Request) {
	s.renderJournal(w, r, "")
}

func (s *Server) renderJournal(w http.ResponseWriter, r *http.Request, problem string) {
	s.journalPage(w, r, problem, false)
}

// financeJournalPrint is the same journal on paper: oldest first, no chrome.
func (s *Server) financeJournalPrint(w http.ResponseWriter, r *http.Request) {
	s.journalPage(w, r, "", true)
}

func (s *Server) journalPage(w http.ResponseWriter, r *http.Request, problem string, printing bool) {
	sess := financeSessionOf(r)
	q := r.URL.Query()
	data := journalData{
		CSRF: sess.csrf, Error: problem, Print: printing,
		Heading:  "Transaction journal",
		Day:      q.Get("day"),
		From:     q.Get("from"),
		To:       q.Get("to"),
		FromTime: q.Get("fromTime"),
		ToTime:   q.Get("toTime"),
	}
	if saved := q.Get("saved"); saved != "" {
		data.Notice = "Transaction saved."
		if n := q.Get("n"); n != "" && n != "1" {
			data.Notice = n + " transactions saved."
		}
	}
	if q.Get("corrected") != "" {
		data.Notice = "Transaction corrected."
	}
	if q.Get("voided") != "" {
		data.Notice = "Transaction voided."
	}

	filter, err := register.ParseJournalFilter(data.Day, data.From, data.To, data.FromTime, data.ToTime, time.Local)
	if err != nil {
		// A filter that will not parse shows the whole journal with the one
		// refusal, rather than an empty page that looks like missing data.
		data.Error = err.Error()
		filter = register.JournalFilter{Every: true, Label: "Every date"}
	}
	data.FilterLabel = filter.Label

	keep := url.Values{}
	for _, name := range []string{"day", "from", "to", "fromTime", "toTime"} {
		if v := q.Get(name); v != "" {
			keep.Set(name, v)
		}
	}
	data.PrintLink = "/finance/journal/print"
	if len(keep) != 0 {
		data.PrintLink += "?" + keep.Encode()
	}
	data.PrintedAt = s.now().Format("Monday, 2 January 2006 · 3:04 pm")

	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		var kept []register.MoneyMovement
		for _, m := range f.Movements {
			if filter.Keep(m) {
				kept = append(kept, m)
			}
		}
		ordered := register.SortedMovements(kept)
		if printing {
			ordered = register.AscendingMovements(kept)
		}
		for _, m := range ordered {
			data.Rows = append(data.Rows, viewMovement(f, m))
		}
		if t, err := register.TotalMoney(f, filter.Keep); err == nil {
			data.Totals = totalsView{money(t.PaidPaise), money(t.ReceivedPaise), money(t.NetPaise)}
		}
	})

	name := "finance-journal.html"
	if printing {
		name = "finance-journal-print.html"
	}
	s.financePage(w, r, data.Heading, name, data)
}

type dashboardData struct {
	CSRF   string
	Admin  bool
	Who    string
	Mobile string
	Role   string
	Totals totalsView
	Recent []movementView
}

// financeDashboard is the protected home screen.
func (s *Server) financeDashboard(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	data := dashboardData{CSRF: sess.csrf, Admin: s.sessionIsAdmin(sess)}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		for _, a := range f.Accounts {
			if a.ID == sess.accountID {
				data.Who, data.Mobile = a.DisplayName, a.Mobile
				data.Role = "Financial user"
				if a.Role == register.FinanceAdmin {
					data.Role = "Administrator"
				}
			}
		}
		if t, err := register.TotalMoney(f, nil); err == nil {
			data.Totals = totalsView{money(t.PaidPaise), money(t.ReceivedPaise), money(t.NetPaise)}
		}
		rows := register.SortedMovements(f.Movements)
		if len(rows) > 10 {
			rows = rows[:10]
		}
		for _, m := range rows {
			data.Recent = append(data.Recent, viewMovement(f, m))
		}
	})
	s.financePage(w, r, "Financial ledger", "finance-home.html", data)
}

type auditRow struct {
	At       time.Time
	Kind     string
	Entity   string
	Summary  string
	Before   string
	After    string
	ByName   string
	ByMobile string
}

type auditData struct {
	Rows []auditRow
}

// financeAudit is the immutable list of everything anybody did. It has no edit
// or delete route: that is the whole point of it.
func (s *Server) financeAudit(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	data := auditData{}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		for _, a := range register.SortedAudit(f) {
			data.Rows = append(data.Rows, auditRow{
				At: a.At, Kind: auditWording(a.Kind), Entity: a.EntityID,
				Summary: a.Summary, Before: a.Before, After: a.After,
				ByName: a.ByName, ByMobile: a.ByMobile,
			})
		}
	})
	s.financePage(w, r, "Financial activity", "finance-audit.html", data)
}

// auditWording turns a stored kind into the sentence a person reads. The
// stored kinds are contract; the wording is what the screen says.
func auditWording(kind string) string {
	switch kind {
	case "account_created":
		return "Authorized account created"
	case "account_disabled":
		return "Account switched off"
	case "account_reset":
		return "Account access reset"
	case "account_edited":
		return "Account details corrected"
	case "password_changed":
		return "Password changed"
	case "recovery_replaced":
		return "Recovery key replaced"
	case "value_created":
		return "Shared value added"
	case "value_renamed":
		return "Shared value corrected"
	case "value_merged":
		return "Shared values merged"
	case "value_deleted":
		return "Shared value removed"
	case "order_created":
		return "Order recorded"
	case "order_edited":
		return "Order corrected"
	case "order_cancelled":
		return "Order cancelled"
	case "product_created":
		return "Product added"
	case "movement_created":
		return "Money recorded"
	case "movement_edited":
		return "Money corrected"
	case "movement_voided":
		return "Money voided"
	}
	return strings.ReplaceAll(kind, "_", " ")
}

// financeNotFound is the protected 404. It says only that the record was not
// found, and shows no neighbouring decrypted data.
func (s *Server) financeNotFound(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	p := page{Title: "Not found", Tabs: false, Finance: true, CSRF: ""}
	if sess != nil {
		p.FinanceAdmin, p.CSRF = s.sessionIsAdmin(sess), sess.csrf
	}
	s.render(w, http.StatusNotFound, p, "finance-notfound.html", nil)
}
