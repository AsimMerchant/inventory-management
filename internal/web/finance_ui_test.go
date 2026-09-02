package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"storeregister/internal/register"
)

// financeNav is the protected navigation every financial page must carry.
var financeNav = []struct{ Label, Path string }{
	{"Financial ledger", "/finance"},
	{"Orders", "/finance/orders"},
	{"Transactions", "/finance/journal"},
	{"Stock returned or sold", "/finance/settlements"},
	{"Financial activity", "/finance/audit"},
}

var adminOnlyNav = []struct{ Label, Path string }{
	{"Authorized people", "/finance/accounts"},
	{"Reusable lists", "/finance/lists"},
}

// financeGET makes one authenticated request and hands back the response so
// headers can be inspected, not just the body.
func financeGET(t *testing.T, c *testClient, path string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("GET", c.e.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 1<<20)
	n, _ := resp.Body.Read(body)
	return resp, string(body[:n])
}

func TestFinancialRouteTableAndSecurityHeaders(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	// Every protected GET carries the whole header set.
	protected := []string{
		"/finance", "/finance/orders", "/finance/orders/new", "/finance/journal",
		"/finance/journal/print", "/finance/audit", "/finance/settlements",
		"/finance/obligations", "/finance/movements/new", "/finance/accounts",
		"/finance/lists", "/finance/supplier-returns/new", "/finance/sales/new",
		"/finance/api/values?kind=party",
	}
	wantHeaders := map[string]string{
		"Cache-Control":           "no-store",
		"Pragma":                  "no-cache",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
	}
	for _, path := range protected {
		resp, _ := financeGET(t, admin, path)
		if resp.StatusCode != 200 {
			t.Errorf("GET %s = %d", path, resp.StatusCode)
		}
		for name, want := range wantHeaders {
			if got := resp.Header.Get(name); got != want {
				t.Errorf("GET %s had %s = %q, want %q", path, name, got, want)
			}
		}
	}

	// The wrong method on a GET-only route is 405, not a silent redirect.
	for _, path := range []string{"/finance", "/finance/journal", "/finance/audit", "/finance/settlements"} {
		status, _ := admin.post(t, path, url.Values{"anything": {"yes"}})
		if status != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, status)
		}
	}

	// An unknown protected record is the shell 404, with no neighbouring data.
	for _, path := range []string{
		"/finance/orders/ORD-9999", "/finance/orders/ORD-9999/edit",
		"/finance/movements/MOV-9999/edit",
		"/finance/settlements/sale/SAL-9999/edit",
	} {
		resp, body := financeGET(t, admin, path)
		if resp.StatusCode != 404 {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
		if !strings.Contains(body, "That financial record was not found.") {
			t.Errorf("GET %s did not use the protected not-found wording", path)
		}
	}

	// No ordinary response gains protected navigation.
	for _, path := range []string{"/stock", "/out", "/inwards", "/suppliers", "/log", "/shift"} {
		_, body := e.get(path)
		for _, item := range financeNav {
			if item.Label == "Financial ledger" || item.Label == "Orders" {
				if strings.Contains(body, `href="`+item.Path+`"`) {
					t.Errorf("%s links to the protected %s", path, item.Path)
				}
			}
		}
		if !strings.Contains(body, "Authorized login") {
			t.Errorf("%s has no Authorized login link", path)
		}
	}
}

func TestEveryFinancialScreenHasProtectedNavigationAndLogout(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, adminID := financeAdmin(t, e)
	user, _ := financeUser(t, e, key, adminID, "Rohan Das", "9900134562", "rohan pass")

	screens := []string{
		"/finance", "/finance/orders", "/finance/orders/new", "/finance/journal",
		"/finance/audit", "/finance/settlements", "/finance/obligations",
		"/finance/movements/new", "/finance/supplier-returns/new",
		"/finance/sales/new", "/finance/password",
	}

	for who, c := range map[string]*testClient{"admin": admin, "user": user} {
		for _, path := range screens {
			status, body := c.get(t, path)
			if status != 200 {
				t.Errorf("%s got %d from %s", who, status, path)
				continue
			}
			for _, item := range financeNav {
				if !strings.Contains(body, `href="`+item.Path+`"`) {
					t.Errorf("%s: %s has no %s link", who, path, item.Label)
				}
				if !strings.Contains(body, ">"+item.Label+"<") {
					t.Errorf("%s: %s does not read %q", who, path, item.Label)
				}
			}
			// Logout is a POST form with the session token, never a link.
			if !strings.Contains(body, `<form method="post" action="/finance/logout"`) {
				t.Errorf("%s: %s has no logout form", who, path)
			}
			if strings.Contains(body, `<a href="/finance/logout"`) {
				t.Errorf("%s: %s makes logout a link", who, path)
			}
			if !strings.Contains(body, `name="csrf"`) {
				t.Errorf("%s: %s logout carries no token", who, path)
			}
			// The authenticated identity, never the on-duty inventory person.
			wantName := "Asha Mehta"
			wantRole := "Administrator"
			if who == "user" {
				wantName, wantRole = "Rohan Das", "Financial user"
			}
			if !strings.Contains(body, wantName) || !strings.Contains(body, wantRole) {
				t.Errorf("%s: %s does not show %s as %s", who, path, wantName, wantRole)
			}
			if strings.Contains(body, "Suresh Kumar · on duty") {
				t.Errorf("%s: %s shows the inventory person instead", who, path)
			}
			// Role-gated controls.
			for _, item := range adminOnlyNav {
				has := strings.Contains(body, `href="`+item.Path+`"`)
				if who == "admin" && !has {
					t.Errorf("admin: %s has no %s link", path, item.Label)
				}
				if who == "user" && has {
					t.Errorf("user: %s offers the admin %s link", path, item.Label)
				}
			}
		}
	}
}

