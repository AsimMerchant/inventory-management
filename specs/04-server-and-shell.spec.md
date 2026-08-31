# Spec: Server Startup, Port Selection, Browser Launch, and the Page Shell

## Objective

Someone double-clicks one file on a Windows 11 laptop and the store register opens in
their browser. No installer, no firewall prompt, no port to type, no black window to
understand. If the browser does not open by itself, the address is sitting there in
the console to be typed by hand.

## Context

- Owns `main.go`, `internal/web/server.go`, `internal/web/shell.go`,
  `internal/web/templates/layout.html`, `internal/web/static/`.
- Depends on `02-persistence.spec.md` for `store.Open`. The routing table below is
  filled in by specs 05–11. `{id}` uses Go 1.22+ pattern routing
  (`mux.HandleFunc("GET /entry/{id}/edit", ...)`).
- `html/template` with `embed.FS`. No JavaScript framework, no CDN — the laptop may
  have no internet.

## Contract

### Startup sequence in `main.go`

1. Resolve the data path from `os.Executable()` (`store.DataPath()`).
2. `store.Open(path)`. On error: print the message from `02` §Open item 2 to stdout,
   print `Press Enter to close.`, wait for a line on stdin, exit with status 1. A
   Windows user double-clicking must be able to read the message before the window
   disappears.
3. Choose a port (below).
4. Print to stdout, exactly:
   ```
     Open this address in your browser:
     http://127.0.0.1:8765

   Leave this window open. If you close it, the register stops.
   ```
   (with the real port substituted). The address comes first deliberately: a reader
   who sees a line like `Store Register is running.` looks away satisfied and never
   reaches the warning below.
5. Start the HTTP server in a goroutine, then open the browser.
6. Block until the process is killed.

### Port selection

```go
func listen() (net.Listener, error)
```

Try `127.0.0.1:8765`, then 8766 … up to and including 8785. Return the first
`net.Listen("tcp", ...)` that succeeds. If all 21 are busy, print
`The Store Register may already be open. Look at your browser tabs before starting it again.`
and exit 1.

**The address is always `127.0.0.1:PORT`. `0.0.0.0`, `:PORT` and `localhost` are all
forbidden** — the first two raise the Windows Defender Firewall dialog, and a
non-technical user faced with a security prompt is a failed handover.

### Browser launch

```go
func openBrowser(url string) error
```

- `windows`: `exec.Command("rundll32", "url.dll,FileProtocolHandler", url)`
- `linux`: `exec.Command("xdg-open", url)` (development only)
- `darwin`: `exec.Command("open", url)`

A failure is logged as one line — `Open your browser and type: http://127.0.0.1:8765` — and never stops the server.

### Server

`http.Server` with `ReadHeaderTimeout: 5s`. No TLS. No graceful-shutdown machinery:
this program is stopped by closing its window.

### Routing table

| Method | Path | Spec |
|---|---|---|
| GET | `/` | redirect: `/shift` if no shift started, else `/stock` |
| GET | `/shift` | 05 |
| POST | `/shift/start` | 05 |
| POST | `/shift/person` | 05 |
| GET | `/stock` | 10 |
| GET | `/out` | 10 |
| GET | `/inwards` | 10 |
| GET | `/suppliers` | 10 |
| GET | `/log` | 12 |
| GET, POST | `/inward/new` | 07 |
| GET, POST | `/issue/new` | 08 |
| GET, POST | `/return/new` | 09 |
| POST | `/product/new` | 06 |
| GET | `/api/products` | 06 |
| GET | `/api/people` | 08 |
| GET, POST | `/entry/{id}/edit` | 11 |
| POST | `/entry/{id}/delete` | 11 |
| GET | `/static/*` | this spec |

Any other path returns 404 with the shell and the sentence
`Nothing here. Go back to the register.` linking to `/`.

### The shell

Every page renders through `layout.html`:

- A chrome bar: a dot, the page title, and on the right
  `Suresh Kumar · on duty` — the on-duty person's name, from the register.
- Five tabs, in this order and with exactly these labels:
  **Stock**, **Out with people**, **Stuff came in**, **Suppliers**, **Who did what**,
  linking to `/stock`, `/out`, `/inwards`, `/suppliers`, `/log`. The current one carries
  the `on` class. The fifth is specified in `12-activity-log.spec.md`, which also
  records the measurement showing five labels fit the `52rem` shell without wrapping.
- Flow pages (`/inward/new`, `/issue/new`, `/return/new`) show the chrome bar with
  their own title — `Stuff came in`, `Someone is taking`, `Someone is returning` —
  and no tabs.
- A slot above the page content for banners, rendered in the order: recovery warning
  (from `LoadResult.Warning`, red, on every page until the process restarts — there is
  no dismiss control, and spec 02 is corrected to match this, not the reverse), then
  page banners.
- The CSS is the one embedded in the walkthrough document, extracted verbatim to
  `internal/web/static/app.css`, including the `prefers-color-scheme` dark block. Class
  names in the templates match the walkthrough's markup: `screen`, `chrome`, `tabs`,
  `tab`, `body`, `field`, `lab`, `inp`, `hint`, `seg`, `opt`, `btn`, `banner`, `tile`,
  `stk`, `pill`, `outrow`, `sug`.

### Guard

Every route except `/shift`, `/shift/start`, `/shift/person` and `/static/*` redirects
(303) to `/shift` when `OnDutyStaffID` is empty. No entry can be made without a name
attached to it.

### Date and time rendering

Three helpers registered as template functions:

