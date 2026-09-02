package web

import (
	"bytes"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

var financeNow = time.Date(2026, 9, 2, 12, 0, 0, 0, register.IST)

var hiddenCSRF = regexp.MustCompile(`name="csrf" value="([^"]+)"`)
var recoveryShown = regexp.MustCompile(`<pre>([^<]+)</pre>`)

func financeClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func financeRequest(t *testing.T, c *http.Client, method, target string, form url.Values) (*http.Response, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	m := hiddenCSRF.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatalf("no csrf in %q", body)
	}
	return html.UnescapeString(m[1])
}

func setupFinanceWeb(t *testing.T, e *env, c *http.Client) (string, string) {
	t.Helper()
	_, body := financeRequest(t, c, "GET", e.URL+"/finance/setup", nil)
	csrf := csrfFrom(t, body)
	resp, body := financeRequest(t, c, "POST", e.URL+"/finance/setup", url.Values{"csrf": {csrf}, "name": {"Asha Mehta"}, "mobile": {"98861 40023"}, "password": {"correct horse"}, "again": {"correct horse"}})
	if resp.StatusCode != 200 {
		t.Fatalf("setup status=%d body=%s", resp.StatusCode, body)
	}
	m := recoveryShown.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("recovery not shown")
	}
	return html.UnescapeString(m[1]), csrfFrom(t, body)
}

func TestFirstAccountIsAdministratorAndRecoveryShownOnce(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	c := financeClient(t)
	recovery, csrf := setupFinanceWeb(t, e, c)
	if recovery == "" {
		t.Fatal("empty recovery")
	}
	raw := string(mustReadFile(t, e.path))
	if strings.Contains(raw, "Asha") || strings.Contains(raw, "98861") {
		t.Fatal("plaintext financial identity in file")
	}
	var key []byte
	var id string
	var err error
	key, id, err = e.st.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) {
		if id != "FAC-0001" || f.Accounts[0].Role != register.FinanceAdmin || f.Accounts[0].Status != "active" {
			t.Errorf("account=%+v", f.Accounts[0])
		}
	})
	resp, _ := financeRequest(t, c, "POST", e.URL+"/finance/setup/confirm", url.Values{"csrf": {csrf}, "saved": {"yes"}})
	if resp.StatusCode != 303 {
		t.Fatalf("confirm=%d", resp.StatusCode)
	}
	_, body := financeRequest(t, c, "GET", e.URL+"/finance", nil)
	if strings.Contains(body, recovery) {
		t.Fatal("recovery shown again")
	}
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) {
		if f.RecoveryConfirmedAt == nil {
			t.Error("confirmation not saved")
		}
	})
}

func TestLostRecoveryConfirmationRotatesBeforeLedgerAccess(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	first := financeClient(t)
	old, _ := setupFinanceWeb(t, e, first)
	c := financeClient(t)
	_, body := financeRequest(t, c, "GET", e.URL+"/finance/login", nil)
	csrf := csrfFrom(t, body)
	resp, _ := financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{"csrf": {csrf}, "mobile": {"9886140023"}, "password": {"correct horse"}})
	if resp.Header.Get("Location") != "/finance/recovery-key/new" {
		t.Fatalf("location=%q", resp.Header.Get("Location"))
	}
	_, body = financeRequest(t, c, "GET", e.URL+"/finance/recovery-key/new", nil)
	csrf = csrfFrom(t, body)
	_, body = financeRequest(t, c, "POST", e.URL+"/finance/recovery-key/new", url.Values{"csrf": {csrf}})
	csrf = csrfFrom(t, body)
	_, body = financeRequest(t, c, "POST", e.URL+"/finance/recovery-key/new", url.Values{"csrf": {csrf}, "confirm": {"yes"}})
	m := recoveryShown.FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("replacement recovery not shown")
	}
	if m[1] == old {
		t.Fatal("recovery was not rotated")
	}
	if _, err := e.st.UnlockFinanceRecovery(old); err == nil {
		t.Fatal("abandoned recovery still works")
	}
}

