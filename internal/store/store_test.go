package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"storeregister/internal/register"
)

// The fixture is built in IST and the file records "+05:30". Pinning time.Local
// to the fixture's zone is what makes reflect.DeepEqual hold after a round trip
// through the file, on a developer machine in any timezone.
func TestMain(m *testing.M) {
	time.Local = register.IST
	os.Exit(m.Run())
}

// openTemp opens a store in a fresh temporary directory.
func openTemp(t *testing.T) (*Store, LoadResult, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	s, res, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, res, path
}

// saveRegister writes reg through the store, exercising the real save sequence.
func saveRegister(t *testing.T, s *Store, reg *register.Register) {
	t.Helper()
	if err := s.Update(func(r *register.Register) error {
		*r = *reg
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func readFile(t *testing.T, path string) *register.Register {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var reg register.Register
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &reg
}

func chairTotal(reg *register.Register) int {
	total := 0
	for _, in := range reg.Inwards {
		if in.ProductID == "PRD-0001" {
			total += in.Quantity
		}
	}
	return total
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	reopened, res, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if res.Source != Main {
		t.Errorf("Source = %v, want Main", res.Source)
	}
	if res.Warning != "" {
		t.Errorf("Warning = %q, want empty", res.Warning)
	}
	reopened.Read(func(r *register.Register) {
		want := register.WalkthroughT0()
		register.LinkInwardParties(want)
		if !reflect.DeepEqual(r, want) {
			t.Error("the reloaded register is not the register that was saved")
		}
	})
}

func TestOnDiskFixtureMatchesCode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "walkthrough-t0.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reg register.Register
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatal(err)
	}
	want := register.WalkthroughT0()
	want.SchemaVersion = 1
	if !reflect.DeepEqual(&reg, want) {
		t.Error("testdata/walkthrough-t0.json no longer matches register.WalkthroughT0()")
	}
	encoded, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(append(encoded, '\n')) != string(data) {
		t.Error("the file format changed: regenerate testdata/walkthrough-t0.json only if that is intended")
	}
}

func TestFileIsHumanReadable(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"  \"schemaVersion\": 5,\n",
		"      \"name\": \"Chairs\",\n",
		"      \"supplier\": \"Sharma Tent House\",\n",
		"Sharma Tent House",
		"Ravi Menon",
		"Water drums (20L)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("saved file does not contain %q", want)
		}
	}
	if !strings.HasSuffix(text, "}\n") || strings.HasSuffix(text, "\n\n") {
		t.Error("saved file does not end with exactly one newline")
	}
}

