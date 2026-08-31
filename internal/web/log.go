package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// The activity log. Nothing on this page writes anything: it is one
// chronological list walked out of the records themselves, so it can never
// drift from what it describes.

// daystamp is a day heading: Thursday, 3 September. Only this page groups by
// day, so the function lives here and is registered here.
func daystamp(t time.Time) string { return t.Format("Monday, 2 January") }

// logRow is one line of the list.
type logRow struct {
	Time    string
	Who     string
	Main    string
	Struck  bool
	Notes   []string
	GoHref  string
	NoActor bool
}

// logDay is one day heading and the rows under it.
type logDay struct {
	Heading string
	Rows    []logRow
}

// logLink is one filter button.
type logLink struct {
	Label string
	Href  string
	On    bool
}

type logData struct {
	Day        string // the value in the date box; "" when every day is shown
	EveryDay   bool
	EveryHref  string
	TodayHref  string
	Today      string
	Kinds      []logLink
	Query      string
	AnybodyRef string
	Product    pickerData
	AnyProduct string
	Find       personPicker

	Days []logDay

	Empty       bool
	EmptyLine   string
	EmptyAdvice string
}

// logView draws the list. A parameter it cannot read falls back to that
// parameter's default: a bad value can only arrive from a hand-typed address,
// and there is nothing here to break.
func (s *Server) logView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := s.now()
	today := now.In(time.Local).Format(dateLayout)

	day := today
	switch asked := strings.TrimSpace(q.Get("day")); {
	case asked == "all":
		day = ""
	case asked == "":
	default:
		if _, err := time.ParseInLocation(dateLayout, asked, time.Local); err == nil {
			day = asked
		}
	}

	kind := logKind(q.Get("kind"))
	query := register.CleanName(q.Get("q"))
	productID := strings.TrimSpace(q.Get("productId"))

	data := logData{Day: day, EveryDay: day == "", Query: query, Today: today}

	p := s.page("Who did what")
	p.Current = "/log"
	s.st.Read(func(reg *register.Register) {
		if productID != "" && productNames(reg)[productID] == "" {
			productID = ""
		}
		filter := register.LogFilter{Day: day, ProductID: productID, Query: query}
		if kind != "" {
			filter.Kinds = []register.LogKind{kind}
		}
		entries := register.FilterLog(reg, register.LogEntries(reg), filter, time.Local)
		data.Days = logDays(reg, entries)

		data.Product = s.picker(reg, "all", false, productID)
		data.Product.Label = "Which product"
		data.Find = personPicker{
			Label: "Which person", Field: "q", Typed: query, Scope: "log",
			Hint: "Search by name, mobile or department.",
		}
	})

	base := url.Values{}
	if day == "" {
		base.Set("day", "all")
	} else if day != today {
		base.Set("day", day)
	}
	if kind != "" {
		base.Set("kind", string(kind))
	}
	if query != "" {
		base.Set("q", query)
	}
	if productID != "" {
		base.Set("productId", productID)
	}

	data.EveryHref = logHref(base, "day", "all")
	data.TodayHref = logHref(base, "day", "")
	data.Kinds = kindLinks(base, kind)
	if query != "" {
		data.AnybodyRef = logHref(base, "q", "")
	}
	if productID != "" {
		data.AnyProduct = logHref(base, "productId", "")
	}

	if len(data.Days) == 0 {
		data.Empty = true
		if kind == "" && query == "" && productID == "" && day != "" {
			when, err := time.ParseInLocation(dateLayout, day, time.Local)
			if err == nil {
				data.EmptyLine = "Nobody wrote anything down on " + daystamp(when) + "."
			}
			data.EmptyAdvice = "Pick another day, or tap Every day."
		} else {
			data.EmptyLine = "Nothing matches what you picked."
			data.EmptyAdvice = "Tap Every day, Everything, Anybody and Any product."
		}
	}

	s.render(w, http.StatusOK, p, "log.html", data)
}

// logKind reads the kind parameter. The five the desk named are the only ones
// with a button; anything else shows everything.
func logKind(asked string) register.LogKind {
	switch register.LogKind(strings.TrimSpace(asked)) {
	case register.LogCameIn:
		return register.LogCameIn
	case register.LogWentOut:
		return register.LogWentOut
	case register.LogCameBack:
		return register.LogCameBack
	case register.LogCorrected:
		return register.LogCorrected
	case register.LogDeleted:
		return register.LogDeleted
	}
	return ""
}

