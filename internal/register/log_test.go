package register

import (
	"reflect"
	"testing"
	"time"
)

func logIDs(entries []LogEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.RecordID
	}
	return ids
}

func entriesOfKind(entries []LogEntry, kind LogKind) []LogEntry {
	var out []LogEntry
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func logEntry(t *testing.T, entries []LogEntry, kind LogKind, id string) LogEntry {
	t.Helper()
	for _, e := range entries {
		if e.Kind == kind && e.RecordID == id {
			return e
		}
	}
	t.Fatalf("missing %s log entry for %s", kind, id)
	return LogEntry{}
}

func TestLogEntriesAtT0(t *testing.T) {
	entries := LogEntries(WalkthroughT0())
	if len(entries) != 21 {
		t.Fatalf("got %d entries, want 21", len(entries))
	}
	want := map[LogKind]int{LogProductAdded: 5, LogPersonAdded: 3, LogCameIn: 6, LogWentOut: 7}
	got := map[LogKind]int{}
	for _, e := range entries {
		got[e.Kind]++
		if string(e.Kind) == "shift_started" {
			t.Error("log contains a shift line")
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kind counts %+v, want %+v", got, want)
	}
}

func TestLogOrderIsNewestRecordedFirst(t *testing.T) {
	got := logIDs(LogEntries(WalkthroughT3()))
	want := []string{"RET-0001", "ISS-0008", "INW-0007", "ISS-0005", "ISS-0003", "ISS-0006"}
	if !reflect.DeepEqual(got[:6], want) {
		t.Errorf("first six %v, want %v", got[:6], want)
	}
}

func TestLogTieBreaksAcrossKinds(t *testing.T) {
	ids := logIDs(LogEntries(WalkthroughT0()))
	if indexOf(ids, "ISS-0002") >= indexOf(ids, "INW-0001") {
		t.Errorf("ISS-0002 does not precede INW-0001: %v", ids)
	}
}

func TestLogTieBreaksWithinAKind(t *testing.T) {
	entries := LogEntries(WalkthroughT0())
	ids := logIDs(entries)
	if indexOf(ids, "ISS-0005") >= indexOf(ids, "ISS-0003") {
		t.Error("ISS-0005 does not precede ISS-0003")
	}
	if got := logIDs(entriesOfKind(entries, LogProductAdded)); !reflect.DeepEqual(got, []string{"PRD-0005", "PRD-0004", "PRD-0003", "PRD-0002", "PRD-0001"}) {
		t.Errorf("product order %v", got)
	}
	if got := logIDs(entriesOfKind(entries, LogPersonAdded)); !reflect.DeepEqual(got, []string{"STF-0003", "STF-0002", "STF-0001"}) {
		t.Errorf("staff order %v", got)
	}
}

func indexOf(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return len(ids)
}

func TestLogIsDeterministic(t *testing.T) {
	reg := WalkthroughT3()
	want := LogEntries(reg)
	for i := 0; i < 50; i++ {
		if got := LogEntries(reg); !reflect.DeepEqual(got, want) {
			t.Fatalf("build %d differs", i+1)
		}
	}
	reversed := WalkthroughT3()
	reverseProducts(reversed.Products)
	reverseStaff(reversed.Staff)
	reverseInwards(reversed.Inwards)
	reverseIssues(reversed.Issues)
	if got := LogEntries(reversed); !reflect.DeepEqual(got, want) {
		t.Error("reversing source slices changed the log")
	}
}

func reverseProducts(s []Product) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
func reverseStaff(s []Staff) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
func reverseInwards(s []Inward) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
func reverseIssues(s []Issue) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func TestLogSortsOnRecordedNotHappened(t *testing.T) {
	reg := WalkthroughT2()
	reg.Issues[7].IssuedAt = time.Date(2026, time.September, 3, 9, 0, 0, 0, IST)
	entries := LogEntries(reg)
	ids := logIDs(entries)
	if indexOf(ids, "ISS-0008") >= indexOf(ids, "INW-0007") || indexOf(ids, "ISS-0008") >= indexOf(ids, "ISS-0003") {
		t.Errorf("backdated issue sorted on happened time: %v", ids[:5])
	}
	e := logEntry(t, entries, LogWentOut, "ISS-0008")
	if got := e.HappenedAt.Format("15:04"); got != "09:00" {
		t.Errorf("HappenedAt %s", got)
	}
	if got := e.At.Format("15:04"); got != "14:18" {
		t.Errorf("At %s", got)
	}
}

func TestLogShowsCorrectionsAsTheirOwnEntries(t *testing.T) {
	reg := WalkthroughT1()
	c := Change{At: time.Date(2026, time.September, 3, 10, 45, 0, 0, IST), By: "Suresh Kumar", Field: "quantity", Label: "How many", From: "500", To: "50"}
	reg.Inwards[6].Quantity = 50
	reg.Inwards[6].Changes = []Change{c}
	entries := LogEntries(reg)
	corrected := logEntry(t, entries, LogCorrected, "INW-0007")
	original := logEntry(t, entries, LogCameIn, "INW-0007")
	if !corrected.At.After(original.At) || corrected.Change == nil || !reflect.DeepEqual(*corrected.Change, c) || entries[0].Kind != LogCorrected {
		t.Errorf("corrected=%+v original=%+v", corrected, original)
	}
}

func TestLogKeepsTwoChangesFromOneEditInFieldOrder(t *testing.T) {
	reg := WalkthroughT1()
	at := time.Date(2026, time.September, 3, 10, 45, 0, 0, IST)
	reg.Inwards[6].Changes = []Change{
		{At: at, By: "Suresh Kumar", Field: "supplier", Label: "Came from", From: "Sharma Tent House", To: "Sharma Tent House & Sons"},
		{At: at, By: "Suresh Kumar", Field: "challan", Label: "Challan no.", From: "STH/4471", To: "STH/4472"},
	}
	changes := entriesOfKind(LogEntries(reg), LogCorrected)
	if len(changes) != 2 || changes[0].Change.Field != "supplier" || changes[1].Change.Field != "challan" || changes[0].ChangeIndex != 0 || changes[1].ChangeIndex != 1 {
		t.Errorf("change order %+v", changes)
	}
}

func TestLogShowsDeletedRecordsAndTheirDeletion(t *testing.T) {
	reg := WalkthroughT1()
	d := Deletion{At: time.Date(2026, time.September, 3, 10, 47, 0, 0, IST), By: "Suresh Kumar", Reason: "Entered twice by mistake."}
	reg.Inwards[1].Deleted = &d
	entries := LogEntries(reg)
	original := logEntry(t, entries, LogCameIn, "INW-0002")
	deleted := logEntry(t, entries, LogDeleted, "INW-0002")
	if !original.RecordDeleted || deleted.Deletion == nil || !reflect.DeepEqual(*deleted.Deletion, d) || !deleted.RecordDeleted {
		t.Errorf("original=%+v deleted=%+v", original, deleted)
	}
	if got := CameIn(reg, "PRD-0001"); got != 890 {
		// T1 has INW-0001 (390), deleted INW-0002, and INW-0007 (500).
		t.Errorf("CameIn chairs %d, want 890", got)
	}
}

func TestLogNamesTheActorPerKind(t *testing.T) {
	entries := LogEntries(WalkthroughT3())
	in := logEntry(t, entries, LogCameIn, "INW-0007")
	is := logEntry(t, entries, LogWentOut, "ISS-0008")
	re := logEntry(t, entries, LogCameBack, "RET-0001")
	anita := logEntry(t, entries, LogPersonAdded, "STF-0002")
	suresh := logEntry(t, entries, LogPersonAdded, "STF-0001")
	if in.Who != "Suresh Kumar" || in.WhoMobile != "98450 22117" ||
		is.Who != "Anita Rao" || is.WhoMobile != "99001 34562" || is.PersonName != "Ravi Menon" ||
		re.Who != "Imran Sheikh" || re.WhoMobile != "90080 77213" || re.PersonName != "Ravi Menon" {
		t.Errorf("stock actors: in=%+v issue=%+v return=%+v", in, is, re)
	}
	if anita.Who != "Suresh Kumar" || anita.WhoMobile != "98450 22117" || anita.PersonName != "Anita Rao" || anita.PersonMobile != "99001 34562" || suresh.Who != "" || suresh.WhoMobile != "" {
		t.Errorf("staff actors: Anita=%+v Suresh=%+v", anita, suresh)
	}
	for _, e := range entriesOfKind(entries, LogProductAdded) {
		if e.Who != "Suresh Kumar" || e.WhoMobile != "98450 22117" {
			t.Errorf("%s actor %q mobile %q", e.RecordID, e.Who, e.WhoMobile)
		}
	}
}

func TestFilterByDay(t *testing.T) {
	reg := WalkthroughT3()
	entries := LogEntries(reg)
	cases := []struct {
		day string
		ids []string
	}{
		{"2026-09-03", []string{"RET-0001", "ISS-0008", "INW-0007", "ISS-0005", "ISS-0003"}},
		{"2026-09-02", []string{"ISS-0006", "INW-0006", "ISS-0004", "INW-0005", "INW-0002"}},
		{"2026-09-01", []string{"INW-0004", "ISS-0007", "INW-0003", "ISS-0002", "INW-0001", "ISS-0001", "PRD-0005", "PRD-0004", "PRD-0003", "PRD-0002", "PRD-0001", "STF-0003", "STF-0002", "STF-0001"}},
	}
	for _, tc := range cases {
		if got := logIDs(FilterLog(reg, entries, LogFilter{Day: tc.day}, IST)); !reflect.DeepEqual(got, tc.ids) {
			t.Errorf("day %s: %v", tc.day, got)
		}
	}
	if got := len(FilterLog(reg, entries, LogFilter{}, IST)); got != 24 {
		t.Errorf("all days got %d", got)
	}
}

func TestFilterByKind(t *testing.T) {
	reg := WalkthroughT3()
	entries := LogEntries(reg)
	in := FilterLog(reg, entries, LogFilter{Kinds: []LogKind{LogCameIn}}, IST)
	if len(in) != 7 {
		t.Errorf("came in count %d", len(in))
	}
	for _, e := range in {
		if e.Kind != LogCameIn {
			t.Errorf("unexpected kind %s", e.Kind)
		}
	}
	if got := logIDs(FilterLog(reg, entries, LogFilter{Kinds: []LogKind{LogCameBack}}, IST)); !reflect.DeepEqual(got, []string{"RET-0001"}) {
		t.Errorf("came back %v", got)
	}
	if got := logIDs(FilterLog(reg, entries, LogFilter{Kinds: []LogKind{LogPersonAdded}}, IST)); !reflect.DeepEqual(got, []string{"STF-0003", "STF-0002", "STF-0001"}) {
		t.Errorf("people %v", got)
	}
}

func TestFilterByProduct(t *testing.T) {
	reg := WalkthroughT3()
	got := logIDs(FilterLog(reg, LogEntries(reg), LogFilter{ProductID: "PRD-0001"}, IST))
	// ISS-0005 is round tables, so it must not survive the Chairs filter.
	want := []string{"RET-0001", "ISS-0008", "INW-0007", "ISS-0003", "INW-0002", "ISS-0002", "INW-0001", "ISS-0001", "PRD-0001"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chairs entries %v", got)
	}
	for _, e := range FilterLog(reg, LogEntries(reg), LogFilter{ProductID: "PRD-0001"}, IST) {
		if e.Kind == LogPersonAdded {
			t.Error("person row survived product filter")
		}
	}
}

func TestFilterByPerson(t *testing.T) {
	reg := WalkthroughT3()
	entries := LogEntries(reg)
	cases := []struct {
		query string
		ids   []string
	}{
		{"98861", []string{"RET-0001", "ISS-0008", "ISS-0005", "ISS-0003"}},
		{"Imran", []string{"RET-0001", "INW-0005", "ISS-0007", "ISS-0002", "STF-0003"}},
		{"cater", []string{"ISS-0008", "ISS-0005", "ISS-0003"}},
		{"Ravi Menon", []string{"RET-0001", "ISS-0008", "ISS-0005", "ISS-0003"}},
		{"9886140023", []string{"RET-0001", "ISS-0008", "ISS-0005", "ISS-0003"}},
	}
	for _, tc := range cases {
		if got := logIDs(FilterLog(reg, entries, LogFilter{Query: tc.query}, IST)); !reflect.DeepEqual(got, tc.ids) {
			t.Errorf("query %q: %v", tc.query, got)
		}
	}
}

func TestFilterExcludesEntriesWithNoValueForTheDimension(t *testing.T) {
	reg := WalkthroughT3()
	entries := LogEntries(reg)
	if got := logIDs(FilterLog(reg, entries, LogFilter{ProductID: "PRD-0001", Query: "Anita Rao"}, IST)); !reflect.DeepEqual(got, []string{"ISS-0008"}) {
		t.Errorf("combined filter %v", got)
	}
	for _, e := range FilterLog(reg, entries, LogFilter{ProductID: "PRD-0002"}, IST) {
		if e.Kind == LogPersonAdded {
			t.Error("person row survived product filter")
		}
	}
}

func TestFiltersCombine(t *testing.T) {
	reg := WalkthroughT3()
	entries := LogEntries(reg)
	f := LogFilter{Day: "2026-09-03", Kinds: []LogKind{LogWentOut}, Query: "98861"}
	if got := logIDs(FilterLog(reg, entries, f, IST)); !reflect.DeepEqual(got, []string{"ISS-0008", "ISS-0005", "ISS-0003"}) {
		t.Errorf("combined %v", got)
	}
	f.ProductID = "PRD-0001"
	if got := logIDs(FilterLog(reg, entries, f, IST)); !reflect.DeepEqual(got, []string{"ISS-0008", "ISS-0003"}) {
		t.Errorf("combined product %v", got)
	}
}

func TestPeopleInLogIncludesStaffAndSettledTakers(t *testing.T) {
	reg := WalkthroughT3()
	reg.Returns = append(reg.Returns,
		Return{ID: "RET-0002", ProductID: "PRD-0001", Allocations: []Allocation{{IssueID: "ISS-0008", Quantity: 5}}, ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023"},
		Return{ID: "RET-0003", ProductID: "PRD-0002", Allocations: []Allocation{{IssueID: "ISS-0005", Quantity: 2}}, ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023"},
	)
	if got := FindPeople(reg, "98861"); len(got) != 0 {
		t.Errorf("settled Ravi still holding: %+v", got)
	}
	ravi := FindPeopleInLog(reg, "98861")
	if len(ravi) != 1 || ravi[0].Name != "Ravi Menon" || ravi[0].TotalOut != 0 || ravi[0].Lines != nil {
		t.Errorf("log Ravi %+v", ravi)
	}
	anita := FindPeopleInLog(reg, "Anita")
	if len(anita) != 1 || anita[0].Name != "Anita Rao" {
		t.Errorf("Anita %+v", anita)
	}
	people := PeopleInLog(WalkthroughT3())
	names := make([]string, len(people))
	for i, p := range people {
		names[i] = p.Name
		if p.TotalOut != 0 || p.Lines != nil {
			t.Errorf("scored person %+v", p)
		}
	}
	want := []string{"Anita Rao", "Farida Begum", "Imran Sheikh", "Joseph D'Cruz", "Lakshmi Iyer", "Ravi Menon", "Suresh Kumar"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("people %v", names)
	}

	// Display fields come from the most recent record, not slice order.
	outOfOrder := WalkthroughT3()
	outOfOrder.Issues[7].TakerDepartment = "Security"
	reverseIssues(outOfOrder.Issues)
	latest := FindPeopleInLog(outOfOrder, "98861")
	if len(latest) != 1 || latest[0].Department != "Security" {
		t.Errorf("latest Ravi display fields %+v", latest)
	}
}

func TestPeopleInLogIncludesTakersOnDeletedRecords(t *testing.T) {
	reg := WalkthroughT0()
	reg.Issues[0].Deleted = &Deletion{At: time.Date(2026, time.September, 3, 10, 47, 0, 0, IST), By: "Suresh Kumar", Reason: "Wrong."}
	got := FindPeopleInLog(reg, "Lakshmi")
	if len(got) != 1 || got[0].Name != "Lakshmi Iyer" {
		t.Errorf("Lakshmi %+v", got)
	}
}
