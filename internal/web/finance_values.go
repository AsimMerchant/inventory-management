package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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

type listRow struct {
	ID, Kind, Value, KindLabel string
	Used                       bool
	Targets                    []register.FinanceReusableValue
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
	_ = s.st.ReadFinance(sess.vaultKey, func(f *register.FinanceData) {
		fill := func(kind register.FinanceValueKind) []listRow {
			rows := []listRow{}
			live := register.LiveFinanceValues(f, kind)
			for _, v := range live {
				others := []register.FinanceReusableValue{}
				for _, o := range live {
					if o.ID != v.ID {
						others = append(others, o)
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
		data.Sections = []listSection{
			{"Suppliers and other parties", fill(register.FinanceParty)},
			{"Purposes", fill(register.FinancePurpose)},
			{"Payment modes", fill(register.FinanceMode)},
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

	var err error
	var notice string
	switch r.PathValue("action") {
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
