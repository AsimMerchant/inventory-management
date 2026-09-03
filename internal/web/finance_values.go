package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

// valueSuggestion is one row of a party, purpose or payment-mode picker.
type valueSuggestion struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// valuePicker is what the shared value-picker template needs. One form may hold
// several of these, so every field name is carried rather than assumed.
type valuePicker struct {
	Kind       string
	Label      string
	IDField    string
	TextField  string
	PickedID   string
	PickedText string
	AddLabel   string
	Values     []register.FinanceReusableValue
}

func valueKind(s string) (register.FinanceValueKind, bool) {
	switch register.FinanceValueKind(s) {
	case register.FinanceParty:
		return register.FinanceParty, true
	case register.FinancePurpose:
		return register.FinancePurpose, true
	case register.FinanceMode:
		return register.FinanceMode, true
	}
	return "", false
}

// financeAPIValues answers the party, purpose and mode pickers. It is behind
// the finance session gate, so the protected lists never reach an ordinary
// page: the inward supplier box still suggests only names off live inwards.
func (s *Server) financeAPIValues(w http.ResponseWriter, r *http.Request) {
	kind, ok := valueKind(r.URL.Query().Get("kind"))
	if !ok {
		http.Error(w, "Unknown list.", http.StatusBadRequest)
		return
	}
	sess := financeSessionOf(r)
	out := []valueSuggestion{}
	_ = s.st.ReadFinance(sess.vaultKey, func(data *register.FinanceData) {
		rows := register.MatchFinanceValues(data, kind, r.URL.Query().Get("q"))
		if len(rows) > maxSuggestions {
			rows = rows[:maxSuggestions]
		}
		for _, v := range rows {
			out = append(out, valueSuggestion{ID: v.ID, Value: v.Value, Label: v.Value})
		}
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// financeAPIProducts answers the product picker on the financial screens. The
// ordinary /api/products sits behind the inventory shift guard, and a
// financial user recording an order has no shift running: without this route
// the picker silently returns nothing and no product can be chosen at all.
func (s *Server) financeAPIProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sess := financeSessionOf(r)
	out := []suggestion{}

	// The settlement screens ask a different question: not "which products
	// exist" but "which can still go back to this supplier", or "which can
	// still be sold". Those lists are shorter and the number is the whole
	// point, so the label carries it.
	switch q.Get("mode") {
	case "return", "sale":
		_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
			party := ""
			// The party narrows the list only when the person asked it to. By
			// default every product that can leave the store is offered,
			// because goods are not always handed to the supplier who sent
			// them.
			if q.Get("onlyParty") != "" {
				party = q.Get("party")
			}
			out = settlementSuggestions(reg, f, q.Get("mode"), party, q.Get("q"))
		})
	default:
		s.st.Read(func(reg *register.Register) {
			out = suggestMode(reg, q.Get("q"), q.Get("mode"))
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// settlementSuggestions lists what may still be sent back or sold, matched the
// same way the product picker matches anywhere else: names starting with the
// query first, then the rest, capped at eight.
//
// The party has no bearing on either list. Goods are handed to whoever is
// taking them — a transporter, somebody doing the rounds — and refusing that
// because the name is not the supplier on the delivery note would be the
// register inventing a rule the store does not have. What limits a return is
// what came in on rent and is still here; what limits a sale is what was
// bought and is still here.
func settlementSuggestions(reg *register.Register, f *register.FinanceData, mode, party, query string) []suggestion {
	rows := []suggestion{}
	for _, p := range reg.Products {
		if p.Deleted != nil {
			continue
		}
		if mode == "sale" {
			available := register.PurchasedAvailableToSell(reg, f, p.ID)
			if available <= 0 {
				continue
			}
			rows = append(rows, suggestion{
				ID: p.ID, Name: p.Name, OnHand: available,
				Label: p.Name + " — " + strconv.Itoa(available) + " available",
			})
			continue
		}

		// A return. Every product that is here can go back, to whoever is
		// taking it, so by default who that is has no bearing on the list.
		available := register.SupplierReturnAvailable(reg, f, "", p.ID)
		if available <= 0 {
			continue
		}
		// The screen may ask for one supplier's goods only. That is somebody
		// narrowing a long list on purpose, never something they must do.
		if party != "" && register.SupplierSentRented(reg, f, party, p.ID) <= 0 {
			continue
		}
		rows = append(rows, suggestion{
			ID: p.ID, Name: p.Name, OnHand: available,
			Label: p.Name + " — " + strconv.Itoa(available) + " available",
		})
	}
	rows = matchByName(rows, query)
	if len(rows) > maxSuggestions {
		rows = rows[:maxSuggestions]
	}
	return rows
}

// matchByName filters and orders an already-built list the way the product
// picker does: case-insensitive substring, names that start with the query
// first, alphabetical within each group.
func matchByName(rows []suggestion, query string) []suggestion {
	q := strings.ToLower(register.CleanName(query))
	kept := []suggestion{}
	for _, r := range rows {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) {
			kept = append(kept, r)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		li, lj := strings.ToLower(kept[i].Name), strings.ToLower(kept[j].Name)
		pi := q != "" && strings.HasPrefix(li, q)
		pj := q != "" && strings.HasPrefix(lj, q)
		if pi != pj {
			return pi
		}
		return li < lj
	})
	return kept
}

// financeSessionOf reads the session serveFinance already put on the request.
// Every /finance route past the public four is reached only through it.
func financeSessionOf(r *http.Request) *financeSession {
	sess, _ := r.Context().Value(financeContextKey{}).(*financeSession)
	return sess
}

// resolveValue turns one picker's submitted pair into a value ID inside the
// caller's transaction. A selected ID wins; otherwise the typed text is matched
// exactly and reused, and only a genuinely new spelling creates a row. Doing
// this inside the closure is what stops two simultaneous posts creating two
// rows that say the same thing.
func resolveValue(data *register.FinanceData, kind register.FinanceValueKind, id, typed, actorID string, at time.Time) (string, error) {
	if id != "" {
		if v, ok := register.ResolveFinanceValue(data, id); ok && v.Kind == kind {
			return v.ID, nil
		}
	}
	if register.CleanName(typed) == "" {
		return "", errors.New("blank")
	}
	return register.AddFinanceValue(data, kind, typed, actorID, at)
}

// pickerFor builds one picker, filled in with whatever the person had already
// chosen or typed so a refusal never costs them their typing.
func pickerFor(data *register.FinanceData, kind register.FinanceValueKind, label, idField, textField, pickedID, pickedText, addLabel string) valuePicker {
	if pickedID != "" && pickedText == "" {
		pickedText = register.FinanceValueText(data, pickedID)
	}
	return valuePicker{
		Kind: string(kind), Label: label, IDField: idField, TextField: textField,
		PickedID: pickedID, PickedText: pickedText, AddLabel: addLabel,
		Values: register.LiveFinanceValues(data, kind),
	}
}

// listTarget is one value a row may be merged into. It is the same shape for
// the vault's three lists and for the open acquisition-kinds list, so the
// screen draws them all the same way.
type listTarget struct {
	ID, Value string
}

type listRow struct {
	ID, Kind, Value, KindLabel string
	Used                       bool
	Targets                    []listTarget
}

type listSection struct {
	Title string
	Rows  []listRow
}

type listsData struct {
	CSRF, Error, Notice string
	Sections            []listSection
}

// financeLists is the administrator's screen for the three shared lists.
func (s *Server) financeLists(w http.ResponseWriter, r *http.Request) {
	s.renderLists(w, r, "", r.URL.Query().Get("done"))
}

func (s *Server) renderLists(w http.ResponseWriter, r *http.Request, problem, notice string) {
	sess := financeSessionOf(r)
	data := listsData{CSRF: sess.csrf, Error: problem, Notice: notice}
	// One lock, both halves: the three financial lists are in the vault and
	// the acquisition kinds are in the open register, and Read and ReadFinance
	// take the same mutex.
	_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
		fill := func(kind register.FinanceValueKind) []listRow {
			rows := []listRow{}
			live := register.LiveFinanceValues(f, kind)
			for _, v := range live {
				others := []listTarget{}
				for _, o := range live {
					if o.ID != v.ID {
						others = append(others, listTarget{ID: o.ID, Value: o.Value})
					}
				}
				rows = append(rows, listRow{
					ID: v.ID, Kind: string(kind), Value: v.Value,
					KindLabel: register.ValueKindLabel(kind),
					Used:      register.FinanceValueIsUsed(f, v.ID) || financeValueIsMergeTarget(f, v.ID),
					Targets:   others,
				})
			}
			return rows
		}
		kinds := []listRow{}
		liveKinds := register.LiveAcquisitionKinds(reg)
		for _, k := range liveKinds {
			others := []listTarget{}
			for _, o := range liveKinds {
				if o.ID != k.ID {
					others = append(others, listTarget{ID: o.ID, Value: o.Name})
				}
			}
			kinds = append(kinds, listRow{
				ID: k.ID, Kind: "acquisitionKind", Value: k.Name,
				KindLabel: "How goods came in",
				Used:      register.AcquisitionKindIsUsed(reg, f, k.ID),
				Targets:   others,
			})
		}
		data.Sections = []listSection{
			{"Suppliers and other parties", fill(register.FinanceParty)},
			{"Purposes", fill(register.FinancePurpose)},
			{"Payment modes", fill(register.FinanceMode)},
			{"Ways goods came in, besides rent and purchase", kinds},
		}
	})
	s.financePage(w, r, "Shared lists", "finance-lists.html", data)
}

func financeValueIsMergeTarget(f *register.FinanceData, id string) bool {
	for _, v := range f.ReusableValues {
		if v.MergedIntoID == id {
			return true
		}
	}
	return false
}

// financeListAction runs one rename, merge or delete and comes straight back to
// the lists screen, saying what happened or why it was refused.
func (s *Server) financeListAction(w http.ResponseWriter, r *http.Request) {
	sess := financeSessionOf(r)
	id := r.PathValue("id")
	now := s.now()
	action := r.PathValue("action")
	if action == "merge" && r.FormValue("confirm") == "yes" && r.FormValue("confirmedTarget") != r.FormValue("target") {
		r.Form.Set("confirm", "")
	}
	// The acquisition kinds live in the open register rather than the vault,
	// so they take the same three actions down their own path. Nothing else
	// about the screen differs.
	if strings.HasPrefix(id, "AKD-") {
		s.kindListAction(w, r, id, action, now)
		return
	}
	if (action == "merge" || action == "delete") && r.FormValue("confirm") != "yes" {
		var source, target register.FinanceReusableValue
		var sourceOK, targetOK bool
		var sourceUsed bool
		_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
			source, sourceOK = register.FinanceValueByID(f, id)
			sourceUsed = register.FinanceValueIsUsed(f, id) || financeValueIsMergeTarget(f, id)
			if action == "merge" {
				target, targetOK = register.FinanceValueByID(f, r.FormValue("target"))
			}
		})
		if !sourceOK || source.MergedIntoID != "" || (action == "merge" && (!targetOK || target.MergedIntoID != "" || target.Kind != source.Kind || target.ID == source.ID)) {
			s.renderLists(w, r, "Pick two different values from the same list.", "")
			return
		}
		if action == "delete" && sourceUsed {
			s.renderLists(w, r, register.ErrValueUsed.Error(), "")
			return
		}
		if action == "merge" {
			s.renderFinanceConfirm(w, r, financeConfirmData{
				Heading: "Combine " + source.Value + " into " + target.Value + "?",
				Warning: "Every financial record that currently shows " + source.Value + " will show " + target.Value + ". The activity history will keep both earlier names.",
				Action:  "/finance/lists/" + id + "/merge", Button: "Yes, combine these values",
				Target: target.ID, ConfirmedTarget: target.ID,
			})
			return
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Delete unused " + strings.ToLower(register.ValueKindLabel(source.Kind)) + " “" + source.Value + "”?",
			Warning: "It will be removed from future suggestions. This is allowed only because no current financial record uses it.",
			Action:  "/finance/lists/" + id + "/delete", Button: "Yes, delete this unused value",
		})
		return
	}

	var err error
	var notice string
	switch action {
	case "rename":
		err = s.st.RenameFinanceValue(sess.vaultKey, sess.accountID, id, r.FormValue("value"), now)
		notice = "Wording corrected."
	case "merge":
		err = s.st.MergeFinanceValue(sess.vaultKey, sess.accountID, id, r.FormValue("target"), now)
		notice = "The two entries are now one."
	case "delete":
		err = s.st.DeleteFinanceValue(sess.vaultKey, sess.accountID, id, now)
		notice = "Removed."
	default:
		s.financeNotFound(w, r)
		return
	}

	switch {
	case errors.Is(err, register.ErrValueUsed):
		s.renderLists(w, r, register.ErrValueUsed.Error(), "")
	case errors.Is(err, store.ErrNotAdmin):
		w.WriteHeader(http.StatusForbidden)
	case err != nil:
		s.renderLists(w, r, err.Error(), "")
	default:
		http.Redirect(w, r, "/finance/lists?"+url.Values{"done": {notice}}.Encode(), http.StatusSeeOther)
	}
}

