# Spec: Protected Finance Vault and Authorized Accounts

## Objective

Add individual, offline financial accounts to the same executable without changing or
locking the ordinary inventory workflow. Financial content is authenticated, encrypted
at rest, and unavailable from every public response and API until a financial user has
logged in.

## Context

- User decisions recorded 2 September 2026: the ordinary desk remains unauthenticated;
  the main screen shows a neutral `Authorized login`; the first financial user is an
  administrator; administrators authorize mobile numbers; each person chooses their own
  password; all financial users see the complete ledger; only administrators manage
  accounts and reusable values; there is no SMS, internet service, or shared PIN.
- This intentionally supersedes the no-authentication and no-money prohibitions in
  `CLAUDE.md`, specs 00, 04, 05, 10 and 12, but only for routes beginning `/finance`.
  Inventory routes, the five inventory tabs and on-duty attribution remain unchanged.
- Spec 15 made schema 2 necessary for product tombstones. A schema-2 executable ignores
  unknown financial fields, so this spec requires schema 3 downgrade protection.
- Go 1.27 provides `crypto/pbkdf2`; no KDF or cipher is implemented by this project.

## Contract

### Inputs

#### Schema 3 and encrypted envelope

Set `register.SchemaVersion = 3`. Keep the existing inventory members in their existing
JSON order and append:

```go
Finance *FinanceEnvelope `json:"finance,omitempty"`
```

The public envelope contains key-wrapping metadata and encrypted bytes, never financial
records or display values:

```go
type FinanceEnvelope struct {
    VaultVersion int              `json:"vaultVersion"` // 1
    KeySlots     []FinanceKeySlot `json:"keySlots"`
    Recovery     FinanceKeySlot   `json:"recovery"`
    Nonce        string           `json:"nonce"`      // base64 RawStdEncoding
    Ciphertext   string           `json:"ciphertext"` // base64 RawStdEncoding
}

type FinanceKeySlot struct {
    AccountID    string `json:"accountId"`
    Kind         string `json:"kind"` // "password" | "setup" | "recovery"
    MobileHash   string `json:"mobileHash,omitempty"`
    Salt         string `json:"salt,omitempty"`
    Iterations   int    `json:"iterations,omitempty"`
    Nonce        string `json:"nonce"`
    WrappedKey   string `json:"wrappedKey"`
    ExpiresAt    *time.Time `json:"expiresAt,omitempty"` // setup only
}
```

`MobileHash` is lowercase hex SHA-256 of `register.MobileKey(mobile)` and is only a
lookup hint. A mobile number, display name, role, status, order, amount, party, purpose,
mode, audit record and financial timestamp must not occur in plaintext anywhere in
`store-register.json`. The number of slots and ciphertext length are not secret.

The decrypted `FinanceData` begins in this spec; specs 18–20 append their owned slices
in order so each implementation stage compiles:

```go
type FinanceData struct {
    Accounts           []FinanceAccount    `json:"accounts"`
    Audit              []FinanceAuditEvent `json:"audit"`
    RecoveryConfirmedAt *time.Time          `json:"recoveryConfirmedAt,omitempty"`
}

type FinanceRole string
const (
    FinanceUser  FinanceRole = "user"
    FinanceAdmin FinanceRole = "admin"
)

type FinanceAccount struct {
    ID          string      `json:"id"` // "FAC-0001"
    DisplayName string      `json:"displayName"`
    Mobile      string      `json:"mobile"`
    Role        FinanceRole `json:"role"`
    Status      string      `json:"status"` // "pending" | "active" | "disabled"
    CreatedAt   time.Time   `json:"createdAt"`
    CreatedByID string      `json:"createdById"` // self for FAC-0001
}

type FinanceAuditEvent struct {
    ID          string    `json:"id"` // "FAE-0001"
    At          time.Time `json:"at"`
    ByAccountID string    `json:"byAccountId"`
    ByName      string    `json:"byName"`
    ByMobile    string    `json:"byMobile"`
    Kind        string    `json:"kind"`
    EntityType  string    `json:"entityType"`
    EntityID    string    `json:"entityId"`
    Summary     string    `json:"summary"`
    Before      string    `json:"before,omitempty"`
    After       string    `json:"after,omitempty"`
}
```

