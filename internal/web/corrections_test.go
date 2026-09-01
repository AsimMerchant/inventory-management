package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"storeregister/internal/register"
)

func inwardEditForm() url.Values {
	return url.Values{
		"quantity": {"500"}, "receivedOn": {"2026-09-03"}, "basis": {"rent"},
		"supplier": {"Sharma Tent House"}, "challanNo": {"STH/4471"}, "from": {"/inwards"},
	}
}

func issueEditForm() url.Values {
	return url.Values{
		"quantity": {"10"}, "takerName": {"Ravi Menon"}, "takerDepartment": {"Catering"},
		"takerMobile": {"98861 40023"}, "issuedAt": {"2026-09-03T14:18"}, "from": {"/out"},
	}
}

func TestFixJointRecipientsIsAudited(t *testing.T) {
	reg := jointReturnRegister()
	allocation := []register.Allocation{{IssueID: "ISS-9002", Quantity: 5}}
	reg.Returns = append(reg.Returns, register.Return{ID: "RET-9001", ProductID: "PRD-0001", Allocations: allocation, ReturnerName: "Amit Sharma", ReturnedAt: tenFortyFive, RecordedAt: tenFortyFive})
	e := newTestServer(t, reg, tenFortyFive)
	form := url.Values{
		"quantity":                  {"30"},
		"takerName":                 {"Ravi Menon"},
		"takerDepartment":           {"Catering"},
		"takerMobile":               {"98861 40023"},
		"additionalTakersPresent":   {"1"},
		"additionalTakerName":       {"Amit Sharma", "Meera Pillai"},
		"additionalTakerDepartment": {"Setup", "Hospitality"},
		"additionalTakerMobile":     {"97740 11298", "95550 11223"},
		"issuedAt":                  {"2026-09-03T15:01"},
		"from":                      {"/out"},
	}
	if location := postEdit(t, e, "ISS-9002", form); location != "/out?fixed=ISS-9002" {
		t.Fatalf("redirect = %q", location)
	}
	saved := e.saved()
	if !reflect.DeepEqual(saved.Returns[0].Allocations, allocation) {
		t.Fatalf("allocation changed from %#v to %#v", allocation, saved.Returns[0].Allocations)
	}
	is := saved.Issues[len(saved.Issues)-1]
	if len(is.Changes) != 2 || is.Changes[0].Field != "recipients" || is.Changes[1].Field != "recipientDetails" {
		t.Fatalf("changes = %#v", is.Changes)
	}
	if is.Changes[0].From != "Ravi Menon, Amit Sharma and Suresh Patel" || is.Changes[0].To != "Ravi Menon, Amit Sharma and Meera Pillai" {
		t.Fatalf("recipient audit = %#v", is.Changes[0])
	}
	if is.Quantity != 30 || len(register.Validate(saved)) != 0 {
		t.Fatalf("corrected register invalid or quantity changed: %#v", register.Validate(saved))
	}
}

func TestFixJointRecipientDetailSwapIsAudited(t *testing.T) {
	reg := jointReturnRegister()
	d, ok := editForm(reg, "ISS-9002")
	if !ok {
		t.Fatal("joint issue missing")
	}
	d.Additional[0].Department, d.Additional[1].Department = d.Additional[1].Department, d.Additional[0].Department
	d.Additional[0].Mobile, d.Additional[1].Mobile = d.Additional[1].Mobile, d.Additional[0].Mobile
	changes, refusal := applyEdit(reg, d, "Anita Rao", tenFortyFive)
	if refusal != "" {
		t.Fatal(refusal)
	}
	if len(changes) != 1 || changes[0].Field != "recipientDetails" {
		t.Fatalf("changes = %#v, want one recipientDetails audit", changes)
	}
	if changes[0].From == changes[0].To {
		t.Fatalf("detail audit did not preserve ordered before/after values: %#v", changes[0])
	}
}

func returnEditForm() url.Values {
	return url.Values{
		"quantity": {"45"}, "returnerName": {"Ravi Menon"}, "returnerMobile": {"98861 40023"},
		"returnedAt": {"2026-09-03T18:05"}, "disposition": {"wont_return"},
		"remark": {"5 chairs broke during setup near the stage. Ravi informed."}, "from": {"/out"},
	}
}

