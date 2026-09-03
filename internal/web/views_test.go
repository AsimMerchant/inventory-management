package web

import (
	"html"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// tenAM is the walkthrough's home screen: 3 September 2026, 10:00 am.

// tableRow is the one table row that names something, so a figure can be
// asserted against the row it belongs to rather than against the whole page.
func tableRow(t *testing.T, body, name string) string {
	t.Helper()
	text := html.UnescapeString(body)
	rows := strings.Split(text, "<tr")
	// rows[0] is everything above the table, banners included, and a banner
	// naming the product is not the row that reports it.
	for _, row := range rows[1:] {
		if strings.Contains(row, name) {
			return row
		}
	}
	t.Fatalf("no table row names %q", name)
	return ""
}

// blockWithID is everything from one id= attribute to the next, which is one
// issue line, one Came back line or one inward row with its corrections.
func blockWithID(t *testing.T, body, id string) string {
	t.Helper()
	text := html.UnescapeString(body)
	start := strings.Index(text, `id="`+id+`"`)
	if start < 0 {
		t.Fatalf("nothing on the page carries id=%q", id)
	}
	rest := text[start+1:]
	if next := strings.Index(rest, `id="`); next >= 0 {
		return rest[:next]
	}
	return rest
}

func TestStockTilesAtT0(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	for _, tt := range []struct{ label, value string }{
		{"Products", "5"},
		{"Out right now", "352"},
		{"People holding", "4"},
		{"Out over 2 days", "3"},
	} {
		want := `<div class="k">` + tt.label + `</div>`
		if !strings.Contains(body, want) {
			t.Errorf("the tile %q is missing", tt.label)
			continue
		}
		after := body[strings.Index(body, want)+len(want):]
		if !strings.Contains(after[:120], ">"+tt.value+"<") {
			t.Errorf("the tile %q does not read %s", tt.label, tt.value)
		}
	}
}

func TestStockTableAtT0(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	for _, th := range []string{"Product", "Type", "Came in", "Out", "On hand"} {
		assertContains(t, body, ">"+th+"</th>")
	}

	order := []string{"Chairs", "Charcoal sacks", "Extension boards", "Round tables", "Water drums (20L)"}
	at := -1
	for _, name := range order {
		i := strings.Index(body, name)
		if i < at {
			t.Errorf("%s is out of order in the stock table", name)
		}
		at = i
	}

	for _, tt := range []struct {
		name        string
		in, out, on string
	}{
		{"Chairs", "700", "310", "390"},
		{"Charcoal sacks", "12", "0", "12"},
		{"Extension boards", "25", "25", "0"},
		{"Round tables", "60", "12", "48"},
		{"Water drums (20L)", "40", "5", "35"},
	} {
		row := tableRow(t, body, tt.name)
		for _, figure := range []string{
			`<td class="num">` + tt.in + `</td>`,
			`<td class="num">` + tt.out + `</td>`,
		} {
			if !strings.Contains(row, figure) {
				t.Errorf("the %s row does not contain %s", tt.name, figure)
			}
		}
		if !strings.Contains(row, ">"+tt.on+"</strong>") {
			t.Errorf("the %s row does not read %s on hand", tt.name, tt.on)
		}
	}
}

func TestStockShowsZeroInRedWithNoIssueButton(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	row := tableRow(t, body, "Extension boards")
	for _, want := range []string{"var(--bad)", "None left"} {
		if !strings.Contains(row, want) {
			t.Errorf("the Extension boards row does not contain %q", want)
		}
	}
	assertNotContains(t, body, "/issue/new?productId=PRD-0004")
}

func TestStockIssueButtonsLinkToProduct(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	row := tableRow(t, body, "Chairs")
	if !strings.Contains(row, "/issue/new?productId=PRD-0001") {
		t.Error("the Chairs row does not link to the issue screen for chairs")
	}
}

func TestStockPills(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	for _, tt := range []struct{ name, class, word string }{
		{"Chairs", "pill rent", "Rent"},
		{"Round tables", "pill rent", "Rent"},
		{"Extension boards", "pill rent", "Rent"},
		{"Water drums (20L)", "pill sale", "Purchase"},
		{"Charcoal sacks", "pill sale", "Purchase"},
	} {
		row := tableRow(t, body, tt.name)
		if !strings.Contains(row, `<span class="`+tt.class+`">`+tt.word+`</span>`) {
			t.Errorf("the %s row does not carry a %s pill reading %s", tt.name, tt.class, tt.word)
		}
	}
}

// A product created before its goods arrive must not claim it was bought.
// Nobody has ticked rent or purchase yet, because there is no delivery to tick.
func TestStockSaysNothingAboutBasisBeforeAnythingArrives(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Products = append(reg.Products, register.Product{
		ID: "PRD-9001", Name: "Shamiana poles", CreatedAt: tenAM, CreatedBy: "Ramesh",
	})
	e := newTestServer(t, reg, tenAM)
	_, body := e.get("/stock")

	row := tableRow(t, body, "Shamiana poles")
	if strings.Contains(row, "Purchase") || strings.Contains(row, "Rent") {
		t.Errorf("a product with no deliveries claims a basis: %s", row)
	}
	if !strings.Contains(row, `<span class="pill none">Not received yet</span>`) {
		t.Errorf("a product with no deliveries does not say so: %s", row)
	}
}

func TestStockThreeButtons(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/stock")

	for _, tt := range []struct{ label, href string }{
		{"+ Stuff came in", "/inward/new"},
		{"Someone is taking", "/issue/new"},
		{"Someone is returning", "/return/new"},
	} {
		assertContains(t, body, tt.label)
		if !strings.Contains(body, `href="`+tt.href+`"`) {
			t.Errorf("%q does not link to %s", tt.label, tt.href)
		}
	}
}

func TestStockSavedBanner(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenAM)
	_, body := e.get("/stock?saved=INW-0007")
	assertContains(t, body, "Added 500 chairs. Chairs: 890 on hand.")
}

