package web

import (
	"html"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

var logT4 = time.Date(2026, time.September, 3, 18, 10, 0, 0, register.IST)

func TestLogNamesEveryJointRecipient(t *testing.T) {
	e := newTestServer(t, jointReturnRegister(), logT4)
	for _, query := range []string{"Ravi", "Amit", "Suresh", "9774011298"} {
		_, body := e.get("/log?day=all&q=" + url.QueryEscape(query))
		assertContains(t, body, "30 chairs went out to Ravi Menon, Amit Sharma and Suresh Patel")
		if got := strings.Count(html.UnescapeString(body), "30 chairs went out to Ravi Menon, Amit Sharma and Suresh Patel"); got != 1 {
			t.Fatalf("query %q showed %d rows", query, got)
		}
	}
}

func TestLogTabIsInTheChromeBar(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, stock := e.get("/stock")
	for _, label := range []string{"Stock", "Out with people", "Stuff came in", "Suppliers", "Who did what"} {
		assertContains(t, stock, label)
	}
	assertContains(t, stock, `href="/log"`)
	resp, body := e.get("/log")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /log returned %d", resp.StatusCode)
	}
	assertContains(t, body, `<a class="tab on" href="/log">Who did what</a>`)
	assertContains(t, body, `<span class="lab">Which product</span>`)
}

func TestLogDefaultsToToday(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log")
	assertContains(t, body, "Thursday, 3 September")
	want := []string{
		"45 chairs came back from Ravi Menon",
		"10 chairs went out to Ravi Menon",
		"500 chairs came in from Sharma Tent House",
		"2 round tables went out to Ravi Menon",
		"40 chairs went out to Ravi Menon",
	}
	last := -1
	for _, line := range want {
		i := strings.Index(html.UnescapeString(body), line)
		if i < 0 || i <= last {
			t.Errorf("%q missing or out of order", line)
		}
		last = i
	}
	if got := strings.Count(body, "Go to this entry"); got != 5 {
		t.Errorf("today has %d rows, want 5", got)
	}
	for _, absent := range []string{"#INW-0001", "Wednesday, 2 September", "added to the product list", "added to the people list"} {
		assertNotContains(t, body, absent)
	}
}

func TestLogShortfallLineMatchesTheOutTab(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	want := "Won't come back: 5 chairs broke during setup near the stage. Ravi informed."
	_, logBody := e.get("/log")
	_, outBody := e.get("/out")
	assertContains(t, logBody, want)
	assertContains(t, outBody, want)
}

func TestLogLinksToTheRecordOnItsTab(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log")
	for _, href := range []string{"/out#RET-0001", "/out#ISS-0008", "/inwards#INW-0007"} {
		assertContains(t, body, `href="`+href+`"`)
	}
	if strings.Count(body, "Go to this entry") != 5 {
		t.Error("not every today row links to its record")
	}
	assertNotContains(t, body, "/entry/")
	_, inwards := e.get("/inwards")
	assertContains(t, inwards, `id="INW-0007"`)
	_, out := e.get("/out")
	assertContains(t, out, `id="ISS-0008"`)
	assertContains(t, out, `id="RET-0001"`)
}

func TestLogEveryDayShowsEveryDay(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=all")
	headings := []string{"Thursday, 3 September", "Wednesday, 2 September", "Tuesday, 1 September"}
	last := -1
	for _, h := range headings {
		i := strings.Index(body, h)
		if i < 0 || i <= last {
			t.Errorf("heading %q missing or out of order", h)
		}
		last = i
	}
	assertContains(t, body, "Chairs added to the product list.")
	assertContains(t, body, "Anita Rao added to the people list.")
	assertNotContains(t, body, "came on duty")
}

func TestLogRowForTheFirstPersonHasNoWho(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=all")
	first := tableRow(t, body, "Suresh Kumar added to the people list.")
	if !strings.Contains(first, "nobody was on duty yet") {
		t.Errorf("first staff row: %s", first)
	}
	anita := tableRow(t, body, "Anita Rao added to the people list.")
	if !strings.Contains(anita, "Suresh Kumar") {
		t.Errorf("Anita row: %s", anita)
	}
}

