package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// fileState is the bytes and modification time of the register file, or a note
// that there is no file at all.
func fileState(t *testing.T, path string) (data []byte, mod time.Time, exists bool) {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, time.Time{}, false
	}
	if err != nil {
		t.Fatalf("looking at the register file: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the register file: %v", err)
	}
	return data, info.ModTime(), true
}

func TestShiftScreenListsEveryone(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.get("/shift")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /shift returned %d, want 200", resp.StatusCode)
	}
	for _, want := range []string{
		"Who is at the desk?", "Tap your name to begin.",
		"Suresh Kumar", "98450 22117",
		"Anita Rao", "99001 34562",
		"Imran Sheikh", "90080 77213",
		"Start shift", "+ Add a person",
	} {
		assertContains(t, body, want)
	}
}

func TestStartShiftSetsOnDuty(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, _ := e.post("/shift/start", url.Values{"staffId": {"STF-0002"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /shift/start returned %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/stock" {
		t.Errorf("started the shift and went to %q, want /stock", got)
	}

	saved := e.saved()
	if saved.OnDutyStaffID != "STF-0002" {
		t.Errorf("the saved register has %q on duty, want STF-0002", saved.OnDutyStaffID)
	}
	if saved.ShiftStartedAt == nil || !saved.ShiftStartedAt.Equal(tenAM) {
		t.Errorf("the saved shift started at %v, want %v", saved.ShiftStartedAt, tenAM)
	}

	_, body := e.get("/stock")
	assertContains(t, body, "Anita Rao · on duty")
}

func TestStartShiftWithNoSelection(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	before, beforeMod, _ := fileState(t, e.path)

	resp, body := e.post("/shift/start", url.Values{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /shift/start with nothing chosen returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Tap your name first.")

	if e.saved().OnDutyStaffID != "STF-0001" {
		t.Error("the on-duty person changed when nobody was chosen")
	}
	after, afterMod, _ := fileState(t, e.path)
	if string(before) != string(after) || !beforeMod.Equal(afterMod) {
		t.Error("the register file was written when nothing was chosen")
	}
}

func TestStartShiftWithUnknownStaff(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)
	before, beforeMod, _ := fileState(t, e.path)

	resp, body := e.post("/shift/start", url.Values{"staffId": {"STF-9999"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /shift/start with an unknown person returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Tap your name first.")

	if e.saved().OnDutyStaffID != "STF-0001" {
		t.Error("the on-duty person changed for an unknown id")
	}
	after, afterMod, _ := fileState(t, e.path)
	if string(before) != string(after) || !beforeMod.Equal(afterMod) {
		t.Error("the register file was written for an unknown id")
	}
}

func TestAddPerson(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.post("/shift/person", url.Values{
		"name":   {"Imran  Sheikh Jr "},
		"mobile": {"90080 77214"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /shift/person returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Imran Sheikh Jr added.")

	saved := e.saved()
	if len(saved.Staff) != 4 {
		t.Fatalf("the saved register has %d people, want 4", len(saved.Staff))
	}
	added := saved.Staff[3]
	if added.ID != "STF-0004" {
		t.Errorf("the new person is %q, want STF-0004", added.ID)
	}
	if added.Name != "Imran Sheikh Jr" {
		t.Errorf("the new person is named %q, want the whitespace collapsed", added.Name)
	}
	if added.Mobile != "90080 77214" {
		t.Errorf("the new person's mobile is %q", added.Mobile)
	}
	if !added.CreatedAt.Equal(tenAM) {
		t.Errorf("the new person was created at %v, want %v", added.CreatedAt, tenAM)
	}
	if added.CreatedBy != "Suresh Kumar" {
		t.Errorf("the new person was added by %q, want Suresh Kumar", added.CreatedBy)
	}
}

func TestFirstPersonOnAFreshRegisterHasNoCreatedBy(t *testing.T) {
	e := newTestServer(t, nil, tenAM)

	e.post("/shift/person", url.Values{"name": {"Suresh Kumar"}, "mobile": {"98450 22117"}})

	saved := e.saved()
	if len(saved.Staff) != 1 {
		t.Fatalf("the saved register has %d people, want 1", len(saved.Staff))
	}
	first := saved.Staff[0]
	if first.ID != "STF-0001" {
		t.Errorf("the first person is %q, want STF-0001", first.ID)
	}
	if !first.CreatedAt.Equal(tenAM) {
		t.Errorf("the first person was created at %v, want %v", first.CreatedAt, tenAM)
	}
	if first.CreatedBy != "" {
		t.Errorf("the first person was added by %q, want nobody at all", first.CreatedBy)
	}

	e.post("/shift/start", url.Values{"staffId": {"STF-0001"}})
	e.post("/shift/person", url.Values{"name": {"Anita Rao"}, "mobile": {"99001 34562"}})

	saved = e.saved()
	if len(saved.Staff) != 2 {
		t.Fatalf("the saved register has %d people, want 2", len(saved.Staff))
	}
	if got := saved.Staff[1].CreatedBy; got != "Suresh Kumar" {
		t.Errorf("the second person was added by %q, want Suresh Kumar", got)
	}
}

func TestAddDuplicatePersonRefused(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.post("/shift/person", url.Values{"name": {"  anita   rao "}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /shift/person returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Anita Rao is already on the list.")

	if got := len(e.saved().Staff); got != 3 {
		t.Errorf("the saved register has %d people, want 3", got)
	}
}

func TestAddPersonWithBlankName(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.post("/shift/person", url.Values{"name": {"   "}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /shift/person returned %d, want 200", resp.StatusCode)
	}
	assertContains(t, body, "Type the person's name.")

	if got := len(e.saved().Staff); got != 3 {
		t.Errorf("the saved register has %d people, want 3", got)
	}
}

func TestFreshRegisterPromptsForFirstPerson(t *testing.T) {
	e := newTestServer(t, nil, tenAM)

	_, body := e.get("/shift")
	assertContains(t, body, "Nobody is on the list yet. Add the first person.")
	assertNotContains(t, body, "Suresh Kumar")
}

func TestEveryFlowRouteBlockedWithoutShift(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.OnDutyStaffID = ""
	reg.ShiftStartedAt = nil
	e := newTestServer(t, reg, tenAM)

	for _, path := range []string{"/inward/new", "/issue/new", "/return/new", "/suppliers"} {
		resp, _ := e.get(path)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s returned %d, want 303", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); got != "/shift" {
			t.Errorf("GET %s sent the person to %q, want /shift", path, got)
		}
	}
}

func TestShiftLivesWithinTheSameDay(t *testing.T) {
	nearMidnight := time.Date(2026, time.September, 3, 23, 58, 0, 0, register.IST)
	reg := register.WalkthroughT0()
	started := time.Date(2026, time.September, 3, 9, 40, 0, 0, register.IST)
	reg.ShiftStartedAt = &started
	e := newTestServerWith(t, reg, nearMidnight, "")

	var who register.Staff
	var live bool
	e.st.Read(func(r *register.Register) { who, live = e.srv.onDuty(r) })
	if !live || who.Name != "Suresh Kumar" {
		t.Errorf("onDuty returned %q, %v; want Suresh Kumar, true", who.Name, live)
	}

	resp, _ := e.get("/stock")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /stock returned %d late on the same day, want 200", resp.StatusCode)
	}
}

func TestShiftIsStaleOnANewDay(t *testing.T) {
	justAfterMidnight := time.Date(2026, time.September, 4, 0, 2, 0, 0, register.IST)
	reg := register.WalkthroughT0()
	started := time.Date(2026, time.September, 3, 9, 40, 0, 0, register.IST)
	reg.ShiftStartedAt = &started
	e := newTestServerWith(t, reg, justAfterMidnight, "")

	var live bool
	e.st.Read(func(r *register.Register) { _, live = e.srv.onDuty(r) })
	if live {
		t.Error("yesterday's shift is still live this morning")
	}

	resp, _ := e.get("/stock")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/shift" {
		t.Errorf("GET /stock returned %d to %q, want 303 to /shift", resp.StatusCode, resp.Header.Get("Location"))
	}

	if got := e.saved().OnDutyStaffID; got != "STF-0001" {
		t.Errorf("the saved register holds %q on duty, want STF-0001 left alone", got)
	}

	_, body := e.get("/shift")
	if !containsSelected(body, "STF-0001") {
		t.Error("the shift screen does not pre-select yesterday's person")
	}
}

// containsSelected reports whether the shift screen has this person's radio
// already chosen. An id is a value in the markup, never words on the screen, so
// this one assertion reads the raw body rather than going through
// assertContains.
func containsSelected(body, id string) bool {
	return strings.Contains(body, `value="`+id+`" checked`)
}

func TestStaleShiftIsIgnoredNotCleared(t *testing.T) {
	justAfterMidnight := time.Date(2026, time.September, 4, 0, 2, 0, 0, register.IST)
	reg := register.WalkthroughT0()
	started := time.Date(2026, time.September, 3, 9, 40, 0, 0, register.IST)
	reg.ShiftStartedAt = &started
	e := newTestServerWith(t, reg, justAfterMidnight, "")

	before, beforeMod, existed := fileState(t, e.path)
	if !existed {
		t.Fatal("the fixture was not written")
	}

	e.get("/stock")
	e.get("/shift")

	after, afterMod, _ := fileState(t, e.path)
	if string(before) != string(after) {
		t.Error("the register file changed when a stale shift was ignored")
	}
	if !beforeMod.Equal(afterMod) {
		t.Error("the register file was rewritten when a stale shift was ignored")
	}
}

func TestNoPasswordFieldAnywhere(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	_, body := e.get("/shift")
	assertNotContains(t, body, `type="password"`)
}
