package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// anitaOnDuty is the T1 register with Anita Rao at the desk, which is where
// every issue test starts: 890 chairs on hand, 2:18 pm.
func anitaOnDuty() *register.Register {
	reg := register.WalkthroughT1()
	reg.OnDutyStaffID = "STF-0002"
	return reg
}

// walkthroughIssue is Ravi's 10 chairs at 2:18 pm.
func walkthroughIssue() url.Values {
	return url.Values{
		"productId":       {"PRD-0001"},
		"quantity":        {"10"},
		"takerName":       {"Ravi Menon"},
		"takerDepartment": {"Catering"},
		"takerMobile":     {"98861 40023"},
		"issuedAt":        {"2026-09-03T14:18"},
	}
}

// twoRaviKumars is the T1 register with the two men of one name the spec keeps
// insisting on: same spelling, different mobiles, different departments.
func twoRaviKumars() *register.Register {
	reg := anitaOnDuty()
	reg.Issues = append(reg.Issues,
		register.Issue{
			ID: "ISS-0009", ProductID: "PRD-0001", Quantity: 8,
			TakerName: "Ravi Kumar", TakerDepartment: "Catering", TakerMobile: "98861 40023",
			PersonInchargeName: "Anita Rao", PersonInchargeMobile: "99001 34562",
			IssuedAt:   time.Date(2026, time.September, 3, 11, 0, 0, 0, register.IST),
			RecordedAt: time.Date(2026, time.September, 3, 11, 0, 0, 0, register.IST),
		},
		register.Issue{
			ID: "ISS-0010", ProductID: "PRD-0001", Quantity: 6,
			TakerName: "Ravi Kumar", TakerDepartment: "Security", TakerMobile: "97740 11298",
			PersonInchargeName: "Anita Rao", PersonInchargeMobile: "99001 34562",
			IssuedAt:   time.Date(2026, time.September, 3, 11, 5, 0, 0, register.IST),
			RecordedAt: time.Date(2026, time.September, 3, 11, 5, 0, 0, register.IST),
		},
	)
	return reg
}

// askPeople fetches the person picker's answers.
func askPeople(t *testing.T, e *env, query string) []personRow {
	t.Helper()
	resp, body := e.get("/api/people?" + query)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/people?%s returned %d, want 200", query, resp.StatusCode)
	}
	var rows []personRow
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("the picker's answer is not readable: %v", err)
	}
	return rows
}

func peopleLabels(rows []personRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Label)
	}
	return out
}

func TestIssueFormRendersWalkthroughLabels(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	resp, body := e.get("/issue/new?productId=PRD-0001")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /issue/new returned %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{
		"Someone is taking",
		"Thursday, 3 September · 2:18 pm",
		"Product",
		"How many",
		"890 on hand right now",
		"Most you can issue is 890.",
		"Who is taking it",
		"Department",
		"Their mobile",
		"Person incharge (giving it)",
		`Anita Rao <span class="sm">(you)</span>`,
		"Your mobile",
		"99001 34562",
		"Cancel",
	} {
		assertContains(t, body, want)
	}
}

func TestIssuePrefillsKnownTaker(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	rows := askPeople(t, e, "q="+url.QueryEscape("Ravi Menon"))
	if len(rows) == 0 || rows[0].Department != "Catering" || rows[0].Mobile != "98861 40023" {
		t.Fatalf("the finder answered %+v, want Ravi Menon of Catering on 98861 40023", rows)
	}

	_, body := e.get("/issue/new?productId=PRD-0001&takerName=" + url.QueryEscape("Ravi Menon"))
	assertContains(t, body, "This person has taken things before. Their details are filled in.")
	assertContains(t, body, `value="Catering"`)
	assertContains(t, body, `value="98861 40023"`)
}

