package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"storeregister/internal/register"
)

// This file is the whole of fixing a wrong entry: the words a correction is
// written in, and the two screens that make one.
//
// Every guard here is the same guard. A change is applied to the register
// inside one store.Update closure, register.Validate is asked whether the
// result is a register the program could have produced, and a closure that
// returns an error leaves neither the file nor the register in memory changed
// (see store.Update). The sentences below only phrase a problem; they never
// decide one. The single exception is named and explained at
// orphanedAllocations.

// ---------------------------------------------------------------------------
// The words
// ---------------------------------------------------------------------------

// entryName is a record in words, as the sub-heading of the correction screen
// and the activity log both say it. It carries no time clause: both callers
// have one of their own.
func entryName(reg *register.Register, recordID string) string {
	names := productNames(reg)
	for _, in := range reg.Inwards {
		if in.ID != recordID {
			continue
		}
		what := strconv.Itoa(in.Quantity) + " " + productWord(names[in.ProductID])
		if in.Supplier == "" {
			return what + " that came in"
		}
		return what + " from " + in.Supplier
	}
	for _, is := range reg.Issues {
		if is.ID == recordID {
			return strconv.Itoa(is.Quantity) + " " + productWord(names[is.ProductID]) +
				" to " + register.RecipientLabel(is)
		}
	}
	for _, re := range reg.Returns {
		if re.ID == recordID {
			return strconv.Itoa(re.Quantity()) + " " + productWord(names[re.ProductID]) +
				" back from " + re.ReturnerName
		}
	}
	return ""
}

// productNames is the product list as a lookup. internal/web does no
// arithmetic, but it does have to write names down.
func productNames(reg *register.Register) map[string]string {
	names := make(map[string]string, len(reg.Products))
	for _, p := range reg.Products {
		names[p.ID] = p.Name
	}
	return names
}

// changePhrase is one correction in words, without the "by whom, when" that
// the activity log's own columns already carry. word is the product's name as
// it reads in the middle of a sentence: the quantity phrase carries the noun
// with both numbers, and the stored Change holds bare figures.
func changePhrase(c register.Change, word string) string {
	switch {
	case c.To == "":
		return "Removed the " + changeNoun(c) + " that said: " + c.From
	case c.From == "":
		return "Added a " + changeNoun(c) + ": " + c.To
	}
	switch c.Field {
	case "quantity":
		return "Changed it from " + c.From + " " + word + " to " + c.To + " " + word
	case "supplier":
		return "Changed the supplier from " + c.From + " to " + c.To
	case "dateReceived":
		return "Changed the date from " + c.From + " to " + c.To
	case "basis":
		return "Changed this from " + strings.ToLower(c.From) + " to " + strings.ToLower(c.To)
	case "challan":
		return "Changed the challan no. from " + c.From + " to " + c.To
	case "taker", "returner":
		return "Changed who took it from " + c.From + " to " + c.To
	case "recipients":
		return "Changed who took it from " + c.From + " to " + c.To
	case "recipientDetails":
		return "Changed their details from " + c.From + " to " + c.To
	case "department":
		return "Changed the department from " + c.From + " to " + c.To
	case "mobile":
		return "Changed the mobile from " + c.From + " to " + c.To
	case "time":
		return "Changed the time from " + c.From + " to " + c.To
	case "remark":
		return "Changed the remark from " + c.From + " to " + c.To
	case "disposition":
		return "Changed what happened from " + c.From + " to " + c.To
	}
	return "Changed the " + strings.ToLower(c.Label) + " from " + c.From + " to " + c.To
}

// changeNoun is what a cleared or newly written field is called in a sentence
// that names its old or new contents.
func changeNoun(c register.Change) string {
	switch c.Field {
	case "remark":
		return "note"
	case "disposition":
		return "note about what happened"
	}
	return strings.ToLower(c.Label)
}

// changeLine is changePhrase with the attribution the lists carry.
func changeLine(c register.Change, word string) string {
	return changePhrase(c, word) + " by " + c.By + ", " + clock(c.At)
}

// deletionLine is the tombstone as the lists show it.
func deletionLine(d register.Deletion) string {
	return "Deleted by " + d.By + ", " + clock(d.At) + " — " + d.Reason
}