Every slice normalizes to `[]`. All IDs are generated with the existing widening
four-digit convention. Mobile identity is `register.MobileKey`; blank or two accounts
with the same non-empty mobile key are refused. Display names use `CleanName`.

Files at schema 1 or 2 load in memory as schema 3 with `Finance == nil`, without an
open-time write. The next successful update writes schema 3 atomically. Schema 3 loads
unchanged. Every other schema is refused. Before deploying this version, copy
`store-register.json` somewhere safe and remove every older executable from the laptop
and pen drive: a v1.1.1 schema-2 reader can fall back to a schema-2 `.bak` and overwrite
schema 3 exactly as documented for the earlier v1.0.1/schema-2 transition.

#### Cryptographic construction

Generate every master key, recovery key, salt, nonce, setup code and session token with
`crypto/rand.Reader`; any short read or error aborts without saving.

- Vault key: 32 random bytes.
- Vault encryption: AES-256-GCM; fresh 12-byte nonce on every encrypted save; exact AAD
  bytes `store-register finance vault v1`; plaintext is `json.Marshal(FinanceData)`.
- Password/setup key: `pbkdf2.Key(sha256.New, secret, salt, 600000, 32)` with a fresh
  16-byte salt. Password and setup slots store `Iterations: 600000`.
- Key wrapping: AES-256-GCM over the 32-byte vault key with a fresh 12-byte nonce. AAD is
  exact UTF-8 `store-register key wrap v1:<accountID>:<kind>`.
- Recovery key: 32 random bytes displayed once as uppercase unpadded base32 in groups of
  four separated by `-`; decoding ignores hyphens and ASCII case. It is used directly as
  the AES-256 wrapping key; the recovery slot has `AccountID: "recovery"`,
  `Kind: "recovery"`, no salt and zero iterations. The recovery key itself is never
  stored or logged.

Every decrypt authenticates GCM before parsing. Wrong password/code/key, changed AAD,
nonce, ciphertext or wrapped key returns the same public sentence
`Those login details did not work.` and reveals no parse or cryptographic error.
Nonces must never repeat for the same key; fresh randomness, not a counter, enforces it.

Ordinary `Store.Update` preserves an existing envelope byte-for-byte at the field level:
it does not decrypt or re-encrypt it. Add store operations which hold the same mutex as
`Update`:

```go
func (s *Store) InitializeFinance(displayName, mobile, password string, now time.Time) (vaultKey []byte, accountID string, recovery string, err error)
func (s *Store) UnlockFinance(mobile, password string) (vaultKey []byte, accountID string, err error)
func (s *Store) UnlockFinanceRecovery(recovery string) ([]byte, error)
func (s *Store) ReadFinance(vaultKey []byte, fn func(*register.FinanceData)) error
func (s *Store) UpdateFinance(vaultKey []byte, fn func(*register.Register, *register.FinanceData) error) error
```

`InitializeFinance` is accepted only while `Finance == nil`; it creates active admin
`FAC-0001`, password/recovery slots and an empty encrypted payload in one atomic save.
`UpdateFinance` deep-copies the full register and decrypted finance data, runs `fn`,
`register.Validate` and `register.ValidateFinance`, encrypts with a fresh nonce and uses
the existing temp/fsync/backup/rename sequence. Any callback, validation, random,
encryption or save error restores exact in-memory and on-disk state.

Every operation that changes a key slot—activation, disable, reset, password change,
recovery, or recovery-key replacement—uses the same private locked transaction:
snapshot register/envelope/decrypted data, mutate slots and payload together, validate,
encrypt with a fresh nonce, atomic save, then publish in memory. No route saves a slot
separately from the account/status/audit change it represents.

#### Account setup and recovery

Routes and exact successful behavior:

