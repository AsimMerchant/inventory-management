// Package store reads and writes the one register file that lives beside the
// executable. Every save is atomic: a crash leaves either the previous good
// file or the new good file, never a truncated one.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"storeregister/internal/register"
)

// FileName is the register file. The backup and the in-flight temp file sit
// beside it with these suffixes.
const (
	FileName      = "store-register.json"
	backupSuffix  = ".bak"
	tempSuffix    = ".tmp"
	corruptPrefix = ".corrupt-"
)

// DataPath is the register file in the directory of the running executable.
// Never the working directory: on Windows a desktop shortcut sets the working
// directory somewhere else and the register would silently move.
func DataPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Join(filepath.Dir(exe), FileName), nil
}

// LoadSource says where the register in memory came from.
type LoadSource int

const (
	Fresh  LoadSource = iota // no file existed; this is a new register
	Main                     // store-register.json
	Backup                   // store-register.json.bak
)

// LoadResult reports what Open found on disk.
type LoadResult struct {
	Source  LoadSource
	Warning string // "" unless Source == Backup or a file was unreadable
}

// Store owns the register in memory and the file on disk.
type Store struct {
	path string
	mu   sync.Mutex
	reg  *register.Register
}

// Open loads the register from path, falling back to the backup and then to a
// fresh empty register. It returns an error only when a file exists but neither
// it nor the backup can be read: nothing has been written or changed in that case.
func Open(path string) (*Store, LoadResult, error) {
	mainPath := path
	bakPath := path + backupSuffix

	reg, mainErr := readRegister(mainPath)
	if mainErr == nil {
		return &Store{path: path, reg: reg}, LoadResult{Source: Main}, nil
	}
	if !errors.Is(mainErr, os.ErrNotExist) {
		// Keep the evidence before touching anything else.
		copyAside(mainPath)
	}

	bakReg, bakErr := readRegister(bakPath)
	if bakErr == nil {
		return &Store{path: path, reg: bakReg}, LoadResult{
			Source:  Backup,
			Warning: backupWarning(bakReg),
		}, nil
	}

	if errors.Is(mainErr, os.ErrNotExist) && errors.Is(bakErr, os.ErrNotExist) {
		empty := &register.Register{SchemaVersion: register.SchemaVersion}
		normalise(empty)
		return &Store{path: path, reg: empty}, LoadResult{Source: Fresh}, nil
	}

	// The wording is fixed by 02-persistence.spec.md. This is console-only —
	// no browser is open yet — and it is read at the worst possible moment, so
	// it says the data is untouched and who to call, and never shows the parse
	// error. The two paths go on their own lines to be read out or copied.
	return nil, LoadResult{}, fmt.Errorf(
		"The register could not be opened. Nothing has been changed. "+
			"Call whoever set this up and give them these two files:\n%s\n%s",
		mainPath, bakPath)
}

// Read hands fn the register for read-only use. Callers must not retain the pointer.
func (s *Store) Read(fn func(*register.Register)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.reg)
}

// Update applies fn to the register and saves it. If fn or the save fails, the
// register in memory is restored to what it was and the error is returned
// unchanged: the caller's mutation is invisible afterwards, on disk and in memory.
func (s *Store) Update(fn func(*register.Register) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := deepCopy(s.reg)
	if err := fn(s.reg); err != nil {
		s.reg = snapshot
		return err
	}
	// Ordinary inventory work never opens or rewrites the protected envelope.
	// Restore it even if a future inventory callback accidentally touches it.
	s.reg.Finance = snapshot.Finance
	if err := s.save(); err != nil {
		s.reg = snapshot
		return err
	}
	return nil
}

// save writes the register in exactly this order: temp file, sync, close,
// rename the current file to .bak, rename the temp over the current file.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpPath := s.path + tempSuffix
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	bakPath := s.path + backupSuffix
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Remove(bakPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(s.path, bakPath); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return err
	}

	// Best effort: Windows cannot sync a directory handle.
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// readRegister reads and parses one file. A missing file reports os.ErrNotExist.
func readRegister(path string) (*register.Register, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reg register.Register
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("%s is not a readable register: %w", path, err)
	}
	// Every schema this program has ever written is still read, and read with
	// every record intact. Only an older exe opening a newer file refuses.
	if reg.SchemaVersion != 1 && reg.SchemaVersion != 2 && reg.SchemaVersion != 3 && reg.SchemaVersion != register.SchemaVersion {
		return nil, fmt.Errorf("%s was written by a different version of this program (schema %d)", path, reg.SchemaVersion)
	}
	reg.SchemaVersion = register.SchemaVersion
	normalise(&reg)
	return &reg, nil
}

// copyAside keeps an unreadable file for the operator. It never deletes or
// overwrites the original, and a failure here must not stop the program starting.
func copyAside(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	stamp := strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "-")
	_ = os.WriteFile(path+corruptPrefix+stamp, data, 0600)
}