func postEdit(t *testing.T, e *env, id string, form url.Values) string {
	t.Helper()
	resp, body := e.post("/entry/"+id+"/edit", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit returned %d, want 303; body: %s", resp.StatusCode, body)
	}
	return resp.Header.Get("Location")
}

func refusedCorrection(t *testing.T, e *env, path string, form url.Values, want string) {
	t.Helper()
	beforeMain := readMaybe(t, e.path)
	beforeBak := readMaybe(t, e.path+".bak")
	beforeMemory := memoryJSON(t, e)
	resp, body := e.post(path, form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refusal returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, want)
	assertContains(t, body, "banner bad")
	for _, bad := range []string{"invalid", "error", "nil", "panic"} {
		assertNotContains(t, body, bad)
	}
	if strings.Contains(body, pathRecordID(path)) {
		t.Errorf("refusal body contains record ID %q", pathRecordID(path))
	}
	if got := readMaybe(t, e.path); !reflect.DeepEqual(got, beforeMain) {
		t.Error("refusal changed the main file")
	}
	if got := readMaybe(t, e.path+".bak"); !reflect.DeepEqual(got, beforeBak) {
		t.Error("refusal changed the backup file")
	}
	if got := memoryJSON(t, e); !reflect.DeepEqual(got, beforeMemory) {
		t.Error("refusal changed the register in memory")
	}
}

func readMaybe(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return b
}

func memoryJSON(t *testing.T, e *env) []byte {
	t.Helper()
	var b []byte
	e.st.Read(func(reg *register.Register) { b, _ = json.Marshal(reg) })
	return b
}

func pathRecordID(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return parts[2]
	}
	return ""
}

func findInward(t *testing.T, reg *register.Register, id string) register.Inward {
	t.Helper()
	for _, in := range reg.Inwards {
		if in.ID == id {
			return in
		}
	}
	t.Fatalf("missing inward %s", id)
	return register.Inward{}
}

func findIssue(t *testing.T, reg *register.Register, id string) register.Issue {
	t.Helper()
	for _, is := range reg.Issues {
		if is.ID == id {
			return is
		}
	}
	t.Fatalf("missing issue %s", id)
	return register.Issue{}
}

func findReturn(t *testing.T, reg *register.Register, id string) register.Return {
	t.Helper()
	for _, re := range reg.Returns {
		if re.ID == id {
			return re
		}
	}
	t.Fatalf("missing return %s", id)
	return register.Return{}
}

func TestFixInwardQuantity(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortyFive)
	f := inwardEditForm()
	f.Set("quantity", "50")
	if got := postEdit(t, e, "INW-0007", f); got != "/inwards?fixed=INW-0007" {
		t.Errorf("redirect %q", got)
	}
	r := e.saved()
	in := findInward(t, r, "INW-0007")
	if register.OnHand(r, "PRD-0001") != 440 || register.CameIn(r, "PRD-0001") != 750 {
		t.Error("wrong chair totals")
	}
	want := register.Change{At: tenFortyFive, By: "Suresh Kumar", Field: "quantity", Label: "How many", From: "500", To: "50"}
	if !reflect.DeepEqual(in.Changes, []register.Change{want}) {
		t.Errorf("changes %+v", in.Changes)
	}
	_, body := e.get("/inwards")
	assertContains(t, body, "Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am")
}

func TestFixInwardSupplierAndChallan(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortyFive)
	f := inwardEditForm()
	f.Set("supplier", "Sharma Tent House & Sons")
	f.Set("challanNo", "STH/4472")
	postEdit(t, e, "INW-0007", f)
	r := e.saved()
	in := findInward(t, r, "INW-0007")
	if len(in.Changes) != 2 || in.Changes[0].Field != "supplier" || in.Changes[1].Field != "challan" {
		t.Fatalf("changes %+v", in.Changes)
	}
	rows := register.SupplierRows(r)
	got := map[string]int{}
	for _, row := range rows {
		if row.ProductID == "PRD-0001" {
			got[row.Supplier] += row.CameIn
		}
	}
	if got["Sharma Tent House & Sons"] != 500 || got["Sharma Tent House"] != 390 {
		t.Errorf("supplier totals %+v", got)
	}
}

