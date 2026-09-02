package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"storeregister/internal/register"
)

func TestSchemaOneMigratesInMemoryWithoutWritingOnOpen(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "walkthrough-t0.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	s, res, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != Main {
		t.Fatalf("source=%v", res.Source)
	}
	s.Read(func(r *register.Register) {
		if r.SchemaVersion != 3 {
			t.Errorf("schema=%d", r.SchemaVersion)
		}
		for _, p := range r.Products {
			if p.Deleted != nil || p.Changes != nil {
				t.Errorf("lifecycle initialized on %s", p.ID)
			}
		}
	})
	afterBytes, _ := os.ReadFile(path)
	after, _ := os.Stat(path)
	if !bytes.Equal(source, afterBytes) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("Open changed schema-1 file")
	}
}

func TestFirstSaveAfterMigrationWritesSchemaThreeAtomically(t *testing.T) {
	source, _ := os.ReadFile(filepath.Join("testdata", "walkthrough-t0.json"))
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(r *register.Register) error { r.OnDutyStaffID = "STF-0001"; return nil }); err != nil {
		t.Fatal(err)
	}
	backup, _ := os.ReadFile(path + backupSuffix)
	if !bytes.Equal(backup, source) {
		t.Fatal("backup is not exact schema-1 source")
	}
	main, _ := os.ReadFile(path)
	if !bytes.Contains(main, []byte(`"schemaVersion": 3`)) {
		t.Fatal("main is not schema 3")
	}
	if _, _, err := Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

func TestUnknownSchemaStillRefused(t *testing.T) {
	for _, version := range []string{"0", "4"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			data := []byte(`{"schemaVersion":` + version + `,"products":[],"staff":[],"inwards":[],"issues":[],"returns":[]}`)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Open(path); err == nil {
				t.Fatalf("schema %s accepted", version)
			}
			got, _ := os.ReadFile(path)
			if !bytes.Equal(got, data) {
				t.Fatal("file changed")
			}
		})
	}
}

func TestStoreRollsBackProductLifecycleSlices(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())
	before, _ := os.ReadFile(path)
	wantErr := os.ErrPermission
	if err := s.Update(func(r *register.Register) error {
		r.Products[0].Name = "Bad"
		r.Products[0].Changes = append(r.Products[0].Changes, register.Change{Field: "bad"})
		r.Products[0].Deleted = &register.Deletion{Reason: "bad"}
		return wantErr
	}); err != wantErr {
		t.Fatalf("err=%v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("disk changed")
	}
	s.Read(func(r *register.Register) {
		if r.Products[0].Name == "Bad" || len(r.Products[0].Changes) != 0 || r.Products[0].Deleted != nil {
			t.Fatal("memory changed")
		}
	})
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	err := s.Update(func(r *register.Register) error {
		r.Products[0].Name = "Also bad"
		r.Products[0].Changes = append(r.Products[0].Changes, register.Change{Field: "bad"})
		r.Products[0].Deleted = &register.Deletion{Reason: "bad"}
		return nil
	})
	_ = os.Chmod(dir, 0700)
	if err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed save changed disk")
	}
	s.Read(func(r *register.Register) {
		if r.Products[0].Name == "Also bad" || len(r.Products[0].Changes) != 0 || r.Products[0].Deleted != nil {
			t.Fatal("failed save changed memory")
		}
	})
}

func TestOldIssuesLoadWithEmptyChallan(t *testing.T) {
	source, _ := os.ReadFile(filepath.Join("testdata", "walkthrough-t0.json"))
	path := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(path, source, 0600)
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Read(func(r *register.Register) {
		for _, is := range r.Issues {
			if is.ChallanNo != "" {
				t.Fatalf("issue %s challan=%q", is.ID, is.ChallanNo)
			}
		}
	})
	if err := s.Update(func(r *register.Register) error { return nil }); err != nil {
		t.Fatal(err)
	}
	main, _ := os.ReadFile(path)
	var raw struct {
		Issues []map[string]any `json:"issues"`
	}
	if err := json.Unmarshal(main, &raw); err != nil {
		t.Fatal(err)
	}
	for _, issue := range raw.Issues {
		if _, ok := issue["challanNo"]; ok {
			t.Fatal("empty issue challan was marshalled")
		}
	}
}