// backupWarning names the time the backup was last written to, so the reader
// knows exactly what has to be entered again.
func backupWarning(reg *register.Register) string {
	stamp := "an unknown time"
	if last, ok := lastEntryTime(reg); ok {
		stamp = last.Format("3:04 pm on 2 January 2006")
	}
	return "Today's register was damaged. This is the last good copy, saved at " +
		stamp + ". Anything entered after that time must be entered again."
}

func lastEntryTime(reg *register.Register) (time.Time, bool) {
	var last time.Time
	ok := false
	note := func(t time.Time) {
		if !ok || t.After(last) {
			last, ok = t, true
		}
	}
	for _, in := range reg.Inwards {
		note(in.RecordedAt)
	}
	for _, is := range reg.Issues {
		note(is.RecordedAt)
	}
	for _, re := range reg.Returns {
		note(re.RecordedAt)
	}
	return last, ok
}

// normalise turns the nil slices of a hand-edited file into empty slices, so
// the file always shows [] rather than null.
func normalise(reg *register.Register) {
	if reg.Products == nil {
		reg.Products = []register.Product{}
	}
	if reg.Staff == nil {
		reg.Staff = []register.Staff{}
	}
	if reg.Inwards == nil {
		reg.Inwards = []register.Inward{}
	}
	if reg.Issues == nil {
		reg.Issues = []register.Issue{}
	}
	if reg.Returns == nil {
		reg.Returns = []register.Return{}
	}
	// A schema-3 file written before disposals existed has no key for them.
	if reg.Disposals == nil {
		reg.Disposals = []register.InventoryDisposal{}
	}
	// AcquisitionKinds is deliberately left alone. A file written before typed
	// kinds existed has no key for them, and is written back without one.
}

// deepCopy copies every slice into a fresh backing array, so an in-place edit of
// an element cannot leak into the snapshot.
func deepCopy(r *register.Register) *register.Register {
	c := *r
	if r.ShiftStartedAt != nil {
		t := *r.ShiftStartedAt
		c.ShiftStartedAt = &t
	}
	c.Products = append([]register.Product{}, r.Products...)
	for i := range c.Products {
		c.Products[i].Changes = copyChanges(c.Products[i].Changes)
		c.Products[i].Deleted = copyDeletion(c.Products[i].Deleted)
	}
	c.Staff = append([]register.Staff{}, r.Staff...)
	if r.AcquisitionKinds != nil {
		c.AcquisitionKinds = append([]register.AcquisitionKind{}, r.AcquisitionKinds...)
		for i := range c.AcquisitionKinds {
			c.AcquisitionKinds[i].Changes = copyChanges(c.AcquisitionKinds[i].Changes)
		}
	}

	c.Inwards = append([]register.Inward{}, r.Inwards...)
	for i := range c.Inwards {
		c.Inwards[i].Changes = copyChanges(c.Inwards[i].Changes)
		c.Inwards[i].Deleted = copyDeletion(c.Inwards[i].Deleted)
	}
	c.Issues = append([]register.Issue{}, r.Issues...)
	for i := range c.Issues {
		c.Issues[i].AdditionalTakers = append([]register.IssueRecipient(nil), c.Issues[i].AdditionalTakers...)
		c.Issues[i].Changes = copyChanges(c.Issues[i].Changes)
		c.Issues[i].Deleted = copyDeletion(c.Issues[i].Deleted)
	}
	c.Returns = append([]register.Return{}, r.Returns...)
	for i := range c.Returns {
		c.Returns[i].Allocations = append([]register.Allocation(nil), c.Returns[i].Allocations...)
		c.Returns[i].Changes = copyChanges(c.Returns[i].Changes)
		c.Returns[i].Deleted = copyDeletion(c.Returns[i].Deleted)
	}
	c.Disposals = append([]register.InventoryDisposal{}, r.Disposals...)
	for i := range c.Disposals {
		c.Disposals[i].Sources = append([]register.DisposalAllocation{}, r.Disposals[i].Sources...)
		if r.Disposals[i].InactiveAt != nil {
			t := *r.Disposals[i].InactiveAt
			c.Disposals[i].InactiveAt = &t
		}
	}
	if r.Finance != nil {
		e := *r.Finance
		e.KeySlots = append([]register.FinanceKeySlot{}, r.Finance.KeySlots...)
		for i := range e.KeySlots {
			if e.KeySlots[i].ExpiresAt != nil {
				t := *e.KeySlots[i].ExpiresAt
				e.KeySlots[i].ExpiresAt = &t
			}
		}
		if e.Recovery.ExpiresAt != nil {
			t := *e.Recovery.ExpiresAt
			e.Recovery.ExpiresAt = &t
		}
		c.Finance = &e
	}
	return &c
}

func copyChanges(in []register.Change) []register.Change {
	if in == nil {
		return nil
	}
	return append([]register.Change(nil), in...)
}

func copyDeletion(in *register.Deletion) *register.Deletion {
	if in == nil {
		return nil
	}
	d := *in
	return &d
}
