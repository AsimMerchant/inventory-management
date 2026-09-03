package web

import (
	"encoding/json"
	"net/http"

	"storeregister/internal/register"
)

// The supplier and other-party list is one list for the whole program: the
// delivery desk picks a supplier off it, and the money screens offer the same
// names. It lives in the open register because the desk is never logged in.
// Only names travel: no amount, no purpose, no payment mode and no link to any
// money record is on this route or on any desk screen.

// partyPicker builds the shared picker for a supplier or other party, filled in
// with whatever was already chosen or typed so a refusal never costs somebody
// their typing.
func partyPicker(reg *register.Register, label, idField, textField, pickedID, pickedText string) valuePicker {
	if pickedID != "" && pickedText == "" {
		pickedText = register.PartyText(reg, pickedID)
	}
	options := []valueOption{}
	for _, p := range register.LiveParties(reg) {
		options = append(options, valueOption{ID: p.ID, Value: p.Name})
	}
	return valuePicker{
		Kind: "party", URL: "/api/parties",
		Label: label, IDField: idField, TextField: textField,
		PickedID: pickedID, PickedText: pickedText,
		Values: options,
	}
}

// readPartyPicker takes the four fields one picker submits: the hidden ID and
// text the browser fills in, and the select and the new-name box the no-script
// fallback uses instead.
func readPartyPicker(r *http.Request, idField, textField string) (string, string) {
	id := r.FormValue(idField)
	text := register.CleanName(r.FormValue(textField))
	if typed := register.CleanName(r.FormValue(textField + "New")); typed != "" {
		return "", typed
	}
	if picked := r.FormValue(idField + "Choice"); picked != "" {
		return picked, ""
	}
	return id, text
}

// resolvePartyID turns one picker's submitted pair into a list entry inside the
// caller's write. A selected ID wins; otherwise the typed name is matched
// exactly and reused, and only a genuinely new spelling adds a row. Doing it
// inside the transaction is what stops two saves at once leaving two rows that
// say the same name. A blank pair is a record that names nobody, which is
// allowed: the desk may not know who delivered.
func resolvePartyID(reg *register.Register, id, typed string) (string, string, error) {
	if id != "" {
		if p, ok := register.ResolveParty(reg, id); ok {
			return p.ID, p.Name, nil
		}
	}
	typed = register.CleanName(typed)
	if typed == "" {
		return "", "", nil
	}
	newID, err := register.AddParty(reg, typed)
	if err != nil {
		return "", "", err
	}
	return newID, register.PartyText(reg, newID), nil
}

// apiParties answers the supplier picker on the delivery screens and on the
// money screens alike. It returns names and identifiers, nothing else.
func (s *Server) apiParties(w http.ResponseWriter, r *http.Request) {
	out := []valueSuggestion{}
	collect := func(reg *register.Register) {
		rows := register.MatchParties(reg, r.URL.Query().Get("q"))
		if len(rows) > maxSuggestions {
			rows = rows[:maxSuggestions]
		}
		for _, p := range rows {
			out = append(out, valueSuggestion{ID: p.ID, Value: p.Name, Label: p.Name})
		}
	}
	if sess := financeSessionOf(r); sess != nil {
		// ReadBoth imports old encrypted FinanceParty rows into a copy. This
		// keeps every existing suggestion usable immediately after login,
		// before the first schema-5 financial write persists the migration.
		_ = s.st.ReadBoth(sess.vaultKey, func(reg *register.Register, _ *register.FinanceData) { collect(reg) })
	} else {
		s.st.Read(collect)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
