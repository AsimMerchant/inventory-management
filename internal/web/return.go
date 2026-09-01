package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// outRow is one thing the person in front of the desk is still holding.
type outRow struct {
	register.OutstandingLine
	Heading string
	Pick    bool   // every row of the picked product, because a chair is a chair
	Href    string // tapping the row picks that product
}

// returnData is everything return.html draws.
type returnData struct {
	Now      time.Time
	Find     personPicker
	Searched bool // something was typed
	Nobody   bool // and nobody is holding anything under it

	Person    bool
	Name      string
	Mobile    string
	Dept      string
	Q         string // what the form carries forward to find this person again
	Rows      []outRow
	TakerName string
	StillVerb string

	Picked         bool
	ProductID      string
	ProductName    string
	IssueIDs       []string
	HoldingIssueID string
	TotalOut       int
	Quantity       string
	Short          int

	ReturnerName   string
	ReturnerMobile string
	ReturnedAt     string
	TakenBackBy    string
	Disposition    string
	Remark         string
	ButtonLabel    string
	Stale          bool
}

// returnNew is the form and the save.
func (s *Server) returnNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.returnSave(w, r)
		return
	}
	data := s.returnForm(r)
	p := s.returnPage()
	if data.Stale {
		p.add(&banner{"bad", "That holding has changed. Pick it again from the list."})
	}
	s.render(w, http.StatusOK, p, "return.html", data)
}

func (s *Server) returnPage() page {
	p := s.page("Someone is returning")
	p.Tabs = false
	return p
}

// returnForm finds the person, lays out what they are holding and fills the
// quantity in with the full outstanding amount. The person at the desk is never
// asked to know how many were taken.
func (s *Server) returnForm(r *http.Request) returnData {
	_ = r.ParseForm()
	now := s.now()

	q := register.CleanName(r.FormValue("q"))
	productID := formProductID(r)
	holdingIssueID := strings.TrimSpace(r.FormValue("holdingIssueId"))
	quantity := strings.TrimSpace(r.FormValue("quantity"))

	data := returnData{
		Now:            now,
		Q:              q,
		Searched:       q != "",
		Quantity:       quantity,
		HoldingIssueID: holdingIssueID,
		StillVerb:      "has",
		ReturnerName:   register.CleanName(r.FormValue("returnerName")),
		ReturnerMobile: register.CleanName(r.FormValue("returnerMobile")),
		ReturnedAt:     strings.TrimSpace(r.FormValue("returnedAt")),
		Disposition:    strings.TrimSpace(r.FormValue("disposition")),
		Remark:         register.CleanName(r.FormValue("remark")),
	}
	if data.ReturnedAt == "" {
		data.ReturnedAt = now.Format(stampLayout)
	}

	s.st.Read(func(reg *register.Register) {
		if who, ok := s.onDuty(reg); ok {
			data.TakenBackBy = who.Name
		}

		found := register.FindPeople(reg, q)
		if holdingIssueID != "" {
			if _, ok := register.JointHoldingForIssue(reg, holdingIssueID); !ok {
				data.Stale = true
			}
		}
		// Somebody who has never taken anything has nothing to bring back, so
		// this picker never offers to make a new person.
		data.Find = personPicker{
			Label: "Find the person", Field: "q", Typed: q,
			Hint: "Search by name, mobile or department.", Autofocus: true,
		}

		switch {
		case len(found) == 1:
			s.fillHolding(reg, &data, found[0], holdingIssueID, productID)
		case len(found) == 0:
			data.Nobody = data.Searched
		default:
			data.Find.Rows = suggestPeople(found)
		}
	})

	data.ButtonLabel = returnButtonLabel(data.ProductName, data.Quantity)
	return data
}

