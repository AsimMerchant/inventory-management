package web

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// The fixture is built in IST and the stale-shift rule compares calendar days in
// the server's local timezone. Pinning time.Local to the fixture's own zone is
// what makes "the same day" mean the same thing on any developer machine.
func TestMain(m *testing.M) {
	time.Local = register.IST
	os.Exit(m.Run())
}

// The walkthrough's three clocks, in the fixture's own zone.
var (
	twoEighteen = time.Date(2026, time.September, 3, 14, 18, 0, 0, register.IST)
	sixOhFive   = time.Date(2026, time.September, 3, 18, 5, 0, 0, register.IST)

	// The two clocks the correction spec is written against: when a wrong
	// entry is fixed, and when one is deleted.
	tenFortyFive  = time.Date(2026, time.September, 3, 10, 45, 0, 0, register.IST)
	tenFortySeven = time.Date(2026, time.September, 3, 10, 47, 0, 0, register.IST)
)

// env is one running register: a server, its store and the file underneath it.
type env struct {
	*httptest.Server
	path string
	st   *store.Store
	srv  *Server
	t    *testing.T
}

// newTestServer starts a server over reg, saved into a fresh directory. A nil
// reg is a fresh install: no file on disk, no staff, no products.
func newTestServer(t *testing.T, reg *register.Register, now time.Time) *env {
	t.Helper()
	return newTestServerWith(t, reg, now, "")
}

// newTestServerWith is newTestServer plus the recovery warning that store.Open
// would have reported.
func newTestServerWith(t *testing.T, reg *register.Register, now time.Time, warning string) *env {
	t.Helper()

	path := filepath.Join(t.TempDir(), store.FileName)
	if reg != nil {
		data, err := json.MarshalIndent(reg, "", "  ")
		if err != nil {
			t.Fatalf("marshalling the fixture: %v", err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}

	st, _, err := store.Open(path)
	if err != nil {
		t.Fatalf("opening the register: %v", err)
	}

	srv := NewServer(st, warning, func() time.Time { return now })
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &env{Server: ts, path: path, st: st, srv: srv, t: t}
}

// client follows nothing: a 303 is a fact under test, not a detour.
func (e *env) client() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// get fetches a path and returns the response and its body.
func (e *env) get(path string) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client().Get(e.URL + path)
	if err != nil {
		e.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("reading GET %s: %v", path, err)
	}
	return resp, string(body)
}

// post submits a form and returns the response and its body.
func (e *env) post(path string, form url.Values) (*http.Response, string) {
	e.t.Helper()
	resp, err := e.client().PostForm(e.URL+path, form)
	if err != nil {
		e.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("reading POST %s: %v", path, err)
	}
	return resp, string(body)
}

// saved reads the register back off disk, which is the only copy that matters.
func (e *env) saved() *register.Register {
	e.t.Helper()
	data, err := os.ReadFile(e.path)
	if err != nil {
		e.t.Fatalf("reading the saved register: %v", err)
	}
	var reg register.Register
	if err := json.Unmarshal(data, &reg); err != nil {
		e.t.Fatalf("parsing the saved register: %v", err)
	}
	return &reg
}

// assertContains compares against the unescaped body. html/template renders an
// apostrophe as &#39; and an ampersand as &amp;, so "Joseph D'Cruz" and
// "don't" never appear literally in a response.
func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(html.UnescapeString(body), want) {
		t.Errorf("the page does not contain %q", want)
	}
}

// assertNotContains is its companion, so no test reaches past the unescape.
func assertNotContains(t *testing.T, body, unwanted string) {
	t.Helper()
	if strings.Contains(html.UnescapeString(body), unwanted) {
		t.Errorf("the page contains %q and should not", unwanted)
	}
}
