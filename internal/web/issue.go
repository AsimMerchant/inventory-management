package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// stampLayout is what an <input type="datetime-local"> sends and expects.
const stampLayout = "2006-01-02T15:04"

// errRefused says a save was turned down inside the store's own lock, where
// the register is what it is at that instant rather than what the page said.
var errRefused = errors.New("refused")

// issueData is everything issue.html draws.
type issueData struct {
	Now         time.Time
	Picker      pickerData
	OnHand      int
	Quantity    string
	ChallanNo   string
	Taker       personPicker
	Department  string
	Mobile      string
	Additional  []additionalTakerData
	IssuedAt    string
	Incharge    string
	InchargeMob string
	Warn        string // the amber line, "" when the taker holds nothing
	ButtonLabel string
}

type additionalTakerData struct {
	Picker     personPicker
	Department string
	Mobile     string
	Index      int
}

// issueNew is the form and the save. The same fields are read from a GET and
// from a refused POST, so the page a person sees after a refusal is drawn by
// exactly the code that drew it the first time.
func (s *Server) issueNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.FormValue("addPerson") != "" || r.FormValue("removePerson") != "" {
			s.render(w, http.StatusOK, s.issuePage(r), "issue.html", s.issueForm(r))
			return
		}
		s.issueSave(w, r)
		return
	}
	s.render(w, http.StatusOK, s.issuePage(r), "issue.html", s.issueForm(r))
}

func (s *Server) issuePage(r *http.Request) page {
	p := s.page("Someone is taking", r)
	p.Tabs = false
	return p
}

// issueForm reads the fields and fills in what the register already knows.
func (s *Server) issueForm(r *http.Request) issueData {
	_ = r.ParseForm()
	now := s.now()

	productID := formProductID(r)
	quantity := strings.TrimSpace(r.FormValue("quantity"))
	takerName := register.CleanName(r.FormValue("takerName"))
	department := register.CleanName(r.FormValue("takerDepartment"))
	mobile := register.CleanName(r.FormValue("takerMobile"))
	issuedAt := strings.TrimSpace(r.FormValue("issuedAt"))
	if issuedAt == "" {
		issuedAt = now.Format(stampLayout)
	}

	data := issueData{Now: now, Quantity: quantity, ChallanNo: register.CleanName(r.FormValue("challanNo")), IssuedAt: issuedAt}
	names := append([]string(nil), r.Form["additionalTakerName"]...)
	departments := append([]string(nil), r.Form["additionalTakerDepartment"]...)
	mobiles := append([]string(nil), r.Form["additionalTakerMobile"]...)
	count := len(names)
	if len(departments) > count {
		count = len(departments)
	}
	if len(mobiles) > count {
		count = len(mobiles)
	}
	if r.FormValue("addPerson") != "" {
		count++
	}
	if raw := r.FormValue("removePerson"); raw != "" {
		if remove, err := strconv.Atoi(raw); err == nil && remove >= 0 && remove < count {
			names = removeStringAt(names, remove)
			departments = removeStringAt(departments, remove)
			mobiles = removeStringAt(mobiles, remove)
			count--
		}
	}
	for i := 0; i < count; i++ {
		additional := additionalTakerData{Index: i, Picker: personPicker{Label: "Another person taking it", Field: "additionalTakerName", AllowNew: true}}
		if i < len(names) {
			additional.Picker.Typed = register.CleanName(names[i])
		}
		if i < len(departments) {
			additional.Department = register.CleanName(departments[i])
		}
		if i < len(mobiles) {
			additional.Mobile = register.CleanName(mobiles[i])
		}
		data.Additional = append(data.Additional, additional)
	}
	s.st.Read(func(reg *register.Register) {
		data.Picker = s.picker(reg, "instock", false, productID)
		if data.Picker.PickedID != "" {
			data.OnHand = register.OnHand(reg, data.Picker.PickedID)
		}
		if who, ok := s.onDuty(reg); ok {
			data.Incharge, data.InchargeMob = who.Name, who.Mobile
		}

		hint := ""
		if known, only := knownPerson(reg, takerName); only {
			hint = "This person has taken things before. Their details are filled in."
			if mobile == "" {
				mobile = known.Mobile
				if department == "" {
					department = known.Department
				}
			}
		}
		data.Taker = personPicker{
			Label: "Who is taking it", Field: "takerName", Typed: takerName,
			Hint: hint, AllowNew: true,
		}
		data.Department, data.Mobile = department, mobile
		data.Warn = holdingWarning(reg, takerName, mobile, now)
	})

	buttonNames := []string{takerName}
	for _, additional := range data.Additional {
		buttonNames = append(buttonNames, additional.Picker.Typed)
	}
	data.ButtonLabel = issueButtonLabel(data.Picker.PickedName, quantity, buttonNames...)
	return data
}

func removeStringAt(values []string, at int) []string {
	if at < 0 || at >= len(values) {
		return values
	}
	return append(values[:at], values[at+1:]...)
}

