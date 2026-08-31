package web

import (
	"net/http"
	"strings"
	"time"

	"storeregister/internal/register"
)

// onDuty is the one place the program asks who is at the desk. A shift is live
// only on its own calendar day: the laptop is shut at night and opened the next
// morning by whoever arrives first, and yesterday's name must not end up on
// today's entries. Nothing is cleared from the file - a stale OnDutyStaffID is
// simply ignored, and the next start overwrites it.
func (s *Server) onDuty(reg *register.Register) (register.Staff, bool) {
	if reg.OnDutyStaffID == "" || reg.ShiftStartedAt == nil {
		return register.Staff{}, false
	}
	if !sameDay(*reg.ShiftStartedAt, s.now()) {
		return register.Staff{}, false
	}
	for _, st := range reg.Staff {
		if st.ID == reg.OnDutyStaffID {
			return st, true
		}
	}
	return register.Staff{}, false
}

// sameDay compares two instants by the calendar day they fall on in the
// server's local timezone, which is the desk's own day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.In(time.Local).Date()
	by, bm, bd := b.In(time.Local).Date()
	return ay == by && am == bm && ad == bd
}

// shiftOption is one tappable name on the arrival screen.
type shiftOption struct {
	ID     string
	Name   string
	Mobile string
	On     bool
}

type shiftData struct {
	Options   []shiftOption
	Empty     bool
	OpenAdder bool
}

// shiftScreen is the walkthrough's arrival screen: no tabs, 26rem wide.
func (s *Server) shiftScreen(w http.ResponseWriter, r *http.Request) {
	s.renderShift(w, http.StatusOK, "", nil)
}

// renderShift draws the arrival screen with an optional banner and an optional
// pre-selected person. selected == "" means the person already on duty, if any.
func (s *Server) renderShift(w http.ResponseWriter, status int, selected string, b *banner) {
	var data shiftData
	s.st.Read(func(reg *register.Register) {
		if selected == "" {
			selected = reg.OnDutyStaffID
		}
		for _, st := range reg.Staff {
			data.Options = append(data.Options, shiftOption{
				ID: st.ID, Name: st.Name, Mobile: st.Mobile, On: st.ID == selected,
			})
		}
	})
	data.Empty = len(data.Options) == 0
	data.OpenAdder = data.Empty

	p := page{Title: "Store Register", Tabs: false, Narrow: true, Banner: b}
	s.render(w, status, p, "shift.html", data)
}

// shiftStart puts a name on everything entered from now on.
func (s *Server) shiftStart(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("staffId"))

	known := false
	s.st.Read(func(reg *register.Register) {
		for _, st := range reg.Staff {
			if st.ID == id {
				known = true
			}
		}
	})
	if !known {
		s.renderShift(w, http.StatusOK, "", &banner{"bad", "Tap your name first."})
		return
	}

	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		reg.OnDutyStaffID = id
		reg.ShiftStartedAt = &now
		return nil
	})
	if err != nil {
		s.renderShift(w, http.StatusOK, "", &banner{"bad", saveFailed})
		return
	}
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

// shiftPerson adds a person to the list. A duplicate name is refused here
// because the list is a short one people read; duplicate people in the entries
// themselves are never blocked.
func (s *Server) shiftPerson(w http.ResponseWriter, r *http.Request) {
	name := register.CleanName(r.FormValue("name"))
	mobile := register.CleanName(r.FormValue("mobile"))

	if name == "" {
		s.renderShift(w, http.StatusOK, "", &banner{"bad", "Type the person's name."})
		return
	}

	var clash string
	s.st.Read(func(reg *register.Register) {
		for _, st := range reg.Staff {
			if register.FoldKey(st.Name) == register.FoldKey(name) {
				clash = st.Name
			}
		}
	})
	if clash != "" {
		s.renderShift(w, http.StatusOK, "", &banner{"bad", clash + " is already on the list."})
		return
	}

	// CreatedBy is the on-duty person's name, and is legitimately empty for the
	// first person on a fresh register: nobody was at the desk to add them. No
	// placeholder is substituted.
	var addedBy, newID string
	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			addedBy = who.Name
		}
	})

	now := s.now()
	err := s.st.Update(func(reg *register.Register) error {
		newID = reg.NextID("STF")
		reg.Staff = append(reg.Staff, register.Staff{
			ID: newID, Name: name, Mobile: mobile,
			CreatedAt: now, CreatedBy: addedBy,
		})
		return nil
	})
	if err != nil {
		s.renderShift(w, http.StatusOK, "", &banner{"bad", saveFailed})
		return
	}
	s.renderShift(w, http.StatusOK, newID, &banner{"ok", name + " added."})
}

// saveFailed is the one sentence shown when the file could not be written. The
// entry is not in the register and the person at the desk has to know that.
const saveFailed = "Nothing was saved. The register file could not be written. Tell somebody before entering more."