func TestPersonPickerFindsByNameOrMobile(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	for _, q := range []string{"Ravi", "98861", "9886140023", "98861 40023"} {
		rows := askPeople(t, e, "q="+url.QueryEscape(q))
		if len(rows) == 0 {
			t.Fatalf("%q found nobody", q)
		}
		if rows[0].Label != "Ravi Menon · 98861 40023 · Catering" {
			t.Errorf("%q offered %q, want Ravi Menon · 98861 40023 · Catering", q, rows[0].Label)
		}
	}
}

func TestPersonPickerShowsTwoPeopleWithOneName(t *testing.T) {
	e := newTestServer(t, twoRaviKumars(), twoEighteen)

	// 03-stock-arithmetic.spec.md breaks a tie on the name by mobile ascending,
	// so 97740 11298 comes before 98861 40023. 08-issue.spec.md lists the two
	// rows the other way round; the ordering rule in 03 governs, and the two
	// men are told apart by the number either way.
	got := peopleLabels(askPeople(t, e, "q="+url.QueryEscape("Ravi Kumar")))
	want := []string{
		"Ravi Kumar · 97740 11298 · Security",
		"Ravi Kumar · 98861 40023 · Catering",
		"+ New person named Ravi Kumar",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the picker offered\n%q\nwant\n%q", got, want)
	}
}

func TestPersonPickerAlwaysOffersANewPerson(t *testing.T) {
	e := newTestServer(t, twoRaviKumars(), twoEighteen)

	for _, q := range []string{"Nobody At All", "Ravi Menon", "Ravi"} {
		rows := askPeople(t, e, "q="+url.QueryEscape(q))
		last := rows[len(rows)-1]
		if !last.New || last.Label != "+ New person named "+q {
			t.Errorf("%q ended with %q, want + New person named %s", q, last.Label, q)
		}

		for _, row := range askPeople(t, e, "scope=log&q="+url.QueryEscape(q)) {
			if row.New {
				t.Errorf("the log picker offered %q and must not", row.Label)
			}
		}
	}
}

func TestPersonPickerNeverBlocks(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("takerName", "Ravii Varma")
	resp, body := e.post("/issue/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}
	for _, unwanted := range []string{"Did you mean", "already", "duplicate", "Are you sure"} {
		assertNotContains(t, body, unwanted)
	}

	var names []string
	for _, p := range register.PeopleHolding(e.saved()) {
		names = append(names, p.Name)
	}
	if !contains(names, "Ravi Menon") || !contains(names, "Ravii Varma") {
		t.Errorf("the people holding stock are %q, want both spellings", names)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestIssueFillsMobileForASingleMatch(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("takerMobile", "")
	form.Set("takerDepartment", "")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}

	saved := e.saved()
	got := saved.Issues[len(saved.Issues)-1]
	if got.TakerMobile != "98861 40023" || got.TakerDepartment != "Catering" {
		t.Errorf("the issue stored %q / %q, want 98861 40023 / Catering",
			got.TakerMobile, got.TakerDepartment)
	}
}

func TestIssueDoesNotGuessBetweenTwoMatches(t *testing.T) {
	e := newTestServer(t, twoRaviKumars(), twoEighteen)

	form := walkthroughIssue()
	form.Set("takerName", "Ravi Kumar")
	form.Set("takerMobile", "")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}

	saved := e.saved()
	if got := saved.Issues[len(saved.Issues)-1].TakerMobile; got != "" {
		t.Errorf("the issue stored the mobile %q, want it left empty", got)
	}
}

func TestIssueAmberBannerForRavi(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001&takerName=" +
		url.QueryEscape("Ravi Menon") + "&takerMobile=" + url.QueryEscape("98861 40023"))
	assertContains(t, body, "banner warn")
	assertContains(t, body, "Ravi Menon is already holding 40 chairs and 2 round tables from earlier today.")
}