func TestLogShowsBothTimesWhereTheyDiffer(t *testing.T) {
	reg := register.WalkthroughT3()
	reg.Issues[7].IssuedAt = time.Date(2026, time.September, 3, 13, 5, 0, 0, register.IST)
	e := newTestServer(t, reg, logT4)
	_, body := e.get("/log")
	row := tableRow(t, body, "10 chairs went out to Ravi Menon")
	for _, want := range []string{"2:18 pm", "Taken at 1:05 pm, typed in at 2:18 pm."} {
		if !strings.Contains(row, want) {
			t.Errorf("issue row lacks %q", want)
		}
	}
	clean := newTestServer(t, register.WalkthroughT3(), logT4)
	_, unchanged := clean.get("/log")
	assertNotContains(t, unchanged, "Taken at ")
	assertNotContains(t, unchanged, "Came back at ")
}

func TestLogShowsADifferentReceivedDate(t *testing.T) {
	reg := register.WalkthroughT1()
	reg.Inwards[6].ReceivedOn = "2026-09-04"
	e := newTestServer(t, reg, logT4)
	_, body := e.get("/log")
	assertContains(t, body, "Received on 4 September.")
	clean := newTestServer(t, register.WalkthroughT1(), logT4)
	_, same := clean.get("/log")
	assertNotContains(t, same, "Received on 3 September.")
}

func TestLogShowsADifferentReceiver(t *testing.T) {
	reg := register.WalkthroughT1()
	reg.Inwards[6].ReceivedBy = "Anita Rao"
	e := newTestServer(t, reg, logT4)
	_, body := e.get("/log")
	row := tableRow(t, body, "500 chairs came in from Sharma Tent House")
	for _, want := range []string{"Suresh Kumar", "Received by Anita Rao."} {
		if !strings.Contains(row, want) {
			t.Errorf("row lacks %q", want)
		}
	}
}

func TestLogCorrectionPhraseMatchesTheInwardsTab(t *testing.T) {
	reg := register.WalkthroughT1()
	at := time.Date(2026, time.September, 3, 10, 45, 0, 0, register.IST)
	reg.Inwards[6].Quantity = 50
	reg.Inwards[6].Changes = []register.Change{{At: at, By: "Suresh Kumar", Field: "quantity", Label: "How many", From: "500", To: "50"}}
	e := newTestServer(t, reg, logT4)
	_, logBody := e.get("/log")
	_, inBody := e.get("/inwards")
	row := tableRow(t, logBody, "Fixed this entry: 50 chairs from Sharma Tent House")
	if !strings.Contains(row, "10:45 am") || !strings.Contains(row, "Suresh Kumar") || !strings.Contains(row, `<div class="sm">Changed it from 500 chairs to 50 chairs</div>`) {
		t.Errorf("correction row %s", row)
	}
	wantLine := "Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am"
	assertContains(t, inBody, wantLine)
	if !strings.HasPrefix(wantLine, "Changed it from 500 chairs to 50 chairs") {
		t.Error("log phrase is not list line prefix")
	}
}

func TestLogDeletionRow(t *testing.T) {
	reg := register.WalkthroughT1()
	d := &register.Deletion{At: time.Date(2026, time.September, 3, 10, 47, 0, 0, register.IST), By: "Suresh Kumar", Reason: "Entered twice by mistake."}
	reg.Inwards[1].Deleted = d
	e := newTestServer(t, reg, logT4)
	_, body := e.get("/log?day=all")
	deleted := tableRow(t, body, "Deleted this entry: 310 chairs that came in")
	for _, want := range []string{"10:47 am", "Deleted — Entered twice by mistake.", "text-decoration:line-through"} {
		if !strings.Contains(deleted, want) {
			t.Errorf("deletion row lacks %q", want)
		}
	}
	original := tableRow(t, body, "This entry was deleted later.")
	if !strings.Contains(original, "This entry was deleted later.") || !strings.Contains(original, "text-decoration:line-through") {
		t.Errorf("original row %s", original)
	}
}

func TestLogFilterButtonsKeepOtherFilters(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=all&q=Imran")
	text := html.UnescapeString(body)
	if !strings.Contains(text, `href="/log?day=all&kind=came_back&q=Imran"`) {
		t.Error("Came back link dropped another filter")
	}
	if !strings.Contains(text, `class="opt on" href="/log?day=all&q=Imran">Every day`) {
		t.Error("Every day is not active")
	}
}

