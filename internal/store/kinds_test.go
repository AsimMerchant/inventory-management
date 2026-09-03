package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"storeregister/internal/register"
)

// TestSchemaThreeFileLoadsUntouched is the promise the schema-4 bump rests on:
// the register the user is running today opens with every record intact and
// gains nothing it did not have.
func TestSchemaThreeFileLoadsUntouched(t *testing.T) {
	r := register.WalkthroughT0()
	r.SchemaVersion = 3
	data := mustEncode(t, r)
	if bytes.Contains(data, []byte("acquisitionKinds")) {
		t.Fatal("a register with no typed kinds wrote the key anyway")
	}
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)

	s, res, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != Main || res.Warning != "" {
		t.Fatalf("source=%v warning=%q", res.Source, res.Warning)
	}
	s.Read(func(got *register.Register) {
		if got.SchemaVersion != register.SchemaVersion {
			t.Errorf("schema=%d", got.SchemaVersion)
		}
		if len(got.Inwards) != len(r.Inwards) || len(got.Issues) != len(r.Issues) {
			t.Errorf("loaded %d inwards and %d issues", len(got.Inwards), len(got.Issues))
		}
		if got.AcquisitionKinds != nil {
			t.Errorf("opening invented %d typed kinds", len(got.AcquisitionKinds))
		}
		for _, in := range got.Inwards {
			if in.KindID != "" {
				t.Errorf("%s came back with a kind", in.ID)
			}
		}
	})
	after, _ := os.ReadFile(path)
	afterStat, _ := os.Stat(path)
	if !bytes.Equal(data, after) || !before.ModTime().Equal(afterStat.ModTime()) {
		t.Fatal("Open changed the schema-3 file")
	}
}