func TestIssueAmberBannerIsPerMobile(t *testing.T) {
	e := newTestServer(t, twoRaviKumars(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001&takerName=" +
		url.QueryEscape("Ravi Kumar") + "&takerMobile=" + url.QueryEscape("98861 40023"))
	assertContains(t, body, "Ravi Kumar is already holding 8 chairs from earlier today.")
	assertNotContains(t, body, "14 chairs")
	assertNotContains(t, body, "6 chairs")
}

func TestIssueAmberBannerDropsTodayClauseForOlderHoldings(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001&takerName=" + url.QueryEscape("Joseph D'Cruz"))
	assertContains(t, body, "Joseph D'Cruz is already holding 120 chairs and 25 extension boards.")
	assertNotContains(t, body, "from earlier today")
}

func TestIssueNoBannerForNewPerson(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001&takerName=" + url.QueryEscape("Meera Pillai"))
	assertNotContains(t, body, "banner warn")
	assertNotContains(t, body, "is already holding")
}

func TestIssue10ChairsToRavi(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	resp, _ := e.post("/issue/new", walkthroughIssue())
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/stock?saved=ISS-0008" {
		t.Errorf("the save went to %q, want /stock?saved=ISS-0008", got)
	}

	saved := e.saved()
	got := saved.Issues[7]
	want := register.WalkthroughT2().Issues[7]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the saved issue is\n%+v\nwant\n%+v", got, want)
	}
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 880 {
		t.Errorf("chairs read %d on hand, want 880", onHand)
	}

	_, body := e.get("/stock?saved=ISS-0008")
	assertContains(t, body, "Gave 10 chairs to Ravi Menon. Chairs: 880 on hand.")
}

// refusedIssue posts a doctored walkthrough issue and insists on a 200 with a
// sentence a person can act on, and nothing written.
func refusedIssue(t *testing.T, e *env, form url.Values, want string) string {
	t.Helper()
	before := len(e.saved().Issues)
	resp, body := e.post("/issue/new", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the refusal returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, want)
	for _, unwanted := range []string{"invalid", "error", "nil", "panic"} {
		assertNotContains(t, body, unwanted)
	}
	if after := len(e.saved().Issues); after != before {
		t.Errorf("the register holds %d issues, want the original %d", after, before)
	}
	return body
}

func TestIssueRefuses900From890(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("quantity", "900")
	refusedIssue(t, e, form, "You have 890 chairs. You cannot give out more than 890.")

	saved := e.saved()
	if n := len(saved.Issues); n != 7 {
		t.Errorf("the register holds %d issues, want 7", n)
	}
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 890 {
		t.Errorf("chairs read %d on hand, want 890", onHand)
	}
}

func TestIssueAllowsExactly890(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("quantity", "890")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("890 chairs were refused")
	}
	if onHand := register.OnHand(e.saved(), "PRD-0001"); onHand != 0 {
		t.Errorf("chairs read %d on hand, want 0", onHand)
	}

	one := walkthroughIssue()
	one.Set("quantity", "1")
	refusedIssue(t, e, one, "There are no chairs left.")
}

func TestIssueZeroStockSentence(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("productId", "PRD-0004") // extension boards, none on hand
	form.Set("quantity", "1")
	body := refusedIssue(t, e, form, "There are no extension boards left.")
	assertNotContains(t, body, "give out more than 0")
}

func TestIssueRefusesZeroAndGarbage(t *testing.T) {
	for _, quantity := range []string{"0", "-3", "abc", "10.5"} {
		t.Run(quantity, func(t *testing.T) {
			e := newTestServer(t, anitaOnDuty(), twoEighteen)
			form := walkthroughIssue()
			form.Set("quantity", quantity)
			refusedIssue(t, e, form, "Type how many chairs they are taking.")
		})
	}
}

func TestIssueRefusesEmptyTaker(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("takerName", "   ")
	refusedIssue(t, e, form, "Who is taking it? Type their name.")
}

func TestIssueRefusesProductWithNoStock(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("productId", "PRD-0004")
	form.Set("quantity", "5")
	refusedIssue(t, e, form, "There are no extension boards left.")

	for _, row := range ask(t, e, "mode=instock") {
		if row.ID == "PRD-0004" {
			t.Error("the issue picker offered extension boards, which are all out")
		}
	}
}