func TestFinancialFormsWorkServerRenderedWithoutJavaScript(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")

	// Every form a person uses is a plain form, with a select or a hidden
	// value for anything that carries an internal id.
	pages := map[string][]string{
		"/finance/orders/new":           {`method="post"`, `name="productId"`, "<noscript>"},
		"/finance/movements/new":        {`method="post"`, `name="orderId"`, "<noscript>"},
		"/finance/supplier-returns/new": {`method="post"`, `name="productId"`, "<noscript>"},
		"/finance/sales/new":            {`method="post"`, `name="productId"`, "<noscript>"},
		"/finance/journal":              {`method="get"`, `name="fromTime"`},
		"/finance/lists":                {`method="post"`},
	}
	for path, wants := range pages {
		status, body := admin.get(t, path)
		if status != 200 {
			t.Fatalf("%s = %d", path, status)
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s has no %s", path, want)
			}
		}
	}

	// The whole run below is plain form posts and GET links: no script at all.
	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")

	orderID := saveOrder(t, e, admin, key, orderForm("Sharma Events", tables, "100", "rent"))
	moveID := saveMoney(t, e, admin, key, moneyForm("out", "5000", "Sharma Events", "Deposit", "Cash"))
	if status, _ := admin.post(t, "/finance/movements/"+moveID+"/edit",
		moneyForm("out", "5500", "Sharma Events", "Deposit", "Cash")); status != 303 {
		t.Error("a correction needed script")
	}
	if status, _ := admin.post(t, "/finance/supplier-returns/new",
		settleForm("Sharma Events", tables, 40, "2026-09-03T15:00")); status != 303 {
		t.Error("a supplier return needed script")
	}
	if status, _ := admin.post(t, "/finance/orders/"+orderID+"/cancel",
		url.Values{"reason": {"Not needed"}}); status != 303 {
		t.Error("a cancellation needed script")
	}
	// The date filter and the print view are plain GETs.
	for _, path := range []string{
		"/finance/journal?day=2026-09-03",
		"/finance/journal?fromTime=2026-09-03T10%3A00&toTime=2026-09-03T23%3A59",
		"/finance/journal/print?day=2026-09-03",
	} {
		if status, _ := admin.get(t, path); status != 200 {
			t.Errorf("%s = %d", path, status)
		}
	}

	// No internal id is ever typed from memory: every id in the markup is a
	// select option value or a hidden field created by the server.
	for _, path := range []string{"/finance/movements/new", "/finance/journal", "/finance/settlements"} {
		_, body := admin.get(t, path)
		for _, prefix := range []string{"ORD-", "MOV-", "SRN-", "SAL-", "PRD-", "PTY-"} {
			for _, line := range strings.Split(body, "\n") {
				if !strings.Contains(line, prefix) {
					continue
				}
				// Ids may appear as option values, hidden values or hrefs the
				// server generated. They may never be asked for in a text box.
				if strings.Contains(line, `type="text"`) && strings.Contains(line, prefix) {
					t.Errorf("%s asks for a %s id in a text box: %s", path, prefix, line)
				}
			}
		}
	}
}