// deletedNotice is what the correction screen says instead of a form.
func deletedNotice(d register.Deletion) string {
	return "Deleted by " + d.By + ", " + clock(d.At) + ". Enter it again if that was a mistake."
}

// changeLines renders a record's corrections, oldest first.
func changeLines(changes []register.Change, word string) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, changeLine(c, word))
	}
	return out
}

// ---------------------------------------------------------------------------
// The record being corrected
// ---------------------------------------------------------------------------

// editData is everything edit.html draws.
type editData struct {
	ID          string
	Kind        string // "inward" | "issue" | "return"
	Heading     string
	Subheading  string
	ProductName string
	Word        string // the product name in the middle of a sentence
	From        string // the tab to go back to: /inwards or /out

	Quantity   string
	ReceivedOn string
	Basis      string
	Supplier   string
	ChallanNo  string
	ReceivedBy string

	TakerName  string
	Department string
	Mobile     string
	Additional []register.IssueRecipient
	IssuedAt   string
	Incharge   string

	ReturnerName   string
	ReturnerMobile string
	ReturnedAt     string
	TakenBackBy    string
	Disposition    string
	Remark         string
	Short          int
	TakerOfReturn  string

	Changes     []string
	ButtonLabel string
	DeleteLabel string

	Deleted     bool
	DeletedLine string
	DeleteOpen  bool // a refused delete leaves its block open, with the reason box
}

// editForm reads a record out of the register and lays it out for the screen.
// A record that is not there gives ok == false.
func editForm(reg *register.Register, id string) (editData, bool) {
	names := productNames(reg)
	d := editData{ID: id}

	switch {
	case strings.HasPrefix(id, "INW-"):
		for _, in := range reg.Inwards {
			if in.ID != id {
				continue
			}
			d.Kind, d.From = "inward", "/inwards"
			d.Heading = "Fix what came in"
			d.ProductName = names[in.ProductID]
			d.Word = productWord(d.ProductName)
			d.Quantity = strconv.Itoa(in.Quantity)
			d.ReceivedOn = in.ReceivedOn
			d.Basis = string(in.Basis)
			d.Supplier, d.ChallanNo, d.ReceivedBy = in.Supplier, in.ChallanNo, in.ReceivedBy
			d.Subheading = entryName(reg, id) + ", received " + shortdateOf(in.ReceivedOn)
			d.Changes = changeLines(in.Changes, d.Word)
			d.finish(in.Deleted)
			return d, true
		}
	case strings.HasPrefix(id, "ISS-"):
		for _, is := range reg.Issues {
			if is.ID != id {
				continue
			}
			d.Kind, d.From = "issue", "/out"
			d.Heading = "Fix what someone took"
			d.ProductName = names[is.ProductID]
			d.Word = productWord(d.ProductName)
			d.Quantity = strconv.Itoa(is.Quantity)
			d.TakerName, d.Department, d.Mobile = is.TakerName, is.TakerDepartment, is.TakerMobile
			d.Additional = append([]register.IssueRecipient(nil), is.AdditionalTakers...)
			d.IssuedAt = is.IssuedAt.Format(stampLayout)
			d.Incharge = is.PersonInchargeName
			d.Subheading = entryName(reg, id) + ", " + clock(is.IssuedAt)
			d.Changes = changeLines(is.Changes, d.Word)
			d.finish(is.Deleted)
			return d, true
		}
	case strings.HasPrefix(id, "RET-"):
		for _, re := range reg.Returns {
			if re.ID != id {
				continue
			}
			d.Kind, d.From = "return", "/out"
			d.Heading = "Fix what came back"
			d.ProductName = names[re.ProductID]
			d.Word = productWord(d.ProductName)
			d.Quantity = strconv.Itoa(re.Quantity())
			d.ReturnerName, d.ReturnerMobile = re.ReturnerName, re.ReturnerMobile
			d.ReturnedAt = re.ReturnedAt.Format(stampLayout)
			d.TakenBackBy = re.TakenBackBy
			d.Disposition = string(re.ShortDisposition)
			d.Remark, d.Short = re.Remark, re.ShortQuantity
			d.TakerOfReturn = takerOfReturn(reg, re)
			d.Subheading = entryName(reg, id) + ", " + clock(re.ReturnedAt)
			d.Changes = changeLines(re.Changes, d.Word)
			d.finish(re.Deleted)
			return d, true
		}
	}
	return d, false
}