// fillHolding lays out one person's outstanding lines and, when a product has
// been tapped, the quantity block that goes with it.
func (s *Server) fillHolding(reg *register.Register, data *returnData, p register.PersonSummary, holdingIssueID, productID string) {
	data.Person = true
	data.Name, data.Mobile, data.Dept = p.Name, p.Mobile, p.Department
	data.TakerName = p.Name
	if data.Q == "" {
		data.Q = personQuery(p)
	}

	// The lines are grouped by product, each group in the order the issues were
	// made, because selection is by product: tapping one chair line picks every
	// chair line, and rows that move together read together. Within a group the
	// oldest issue is first, which is also the order the return fills them.
	var selected register.JointHolding
	holdings := register.JointHoldings(reg)
	personHoldings := 0
	for _, holding := range holdings {
		for _, recipient := range holding.Recipients {
			if register.PersonOf(recipient.Name, recipient.Mobile) == p.ID {
				personHoldings++
				break
			}
		}
	}
	if holdingIssueID == "" && productID != "" {
		for _, holding := range holdings {
			if len(holding.Recipients) != 1 || register.PersonOf(holding.Recipients[0].Name, holding.Recipients[0].Mobile) != p.ID {
				continue
			}
			for _, line := range holding.Lines {
				if line.ProductID == productID {
					holdingIssueID, data.HoldingIssueID = holding.AnchorIssueID, holding.AnchorIssueID
					break
				}
			}
		}
	}
	for _, holding := range holdings {
		member := false
		for _, recipient := range holding.Recipients {
			if register.PersonOf(recipient.Name, recipient.Mobile) == p.ID {
				member = true
			}
		}
		if !member {
			continue
		}
		context := ""
		if len(holding.Recipients) > 1 {
			context = " - holding together"
		} else if personHoldings > 1 {
			context = " - holding alone"
		}
		for i, l := range inProductGroups(holding.Lines) {
			heading := ""
			if i == 0 && context != "" {
				heading = holding.Label + context
			}
			data.Rows = append(data.Rows, outRow{OutstandingLine: l, Heading: heading, Pick: holding.AnchorIssueID == holdingIssueID && l.ProductID == productID, Href: "/return/new?q=" + url.QueryEscape(data.Q) + "&holdingIssueId=" + holding.AnchorIssueID + "&productId=" + l.ProductID})
		}
		if holding.AnchorIssueID == holdingIssueID {
			selected = holding
		}
	}
	if holdingIssueID != "" && selected.AnchorIssueID == "" {
		data.Stale = true
		return
	}
	for _, h := range register.HoldingByProduct(selected.Lines) {
		if h.ProductID == productID {
			data.Picked = true
			data.ProductID, data.ProductName, data.TotalOut = h.ProductID, h.ProductName, h.Out
		}
	}
	if selected.Label != "" {
		data.TakerName = selected.Label
		if len(selected.Recipients) > 1 {
			data.StillVerb = "have"
		}
	}
	if !data.Picked {
		return
	}

	for _, l := range selected.Lines {
		if l.ProductID == productID {
			data.IssueIDs = append(data.IssueIDs, l.IssueID)
		}
	}
	// The box arrives already filled with everything that is out. Imran only
	// changes it when fewer come back than went out.
	if data.Quantity == "" {
		data.Quantity = strconv.Itoa(data.TotalOut)
	}
	if data.ReturnerName == "" {
		data.ReturnerName, data.ReturnerMobile = p.Name, p.Mobile
	}
	if n, err := wholeNumber(data.Quantity); err == nil && n < data.TotalOut {
		data.Short = data.TotalOut - n
	}
}

// inProductGroups keeps the oldest-first order inside each product and puts the
// products in the order their oldest line appears.
func inProductGroups(lines []register.OutstandingLine) []register.OutstandingLine {
	var order []string
	grouped := map[string][]register.OutstandingLine{}
	for _, l := range lines {
		if _, seen := grouped[l.ProductID]; !seen {
			order = append(order, l.ProductID)
		}
		grouped[l.ProductID] = append(grouped[l.ProductID], l)
	}
	out := make([]register.OutstandingLine, 0, len(lines))
	for _, id := range order {
		out = append(out, grouped[id]...)
	}
	return out
}

// personQuery is what finds this person again on the next page: the mobile if
// there is one, because two people share a name and no two share a number.
func personQuery(p register.PersonSummary) string {
	if p.Mobile != "" {
		return p.Mobile
	}
	return p.Name
}

// suggestPeople renders the rows for a search that matched several people, each
// a link that picks that one.
func suggestPeople(found []register.PersonSummary) []personRow {
	rows := make([]personRow, 0, len(found))
	for _, p := range found {
		rows = append(rows, personRow{
			Name: p.Name, Mobile: p.Mobile, Department: p.Department,
			Label: personLabel(p.Name, p.Mobile, p.Department),
			Href:  "/return/new?q=" + url.QueryEscape(personQuery(p)),
		})
	}
	return rows
}

func returnButtonLabel(productName, quantity string) string {
	n, err := wholeNumber(quantity)
	if productName == "" || err != nil {
		return "Take back"
	}
	return "Take back " + strconv.Itoa(n) + " " + productWord(productName)
}

