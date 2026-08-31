package register

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFixtureT0Totals(t *testing.T) {
	r := WalkthroughT0()
	cases := []struct {
		productID   string
		name        string
		cameIn, out int
	}{
		{"PRD-0001", "Chairs", 700, 310},
		{"PRD-0002", "Round tables", 60, 12},
		{"PRD-0003", "Water drums (20L)", 40, 5},
		{"PRD-0004", "Extension boards", 25, 25},
		{"PRD-0005", "Charcoal sacks", 12, 0},
	}
	for _, c := range cases {
		in := 0
		for _, w := range r.Inwards {
			if w.ProductID == c.productID {
				in += w.Quantity
			}
		}
		out := 0
		for _, is := range r.Issues {
			if is.ProductID == c.productID {
				out += is.Quantity
			}
		}
		if in != c.cameIn || out != c.out {
			t.Errorf("%s: came in %d out %d, want %d and %d", c.name, in, out, c.cameIn, c.out)
		}
	}
}

func TestFixtureReferentialIntegrity(t *testing.T) {
	points := map[string]*Register{
		"T0": WalkthroughT0(),
		"T1": WalkthroughT1(),
		"T2": WalkthroughT2(),
		"T3": WalkthroughT3(),
	}
	for name, r := range points {
		products := map[string]bool{}
		for _, p := range r.Products {
			if products[p.ID] {
				t.Errorf("%s: duplicate product id %s", name, p.ID)
			}
			products[p.ID] = true
		}
		staff := map[string]bool{}
		for _, s := range r.Staff {
			if staff[s.ID] {
				t.Errorf("%s: duplicate staff id %s", name, s.ID)
			}
			staff[s.ID] = true
		}
		if !staff[r.OnDutyStaffID] {
			t.Errorf("%s: on-duty staff %s is not in the staff list", name, r.OnDutyStaffID)
		}
		inwards := map[string]bool{}
		for _, w := range r.Inwards {
			if inwards[w.ID] {
				t.Errorf("%s: duplicate inward id %s", name, w.ID)
			}
			inwards[w.ID] = true
			if !products[w.ProductID] {
				t.Errorf("%s: inward %s names unknown product %s", name, w.ID, w.ProductID)
			}
		}
		issues := map[string]bool{}
		for _, is := range r.Issues {
			if issues[is.ID] {
				t.Errorf("%s: duplicate issue id %s", name, is.ID)
			}
			issues[is.ID] = true
			if !products[is.ProductID] {
				t.Errorf("%s: issue %s names unknown product %s", name, is.ID, is.ProductID)
			}
		}
		returns := map[string]bool{}
		for _, re := range r.Returns {
			if returns[re.ID] {
				t.Errorf("%s: duplicate return id %s", name, re.ID)
			}
			returns[re.ID] = true
			if !products[re.ProductID] {
				t.Errorf("%s: return %s names unknown product %s", name, re.ID, re.ProductID)
			}
			for _, a := range re.Allocations {
				if !issues[a.IssueID] {
					t.Errorf("%s: return %s allocates against unknown issue %s", name, re.ID, a.IssueID)
				}
			}
		}
	}
}

func TestFixtureNeverStoresAComputedTotal(t *testing.T) {
	b, err := json.MarshalIndent(WalkthroughT3(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"quantity": 890`) {
		t.Error("890 is stored in the file; it must only ever be computed")
	}
}

func TestFixtureTimepointsBuildOnEachOther(t *testing.T) {
	t0, t1, t2, t3 := WalkthroughT0(), WalkthroughT1(), WalkthroughT2(), WalkthroughT3()
	if len(t0.Inwards) != 6 || len(t0.Issues) != 7 || len(t0.Returns) != 0 {
		t.Errorf("T0 has %d inwards, %d issues, %d returns", len(t0.Inwards), len(t0.Issues), len(t0.Returns))
	}
	if len(t1.Inwards) != 7 || t1.Inwards[6].ID != "INW-0007" {
		t.Error("T1 does not add INW-0007")
	}
	if len(t2.Issues) != 8 || t2.Issues[7].ID != "ISS-0008" {
		t.Error("T2 does not add ISS-0008")
	}
	if len(t3.Returns) != 1 || t3.Returns[0].ID != "RET-0001" {
		t.Error("T3 does not add RET-0001")
	}
	if t3.Returns[0].ShortQuantity != 5 || t3.Returns[0].ShortDisposition != WontComeBack {
		t.Error("RET-0001 does not record the 5 short chairs")
	}
	// Each call is a fresh register, not a shared one.
	WalkthroughT0().Inwards[0].Quantity = 1
	if WalkthroughT0().Inwards[0].Quantity != 390 {
		t.Error("WalkthroughT0 hands out shared state")
	}
}

func TestFixtureCarriesProvenance(t *testing.T) {
	r := WalkthroughT0()
	for _, p := range r.Products {
		if p.CreatedAt.IsZero() {
			t.Errorf("%s has no CreatedAt", p.ID)
		}
		if p.CreatedBy != "Suresh Kumar" {
			t.Errorf("%s CreatedBy = %q, want Suresh Kumar", p.ID, p.CreatedBy)
		}
	}
	for _, in := range r.Inwards {
		if in.RecordedBy == "" || in.RecordedBy != in.ReceivedBy {
			t.Errorf("%s RecordedBy = %q, ReceivedBy = %q", in.ID, in.RecordedBy, in.ReceivedBy)
		}
	}
	// Nobody was on duty when the first person was added. That empty name is
	// deliberate and must not be filled in.
	if r.Staff[0].CreatedBy != "" {
		t.Errorf("STF-0001 CreatedBy = %q, want empty", r.Staff[0].CreatedBy)
	}
	for _, s := range r.Staff[1:] {
		if s.CreatedBy != "Suresh Kumar" {
			t.Errorf("%s CreatedBy = %q, want Suresh Kumar", s.ID, s.CreatedBy)
		}
	}
	for i := 1; i < len(r.Staff); i++ {
		if !r.Staff[i].CreatedAt.After(r.Staff[i-1].CreatedAt) {
			t.Errorf("%s was created at %v, not after %s at %v",
				r.Staff[i].ID, r.Staff[i].CreatedAt, r.Staff[i-1].ID, r.Staff[i-1].CreatedAt)
		}
	}
}
