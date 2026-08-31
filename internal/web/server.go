package web

import (
	"net/http"
	"strings"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// Server is the whole web side of the register. There is one of it, and it
// holds the one store. now is a field so tests can pin the clock.
type Server struct {
	st      *store.Store
	warning string // the recovery warning, shown on every page until restart
	now     func() time.Time
	mux     *http.ServeMux
}

// NewServer wires the routing table. warning is LoadResult.Warning.
func NewServer(st *store.Store, warning string, now func() time.Time) *Server {
	s := &Server{st: st, warning: warning, now: now, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	m := s.mux

	m.HandleFunc("GET /{$}", s.home)

	// 05
	m.HandleFunc("GET /shift", s.shiftScreen)
	m.HandleFunc("POST /shift/start", s.shiftStart)
	m.HandleFunc("POST /shift/person", s.shiftPerson)

	// 10 fills these four in. For now each one draws the shell and its title,
	// so the tabs, the on-duty name and the recovery banner are already real.
	m.HandleFunc("GET /stock", s.stubView("Stock", "/stock"))
	m.HandleFunc("GET /out", s.stubView("Out with people", "/out"))
	m.HandleFunc("GET /inwards", s.stubView("Stuff came in", "/inwards"))
	m.HandleFunc("GET /suppliers", s.stubView("Suppliers", "/suppliers"))

	// 12 fills this in.
	m.HandleFunc("GET /log", s.stubView("Who did what", "/log"))

	// 07, 08, 09 fill these in. Registered now so the shift guard covers them.
	m.HandleFunc("/inward/new", s.stubFlow("Stuff came in"))
	m.HandleFunc("/issue/new", s.stubFlow("Someone is taking"))
	m.HandleFunc("/return/new", s.stubFlow("Someone is returning"))

	// 06
	m.HandleFunc("POST /product/new", s.productNew)
	m.HandleFunc("GET /api/products", s.apiProducts)

	// 08 fills this in.
	m.HandleFunc("GET /api/people", s.apiPeopleStub)

	// 11 fills these in.
	m.HandleFunc("/entry/{id}/edit", s.stubFlow("Fix this entry"))
	m.HandleFunc("POST /entry/{id}/delete", s.stubFlow("Fix this entry"))

	m.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	// Anything else. This pattern is also how the guard tells an unknown path
	// from a real route: an unknown path is a 404, not a trip to /shift.
	m.HandleFunc("/", s.notFound)
}

// ServeHTTP applies the shift guard and then routes. Every route except the
// shift screen itself and the static files needs a name to stamp on entries.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	return strings.HasPrefix(path, "/static/")
}

// home opens the register. Anybody reaching here has a name on them already:
// the guard above sent a visitor with no shift running to /shift.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, s.page("Store Register"), "notfound.html", nil)
}

// page is the shell as every handler starts with it: title, on-duty name, tabs on.
func (s *Server) page(title string) page {
	p := page{Title: title, Tabs: true}
	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			p.OnDuty = who.Name
		}
	})
	return p
}

// stubView draws one of the read-only tabs before its own spec is built.
func (s *Server) stubView(title, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.page(title)
		p.Current = path
		s.render(w, http.StatusOK, p, "stub.html", nil)
	}
}

// stubFlow draws a flow page's chrome before its own spec is built. Flow pages
// carry their own title and no tabs.
func (s *Server) stubFlow(title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.page(title)
		p.Tabs = false
		s.render(w, http.StatusOK, p, "stub.html", nil)
	}
}

// apiPeopleStub answers the person picker. 08 fills it in; until then it finds
// nobody, which is what an empty register would answer anyway.
func (s *Server) apiPeopleStub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("[]\n"))
}
