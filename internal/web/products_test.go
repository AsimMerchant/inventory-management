package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
)

// ask fetches the picker's answers.
func ask(t *testing.T, e *env, query string) []suggestion {
	t.Helper()
	resp, body := e.get("/api/products?" + query)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/products?%s returned %d, want 200", query, resp.StatusCode)
	}
	var found []suggestion
	if err := json.Unmarshal([]byte(body), &found); err != nil {
		t.Fatalf("the picker's answer is not readable: %v", err)
	}
	return found
}

func labels(found []suggestion) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.Label)
	}
	return out
}

func assertLabels(t *testing.T, got []suggestion, want ...string) {
	t.Helper()
	have := labels(got)
	if len(have) != len(want) {
		t.Fatalf("the picker offered %v, want %v", have, want)
	}
	for i := range want {
		if have[i] != want[i] {
			t.Errorf("row %d is %q, want %q", i+1, have[i], want[i])
		}
	}
}

func TestSuggestChaMatchesWalkthrough(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	assertLabels(t, ask(t, e, "q=Cha"), "Chairs — 390 on hand", "Charcoal sacks — 12 on hand")
}

func TestSuggestIsCaseInsensitive(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	for _, q := range []string{"q=cha", "q=CHA", "q=" + url.QueryEscape("  Cha ")} {
		assertLabels(t, ask(t, e, q), "Chairs — 390 on hand", "Charcoal sacks — 12 on hand")
	}
}

func TestSuggestMatchesMidWord(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	assertLabels(t, ask(t, e, "q=drum"), "Water drums (20L) — 35 on hand")
}

func TestSuggestPrefixMatchesRankFirst(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Products = append(reg.Products, register.Product{
		ID: "PRD-0006", Name: "Folding chairs stand",
		CreatedAt: tenAM, CreatedBy: "Suresh Kumar",
	})
	e := newTestServer(t, reg, tenAM)

	assertLabels(t, ask(t, e, "q=cha"),
		"Chairs — 390 on hand",
		"Charcoal sacks — 12 on hand",
		"Folding chairs stand — 0 on hand")
}

func TestSuggestInStockModeHidesEmpties(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	inStock := ask(t, e, "q=&mode=instock")
	for _, f := range inStock {
		if f.Name == "Extension boards" {
			t.Error("the issue screen offered Extension boards, which are all out")
		}
	}
	if len(inStock) != 4 {
		t.Errorf("the issue screen offered %v, want the four that are on hand", labels(inStock))
	}

	all := ask(t, e, "q=&mode=all")
	if len(all) != 5 {
		t.Errorf("the inward screen offered %v, want all five", labels(all))
	}
}

func TestSuggestOnHandFollowsTheRegister(t *testing.T) {
	t1 := newTestServer(t, register.WalkthroughT1(), tenAM)
	assertLabels(t, ask(t, t1, "q=Chairs"), "Chairs — 890 on hand")

	t3 := newTestServer(t, register.WalkthroughT3(), tenAM)
	assertLabels(t, ask(t, t3, "q=Chairs"), "Chairs — 925 on hand")
}

func TestSuggestCapsAtEight(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Products = nil
	for i := 1; i <= 20; i++ {
		reg.Products = append(reg.Products, register.Product{
			ID:        "PRD-" + pad(i),
			Name:      "Sample a" + string(rune('a'+i-1)),
			CreatedAt: tenAM, CreatedBy: "Suresh Kumar",
		})
	}
	e := newTestServer(t, reg, tenAM)

	if got := ask(t, e, "q=a"); len(got) != 8 {
		t.Errorf("the picker offered %d rows, want 8", len(got))
	}
}

// TestSuggestCapsAfterRanking guards the order of the two rules: rank the
// names that start with the query first, then cut the list to eight. Cut first
// and a person typing three letters is offered eight products that merely
// contain them while the one they meant is not on the list at all.
func TestSuggestCapsAfterRanking(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.Products = nil
	n := 0
	add := func(name string) {
		n++
		reg.Products = append(reg.Products, register.Product{
			ID: "PRD-" + pad(n), Name: name, CreatedAt: tenAM, CreatedBy: "Suresh Kumar",
		})
	}
	// Five that merely contain "tab", added first so register order alone
	// would put them at the top.
	for i := 1; i <= 5; i++ {
		add("Folding table " + string(rune('0'+i)))
	}
	// Ten that start with it.
	for i := 1; i <= 10; i++ {
		add("Tables " + string(rune('a'+i-1)))
	}
	e := newTestServer(t, reg, tenAM)

	found := ask(t, e, "q=tab")
	if len(found) != 8 {
		t.Fatalf("the picker offered %d rows, want 8", len(found))
	}
	for _, f := range found {
		if !strings.HasPrefix(strings.ToLower(f.Name), "tab") {
			t.Errorf("%q is on the list ahead of a name that starts with the query", f.Name)
		}
	}
}

