package register

import "sort"

// This file is the return's arithmetic. It lives here, with the rest of the
// numbers, because internal/web does no arithmetic: a handler asks for a plan
// and writes down the answer.

// ReturnPlan is how one handback lands on the issues behind it.
type ReturnPlan struct {
	Allocations []Allocation // oldest issue first, zero-quantity lines omitted
	Short       int          // how many of the outstanding total did not come back
	TakerName   string       // the person the stock was issued to
	TakerMobile string
	ProductID   string
	ProductName string
	Out         int // how many were outstanding across those issues
}

// PlanReturn spreads qty across the given issues, oldest IssuedAt first and
// ties broken by issue ID, filling each line's outstanding amount before moving
// to the next. The taker is read from the issues, never from whoever carried
// the goods to the desk.
//
// It assumes CheckReturn has already passed on the same register: call them
// together, inside the same save, so two desks cannot return the same stock
// twice.
func PlanReturn(r *Register, issueIDs []string, qty int) ReturnPlan {
	names := productNames(r)

	var chosen []Issue
	for _, id := range issueIDs {
		for _, is := range LiveIssues(r) {
			if is.ID == id {
				chosen = append(chosen, is)
			}
		}
	}
	sort.Slice(chosen, func(i, j int) bool {
		if !chosen[i].IssuedAt.Equal(chosen[j].IssuedAt) {
			return chosen[i].IssuedAt.Before(chosen[j].IssuedAt)
		}
		return chosen[i].ID < chosen[j].ID
	})

	plan := ReturnPlan{}
	left := qty
	for _, is := range chosen {
		if plan.TakerName == "" {
			plan.TakerName = RecipientLabel(is)
			plan.TakerMobile = is.TakerMobile
			plan.ProductID = is.ProductID
			plan.ProductName = names[is.ProductID]
		}
		out := OutstandingOnIssue(r, is.ID)
		plan.Out += out
		take := out
		if take > left {
			take = left
		}
		if take > 0 {
			plan.Allocations = append(plan.Allocations, Allocation{IssueID: is.ID, Quantity: take})
			left -= take
		}
	}
	plan.Short = plan.Out - qty
	if plan.Short < 0 {
		plan.Short = 0
	}
	return plan
}

// TakerOf names the person a set of allocations was issued to. A return record
// carries the person who handed the goods back; the person who owes them is on
// the issue.
func TakerOf(r *Register, allocations []Allocation) string {
	for _, a := range allocations {
		for _, is := range LiveIssues(r) {
			if is.ID == a.IssueID {
				return RecipientLabel(is)
			}
		}
	}
	return ""
}

// SameProduct reports whether every named issue exists and is for one product,
// held by one person. A return entry covers one product and one person: the
// desk is handing back chairs, and the allocation, the shortfall and every
// sentence on the screen name a single taker.
func SameProduct(r *Register, issueIDs []string) bool {
	if len(issueIDs) == 0 {
		return false
	}
	productID := ""
	var who PersonID
	for _, id := range issueIDs {
		found := false
		for _, is := range LiveIssues(r) {
			if is.ID != id {
				continue
			}
			found = true
			person := PersonOf(is.TakerName, is.TakerMobile)
			if productID == "" {
				productID, who = is.ProductID, person
			}
			if is.ProductID != productID || person != who {
				return false
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ProductHolding is how many of one product a person is still holding.
type ProductHolding struct {
	ProductID   string
	ProductName string
	Out         int
}

// HoldingByProduct collapses a person's outstanding lines to one row per
// product, most first, ties alphabetical. It is what the amber warning on the
// issue screen reads out: "40 chairs and 2 round tables".
func HoldingByProduct(lines []OutstandingLine) []ProductHolding {
	var rows []ProductHolding
	at := map[string]int{}
	for _, l := range lines {
		if i, seen := at[l.ProductID]; seen {
			rows[i].Out += l.Out
			continue
		}
		at[l.ProductID] = len(rows)
		rows = append(rows, ProductHolding{
			ProductID: l.ProductID, ProductName: l.ProductName, Out: l.Out,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Out != rows[j].Out {
			return rows[i].Out > rows[j].Out
		}
		return FoldKey(rows[i].ProductName) < FoldKey(rows[j].ProductName)
	})
	return rows
}
