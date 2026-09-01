package register

import (
	"sort"
	"strings"
	"time"
)

// This file is the only place in the package that reads a tombstone. Every
// function in arith.go skips deleted records, because a deleted record counts
// towards nothing; the activity log is the one screen where nothing is ever
// hidden, so its builder lives here and walks the raw slices. It takes no clock
// and does no I/O, exactly like arith.go.

// LogKind says what kind of thing happened.
type LogKind string

const (
	LogProductAdded LogKind = "product_added"
	LogPersonAdded  LogKind = "person_added"
	LogCameIn       LogKind = "came_in"
	LogWentOut      LogKind = "went_out"
	LogCameBack     LogKind = "came_back"
	LogCorrected    LogKind = "corrected"
	LogDeleted      LogKind = "deleted"
)

// LogEntry is one line of the activity log. It is derived from the records
// every time the page is drawn and is never stored: there is no log table in
// the file and no field in the register that holds one.
type LogEntry struct {
	At            time.Time // when it was recorded - the field the list sorts on
	Kind          LogKind
	RecordID      string // "INW-0007", "ISS-0008", "RET-0001", "PRD-0003", "STF-0002"
	RecordTab     string // "/inwards" or "/out"; "" when the entry has no record page
	RecordDeleted bool   // the record this row is about carries a tombstone

	Who       string // the actor; "" only when CreatedBy was empty
	WhoMobile string // the actor's mobile when known

	PersonName       string // the subject person; "" when the event has none
	PersonMobile     string
	PersonDepartment string
	Recipients       []IssueRecipient

	ProductID   string // "" only for LogPersonAdded
	ProductName string
	Quantity    int // 0 for the added, corrected and deleted kinds

	Supplier   string // LogCameIn only
	ReceivedBy string // LogCameIn only

	HappenedAt time.Time // LogWentOut: IssuedAt. LogCameBack: ReturnedAt.
	ReceivedOn string    // LogCameIn only, "2026-09-03"

	ShortQuantity    int         // LogCameBack only
	ShortDisposition Disposition // LogCameBack only
	Remark           string      // LogCameBack only

	Change      *Change   // set only when Kind == LogCorrected
	ChangeIndex int       // index within the record's Changes slice
	Deletion    *Deletion // set only when Kind == LogDeleted
}

// LogEntries is everything that has ever happened at the desk, newest first.
// Deleted records are in the list, marked, along with the deletion itself.
func LogEntries(r *Register) []LogEntry {
	names := productNames(r)
	mobiles := staffMobiles(r)
	var out []LogEntry

	for _, p := range r.Products {
		e := LogEntry{
			At: p.CreatedAt, Kind: LogProductAdded, RecordID: p.ID,
			Who: p.CreatedBy, ProductID: p.ID, ProductName: p.Name,
		}
		out = append(out, fill(r, e, nil, mobiles))
	}
	for _, st := range r.Staff {
		e := LogEntry{
			At: st.CreatedAt, Kind: LogPersonAdded, RecordID: st.ID,
			Who: st.CreatedBy, PersonName: st.Name, PersonMobile: st.Mobile,
		}
		out = append(out, fill(r, e, nil, mobiles))
	}
	for _, in := range r.Inwards {
		e := LogEntry{
			At: in.RecordedAt, Kind: LogCameIn, RecordID: in.ID, RecordTab: "/inwards",
			Who: in.RecordedBy, ProductID: in.ProductID, ProductName: names[in.ProductID],
			Quantity: in.Quantity, Supplier: in.Supplier, ReceivedBy: in.ReceivedBy,
			ReceivedOn: in.ReceivedOn,
		}
		out = append(out, fill(r, e, in.Deleted, mobiles))
		out = append(out, changesOf(r, e, in.Changes, in.Deleted, mobiles)...)
	}
	for _, is := range r.Issues {
		e := LogEntry{
			At: is.RecordedAt, Kind: LogWentOut, RecordID: is.ID, RecordTab: "/out",
			Who: is.PersonInchargeName, WhoMobile: is.PersonInchargeMobile,
			PersonName: is.TakerName, PersonMobile: is.TakerMobile,
			PersonDepartment: is.TakerDepartment,
			Recipients:       RecipientsOf(is),
			ProductID:        is.ProductID, ProductName: names[is.ProductID],
			Quantity: is.Quantity, HappenedAt: is.IssuedAt,
		}
		out = append(out, fill(r, e, is.Deleted, mobiles))
		out = append(out, changesOf(r, e, is.Changes, is.Deleted, mobiles)...)
	}
	for _, re := range r.Returns {
		e := LogEntry{
			At: re.RecordedAt, Kind: LogCameBack, RecordID: re.ID, RecordTab: "/out",
			Who: re.TakenBackBy, PersonName: re.ReturnerName, PersonMobile: re.ReturnerMobile,
			ProductID: re.ProductID, ProductName: names[re.ProductID],
			Quantity: re.Quantity(), HappenedAt: re.ReturnedAt,
			ShortQuantity: re.ShortQuantity, ShortDisposition: re.ShortDisposition,
			Remark: re.Remark,
		}
		out = append(out, fill(r, e, re.Deleted, mobiles))
		out = append(out, changesOf(r, e, re.Changes, re.Deleted, mobiles)...)
	}

	sortLog(out)
	return out
}