// kindListAction runs one rename, merge or delete on the shared list of ways
// goods came in. It is the value actions above with one difference: the list
// is plain register data, because the delivery desk reads it and is never
// logged in. The confirmation screens, the refusals and the destination are
// the same.
func (s *Server) kindListAction(w http.ResponseWriter, r *http.Request, id, action string, now time.Time) {
	sess := financeSessionOf(r)
	if (action == "merge" || action == "delete") && r.FormValue("confirm") != "yes" {
		var source, target register.AcquisitionKind
		var sourceOK, targetOK, sourceUsed bool
		_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, f *register.FinanceData) {
			source, sourceOK = register.AcquisitionKindByID(reg, id)
			sourceUsed = register.AcquisitionKindIsUsed(reg, f, id)
			if action == "merge" {
				target, targetOK = register.AcquisitionKindByID(reg, r.FormValue("target"))
			}
		})
		if !sourceOK || source.MergedIntoID != "" || (action == "merge" && (!targetOK || target.MergedIntoID != "" || target.ID == source.ID)) {
			s.renderLists(w, r, "Pick two different values from the same list.", "")
			return
		}
		if action == "delete" && sourceUsed {
			s.renderLists(w, r, register.ErrKindUsed.Error(), "")
			return
		}
		if action == "merge" {
			s.renderFinanceConfirm(w, r, financeConfirmData{
				Heading: "Combine " + source.Name + " into " + target.Name + "?",
				Warning: "Every delivery and order that currently shows " + source.Name + " will show " + target.Name + ". The activity history will keep both earlier words.",
				Action:  "/finance/lists/" + id + "/merge", Button: "Yes, combine these values",
				Target: target.ID, ConfirmedTarget: target.ID,
			})
			return
		}
		s.renderFinanceConfirm(w, r, financeConfirmData{
			Heading: "Delete unused way goods came in “" + source.Name + "”?",
			Warning: "It will be removed from future suggestions. This is allowed only because no delivery and no order uses it.",
			Action:  "/finance/lists/" + id + "/delete", Button: "Yes, delete this unused value",
		})
		return
	}

	var err error
	var notice string
	switch action {
	case "rename":
		err = s.st.RenameAcquisitionKind(sess.vaultKey, sess.accountID, id, r.FormValue("value"), now)
		notice = "Wording corrected."
	case "merge":
		err = s.st.MergeAcquisitionKind(sess.vaultKey, sess.accountID, id, r.FormValue("target"), now)
		notice = "The two entries are now one."
	case "delete":
		err = s.st.DeleteAcquisitionKind(sess.vaultKey, sess.accountID, id, now)
		notice = "Removed."
	default:
		s.financeNotFound(w, r)
		return
	}

	switch {
	case errors.Is(err, register.ErrKindUsed):
		s.renderLists(w, r, register.ErrKindUsed.Error(), "")
	case errors.Is(err, store.ErrNotAdmin):
		w.WriteHeader(http.StatusForbidden)
	case err != nil:
		s.renderLists(w, r, err.Error(), "")
	default:
		http.Redirect(w, r, "/finance/lists?"+url.Values{"done": {notice}}.Encode(), http.StatusSeeOther)
	}
}