func TestFixInwardChangingNothingAppendsNothing(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortyFive)
	postEdit(t, e, "INW-0007", inwardEditForm())
	if got := findInward(t, e.saved(), "INW-0007").Changes; len(got) != 0 {
		t.Errorf("changes %+v", got)
	}
}

func TestFixInwardRefusedBelowWhatIsOut(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyFive)
	postEdit(t, e, "INW-0002", url.Values{"quantity": {"310"}, "receivedOn": {"2026-09-02"}, "basis": {"purchase"}, "reason": {"unused"}})
	// The setup above was an unchanged edit; delete the second inward as specified.
	resp, _ := e.post("/entry/INW-0002/delete", url.Values{"reason": {"Fixture setup."}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup delete returned %d", resp.StatusCode)
	}
	f := url.Values{"quantity": {"200"}, "receivedOn": {"2026-09-01"}, "basis": {"rent"}, "supplier": {"Sharma Tent House"}, "challanNo": {"STH/4390"}, "from": {"/inwards"}}
	refusedCorrection(t, e, "/entry/INW-0001/edit", f, "310 chairs are out with people. Take some back before you go below 310 chairs.")
	f.Set("quantity", "310")
	postEdit(t, e, "INW-0001", f)
	if got := register.OnHand(e.saved(), "PRD-0001"); got != 0 {
		t.Errorf("on hand %d", got)
	}
}

func TestFixInwardBoundaryIsTheFieldRuleNotTheRegisterRule(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortyFive)
	f := url.Values{"quantity": {"1"}, "receivedOn": {"2026-09-01"}, "basis": {"rent"}, "supplier": {"Sharma Tent House"}, "challanNo": {"STH/4390"}, "from": {"/inwards"}}
	postEdit(t, e, "INW-0001", f)
	if register.CameIn(e.saved(), "PRD-0001") != 311 || register.OnHand(e.saved(), "PRD-0001") != 1 {
		t.Error("boundary totals wrong")
	}
	f.Set("quantity", "0")
	refusedCorrection(t, e, "/entry/INW-0001/edit", f, "How many came in? Type a number of 1 or more.")
}

func TestFixInwardCannotChangeProduct(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortyFive)
	f := inwardEditForm()
	f.Set("productId", "PRD-0002")
	postEdit(t, e, "INW-0007", f)
	in := findInward(t, e.saved(), "INW-0007")
	if in.ProductID != "PRD-0001" || len(in.Changes) != 0 {
		t.Errorf("inward changed: %+v", in)
	}
	_, body := e.get("/entry/INW-0007/edit")
	assertContains(t, body, "Wrong product? Delete this entry and enter it again.")
}

func TestFixIssueQuantityUp(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT2(), tenFortyFive)
	f := issueEditForm()
	f.Set("quantity", "50")
	postEdit(t, e, "ISS-0008", f)
	r := e.saved()
	if register.OnHand(r, "PRD-0001") != 840 {
		t.Error("wrong on hand")
	}
	if got := raviChairTotal(r); got != 90 {
		t.Errorf("Ravi holds %d", got)
	}
}

func TestFixIssueRefusedAboveOnHand(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT2(), tenFortyFive)
	f := issueEditForm()
	f.Set("quantity", "900")
	refusedCorrection(t, e, "/entry/ISS-0008/edit", f, "Counting the 10 chairs already on this entry, you have 890 chairs. You cannot give out more than 890.")
	f.Set("quantity", "890")
	postEdit(t, e, "ISS-0008", f)
	if register.OnHand(e.saved(), "PRD-0001") != 0 {
		t.Error("boundary was not accepted")
	}
}

func TestFixIssueRefusedBelowWhatCameBack(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortyFive)
	f := issueEditForm()
	f.Set("quantity", "30")
	f.Set("issuedAt", "2026-09-03T09:40")
	refusedCorrection(t, e, "/entry/ISS-0003/edit", f, "40 chairs have already come back. To go below 40 chairs, fix that return first.")
	f.Set("quantity", "40")
	postEdit(t, e, "ISS-0003", f)
}