func TestStockAfterFullWalkthroughSequence(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/stock")

	row := tableRow(t, body, "Chairs")
	for _, want := range []string{`<td class="num">1200</td>`, `<td class="num">275</td>`, `>925</strong>`} {
		if !strings.Contains(row, want) {
			t.Errorf("the Chairs row does not contain %q", want)
		}
	}
	after := body[strings.Index(body, `<div class="k">Out right now</div>`):]
	if !strings.Contains(after[:120], ">317<") {
		t.Error("the Out right now tile does not read 317")
	}
}

func TestOutWithPeopleAtT0(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/out")

	text := html.UnescapeString(body)
	at := -1
	for _, name := range []string{"Farida Begum", "Joseph D'Cruz", "Lakshmi Iyer", "Ravi Menon"} {
		i := strings.Index(text, name)
		if i < at {
			t.Errorf("%s is out of order on the page", name)
		}
		at = i
	}

	assertContains(t, body, "Ravi Menon · Catering · 98861 40023")
	assertContains(t, body, "holding 42")
	assertContains(t, body, "Issued 9:40 am by Suresh Kumar · 40 taken, 0 back")

	chairs := blockWithID(t, body, "ISS-0003")
	if !strings.Contains(chairs, "40 out") {
		t.Error("Ravi's chairs line does not read 40 out")
	}
	tables := blockWithID(t, body, "ISS-0005")
	if !strings.Contains(tables, "2 out") {
		t.Error("Ravi's round tables line does not read 2 out")
	}
}

func TestOutWithPeopleFlagsAgedLines(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/out")

	for _, id := range []string{"ISS-0001", "ISS-0002", "ISS-0007"} {
		if !strings.Contains(blockWithID(t, body, id), "Out over 2 days") {
			t.Errorf("%s is not flagged as out over 2 days", id)
		}
	}
	for _, id := range []string{"ISS-0003", "ISS-0005"} {
		if strings.Contains(blockWithID(t, body, id), "Out over 2 days") {
			t.Errorf("%s is flagged as out over 2 days and should not be", id)
		}
	}
}