// fill finishes a record's own row: the actor's mobile where the register knows
// it, and the tombstone mark that every row about a deleted record carries.
func fill(r *Register, e LogEntry, deleted *Deletion, mobiles map[string]string) LogEntry {
	if e.WhoMobile == "" {
		e.WhoMobile = mobiles[FoldKey(e.Who)]
	}
	e.RecordDeleted = deleted != nil
	if deleted != nil && e.Kind == LogDeleted {
		e.Deletion = deleted
	}
	return e
}

// changesOf is the corrected and deleted rows belonging to one record. They
// keep the record's product and subject person, so a filter on either finds
// them.
func changesOf(r *Register, base LogEntry, changes []Change, deleted *Deletion,
	mobiles map[string]string) []LogEntry {

	var out []LogEntry
	for i := range changes {
		c := changes[i]
		e := base
		e.Kind, e.At, e.Who, e.WhoMobile = LogCorrected, c.At, c.By, ""
		e.Quantity, e.HappenedAt = 0, time.Time{}
		e.Supplier, e.ReceivedBy, e.ReceivedOn = "", "", ""
		e.ShortQuantity, e.ShortDisposition, e.Remark = 0, "", ""
		e.Change, e.ChangeIndex = &c, i
		out = append(out, fill(r, e, deleted, mobiles))
	}
	if deleted != nil {
		e := base
		e.Kind, e.At, e.Who, e.WhoMobile = LogDeleted, deleted.At, deleted.By, ""
		e.Quantity, e.HappenedAt = 0, time.Time{}
		e.Supplier, e.ReceivedBy, e.ReceivedOn = "", "", ""
		e.ShortQuantity, e.ShortDisposition, e.Remark = 0, "", ""
		out = append(out, fill(r, e, deleted, mobiles))
	}
	return out
}

// staffMobiles is the staff list keyed by folded name, so a row can name the
// actor's mobile without a second lookup at the screen.
func staffMobiles(r *Register) map[string]string {
	out := make(map[string]string, len(r.Staff))
	for _, st := range r.Staff {
		if st.Mobile != "" {
			out[FoldKey(st.Name)] = st.Mobile
		}
	}
	return out
}

// logRank orders the kinds within one instant. It sorts descending, so a thing
// that happened to stock sits above the thing that created the record it
// happened to.
func logRank(k LogKind) int {
	switch k {
	case LogProductAdded:
		return 1
	case LogPersonAdded:
		return 2
	case LogCameIn:
		return 3
	case LogWentOut:
		return 4
	case LogCameBack:
		return 5
	case LogCorrected:
		return 6
	case LogDeleted:
		return 7
	}
	return 0
}

// sortLog is newest first, then kind, then record, then - the one deliberate
// exception - the changes of a single edit in the order the form is laid out.
func sortLog(entries []LogEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		if ra, rb := logRank(a.Kind), logRank(b.Kind); ra != rb {
			return ra > rb
		}
		if a.RecordID != b.RecordID {
			return a.RecordID > b.RecordID
		}
		return a.ChangeIndex < b.ChangeIndex
	})
}

// LogFilter is the four things the page can be narrowed by. They combine: an
// entry is shown only if it matches every active one, and an entry that has no
// value for a filtered dimension does not match.
type LogFilter struct {
	Day       string // "2026-09-03", or "" meaning every day
	Kinds     []LogKind
	ProductID string
	Query     string
}