func TestBackupHoldsPreviousVersion(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	if err := s.Update(func(r *register.Register) error {
		r.Inwards = append(r.Inwards, register.Inward{
			ID: r.NextID("INW"), ProductID: "PRD-0001", Quantity: 500,
			ReceivedOn: "2026-09-03", Basis: register.Rent,
			Supplier: "Sharma Tent House", ChallanNo: "STH/4471",
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	main := readFile(t, path)
	bak := readFile(t, path+backupSuffix)
	if len(main.Inwards) != 7 {
		t.Errorf("main file has %d inwards, want 7", len(main.Inwards))
	}
	if len(bak.Inwards) != 6 {
		t.Errorf("backup has %d inwards, want 6", len(bak.Inwards))
	}
	if got := chairTotal(bak); got != 700 {
		t.Errorf("backup chair total %d, want 700", got)
	}
	if got := chairTotal(main); got != 1200 {
		t.Errorf("main chair total %d, want 1200", got)
	}
}

func TestCrashBetweenBackupAndRename(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())
	saveRegister(t, s, register.WalkthroughT1())

	// Step 6 done, step 7 not: .bak holds T0, .tmp holds T1, no main file.
	t1, err := json.MarshalIndent(register.WalkthroughT1(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+tempSuffix, append(t1, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+backupSuffix, mustEncode(t, register.WalkthroughT0()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	reopened, res, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if res.Source != Backup {
		t.Errorf("Source = %v, want Backup", res.Source)
	}
	if res.Warning == "" {
		t.Error("Warning is empty; the screen would say nothing")
	}
	reopened.Read(func(r *register.Register) {
		if len(r.Inwards) != 6 {
			t.Errorf("loaded %d inwards, want 6 from the backup", len(r.Inwards))
		}
	})
}

func TestCrashDuringTempWrite(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	if err := os.WriteFile(path+tempSuffix, []byte(`{"schemaVersion":1,"products":[`), 0600); err != nil {
		t.Fatal(err)
	}

	reopened, res, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if res.Source != Main || res.Warning != "" {
		t.Errorf("Source = %v Warning = %q, want Main and no warning", res.Source, res.Warning)
	}
	reopened.Read(func(r *register.Register) {
		if len(r.Inwards) != 6 {
			t.Errorf("loaded %d inwards, want 6", len(r.Inwards))
		}
	})
}

func TestTruncatedMainFileFallsBackToBackup(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())
	saveRegister(t, s, register.WalkthroughT1())

	full, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	truncated := full[:40]
	if err := os.WriteFile(path, truncated, 0600); err != nil {
		t.Fatal(err)
	}

	reopened, res, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if res.Source != Backup {
		t.Errorf("Source = %v, want Backup", res.Source)
	}
	if res.Warning == "" {
		t.Error("Warning is empty")
	}
	reopened.Read(func(r *register.Register) {
		if len(r.Inwards) != 6 {
			t.Errorf("loaded %d inwards, want 6 from the backup", len(r.Inwards))
		}
	})

	matches, err := filepath.Glob(path + corruptPrefix + "*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d rescued copies, want 1", len(matches))
	}
	saved, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != string(truncated) {
		t.Error("the rescued copy is not the damaged file")
	}
}

func TestBothFilesCorruptRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	rubbish := []byte("not json at all")
	if err := os.WriteFile(path, rubbish, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+backupSuffix, rubbish, 0600); err != nil {
		t.Fatal(err)
	}

	s, _, err := Open(path)
	if err == nil {
		t.Fatal("Open succeeded on two unreadable files")
	}
	if s != nil {
		t.Error("Open returned a store as well as an error")
	}
	for _, p := range []string{path, path + backupSuffix} {
		info, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("%s is gone: %v", p, statErr)
		}
		if info.Size() != int64(len(rubbish)) {
			t.Errorf("%s changed size to %d", p, info.Size())
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(data) != string(rubbish) {
			t.Errorf("%s was rewritten", p)
		}
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), path+backupSuffix) {
		t.Errorf("the error does not name both files: %v", err)
	}

	// The sentence is settled in 02-persistence.spec.md. It is read at the
	// worst moment there is, so it must say the data is untouched and who to
	// call, and it must never show the JSON parser's own words.
	want := "The register could not be opened. Nothing has been changed. " +
		"Call whoever set this up and give them these two files:\n" +
		path + "\n" + path + backupSuffix
	if err.Error() != want {
		t.Errorf("the wording is not the settled one:\n got: %q\nwant: %q", err.Error(), want)
	}
	for _, leak := range []string{"invalid character", "unmarshal", "is not a readable register"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the parser's own words leaked into the message: %q", leak)
		}
	}
}

func TestFreshDirectoryStartsEmpty(t *testing.T) {
	s, res, path := openTemp(t)
	if res.Source != Fresh {
		t.Errorf("Source = %v, want Fresh", res.Source)
	}
	s.Read(func(r *register.Register) {
		if r.SchemaVersion != register.SchemaVersion {
			t.Errorf("SchemaVersion = %d, want %d", r.SchemaVersion, register.SchemaVersion)
		}
		if len(r.Products) != 0 || len(r.Staff) != 0 || len(r.Inwards) != 0 {
			t.Error("a fresh register is not empty")
		}
		if r.Products == nil || r.Staff == nil || r.Inwards == nil || r.Issues == nil || r.Returns == nil {
			t.Error("a fresh register has nil slices; the file would show null")
		}
	})
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Open wrote %d files before the first Update", len(entries))
	}
}

func TestWrongSchemaVersionRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	future := mustEncode(t, register.WalkthroughT0())
	future = []byte(strings.Replace(string(future), `"schemaVersion": 5`, `"schemaVersion": 6`, 1))
	if err := os.WriteFile(path, future, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+backupSuffix, mustEncode(t, register.WalkthroughT0()), 0600); err != nil {
		t.Fatal(err)
	}

	s, res, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.Source != Backup || res.Warning == "" {
		t.Errorf("Source = %v Warning = %q, want Backup with a warning", res.Source, res.Warning)
	}
	s.Read(func(r *register.Register) {
		if len(r.Inwards) != 6 {
			t.Errorf("loaded %d inwards, want 6", len(r.Inwards))
		}
	})

	// With no backup at all, there is nothing to fall back to.
	dir2 := t.TempDir()
	path2 := filepath.Join(dir2, FileName)
	if err := os.WriteFile(path2, future, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path2); err == nil {
		t.Error("Open accepted a schema 6 file with no backup")
	}
}

func TestUpdateErrorWritesNothing(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT1())

	refusal := errors.New("890 on hand, cannot issue 900")
	err := s.Update(func(r *register.Register) error {
		r.Issues = append(r.Issues, register.Issue{
			ID: r.NextID("ISS"), ProductID: "PRD-0001", Quantity: 10,
			TakerName: "Ravi Menon", TakerMobile: "98861 40023",
		})
		return refusal
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("Update returned %v, want the refusal unchanged", err)
	}

	if got := len(readFile(t, path).Issues); got != 7 {
		t.Errorf("file has %d issues, want 7", got)
	}
	if _, err := os.Stat(path + backupSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Error("a backup was written by a failed update")
	}
	s.Read(func(r *register.Register) {
		if len(r.Issues) != 7 {
			t.Errorf("memory has %d issues, want 7", len(r.Issues))
		}
	})
}

