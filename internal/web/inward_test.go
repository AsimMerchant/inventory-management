package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// tenFortyTwo is the walkthrough's first entry: 500 chairs at the gate.
var tenFortyTwo = time.Date(2026, time.September, 3, 10, 42, 0, 0, register.IST)

// walkthroughInward is the post the walkthrough makes at 10:42.
func walkthroughInward() url.Values {
	return url.Values{
		"productId":  {"PRD-0001"},
		"quantity":   {"500"},
		"receivedOn": {"2026-09-03"},
		"basis":      {"rent"},
		"supplier":   {"Sharma Tent House"},
		"challanNo":  {"STH/4471"},
	}
}

func TestInwardFormRendersWalkthroughLabels(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	resp, body := e.get("/inward/new")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /inward/new returned %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{
		"Record what arrived",
		"Thursday, 3 September · 10:42 am",
		"Product",
		"How many",
		"Date received",
		"How these came in",
		"On rent — goes back to supplier",
		"Purchased for resale — does not come back",
		"Some other way — donated, sponsored, borrowed",
		"Came from",
		"Leave blank if you don't know.",
		"Challan no.",
		"Received by",
		`Suresh Kumar <span class="sm">(you)</span>`,
		"Cancel",
	} {
		assertContains(t, body, want)
	}
}

func TestInwardFormDefaultsDateToToday(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	_, body := e.get("/inward/new")
	assertContains(t, body, `name="receivedOn" value="2026-09-03"`)
}

func TestInward500ChairsRaisesOnHandTo890(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	resp, _ := e.post("/inward/new", walkthroughInward())
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/stock?saved=INW-0007" {
		t.Errorf("the save went to %q, want /stock?saved=INW-0007", got)
	}

	saved := e.saved()
	if len(saved.Inwards) != 7 {
		t.Fatalf("the register holds %d inwards, want 7", len(saved.Inwards))
	}
	got := saved.Inwards[6]
	want := register.WalkthroughT1().Inwards[6]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the saved inward is\n%+v\nwant\n%+v", got, want)
	}
	if onHand := register.OnHand(saved, "PRD-0001"); onHand != 890 {
		t.Errorf("chairs read %d on hand, want 890", onHand)
	}

	_, body := e.get("/stock?saved=INW-0007")
	assertContains(t, body, "Added 500 chairs. Chairs: 890 on hand.")

	// The stock row 07 could not assert until 10 drew the table.
	row := tableRow(t, body, "Chairs")
	for _, want := range []string{`<td class="num">1200</td>`, `<td class="num">310</td>`, `>890</strong>`} {
		if !strings.Contains(row, want) {
			t.Errorf("the Chairs row does not contain %q", want)
		}
	}
}

func TestInwardWithBlankSupplierAndChallan(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	form := walkthroughInward()
	form.Set("supplier", "")
	form.Set("challanNo", "")
	resp, _ := e.post("/inward/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}

	got := e.saved().Inwards[6]
	if got.Supplier != "" || got.ChallanNo != "" {
		t.Errorf("supplier is %q and challan %q, want both empty", got.Supplier, got.ChallanNo)
	}

	raw, err := os.ReadFile(e.path)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if !strings.Contains(string(raw), `"supplier": ""`) {
		t.Error(`the file does not contain "supplier": ""`)
	}
}

// refusedInward posts a doctored walkthrough inward and insists on a 200 with a
// sentence a person can act on.
func refusedInward(t *testing.T, e *env, form url.Values, want string) {
	t.Helper()
	resp, body := e.post("/inward/new", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the refusal returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, want)
	assertContains(t, body, "banner bad")
	for _, unwanted := range []string{"invalid", "error", "nil", "panic"} {
		assertNotContains(t, body, unwanted)
	}
	if n := len(e.saved().Inwards); n != 6 {
		t.Errorf("the register holds %d inwards, want the original 6", n)
	}
}

func TestInwardRefusesZeroAndNegative(t *testing.T) {
	for _, quantity := range []string{"0", "-5", "abc"} {
		t.Run(quantity, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
			form := walkthroughInward()
			form.Set("quantity", quantity)
			refusedInward(t, e, form, "Type how many chairs came in.")
		})
	}
}