func (d *editData) finish(deleted *register.Deletion) {
	if deleted != nil {
		d.Deleted = true
		d.DeletedLine = deletedNotice(*deleted)
	}
	d.ButtonLabel = "Save this fix"
	d.DeleteLabel = "Yes, delete these " + d.Quantity + " " + d.Word
}

// takerOfReturn names the person the stock was issued to, live issue or not.
// The return screen reads it off the issues, never off whoever carried the
// goods to the desk.
func takerOfReturn(reg *register.Register, re register.Return) string {
	for _, a := range re.Allocations {
		for _, is := range reg.Issues {
			if is.ID == a.IssueID {
				return register.RecipientLabel(is)
			}
		}
	}
	return re.ReturnerName
}

// shortdateOf renders a stored date-only field as 3 September. An unparseable
// one is shown exactly as it is stored rather than swallowed.
func shortdateOf(stored string) string {
	t, err := time.ParseInLocation(dateLayout, stored, time.Local)
	if err != nil {
		return stored
	}
	return shortdate(t)
}

// ---------------------------------------------------------------------------
// The screens
// ---------------------------------------------------------------------------

// entryEdit is the correction form and the correction itself.
func (s *Server) entryEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.Method == http.MethodPost {
		s.entrySave(w, r, id)
		return
	}

	var data editData
	var found bool
	s.st.Read(func(reg *register.Register) {
		data, found = editForm(reg, id)
	})
	if !found {
		s.notFound(w, r)
		return
	}
	s.renderEdit(w, data, nil)
}

func (s *Server) renderEdit(w http.ResponseWriter, data editData, b *banner) {
	p := s.page("Fix an entry")
	p.Tabs = false
	p.add(b)
	s.render(w, http.StatusOK, p, "edit.html", data)
}

// entrySave applies one correction. Nothing is written until register.Validate
// has seen the result.
func (s *Server) entrySave(w http.ResponseWriter, r *http.Request, id string) {
	_ = r.ParseForm()
	now := s.now()

	var data editData
	var found bool
	var by string
	s.st.Read(func(reg *register.Register) {
		data, found = editForm(reg, id)
		if who, ok := s.onDuty(reg); ok {
			by = who.Name
		}
	})
	if !found {
		s.notFound(w, r)
		return
	}
	if data.Deleted {
		s.renderEdit(w, data, nil)
		return
	}

	// What the person typed goes back on the screen whatever happens next, so
	// nothing is typed twice.
	typed := data
	typed.readForm(r)
	if typed.Kind == "issue" && r.FormValue("addPerson") != "" {
		typed.Additional = append(typed.Additional, register.IssueRecipient{})
		s.renderEdit(w, typed, nil)
		return
	}
	if typed.Kind == "issue" && r.FormValue("removePerson") != "" {
		if at, err := strconv.Atoi(r.FormValue("removePerson")); err == nil && at >= 0 && at < len(typed.Additional) {
			typed.Additional = append(typed.Additional[:at], typed.Additional[at+1:]...)
		}
		s.renderEdit(w, typed, nil)
		return
	}

	refuse := func(text string) {
		s.renderEdit(w, typed, &banner{"bad", text})
	}

	if text := fieldRefusal(typed); text != "" {
		refuse(text)
		return
	}

	var problems []register.Problem
	var refusal string
	err := s.st.Update(func(reg *register.Register) error {
		changes, text := applyEdit(reg, typed, by, now)
		if text != "" {
			refusal = text
			return errRefused
		}
		// One checker. The change is on the register now; if Validate
		// complains, returning an error puts the register back as it was, on
		// disk and in memory both.
		if problems = register.Validate(reg); len(problems) > 0 {
			return errRefused
		}
		appendChanges(reg, id, changes)
		return nil
	})
	switch {
	case errors.Is(err, errRefused):
		if refusal == "" {
			s.st.Read(func(reg *register.Register) {
				refusal = editRefusal(reg, data, typed, problems)
			})
		}
		refuse(refusal)
		return
	case err != nil:
		refuse(saveFailed)
		return
	}
	http.Redirect(w, r, typed.From+"?fixed="+id, http.StatusSeeOther)
}