// kindLinks is the row of six. Each keeps every other active filter, so tapping
// one never quietly widens the list.
func kindLinks(base url.Values, active register.LogKind) []logLink {
	type pair struct {
		label string
		kind  register.LogKind
	}
	all := []pair{
		{"Everything", ""},
		{"Came in", register.LogCameIn},
		{"Went out", register.LogWentOut},
		{"Came back", register.LogCameBack},
		{"Entry fixed", register.LogCorrected},
		{"Entry deleted", register.LogDeleted},
	}
	links := make([]logLink, 0, len(all))
	for _, p := range all {
		links = append(links, logLink{
			Label: p.label,
			Href:  logHref(base, "kind", string(p.kind)),
			On:    p.kind == active,
		})
	}
	return links
}

// logHref is the current address with one parameter changed. An empty value
// clears that parameter.
func logHref(base url.Values, key, value string) string {
	next := url.Values{}
	for k, v := range base {
		next[k] = append([]string(nil), v...)
	}
	if value == "" {
		next.Del(key)
	} else {
		next.Set(key, value)
	}
	if len(next) == 0 {
		return "/log"
	}
	return "/log?" + next.Encode()
}

// logDays groups the rows under their day, newest day first. The entries are
// already in order, so the groups come out in order too.
func logDays(reg *register.Register, entries []register.LogEntry) []logDay {
	var days []logDay
	for _, e := range entries {
		heading := daystamp(e.At.In(time.Local))
		if len(days) == 0 || days[len(days)-1].Heading != heading {
			days = append(days, logDay{Heading: heading})
		}
		day := &days[len(days)-1]
		day.Rows = append(day.Rows, logRowOf(reg, e))
	}
	return days
}

// logRowOf is one entry in words. Every sentence a correction or a deletion is
// described in comes from corrections.go; this page is not a second source of
// truth for any of them.
func logRowOf(reg *register.Register, e register.LogEntry) logRow {
	word := productWord(e.ProductName)
	row := logRow{Time: clock(e.At), Who: e.Who}
	if e.Who == "" {
		row.NoActor = true
		row.Who = "nobody was on duty yet"
	}
	if e.RecordTab != "" {
		row.GoHref = e.RecordTab + "#" + e.RecordID
	}

	switch e.Kind {
	case register.LogCameIn:
		if e.Supplier == "" {
			row.Main = entryName(reg, e.RecordID)
		} else {
			row.Main = strconv.Itoa(e.Quantity) + " " + word + " came in from " + e.Supplier
		}
	case register.LogWentOut:
		row.Main = strconv.Itoa(e.Quantity) + " " + word + " went out to " + e.PersonName
	case register.LogCameBack:
		row.Main = strconv.Itoa(e.Quantity) + " " + word + " came back from " + e.PersonName
	case register.LogCorrected:
		row.Main = "Fixed this entry: " + entryName(reg, e.RecordID)
	case register.LogDeleted:
		row.Main = "Deleted this entry: " + entryName(reg, e.RecordID)
		row.Struck = true
	case register.LogProductAdded:
		row.Main = e.ProductName + " added to the product list."
	case register.LogPersonAdded:
		row.Main = e.PersonName + " added to the people list."
	}

	switch {
	case e.Kind == register.LogCorrected && e.Change != nil:
		row.Notes = append(row.Notes, changePhrase(*e.Change, word))
	case e.Kind == register.LogDeleted && e.Deletion != nil:
		row.Notes = append(row.Notes, "Deleted — "+e.Deletion.Reason)
	}
	if e.Kind == register.LogWentOut && !e.HappenedAt.Equal(e.At) {
		row.Notes = append(row.Notes, "Taken at "+clock(e.HappenedAt)+", typed in at "+clock(e.At)+".")
	}
	if e.Kind == register.LogCameBack && !e.HappenedAt.Equal(e.At) {
		row.Notes = append(row.Notes, "Came back at "+clock(e.HappenedAt)+", typed in at "+clock(e.At)+".")
	}
	if e.Kind == register.LogCameIn {
		if e.ReceivedOn != e.At.In(time.Local).Format(dateLayout) {
			row.Notes = append(row.Notes, "Received on "+shortdateOf(e.ReceivedOn)+".")
		}
		if e.ReceivedBy != "" && e.ReceivedBy != e.Who {
			row.Notes = append(row.Notes, "Received by "+e.ReceivedBy+".")
		}
	}
	if e.Kind == register.LogCameBack && e.ShortQuantity > 0 {
		if line := shortfallLine(returnOf(reg, e.RecordID)); line != "" {
			row.Notes = append(row.Notes, line)
		}
	}
	// A deleted arrival reads word for word like a live one. Typography is not
	// enough to tell them apart on the one page whose job is explaining a
	// number that looks wrong, so the row says it in words.
	if e.RecordDeleted && e.Kind != register.LogDeleted {
		row.Struck = true
		row.Notes = append(row.Notes, "This entry was deleted later.")
	}
	return row
}

func returnOf(reg *register.Register, id string) register.Return {
	for _, re := range reg.Returns {
		if re.ID == id {
			return re
		}
	}
	return register.Return{}
}