func pad(n int) string {
	digits := []byte{'0', '0', '0', '0'}
	for i := 3; i >= 0 && n > 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

func TestCreateProductAppends(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, _ := e.post("/product/new", url.Values{
		"name":   {"Gas cylinders"},
		"return": {"/inward/new"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /product/new returned %d, want 303", resp.StatusCode)
	}

	saved := e.saved()
	if len(saved.Products) != 6 {
		t.Fatalf("the saved register has %d products, want 6", len(saved.Products))
	}
	added := saved.Products[5]
	if added.ID != "PRD-0006" || added.Name != "Gas cylinders" {
		t.Errorf("the new product is %q named %q, want PRD-0006 Gas cylinders", added.ID, added.Name)
	}
	if !added.CreatedAt.Equal(tenAM) {
		t.Errorf("the new product was created at %v, want %v", added.CreatedAt, tenAM)
	}
	if added.CreatedBy != "Suresh Kumar" {
		t.Errorf("the new product was added by %q, want Suresh Kumar", added.CreatedBy)
	}

	// The form it came from opens with the product already chosen.
	if got := resp.Header.Get("Location"); got != "/inward/new?added=Gas+cylinders&picked=PRD-0006" {
		t.Errorf("came back to %q, want the inward form with the new product picked", got)
	}
}

func TestCreateProductRefusesCaseDuplicate(t *testing.T) {
	tests := []struct {
		name  string
		typed string
	}{
		{"lower case", "chairs"},
		{"upper case", "CHAIRS"},
		{"exactly as it is", "Chairs"},
		{"padded", "  chairs  "},
		{"trailing tab", "Chairs\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), tenAM)

			resp, body := e.post("/product/new", url.Values{"name": {tt.typed}, "return": {"/inward/new"}})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST /product/new returned %d, want 200", resp.StatusCode)
			}
			assertContains(t, body, "Chairs is already on the list. Pick it.")

			if got := len(e.saved().Products); got != 5 {
				t.Errorf("the saved register has %d products, want 5", got)
			}
		})
	}
}

func TestCreateProductTrimsAndCollapses(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	e.post("/product/new", url.Values{"name": {"  Gas   cylinders "}, "return": {"/inward/new"}})

	saved := e.saved()
	if len(saved.Products) != 6 {
		t.Fatalf("the saved register has %d products, want 6", len(saved.Products))
	}
	if got := saved.Products[5].Name; got != "Gas cylinders" {
		t.Errorf("the new product is named %q, want Gas cylinders", got)
	}
}

func TestCreateProductRefusesBlank(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.post("/product/new", url.Values{"name": {"   "}, "return": {"/inward/new"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /product/new returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Type the new product's name.")

	if got := len(e.saved().Products); got != 5 {
		t.Errorf("the saved register has %d products, want 5", got)
	}
}

func TestCreateProductAsksAboutANearDuplicate(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.post("/product/new", url.Values{"name": {"Chair"}, "return": {"/inward/new"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /product/new returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Did you mean Chairs?")
	assertContains(t, body, `Adding "Chair" makes a second, separate product.`)

	if got := len(e.saved().Products); got != 5 {
		t.Errorf("the saved register has %d products, want 5 until it is confirmed", got)
	}

	// Confirmed once, deliberately, it goes in.
	resp, _ = e.post("/product/new", url.Values{
		"name": {"Chair"}, "return": {"/inward/new"}, "confirm": {"yes"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the confirmed product returned %d, want 303", resp.StatusCode)
	}
	saved := e.saved()
	if len(saved.Products) != 6 || saved.Products[5].Name != "Chair" {
		t.Errorf("the confirmed product was not saved: %d products", len(saved.Products))
	}
}

// TestNearDuplicateFiresOnFourSharedCharacters records that the guard is
// deliberately eager: two genuinely different products that share four opening
// characters are confirmed once, not refused. Water drums and a Water tanker
// are both real things, and one press apart.
func TestNearDuplicateFiresOnFourSharedCharacters(t *testing.T) {
	tests := []struct {
		name    string
		typed   string
		asks    bool
		against string
	}{
		{"a missing letter", "Chair", true, "Chairs"},
		{"a plural", "Round table", true, "Round tables"},
		{"four shared characters, different thing", "Water tanker", true, "Water drums (20L)"},
		{"three shared characters is not enough", "Chalk boxes", false, ""},
		{"nothing like anything on the list", "Gas cylinders", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), tenAM)

			resp, body := e.post("/product/new", url.Values{"name": {tt.typed}, "return": {"/inward/new"}})
			if tt.asks {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("returned %d, want 200 with the question", resp.StatusCode)
				}
				assertContains(t, body, "Did you mean "+tt.against+"?")
				if got := len(e.saved().Products); got != 5 {
					t.Errorf("%d products were saved before the question was answered", got)
				}
				return
			}
			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("returned %d, want 303 straight through", resp.StatusCode)
			}
			if got := len(e.saved().Products); got != 6 {
				t.Errorf("the saved register has %d products, want 6", got)
			}
		})
	}
}

func TestCreateProductWithNoNearDuplicateNeedsNoConfirmation(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, _ := e.post("/product/new", url.Values{"name": {"Gas cylinders"}, "return": {"/inward/new"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("a name like nothing on the list returned %d, want 303 straight through", resp.StatusCode)
	}
}

func TestProductRoutesNeedAShift(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.OnDutyStaffID = ""
	reg.ShiftStartedAt = nil
	e := newTestServer(t, reg, tenAM)

	for _, path := range []string{"/api/products?q=Cha"} {
		resp, _ := e.get(path)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s returned %d with nobody on duty, want 303", path, resp.StatusCode)
		}
	}

	resp, _ := e.post("/product/new", url.Values{"name": {"Gas cylinders"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("POST /product/new returned %d with nobody on duty, want 303", resp.StatusCode)
	}
	if got := len(e.saved().Products); got != 5 {
		t.Errorf("a product was added with nobody on duty: %d products", got)
	}
}
