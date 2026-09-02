package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

const financeCookie = "store_finance_session"
const preauthCookie = "store_finance_preauth"

type financeSession struct {
	vaultKey        []byte
	accountID       string
	createdAt       time.Time
	lastActivity    time.Time
	csrf            string
	recoveryPending bool
}

type financeContextKey struct{}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) financeRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /finance/setup", s.financeSetup)
	m.HandleFunc("POST /finance/setup", s.financeSetup)
	m.HandleFunc("POST /finance/setup/confirm", s.financeSetupConfirm)
	m.HandleFunc("GET /finance/login", s.financeLogin)
	m.HandleFunc("POST /finance/login", s.financeLogin)
	m.HandleFunc("POST /finance/logout", s.financeLogout)
	m.HandleFunc("GET /finance/activate", s.financeActivate)
	m.HandleFunc("POST /finance/activate", s.financeActivate)
	m.HandleFunc("GET /finance/recover", s.financeRecover)
	m.HandleFunc("POST /finance/recover", s.financeRecover)
	m.HandleFunc("GET /finance", s.financeDashboard)
	m.HandleFunc("GET /finance/accounts", s.financeAccounts)
	m.HandleFunc("POST /finance/accounts/new", s.financeAccountNew)
	m.HandleFunc("POST /finance/accounts/{id}/disable", s.financeAccountDisable)
	m.HandleFunc("POST /finance/accounts/{id}/reset", s.financeAccountReset)
	m.HandleFunc("GET /finance/accounts/{id}/edit", s.financeAccountEdit)
	m.HandleFunc("POST /finance/accounts/{id}/edit", s.financeAccountEdit)
	m.HandleFunc("GET /finance/password", s.financePassword)
	m.HandleFunc("POST /finance/password", s.financePassword)
	m.HandleFunc("GET /finance/recovery-key/new", s.financeRecoveryNew)
	m.HandleFunc("POST /finance/recovery-key/new", s.financeRecoveryNew)

	// 18
	m.HandleFunc("GET /finance/api/values", s.financeAPIValues)
	m.HandleFunc("GET /finance/lists", s.financeLists)
	m.HandleFunc("POST /finance/lists/{id}/{action}", s.financeListAction)
	m.HandleFunc("GET /finance/orders", s.financeOrders)
	m.HandleFunc("GET /finance/orders/new", s.financeOrderNew)
	m.HandleFunc("POST /finance/orders/new", s.financeOrderNew)
	m.HandleFunc("GET /finance/orders/{id}", s.financeOrderDetail)
	m.HandleFunc("GET /finance/orders/{id}/edit", s.financeOrderEdit)
	m.HandleFunc("POST /finance/orders/{id}/edit", s.financeOrderEdit)
	m.HandleFunc("POST /finance/orders/{id}/cancel", s.financeOrderCancel)
	m.HandleFunc("POST /finance/product/new", s.financeProductNew)

	// 19
	m.HandleFunc("GET /finance/movements/new", s.financeMoneyNew)
	m.HandleFunc("POST /finance/movements/new", s.financeMoneyNew)
	m.HandleFunc("GET /finance/movements/{id}/edit", s.financeMoneyEdit)
	m.HandleFunc("POST /finance/movements/{id}/edit", s.financeMoneyEdit)
	m.HandleFunc("POST /finance/movements/{id}/void", s.financeMoneyVoid)
	m.HandleFunc("GET /finance/journal", s.financeJournal)
	m.HandleFunc("GET /finance/journal/print", s.financeJournalPrint)
	m.HandleFunc("GET /finance/audit", s.financeAudit)
	m.HandleFunc("GET /finance/supplier-returns/new", s.financeSupplierReturnNew)
	m.HandleFunc("GET /finance/sales/new", s.financeSaleNew)
	m.HandleFunc("GET /finance/settlements", s.financeSettlements)
}

func financeHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func publicFinancePath(path string) bool {
	switch path {
	case "/finance/login", "/finance/setup", "/finance/activate", "/finance/recover":
		return true
	}
	return false
}