// readForm overlays what was submitted onto what the record says. The product
// is never read: moving stock from one pool to another is two corrections
// wearing one coat.
func (d *editData) readForm(r *http.Request) {
	get := func(name, current string) string {
		if _, ok := r.Form[name]; !ok {
			return current
		}
		return strings.TrimSpace(r.FormValue(name))
	}
	d.Quantity = get("quantity", d.Quantity)
	switch d.Kind {
	case "inward":
		d.ReceivedOn = get("receivedOn", d.ReceivedOn)
		d.Basis = get("basis", d.Basis)
		d.Supplier = register.CleanName(get("supplier", d.Supplier))
		d.ChallanNo = get("challanNo", d.ChallanNo)
	case "issue":
		d.TakerName = register.CleanName(get("takerName", d.TakerName))
		d.Department = register.CleanName(get("takerDepartment", d.Department))
		d.Mobile = register.CleanName(get("takerMobile", d.Mobile))
		if _, ok := r.Form["additionalTakersPresent"]; ok {
			names, departments, mobiles := r.Form["additionalTakerName"], r.Form["additionalTakerDepartment"], r.Form["additionalTakerMobile"]
			d.Additional = nil
			for i, name := range names {
				recipient := register.IssueRecipient{Name: register.CleanName(name)}
				if i < len(departments) {
					recipient.Department = register.CleanName(departments[i])
				}
				if i < len(mobiles) {
					recipient.Mobile = register.CleanName(mobiles[i])
				}
				d.Additional = append(d.Additional, recipient)
			}
		}
		d.IssuedAt = get("issuedAt", d.IssuedAt)
	case "return":
		d.ReturnerName = register.CleanName(get("returnerName", d.ReturnerName))
		d.ReturnerMobile = register.CleanName(get("returnerMobile", d.ReturnerMobile))
		d.ReturnedAt = get("returnedAt", d.ReturnedAt)
		d.Disposition = get("disposition", d.Disposition)
		d.Remark = register.CleanName(get("remark", d.Remark))
	}
	if n, err := wholeNumber(d.Quantity); err == nil {
		d.ButtonLabel = "Make it " + strconv.Itoa(n) + " " + d.Word
		d.DeleteLabel = "Yes, delete these " + strconv.Itoa(n) + " " + d.Word
	}
}

// fieldRefusal is the entry screens' own field rules, unchanged. A quantity of
// zero is not a correction, it is a deletion, and deletion has its own button.
func fieldRefusal(d editData) string {
	if _, err := wholeNumber(d.Quantity); err != nil {
		switch d.Kind {
		case "inward":
			return "How many came in? Type a number of 1 or more."
		case "issue":
			return "How many did they take? Type a number of 1 or more."
		default:
			return "How many came back? Type a number of 1 or more."
		}
	}
	switch d.Kind {
	case "inward":
		if _, err := time.Parse(dateLayout, d.ReceivedOn); err != nil {
			return "Type the date like this: 03-09-2026."
		}
		if d.Basis != string(register.Rent) && d.Basis != string(register.Purchase) {
			return "Choose rent or purchase."
		}
	case "issue":
		if d.TakerName == "" {
			return "Who is taking it? Type their name."
		}
		for _, recipient := range d.Additional {
			if recipient.Name == "" {
				return "Type the name of every person taking it."
			}
		}
		if _, err := time.ParseInLocation(stampLayout, d.IssuedAt, time.Local); err != nil {
			return "Type the time like this: 14:18."
		}
	case "return":
		if d.ReturnerName == "" {
			return "Who is handing it back? Type their name."
		}
		if _, err := time.ParseInLocation(stampLayout, d.ReturnedAt, time.Local); err != nil {
			return "Type the time like this: 18:05."
		}
	}
	return ""
}

