package web

import (
	"encoding/json"
	"errors"
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
	mode := r.URL.Query().Get("mode")

	var out []suggestion
	s.st.Read(func(reg *register.Register) {
		out = suggestMode(reg, q, mode)
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// suggest is matchProducts capped at eight rows, which is what the picker's
// dropdown shows. The uncapped list is the <noscript> fallback.
func suggest(reg *register.Register, query string, inStockOnly bool) []suggestion {
	mode := "all"
	if inStockOnly {
		mode = "instock"
	}
	return suggestMode(reg, query, mode)
}

func suggestMode(reg *register.Register, query, mode string) []suggestion {
	rows := matchProductsMode(reg, query, mode)
	if len(rows) > maxSuggestions {
		rows = rows[:maxSuggestions]
	}
	return rows
}

// matchProducts matches products by case-insensitive substring, names that
// start with the query first, alphabetical within each group.
func matchProducts(reg *register.Register, query string, inStockOnly bool) []suggestion {
	mode := "all"
	if inStockOnly {
		mode = "instock"
	}
	return matchProductsMode(reg, query, mode)
}

func matchProductsMode(reg *register.Register, query, mode string) []suggestion {
	q := strings.ToLower(register.CleanName(query))

	rows := []suggestion{}
	for _, p := range reg.Products {
		if p.Deleted != nil && mode != "log" {
			continue
		}
		lower := strings.ToLower(p.Name)
		if q != "" && !strings.Contains(lower, q) {
			continue
		}
		onHand := 0
		if p.Deleted == nil {
			onHand = register.OnHand(reg, p.ID)
		}
		if mode == "instock" && onHand == 0 {
			continue
		}
		label := p.Name + " — " + strconv.Itoa(onHand) + " on hand"
		if p.Deleted != nil {
			label = p.Name + " — deleted"
		}
		rows = append(rows, suggestion{
			ID: p.ID, Name: p.Name, OnHand: onHand,
			Label: label,
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
			if p.Deleted != nil {
				continue
			}
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

	var newID string
	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		for _, p := range reg.Products {
			if p.Deleted != nil {
				continue
			}
			if register.FoldKey(p.Name) == register.FoldKey(name) {
				existing = p.Name
				return errProductRefused
			}
		}
		if r.FormValue("confirm") != "yes" {
			if nearMatch = nearDuplicate(reg, name); nearMatch != "" {
				return errProductRefused
			}
		}
		who, ok := s.onDuty(reg)
		if !ok {
			return errProductRefused
		}
		newID = reg.NextID("PRD")
		reg.Products = append(reg.Products, register.Product{
			ID: newID, Name: name, CreatedAt: now, CreatedBy: who.Name,
		})
		return nil
	})
	if errors.Is(err, errProductRefused) {
		if existing != "" {
			s.renderReturnPage(w, back, &banner{"bad", existing + " is already on the list. Pick it."})
			return
		}
		if nearMatch != "" {
			s.renderConfirmProduct(w, name, nearMatch, back)
			return
		}
	}
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
		if p.Deleted != nil {
			continue
		}
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
	// Endpoint is which route the picker asks for suggestions. The ordinary
	// desk uses /api/products, which is behind the shift guard. The financial
	// area has no shift, so it asks its own session-gated route instead.
	Endpoint string
	// PartyFrom is a CSS selector for the field holding the supplier, when the
	// list depends on one. CountInto is a CSS selector for an element showing
	// how many may be returned or sold, which the picker keeps in step with
	// whatever was just chosen.
	PartyFrom string
	CountInto string
}

// picker resolves a submitted productId against the list. A productId that
// names nothing leaves PickedName empty, and every form refuses on that.
func (s *Server) picker(reg *register.Register, mode string, allowNew bool, pickedID string) pickerData {
	p := pickerData{Label: "Product", Mode: mode, AllowNew: allowNew, PickedID: pickedID}
	p.Products = matchProductsMode(reg, "", mode)
	for _, prod := range reg.Products {
		if prod.ID == pickedID && (prod.Deleted == nil || mode == "log") {
			p.PickedName = prod.Name
		}
	}
	if p.PickedName == "" {
		p.PickedID = ""
	}
	return p
}

type productEditData struct {
	Name     string
	FormName string
	Impact   register.ProductImpact
	Reason   string
	Refusal  string
	Deleted  bool
}

func (s *Server) productEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.Method == http.MethodPost {
		s.productRename(w, r, id)
		return
	}
	s.renderProductEdit(w, id, "", "")
}

func (s *Server) renderProductEdit(w http.ResponseWriter, id, reason, refusal string) {
	s.renderProductEditName(w, id, reason, refusal, "")
}

func (s *Server) renderProductEditName(w http.ResponseWriter, id, reason, refusal, typedName string) {
	var data productEditData
	status := http.StatusOK
	s.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.ID != id {
				continue
			}
			data.Name = p.Name
			data.FormName = p.Name
			if typedName != "" || refusal == "Type the product's name." {
				data.FormName = typedName
			}
			if p.Deleted != nil {
				data.Deleted = true
				return
			}
			data.Impact, _ = register.ProductDeletionImpact(reg, id)
			data.Reason, data.Refusal = reason, refusal
			return
		}
		status = http.StatusNotFound
	})
	p := s.page("Fix a product")
	p.Tabs = false
	if status == http.StatusNotFound {
		s.render(w, status, s.page("Store Register"), "notfound.html", nil)
		return
	}
	s.render(w, status, p, "product-edit.html", data)
}