func (s *Server) serveFinance(w http.ResponseWriter, r *http.Request) {
	financeHeaders(w)
	if publicFinancePath(r.URL.Path) {
		s.mux.ServeHTTP(w, r)
		return
	}
	session, expired := s.authenticatedFinanceSession(r)
	if session == nil {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			target := "/finance/login"
			if expired {
				target += "?expired=1"
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
		return
	}
	if session.recoveryPending && r.URL.Path != "/finance/setup/confirm" && r.URL.Path != "/finance/logout" && r.URL.Path != "/finance/recovery-key/new" {
		http.Redirect(w, r, "/finance/recovery-key/new", http.StatusSeeOther)
		return
	}
	if r.URL.Path == "/finance/setup/confirm" && !session.recoveryPending {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost && !constantEqual(session.csrf, r.FormValue("csrf")) {
		http.Error(w, "This page expired. Go back and try again.", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/finance/accounts") && !s.sessionIsAdmin(session) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	// Anybody authorized may add a value while recording something. Correcting
	// the shared lists afterwards is an administrator's job.
	if strings.HasPrefix(r.URL.Path, "/finance/lists") && !s.sessionIsAdmin(session) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.Path == "/finance/recovery-key/new" && !s.sessionIsAdmin(session) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), financeContextKey{}, session))
	s.mux.ServeHTTP(w, r)
}

func constantEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) authenticatedFinanceSession(r *http.Request) (*financeSession, bool) {
	cookie, err := r.Cookie(financeCookie)
	if err != nil {
		return nil, false
	}
	now := s.now()
	s.financeMu.Lock()
	defer s.financeMu.Unlock()
	sess := s.financeSessions[cookie.Value]
	if sess == nil {
		return nil, false
	}
	if now.Sub(sess.lastActivity) >= 15*time.Minute {
		delete(s.financeSessions, cookie.Value)
		return nil, true
	}
	// Recheck active status on every request so disable takes effect immediately.
	active := false
	if err := s.st.ReadFinance(sess.vaultKey, func(data *register.FinanceData) {
		for _, a := range data.Accounts {
			if a.ID == sess.accountID && a.Status == "active" {
				active = true
			}
		}
	}); err != nil || !active {
		delete(s.financeSessions, cookie.Value)
		return nil, false
	}
	sess.lastActivity = now
	return sess, false
}

func (s *Server) sessionIsAdmin(sess *financeSession) bool {
	admin := false
	_ = s.st.ReadFinance(sess.vaultKey, func(data *register.FinanceData) {
		for _, a := range data.Accounts {
			if a.ID == sess.accountID && a.Status == "active" && a.Role == register.FinanceAdmin {
				admin = true
			}
		}
	})
	return admin
}

func (s *Server) newFinanceSession(w http.ResponseWriter, key []byte, accountID string, pending bool) (*financeSession, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sess := &financeSession{vaultKey: append([]byte(nil), key...), accountID: accountID, createdAt: now, lastActivity: now, csrf: csrf, recoveryPending: pending}
	s.financeMu.Lock()
	s.financeSessions[token] = sess
	s.financeMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: financeCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return sess, nil
}

func (s *Server) preauthCSRF(w http.ResponseWriter, r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(preauthCookie); err == nil {
		s.financeMu.Lock()
		token := s.preauth[cookie.Value]
		s.financeMu.Unlock()
		if token != "" {
			return token, true
		}
	}
	id, err := randomToken()
	if err != nil {
		return "", false
	}
	token, err := randomToken()
	if err != nil {
		return "", false
	}
	s.financeMu.Lock()
	s.preauth[id] = token
	s.financeMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: preauthCookie, Value: id, Path: "/finance", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return token, true
}

func (s *Server) verifyPreauth(r *http.Request) bool {
	cookie, err := r.Cookie(preauthCookie)
	if err != nil {
		return false
	}
	s.financeMu.Lock()
	token := s.preauth[cookie.Value]
	s.financeMu.Unlock()
	return constantEqual(token, r.FormValue("csrf"))
}

