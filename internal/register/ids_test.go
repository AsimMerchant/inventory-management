package register

import "testing"

func TestNextIDContinuesFromHighest(t *testing.T) {
	r := WalkthroughT0()
	cases := []struct{ prefix, want string }{
		{"INW", "INW-0007"},
		{"ISS", "ISS-0008"},
		{"RET", "RET-0001"},
		{"PRD", "PRD-0006"},
		{"STF", "STF-0004"},
	}
	for _, c := range cases {
		if got := r.NextID(c.prefix); got != c.want {
			t.Errorf("NextID(%q) = %q, want %q", c.prefix, got, c.want)
		}
	}
}

func TestNextIDOnEmptyRegister(t *testing.T) {
	r := &Register{SchemaVersion: 1}
	if got := r.NextID("PRD"); got != "PRD-0001" {
		t.Errorf("NextID(PRD) = %q, want PRD-0001", got)
	}
}

func TestNextIDPastFourDigits(t *testing.T) {
	r := WalkthroughT0()
	r.Inwards = append(r.Inwards, Inward{ID: "INW-9999", ProductID: "PRD-0001", Quantity: 1})
	if got := r.NextID("INW"); got != "INW-10000" {
		t.Errorf("NextID(INW) = %q, want INW-10000", got)
	}
}

func TestNextIDIgnoresMalformedIDs(t *testing.T) {
	r := &Register{SchemaVersion: 1, Products: []Product{
		{ID: "PRD-0003"}, {ID: "junk"}, {ID: "PRD-abc"},
	}}
	if got := r.NextID("PRD"); got != "PRD-0004" {
		t.Errorf("NextID(PRD) = %q, want PRD-0004", got)
	}
	if got := r.NextID("XXX"); got != "XXX-0001" {
		t.Errorf("NextID(XXX) = %q, want XXX-0001", got)
	}
}