func TestOutWithPeopleShowsShortfallRemark(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/out")

	line := blockWithID(t, body, "ISS-0008")
	if !strings.Contains(line, "5 out") {
		t.Error("Ravi's chairs line does not read 5 out")
	}
	assertContains(t, body,
		"Won't come back: 5 chairs broke during setup near the stage. Ravi informed.")
}

func TestOutWithPeopleEmpty(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Issues = nil
	e := newTestServer(t, reg, tenAM)

	_, body := e.get("/out")
	assertContains(t, body, "Nothing is out with anybody right now.")
	assertContains(t, body, "Nothing has come back yet.")
}

func TestInwardsTabListsNewestFirst(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenAM)
	_, body := e.get("/inwards")

	text := html.UnescapeString(body)
	first := strings.Index(text, `id="INW-`)
	if !strings.HasPrefix(text[first:], `id="INW-0007"`) {
		t.Errorf("the first row is %q, want INW-0007", text[first:first+20])
	}

	row := blockWithID(t, body, "INW-0007")
	for _, want := range []string{"2026-09-03", "Chairs", ">500<", "Rent", "Sharma Tent House", "STH/4471", "Suresh Kumar"} {
		if !strings.Contains(row, want) {
			t.Errorf("the INW-0007 row does not contain %q", want)
		}
	}
	if !strings.Contains(blockWithID(t, body, "INW-0002"), "nobody wrote it down") {
		t.Error("INW-0002 does not say nobody wrote the supplier down")
	}
}

func TestInwardsTabLinksToFixThis(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenAM)
	_, body := e.get("/inwards")

	for _, in := range register.WalkthroughT1().Inwards {
		row := blockWithID(t, body, in.ID)
		if !strings.Contains(row, "/entry/"+in.ID+"/edit") {
			t.Errorf("the %s row has no Fix this link", in.ID)
		}
	}
}

func TestRowsCarryAnchors(t *testing.T) {
	reg := register.WalkthroughT3()
	reg.Inwards[1].Deleted = &register.Deletion{
		At: tenFortyFive, By: "Suresh Kumar", Reason: "Entered twice by mistake.",
	}
	e := newTestServer(t, reg, sixOhFive)

	_, inwards := e.get("/inwards")
	for _, in := range reg.Inwards {
		if !strings.Contains(inwards, `id="`+in.ID+`"`) {
			t.Errorf("/inwards carries no anchor for %s", in.ID)
		}
	}

	_, out := e.get("/out")
	for _, id := range []string{"ISS-0008", "RET-0001"} {
		if !strings.Contains(out, `id="`+id+`"`) {
			t.Errorf("/out carries no anchor for %s", id)
		}
	}
}

func TestInwardsTabShowsCorrectionsAndTombstones(t *testing.T) {
	reg := register.WalkthroughT1()
	reg.Inwards[6].Quantity = 50
	reg.Inwards[6].Changes = []register.Change{{
		At: tenFortyFive, By: "Suresh Kumar", Field: "quantity", Label: "How many",
		From: "500", To: "50",
	}}
	reg.Inwards[1].Deleted = &register.Deletion{
		At: tenFortySeven, By: "Suresh Kumar", Reason: "Entered twice by mistake.",
	}
	e := newTestServer(t, reg, sixOhFive)

	_, body := e.get("/inwards")
	assertContains(t, body, "Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am")
	assertContains(t, body, "Deleted by Suresh Kumar, 10:47 am — Entered twice by mistake.")
	assertNotContains(t, body, "/entry/INW-0002/edit")

	_, stock := e.get("/stock")
	row := tableRow(t, stock, "Chairs")
	if !strings.Contains(row, `<td class="num">440</td>`) {
		t.Error("the Chairs stock row does not read 440 came in")
	}
}