func TestLogFilterByPersonFromThePicker(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=all&q=98861")
	for _, href := range []string{"/out#ISS-0003", "/out#ISS-0005", "/out#ISS-0008", "/out#RET-0001"} {
		assertContains(t, body, href)
	}
	if strings.Count(body, "Go to this entry") != 4 {
		t.Errorf("person filter has %d record rows", strings.Count(body, "Go to this entry"))
	}
	logRows := askPeople(t, e, "scope=log&q=Imran")
	if !reflect.DeepEqual(peopleLabels(logRows), []string{"Imran Sheikh · 90080 77213"}) {
		t.Errorf("log picker %+v", logRows)
	}
	normal := askPeople(t, e, "q=Imran")
	for _, r := range normal {
		if !r.New {
			t.Errorf("normal picker unexpectedly found Imran: %+v", normal)
		}
	}
}

func TestLogPickerOffersNoNewPerson(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	if rows := askPeople(t, e, "scope=log&q=Meera"); len(rows) != 0 {
		t.Errorf("log picker offered %+v", rows)
	}
	rows := askPeople(t, e, "q=Meera")
	if len(rows) != 1 || !rows[0].New || rows[0].Label != "+ New person named Meera" {
		t.Errorf("normal picker %+v", rows)
	}
}

func TestLogEmptyForADayWithNothingOnIt(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=2026-08-30")
	assertContains(t, body, "Nobody wrote anything down on Sunday, 30 August.")
	assertContains(t, body, "Pick another day, or tap Every day.")
}

func TestLogEmptyWithFiltersOn(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log?day=2026-09-03&kind=came_back&q=" + url.QueryEscape("Lakshmi Iyer"))
	assertContains(t, body, "Nothing matches what you picked.")
	assertContains(t, body, "Tap Every day, Everything, Anybody, Any product and Any challan.")
	assertNotContains(t, body, "Nobody wrote anything down on")
}

func TestLogIgnoresRubbishParameters(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	resp, body := e.get("/log?day=yesterday&kind=exploded&productId=PRD-9999")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	assertContains(t, body, "Thursday, 3 September")
	for _, bad := range []string{"invalid", "error", "register.Log", "LogEntry", "LogKind"} {
		assertNotContains(t, body, bad)
	}
}

func TestLogIsReadOnly(t *testing.T) {
	reg := register.WalkthroughT3()
	e := newTestServer(t, reg, logT4)
	resp, _ := e.post("/log", url.Values{})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /log returned %d", resp.StatusCode)
	}
	before, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = e.get("/log")
	after, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("GET /log changed the file")
	}
	st, _, err := store.Open(e.path)
	if err != nil {
		t.Fatal(err)
	}
	var reopened register.Register
	st.Read(func(r *register.Register) { reopened = *r })
	register.LinkInwardParties(reg)
	if !reflect.DeepEqual(&reopened, reg) {
		t.Errorf("reopened register differs")
	}
}

func TestLogNeedsAShift(t *testing.T) {
	reg := register.WalkthroughT3()
	reg.OnDutyStaffID = ""
	e := newTestServer(t, reg, logT4)
	resp, _ := e.get("/log")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/shift" {
		t.Errorf("status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLogNeverAbbreviatesAName(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	_, body := e.get("/log")
	text := html.UnescapeString(body)
	for _, want := range []string{"10 chairs went out to Ravi Menon", "40 chairs went out to Ravi Menon", "45 chairs came back from Ravi Menon"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{" to Ravi<", " from Ravi<", "> to Ravi </", "> from Ravi </"} {
		if strings.Contains(text, bad) {
			t.Errorf("abbreviated shape %q", bad)
		}
	}
}

func TestLogShowsNoCount(t *testing.T) {
	re := regexp.MustCompile(`[0-9]+ (lines|rows|entries|results|records)`)
	e := newTestServer(t, register.WalkthroughT3(), logT4)
	for _, path := range []string{"/log", "/log?day=all"} {
		_, body := e.get(path)
		if re.MatchString(html.UnescapeString(body)) {
			t.Errorf("%s displays a result count", path)
		}
	}
}