func (s *Server) publicFinancePage(w http.ResponseWriter, name string, data any) {
	s.render(w, http.StatusOK, page{Title: "Authorized login", Narrow: true}, name, data)
}

func (s *Server) financePage(w http.ResponseWriter, r *http.Request, title, name string, data any) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	p := page{Title: title, Tabs: false, Finance: true, FinanceAdmin: s.sessionIsAdmin(sess), CSRF: sess.csrf}
	s.render(w, http.StatusOK, p, name, data)
}

type authFormData struct {
	CSRF, Error       string
	Expired, CanSetup bool
}

func (s *Server) financeLogin(w http.ResponseWriter, r *http.Request) {
	csrf, ok := s.preauthCSRF(w, r)
	if !ok {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	d := authFormData{CSRF: csrf, Expired: r.URL.Query().Get("expired") == "1", CanSetup: !s.st.FinanceExists()}
	if r.Method == http.MethodGet {
		s.publicFinancePage(w, "finance-login.html", d)
		return
	}
	if !s.verifyPreauth(r) {
		http.Error(w, "This page expired. Go back and try again.", 403)
		return
	}
	mobile := r.FormValue("mobile")
	if s.throttled(mobile) {
		d.Error = "Too many attempts. Wait 15 minutes and try again."
		s.publicFinancePage(w, "finance-login.html", d)
		return
	}
	key, id, err := s.st.UnlockFinance(mobile, r.FormValue("password"))
	if err != nil {
		s.failedAttempt(mobile)
		d.Error = store.ErrLoginFailed.Error()
		s.publicFinancePage(w, "finance-login.html", d)
		return
	}
	s.clearAttempts(mobile)
	pending := false
	_ = s.st.ReadFinance(key, func(f *register.FinanceData) { pending = f.RecoveryConfirmedAt == nil })
	if _, err := s.newFinanceSession(w, key, id, pending); err != nil {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	if pending {
		http.Redirect(w, r, "/finance/recovery-key/new", 303)
	} else {
		http.Redirect(w, r, "/finance", 303)
	}
}

func (s *Server) financeSetup(w http.ResponseWriter, r *http.Request) {
	if s.st.FinanceExists() {
		http.Redirect(w, r, "/finance/login", 303)
		return
	}
	csrf, ok := s.preauthCSRF(w, r)
	if !ok {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	d := authFormData{CSRF: csrf}
	if r.Method == http.MethodGet {
		s.publicFinancePage(w, "finance-setup.html", d)
		return
	}
	if !s.verifyPreauth(r) {
		http.Error(w, "This page expired. Go back and try again.", 403)
		return
	}
	if r.FormValue("password") != r.FormValue("again") {
		d.Error = "The passwords do not match."
		s.publicFinancePage(w, "finance-setup.html", d)
		return
	}
	key, id, recovery, err := s.st.InitializeFinance(r.FormValue("name"), r.FormValue("mobile"), r.FormValue("password"), s.now())
	if err != nil {
		d.Error = err.Error()
		s.publicFinancePage(w, "finance-setup.html", d)
		return
	}
	sess, err := s.newFinanceSession(w, key, id, true)
	if err != nil {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	s.financePage(w, r.WithContext(context.WithValue(r.Context(), financeContextKey{}, sess)), "Save this recovery key", "finance-confirm.html", struct{ Recovery, CSRF string }{recovery, sess.csrf})
}

func (s *Server) financeSetupConfirm(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	if r.FormValue("saved") != "yes" {
		http.Error(w, "Confirm that you saved the recovery key.", 400)
		return
	}
	if err := s.st.ConfirmFinanceRecovery(sess.vaultKey, sess.accountID, s.now()); err != nil {
		http.Error(w, "The change could not be saved.", 500)
		return
	}
	sess.recoveryPending = false
	http.Redirect(w, r, "/finance", 303)
}

func (s *Server) financeActivate(w http.ResponseWriter, r *http.Request) {
	csrf, _ := s.preauthCSRF(w, r)
	d := authFormData{CSRF: csrf}
	if r.Method == http.MethodGet {
		s.publicFinancePage(w, "finance-activate.html", d)
		return
	}
	if !s.verifyPreauth(r) {
		http.Error(w, "This page expired. Go back and try again.", 403)
		return
	}
	mobile := r.FormValue("mobile")
	if s.throttled(mobile) {
		d.Error = "Too many attempts. Wait 15 minutes and try again."
		s.publicFinancePage(w, "finance-activate.html", d)
		return
	}
	if r.FormValue("password") != r.FormValue("again") {
		d.Error = "The passwords do not match."
		s.publicFinancePage(w, "finance-activate.html", d)
		return
	}
	key, id, err := s.st.ActivateFinance(mobile, r.FormValue("code"), r.FormValue("password"), s.now())
	if err != nil {
		s.failedAttempt(mobile)
		d.Error = publicAuthError(err)
		s.publicFinancePage(w, "finance-activate.html", d)
		return
	}
	s.clearAttempts(mobile)
	if _, err = s.newFinanceSession(w, key, id, false); err != nil {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	http.Redirect(w, r, "/finance", 303)
}

func (s *Server) financeRecover(w http.ResponseWriter, r *http.Request) {
	csrf, _ := s.preauthCSRF(w, r)
	d := authFormData{CSRF: csrf}
	if r.Method == http.MethodGet {
		s.publicFinancePage(w, "finance-recover.html", d)
		return
	}
	if !s.verifyPreauth(r) {
		http.Error(w, "This page expired. Go back and try again.", 403)
		return
	}
	mobile := r.FormValue("mobile")
	if s.throttled(mobile) {
		d.Error = "Too many attempts. Wait 15 minutes and try again."
		s.publicFinancePage(w, "finance-recover.html", d)
		return
	}
	if r.FormValue("password") != r.FormValue("again") {
		d.Error = "The passwords do not match."
		s.publicFinancePage(w, "finance-recover.html", d)
		return
	}
	key, id, err := s.st.RecoverFinanceAdministrator(r.FormValue("recovery"), mobile, r.FormValue("password"), s.now())
	if err != nil {
		s.failedAttempt(mobile)
		d.Error = publicAuthError(err)
		s.publicFinancePage(w, "finance-recover.html", d)
		return
	}
	s.clearAttempts(mobile)
	s.invalidateAccountSessions(id, "")
	if _, err = s.newFinanceSession(w, key, id, false); err != nil {
		http.Error(w, "The page could not be opened.", 500)
		return
	}
	http.Redirect(w, r, "/finance", 303)
}

func publicAuthError(err error) string {
	if err == nil {
		return ""
	}
	return store.ErrLoginFailed.Error()
}

func (s *Server) financeLogout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(financeCookie)
	s.financeMu.Lock()
	if c != nil {
		delete(s.financeSessions, c.Value)
	}
	s.financeMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: financeCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/stock", 303)
}

type accountsPageData struct {
	Accounts          []register.FinanceAccount
	CSRF, Code, Error string
}

func (s *Server) financeAccounts(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	d := accountsPageData{CSRF: sess.csrf}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) { d.Accounts = append([]register.FinanceAccount{}, f.Accounts...) })
	s.financePage(w, r, "Authorized people", "finance-accounts.html", d)
}
func (s *Server) financeAccountNew(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	_, code, err := s.st.AuthorizeFinanceAccount(sess.vaultKey, sess.accountID, r.FormValue("name"), r.FormValue("mobile"), register.FinanceRole(r.FormValue("role")), s.now())
	d := accountsPageData{CSRF: sess.csrf, Code: code}
	if err != nil {
		d.Error = err.Error()
	}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) { d.Accounts = append([]register.FinanceAccount{}, f.Accounts...) })
	s.financePage(w, r, "Authorized people", "finance-accounts.html", d)
}
func (s *Server) financeAccountDisable(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	id := r.PathValue("id")
	if err := s.st.DisableFinanceAccount(sess.vaultKey, sess.accountID, id, s.now()); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.invalidateAccountSessions(id, "")
	http.Redirect(w, r, "/finance/accounts", 303)
}
func (s *Server) financeAccountReset(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	id := r.PathValue("id")
	code, err := s.st.ResetFinanceAccount(sess.vaultKey, sess.accountID, id, s.now())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.invalidateAccountSessions(id, "")
	d := accountsPageData{CSRF: sess.csrf, Code: code}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) { d.Accounts = append([]register.FinanceAccount{}, f.Accounts...) })
	s.financePage(w, r, "Authorized people", "finance-accounts.html", d)
}

