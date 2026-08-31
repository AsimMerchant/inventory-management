package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"storeregister/internal/register"
)

// maxSuggestions is how many rows the picker shows. More than this and the list
// stops being something a person reads at a glance.
const maxSuggestions = 8

// suggestion is one row of the product picker.
type suggestion struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	OnHand int    `json:"onHand"`
	Label  string `json:"label"`
}

// apiProducts answers the picker. mode=instock drops what is not there.
func (s *Server) apiProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	inStockOnly := r.URL.Query().Get("mode") == "instock"

	var out []suggestion
	s.st.Read(func(reg *register.Register) {
		out = suggest(reg, q, inStockOnly)
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// suggest is matchProducts capped at eight rows, which is what the picker's
// dropdown shows. The uncapped list is the <noscript> fallback.
func suggest(reg *register.Register, query string, inStockOnly bool) []suggestion {
	rows := matchProducts(reg, query, inStockOnly)
	if len(rows) > maxSuggestions {
		rows = rows[:maxSuggestions]
	}
	return rows
}

// matchProducts matches products by case-insensitive substring, names that
// start with the query first, alphabetical within each group.
func matchProducts(reg *register.Register, query string, inStockOnly bool) []suggestion {
	q := strings.ToLower(register.CleanName(query))

	rows := []suggestion{}
	for _, p := range reg.Products {
		lower := strings.ToLower(p.Name)
		if q != "" && !strings.Contains(lower, q) {
			continue
		}
		onHand := register.OnHand(reg, p.ID)
		if inStockOnly && onHand == 0 {
			continue
		}
		rows = append(rows, suggestion{
			ID: p.ID, Name: p.Name, OnHand: onHand,
			Label: p.Name + " — " + strconv.Itoa(onHand) + " on hand",
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		li := strings.ToLower(rows[i].Name)
		lj := strings.ToLower(rows[j].Name)
		pi := q != "" && strings.HasPrefix(li, q)
		pj := q != "" && strings.HasPrefix(lj, q)
		if pi != pj {
			return pi
		}
		return li < lj
	})

	return rows
}

// productNew is the only handler in the program that may append a product.
// Every other handler resolves a productId against the list and refuses if it
// does not match: a name typed but never picked never becomes a record.
func (s *Server) productNew(w http.ResponseWriter, r *http.Request) {
	name := register.CleanName(r.FormValue("name"))
	back := backPath(r.FormValue("return"))

	if name == "" {
		s.renderReturnPage(w, back, &banner{"bad", "Type the new product's name."})
		return
	}

	var existing, nearMatch string
	s.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if register.FoldKey(p.Name) == register.FoldKey(name) {
				existing = p.Name
			}
		}
		if existing == "" {
			nearMatch = nearDuplicate(reg, name)
		}
	})
	if existing != "" {
		s.renderReturnPage(w, back, &banner{"bad", existing + " is already on the list. Pick it."})
		return
	}

	// A shared four-character prefix is close enough to be a misspelling and far
	// enough apart that the case-fold guard cannot see it: Chair against Chairs,
	// Round table against Round tables. The person confirms once, deliberately.
	if nearMatch != "" && r.FormValue("confirm") != "yes" {
		s.renderConfirmProduct(w, name, nearMatch, back)
		return
	}

	// The shift guard means somebody is always on duty here, so CreatedBy on a
	// product is never empty.
	var addedBy, newID string
	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			addedBy = who.Name
		}
	})

	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		newID = reg.NextID("PRD")
		reg.Products = append(reg.Products, register.Product{
			ID: newID, Name: name, CreatedAt: now, CreatedBy: addedBy,
		})
		return nil
	})
	if err != nil {
		s.renderReturnPage(w, back, &banner{"bad", saveFailed})
		return
	}

	q := url.Values{"picked": {newID}, "added": {name}}
	http.Redirect(w, r, back+"?"+q.Encode(), http.StatusSeeOther)
}

// nearDuplicate returns the name of an existing product sharing a case-folded
// four-character prefix with n, or "" when there is none. Characters, not
// bytes: a name that starts with a multi-byte character must not be cut
// through the middle of one.
func nearDuplicate(reg *register.Register, n string) string {
	key := prefix4(n)
	if key == "" {
		return ""
	}
	for _, p := range reg.Products {
		if prefix4(p.Name) == key {
			return p.Name
		}
	}
	return ""
}

// prefix4 is the first four characters of a folded name, or "" if it is shorter.
func prefix4(s string) string {
	r := []rune(register.FoldKey(s))
	if len(r) < 4 {
		return ""
	}
	return string(r[:4])
}

type confirmData struct {
	Typed    string
	Existing string
	Return   string
}

// renderConfirmProduct asks once before making a second, separate product.
func (s *Server) renderConfirmProduct(w http.ResponseWriter, typed, existing, back string) {
	p := s.page("Add a product")
	p.Tabs = false
	s.render(w, http.StatusOK, p, "productconfirm.html", confirmData{
		Typed: typed, Existing: existing, Return: back,
	})
}

// renderReturnPage shows a refusal on the shell of the page the form came from,
// with one way back. back is kept for the caller that names the form it came
// from; the way out of a refusal is the same either way.
func (s *Server) renderReturnPage(w http.ResponseWriter, back string, b *banner) {
	p := s.page("Store Register")
	p.Tabs = false
	p.add(b)
	s.render(w, http.StatusOK, p, "stub.html", nil)
}

// backPath keeps the return path to the two the form may send, so a hand-built
// request cannot bounce somebody off this program.
func backPath(v string) string {
	switch v {
	case "/inward/new", "/stock":
		return v
	}
	return "/stock"
}

// pickerData is what picker.html draws. Products is the whole filtered list,
// uncapped: it is the <noscript> fallback, and a person without JavaScript must
// still see every product, not the first eight.
type pickerData struct {
	Label      string
	Mode       string
	AllowNew   bool
	PickedID   string
	PickedName string
	Products   []suggestion
}

// picker resolves a submitted productId against the list. A productId that
// names nothing leaves PickedName empty, and every form refuses on that.
func (s *Server) picker(reg *register.Register, mode string, allowNew bool, pickedID string) pickerData {
	p := pickerData{Label: "Product", Mode: mode, AllowNew: allowNew, PickedID: pickedID}
	p.Products = matchProducts(reg, "", mode == "instock")
	for _, prod := range reg.Products {
		if prod.ID == pickedID {
			p.PickedName = prod.Name
		}
	}
	if p.PickedName == "" {
		p.PickedID = ""
	}
	return p
}

// formProductID reads the picked product out of a request. The picker submits a
// hidden field and the <noscript> fallback submits a <select> of the same name,
// so both arrive; whichever carries a value is the answer.
func formProductID(r *http.Request) string {
	_ = r.ParseForm()
	for _, v := range r.Form["productId"] {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