func TestIssueIgnoresPersonInchargeFromForm(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("personInchargeName", "Somebody Else")
	form.Set("personInchargeMobile", "00000 00000")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save was refused")
	}

	saved := e.saved()
	got := saved.Issues[len(saved.Issues)-1]
	if got.PersonInchargeName != "Anita Rao" || got.PersonInchargeMobile != "99001 34562" {
		t.Errorf("the issue was stamped %q / %q, want Anita Rao / 99001 34562",
			got.PersonInchargeName, got.PersonInchargeMobile)
	}
}

// TestOnDutyNameStampsAnEntry is the test 05 deferred until a form existed.
func TestOnDutyNameStampsAnEntry(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("personInchargeName", "Somebody Else")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save was refused")
	}

	saved := e.saved()
	got := saved.Issues[len(saved.Issues)-1]
	if got.PersonInchargeName != "Anita Rao" || got.PersonInchargeMobile != "99001 34562" {
		t.Errorf("the issue was stamped %q / %q, want the person on duty",
			got.PersonInchargeName, got.PersonInchargeMobile)
	}
}

func TestIssueEditableTimestampIsHonoured(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("issuedAt", "2026-09-03T13:05")
	if resp, _ := e.post("/issue/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save was refused")
	}

	saved := e.saved()
	got := saved.Issues[len(saved.Issues)-1]
	want := time.Date(2026, time.September, 3, 13, 5, 0, 0, register.IST)
	if !got.IssuedAt.Equal(want) {
		t.Errorf("the issue is timed %s, want %s", got.IssuedAt, want)
	}
	if !got.RecordedAt.Equal(twoEighteen) {
		t.Errorf("the issue was recorded at %s, want %s", got.RecordedAt, twoEighteen)
	}
}

func TestIssueRevalidatesAtSaveTime(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001")
	assertContains(t, body, "890 on hand right now")

	// Somebody at another tab takes 885 chairs while this form sits open.
	if err := e.st.Update(func(reg *register.Register) error {
		reg.Issues = append(reg.Issues, register.Issue{
			ID: "ISS-0009", ProductID: "PRD-0001", Quantity: 885,
			TakerName: "Farida Begum", TakerMobile: "98455 30918",
			PersonInchargeName: "Anita Rao", PersonInchargeMobile: "99001 34562",
			IssuedAt: twoEighteen, RecordedAt: twoEighteen,
		})
		return nil
	}); err != nil {
		t.Fatalf("mutating the store: %v", err)
	}

	refusedIssue(t, e, walkthroughIssue(), "You have 5 chairs. You cannot give out more than 5.")
}

func TestIssueKeepsTypedValuesOnRefusal(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	form := walkthroughIssue()
	form.Set("quantity", "900")
	_, body := e.post("/issue/new", form)
	for _, want := range []string{"Ravi Menon", "Catering", "98861 40023"} {
		assertContains(t, body, want)
	}
}

func TestIssueButtonLabel(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	_, body := e.get("/issue/new?productId=PRD-0001&quantity=10&takerName=" +
		url.QueryEscape("Ravi Menon"))
	assertContains(t, body, "Issue 10 chairs to Ravi")

	_, fresh := e.get("/issue/new")
	if !strings.Contains(fresh, ">Issue<") {
		t.Error("the fresh form's button does not read Issue")
	}
}

// TestOnHandNeverNegativeAcrossIssueTests is acceptance criterion 5 of spec 08:
// no test in this package may leave a product below zero.
func TestOnHandNeverNegativeAcrossIssueTests(t *testing.T) {
	e := newTestServer(t, anitaOnDuty(), twoEighteen)

	for _, form := range []url.Values{walkthroughIssue(), walkthroughIssue()} {
		e.post("/issue/new", form)
	}
	saved := e.saved()
	for _, p := range saved.Products {
		if onHand := register.OnHand(saved, p.ID); onHand < 0 {
			t.Errorf("%s reads %d on hand", p.Name, onHand)
		}
	}
	if problems := register.Validate(saved); len(problems) != 0 {
		t.Errorf("the register breaks %d invariants: %+v", len(problems), problems)
	}
}