// applyEdit writes the submitted values onto the record and returns the
// corrections that describes. The second return is a field-level refusal that
// only shows up once the numbers have been re-spread across the issues.
func applyEdit(reg *register.Register, d editData, by string, now time.Time) ([]register.Change, string) {
	var changes []register.Change
	note := func(field, label, from, to string) {
		if from == to {
			return
		}
		changes = append(changes, register.Change{
			At: now, By: by, Field: field, Label: label, From: from, To: to,
		})
	}
	qty, _ := wholeNumber(d.Quantity)

	switch d.Kind {
	case "inward":
		for i := range reg.Inwards {
			in := &reg.Inwards[i]
			if in.ID != d.ID {
				continue
			}
			note("quantity", "How many", strconv.Itoa(in.Quantity), strconv.Itoa(qty))
			note("dateReceived", "Date received", shortdateOf(in.ReceivedOn), shortdateOf(d.ReceivedOn))
			note("basis", "Rent or purchase", basisWord(in.Basis), basisWord(register.Basis(d.Basis)))
			note("supplier", "Came from", in.Supplier, d.Supplier)
			note("challan", "Challan no.", in.ChallanNo, d.ChallanNo)
			in.Quantity, in.ReceivedOn = qty, d.ReceivedOn
			in.Basis, in.Supplier, in.ChallanNo = register.Basis(d.Basis), d.Supplier, d.ChallanNo
		}
	case "issue":
		issuedAt, _ := time.ParseInLocation(stampLayout, d.IssuedAt, time.Local)
		for i := range reg.Issues {
			is := &reg.Issues[i]
			if is.ID != d.ID {
				continue
			}
			note("quantity", "How many", strconv.Itoa(is.Quantity), strconv.Itoa(qty))
			oldRecipients, newRecipients := register.RecipientsOf(*is), recipientsOfEdit(d)
			if len(oldRecipients) > 1 || len(newRecipients) > 1 {
				note("recipients", "Who is taking it", register.RecipientLabel(*is), recipientLabelOf(newRecipients))
				if recipientContactChanged(oldRecipients, newRecipients) {
					note("recipientDetails", "Their details", recipientDetails(oldRecipients), recipientDetails(newRecipients))
				}
			} else {
				note("taker", "Who is taking it", is.TakerName, d.TakerName)
				note("department", "Department", is.TakerDepartment, d.Department)
				note("mobile", "Their mobile", is.TakerMobile, d.Mobile)
			}
			note("time", "Time taken", clock(is.IssuedAt), clock(issuedAt))
			is.Quantity, is.TakerName = qty, d.TakerName
			is.TakerDepartment, is.TakerMobile = d.Department, d.Mobile
			is.AdditionalTakers = append([]register.IssueRecipient(nil), d.Additional...)
			is.IssuedAt = issuedAt
		}
	case "return":
		if refusal := applyReturnEdit(reg, d, qty, note); refusal != "" {
			return nil, refusal
		}
	}
	return changes, ""
}

func recipientsOfEdit(d editData) []register.IssueRecipient {
	out := []register.IssueRecipient{{Name: d.TakerName, Department: d.Department, Mobile: d.Mobile}}
	return append(out, d.Additional...)
}

func recipientLabelOf(recipients []register.IssueRecipient) string {
	is := register.Issue{TakerName: recipients[0].Name, TakerDepartment: recipients[0].Department, TakerMobile: recipients[0].Mobile, AdditionalTakers: recipients[1:]}
	return register.RecipientLabel(is)
}

func recipientDetails(recipients []register.IssueRecipient) string {
	parts := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		parts = append(parts, recipient.Name+" | "+recipient.Department+" | "+recipient.Mobile)
	}
	return strings.Join(parts, "; ")
}

func recipientContactChanged(old, new []register.IssueRecipient) bool {
	return recipientDetails(old) != recipientDetails(new)
}