func TestDeepCopyShareNoBackingArrays(t *testing.T) {
	orig := register.WalkthroughT3()
	c := deepCopy(orig)

	c.Issues[0].Quantity = 1
	c.Returns = append(c.Returns, register.Return{ID: "RET-0002"})
	c.Inwards[0].Supplier = "Changed"
	c.Returns[0].Allocations[0].Quantity = 999
	c.Products[0].Name = "Stools"

	if orig.Issues[0].Quantity != 150 {
		t.Errorf("original issue quantity is %d, want 150", orig.Issues[0].Quantity)
	}
	if len(orig.Returns) != 1 {
		t.Errorf("original has %d returns, want 1", len(orig.Returns))
	}
	if orig.Inwards[0].Supplier != "Sharma Tent House" {
		t.Errorf("original supplier is %q", orig.Inwards[0].Supplier)
	}
	if orig.Returns[0].Allocations[0].Quantity != 40 {
		t.Errorf("original allocation is %d, want 40", orig.Returns[0].Allocations[0].Quantity)
	}
	if orig.Products[0].Name != "Chairs" {
		t.Errorf("original product name is %q", orig.Products[0].Name)
	}

	// And the other direction.
	orig2 := register.WalkthroughT3()
	c2 := deepCopy(orig2)
	orig2.Issues[0].Quantity = 7
	orig2.Inwards[0].Supplier = "Also changed"
	orig2.Returns[0].Allocations[0].Quantity = 1
	orig2.Returns = append(orig2.Returns, register.Return{ID: "RET-0003"})
	if c2.Issues[0].Quantity != 150 || c2.Inwards[0].Supplier != "Sharma Tent House" ||
		c2.Returns[0].Allocations[0].Quantity != 40 || len(c2.Returns) != 1 {
		t.Error("the copy followed a change to the original")
	}

	// Corrections and tombstones are copied too.
	orig3 := register.WalkthroughT1()
	orig3.Inwards[6].Changes = []register.Change{{Field: "quantity", From: "500", To: "50"}}
	orig3.Inwards[1].Deleted = &register.Deletion{By: "Suresh Kumar", Reason: "Entered twice by mistake."}
	shift := register.MustTime("2026-09-03T08:00:00+05:30")
	orig3.ShiftStartedAt = &shift
	c3 := deepCopy(orig3)
	c3.Inwards[6].Changes[0].To = "5"
	c3.Inwards[1].Deleted.Reason = "Something else"
	*c3.ShiftStartedAt = register.MustTime("2026-09-03T09:00:00+05:30")
	if orig3.Inwards[6].Changes[0].To != "50" {
		t.Error("the audit line is shared with the copy")
	}
	if orig3.Inwards[1].Deleted.Reason != "Entered twice by mistake." {
		t.Error("the tombstone is shared with the copy")
	}
	if !orig3.ShiftStartedAt.Equal(register.MustTime("2026-09-03T08:00:00+05:30")) {
		t.Error("the shift start time is shared with the copy")
	}
}

func TestUpdateRollsBackInMemoryAfterWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: an unwritable directory is still writable")
	}
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	err := s.Update(func(r *register.Register) error {
		r.Inwards = append(r.Inwards, register.Inward{ID: r.NextID("INW"), ProductID: "PRD-0001", Quantity: 500})
		return nil
	})
	if err == nil {
		t.Fatal("Update succeeded with an unwritable directory")
	}
	s.Read(func(r *register.Register) {
		if len(r.Inwards) != 6 {
			t.Errorf("memory has %d inwards, want 6", len(r.Inwards))
		}
	})
}

func TestConcurrentUpdatesAreSerialised(t *testing.T) {
	s, _, path := openTemp(t)
	saveRegister(t, s, register.WalkthroughT0())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Update(func(r *register.Register) error {
				r.Issues = append(r.Issues, register.Issue{
					ID: r.NextID("ISS"), ProductID: "PRD-0001", Quantity: 1,
					TakerName: "Lakshmi Iyer", TakerDepartment: "Kitchen", TakerMobile: "99860 11204",
				})
				return nil
			}); err != nil {
				t.Errorf("Update: %v", err)
			}
		}()
	}
	wg.Wait()

	saved := readFile(t, path)
	if len(saved.Issues) != 57 {
		t.Fatalf("file has %d issues, want 57", len(saved.Issues))
	}
	seen := map[string]bool{}
	for _, is := range saved.Issues {
		if seen[is.ID] {
			t.Errorf("duplicate issue id %s", is.ID)
		}
		seen[is.ID] = true
	}
	for i := 8; i <= 57; i++ {
		id := fmt.Sprintf("ISS-%04d", i)
		if !seen[id] {
			t.Errorf("missing %s", id)
		}
	}
}

func TestDataPathSitsBesideTheExecutable(t *testing.T) {
	path, err := DataPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != FileName {
		t.Errorf("DataPath = %q, want it to end in %s", path, FileName)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("DataPath = %q, want an absolute path", path)
	}
}

func mustEncode(t *testing.T, reg *register.Register) []byte {
	t.Helper()
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}