// holdingWarning is the amber line: what this person already has of yours. It
// is information and not an obstacle - there is nothing to dismiss and nothing
// to confirm. The person is matched on name and mobile together, so one Ravi
// Kumar's holdings are never shown while stock goes to the other.
func holdingWarning(reg *register.Register, name, mobile string, now time.Time) string {
	if register.CleanName(name) == "" {
		return ""
	}
	want := register.PersonOf(name, mobile)
	for _, p := range register.PeopleHolding(reg) {
		if p.ID != want {
			continue
		}
		var clauses []string
		for _, h := range register.HoldingByProduct(p.Lines) {
			clauses = append(clauses, strconv.Itoa(h.Out)+" "+productWord(h.ProductName))
		}
		today := true
		for _, l := range p.Lines {
			if !sameDay(l.IssuedAt, now) {
				today = false
			}
		}
		sentence := p.Name + " is already holding " + joinClauses(clauses)
		if today {
			sentence += " from earlier today"
		}
		return sentence + "."
	}
	return ""
}

// joinClauses reads a list the way a person says it: "40 chairs and 2 round
// tables", "120 chairs, 25 extension boards and 3 water drums".
func joinClauses(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// issueButtonLabel is "Issue 10 chairs to Ravi": the first name only, because
// that is how the sentence is said out loud at the desk.
func issueButtonLabel(productName, quantity string, takerNames ...string) string {
	n, err := strconv.Atoi(strings.TrimSpace(quantity))
	firsts := make([]string, 0, len(takerNames))
	for _, name := range takerNames {
		if first := firstWord(name); first != "" {
			firsts = append(firsts, first)
		}
	}
	if productName == "" || err != nil || n < 1 || len(firsts) != len(takerNames) || len(firsts) == 0 {
		return "Issue"
	}
	return "Issue " + strconv.Itoa(n) + " " + productWord(productName) + " to " + joinClauses(firsts)
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// issueSave validates in the order the spec lists and writes nothing until
// every field has passed.
func (s *Server) issueSave(w http.ResponseWriter, r *http.Request) {
	data := s.issueForm(r)

	refuse := func(text string) {
		p := s.issuePage(r)
		p.add(&banner{"bad", text})
		s.render(w, http.StatusOK, p, "issue.html", data)
	}

	productID, name := data.Picker.PickedID, data.Picker.PickedName
	if productID == "" || name == "" {
		refuse("Pick the product from the list.")
		return
	}
	n, err := wholeNumber(data.Quantity)
	if err != nil {
		refuse("Type how many " + productWord(name) + " they are taking.")
		return
	}

	// Checked here for the order the sentences appear in, and again inside the
	// save below against the register as it is at that instant.
	var refusal string
	s.st.Read(func(reg *register.Register) {
		refusal = overIssueSentence(register.CheckIssue(reg, productID, n, s.now()), name)
	})
	if refusal != "" {
		refuse(refusal)
		return
	}

	takerName := data.Taker.Typed
	if takerName == "" {
		refuse("Who is taking it? Type their name.")
		return
	}
	if len(r.Form["additionalTakerName"]) != len(r.Form["additionalTakerDepartment"]) || len(r.Form["additionalTakerName"]) != len(r.Form["additionalTakerMobile"]) {
		refuse("Type the name of every person taking it.")
		return
	}
	additional := make([]register.IssueRecipient, 0, len(data.Additional))
	for _, taker := range data.Additional {
		if taker.Picker.Typed == "" {
			refuse("Type the name of every person taking it.")
			return
		}
		additional = append(additional, register.IssueRecipient{Name: taker.Picker.Typed, Department: taker.Department, Mobile: taker.Mobile})
	}
	issuedAt, err := time.ParseInLocation(stampLayout, data.IssuedAt, time.Local)
	if err != nil {
		refuse("Type the time like this: 14:18.")
		return
	}

	// PersonInchargeName and PersonInchargeMobile come from the shift, never
	// from the form: a request that supplies them is ignored.
	incharge, inchargeMobile := data.Incharge, data.InchargeMob

	now := s.now()
	var newID string
	err = s.st.Update(func(reg *register.Register) error {
		if refusal = overIssueSentence(register.CheckIssue(reg, productID, n, now), name); refusal != "" {
			return errRefused
		}
		newID = reg.NextID("ISS")
		reg.Issues = append(reg.Issues, register.Issue{
			ID: newID, ProductID: productID, Quantity: n,
			ChallanNo: register.CleanName(r.FormValue("challanNo")),
			TakerName: takerName, TakerDepartment: data.Department,
			TakerMobile:        data.Mobile,
			AdditionalTakers:   additional,
			PersonInchargeName: incharge, PersonInchargeMobile: inchargeMobile,
			IssuedAt: issuedAt, RecordedAt: now,
		})
		return nil
	})
	switch {
	case errors.Is(err, errRefused):
		data = s.issueForm(r) // the numbers on the page were stale; draw them again
		refuse(refusal)
		return
	case err != nil:
		refuse(saveFailed)
		return
	}
	http.Redirect(w, r, "/stock?saved="+newID, http.StatusSeeOther)
}

// overIssueSentence turns a refusal from CheckIssue into the sentence at the
// desk, or "" when the quantity was allowed. The product's own name carries its
// plural, so a quantity of one never produces "Only 1 chairs are on hand".
func overIssueSentence(err error, name string) string {
	var q register.QuantityError
	if !errors.As(err, &q) {
		return ""
	}
	if q.Allowed == 0 {
		return "There are no " + productWord(name) + " left."
	}
	allowed := strconv.Itoa(q.Allowed)
	return "You have " + allowed + " " + productWord(name) +
		". You cannot give out more than " + allowed + "."
}