func TestOutTabListsReturnsFlat(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)

	_, body := e.get("/out")
	assertContains(t, body, "Came back")
	assertContains(t, body, "45 chairs from Ravi Menon · 6:05 pm · taken back by Imran Sheikh")
	if !strings.Contains(body, "/entry/RET-0001/edit") {
		t.Error("the RET-0001 line has no Fix this link")
	}

	// Ravi's last 5 chairs come back too, so neither issue is outstanding any
	// more. Both returns are still listed.
	reg := register.WalkthroughT3()
	sixThirty := time.Date(2026, time.September, 3, 18, 30, 0, 0, register.IST)
	reg.Returns = append(reg.Returns, register.Return{
		ID: "RET-0002", ProductID: "PRD-0001",
		Allocations:  []register.Allocation{{IssueID: "ISS-0008", Quantity: 5}},
		ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
		TakenBackBy: "Imran Sheikh",
		ReturnedAt:  sixThirty, RecordedAt: sixThirty,
	})
	e2 := newTestServer(t, reg, sixThirty)
	_, body2 := e2.get("/out")
	assertContains(t, body2, "5 chairs from Ravi Menon · 6:30 pm · taken back by Imran Sheikh")
	assertContains(t, body2, "45 chairs from Ravi Menon · 6:05 pm · taken back by Imran Sheikh")
	if strings.Contains(body2, `class="outrow" id="ISS-0008"`) {
		t.Error("ISS-0008 is still shown as outstanding after everything came back")
	}
}

func TestOutTabSeparatesTwoPeopleWithOneName(t *testing.T) {
	reg := register.WalkthroughT2()
	nine := time.Date(2026, time.September, 3, 9, 0, 0, 0, register.IST)
	reg.Issues = append(reg.Issues,
		register.Issue{
			ID: "ISS-0009", ProductID: "PRD-0001", Quantity: 6,
			TakerName: "Ravi Kumar", TakerDepartment: "Kitchen", TakerMobile: "90011 22334",
			PersonInchargeName: "Suresh Kumar", IssuedAt: nine, RecordedAt: nine,
		},
		register.Issue{
			ID: "ISS-0010", ProductID: "PRD-0001", Quantity: 4,
			TakerName: "Ravi Kumar", TakerDepartment: "Kitchen", TakerMobile: "93400 55118",
			PersonInchargeName: "Suresh Kumar", IssuedAt: nine, RecordedAt: nine,
		},
	)
	e := newTestServer(t, reg, sixOhFive)

	_, body := e.get("/out")
	assertContains(t, body, "holding 6")
	assertContains(t, body, "holding 4")
	assertContains(t, body, "90011 22334")
	assertContains(t, body, "93400 55118")
	assertNotContains(t, body, "holding 10")
}

func TestSuppliersBannerCountsOut(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	_, body := e.get("/suppliers")
	assertContains(t, body, "352 things are still out with people.")
}

func TestSuppliersBannerHiddenWhenNothingOut(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Issues = nil
	e := newTestServer(t, reg, tenAM)

	_, body := e.get("/suppliers")
	assertNotContains(t, body, "things are still out with people.")
}