// TestTypedKindSurvivesASave is the round trip: what the desk saved is what the
// file says and what the next open reads back.
func TestTypedKindSurvivesASave(t *testing.T) {
	s, _, path := openTemp(t)
	var id string
	if err := s.Update(func(r *register.Register) error {
		var err error
		id, err = register.AddAcquisitionKind(r, "Donated", "Suresh Kumar", financeNow)
		if err != nil {
			return err
		}
		r.Inwards = append(r.Inwards, register.Inward{
			ID: r.NextID("INW"), ProductID: "PRD-0001", Quantity: 50,
			ReceivedOn: "2026-09-03", Basis: register.Other, KindID: id,
			Supplier: "Sharma Tent House", ReceivedBy: "Suresh Kumar",
			RecordedAt: financeNow, RecordedBy: "Suresh Kumar",
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(path)
	for _, want := range []string{`"acquisitionKinds"`, `"name": "Donated"`, `"kindId": "AKD-0001"`, `"basis": "other"`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the saved file does not contain %s", want)
		}
	}
	reopened, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Read(func(r *register.Register) {
		if got := register.BasisWord(r, register.Other, id); got != "Donated" {
			t.Errorf("after reopening the delivery reads %q", got)
		}
	})
}

// kindStore is a vault with an administrator, an ordinary user and two typed
// kinds already on the shared list.
func kindStore(t *testing.T) (*Store, []byte, string, string, map[string]string) {
	t.Helper()
	s, _, key, _ := initializedFinance(t)
	userID, code, err := s.AuthorizeFinanceAccount(key, "FAC-0001", "Ravi Menon", "90350 66471", register.FinanceUser, financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ActivateFinance("90350 66471", code, "correct horse", financeNow); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	if err := s.Update(func(r *register.Register) error {
		for _, word := range []string{"Donated", "Donatd"} {
			id, err := register.AddAcquisitionKind(r, word, "Suresh Kumar", financeNow)
			if err != nil {
				return err
			}
			ids[word] = id
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, key, "FAC-0001", userID, ids
}

func kindsOf(t *testing.T, s *Store) []register.AcquisitionKind {
	t.Helper()
	var out []register.AcquisitionKind
	s.Read(func(r *register.Register) { out = append(out, r.AcquisitionKinds...) })
	return out
}

func TestAcquisitionKindListActions(t *testing.T) {
	t.Run("rename", func(t *testing.T) {
		s, key, admin, _, ids := kindStore(t)
		if err := s.RenameAcquisitionKind(key, admin, ids["Donatd"], "Gifted", financeNow); err != nil {
			t.Fatal(err)
		}
		s.Read(func(r *register.Register) {
			if got := register.AcquisitionKindText(r, ids["Donatd"]); got != "Gifted" {
				t.Errorf("the word reads %q", got)
			}
		})
		// A rename onto a word already there is refused rather than making
		// two rows say the same thing.
		if err := s.RenameAcquisitionKind(key, admin, ids["Donatd"], "donated", financeNow); err == nil {
			t.Error("renaming onto an existing word was allowed")
		}
		if err := s.RenameAcquisitionKind(key, admin, ids["Donatd"], "  ", financeNow); err == nil {
			t.Error("a blank rename was allowed")
		}
	})

	t.Run("merge", func(t *testing.T) {
		s, key, admin, _, ids := kindStore(t)
		if err := s.MergeAcquisitionKind(key, admin, ids["Donatd"], ids["Donated"], financeNow); err != nil {
			t.Fatal(err)
		}
		s.Read(func(r *register.Register) {
			if got := register.AcquisitionKindText(r, ids["Donatd"]); got != "Donated" {
				t.Errorf("a record saved with the typo now reads %q", got)
			}
			if len(register.LiveAcquisitionKinds(r)) != 1 {
				t.Error("the merged word is still offered")
			}
			if len(r.AcquisitionKinds) != 2 {
				t.Error("the merged row was thrown away")
			}
		})
		if err := s.MergeAcquisitionKind(key, admin, ids["Donated"], ids["Donated"], financeNow); err == nil {
			t.Error("a word was merged into itself")
		}
	})

	t.Run("delete an unused word", func(t *testing.T) {
		s, key, admin, _, ids := kindStore(t)
		if err := s.DeleteAcquisitionKind(key, admin, ids["Donatd"], financeNow); err != nil {
			t.Fatal(err)
		}
		if got := kindsOf(t, s); len(got) != 1 || got[0].Name != "Donated" {
			t.Errorf("the list is now %+v", got)
		}
	})

	t.Run("a word in use is never deleted", func(t *testing.T) {
		s, key, admin, _, ids := kindStore(t)
		if err := s.Update(func(r *register.Register) error {
			r.Inwards = append(r.Inwards, register.Inward{
				ID: r.NextID("INW"), ProductID: "PRD-0001", Quantity: 5,
				ReceivedOn: "2026-09-03", Basis: register.Other, KindID: ids["Donated"],
				ReceivedBy: "Suresh Kumar", RecordedAt: financeNow, RecordedBy: "Suresh Kumar",
			})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteAcquisitionKind(key, admin, ids["Donated"], financeNow); !errors.Is(err, register.ErrKindUsed) {
			t.Fatalf("deleting a used word gave %v", err)
		}
		if len(kindsOf(t, s)) != 2 {
			t.Error("the list changed anyway")
		}
	})

	t.Run("only an administrator may correct the list", func(t *testing.T) {
		s, key, _, user, ids := kindStore(t)
		for _, err := range []error{
			s.RenameAcquisitionKind(key, user, ids["Donatd"], "Gifted", financeNow),
			s.MergeAcquisitionKind(key, user, ids["Donatd"], ids["Donated"], financeNow),
			s.DeleteAcquisitionKind(key, user, ids["Donatd"], financeNow),
		} {
			if !errors.Is(err, ErrNotAdmin) {
				t.Errorf("an ordinary user got %v", err)
			}
		}
		if got := kindsOf(t, s); len(got) != 2 || got[1].Name != "Donatd" {
			t.Errorf("the list is now %+v", got)
		}
	})

	t.Run("a correction is audited in the vault", func(t *testing.T) {
		s, key, admin, _, ids := kindStore(t)
		if err := s.RenameAcquisitionKind(key, admin, ids["Donatd"], "Gifted", financeNow); err != nil {
			t.Fatal(err)
		}
		var found bool
		if err := s.ReadFinance(key, func(f *register.FinanceData) {
			for _, e := range f.Audit {
				if e.Kind == "kind_renamed" && e.EntityID == ids["Donatd"] && e.Before == "Donatd" && e.After == "Gifted" {
					found = true
				}
			}
		}); err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Error("the rename left no audit row")
		}
	})
}
