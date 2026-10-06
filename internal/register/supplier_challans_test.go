package register

import "testing"

func TestSupplierChallanVocabularyPartyScopeAndMergeRules(t *testing.T) {
	r := &Register{SchemaVersion: SchemaVersion}
	sharma, _ := AddParty(r, "Sharma Events")
	gupta, _ := AddParty(r, "Gupta Events")
	first, err := AddSupplierChallan(r, sharma, " STH/4390 ")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := AddSupplierChallan(r, sharma, "sth/4390"); err != nil || again != first {
		t.Fatalf("duplicate reuse = %q, %v", again, err)
	}
	other, _ := AddSupplierChallan(r, gupta, "STH/4390")
	AddSupplierChallan(r, sharma, "4390-old")
	AddSupplierChallan(r, sharma, "Z/4390")
	rows := MatchSupplierChallans(r, sharma, "4390")
	if len(rows) != 3 || rows[0].Number != "4390-old" || rows[1].ID != first || rows[2].Number != "Z/4390" {
		t.Fatalf("match order = %+v", rows)
	}
	if got, party, ok := ResolveSupplierChallan(r, first); !ok || got.Number != "STH/4390" || party.ID != sharma {
		t.Fatalf("resolution = %+v %+v %v", got, party, ok)
	}

	// A later party merge may make old rows folded duplicates. They remain
	// valid and suggestions return the lowest SCH ID once.
	for i := range r.Parties {
		if r.Parties[i].ID == gupta {
			r.Parties[i].MergedIntoID = sharma
		}
	}
	if err := ValidateSupplierChallans(r); err != nil {
		t.Fatalf("post-merge duplicate refused: %v", err)
	}
	rows = MatchSupplierChallans(r, sharma, "STH/4390")
	if len(rows) != 1 || rows[0].ID != first || rows[0].ID == other {
		t.Fatalf("merged dedup = %+v", rows)
	}
}