| Method | Route | Access | Result |
|---|---|---|---|
| GET, POST | `/finance/setup` | only when no vault | first admin setup |
| POST | `/finance/setup/confirm` | first admin setup session | acknowledge recovery key |
| GET, POST | `/finance/login` | public | login form/session |
| POST | `/finance/logout` | finance session | destroy session; 303 `/stock` |
| GET | `/finance/accounts` | admin | account list and add form |
| POST | `/finance/accounts/new` | admin | authorize mobile; show setup code once |
| POST | `/finance/accounts/{id}/disable` | admin | disable non-last-admin account |
| POST | `/finance/accounts/{id}/reset` | admin | invalidate password slot; show new setup code once |
| GET, POST | `/finance/accounts/{id}/edit` | admin | correct name/mobile/role |
| GET, POST | `/finance/activate` | public | mobile + setup code + own password |
| GET, POST | `/finance/recover` | public | recovery key + active admin mobile + new password |
| GET, POST | `/finance/recovery-key/new` | active admin | replace recovery key; show once |
| GET, POST | `/finance/password` | active finance user | change own password |

First setup fields are `Your name`, `Mobile number`, `Create password`, `Type password
again`; password is at least 8 Unicode characters, both entries must match. After the
atomic save, show the recovery key once with heading `Save this recovery key` and text
`Keep it somewhere safe. It is the only way back in if every administrator forgets
their password.` The user must tick `I saved the recovery key` and POST before entering
`/finance`; no route ever displays that key again.

Immediately after `InitializeFinance`, the web layer creates the first ordinary finance
session from the returned vault key and FAC-0001, marks that in-memory session
`recoveryPending`, and renders the key directly from the return value. The key is not put
in a cookie, URL, hidden input, session object, audit event or second response. The
checkbox form carries the session CSRF token. Successful
`POST /finance/setup/confirm` atomically sets `RecoveryConfirmedAt = now`, clears the
session flag and redirects 303 to `/finance`. Until then that session may access only
the confirmation POST and logout. If the page/session is lost before confirmation, a
later successful admin login redirects to `/finance/recovery-key/new`; that route
replaces the unconfirmed recovery slot with a newly generated key, shows it once under
the same confirmation gate, and invalidates the abandoned key. An active admin may also
use this route deliberately; replacement requires their current password and
invalidates the previous recovery key atomically.

An admin adds `Name`, `Mobile number`, and role choices `Financial user` / `Financial
administrator`. The server generates a 20-character uppercase unpadded base32 setup
code (100 random bits), wraps the vault key in a `setup` slot expiring exactly 24 hours
after creation, and shows the code once. `/finance/activate` requires matching mobile
and unexpired code, then replaces that slot with a password slot and marks the account
active in one save. A reset invalidates every password/setup slot for that account and
creates one new 24-hour setup slot. Admins cannot learn a password. Disabling removes
all its slots and sessions; the last active administrator cannot be disabled or demoted.

Recovery accepts only an active administrator's mobile. It unwraps via `Recovery`,
verifies that account inside the authenticated payload, replaces that administrator's
password slot, and leaves all data and other accounts unchanged. It does not rotate or
display a new recovery key.

Account edit fields are `Name`, `Mobile number`, and role. Name/mobile are cleaned;
mobile remains nonblank/unique. Changing mobile atomically changes its password slot's
`MobileHash` without changing the wrapped key, account ID or password. Changing role is
refused if it would leave no active admin. The user's own password form requires
`Current password`, `New password`, `Type new password again`; it verifies the current
password, creates a fresh salt/password wrapper, invalidates that account's other
sessions, and retains the submitting replacement session. Password text never enters
audit.

Every account create, activation, role/status change, reset and recovery appends a
`FinanceAuditEvent` with immutable account ID plus actor snapshots. Setup/recovery never
stores the submitted secret, password, setup code or recovery key in audit.
Account event `Kind` values are exactly `account_created`, `account_activated`,
`account_edited`, `account_disabled`, `account_reset`, `account_recovered`,
`password_changed`, and `recovery_key_replaced`; `EntityType` is `account` except the
last (`recovery`). `Before`/`After` contain only the changed display/mobile/role/status,
never a credential or key-slot value.

#### Sessions, CSRF and throttling

Successful login creates a 32-random-byte base64url session token held only in memory;
the session holds a private copy of the vault key, account ID, creation time and last
activity. Cookie `store_finance_session` is `HttpOnly`, `SameSite=Strict`, `Path=/`, and
has no `Secure` flag because the server is loopback HTTP. Session tokens and vault keys
never enter URLs, HTML, JSON or logs. Restart logs everyone out.

