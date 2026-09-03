package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
)

// donatedInward is the walkthrough delivery with the third choice ticked and a
// word typed for the first time.
func donatedInward(word string) url.Values {
	form := walkthroughInward()
	form.Set("basis", "other")
	form.Set("newKind", word)
	return form
}

// kindsOnFile reads the shared list back off disk, which is the only copy that
// matters.
func kindsOnFile(t *testing.T, e *env) []register.AcquisitionKind {
	t.Helper()
	return e.saved().AcquisitionKinds
}

func TestInwardFormOffersTheTypedKinds(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.AcquisitionKinds = []register.AcquisitionKind{
		{ID: "AKD-0001", Name: "Donated", CreatedAt: tenFortyTwo, CreatedBy: "Suresh Kumar"},
	}
	e := newTestServer(t, reg, tenFortyTwo)

	_, body := e.get("/inward/new")
	for _, want := range []string{
		"Some other way — donated, sponsored, borrowed",
		`<option value="AKD-0001"`,
		"Donated",
		`name="newKind"`,
	} {
		assertContains(t, body, want)
	}
	// The whole control is plain markup: a select and a text box, both of
	// which submit with JavaScript switched off.
	if strings.Contains(body, `data-values data-kind="acquisitionKind"`) {
		t.Error("the kind box needs a script to work")
	}
}

func TestInwardSavesATypedKind(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)

	resp, body := e.post("/inward/new", donatedInward("Donated"))
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("saving = %d: %s", resp.StatusCode, body)
	}
	kinds := kindsOnFile(t, e)
	if len(kinds) != 1 || kinds[0].Name != "Donated" || kinds[0].ID != "AKD-0001" {
		t.Fatalf("the shared list is %+v", kinds)
	}
	if kinds[0].CreatedBy != "Suresh Kumar" {
		t.Errorf("the word was recorded against %q", kinds[0].CreatedBy)
	}
	saved := e.saved().Inwards
	last := saved[len(saved)-1]
	if last.Basis != register.Other || last.KindID != "AKD-0001" {
		t.Fatalf("the delivery saved as %q / %q", last.Basis, last.KindID)
	}

	// The same word again picks the row already there rather than making a
	// second one, and the list is offered as a choice from then on.
	second := donatedInward("")
	second.Set("kindId", "AKD-0001")
	if resp, body := e.post("/inward/new", second); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the second delivery = %d: %s", resp.StatusCode, body)
	}
	if got := kindsOnFile(t, e); len(got) != 1 {
		t.Errorf("the shared list grew to %d words", len(got))
	}
}

func TestInwardRefusesTheThirdChoiceWithNoWord(t *testing.T) {
	cases := []struct {
		name string
		set  func(url.Values)
		want string
	}{
		{"nothing picked and nothing typed", func(f url.Values) {}, "Pick how these came in from the list, or type a new word for it."},
		{"a word that is not on the list", func(f url.Values) { f.Set("kindId", "AKD-9999") }, "Pick how these came in from the list, or type a new word for it."},
		{"no choice at all", func(f url.Values) { f.Set("basis", "") }, "Choose how these came in."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
			form := donatedInward("")
			c.set(form)
			refusedInward(t, e, form, c.want)
			if got := kindsOnFile(t, e); len(got) != 0 {
				t.Errorf("a refused delivery left %d words on the list", len(got))
			}
		})
	}
}

func TestStockAndSuppliersShowTheTypedWord(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	// Charcoal sacks came in bought and nothing else, so the row has no rent
	// on it to speak for the product first.
	form := donatedInward("Donated")
	form.Set("productId", "PRD-0005")
	if resp, body := e.post("/inward/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("saving = %d: %s", resp.StatusCode, body)
	}

	_, stock := e.get("/stock")
	if !strings.Contains(stock, ">Donated<") {
		t.Error("the stock list does not show the typed word")
	}
	_, inwards := e.get("/inwards")
	if !strings.Contains(inwards, ">Donated<") {
		t.Error("what came in does not show the typed word")
	}
	_, suppliers := e.get("/suppliers")
	if !strings.Contains(suppliers, ">Donated<") {
		t.Error("the suppliers record does not show the typed word")
	}
}

