# Spec: Atomic Persistence and Crash Recovery

## Objective

A day's entries at a live gathering cannot be recreated. Every save must leave the
disk holding either the complete register as it was before, or the complete register
as it is now — never half of either. If the good file is gone when the program next
starts, it must come up on yesterday's backup and say so on screen rather than
starting empty and pretending nothing happened.

## Context

- Owns `internal/store/store.go` and its tests.
- Depends on `01-data-model.spec.md` for `register.Register`.
- Standard library only: `os`, `path/filepath`, `encoding/json`, `sync`.
- The web layer never touches disk; it calls this package.

## Contract

### Paths

```go
func DataPath() (string, error)   // <dir of os.Executable()>/store-register.json
```

Resolved from `os.Executable()` and `filepath.EvalSymlinks`, **never** from the
working directory — on Windows a desktop shortcut sets the working directory
somewhere else and the register would silently move.

Three files live in that directory:

| Path | Role |
|---|---|
| `store-register.json` | the register |
| `store-register.json.bak` | the previous good register |
| `store-register.json.tmp` | in flight during a save; never read at startup |

### Store

```go
type Store struct { /* unexported: path string; mu sync.Mutex; reg *register.Register */ }

func Open(path string) (*Store, LoadResult, error)
func (s *Store) Update(fn func(*register.Register) error) error
func (s *Store) Read(fn func(*register.Register))
```

`Update` takes the mutex, then — **unconditionally, before calling `fn`** — takes a
deep copy of the register:

```go
func deepCopy(r *register.Register) *register.Register
```

which copies every slice into a fresh backing array, not a shared one, so an in-place
edit of an element by `fn` cannot leak into the snapshot. It then calls `fn` against
the live register. If `fn` returns nil, the register is written to disk by the sequence
below. If either `fn` or the write fails, the in-memory register is **replaced by the
snapshot** and the error is returned unchanged. Either way the caller's mutation is
invisible after a failure, in memory as well as on disk. The mutex exists because the person at the desk may have two
browser tabs open.

`Read` takes the mutex and hands `fn` the register for read-only use. Callers must
not retain the pointer.

### The save sequence — exactly this order

1. `json.MarshalIndent(reg, "", "  ")`, append `'\n'`.
2. Create `store-register.json.tmp` (`os.OpenFile`, `O_WRONLY|O_CREATE|O_TRUNC`, mode `0600`).
3. Write all bytes.
4. `f.Sync()`.
5. `f.Close()`.
6. If `store-register.json` exists, `os.Rename` it to `store-register.json.bak`,
   removing any existing `.bak` first if the platform requires it (Windows `os.Rename`
   over an existing file works on Go's implementation; call `os.Remove` on the `.bak`
   first anyway and ignore `ErrNotExist`).
7. `os.Rename` the `.tmp` over `store-register.json`.
8. Open the containing directory and `Sync()` it, ignoring the error on Windows where
   directory handles cannot be synced.

Crash outcomes this guarantees:

| Crash after step | On disk | Startup result |
|---|---|---|
| 1–5 | old main intact, stray `.tmp` | old register loads; `.tmp` ignored and overwritten next save |
| 6 | no main, `.bak` = old, `.tmp` = new | `.bak` loads, `RecoveredFromBackup` reported |
| 7–8 | new main, `.bak` = old | new register loads |

### Loading

```go
type LoadResult struct {
    Source  LoadSource // Fresh | Main | Backup
    Warning string     // "" unless Source == Backup or a file was unreadable
}
```

`Open` tries in order:

1. Read `store-register.json`. If it reads and `json.Unmarshal` succeeds and
   `SchemaVersion == 1`, that is the register. `Source = Main`.
2. Otherwise read `store-register.json.bak` under the same rules. On success,
   `Source = Backup` and `Warning` is set (wording in Open, below).
3. Otherwise, if **neither file exists**, return an empty register seeded per
   `06-shift-and-people.spec.md` with `SchemaVersion: 1` and empty slices.
   `Source = Fresh`.
4. Otherwise — a file exists but neither it nor the backup can be parsed — return an
   error. The program must not start, must not overwrite anything, and must print the
   path of both files to the console so they can be rescued by hand.

A parse failure of the main file must never delete or overwrite it. The failed file is
copied to `store-register.json.corrupt-<RFC3339 with colons replaced by dashes>`
before the backup is tried, so the operator keeps the evidence.