// applyReturnEdit re-runs the oldest-first allocation from scratch, exactly as
// a fresh return of that number would. It never adjusts the existing split in
// place, which would quietly give a different answer from entering the same
// number twice.
func applyReturnEdit(reg *register.Register, d editData, qty int,
	note func(field, label, from, to string)) string {

	returnedAt, _ := time.ParseInLocation(stampLayout, d.ReturnedAt, time.Local)

	for i := range reg.Returns {
		re := &reg.Returns[i]
		if re.ID != d.ID {
			continue
		}
		was := re.Quantity()
		taker := takerOfReturn(reg, *re)
		var holding register.JointHolding
		var groupIssueID string
		if len(re.Allocations) > 0 {
			for _, is := range register.LiveIssues(reg) {
				if is.ID == re.Allocations[0].IssueID && len(is.AdditionalTakers) > 0 {
					groupIssueID = is.ID
				}
			}
			for _, candidate := range register.JointHoldings(reg) {
				for _, line := range candidate.Lines {
					if line.IssueID == re.Allocations[0].IssueID {
						holding = candidate
					}
				}
			}
		}
		// The old allocations go first, so what is outstanding on each issue
		// reads as it did before this return was ever entered.
		re.Allocations = nil
		var issueIDs []string
		if groupIssueID != "" {
			issueIDs = []string{groupIssueID}
		}
		for _, line := range holding.Lines {
			if groupIssueID == "" && line.ProductID == re.ProductID {
				issueIDs = append(issueIDs, line.IssueID)
			}
		}
		if len(issueIDs) == 0 {
			issueIDs = issuesFor(reg, re.ProductID, taker)
		}

		plan := register.PlanReturn(reg, issueIDs, qty)
		// Let Validate decide an over-return too. PlanReturn stops at what is
		// outstanding, so put any excess on the last chosen issue temporarily;
		// the Update rollback removes it after Validate reports NegativeOut.
		if plan.Out < qty && len(plan.Allocations) > 0 {
			plan.Allocations[len(plan.Allocations)-1].Quantity += qty - plan.Out
		}

		disposition, remark := d.Disposition, d.Remark
		if plan.Short == 0 {
			disposition, remark = "", ""
		} else {
			short := strconv.Itoa(plan.Short)
			if disposition != string(register.ExpectedBack) && disposition != string(register.WontComeBack) {
				return "Tap one: the " + short + " " + d.Word +
					" are coming back later, or they are gone."
			}
			if remark == "" {
				return "Write what happened to the " + short + " " + d.Word + "."
			}
		}

		note("quantity", "How many", strconv.Itoa(was), strconv.Itoa(qty))
		note("returner", "Who is handing it back", re.ReturnerName, d.ReturnerName)
		note("mobile", "Their mobile", re.ReturnerMobile, d.ReturnerMobile)
		note("time", "Time returned", clock(re.ReturnedAt), clock(returnedAt))
		note("disposition", "What happened", dispositionWord(re.ShortDisposition), dispositionWord(register.Disposition(disposition)))
		note("remark", "Remark", re.Remark, remark)

		re.Allocations = plan.Allocations
		re.ReturnerName, re.ReturnerMobile = d.ReturnerName, d.ReturnerMobile
		re.ReturnedAt = returnedAt
		re.ShortQuantity = plan.Short
		re.ShortDisposition = register.Disposition(disposition)
		re.Remark = remark
	}
	return ""
}

// issuesFor is every live issue of one product held by one person, which is the
// set a fresh return of that product would be spread across.
func issuesFor(reg *register.Register, productID, takerName string) []string {
	want := register.FoldKey(takerName)
	var ids []string
	for _, is := range register.LiveIssues(reg) {
		if is.ProductID == productID && register.FoldKey(is.TakerName) == want {
			ids = append(ids, is.ID)
		}
	}
	return ids
}

func basisWord(b register.Basis) string {
	if b == register.Rent {
		return "Rent"
	}
	return "Purchase"
}

func dispositionWord(d register.Disposition) string {
	switch d {
	case register.ExpectedBack:
		return "Still expected back"
	case register.WontComeBack:
		return "Won't come back"
	}
	return ""
}

// appendChanges files the corrections on the record they describe. The return's
// own changes are collected by applyReturnEdit and passed through here too.
func appendChanges(reg *register.Register, id string, changes []register.Change) {
	if len(changes) == 0 {
		return
	}
	for i := range reg.Inwards {
		if reg.Inwards[i].ID == id {
			reg.Inwards[i].Changes = append(reg.Inwards[i].Changes, changes...)
		}
	}
	for i := range reg.Issues {
		if reg.Issues[i].ID == id {
			reg.Issues[i].Changes = append(reg.Issues[i].Changes, changes...)
		}
	}
	for i := range reg.Returns {
		if reg.Returns[i].ID == id {
			reg.Returns[i].Changes = append(reg.Returns[i].Changes, changes...)
		}
	}
}