func TestFinancialRefusalsPreserveNonSecretInputOnly(t *testing.T) {
	e := newTestServer(t, emptyStock(), orderNow)
	admin, key, _ := financeAdmin(t, e)
	tables := productIDNamed(t, e, "Round tables")
	receive(t, e, tables, 100, "rent", "Sharma Events", "2026-09-03")

	// A money row refused for a bad amount keeps everything else typed.
	bad := moneyForm("out", "not money", "Sharma Events", "Freight", "Cash")
	bad.Set("reference", "INV/88")
	bad.Set("remarks", "Paid at the gate")
	bad.Set("occurredAt", "2026-09-01T09:15")
	status, body := admin.post(t, "/finance/movements/new", bad)
	if status != 200 {
		t.Fatalf("a bad amount gave %d", status)
	}
	for _, want := range []string{"Sharma Events", "Freight", "INV/88", "Paid at the gate", "2026-09-01T09:15", "not money"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refused money form lost %q", want)
		}
	}
	// And created nothing.
	if err := e.st.ReadFinance(key, func(f *register.FinanceData) {
		if len(f.Movements) != 0 {
			t.Error("a refused row was written")
		}
		if len(register.LiveFinanceValues(f, register.FinanceParty)) != 0 {
			t.Error("a refused row left an orphan party")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// A supplier return refused for too many keeps the typed values.
	tooMuch := settleForm("Sharma Events", tables, 500, "2026-09-03T15:00")
	tooMuch.Set("reference", "GATE/12")
	tooMuch.Set("remarks", "Loaded on their truck")
	status, body = admin.post(t, "/finance/supplier-returns/new", tooMuch)
	if status != 200 {
		t.Fatalf("too many gave %d", status)
	}
	for _, want := range []string{"Sharma Events", "500", "GATE/12", "Loaded on their truck"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refused return form lost %q", want)
		}
	}

	// Secrets are never handed back. A failed login blanks the password.
	c := financeClient(t)
	_, loginBody := financeRequest(t, c, "GET", e.URL+"/finance/login", nil)
	csrf := csrfFrom(t, loginBody)
	_, body = financeRequest(t, c, "POST", e.URL+"/finance/login", url.Values{
		"csrf": {csrf}, "mobile": {"9886140023"}, "password": {"wrong password here"},
	})
	if strings.Contains(body, "wrong password here") {
		t.Error("a failed login handed the password back")
	}
	if !strings.Contains(body, `type="password"`) {
		t.Error("the login form has no password field")
	}
	// And the setup form never echoes what was typed into a secret field.
	fresh := newTestServer(t, nil, orderNow)
	c2 := financeClient(t)
	_, setupBody := financeRequest(t, c2, "GET", fresh.URL+"/finance/setup", nil)
	csrf2 := csrfFrom(t, setupBody)
	_, body = financeRequest(t, c2, "POST", fresh.URL+"/finance/setup", url.Values{
		"csrf": {csrf2}, "name": {"Asha Mehta"}, "mobile": {"98861 40023"},
		"password": {"short"}, "again": {"different one"},
	})
	for _, secret := range []string{"short", "different one"} {
		if strings.Contains(body, secret) {
			t.Errorf("the setup form handed back %q", secret)
		}
	}
}

func TestOrdinaryInventoryExperienceRemainsUnchanged(t *testing.T) {
	pages := []string{"/stock", "/out", "/inwards", "/suppliers", "/log?day=all",
		"/inward/new", "/issue/new", "/return/new", "/shift"}

	// The same register, once with no vault at all and once with a populated
	// one that nobody has opened.
	plain := newTestServer(t, register.WalkthroughT0(), orderNow)
	before := map[string]string{}
	for _, path := range pages {
		_, body := plain.get(path)
		before[path] = body
	}

	withVault := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, key, _ := financeAdmin(t, withVault)
	chairs := productIDNamed(t, withVault, "Chairs")
	saveOrder(t, withVault, admin, key, orderForm("Sharma Events", chairs, "100", "rent"))
	saveMoney(t, withVault, admin, key, moneyForm("out", "5000", "Sharma Events", "Deposit", "Cash"))

	for _, path := range pages {
		_, body := withVault.get(path)
		if body != before[path] {
			t.Errorf("%s changed once a vault existed:\n--- before ---\n%s\n--- after ---\n%s",
				path, before[path], body)
		}
	}
}

func TestFinanceHeadersAndFormsContainNoExternalResource(t *testing.T) {
	e := newTestServer(t, register.WalkthroughT0(), orderNow)
	admin, _, _ := financeAdmin(t, e)

	pages := []string{
		"/finance", "/finance/orders/new", "/finance/movements/new",
		"/finance/journal", "/finance/journal/print", "/finance/settlements",
		"/finance/supplier-returns/new", "/finance/sales/new", "/finance/lists",
		"/finance/accounts", "/finance/audit", "/finance/obligations",
	}
	for _, path := range pages {
		resp, body := financeGET(t, admin, path)
		if resp.StatusCode != 200 {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
		// Nothing off this machine, and no inline handler.
		for _, forbidden := range []string{"http://", "https://", "//cdn", "onclick=", "onload=", "javascript:"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains %q", path, forbidden)
			}
		}
		// A password box is never remembered by the browser.
		if strings.Contains(body, `type="password"`) && !strings.Contains(body, `autocomplete`) {
			t.Errorf("%s has a password field with no autocomplete rule", path)
		}
		// Protected pages are never cached.
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s is cacheable", path)
		}
	}

	// No amount, party or remark travels in a URL. Only journal filters and
	// opaque record ids do.
	_, journal := financeGET(t, admin, "/finance/journal")
	for _, line := range strings.Split(journal, "\n") {
		if !strings.Contains(line, "href=") {
			continue
		}
		for _, forbidden := range []string{"amount=", "party=", "remarks=", "purpose=", "password="} {
			if strings.Contains(line, forbidden) {
				t.Errorf("a link carries %s: %s", forbidden, line)
			}
		}
	}

	// Every mutation is a POST. No GET route writes.
	before := mustReadFile(t, e.path)
	for _, path := range []string{
		"/finance/movements/new", "/finance/orders/new", "/finance/supplier-returns/new",
		"/finance/sales/new", "/finance/journal", "/finance/settlements", "/finance/lists",
	} {
		financeGET(t, admin, path)
	}
	if string(mustReadFile(t, e.path)) != string(before) {
		t.Error("a GET changed the register file")
	}
}
