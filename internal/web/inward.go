package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"storeregister/internal/register"
)

// productWord is how a product name reads in the middle of a sentence: "Save —
// 500 chairs in", "There are no chairs left." A name that carries its own
// capitals or a number is left exactly as it was stored, because lowercasing
// "Water drums (20L)" would be mangling somebody's product name to fit a
// sentence. Shared with the issue and return screens.
func productWord(name string) string {
	rs := []rune(name)
	if len(rs) == 0 {
		return name
	}
	for i, r := range rs {
		if unicode.IsDigit(r) {
			return name
		}
		if i > 0 && unicode.IsUpper(r) {
			return name
		}
	}
	return string(unicode.ToLower(rs[0])) + string(rs[1:])
}

// dateLayout is how a date-only field is stored and how an <input type="date">
// hands it back.
const dateLayout = "2006-01-02"

// inwardData is everything inward.html draws, on a fresh form and on a refused
// one alike. Every typed value comes back so nothing is typed twice.
type inwardData struct {
	Now         time.Time
	Picker      pickerData
	Quantity    string
	ReceivedOn  string
	Basis       string
	KindID      string                     // the kind picked off the list, when the basis is "other"
	NewKind     string                     // a kind typed for the first time
	Kinds       []register.AcquisitionKind // the shared list of typed kinds
	Party       valuePicker                // the shared supplier and other-party list
	ChallanNo   string
	ReceivedBy  string
	ButtonLabel string
}

// inwardNew is both the form and the save. One handler so a refusal re-renders
// through exactly the same code that drew the page in the first place.
func (s *Server) inwardNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.inwardSave(w, r)
		return
	}

	q := r.URL.Query()
	data := s.inwardForm(q.Get("picked"), "", "", "")

	var b *banner
	if added := q.Get("added"); added != "" {
		b = &banner{"ok", added + " added to the product list."}
	}
	s.renderInward(w, data, b)
}

// inwardForm builds the page from a picked product, a typed quantity and
// whatever the supplier picker was holding.
func (s *Server) inwardForm(productID, quantity, partyID, partyText string) inwardData {
	now := s.now()
	data := inwardData{
		Now:        now,
		Quantity:   quantity,
		ReceivedOn: now.Format(dateLayout),
		Basis:      string(register.Rent),
	}
	s.st.Read(func(reg *register.Register) {
		data.Picker = s.picker(reg, "all", true, productID)
		data.Party = partyPicker(reg, "Came from", "partyId", "partyName", partyID, partyText)
		data.Kinds = register.LiveAcquisitionKinds(reg)
		if who, ok := s.onDuty(reg); ok {
			data.ReceivedBy = who.Name
		}
	})
	data.ButtonLabel = inwardButtonLabel(data.Picker.PickedName, quantity)
	return data
}

// inwardButtonLabel is "Save — 500 chairs in", or "Save" before there is
// anything to say. The same words are recomputed in the browser as the two
// fields change.
func inwardButtonLabel(productName, quantity string) string {
	n, err := strconv.Atoi(strings.TrimSpace(quantity))
	if productName == "" || err != nil || n < 1 {
		return "Save"
	}
	return "Save — " + strconv.Itoa(n) + " " + productWord(productName) + " in"
}

func (s *Server) renderInward(w http.ResponseWriter, data inwardData, b *banner) {
	p := s.page("Stuff came in")
	p.Tabs = false
	p.add(b)
	s.render(w, http.StatusOK, p, "inward.html", data)
}

// inwardSave validates in the order the spec lists and writes nothing until
// every field has passed. Stuff arriving is never refused for being too much:
// there is no maximum quantity here.
func (s *Server) inwardSave(w http.ResponseWriter, r *http.Request) {
	productID := formProductID(r)
	quantity := strings.TrimSpace(r.FormValue("quantity"))
	receivedOn := strings.TrimSpace(r.FormValue("receivedOn"))
	basis := strings.TrimSpace(r.FormValue("basis"))
	kindID := strings.TrimSpace(r.FormValue("kindId"))
	newKind := register.CleanName(r.FormValue("newKind"))
	partyID, partyText := readPartyPicker(r, "partyId", "partyName")
	if partyID == "" && partyText == "" {
		// A page left open while the executable is replaced still posts the
		// former supplier field. Treat it as a typed shared name rather than
		// silently losing what the desk entered.
		partyText = register.CleanName(r.FormValue("supplier"))
	}
	challanNo := strings.TrimSpace(r.FormValue("challanNo"))

	data := s.inwardForm(productID, quantity, partyID, partyText)
	data.ReceivedOn = receivedOn
	data.Basis = basis
	data.KindID, data.NewKind = kindID, newKind
	data.ChallanNo = challanNo

	refuse := func(text string) {
		s.renderInward(w, data, &banner{"bad", text})
	}

	// A name typed into the box but never picked off the list arrives here as an
	// empty productId, and is refused. That is the whole product invariant.
	name := data.Picker.PickedName
	if productID == "" || name == "" {
		refuse("Pick the product from the list.")
		return
	}
	n, err := wholeNumber(quantity)
	if err != nil {
		refuse("Type how many " + productWord(name) + " came in.")
		return
	}
	if _, err := time.Parse(dateLayout, receivedOn); err != nil {
		refuse("Type the date like this: 03-09-2026.")
		return
	}
	if basis != string(register.Rent) && basis != string(register.Purchase) && basis != string(register.Other) {
		refuse("Choose how these came in.")
		return
	}
	if basis != string(register.Other) {
		kindID, newKind = "", ""
	} else if newKind == "" && kindID == "" {
		refuse("Pick how these came in from the list, or type a new word for it.")
		return
	}

	var onDutyName string
	knownKind := false
	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			onDutyName = who.Name
		}
		_, knownKind = register.ResolveAcquisitionKind(reg, kindID)
	})
	if basis == string(register.Other) && newKind == "" && !knownKind {
		refuse("Pick how these came in from the list, or type a new word for it.")
		return
	}

	now := s.now()
	var newID string
	err = s.st.Update(func(reg *register.Register) error {
		// A word typed for the first time joins the shared list in the same
		// write as the delivery it describes, so two desks saving "donated"
		// at once cannot leave two rows saying it.
		id := kindID
		if basis == string(register.Other) && newKind != "" {
			added, addErr := register.AddAcquisitionKind(reg, newKind, onDutyName, now)
			if addErr != nil {
				return addErr
			}
			id = added
		}
		// A supplier typed for the first time joins the shared list in the
		// same write as the delivery, so two desks saving one name at once
		// cannot leave two rows saying it. The name is stored on the delivery
		// as well, as history and so the file reads as words by hand.
		party, supplier, partyErr := resolvePartyID(reg, partyID, partyText)
		if partyErr != nil {
			return partyErr
		}
		newID = reg.NextID("INW")
		reg.Inwards = append(reg.Inwards, register.Inward{
			ID: newID, ProductID: productID, Quantity: n,
			ReceivedOn: receivedOn, Basis: register.Basis(basis), KindID: id,
			PartyID: party, Supplier: supplier, ChallanNo: challanNo,
			ReceivedBy: onDutyName, RecordedAt: now, RecordedBy: onDutyName,
		})
		return nil
	})
	if err != nil {
		refuse(saveFailed)
		return
	}
	http.Redirect(w, r, "/stock?saved="+newID, http.StatusSeeOther)
}

// wholeNumber reads a quantity exactly as typed. "12.5", "abc", "0" and "-5"
// are all the same answer to the person at the desk: type how many.
func wholeNumber(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, strconv.ErrRange
	}
	return n, nil
}
