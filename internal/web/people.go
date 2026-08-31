package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"storeregister/internal/register"
)

// personLine is one thing a person is still holding, as the picker reports it.
type personLine struct {
	IssueID     string    `json:"issueId"`
	ProductID   string    `json:"productId"`
	ProductName string    `json:"productName"`
	Taken       int       `json:"taken"`
	Back        int       `json:"back"`
	Out         int       `json:"out"`
	IssuedAt    time.Time `json:"issuedAt"`
	IssuedBy    string    `json:"issuedBy"`
}

// personRow is one suggestion row. The last row of the list is the offer to
// make a new person, and carries New instead of a person.
type personRow struct {
	Name       string       `json:"name"`
	Mobile     string       `json:"mobile"`
	Department string       `json:"department"`
	TotalOut   int          `json:"totalOut"`
	Label      string       `json:"label"`
	New        bool         `json:"new,omitempty"`
	Href       string       `json:"-"` // set only where a row is a link
	Lines      []personLine `json:"lines"`
}

// apiPeople is the one person-finder. The issue screen, the returning screen,
// the "Out with people" view and the activity log all call it and all draw the
// same rows. scope=log filters a read-only list, so it never offers to make
// somebody new.
func (s *Server) apiPeople(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	scope := r.URL.Query().Get("scope")

	var rows []personRow
	s.st.Read(func(reg *register.Register) {
		rows = findPeople(reg, q, scope == "log")
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

// findPeople is FindPeople plus the offer at the bottom of the list. It offers
// and never insists: two spellings of one man are two rows, nobody is warned,
// nothing is blocked. That is the opposite of the product picker, deliberately.
func findPeople(reg *register.Register, query string, logScope bool) []personRow {
	rows := []personRow{}
	// The activity log searches a wider cast: staff, and everybody who has
	// handed everything back already. It offers nobody new, because nothing is
	// created from a read-only page.
	found := register.FindPeople(reg, query)
	if logScope {
		found = register.FindPeopleInLog(reg, query)
	}
	for _, p := range found {
		rows = append(rows, personRow{
			Name:       p.Name,
			Mobile:     p.Mobile,
			Department: p.Department,
			TotalOut:   p.TotalOut,
			Label:      personLabel(p.Name, p.Mobile, p.Department),
			Lines:      linesOf(p.Lines),
		})
	}
	if !logScope && offersNewPerson(query) {
		typed := register.CleanName(query)
		rows = append(rows, personRow{New: true, Name: typed,
			Label: "+ New person named " + typed})
	}
	return rows
}

// offersNewPerson hides the last row when there is nothing to name it after.
// A bare mobile number is not a name: "+ New person named 98861 40023" reads
// oddly and would put a number in the name column for good.
func offersNewPerson(query string) bool {
	typed := register.CleanName(query)
	if typed == "" {
		return false
	}
	return register.MobileKey(typed) != strings.ReplaceAll(typed, " ", "")
}

// personLabel is the suggestion row: name, then the mobile that tells two
// people of one name apart, then the department.
func personLabel(name, mobile, department string) string {
	parts := []string{name}
	if mobile != "" {
		parts = append(parts, mobile)
	}
	if department != "" {
		parts = append(parts, department)
	}
	return strings.Join(parts, " · ")
}

func linesOf(in []register.OutstandingLine) []personLine {
	out := make([]personLine, 0, len(in))
	for _, l := range in {
		out = append(out, personLine{
			IssueID: l.IssueID, ProductID: l.ProductID, ProductName: l.ProductName,
			Taken: l.Taken, Back: l.Back, Out: l.Out,
			IssuedAt: l.IssuedAt, IssuedBy: l.IssuedBy,
		})
	}
	return out
}

// knownPerson is the one silent kindness. When what was typed is exactly one
// person's name, their mobile and department are already known and there is no
// reason to make the desk type them again - or to let the same man become two
// rows through a hurried second entry. The match here is exact, not the loose
// substring the suggestion list uses: typing "Ravi" must never stamp one Ravi's
// mobile onto another man's entry.
func knownPerson(reg *register.Register, name string) (register.PersonSummary, bool) {
	want := register.FoldKey(name)
	if want == "" {
		return register.PersonSummary{}, false
	}
	var found register.PersonSummary
	n := 0
	for _, p := range register.PeopleHolding(reg) {
		if register.FoldKey(p.Name) == want {
			found = p
			n++
		}
	}
	return found, n == 1
}

// personPicker is what person-picker.html draws. Rows are server-rendered on
// the returning screen, where the whole flow works without JavaScript; on the
// issue screen they are drawn by person-picker.js from /api/people.
type personPicker struct {
	Label     string
	Field     string
	Typed     string
	Hint      string
	HintKind  string // "", "good", "bad"
	Autofocus bool
	AllowNew  bool
	Scope     string
	Rows      []personRow
}