func TestAdminAuthorizesAndPersonChoosesPassword(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	c := financeClient(t)
	_, csrf := setupFinanceWeb(t, e, c)
	financeRequest(t, c, "POST", e.URL+"/finance/setup/confirm", url.Values{"csrf": {csrf}, "saved": {"yes"}})
	_, body := financeRequest(t, c, "GET", e.URL+"/finance/accounts", nil)
	csrf = csrfFrom(t, body)
	_, body = financeRequest(t, c, "POST", e.URL+"/finance/accounts/new", url.Values{"csrf": {csrf}, "name": {"Rohan Das"}, "mobile": {"99001 34562"}, "role": {"user"}})
	m := regexp.MustCompile(`<pre>([^<]+)</pre>`).FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("setup code not shown")
	}
	r := financeClient(t)
	_, body = financeRequest(t, r, "GET", e.URL+"/finance/activate", nil)
	csrf = csrfFrom(t, body)
	resp, _ := financeRequest(t, r, "POST", e.URL+"/finance/activate", url.Values{"csrf": {csrf}, "mobile": {"9900134562"}, "code": {m[1]}, "password": {"rohan pass"}, "again": {"rohan pass"}})
	if resp.StatusCode != 303 {
		t.Fatalf("activate=%d", resp.StatusCode)
	}
	if _, _, err := e.st.UnlockFinance("9900134562", "correct horse"); err == nil {
		t.Fatal("admin password unlocked Rohan")
	}
}