type editPageData struct {
	Account     register.FinanceAccount
	CSRF, Error string
}

func (s *Server) financeAccountEdit(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	id := r.PathValue("id")
	d := editPageData{CSRF: sess.csrf}
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		for _, a := range f.Accounts {
			if a.ID == id {
				d.Account = a
			}
		}
	})
	if r.Method == http.MethodPost {
		if err := s.st.EditFinanceAccount(sess.vaultKey, sess.accountID, id, r.FormValue("name"), r.FormValue("mobile"), register.FinanceRole(r.FormValue("role")), s.now()); err != nil {
			d.Error = err.Error()
		} else {
			http.Redirect(w, r, "/finance/accounts", 303)
			return
		}
	}
	s.financePage(w, r, "Edit authorized person", "finance-account-edit.html", d)
}

func (s *Server) financePassword(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	d := authFormData{CSRF: sess.csrf}
	if r.Method == http.MethodPost {
		if r.FormValue("password") != r.FormValue("again") {
			d.Error = "The passwords do not match."
		} else if err := s.st.ChangeFinancePassword(sess.vaultKey, sess.accountID, r.FormValue("current"), r.FormValue("password"), s.now()); err != nil {
			d.Error = publicAuthError(err)
		} else {
			cookie, _ := r.Cookie(financeCookie)
			keep := ""
			if cookie != nil {
				keep = cookie.Value
			}
			s.invalidateAccountSessions(sess.accountID, keep)
			http.Redirect(w, r, "/finance", 303)
			return
		}
	}
	s.financePage(w, r, "Change password", "finance-password.html", d)
}