func TestInwardRefusesFractional(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("quantity", "12.5")
	refusedInward(t, e, form, "Type how many chairs came in.")
}

func TestInwardRefusesUnknownProduct(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("productId", "PRD-9999")
	refusedInward(t, e, form, "Pick the product from the list.")

	if n := len(e.saved().Products); n != 5 {
		t.Errorf("the register holds %d products, want 5", n)
	}
}

func TestInwardRefusesMissingBasis(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("basis", "")
	refusedInward(t, e, form, "Choose how these came in.")
}

func TestInwardRefusesBadDate(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("receivedOn", "03/09/2026")
	refusedInward(t, e, form, "Type the date like this: 03-09-2026.")
}

func TestInwardKeepsTypedValuesOnRefusal(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("quantity", "0")

	_, body := e.post("/inward/new", form)
	for _, want := range []string{"Sharma Tent House", "STH/4471", "Chairs"} {
		assertContains(t, body, want)
	}
}

func TestInwardOfPurchasedStock(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	resp, _ := e.post("/inward/new", url.Values{
		"productId":  {"PRD-0003"},
		"quantity":   {"40"},
		"receivedOn": {"2026-09-03"},
		"basis":      {"purchase"},
		"supplier":   {""},
		"challanNo":  {""},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the save returned %d, want 303", resp.StatusCode)
	}

	saved := e.saved()
	if got := saved.Inwards[6].Basis; got != register.Purchase {
		t.Errorf("the basis is %q, want purchase", got)
	}
	found := false
	for _, row := range register.SupplierRows(saved) {
		if row.ProductID == "PRD-0003" && !row.OnRent {
			found = true
		}
	}
	if !found {
		t.Error("the water drums are not on a non-rent supplier row")
	}
}

func TestInwardButtonLabel(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"Chairs", "chairs"},
		{"Round tables", "round tables"},
		{"Extension boards", "extension boards"},
		{"Water drums (20L)", "Water drums (20L)"},
		{"Charcoal sacks", "charcoal sacks"},
	}
	for _, tt := range tests {
		if got := productWord(tt.name); got != tt.want {
			t.Errorf("productWord(%q) is %q, want %q", tt.name, got, tt.want)
		}
	}

	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	form := walkthroughInward()
	form.Set("receivedOn", "03/09/2026") // refused, so the form comes back drawn
	_, body := e.post("/inward/new", form)
	assertContains(t, body, "Save — 500 chairs in")

	_, fresh := e.get("/inward/new")
	assertContains(t, fresh, ">Save<")
}

func TestInwardIsAtomicOnDisk(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	e.post("/inward/new", walkthroughInward())

	raw, err := os.ReadFile(e.path + ".bak")
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	var backup register.Register
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatalf("the backup does not parse: %v", err)
	}
	if len(backup.Inwards) != 6 {
		t.Errorf("the backup holds %d inwards, want 6", len(backup.Inwards))
	}
	if n := len(e.saved().Inwards); n != 7 {
		t.Errorf("the register holds %d inwards, want 7", n)
	}
}

// The four tests 05 and 06 deferred until a real form existed.

func TestNoScriptFallbackListsProducts(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	_, body := e.get("/inward/new")
	noscript := between(body, "<noscript>", "</noscript>")
	assertContains(t, noscript, `<select class="inp" name="productId">`)
	for _, p := range register.WalkthroughT0().Products {
		assertContains(t, noscript, `value="`+p.ID+`"`)
	}
}

func TestAddNewRowOnlyOnInward(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), twoEighteen)

	_, inward := e.get("/inward/new")
	assertContains(t, inward, "as a brand-new product")

	_, issue := e.get("/issue/new")
	assertNotContains(t, issue, "as a brand-new product")
}

func TestFormWithoutProductIdRefused(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	form := walkthroughInward()
	form.Del("productId")
	form.Set("productName", "Chairs")
	refusedInward(t, e, form, "Pick the product from the list.")
}

// between returns the text of the first region between two markers.
func between(body, open, close string) string {
	from := strings.Index(body, open)
	if from < 0 {
		return ""
	}
	rest := body[from+len(open):]
	to := strings.Index(rest, close)
	if to < 0 {
		return rest
	}
	return rest[:to]
}