func TestFixScreenCarriesTheThirdChoice(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.AcquisitionKinds = []register.AcquisitionKind{
		{ID: "AKD-0001", Name: "Donated", CreatedAt: tenFortyTwo, CreatedBy: "Suresh Kumar"},
	}
	e := newTestServer(t, reg, tenFortyFive)

	_, body := e.get("/entry/INW-0002/edit")
	for _, want := range []string{
		"Some other way — donated, sponsored, borrowed",
		`<option value="AKD-0001"`,
		`name="newKind"`,
	} {
		assertContains(t, body, want)
	}

	// A wrong tick is corrected here, including onto a word typed on this
	// very screen.
	form := url.Values{
		"quantity": {"310"}, "receivedOn": {"2026-09-02"},
		"basis": {"other"}, "newKind": {"Sponsored"},
		"supplier": {""}, "challanNo": {""},
	}
	resp, body := e.post("/entry/INW-0002/edit", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the correction = %d: %s", resp.StatusCode, body)
	}
	saved := e.saved()
	var fixed register.Inward
	for _, in := range saved.Inwards {
		if in.ID == "INW-0002" {
			fixed = in
		}
	}
	if fixed.Basis != register.Other || register.BasisWord(saved, fixed.Basis, fixed.KindID) != "Sponsored" {
		t.Fatalf("INW-0002 now reads %q / %q", fixed.Basis, fixed.KindID)
	}
	if len(fixed.Changes) != 1 || fixed.Changes[0].From != "Purchase" || fixed.Changes[0].To != "Sponsored" {
		t.Errorf("the correction was recorded as %+v", fixed.Changes)
	}
}

func TestFixScreenRefusesTheThirdChoiceWithNoWord(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyFive)
	form := url.Values{
		"quantity": {"310"}, "receivedOn": {"2026-09-02"}, "basis": {"other"},
		"supplier": {""}, "challanNo": {""},
	}
	resp, body := e.post("/entry/INW-0002/edit", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the refusal returned %d", resp.StatusCode)
	}
	assertContains(t, body, "Pick how these came in from the list, or type a new word for it.")
	for _, in := range e.saved().Inwards {
		if in.ID == "INW-0002" && in.Basis != register.Purchase {
			t.Error("a refused correction changed the delivery")
		}
	}
}

// TestOneSharedVocabulary is the rule the whole design rests on here: the desk
// decides, the ledger only offers words, and each side sees what the other
// typed.
func TestOneSharedVocabulary(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	admin, key, _ := financeAdmin(t, e)

	// A word typed at the desk reaches the money screen.
	if resp, body := e.post("/inward/new", donatedInward("Donated")); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the delivery = %d: %s", resp.StatusCode, body)
	}
	status, form := admin.get(t, "/finance/movements/new")
	if status != 200 {
		t.Fatalf("the money form answered %d", status)
	}
	for _, want := range []string{"Some other way — donated, sponsored, borrowed", `value="AKD-0001"`, "Donated"} {
		if !strings.Contains(form, want) {
			t.Errorf("the money screen does not offer %q", want)
		}
	}

	// A word typed on the money screen reaches the desk.
	money := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(money, 0, 0, "PRD-0002", "40", "other")
	money["newKind-0-0"] = []string{"Sponsored"}
	if status, body := admin.post(t, "/finance/movements/new", money); status != 303 {
		t.Fatalf("the money entry = %d: %s", status, body)
	}
	kinds := kindsOnFile(t, e)
	if len(kinds) != 2 || kinds[1].Name != "Sponsored" {
		t.Fatalf("the shared list is %+v", kinds)
	}
	line := orders(t, e, key)[0].Lines[0]
	if line.Basis != register.Other || line.KindID != kinds[1].ID {
		t.Fatalf("the order line saved as %q / %q", line.Basis, line.KindID)
	}
	_, desk := e.get("/inward/new")
	assertContains(t, desk, "Sponsored")

	// Nothing is pre-filled at the desk from what the ledger recorded.
	if strings.Contains(desk, "the ledger") {
		t.Error("the desk is being told what the ledger says")
	}
	if strings.Contains(desk, `value="other" checked`) {
		t.Error("the desk had a choice made for it")
	}
	reopenValid(t, e)
}

func TestMoneyScreenRefusesTheThirdChoiceWithNoWord(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	admin, _, _ := financeAdmin(t, e)
	money := moneyForm("out", "5000", "Sharma Events", "Advance", "Cash")
	moneyLine(money, 0, 0, "PRD-0002", "40", "other")
	status, body := admin.post(t, "/finance/movements/new", money)
	if status != 200 || !strings.Contains(body, "Pick how the goods came in from the list, or type a new word for it.") {
		t.Fatalf("gave %d without the refusal: %s", status, body)
	}
	if got := kindsOnFile(t, e); len(got) != 0 {
		t.Errorf("a refused entry left %d words on the list", len(got))
	}
}