func (s *Server) financeRecoveryNew(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(financeContextKey{}).(*financeSession)
	d := authFormData{CSRF: sess.csrf}
	if r.Method == http.MethodPost {
		recovery, err := s.st.ReplaceFinanceRecovery(sess.vaultKey, sess.accountID, r.FormValue("password"), s.now())
		if err != nil {
			d.Error = publicAuthError(err)
		} else {
			sess.recoveryPending = true
			s.financePage(w, r, "Save this recovery key", "finance-confirm.html", struct{ Recovery, CSRF string }{recovery, sess.csrf})
			return
		}
	}
	s.financePage(w, r, "Replace recovery key", "finance-recovery-key.html", d)
}

func (s *Server) invalidateAccountSessions(id, keep string) {
	s.financeMu.Lock()
	defer s.financeMu.Unlock()
	for token, sess := range s.financeSessions {
		if sess.accountID == id && token != keep {
			delete(s.financeSessions, token)
		}
	}
}
func (s *Server) throttled(mobile string) bool {
	key := register.MobileKey(mobile)
	now := s.now()
	s.financeMu.Lock()
	defer s.financeMu.Unlock()
	a := s.financeFailures[key]
	kept := a[:0]
	for _, at := range a {
		if now.Sub(at) < 15*time.Minute {
			kept = append(kept, at)
		}
	}
	s.financeFailures[key] = kept
	return len(kept) >= 5
}
func (s *Server) failedAttempt(mobile string) {
	key := register.MobileKey(mobile)
	s.financeMu.Lock()
	s.financeFailures[key] = append(s.financeFailures[key], s.now())
	s.financeMu.Unlock()
}
func (s *Server) clearAttempts(mobile string) {
	s.financeMu.Lock()
	delete(s.financeFailures, register.MobileKey(mobile))
	s.financeMu.Unlock()
}