func TestExpiredOrReplacedSetupCodeCannotActivate(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	key, admin, _, err := e.st.InitializeFinance("Asha", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	id, old, err := e.st.AuthorizeFinanceAccount(key, admin, "Rohan", "9900134562", register.FinanceUser, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.ActivateFinance("9900134562", old, "rohan pass", financeNow.Add(24*time.Hour)); err == nil {
		t.Fatal("code accepted at expiry")
	}
	newCode, err := e.st.ResetFinanceAccount(key, admin, id, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.ActivateFinance("9900134562", old, "rohan pass", financeNow); err == nil {
		t.Fatal("replaced code accepted")
	}
	if _, _, err := e.st.ActivateFinance("9900134562", newCode, "rohan pass", financeNow); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.ActivateFinance("9900134562", newCode, "another pass", financeNow); err == nil {
		t.Fatal("consumed code accepted")
	}
}

func TestAdminResetAndOfflineRecovery(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	key, admin, recovery, err := e.st.InitializeFinance("Asha", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	second, code, err := e.st.AuthorizeFinanceAccount(key, admin, "Meera", "9000011111", register.FinanceAdmin, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.ActivateFinance("9000011111", code, "meera pass", financeNow); err != nil {
		t.Fatal(err)
	}
	reset, err := e.st.ResetFinanceAccount(key, admin, second, financeNow)
	if err != nil || reset == "" {
		t.Fatalf("reset=%q err=%v", reset, err)
	}
	if _, _, err := e.st.UnlockFinance("9000011111", "meera pass"); err == nil {
		t.Fatal("old password survived reset")
	}
	newKey, id, err := e.st.RecoverFinanceAdministrator(recovery, "9886140023", "recovered pass", financeNow)
	if err != nil || id != admin {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, _, err := e.st.UnlockFinance("9886140023", "correct horse"); err == nil {
		t.Fatal("old admin password survived recovery")
	}
	if err := e.st.ReadFinance(newKey, func(f *register.FinanceData) {
		if len(f.Accounts) != 2 {
			t.Errorf("accounts=%d", len(f.Accounts))
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountDestructiveActionsRequireImpactConfirmation(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	admin, key, adminID := financeAdmin(t, e)
	_, userID := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")
	before := mustReadFile(t, e.path)
	status, body := admin.post(t, "/finance/accounts/"+userID+"/reset", nil)
	if status != 200 || !strings.Contains(body, "Reset password access for Rohan Das?") ||
		!strings.Contains(body, "Yes, reset password access") {
		t.Fatalf("reset confirmation = %d: %s", status, body)
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("first reset step wrote the store")
	}
	if status, body = admin.post(t, "/finance/accounts/"+userID+"/reset", url.Values{"confirm": {"yes"}}); status != 200 ||
		!strings.Contains(body, "Give this setup code to Rohan Das") || !strings.Contains(body, "expires in 24 hours") {
		t.Fatalf("confirmed reset = %d: %s", status, body)
	}

	before = mustReadFile(t, e.path)
	status, body = admin.post(t, "/finance/accounts/"+userID+"/disable", nil)
	if status != 200 || !strings.Contains(body, "Disable authorized access for Rohan Das?") ||
		!strings.Contains(body, "Yes, disable authorized access") {
		t.Fatalf("disable confirmation = %d: %s", status, body)
	}
	if !bytes.Equal(before, mustReadFile(t, e.path)) {
		t.Fatal("first disable step wrote the store")
	}
	if status, body = admin.post(t, "/finance/accounts/"+userID+"/disable", url.Values{"confirm": {"yes"}}); status != 303 {
		t.Fatalf("confirmed disable = %d: %s", status, body)
	}
}

func TestNewPasswordMinimumIsPlainAndSecretsCleared(t *testing.T) {
	fresh := newTestServer(t, nil, financeNow)
	setupClient := financeClient(t)
	_, setupPage := financeRequest(t, setupClient, "GET", fresh.URL+"/finance/setup", nil)
	_, setupBody := financeRequest(t, setupClient, "POST", fresh.URL+"/finance/setup", url.Values{
		"csrf": {csrfFrom(t, setupPage)}, "name": {"Asha Mehta"}, "mobile": {"9886140023"}, "password": {"short"}, "again": {"short"},
	})
	if !strings.Contains(setupBody, "Password must be at least 8 characters.") || !strings.Contains(setupBody, `value="Asha Mehta"`) || !strings.Contains(setupBody, `value="9886140023"`) || strings.Contains(setupBody, `value="short"`) {
		t.Fatal("setup minimum/refusal did not preserve only name and mobile")
	}

	e := newTestServer(t, nil, financeNow)
	admin, key, adminID := financeAdmin(t, e)
	_, code, err := e.st.AuthorizeFinanceAccount(key, adminID, "Rohan Das", "9900134562", register.FinanceUser, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	public := financeClient(t)
	_, page := financeRequest(t, public, "GET", e.URL+"/finance/activate", nil)
	_, body := financeRequest(t, public, "POST", e.URL+"/finance/activate", url.Values{
		"csrf": {csrfFrom(t, page)}, "mobile": {"9900134562"}, "code": {code}, "password": {"short"}, "again": {"short"},
	})
	if !strings.Contains(body, "Password must be at least 8 characters.") || !strings.Contains(body, `value="9900134562"`) || strings.Contains(body, code) || strings.Contains(body, `value="short"`) {
		t.Fatal("activation minimum/refusal did not preserve only mobile")
	}
	_, page = financeRequest(t, public, "GET", e.URL+"/finance/recover", nil)
	_, body = financeRequest(t, public, "POST", e.URL+"/finance/recover", url.Values{
		"csrf": {csrfFrom(t, page)}, "mobile": {"9886140023"}, "recovery": {"secret-recovery"}, "password": {"short"}, "again": {"short"},
	})
	if !strings.Contains(body, "Password must be at least 8 characters.") || !strings.Contains(body, `value="9886140023"`) || strings.Contains(body, "secret-recovery") || strings.Contains(body, `value="short"`) {
		t.Fatal("recovery minimum/refusal did not preserve only mobile")
	}
	_, body = admin.post(t, "/finance/password", url.Values{"current": {"correct horse"}, "password": {"short"}, "again": {"short"}})
	if !strings.Contains(body, "Password must be at least 8 characters.") || strings.Contains(body, `value="correct horse"`) || strings.Contains(body, `value="short"`) {
		t.Fatal("password-change minimum leaked a secret")
	}
}

func TestAccountCorrectionAndOwnPasswordChangeAreAtomic(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	key, admin, _, err := e.st.InitializeFinance("Asha", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	id, code, err := e.st.AuthorizeFinanceAccount(key, admin, "Rohan Dsa", "99001 34562", register.FinanceUser, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	rohanKey, _, err := e.st.ActivateFinance("9900134562", code, "rohan pass", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.EditFinanceAccount(key, admin, id, "Rohan Das", "99001 34563", register.FinanceAdmin, financeNow); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.UnlockFinance("9900134562", "rohan pass"); err == nil {
		t.Fatal("old mobile still logs in")
	}
	if _, _, err := e.st.UnlockFinance("9900134563", "rohan pass"); err != nil {
		t.Fatal(err)
	}
	if err := e.st.ChangeFinancePassword(rohanKey, id, "rohan pass", "brand new pass", financeNow); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.UnlockFinance("9900134563", "rohan pass"); err == nil {
		t.Fatal("old password still logs in")
	}
	if _, _, err := e.st.UnlockFinance("9900134563", "brand new pass"); err != nil {
		t.Fatal(err)
	}
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) {
		joined := ""
		for _, a := range f.Audit {
			joined += a.Before + a.After
		}
		if strings.Contains(joined, "rohan pass") || !strings.Contains(joined, "Rohan Dsa") || !strings.Contains(joined, "Rohan Das") {
			t.Errorf("audit=%q", joined)
		}
	})
}

func TestCannotDisableLastAdministrator(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	key, id, _, err := e.st.InitializeFinance("Asha", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	err = e.st.DisableFinanceAccount(key, id, id, financeNow)
	if err == nil || err.Error() != "Keep at least one financial administrator active." {
		t.Fatalf("err=%v", err)
	}
}

func TestFinanceAuthorizationIsServerEnforced(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	c := financeClient(t)
	resp, body := financeRequest(t, c, "GET", e.URL+"/finance", nil)
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/finance/login" {
		t.Fatalf("GET status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, body = financeRequest(t, c, "POST", e.URL+"/finance/accounts/new", url.Values{"name": {"Bad"}})
	if resp.StatusCode != 403 || body != "" {
		t.Fatalf("POST status=%d body=%q", resp.StatusCode, body)
	}
}

func TestFinanceSessionCookieLogoutAndIdleExpiry(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	recorder := httptest.NewRecorder()
	if _, err := e.srv.newFinanceSession(recorder, make([]byte, 32), "FAC-test", false); err != nil {
		t.Fatal(err)
	}
	set := recorder.Result().Cookies()
	if len(set) != 1 || set[0].Name != financeCookie || !set[0].HttpOnly || set[0].SameSite != http.SameSiteStrictMode || set[0].Path != "/" || set[0].Secure {
		t.Fatalf("session cookie flags=%+v", set)
	}
	c := financeClient(t)
	_, csrf := setupFinanceWeb(t, e, c)
	var cookie *http.Cookie
	for _, v := range c.Jar.Cookies(mustURL(t, e.URL)) {
		if v.Name == financeCookie {
			cookie = v
		}
	}
	if cookie == nil {
		t.Fatal("session cookie not set")
	}
	financeRequest(t, c, "POST", e.URL+"/finance/setup/confirm", url.Values{"csrf": {csrf}, "saved": {"yes"}})
	e.srv.now = func() time.Time { return financeNow.Add(14*time.Minute + 59*time.Second) }
	resp, _ := financeRequest(t, c, "GET", e.URL+"/finance", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("14:59 status=%d", resp.StatusCode)
	}
	e.srv.now = func() time.Time { return financeNow.Add(29*time.Minute + 59*time.Second) }
	resp, _ = financeRequest(t, c, "GET", e.URL+"/finance", nil)
	if resp.Header.Get("Location") != "/finance/login?expired=1" {
		t.Fatalf("status=%d location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, body := financeRequest(t, c, "GET", e.URL+"/finance/login?expired=1", nil)
	assertContains(t, body, "You were logged out because the computer was left idle.")
	loginCSRF := csrfFrom(t, body)
	resp, _ = financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{"csrf": {loginCSRF}, "mobile": {"9886140023"}, "password": {"correct horse"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status=%d", resp.StatusCode)
	}
	_, body = financeRequest(t, c, "GET", e.URL+"/finance", nil)
	logoutCSRF := csrfFrom(t, body)
	resp, _ = financeRequest(t, c, "POST", e.URL+"/finance/logout", url.Values{"csrf": {logoutCSRF}})
	if resp.Header.Get("Location") != "/stock" {
		t.Fatalf("logout location=%q", resp.Header.Get("Location"))
	}
	resp, _ = financeRequest(t, c, "GET", e.URL+"/finance", nil)
	if resp.Header.Get("Location") != "/finance/login" {
		t.Fatalf("session survived logout")
	}
}

func TestFinanceCSRFAndThrottle(t *testing.T) {
	e := newTestServer(t, nil, financeNow)
	key, _, _, err := e.st.InitializeFinance("Asha", "9886140023", "correct horse", financeNow)
	_ = key
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.ConfirmFinanceRecovery(key, "FAC-0001", financeNow); err != nil {
		t.Fatal(err)
	}
	auth := financeClient(t)
	_, body := financeRequest(t, auth, "GET", e.URL+"/finance/login", nil)
	csrf := csrfFrom(t, body)
	financeRequest(t, auth, "POST", e.URL+"/finance/login", url.Values{"csrf": {csrf}, "mobile": {"9886140023"}, "password": {"correct horse"}})
	resp, body := financeRequest(t, auth, "POST", e.URL+"/finance/accounts/new", url.Values{"name": {"Should not save"}, "mobile": {"9000000000"}, "role": {"user"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d", resp.StatusCode)
	}
	assertContains(t, body, "This page expired. Go back and try again.")
	resp, _ = financeRequest(t, auth, "POST", e.URL+"/finance/accounts/new", url.Values{"csrf": {"wrong"}, "name": {"Should not save"}, "mobile": {"9000000000"}, "role": {"user"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong csrf status=%d", resp.StatusCode)
	}
	_ = e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Accounts) != 1 {
			t.Fatal("CSRF refusal wrote an account")
		}
	})
	c := financeClient(t)
	for i := 0; i < 5; i++ {
		_, body := financeRequest(t, c, "GET", e.URL+"/finance/login", nil)
		csrf := csrfFrom(t, body)
		financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{"csrf": {csrf}, "mobile": {"9886140023"}, "password": {"wrong pass"}})
	}
	_, body = financeRequest(t, c, "GET", e.URL+"/finance/login", nil)
	csrf = csrfFrom(t, body)
	_, body = financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{"csrf": {csrf}, "mobile": {"9886140023"}, "password": {"correct horse"}})
	assertContains(t, body, "Too many attempts. Wait 15 minutes and try again.")
}

func TestPublicPagesLeakNoFinanceContent(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), twoEighteen)
	key, _, _, err := e.st.InitializeFinance("Secret Asha", "9886140023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		f.Audit = append(f.Audit, register.FinanceAuditEvent{ID: f.NextID("FAE"), At: financeNow, ByAccountID: "FAC-0001", ByName: "Secret Asha", ByMobile: "9886140023", Kind: "account_edited", EntityType: "account", EntityID: "FAC-0001", Summary: "Protected marker Z9Q"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	paths := []string{"/shift", "/stock", "/out", "/inwards", "/suppliers", "/log", "/inward/new", "/issue/new", "/return/new", "/product/PRD-0001/edit", "/api/products", "/api/people", "/finance/login", "/finance/activate", "/finance/recover"}
	for _, path := range paths {
		_, body := e.get(path)
		assertNotContains(t, body, "Secret Asha")
		assertNotContains(t, body, "9886140023")
		assertNotContains(t, body, "Protected marker Z9Q")
		assertNotContains(t, body, "Financial ledger")
		if !strings.HasPrefix(path, "/api/") {
			assertContains(t, body, "Authorized login")
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
