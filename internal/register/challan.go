package register

import (
	"sort"
	"strings"
	"time"
)

type ChallanMatch struct {
	IssueID        string
	HoldingIssueID string
	ProductID      string
	ProductName    string
	Outstanding    int
	Recipients     []IssueRecipient
	RecipientLabel string
	IssuedAt       time.Time
	ChallanNo      string
}

func FindOutstandingByChallan(r *Register, query string) []ChallanMatch {
	want := FoldKey(query)
	if want == "" {
		return nil
	}
	names := productNames(r)
	var matches []ChallanMatch
	for _, is := range LiveIssues(r) {
		out := OutstandingOnIssue(r, is.ID)
		if is.ChallanNo == "" || out <= 0 || !strings.Contains(FoldKey(is.ChallanNo), want) {
			continue
		}
		holding, ok := holdingContainingIssue(r, is.ID)
		if !ok {
			continue
		}
		matches = append(matches, ChallanMatch{IssueID: is.ID, HoldingIssueID: holding.AnchorIssueID, ProductID: is.ProductID, ProductName: names[is.ProductID], Outstanding: out, Recipients: RecipientsOf(is), RecipientLabel: RecipientLabel(is), IssuedAt: is.IssuedAt, ChallanNo: is.ChallanNo})
	}
	sort.Slice(matches, func(i, j int) bool {
		if a, b := FoldKey(matches[i].ChallanNo), FoldKey(matches[j].ChallanNo); a != b {
			return a < b
		}
		if !matches[i].IssuedAt.Equal(matches[j].IssuedAt) {
			return matches[i].IssuedAt.Before(matches[j].IssuedAt)
		}
		return matches[i].IssueID < matches[j].IssueID
	})
	return matches
}

func holdingContainingIssue(r *Register, issueID string) (JointHolding, bool) {
	for _, h := range JointHoldings(r) {
		for _, line := range h.Lines {
			if line.IssueID == issueID {
				return h, true
			}
		}
	}
	return JointHolding{}, false
}