`nil` slices from a hand-edited file are normalised to empty slices at load.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/store/store.go`
- `/home/asim/Projects/inventory-management/internal/store/store_test.go`
- `/home/asim/Projects/inventory-management/internal/store/testdata/walkthrough-t0.json`

## Required tests

All tests use `t.TempDir()`. None may touch the real executable directory.

`TestSaveThenLoadRoundTrip` — save `register.WalkthroughT0()`, reopen the directory,
and the loaded register is `reflect.DeepEqual` to the fixture. `Source == Main`.

`TestOnDiskFixtureMatchesCode` — `testdata/walkthrough-t0.json` unmarshals to a
register `reflect.DeepEqual` to `register.WalkthroughT0()`, and
`json.MarshalIndent(register.WalkthroughT0(), "", "  ") + "\n"` is byte-identical to
the file. This pins the file format: any struct-tag change breaks this test.

`TestFileIsHumanReadable` — after saving T0, the raw bytes contain the lines
`  "schemaVersion": 1,` and `      "name": "Sharma Tent House",`-style indentation
(two-space, nested), contain the literal `Sharma Tent House`, `Ravi Menon` and
`Water drums (20L)`, and end with a single `\n`.

`TestBackupHoldsPreviousVersion` — save T0; then `Update` appending `INW-0007`
(500 chairs, Sharma Tent House, challan `STH/4471`). Assert `store-register.json`
parses with 7 inwards and `store-register.json.bak` parses with 6, and that the `.bak`
chair total is 700 while the main file's is 1200.

`TestCrashBetweenBackupAndRename` — simulate step 6 completing and step 7 not: write
T0, save T1, then by hand delete `store-register.json` leaving `.bak` (T0) and a valid
`.tmp` (T1). Reopen: the register loads from `.bak` with 6 inwards, `Source == Backup`,
and `Warning != ""`.

`TestCrashDuringTempWrite` — write T0 normally, then write a truncated
`store-register.json.tmp` containing `{"schemaVersion":1,"products":[` and reopen.
The register loads from the main file with 6 inwards, `Source == Main`, `Warning == ""`.
The stray `.tmp` must not be read and must not cause an error.

`TestTruncatedMainFileFallsBackToBackup` — save T0, save T1, then overwrite
`store-register.json` with the first 40 bytes of itself. Reopen: 6 inwards load from
`.bak`, `Source == Backup`, `Warning != ""`, and a
`store-register.json.corrupt-*` file exists containing those 40 bytes.

`TestBothFilesCorruptRefusesToStart` — both main and `.bak` contain `not json at all`.
`Open` returns a non-nil error; both files are still on disk byte-for-byte unchanged
afterwards.

`TestFreshDirectoryStartsEmpty` — empty temp dir. `Source == Fresh`, `SchemaVersion == 1`,
`len(Products) == 0`, and no file is written until the first `Update`.

`TestWrongSchemaVersionRefused` — main file with `"schemaVersion": 2` and a valid `.bak`
at version 1 falls back to the backup with a warning; with no `.bak`, `Open` errors.

`TestUpdateErrorWritesNothing` — save T0; call `Update` with a function that appends
`ISS-0008` (10 chairs to Ravi Menon) and then returns
`errors.New("890 on hand, cannot issue 900")`. Assert the file on disk still has 7
issues, no `.bak` was created by this call, and a subsequent `Read` sees 7 issues —
the failed mutation is not visible in memory either.

`TestDeepCopyShareNoBackingArrays` — `c := deepCopy(register.WalkthroughT0())`; mutate
`c.Issues[0].Quantity = 1` and `c.Returns = append(c.Returns, ...)` and
`c.Inwards[0].Supplier = "Changed"`; the original still reads 150, 0 returns and
`Sharma Tent House`. Then the same in the other direction.

`TestUpdateRollsBackInMemoryAfterWriteFailure` — save T0, then make the directory
unwritable (`os.Chmod(dir, 0500)`), then `Update` appending `INW-0007`. The call
returns an error and a following `Read` sees 6 inwards, not 7. Skipped when running as
root.

`TestConcurrentUpdatesAreSerialised` — from 50 goroutines, each `Update` appends one
issue of 1 chair to Lakshmi Iyer. After `sync.WaitGroup` completes, the file on disk
has exactly 57 issues and 50 distinct IDs `ISS-0008` … `ISS-0057`.

## Acceptance criteria

1. `go test ./internal/store/ -count=1` passes; all fourteen tests above exist by name.
2. `grep -n 'os.Rename' internal/store/store.go` shows exactly two calls, and the
   `.bak` rename appears before the `.tmp` rename in file order.
3. `grep -n 'Sync()' internal/store/store.go` shows the file sync occurring before
   either rename.
4. `grep -rn 'os.Getwd\|"\./' internal/store/store.go` returns nothing.
5. `go test ./internal/store/ -race -count=1` passes.
6. After `TestBothFilesCorruptRefusesToStart`, an `os.Stat` comparison in the test
   proves both files' size and content are unchanged.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/store/ -race -count=1 -v
go vet ./internal/store/
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

## Open

1. **The on-screen wording when the backup is used.** The walkthrough says only "say so
   plainly on screen" — it gives no sentence. Settled, as a red banner at the top of
   every page until the process restarts (spec 04 governs; there is no dismiss
   control):
   `Today's register was damaged. This is the last good copy, saved at <time of the last entry in the backup>. Anything entered after that time must be entered again.`
   The banner states the timestamp it recovered to. The earlier draft asked the
   reader to "check the newest entries are all here", which is exactly the recall the
   whole design forbids — the person reading it may have just walked in for their
   shift and never saw those entries.
2. **The wording when both files fail** (console only, no browser is open yet).
   Settled: `The register could not be opened. Nothing has been changed. Call whoever set this up and give them these two files:` then the two paths on their own lines.
3. **Retention of `.corrupt-*` files.** Recommend never deleting them automatically.