func TestSuppliersTableAtT3(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/suppliers")

	text := html.UnescapeString(body)
	rows := []struct {
		supplier, product, cameIn, pill string
	}{
		{"Gupta Electricals", "Extension boards", "25", "Rent"},
		{"Sharma Tent House", "Chairs", "890", "Rent"},
		{"Sharma Tent House", "Round tables", "60", "Rent"},
		{"we bought it", "Chairs", "310", "Purchase"},
		{"we bought it", "Charcoal sacks", "12", "Purchase"},
		{"we bought it", "Water drums (20L)", "40", "Purchase"},
	}
	at := 0
	for _, want := range rows {
		i := strings.Index(text[at:], want.supplier)
		if i < 0 {
			t.Fatalf("the row for %s / %s is missing or out of order", want.supplier, want.product)
		}
		row := text[at+i:]
		if end := strings.Index(row, "</tr>"); end > 0 {
			row = row[:end]
		}
		for _, cell := range []string{want.product, ">" + want.cameIn + "</strong>", want.pill} {
			if !strings.Contains(row, cell) {
				t.Errorf("the %s / %s row does not contain %q", want.supplier, want.product, cell)
			}
		}
		at += i + len(want.supplier)
	}

	chairs := text[strings.Index(text, "Sharma Tent House"):]
	if !strings.Contains(chairs[:400], "5 broken or lost") {
		t.Error("the Sharma Tent House chairs row does not note 5 broken or lost")
	}
	// A bought row is a plain record of what was bought, with no note on it.
	bought := text[strings.Index(text, "we bought it"):]
	if strings.Contains(bought, "broken or lost") {
		t.Error("a bought row carries a broken-or-lost note")
	}
}

func TestSuppliersHasNoDebtColumns(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/suppliers")

	for _, th := range []string{"Supplier", "Product", "Came in", "Type"} {
		assertContains(t, body, ">"+th+"</th>")
	}
	lower := strings.ToLower(html.UnescapeString(body))
	for _, unwanted := range []string{"given back", "still owed", "all back", "owed", "took", "balance"} {
		if strings.Contains(lower, unwanted) {
			t.Errorf("the suppliers page contains %q", unwanted)
		}
	}
}

func TestSuppliersHasNoButtons(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), sixOhFive)
	_, body := e.get("/suppliers")

	for _, unwanted := range []string{"<form", "<button", `class="btn`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the suppliers page contains %q", unwanted)
		}
	}
	lower := strings.ToLower(html.UnescapeString(body))
	for _, unwanted := range []string{"settl", "paid", "amount", "₹"} {
		if strings.Contains(lower, unwanted) {
			t.Errorf("the suppliers page contains %q", unwanted)
		}
	}
	if strings.Contains(html.UnescapeString(body), "Rs") {
		t.Error("the suppliers page contains Rs")
	}
}

func TestTheFourReadingScreensRenderOverAnyRegister(t *testing.T) {
	for _, tt := range []struct {
		name string
		reg  *register.Register
	}{
		{"the walkthrough", register.WalkthroughT0()},
		{"an empty register", emptyRegister()},
	} {
		e := newTestServer(t, tt.reg, tenAM)
		for _, path := range []string{"/stock", "/out", "/inwards", "/suppliers"} {
			resp, body := e.get(path)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s over %s returned %d, want 200", path, tt.name, resp.StatusCode)
			}
			if len(strings.TrimSpace(body)) == 0 {
				t.Errorf("%s over %s returned an empty body", path, tt.name)
			}
		}
	}
}

// emptyRegister is a register with one person on duty and nothing else, which
// is what the second day of a fresh install looks like before anything is
// entered.
func emptyRegister() *register.Register {
	started := time.Date(2026, time.September, 3, 8, 0, 0, 0, register.IST)
	return &register.Register{
		SchemaVersion:  register.SchemaVersion,
		OnDutyStaffID:  "STF-0001",
		ShiftStartedAt: &started,
		Staff: []register.Staff{{
			ID: "STF-0001", Name: "Suresh Kumar", Mobile: "98450 22117",
			CreatedAt: started,
		}},
	}
}

// A ghost button has no fill, so the dark-mode ink meant for the filled button
// must never reach it: near-black text on a near-black page is unreadable and
// the reader cannot tell what they are pressing.
func TestDarkModeInkDoesNotReachGhostButtons(t *testing.T) {
	css, err := os.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`:root[data-theme="dark"] .btn:not(.ghost){color:#0F1720}`,
		`:root:not([data-theme="light"]) .btn:not(.ghost){color:#0F1720}`,
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("the dark-mode button ink is not scoped away from ghost buttons: %s", want)
		}
	}
}