func renameConflict(reg *register.Register, id, name string) (string, string) {
	for _, p := range reg.Products {
		if p.ID == id || p.Deleted != nil {
			continue
		}
		if register.FoldKey(p.Name) == register.FoldKey(name) {
			return p.Name, ""
		}
	}
	key := prefix4(name)
	if key != "" {
		for _, p := range reg.Products {
			if p.ID != id && p.Deleted == nil && prefix4(p.Name) == key {
				return "", p.Name
			}
		}
	}
	return "", ""
}

var errProductRefused = errors.New("product change refused")

func (s *Server) productRename(w http.ResponseWriter, r *http.Request, id string) {
	name := register.CleanName(r.FormValue("name"))
	confirm := r.FormValue("confirm") == "yes"
	var oldName, refusal string
	var near string
	s.st.Read(func(reg *register.Register) {
		p, ok := register.ProductByID(reg, id)
		if !ok {
			return
		}
		oldName = p.Name
		if name == "" {
			refusal = "Type the product's name."
			return
		}
		if dup, n := renameConflict(reg, id, name); dup != "" {
			refusal = dup + " is already on the list. Pick a different name."
		} else {
			near = n
		}
	})
	if oldName == "" {
		s.renderProductEdit(w, id, "", "")
		return
	}
	if refusal != "" {
		s.renderProductEditName(w, id, "", refusal, name)
		return
	}
	if near != "" && !confirm {
		p := s.page("Fix a product")
		p.Tabs = false
		s.render(w, http.StatusOK, p, "product-rename-confirm.html", struct{ ID, Old, New, Existing string }{id, oldName, name, near})
		return
	}
	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		p, ok := register.ProductByID(reg, id)
		if !ok {
			return errProductRefused
		}
		oldName = p.Name
		if name == "" {
			refusal = "Type the product's name."
			return errProductRefused
		}
		dup, n := renameConflict(reg, id, name)
		if dup != "" {
			refusal = dup + " is already on the list. Pick a different name."
			return errProductRefused
		}
		if n != "" && !confirm {
			refusal = n + " is already on the list. Rename this product to " + name + " anyway?"
			return errProductRefused
		}
		who, ok := s.onDuty(reg)
		if !ok {
			return errProductRefused
		}
		return register.RenameProduct(reg, id, name, who.Name, now)
	})
	if errors.Is(err, errProductRefused) {
		s.renderProductEditName(w, id, "", refusal, name)
		return
	}
	if err != nil {
		s.renderProductEditName(w, id, "", saveFailed, name)
		return
	}
	q := url.Values{"renamed": {id}, "old": {oldName}}
	http.Redirect(w, r, "/stock?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) productDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	reason := register.CleanName(r.FormValue("reason"))
	version := strings.TrimSpace(r.FormValue("impactVersion"))
	if reason == "" {
		s.renderProductEdit(w, id, reason, "Write why this product is being deleted.")
		return
	}
	var stale bool
	s.st.Read(func(reg *register.Register) {
		impact, ok := register.ProductDeletionImpact(reg, id)
		stale = ok && (version == "" || version != impact.Version)
	})
	if stale {
		s.renderProductEdit(w, id, reason, "This product changed. Check the numbers and confirm again.")
		return
	}
	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		impact, ok := register.ProductDeletionImpact(reg, id)
		if !ok {
			return errProductRefused
		}
		if version != impact.Version {
			return errProductRefused
		}
		who, ok := s.onDuty(reg)
		if !ok {
			return errProductRefused
		}
		return register.DeleteProductCascade(reg, id, who.Name, now, reason)
	})
	if errors.Is(err, errProductRefused) {
		s.renderProductEdit(w, id, reason, "This product changed. Check the numbers and confirm again.")
		return
	}
	if err != nil {
		s.renderProductEdit(w, id, reason, saveFailed)
		return
	}
	http.Redirect(w, r, "/stock?productDeleted="+url.QueryEscape(id), http.StatusSeeOther)
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
