package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"storeregister/internal/register"
)

// financeNewProduct is the answer to "the thing I am paying for is not on the
// list yet". The ordinary /product/new needs somebody on duty, and nobody is on
// duty in the financial area, so it cannot be reused: it would refuse silently.
//
// The product invariant is unchanged. A name that already exists is refused, a
// name close enough to be a misspelling has to be confirmed once, and the
// product this creates is an ordinary product that appears on the Stock screen
// at zero for the desk to receive normally.
func (s *Server) financeNewProduct(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	name := register.CleanName(r.FormValue("name"))
	if name == "" {
		writeProductAnswer(w, productAnswer{Error: "Type the new product's name."})
		return
	}

	who, _, _, _ := s.whoAmI(sess)

	var existing, near string
	s.st.Read(func(reg *register.Register) {
		for _, p := range reg.Products {
			if p.Deleted == nil && register.FoldKey(p.Name) == register.FoldKey(name) {
				existing = p.Name
			}
		}
		if existing == "" {
			near = nearDuplicate(reg, name)
		}
	})
	if existing != "" {
		writeProductAnswer(w, productAnswer{Error: existing + " is already on the list. Pick it."})
		return
	}
	// One deliberate confirmation, the same as the desk gets. A split product
	// silently halves the on-hand count, so this is never waved through.
	if near != "" && r.FormValue("confirm") != "yes" {
		writeProductAnswer(w, productAnswer{
			NeedsConfirm: true, Near: near,
			Error: near + " is already on the list. Adding " + name + " makes a second, separate product.",
		})
		return
	}

	var id string
	err := s.st.Update(func(reg *register.Register) error {
		// Both checks again inside the write: two people adding the same name
		// at once must not both win.
		for _, p := range reg.Products {
			if p.Deleted == nil && register.FoldKey(p.Name) == register.FoldKey(name) {
				existing = p.Name
				return errProductRefused
			}
		}
		if r.FormValue("confirm") != "yes" {
			if near = nearDuplicate(reg, name); near != "" {
				return errProductRefused
			}
		}
		id = reg.NextID("PRD")
		reg.Products = append(reg.Products, register.Product{
			ID: id, Name: name, CreatedAt: s.now(), CreatedBy: who,
		})
		return nil
	})
	switch {
	case errors.Is(err, errProductRefused) && existing != "":
		writeProductAnswer(w, productAnswer{Error: existing + " is already on the list. Pick it."})
	case errors.Is(err, errProductRefused):
		writeProductAnswer(w, productAnswer{NeedsConfirm: true, Near: near,
			Error: near + " is already on the list. Adding " + name + " makes a second, separate product."})
	case err != nil:
		writeProductAnswer(w, productAnswer{Error: saveFailed})
	default:
		writeProductAnswer(w, productAnswer{ID: id, Name: name})
	}
}

// productAnswer is what the picker gets back. It is JSON because the screens
// that ask are half-filled forms: a redirect would throw away everything the
// person had already typed.
type productAnswer struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Error        string `json:"error,omitempty"`
	NeedsConfirm bool   `json:"needsConfirm,omitempty"`
	Near         string `json:"near,omitempty"`
}

func writeProductAnswer(w http.ResponseWriter, a productAnswer) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}
