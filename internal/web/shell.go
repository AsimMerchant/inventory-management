// Package web serves the register over loopback HTTP. It holds no arithmetic:
// every number on every page comes from internal/register.
package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// funcs are the three date renderings the whole program shares. They format the
// time as it was given, in its own zone, so a page never depends on where the
// developer's machine happens to be.
var funcs = template.FuncMap{
	"longstamp": longstamp,
	"clock":     clock,
	"shortdate": shortdate,
}

// longstamp is the subtitle on every flow page: Thursday, 3 September · 10:42 am.
func longstamp(t time.Time) string { return t.Format("Monday, 2 January · 3:04 pm") }

// clock is the time alone: 9:40 am.
func clock(t time.Time) string { return t.Format("3:04 pm") }

// shortdate is the day alone: 3 September.
func shortdate(t time.Time) string { return t.Format("2 January") }

var templates = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))

// tab is one entry in the chrome bar.
type tab struct {
	Label string
	Path  string
}

// tabs is the chrome bar, in order. The fifth is the activity log.
var tabs = []tab{
	{"Stock", "/stock"},
	{"Out with people", "/out"},
	{"Stuff came in", "/inwards"},
	{"Suppliers", "/suppliers"},
	{"Who did what", "/log"},
}

// banner is one coloured line above the page content. Kind is "ok", "warn" or "bad".
type banner struct {
	Kind string
	Text string
}

// page is everything layout.html needs. Data carries whatever the page itself needs.
type page struct {
	Title   string // the words in the chrome bar
	Current string // the path of the tab that is on, "" on a flow page
	Tabs    bool   // flow pages and the shift screen show none
	OnDuty  string // the on-duty person's name, "" when nobody is
	Narrow  bool   // the shift screen is 26rem wide, everything else 52rem
	Warning string // the recovery warning, on every page until restart
	Banner  *banner
	Tabbar  []tab
	Content template.HTML
}

// render draws one content template inside the shell. The content is rendered
// first and handed to the layout as HTML, so every template in the directory can
// carry its own name and the whole directory parses as one set.
func (s *Server) render(w http.ResponseWriter, status int, p page, name string, data any) {
	p.Tabbar = tabs
	p.Warning = s.warning

	var body bytes.Buffer
	if err := templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "The page could not be drawn. Go back to the register.", http.StatusInternalServerError)
		return
	}
	p.Content = template.HTML(body.String())

	var full bytes.Buffer
	if err := templates.ExecuteTemplate(&full, "layout.html", p); err != nil {
		http.Error(w, "The page could not be drawn. Go back to the register.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(full.Bytes())
}
