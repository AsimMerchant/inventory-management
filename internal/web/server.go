package web

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// Server is the whole web side of the register. There is one of it, and it
// holds the one store. now is a field so tests can pin the clock.
type Server struct {
	st              *store.Store
	warning         string // the recovery warning, shown on every page until restart
	now             func() time.Time
	mux             *http.ServeMux
	financeMu       sync.Mutex
	financeSessions map[string]*financeSession
	preauth         map[string]string
	financeFailures map[string][]time.Time
}

// NewServer wires the routing table. warning is LoadResult.Warning.
func NewServer(st *store.Store, warning string, now func() time.Time) *Server {
	s := &Server{st: st, warning: warning, now: now, mux: http.NewServeMux(), financeSessions: map[string]*financeSession{}, preauth: map[string]string{}, financeFailures: map[string][]time.Time{}}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux

	m.HandleFunc("GET /{$}", s.home)
	s.financeRoutes(m)

	// 05
	m.HandleFunc("GET /shift", s.shiftScreen)
	m.HandleFunc("POST /shift/start", s.shiftStart)
	m.HandleFunc("POST /shift/person", s.shiftPerson)

	// 10
	m.HandleFunc("GET /stock", s.stockView)
	m.HandleFunc("GET /out", s.outView)
	m.HandleFunc("GET /inwards", s.inwardsView)
	m.HandleFunc("GET /suppliers", s.suppliersView)

	// 12. Registered for GET only: POST /log is a 405 from the pattern router,
	// because nothing on that page writes anything.
	m.HandleFunc("GET /log", s.logView)

	// 07, 08, 09
	m.HandleFunc("/inward/new", s.inwardNew)
	m.HandleFunc("/issue/new", s.issueNew)
	m.HandleFunc("/return/new", s.returnNew)

	// 06
	m.HandleFunc("POST /product/new", s.productNew)
	m.HandleFunc("GET /product/{id}/edit", s.productEdit)
	m.HandleFunc("POST /product/{id}/edit", s.productEdit)
	m.HandleFunc("POST /product/{id}/delete", s.productDelete)
	m.HandleFunc("GET /api/products", s.apiProducts)

	// 08
	m.HandleFunc("GET /api/people", s.apiPeople)

	// 11
	m.HandleFunc("/entry/{id}/edit", s.entryEdit)
	m.HandleFunc("POST /entry/{id}/delete", s.entryDelete)

	m.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	// Anything else. This pattern is also how the guard tells an unknown path
	// from a real route: an unknown path is a 404, not a trip to /shift.
	m.HandleFunc("/", s.notFound)
}

// ServeHTTP applies the shift guard and then routes. Every route except the
// shift screen itself and the static files needs a name to stamp on entries.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/finance") {
		s.serveFinance(w, r)
		return
	}
	_, pattern := s.mux.Handler(r)
	if pattern != "/" && !exemptFromShift(r.URL.Path) {
		var running bool
		s.st.Read(func(reg *register.Register) {
			_, running = s.onDuty(reg)
		})
		if !running {
			http.Redirect(w, r, "/shift", http.StatusSeeOther)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func exemptFromShift(path string) bool {
	switch path {
	case "/shift", "/shift/start", "/shift/person":
		return true
	}
	return strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/finance")
}

// home opens the register. Anybody reaching here has a name on them already:
// the guard above sent a visitor with no shift running to /shift.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

// notFound draws the shell over an unknown path, and answers a wrong method on
// a path that is real. Go's router would give the 405 itself if this program
// had no catch-all pattern; the catch-all is what puts the shell on a mistyped
// address, so the check is made here instead.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		probe := r.Clone(r.Context())
		probe.Method = http.MethodGet
		if _, pattern := s.mux.Handler(probe); pattern != "/" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
	s.render(w, http.StatusNotFound, s.page("Store Register", r), "notfound.html", nil)
}

// page is the shell as every handler starts with it: title, on-duty name, tabs on.
func (s *Server) page(title string, request ...*http.Request) page {
	p := page{Title: title, Tabs: true}
	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			p.OnDuty = who.Name
		}
	})
	if len(request) != 0 {
		if sess, _ := s.authenticatedFinanceSession(request[0]); sess != nil {
			p.Finance = true
			p.FinanceAdmin = s.sessionIsAdmin(sess)
			p.CSRF = sess.csrf
		}
	}
	return p
}