// ---------------------------------------------------------------------------
// Deleting
// ---------------------------------------------------------------------------

// entryDelete tombstones a record. The record stays in the file, stays on its
// list, and counts towards nothing.
func (s *Server) entryDelete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id := r.PathValue("id")
	now := s.now()

	var data editData
	var found bool
	var by string
	s.st.Read(func(reg *register.Register) {
		data, found = editForm(reg, id)
		if who, ok := s.onDuty(reg); ok {
			by = who.Name
		}
	})
	if !found {
		s.notFound(w, r)
		return
	}
	if data.Deleted {
		s.renderEdit(w, data, nil)
		return
	}

	data.DeleteOpen = true
	reason := register.CleanName(r.FormValue("reason"))
	if reason == "" {
		s.renderEdit(w, data, &banner{"bad", "Say why you are deleting this."})
		return
	}

	var problems []register.Problem
	var orphaned []string
	err := s.st.Update(func(reg *register.Register) error {
		tombstone(reg, id, register.Deletion{At: now, By: by, Reason: reason})
		// The same one checker as the edit path.
		problems = register.Validate(reg)
		// And the one thing it cannot see: see the note on orphanedAllocations.
		orphaned = orphanedAllocations(reg)
		if len(problems) > 0 || len(orphaned) > 0 {
			return errRefused
		}
		return nil
	})
	switch {
	case errors.Is(err, errRefused):
		var refusal string
		s.st.Read(func(reg *register.Register) {
			refusal = deleteRefusal(reg, data, problems, orphaned)
		})
		s.renderEdit(w, data, &banner{"bad", refusal})
		return
	case err != nil:
		s.renderEdit(w, data, &banner{"bad", saveFailed})
		return
	}
	http.Redirect(w, r, data.From+"?deleted="+id, http.StatusSeeOther)
}

func tombstone(reg *register.Register, id string, d register.Deletion) {
	for i := range reg.Inwards {
		if reg.Inwards[i].ID == id {
			reg.Inwards[i].Deleted = &d
		}
	}
	for i := range reg.Issues {
		if reg.Issues[i].ID == id {
			reg.Issues[i].Deleted = &d
		}
	}
	for i := range reg.Returns {
		if reg.Returns[i].ID == id {
			reg.Returns[i].Deleted = &d
		}
	}
}

// orphanedAllocations names every issue that a live return still puts stock
// back against although the issue itself is now a tombstone.
//
// This is the one guard in this file that register.Validate cannot make.
// Validate's over-allocation check walks the live issues, so a deleted issue is
// simply not looked at, while Returned() still counts the return's allocations
// against it. On-hand comes out too high and nothing complains. Deleting the
// return first and the issue second is the way out of a wrong pair of entries,
// and a deleted return holds nothing, so this only ever blocks the order that
// would leave the register lying.
func orphanedAllocations(reg *register.Register) []string {
	live := map[string]bool{}
	for _, is := range register.LiveIssues(reg) {
		live[is.ID] = true
	}
	var orphans []string
	for _, re := range register.LiveReturns(reg) {
		for _, a := range re.Allocations {
			if !live[a.IssueID] {
				orphans = append(orphans, a.IssueID)
			}
		}
	}
	return orphans
}

// ---------------------------------------------------------------------------
// The sentences a refusal is shown in
// ---------------------------------------------------------------------------
//
// Everything below only phrases a problem somebody else decided. The figures
// come from the register as it stands, before the refused change, plus the
// record being corrected - never from a second formula that has to agree with
// Validate.