Idle expiry is 15 minutes. On each authenticated financial request, expire when
`now.Sub(lastActivity) >= 15*time.Minute`; otherwise update last activity. Expired or
unknown sessions redirect GET/HEAD 303 to `/finance/login?expired=1`; non-GET financial
requests return 403 and write nothing. The login page says `You were logged out because
the computer was left idle.` for `expired=1`.

Every state-changing `/finance` form carries a session-bound 32-random-byte CSRF token.
Missing/wrong token returns 403 with `This page expired. Go back and try again.` and no
write. Login/setup/activation/recovery use a separate SameSite=Strict pre-auth cookie
and matching form token. Compare tokens with `crypto/subtle.ConstantTimeCompare`.

After five failed login/activation/recovery submissions for one normalized mobile from
one process within 15 minutes, refuse further attempts for that mobile until 15 minutes
after the fifth failure with `Too many attempts. Wait 15 minutes and try again.` Correct
credentials during lockout do not bypass it. A successful attempt clears that mobile's
failures. Throttle state is in memory and contains no password or recovery key.

#### Public and authenticated shell

The shared public chrome adds one ordinary link `Authorized login` to
`/finance/login`. Keep the five inventory tabs unchanged. Before authentication, no
response outside `/finance/login`, `/finance/setup`, `/finance/activate` and
`/finance/recover` contains `Financial ledger`, currency, amount, party, order, purpose,
payment mode, account mobile, or any decrypted suggestion.

After login, the shared chrome additionally shows `Financial ledger` → `/finance` and a
prominent POST button `Logout`. Administrators also see `Authorized people` →
`/finance/accounts`. Finance pages work with no on-duty inventory shift; the shift guard
must not intercept `/finance/*`. An unauthenticated direct request to any other finance
GET redirects 303 to login; an unauthenticated finance API or mutation returns 403 and
an empty/non-sensitive body. Authorization is checked on the server on every request;
hiding markup is never the guard.

The public login page itself is neutral: heading `Authorized login`, fields `Mobile
number` and `Password`, button `Log in`, and links `Set up authorized access` (only when
no vault exists), `Activate my account`, and `Recover access`. Setup/activation/recovery
pages use only `authorized access/account` wording until authentication succeeds; none
contains `financial`, `money`, `payment`, `ledger`, `order`, `supplier`, an amount or a
protected value. Finance-only JavaScript/CSS is served under `/finance/static/*` through
the same session guard; unauthenticated requests receive no source containing financial
labels.

### Outputs

- Ordinary inventory remains available without a financial account or session.
- Only an active account can decrypt financial data; every financial user sees the full
  protected area, while account/shared-value management requires an admin.
- The JSON remains human-readable for inventory content but exposes no financial value.
- Passwords are never stored; recovery works offline without SMS or internet.

### Side effects

- Successful setup/account/security operations use one atomic save and append audit.
- Login/logout/session expiry and failed authentication do not rewrite the register.
- Static assets and ordinary inventory GETs never open the vault.

## Files to create or modify

- `internal/register/model.go`, `finance_model.go`, `finance_validate.go` — envelope,
  decrypted account/audit types and validation.
- `internal/store/store.go`, `finance.go` and tests — migration, crypto, rollback.
- `internal/web/server.go`, `shell.go`, `finance_auth.go`, templates and tests — guards,
  account flows, sessions and CSRF.
- `main.go` only if server construction needs the new session clock/random source.

## Required tests

`TestSchemaTwoLoadsAsThreeWithoutWriting` — load a real schema-2 v1.1.1 fixture; bytes
and `.bak` are unchanged, inventory is identical, `Finance == nil`, in-memory version is
3, and the next ordinary save is schema 3.

`TestFinanceVaultRoundTripContainsNoPlaintext` — initialize Asha Mehta / 98861 40023,
append an audit summary `Protected vault marker 7QX9`, save/reopen/unlock, and recover all
values; raw main and backup contain none of `Asha`, `98861`, or
`Protected vault marker 7QX9`. Specs 19/21 repeat this with actual amounts/parties.

`TestFinanceUsesFreshAuthenticatedEncryption` — two saves of unchanged finance produce
different valid nonces/ciphertexts; changed nonce/ciphertext/AAD/wrapped key each refuses
without returning partial data.

`TestWrongPasswordAndRecoveryAreIndistinguishable` — wrong password, setup code and
recovery key return the same public error and expose no crypto/parser text.

