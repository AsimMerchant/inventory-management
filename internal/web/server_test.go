package web

import (
	"html/template"
	"net/http"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

// tenAM is the walkthrough's home screen: 3 September 2026, 10:00 am.
var tenAM = time.Date(2026, time.September, 3, 10, 0, 0, 0, register.IST)

func TestUnknownPathIs404WithPlainSentence(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.get("/nowhere")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /nowhere returned %d, want 404", resp.StatusCode)
	}
	assertContains(t, body, "Nothing here.")
	for _, unwanted := range []string{"invalid", "goroutine", "register.Register", "*web.Server", ".go:"} {
		assertNotContains(t, body, unwanted)
	}
}

func TestNoShiftRedirectsToShift(t *testing.T) {
	reg := register.WalkthroughT0()
	reg.OnDutyStaffID = ""
	reg.ShiftStartedAt = nil
	e := newTestServer(t, reg, tenAM)

	for _, path := range []string{"/stock", "/issue/new", "/return/new"} {
		resp, _ := e.get(path)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s returned %d, want 303", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); got != "/shift" {
			t.Errorf("GET %s sent the person to %q, want /shift", path, got)
		}
	}

	resp, _ := e.get("/shift")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /shift returned %d, want 200", resp.StatusCode)
	}
}

func TestShellShowsOnDutyName(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	_, body := e.get("/stock")
	assertContains(t, body, "Suresh Kumar · on duty")
	for _, label := range []string{"Stock", "Out with people", "Stuff came in", "Suppliers", "Who did what"} {
		assertContains(t, body, label)
	}
}

func TestRecoveryWarningShowsOnEveryPage(t *testing.T) {
	warning := "Today's register was damaged. This is the last good copy, saved at 9:40 am on 3 September 2026. Anything entered after that time must be entered again."
	e := newTestServerWith(t, register.WalkthroughT0(), tenAM, warning)

	for _, path := range []string{"/stock", "/out", "/inwards", "/suppliers", "/log"} {
		resp, body := e.get(path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned %d, want 200", path, resp.StatusCode)
		}
		assertContains(t, body, warning)
		if !strings.Contains(body, `<div class="banner bad">`) {
			t.Errorf("GET %s does not carry the warning in a banner bad element", path)
		}
	}
}

func TestLongstampMatchesWalkthrough(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"morning", longstamp(time.Date(2026, time.September, 3, 10, 42, 0, 0, register.IST)), "Thursday, 3 September · 10:42 am"},
		{"afternoon", longstamp(time.Date(2026, time.September, 3, 14, 18, 0, 0, register.IST)), "Thursday, 3 September · 2:18 pm"},
		{"evening", longstamp(time.Date(2026, time.September, 3, 18, 5, 0, 0, register.IST)), "Thursday, 3 September · 6:05 pm"},
		{"clock", clock(time.Date(2026, time.September, 3, 9, 40, 0, 0, register.IST)), "9:40 am"},
		{"shortdate", shortdate(time.Date(2026, time.September, 3, 10, 42, 0, 0, register.IST)), "3 September"},
		{"shortdate just after midnight", shortdate(time.Date(2026, time.September, 4, 0, 5, 0, 0, register.IST)), "4 September"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestAllTemplatesParse(t *testing.T) {
	if _, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"); err != nil {
		t.Fatalf("the templates do not parse: %v", err)
	}
	for _, name := range []string{"layout.html", "shift.html", "picker.html", "productconfirm.html", "notfound.html", "stub.html"} {
		if templates.Lookup(name) == nil {
			t.Errorf("template %s is missing", name)
		}
	}
}

func TestStaticAssetsEmbedded(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), tenAM)

	resp, body := e.get("/static/app.css")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /static/app.css returned %d, want 200", resp.StatusCode)
	}
	if len(body) < 2000 {
		t.Errorf("app.css is %d bytes, which is too small to be the walkthrough's CSS", len(body))
	}
	if !strings.Contains(body, "prefers-color-scheme") {
		t.Error("app.css has lost its dark-mode block")
	}
	for _, unwanted := range []string{"http://", "https://"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("app.css fetches %s from the internet, which the laptop may not have", unwanted)
		}
	}
}