func TestFixIssueTaker(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT2(), tenFortyFive)
	f := issueEditForm()
	f.Set("takerMobile", "98861 40024")
	postEdit(t, e, "ISS-0008", f)
	people := register.PeopleHolding(e.saved())
	var totals []int
	for _, p := range people {
		if p.Name == "Ravi Menon" {
			totals = append(totals, p.TotalOut)
		}
	}
	if !reflect.DeepEqual(totals, []int{42, 10}) && !reflect.DeepEqual(totals, []int{10, 42}) {
		t.Errorf("Ravi totals %v", totals)
	}
}

func TestFixIssueTime(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT2(), tenFortyFive)
	before := findIssue(t, register.WalkthroughT2(), "ISS-0008").RecordedAt
	f := issueEditForm()
	f.Set("issuedAt", "2026-09-03T13:05")
	postEdit(t, e, "ISS-0008", f)
	is := findIssue(t, e.saved(), "ISS-0008")
	if len(is.Changes) != 1 || is.Changes[0].From != "2:18 pm" || is.Changes[0].To != "1:05 pm" || !is.RecordedAt.Equal(before) {
		t.Errorf("issue %+v", is)
	}
}

func raviChairTotal(r *register.Register) int {
	total := 0
	for _, p := range register.PeopleHolding(r) {
		if p.Name == "Ravi Menon" {
			for _, line := range p.Lines {
				if line.ProductID == "PRD-0001" {
					total += line.Out
				}
			}
		}
	}
	return total
}

func TestFixReturnQuantityDownReallocates(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortyFive)
	f := returnEditForm()
	f.Set("quantity", "40")
	f.Set("remark", "10 chairs broke during setup near the stage. Ravi informed.")
	postEdit(t, e, "RET-0001", f)
	r := e.saved()
	re := findReturn(t, r, "RET-0001")
	if !reflect.DeepEqual(re.Allocations, []register.Allocation{{IssueID: "ISS-0003", Quantity: 40}}) || re.ShortQuantity != 10 || register.OnHand(r, "PRD-0001") != 920 || raviChairTotal(r) != 10 {
		t.Errorf("return %+v", re)
	}
}

func TestFixReturnQuantityUpClearsTheShortfall(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortyFive)
	f := returnEditForm()
	f.Set("quantity", "50")
	f.Set("disposition", "")
	f.Set("remark", "")
	postEdit(t, e, "RET-0001", f)
	r := e.saved()
	re := findReturn(t, r, "RET-0001")
	wantA := []register.Allocation{{IssueID: "ISS-0003", Quantity: 40}, {IssueID: "ISS-0008", Quantity: 10}}
	if !reflect.DeepEqual(re.Allocations, wantA) || re.ShortQuantity != 0 || re.ShortDisposition != "" || re.Remark != "" || register.OnHand(r, "PRD-0001") != 930 || raviChairTotal(r) != 0 {
		t.Errorf("return %+v", re)
	}
	if len(re.Changes) != 3 {
		t.Fatalf("changes %+v", re.Changes)
	}
	_, body := e.get("/out")
	assertContains(t, body, "Removed the note that said: 5 chairs broke during setup near the stage. Ravi informed. by Suresh Kumar, 10:45 am")
	assertNotContains(t, body, "to </")
}

func TestFixReturnRefusedAboveWhatWasTaken(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortyFive)
	f := returnEditForm()
	f.Set("quantity", "51")
	refusedCorrection(t, e, "/entry/RET-0001/edit", f, "Ravi Menon took 50 chairs. You cannot put back more than 50.")
}

func TestFixReturnStillDemandsARemarkWhenShort(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortyFive)
	f := returnEditForm()
	f.Set("quantity", "40")
	f.Set("disposition", "")
	f.Set("remark", "")
	refusedCorrection(t, e, "/entry/RET-0001/edit", f, "Tap one: the 10 chairs are coming back later, or they are gone.")
	f.Set("disposition", "wont_return")
	refusedCorrection(t, e, "/entry/RET-0001/edit", f, "Write what happened to the 10 chairs.")
}

func TestFixReturnReallocationMatchesAFreshEntry(t *testing.T) {
	base := register.WalkthroughT3()
	d, _ := editForm(base, "RET-0001")
	d.Quantity = "40"
	d.Remark = "10 chairs broke during setup near the stage. Ravi informed."
	applyEdit(base, d, "Suresh Kumar", tenFortyFive)
	fresh := register.WalkthroughT3()
	fresh.Returns = nil
	plan := register.PlanReturn(fresh, []string{"ISS-0003", "ISS-0008"}, 40)
	if !reflect.DeepEqual(findReturn(t, base, "RET-0001").Allocations, plan.Allocations) {
		t.Error("edited allocation differs from fresh allocation")
	}
}