`TestFinanceUpdateFailureRollsBackEverything` — callback error, invalid finance state,
random failure and forced save failure each leave register, envelope, main and backup at
their exact pre-call values.

`TestFirstAccountIsAdministratorAndRecoveryShownOnce` — exact fields/strings, active
FAC-0001 admin, authenticated recovery-pending session, CSRF-protected acknowledgement,
persisted confirmation time, and no later route/file/session contains the recovery key.

`TestLostRecoveryConfirmationRotatesBeforeLedgerAccess` — lose the initial session,
log in as FAC-0001, verify every ledger route redirects to replacement-key setup, replace
and acknowledge it, verify the abandoned key fails and normal ledger access begins.

`TestAdminAuthorizesAndPersonChoosesPassword` — Asha authorizes Rohan Das / 99001 34562
as Financial user; Rohan activates within 24 hours and logs in with his own password;
Asha's password cannot log into Rohan's account.

`TestExpiredOrReplacedSetupCodeCannotActivate` — expiry boundary, reset invalidation and
one-time consumption each refuse without changing account/data.

`TestAdminResetAndOfflineRecovery` — admin reset cannot reveal the old password; recovery
key resets an active admin and preserves every ledger byte after decrypt.

`TestAccountCorrectionAndOwnPasswordChangeAreAtomic` — admin corrects Rohan's name,
mobile and role with exact before/after audit; login moves to the new mobile; Rohan
changes his own password, old password/sessions fail, submitting session remains, and no
credential appears in audit/raw JSON.

`TestCannotDisableLastAdministrator` — exact refusal `Keep at least one financial
administrator active.`; adding a second admin then permits disabling the first and
immediately removes their sessions.

`TestFinanceAuthorizationIsServerEnforced` — unauthenticated/user/admin matrix over all
routes and mutation handlers; user cannot manage accounts or reusable values even with a
hand-built POST.

`TestFinanceSessionCookieLogoutAndIdleExpiry` — cookie flags, authenticated chrome,
logout, 14:59 activity, exact 15:00 expiry, expired banner and no post-expiry write.

`TestFinanceCSRFAndThrottle` — missing/wrong CSRF refuses every mutation; five failed
logins lock only that mobile for 15 minutes and successful login after boundary clears it.

`TestPublicPagesLeakNoFinanceContent` — with a populated unlocked vault, crawl every
ordinary route/API and assert only `Authorized login` appears; no protected strings,
financial navigation, amount, party, purpose, mode or account mobile appears.

## Acceptance criteria

1. Schema 1/2 migration, schema 3 save and downgrade warning procedure are tested.
2. AES-256-GCM and Go's `crypto/pbkdf2` are the only vault/password constructions; all
   random values come from `crypto/rand` and all authentication precedes parsing.
3. Raw main/backup/corrupt copies contain no decrypted financial field or value.
4. Every finance route passes the session/role/CSRF matrix and idle expiry tests.
5. Existing inventory tests and unauthenticated workflows remain unchanged.
6. No third-party dependency, network call, TLS listener or non-loopback listener exists.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/store/ ./internal/register/ -run 'TestSchemaTwo|TestFinance' -race -count=1 -v
go test ./internal/web/ -run 'TestFirstAccount|TestAdmin|TestExpired|TestCannotDisable|TestFinanceAuthorization|TestFinanceSession|TestFinanceCSRF|TestPublicPages' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'crypto/(aes|cipher|pbkdf2|rand|sha256|subtle)' internal
rg -n 'math/rand|crypto/des|crypto/rc4|NewCBC|NewCTR' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n '"displayName"|"mobile"|Sharma Events|"amountPaise":500000|"purposeId"|"modeId"' store-register.json 2>/dev/null # must print nothing
rg -n 'net/http' internal/register internal/store # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. Windows file permissions cannot protect the JSON from a Windows administrator or
   malware. This contract protects financial confidentiality when the file is casually
   opened/copied and when an unauthenticated person uses the app; it does not claim to
   harden the laptop operating system.
2. The 15-minute timeout and eight-character minimum are engineering defaults because
   the user required inactivity expiry/passwords but did not choose exact values. Change
   them only by amending the tests and contract together.