// returnSave validates in the order the spec lists, then writes the return with
// the quantity spread across the issues oldest first.
func (s *Server) returnSave(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	data := s.returnForm(r)

	refuse := func(text string) {
		p := s.returnPage()
		p.add(&banner{"bad", text})
		s.render(w, http.StatusOK, p, "return.html", data)
	}

	issueIDs := r.Form["issueIds"]
	holdingIssueID := strings.TrimSpace(r.FormValue("holdingIssueId"))
	quantity := strings.TrimSpace(r.FormValue("quantity"))
	returnerName := register.CleanName(r.FormValue("returnerName"))
	returnerMobile := register.CleanName(r.FormValue("returnerMobile"))
	disposition := strings.TrimSpace(r.FormValue("disposition"))
	remark := register.CleanName(r.FormValue("remark"))

	var plan register.ReturnPlan
	var refusal string
	s.st.Read(func(reg *register.Register) {
		plan, refusal = planReturnForHolding(reg, holdingIssueID, issueIDs, quantity, returnerName, disposition, remark)
	})
	if refusal != "" {
		refuse(refusal)
		return
	}

	returnedAt, err := time.ParseInLocation(stampLayout, strings.TrimSpace(r.FormValue("returnedAt")), time.Local)
	if err != nil {
		refuse("Type the time like this: 18:05.")
		return
	}

	now := s.now()
	var newID string
	err = s.st.Update(func(reg *register.Register) error {
		// Re-run against the register as it is at this instant, not against the
		// numbers the page was drawn with: two tabs must not return the same
		// fifty chairs twice.
		plan, refusal = planReturnForHolding(reg, holdingIssueID, issueIDs, quantity, returnerName, disposition, remark)
		if refusal != "" {
			return errRefused
		}
		newID = reg.NextID("RET")
		entry := register.Return{
			ID: newID, ProductID: plan.ProductID, Allocations: plan.Allocations,
			ReturnerName: returnerName, ReturnerMobile: returnerMobile,
			TakenBackBy: data.TakenBackBy,
			ReturnedAt:  returnedAt, RecordedAt: now,
			ShortQuantity: plan.Short,
		}
		// A shortfall is a note on the record, never a stock movement: the five
		// chairs stay out against the issue and against the taker's name.
		if plan.Short > 0 {
			entry.ShortDisposition = register.Disposition(disposition)
			entry.Remark = remark
		}
		reg.Returns = append(reg.Returns, entry)
		return nil
	})
	switch {
	case errors.Is(err, errRefused):
		data = s.returnForm(r)
		refuse(refusal)
		return
	case err != nil:
		refuse(saveFailed)
		return
	}
	http.Redirect(w, r, "/stock?saved="+newID, http.StatusSeeOther)
}

func planReturnForHolding(reg *register.Register, anchor string, postedIssueIDs []string, quantity, returnerName, disposition, remark string) (register.ReturnPlan, string) {
	// Old schema-1 pages and stakeholder bookmarks did not carry an anchor.
	// Keep those one-person posts working exactly as before; every newly drawn
	// form carries an anchor and receives the stronger holding-boundary guard.
	if anchor == "" {
		return planReturn(reg, postedIssueIDs, quantity, returnerName, disposition, remark)
	}
	holding, ok := register.JointHoldingForIssue(reg, anchor)
	if !ok {
		return register.ReturnPlan{}, "That holding has changed. Pick it again from the list."
	}
	productID := ""
	for _, is := range register.LiveIssues(reg) {
		for _, id := range postedIssueIDs {
			if is.ID == id {
				productID = is.ProductID
			}
		}
	}
	var issueIDs []string
	for _, line := range holding.Lines {
		if line.ProductID == productID {
			issueIDs = append(issueIDs, line.IssueID)
		}
	}
	if len(issueIDs) != len(postedIssueIDs) {
		return register.ReturnPlan{}, "That holding has changed. Pick it again from the list."
	}
	for i := range issueIDs {
		if issueIDs[i] != postedIssueIDs[i] {
			return register.ReturnPlan{}, "That holding has changed. Pick it again from the list."
		}
	}
	return planReturn(reg, issueIDs, quantity, returnerName, disposition, remark)
}

// planReturn is every refusal on the returning screen, in order, and the plan
// that survives them. It is called once for the sentence the desk reads and
// again inside the save, against whatever the register says by then.
func planReturn(reg *register.Register, issueIDs []string, quantity, returnerName, disposition, remark string) (register.ReturnPlan, string) {
	tap := "Tap the row for the thing they are bringing back."
	if !register.SameProduct(reg, issueIDs) {
		return register.ReturnPlan{}, tap
	}

	// The taker is the person the stock was issued to, read off the issues -
	// never the person carrying it back to the desk.
	outstanding := register.PlanReturn(reg, issueIDs, 0)
	word := productWord(outstanding.ProductName)

	n, err := wholeNumber(quantity)
	if err != nil {
		return outstanding, "Type how many " + word + " are coming back."
	}
	if err := register.CheckReturn(reg, issueIDs, n); err != nil {
		out := strconv.Itoa(outstanding.Out)
		return outstanding, outstanding.TakerName + " has " + out + " " + word +
			". You cannot take back more than " + out + "."
	}
	if returnerName == "" {
		return outstanding, "Who is handing it back? Type their name."
	}

	plan := register.PlanReturn(reg, issueIDs, n)
	if plan.Short > 0 {
		short := strconv.Itoa(plan.Short)
		if disposition != string(register.ExpectedBack) && disposition != string(register.WontComeBack) {
			return plan, "Tap one: the " + short + " " + word +
				" are coming back later, or they are gone."
		}
		if remark == "" {
			return plan, "Write what happened to the " + short + " " + word + "."
		}
	}
	return plan, ""
}