func editRefusal(reg *register.Register, was, typed editData, problems []register.Problem) string {
	asked, _ := wholeNumber(typed.Quantity)
	current, _ := wholeNumber(was.Quantity)
	word := was.Word

	for _, p := range problems {
		switch {
		case was.Kind == "inward" && p.Kind == register.NegativeOnHand:
			out := strconv.Itoa(register.OutWithPeople(reg, p.ProductID))
			return out + " " + word + " are out with people. Take some back before you go below " +
				out + " " + word + "."

		case was.Kind == "issue" && p.Kind == register.NegativeOnHand:
			// The quantity already on this entry is being re-spent, not spent
			// twice, so it belongs in the ceiling.
			ceiling := strconv.Itoa(asked + p.Have)
			return "Counting the " + strconv.Itoa(current) + " " + word +
				" already on this entry, you have " + ceiling +
				" " + word + ". You cannot give out more than " + ceiling + "."

		case was.Kind == "issue" && p.Kind == register.OverAllocatedIssue && p.IssueID == was.ID:
			back := strconv.Itoa(p.Have)
			return back + " " + word + " have already come back. To go below " + back + " " +
				word + ", fix that return first."

		case was.Kind == "return" && p.Kind == register.NegativeOnHand:
			gone, came := goneAndCame(reg, p.ProductID)
			least := strconv.Itoa(gone - came)
			return strconv.Itoa(gone) + " " + word + " have gone out and only " +
				strconv.Itoa(came) + " came in. Keep this at " + least + " " + word +
				" or more, or delete the issue first."

		case was.Kind == "return" && p.Kind == register.NegativeOut:
			allowed := strconv.Itoa(asked + p.Have)
			return was.TakerOfReturn + " took " + allowed + " " + word +
				". You cannot put back more than " + allowed + "."

		case was.Kind == "return" && p.Kind == register.OverAllocatedIssue:
			allowed := strconv.Itoa(asked - (p.Have - p.Want))
			return was.TakerOfReturn + " took " + allowed + " " + word +
				". You cannot put back more than " + allowed + "."
		}
	}
	return "That change would leave the register wrong. Nothing was saved."
}

func deleteRefusal(reg *register.Register, was editData, problems []register.Problem, orphaned []string) string {
	word := was.Word
	for _, p := range problems {
		switch {
		case was.Kind == "inward" && p.Kind == register.NegativeOnHand:
			out := strconv.Itoa(register.OutWithPeople(reg, p.ProductID))
			return out + " of these " + word + " are out with people. Take them back first, then delete this."

		case was.Kind == "return" && p.Kind == register.NegativeOnHand:
			gone, came := goneAndCame(reg, p.ProductID)
			return strconv.Itoa(gone) + " " + word + " have gone out and only " +
				strconv.Itoa(came) + " came in. Delete the issue that took them first, then this."
		}
	}
	for _, issueID := range orphaned {
		if issueID != was.ID {
			continue
		}
		back := strconv.Itoa(register.AllocatedFromLiveReturns(reg, issueID))
		return back + " " + word + " have already come back. Delete that return first, then this."
	}
	return "Deleting this would leave the register wrong. Nothing was deleted."
}

// goneAndCame is the pair of figures the two negative-on-hand sentences name:
// everything ever issued of a product, and everything ever received.
func goneAndCame(reg *register.Register, productID string) (gone, came int) {
	for _, is := range register.LiveIssues(reg) {
		if is.ProductID == productID {
			gone += is.Quantity
		}
	}
	return gone, register.CameIn(reg, productID)
}

// fixedBanner and deletedBanner are what the tab says when a correction lands.
func fixedBanner(reg *register.Register, id string) []banner {
	d, ok := editForm(reg, id)
	if !ok {
		return nil
	}
	productID := productIDOf(reg, id)
	return []banner{{"ok", "Fixed to " + d.Quantity + " " + d.Word + ". " + d.ProductName +
		": " + strconv.Itoa(register.OnHand(reg, productID)) + " on hand."}}
}

func deletedBanner(reg *register.Register, id string) []banner {
	d, ok := editForm(reg, id)
	if !ok {
		return nil
	}
	productID := productIDOf(reg, id)
	return []banner{{"ok", "Deleted. " + d.ProductName + ": " +
		strconv.Itoa(register.OnHand(reg, productID)) + " on hand."}}
}

func productIDOf(reg *register.Register, id string) string {
	for _, in := range reg.Inwards {
		if in.ID == id {
			return in.ProductID
		}
	}
	for _, is := range reg.Issues {
		if is.ID == id {
			return is.ProductID
		}
	}
	for _, re := range reg.Returns {
		if re.ID == id {
			return re.ProductID
		}
	}
	return ""
}