// FilterLog narrows the log. loc is the zone Day is read in; no clock is read
// here.
func FilterLog(r *Register, entries []LogEntry, f LogFilter, loc *time.Location) []LogEntry {
	out := []LogEntry{}
	for _, e := range entries {
		if f.Day != "" && e.At.In(loc).Format("2006-01-02") != f.Day {
			continue
		}
		if len(f.Kinds) > 0 && !hasKind(f.Kinds, e.Kind) {
			continue
		}
		if f.ProductID != "" && e.ProductID != f.ProductID {
			continue
		}
		if !matchesQuery(e, f.Query) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func hasKind(kinds []LogKind, k LogKind) bool {
	for _, want := range kinds {
		if want == k {
			return true
		}
	}
	return false
}

// matchesQuery finds a row by any name, department or mobile on it - the person
// who did it as readily as the person it was done for.
func matchesQuery(e LogEntry, query string) bool {
	text := FoldKey(query)
	if text == "" {
		return true
	}
	digits := MobileKey(query)
	if strings.Contains(FoldKey(e.Who), text) ||
		strings.Contains(FoldKey(e.PersonName), text) ||
		strings.Contains(FoldKey(e.PersonDepartment), text) ||
		strings.Contains(FoldKey(e.ReceivedBy), text) {
		return true
	}
	for _, recipient := range e.Recipients {
		if strings.Contains(FoldKey(recipient.Name), text) || strings.Contains(FoldKey(recipient.Department), text) || (digits != "" && strings.Contains(MobileKey(recipient.Mobile), digits)) {
			return true
		}
	}
	if digits == "" {
		return false
	}
	return strings.Contains(MobileKey(e.WhoMobile), digits) ||
		strings.Contains(MobileKey(e.PersonMobile), digits)
}

// PeopleInLog is the whole cast of the log: every staff member and everybody
// who has ever taken or handed back anything, deleted records included. It
// keeps no score, so TotalOut is always 0 - the person being looked for on this
// page has usually handed everything back already.
func PeopleInLog(r *Register) []PersonSummary {
	type personState struct {
		person                   PersonSummary
		nameAt, mobileAt, deptAt time.Time
	}
	var order []PersonID
	found := map[PersonID]*personState{}

	take := func(name, mobile, department string, at time.Time) {
		id := PersonOf(name, mobile)
		p, seen := found[id]
		if !seen {
			p = &personState{person: PersonSummary{ID: id}}
			found[id] = p
			order = append(order, id)
		}
		// Field by field, so a returner's blank department does not wipe out
		// the one their issue recorded.
		if name != "" && (p.nameAt.IsZero() || at.After(p.nameAt)) {
			p.person.Name, p.nameAt = name, at
		}
		if mobile != "" && (p.mobileAt.IsZero() || at.After(p.mobileAt)) {
			p.person.Mobile, p.mobileAt = mobile, at
		}
		if department != "" && (p.deptAt.IsZero() || at.After(p.deptAt)) {
			p.person.Department, p.deptAt = department, at
		}
	}

	for _, st := range r.Staff {
		take(st.Name, st.Mobile, "", st.CreatedAt)
	}
	for _, is := range r.Issues {
		for _, recipient := range RecipientsOf(is) {
			take(recipient.Name, recipient.Mobile, recipient.Department, is.RecordedAt)
		}
	}
	for _, re := range r.Returns {
		take(re.ReturnerName, re.ReturnerMobile, "", re.RecordedAt)
	}

	people := make([]PersonSummary, 0, len(order))
	for _, id := range order {
		people = append(people, found[id].person)
	}
	sort.Slice(people, func(i, j int) bool {
		a, b := FoldKey(people[i].Name), FoldKey(people[j].Name)
		if a != b {
			return a < b
		}
		return people[i].ID.MobileKey < people[j].ID.MobileKey
	})
	return people
}

// FindPeopleInLog is FindPeople over that wider population: the same three
// matching rules, so the desk meets one picker in this program and not two.
func FindPeopleInLog(r *Register, query string) []PersonSummary {
	people := PeopleInLog(r)
	text := FoldKey(query)
	if text == "" {
		return people
	}
	digits := MobileKey(query)

	var found []PersonSummary
	for _, p := range people {
		switch {
		case strings.Contains(FoldKey(p.Name), text),
			strings.Contains(FoldKey(p.Department), text),
			digits != "" && strings.Contains(MobileKey(p.Mobile), digits):
			found = append(found, p)
		}
	}
	return found
}