// TestTypedKindOpensBothSettlementDoors is rule two, seen through the screens
// the person actually uses.
func TestTypedKindOpensBothSettlementDoors(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	admin, _, _ := financeAdmin(t, e)

	// Charcoal sacks came in bought; extension boards on rent. Add a donated
	// delivery of round tables.
	form := donatedInward("Donated")
	form.Set("productId", "PRD-0002")
	form.Set("quantity", "50")
	if resp, body := e.post("/inward/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the delivery = %d: %s", resp.StatusCode, body)
	}

	cases := []struct {
		mode    string
		product string
		want    bool
	}{
		{"return", "Round tables", true},
		{"sale", "Round tables", true},
		{"return", "Charcoal sacks", false}, // bought, so it only sells
		{"sale", "Charcoal sacks", true},
		{"return", "Water drums (20L)", false},
	}
	for _, c := range cases {
		got := false
		for _, name := range names(suggestionsOf(t, admin, "mode="+c.mode+"&q="+url.QueryEscape(c.product))) {
			if name == c.product {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("%s on the %s screen: offered=%v want=%v", c.product, c.mode, got, c.want)
		}
	}
}

func TestSharedListsCarryTheKinds(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	admin, _, _ := financeAdmin(t, e)
	first := donatedInward("Donatd")
	first.Set("productId", "PRD-0005")
	if resp, body := e.post("/inward/new", first); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("the delivery = %d: %s", resp.StatusCode, body)
	}
	// A second word, on nothing at all, so it may be deleted.
	form := donatedInward("Sponsored")
	form.Set("quantity", "1")
	if resp, _ := e.post("/inward/new", form); resp.StatusCode != http.StatusSeeOther {
		t.Fatal("the second delivery was refused")
	}
	if err := e.st.Update(func(reg *register.Register) error {
		reg.Inwards = reg.Inwards[:len(reg.Inwards)-1]
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	status, body := admin.get(t, "/finance/lists")
	if status != 200 {
		t.Fatalf("the lists screen answered %d", status)
	}
	for _, want := range []string{"Ways goods came in, besides rent and purchase", "Donatd", "Sponsored"} {
		if !strings.Contains(body, want) {
			t.Errorf("the lists screen does not show %q", want)
		}
	}

	// Rename, and every delivery saved with the typo shows the new word.
	if status, body := admin.post(t, "/finance/lists/AKD-0001/rename", url.Values{"value": {"Donated"}}); status != 303 {
		t.Fatalf("the rename = %d: %s", status, body)
	}
	_, stock := e.get("/stock")
	if !strings.Contains(stock, ">Donated<") {
		t.Error("the stock list did not follow the rename")
	}

	// A word in use is refused, in the words the other lists use.
	status, body = admin.post(t, "/finance/lists/AKD-0001/delete", url.Values{"confirm": {"yes"}})
	if status != 200 || !strings.Contains(body, "This kind has been used.") {
		t.Fatalf("deleting a used word gave %d: %s", status, body)
	}
	if len(kindsOnFile(t, e)) != 2 {
		t.Error("a refused delete changed the list")
	}

	// An unused one asks first and then goes.
	status, body = admin.post(t, "/finance/lists/AKD-0002/delete", nil)
	if status != 200 || !strings.Contains(body, "Yes, delete this unused value") {
		t.Fatalf("the confirmation gave %d: %s", status, body)
	}
	if status, body := admin.post(t, "/finance/lists/AKD-0002/delete", url.Values{"confirm": {"yes"}}); status != 303 {
		t.Fatalf("the delete = %d: %s", status, body)
	}
	if got := kindsOnFile(t, e); len(got) != 1 || got[0].Name != "Donated" {
		t.Errorf("the list is now %+v", got)
	}
}

func TestSharedListsMergeTwoWordsIntoOne(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyTwo)
	admin, _, _ := financeAdmin(t, e)
	for _, word := range []string{"Donated", "Donatd"} {
		form := donatedInward(word)
		if resp, body := e.post("/inward/new", form); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("the %s delivery = %d: %s", word, resp.StatusCode, body)
		}
	}
	status, body := admin.post(t, "/finance/lists/AKD-0002/merge", url.Values{"target": {"AKD-0001"}})
	if status != 200 || !strings.Contains(body, "Combine Donatd into Donated?") {
		t.Fatalf("the confirmation gave %d: %s", status, body)
	}
	if status, body := admin.post(t, "/finance/lists/AKD-0002/merge", url.Values{
		"target": {"AKD-0001"}, "confirm": {"yes"}, "confirmedTarget": {"AKD-0001"},
	}); status != 303 {
		t.Fatalf("the merge = %d: %s", status, body)
	}
	saved := e.saved()
	for _, in := range saved.Inwards {
		if in.Basis == register.Other && register.BasisWord(saved, in.Basis, in.KindID) != "Donated" {
			t.Errorf("%s still reads %q", in.ID, register.BasisWord(saved, in.Basis, in.KindID))
		}
	}
	if len(saved.AcquisitionKinds) != 2 {
		t.Error("the merged row was thrown away")
	}
}