func TestDeleteInwardEnteredTwice(t *testing.T) {
	r := register.WalkthroughT1()
	dup := r.Inwards[len(r.Inwards)-1]
	dup.ID = "INW-0008"
	r.Inwards = append(r.Inwards, dup)
	e := newTestServer(t, r, tenFortySeven)
	resp, _ := e.post("/entry/INW-0008/delete", url.Values{"reason": {"Entered twice by mistake."}})
	if resp.StatusCode != http.StatusSeeOther || register.OnHand(e.saved(), "PRD-0001") != 890 {
		t.Errorf("delete status %d", resp.StatusCode)
	}
	if findInward(t, e.saved(), "INW-0008").Deleted == nil {
		t.Fatal("no tombstone")
	}
	_, body := e.get("/inwards")
	assertContains(t, body, "Deleted by Suresh Kumar, 10:47 am — Entered twice by mistake.")
}

func TestDeleteRefusedWithoutAReason(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortySeven)
	refusedCorrection(t, e, "/entry/INW-0007/delete", url.Values{"reason": {"   "}}, "Why are you deleting this?")
}

func TestDeleteInwardRefusedWhenStockHasGoneOut(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenFortySeven)
	resp, _ := e.post("/entry/INW-0002/delete", url.Values{"reason": {"Fixture setup."}})
	if resp.StatusCode != 303 {
		t.Fatal("setup delete refused")
	}
	if register.OnHand(e.saved(), "PRD-0001") != 80 {
		t.Fatal("setup total wrong")
	}
	refusedCorrection(t, e, "/entry/INW-0001/delete", url.Values{"reason": {"Wrong."}}, "310 of these chairs are out with people. Take them back first, then delete this.")
}

func TestDeleteIssueWithNothingBack(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortySeven)
	resp, _ := e.post("/entry/ISS-0005/delete", url.Values{"reason": {"Ravi never took the tables."}})
	if resp.StatusCode != 303 || register.OnHand(e.saved(), "PRD-0002") != 50 || raviChairTotal(e.saved()) != 5 {
		t.Errorf("delete status %d", resp.StatusCode)
	}
}

func TestDeleteIssueRefusedWhenSomethingCameBack(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortySeven)
	refusedCorrection(t, e, "/entry/ISS-0003/delete", url.Values{"reason": {"Wrong."}}, "40 chairs have already come back. Delete that return first, then this.")
}

func TestDeleteReturnThenTheIssue(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT3(), tenFortySeven)
	resp, _ := e.post("/entry/RET-0001/delete", url.Values{"reason": {"Wrong person, Ravi never brought these."}})
	if resp.StatusCode != 303 {
		t.Fatal("return delete refused")
	}
	if register.OnHand(e.saved(), "PRD-0001") != 880 || raviChairTotal(e.saved()) != 50 {
		t.Fatal("return delete totals wrong")
	}
	resp, _ = e.post("/entry/ISS-0003/delete", url.Values{"reason": {"Entered twice."}})
	if resp.StatusCode != 303 {
		t.Fatal("issue delete refused")
	}
}

func negativeOnHandRegister() *register.Register {
	r := register.WalkthroughT0()
	r.Inwards = r.Inwards[:1]
	r.Inwards[0].Quantity = 10
	r.Issues = nil
	r.Returns = nil
	r.Issues = append(r.Issues, register.Issue{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 10, TakerName: "Ravi Menon", TakerMobile: "1", IssuedAt: twoEighteen, RecordedAt: twoEighteen})
	r.Returns = append(r.Returns, register.Return{ID: "RET-0001", ProductID: "PRD-0001", Allocations: []register.Allocation{{IssueID: "ISS-0001", Quantity: 10}}, ReturnerName: "Ravi Menon", ReturnerMobile: "1", ReturnedAt: sixOhFive, RecordedAt: sixOhFive})
	r.Issues = append(r.Issues, register.Issue{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 10, TakerName: "Anita Rao", TakerMobile: "2", IssuedAt: tenFortyFive, RecordedAt: tenFortyFive})
	return r
}

