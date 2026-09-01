package register

import (
	"testing"
	"time"
)

func allocationTestRegister() *Register {
	at := time.Date(2026, time.September, 1, 10, 0, 0, 0, IST)
	return &Register{
		SchemaVersion: SchemaVersion,
		Products: []Product{
			{ID: "PRD-0001", Name: "Chairs"},
			{ID: "PRD-0002", Name: "Round Tables"},
		},
		Issues: []Issue{
			{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 10, TakerName: "Ravi Menon", TakerMobile: "98861 40023", IssuedAt: at},
			{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 8, TakerName: "Ravi Menon", TakerMobile: "98861 40023", IssuedAt: at.Add(time.Minute)},
			{ID: "ISS-0003", ProductID: "PRD-0002", Quantity: 4, TakerName: "Ravi Menon", TakerMobile: "98861 40023", IssuedAt: at.Add(2 * time.Minute)},
			{ID: "ISS-0004", ProductID: "PRD-0001", Quantity: 3, TakerName: "Amit Sharma", AdditionalTakers: []IssueRecipient{{Name: "Suresh Patel"}}, IssuedAt: at.Add(3 * time.Minute)},
		},
		Returns: []Return{{ID: "RET-0001", ProductID: "PRD-0001", Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 3}}}},
	}
}

func TestTakerOfUsesEveryJointRecipient(t *testing.T) {
	r := allocationTestRegister()
	if got := TakerOf(r, []Allocation{{IssueID: "ISS-0004", Quantity: 1}}); got != "Amit Sharma and Suresh Patel" {
		t.Fatalf("TakerOf joint issue = %q", got)
	}
	if got := TakerOf(r, []Allocation{{IssueID: "ISS-9999", Quantity: 1}}); got != "" {
		t.Fatalf("TakerOf unknown issue = %q", got)
	}
}

func TestSameProductRequiresExistingIssuesForOnePersonAndProduct(t *testing.T) {
	r := allocationTestRegister()
	for _, tc := range []struct {
		name string
		ids  []string
		want bool
	}{
		{name: "none", ids: nil, want: false},
		{name: "same person and product", ids: []string{"ISS-0001", "ISS-0002"}, want: true},
		{name: "different product", ids: []string{"ISS-0001", "ISS-0003"}, want: false},
		{name: "different person", ids: []string{"ISS-0001", "ISS-0004"}, want: false},
		{name: "unknown issue", ids: []string{"ISS-9999"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameProduct(r, tc.ids); got != tc.want {
				t.Fatalf("SameProduct(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}

func TestHoldingByProductCombinesAndSortsRows(t *testing.T) {
	rows := HoldingByProduct([]OutstandingLine{
		{ProductID: "PRD-0002", ProductName: "Round Tables", Out: 2},
		{ProductID: "PRD-0001", ProductName: "Chairs", Out: 3},
		{ProductID: "PRD-0002", ProductName: "Round Tables", Out: 1},
		{ProductID: "PRD-0003", ProductName: "Benches", Out: 3},
	})
	if len(rows) != 3 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].ProductName != "Benches" || rows[0].Out != 3 || rows[1].ProductName != "Chairs" || rows[1].Out != 3 || rows[2].ProductName != "Round Tables" || rows[2].Out != 3 {
		t.Fatalf("combined and sorted rows = %#v", rows)
	}
}

func TestJointHoldingForIssueAcceptsOnlyTheAnchor(t *testing.T) {
	r := allocationTestRegister()
	holding, ok := JointHoldingForIssue(r, "ISS-0001")
	if !ok || holding.AnchorIssueID != "ISS-0001" || len(holding.Lines) != 3 {
		t.Fatalf("anchor lookup = %#v, %v", holding, ok)
	}
	if _, ok := JointHoldingForIssue(r, "ISS-0002"); ok {
		t.Fatal("non-anchor solo issue was accepted")
	}
	if _, ok := JointHoldingForIssue(r, "ISS-9999"); ok {
		t.Fatal("unknown issue was accepted")
	}
}

func TestAllocatedFromLiveReturnsSkipsDeletedReturns(t *testing.T) {
	r := allocationTestRegister()
	if got := AllocatedFromLiveReturns(r, "ISS-0001"); got != 3 {
		t.Fatalf("live allocation = %d, want 3", got)
	}
	r.Returns[0].Deleted = &Deletion{}
	if got := AllocatedFromLiveReturns(r, "ISS-0001"); got != 0 {
		t.Fatalf("allocation after delete = %d, want 0", got)
	}
}