- `longstamp` — `Monday, 2 January · 3:04 pm` → `Thursday, 3 September · 10:42 am`,
  used for the screen subtitle on every flow page.
- `clock` — `3:04 pm` → `9:40 am`, used in outstanding lines.
- `shortdate` — `2 January` → `3 September`, used by the `date received` correction
  line in `11-corrections.spec.md` and the `Received on ...` line in
  `12-activity-log.spec.md`. It lives here so the two specs share one rendering rather
  than each formatting a date of its own.

## Files to create or modify

- `/home/asim/Projects/inventory-management/main.go`
- `/home/asim/Projects/inventory-management/internal/web/server.go`
- `/home/asim/Projects/inventory-management/internal/web/shell.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/layout.html`
- `/home/asim/Projects/inventory-management/internal/web/static/app.css`
- `/home/asim/Projects/inventory-management/internal/web/server_test.go`
- `/home/asim/Projects/inventory-management/internal/web/testhelp_test.go` — the shared
  test helpers every web spec uses: `newTestServer(t, reg, now)` returning an
  `httptest.Server` over a `store.Store` in `t.TempDir()`, and
  `assertContains(t, body, want)`, which compares against
  `html.UnescapeString(body)` per convention 2 in `00-index.spec.md`. Without the
  unescape, every assertion containing an apostrophe (`Joseph D'Cruz`,
  `Leave blank if you don't know.`) fails against correct output.

## Required tests

`TestListenBindsLoopbackOnly` — call `listen()`, assert the resulting
`Listener.Addr().String()` starts with `127.0.0.1:` and the port is 8765 when free.

`TestListenSkipsBusyPort` — occupy 8765 and 8766 with two listeners, call `listen()`,
assert the returned port is 8767.

`TestListenGivesUpAfterRange` — occupy 8765–8785, assert `listen()` returns an error
naming the range.

`TestConsoleBannerContainsAddress` — the function that formats the console message,
given port 8767, produces a string containing `http://127.0.0.1:8767` and the line
`Leave this window open. If you close it, the register stops.`

`TestUnknownPathIs404WithPlainSentence` — `GET /nowhere` on an `httptest` server built
over `register.WalkthroughT0()` returns 404 and a body containing
`Nothing here.` and no stack trace, no Go type name, and not the word
`invalid`.

`TestNoShiftRedirectsToShift` — with `OnDutyStaffID` empty, `GET /stock`,
`GET /issue/new` and `GET /return/new` each return 303 with `Location: /shift`, while
`GET /shift` returns 200.

`TestShellShowsOnDutyName` — with `WalkthroughT0()` (Suresh Kumar on duty),
`GET /stock` body contains `Suresh Kumar · on duty` and the five tab labels
`Stock`, `Out with people`, `Stuff came in`, `Suppliers`, `Who did what`.

`TestRecoveryWarningShowsOnEveryPage` — a server constructed with a non-empty
`LoadResult.Warning` renders that warning inside a `banner bad` element on `/stock`,
`/out`, `/inwards`, `/suppliers` and `/log`.

`TestLongstampMatchesWalkthrough` — `longstamp(2026-09-03T10:42:00+05:30)` equals
`Thursday, 3 September · 10:42 am`; `longstamp` of 14:18 equals
`Thursday, 3 September · 2:18 pm`; `longstamp` of 18:05 equals
`Thursday, 3 September · 6:05 pm`. `clock(2026-09-03T09:40:00+05:30)` equals `9:40 am`.
`shortdate(2026-09-03T10:42:00+05:30)` equals `3 September` and
`shortdate(2026-09-04T00:05:00+05:30)` equals `4 September`.

`TestAllTemplatesParse` — `template.ParseFS` over the whole embedded template
directory succeeds, and every template named in the routing table exists.

`TestStaticAssetsEmbedded` — `GET /static/app.css` returns 200, a body over 2000 bytes,
and the body contains `prefers-color-scheme`. Assert the embedded CSS contains no
`http://` or `https://` URL — nothing may be fetched from the internet.

## Acceptance criteria

1. `grep -rn --include=*.go --exclude=*_test.go '0\.0\.0\.0\|net.Listen("tcp", ":' main.go internal/web/` returns nothing.
2. `grep -rn '127\.0\.0\.1' main.go internal/web/server.go` returns at least one line.
3. `grep -rn 'https\?://' internal/web/templates/ internal/web/static/` returns nothing.
4. `go test ./internal/web/ -count=1` passes with all eleven tests above.
   `grep -c 'strings.Contains(string(body)' internal/web/*_test.go` returns 0 for every
   file — body assertions go through `assertContains`, which unescapes first.
5. `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe ./...` succeeds and `file /tmp/register.exe` reports a PE32+ executable.
6. `go list -deps ./... | grep -v '^storeregister' | grep '\.'` prints nothing — zero third-party dependencies.
7. Running the binary in an empty directory prints a line matching `^  http://127\.0\.0\.1:87[0-8][0-9]$` within two seconds.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -count=1 -v
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe ./...
grep -rn --include=*.go --exclude=*_test.go '0\.0\.0\.0' .   # must print nothing
```

## Open

1. **The port number 8765.** The walkthrough does not name one. Chosen because it is
   outside the IANA registered range in common use and easy to read aloud. Confirm.
2. **The console text.** Written here because a black window with nothing in it is a
   failure mode; the walkthrough covers only browser screens. Needs plain-language
   review.
3. **Browser launch on Windows.** `rundll32 url.dll,FileProtocolHandler` avoids the
   quoting traps of `cmd /c start` and needs no shell. Cannot be tested from Linux —
   list it under "not verifiable from here" at release.