func TestDeleteReturnRefusedWhenOnHandWouldGoNegative(t *testing.T) {
	e := newTestServer(t, negativeOnHandRegister(), tenFortySeven)
	refusedCorrection(t, e, "/entry/RET-0001/delete", url.Values{"reason": {"Wrong."}}, "20 chairs have gone out and only 10 came in. Delete the issue that took them first, then this.")
	f := url.Values{"quantity": {"3"}, "returnerName": {"Ravi Menon"}, "returnerMobile": {"1"}, "returnedAt": {"2026-09-03T18:05"}, "disposition": {"wont_return"}, "remark": {"7 chairs missing."}, "from": {"/out"}}
	refusedCorrection(t, e, "/entry/RET-0001/edit", f, "20 chairs have gone out and only 10 came in. Keep this at 10 chairs or more, or delete the issue first.")
}

func TestDeletedEntryCannotBeEditedAgain(t *testing.T) {
	r := register.WalkthroughT1()
	d := &register.Deletion{At: tenFortySeven, By: "Suresh Kumar", Reason: "Entered twice."}
	r.Inwards[6].Deleted = d
	e := newTestServer(t, r, tenFortySeven)
	want := "Deleted by Suresh Kumar, 10:47 am. Enter it again if that was a mistake."
	resp, body := e.get("/entry/INW-0007/edit")
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	assertContains(t, body, want)
	assertNotContains(t, body, "Save this fix")
	before := memoryJSON(t, e)
	for _, path := range []string{"/entry/INW-0007/edit", "/entry/INW-0007/delete"} {
		resp, body = e.post(path, inwardEditForm())
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
		assertContains(t, body, want)
	}
	if !reflect.DeepEqual(before, memoryJSON(t, e)) {
		t.Error("deleted record changed")
	}
}

func TestValidateIsCleanAfterEverySuccessfulCorrection(t *testing.T) {
	cases := []struct {
		name, id string
		reg      *register.Register
		form     url.Values
	}{
		{"inward", "INW-0007", register.WalkthroughT1(), func() url.Values { f := inwardEditForm(); f.Set("quantity", "50"); return f }()},
		{"issue", "ISS-0008", register.WalkthroughT2(), func() url.Values { f := issueEditForm(); f.Set("quantity", "50"); return f }()},
		{"return", "RET-0001", register.WalkthroughT3(), func() url.Values {
			f := returnEditForm()
			f.Set("quantity", "40")
			f.Set("remark", "10 chairs missing.")
			return f
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestServer(t, tc.reg, tenFortyFive)
			postEdit(t, e, tc.id, tc.form)
			if p := register.Validate(e.saved()); len(p) > 0 {
				t.Errorf("problems %+v", p)
			}
		})
	}
}

func TestRefusedCorrectionWritesNothing(t *testing.T) {
	cases := []struct {
		name, path, want string
		reg              *register.Register
		form             url.Values
	}{
		{"issue over", "/entry/ISS-0008/edit", "Counting the 10 chairs", register.WalkthroughT2(), func() url.Values { f := issueEditForm(); f.Set("quantity", "900"); return f }()},
		{"return over", "/entry/RET-0001/edit", "Ravi Menon took 50 chairs", register.WalkthroughT3(), func() url.Values { f := returnEditForm(); f.Set("quantity", "51"); return f }()},
		{"delete allocated issue", "/entry/ISS-0003/delete", "40 chairs have already come back", register.WalkthroughT3(), url.Values{"reason": {"Wrong."}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestServer(t, tc.reg, tenFortyFive)
			refusedCorrection(t, e, tc.path, tc.form, tc.want)
		})
	}
}

func TestCorrectionsAccumulate(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT1(), tenFortyFive)
	f := inwardEditForm()
	f.Set("quantity", "50")
	postEdit(t, e, "INW-0007", f)
	f.Set("quantity", "55")
	postEdit(t, e, "INW-0007", f)
	f.Set("challanNo", "STH/4472")
	postEdit(t, e, "INW-0007", f)
	changes := findInward(t, e.saved(), "INW-0007").Changes
	if len(changes) != 3 || changes[0].From != "500" || changes[0].To != "50" {
		t.Errorf("changes %+v", changes)
	}
}
