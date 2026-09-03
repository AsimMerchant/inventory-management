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
- Spec 15 made schema 2 necessary for product tombstones; the vault and later shared
  vocabularies advanced the file through schemas 3–5. Older executables do not safely
  understand the current file, so this spec requires schema-5 downgrade protection.
- Go 1.27 provides `crypto/pbkdf2`; no KDF or cipher is implemented by this project.

## Contract

### Inputs

#### Schema 5, shared public parties and encrypted envelope

Set `register.SchemaVersion = 5`. Schema 3 introduced the finance envelope and schema 4
introduced public acquisition kinds. Schema 5 adds the one supplier/other-party
vocabulary required by both the unauthenticated delivery desk and authenticated finance
screens. Keep the existing inventory members in their existing JSON order and include:

```go
Parties []Party          `json:"parties,omitempty"`
Finance *FinanceEnvelope `json:"finance,omitempty"`

type Party struct {
    ID            string   `json:"id"`
    Name          string   `json:"name"`
    PreviousNames []string `json:"previousNames,omitempty"`
    MergedIntoID  string   `json:"mergedIntoId,omitempty"`
}
```

Add field `PartyID` with Go type `string`, JSON name `partyId` and `omitempty` to the
existing `Inward`. Retain field `Supplier` with Go type `string` and JSON name `supplier`
unchanged as the historical human-readable snapshot.

Those four fields are the complete public `Party` contract. A newly created party uses
`PRT-0001` and the existing widening four-digit ID convention. A party imported from a
schema-4 vault keeps its `PTY-0001` ID so no encrypted financial reference is rewritten.
Both prefixes are valid party IDs. `Name` is the current cleaned display name;
`PreviousNames` contains names only; `MergedIntoID` points to another public party and
resolvers follow it transitively. Public party JSON must not gain creation time, creator,
account ID, mobile, change actor, change time, audit event, amount, purpose, payment mode,
order/movement/settlement link or any other fact that says money changed hands.

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
lookup hint. A financial account mobile number/display name/role/status, order, amount,
purpose, mode, financial link, audit provenance and financial timestamp must not occur
in plaintext anywhere in `store-register.json`. Public party IDs, current names,
previous names and merge targets are intentional exceptions because inventory staff
must use the same vocabulary without logging in. The existence of a public party must
reveal no financial relationship. The number of slots and ciphertext length are not
secret.

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

Files at schema 1, 2, 3 or 4 load in memory as schema 5 without an open-time write;
schema 5 loads unchanged and every other schema is refused. During that in-memory load,
each nonblank legacy `Inward.Supplier` is cleaned, matched by `FoldKey`, added once to
`Register.Parties` if needed, and assigned to `Inward.PartyID`. Distinct non-equal
spellings such as `Sharma Events` and `Sharma Tents` remain distinct until an admin
combines them. `Inward.Supplier` remains the immutable name snapshot and is never
rewritten by a later rename or combine.

A schema-4 vault may still contain `FinanceReusableValue{Kind: FinanceParty}` rows that
cannot be read during unauthenticated open. An authenticated combined read must import
those rows into a copy of `Register.Parties` so `/api/parties` and every finance picker
offer all existing names before any schema-5 write. The first successful authenticated
finance mutation imports them permanently in the same locked atomic transaction as that
mutation and removes only the party-kind rows from encrypted `ReusableValues`:

- each imported row keeps its `PTY-*` ID and existing `MergedIntoID`;
- every `FinanceOrder.PartyID`, `MoneyMovement.PartyID`,
  `SupplierReturn.PartyID` and `StockSale.BuyerPartyID` remains byte-for-byte unchanged;
- a folded match already created from an inward is retained as the live `PRT-*` row and
  the imported `PTY-*` row points to it, rather than either reference being rewritten;
- earlier value corrections contribute their cleaned `From` names to
  `Party.PreviousNames`, and earlier merge chains continue to resolve;
- creator/account/mobile/timestamps and all finance audit/change provenance remain only
  in encrypted finance data; they are not copied into `Party`.

The import, callback change, cross-boundary validation, fresh vault encryption, schema-5
public data and existing temp/fsync/backup/rename save are one transaction. Any callback,
validation, encryption or save failure leaves memory, main and backup unchanged. The
next successful ordinary or financial save writes schema 5. Before deploying, copy
`store-register.json` somewhere safe and remove every older executable from the laptop
and pen drive because an older reader can fall back to an older-schema `.bak` and later
overwrite newer fields.

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
func (s *Store) ReadBoth(vaultKey []byte, fn func(*register.Register, *register.FinanceData)) error
func (s *Store) UpdateFinance(vaultKey []byte, fn func(*register.Register, *register.FinanceData) error) error
```

`InitializeFinance` is accepted only while `Finance == nil`; it creates active admin
`FAC-0001`, password/recovery slots and an empty encrypted payload in one atomic save.
`ReadBoth` holds the store lock, decrypts, imports any schema-4 party rows into copies,
and passes those copies to `fn` without saving. `UpdateFinance` deep-copies the full
register and decrypted finance data, imports old party rows, runs `fn`,
`register.Validate`, `register.ValidateFinance` and `register.ValidatePartyReferences`,
encrypts with a fresh nonce and uses the existing temp/fsync/backup/rename sequence. Any
callback, validation, random, encryption or save error restores exact in-memory and
on-disk state.

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

Setup, activation, recovery and password change refuse a shorter new password with
`Password must be at least 8 characters.` before checking any setup code, recovery key
or current password. A refusal preserves only non-secret name/mobile fields; every
password, setup-code and recovery-key input is blank in the response.

Authentication forms are structured, not an undifferentiated run of controls. Each
uses one visible bordered section with a heading immediately before its fields: `Set up
authorized access`, `Log in to authorized access`, `Activate your account`, `Recover
authorized access`, or `Change your password`. Labels stay visible above controls;
explanatory text is outside inputs; the primary action is last. Setup-code entry is
labelled `One-time setup code`, never bare `Code` or `Password`.

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

Every recovery-key replacement, forced or deliberate, is two-step. First show `Replace the recovery key?`
and `The current recovery key will stop working. You must save and confirm the new key
before you can return to the financial ledger.`, with `Continue to replace key`. The
confirmation repeats that warning and uses `Yes, replace recovery key`. For a deliberate
replacement it also asks for `Current password`; the forced lost-confirmation path
relies on the authenticated recovery-pending admin session. Only the second
CSRF-protected POST with `confirm=yes` replaces it.

An admin section headed `Authorize another person` adds `Name`, `Mobile number`, and
role choices `Financial user` / `Financial administrator`, with button `Create one-time
setup code`. The server generates a 20-character uppercase unpadded base32 setup
code (100 random bits), wraps the vault key in a `setup` slot expiring exactly 24 hours
after creation, and shows the code once. `/finance/activate` requires matching mobile
and unexpired code, then replaces that slot with a password slot and marks the account
active in one save. A reset invalidates every password/setup slot for that account and
creates one new 24-hour setup slot. Admins cannot learn a password. Disabling removes
all its slots and sessions; the last active administrator cannot be disabled or demoted.

The one-time-code page heading is `Give this setup code to <Name>` and says exactly:
`This code works once and expires in 24 hours. Give it only to <Name>. They must open
Authorized login, choose Activate my account, and create their own password.` The code
is visually separated and never repeated after navigation.

Account actions are `Edit details`, `Reset password access`, and `Disable authorized
access`. Reset and disable are two-step. First POST writes nothing and renders:

- `Reset password access for <Name>?` / `Their current password and every earlier setup
  code will stop working. You will receive a new one-time setup code to give them.` /
  `Yes, reset password access`;
- `Disable authorized access for <Name>?` / `They will be logged out and will not be
  able to open the financial ledger. Their earlier ledger entries and audit history will
  stay.` / `Yes, disable authorized access`.

Only a second CSRF-protected POST with `confirm=yes` mutates. Target/status and the
last-active-administrator guard are rechecked on the second POST.

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
- The delivery desk and authenticated finance screens use one public party vocabulary;
  only names, aliases and stable IDs cross the vault boundary.
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

`TestSchemaTwoLoadsAsCurrentWithoutWriting` — load a real schema-2 v1.1.1 fixture; bytes
and `.bak` are unchanged, inventory is identical, legacy supplier snapshots are linked
to public parties in memory, `Finance == nil`, in-memory version is 5, and the next
ordinary save is schema 5.

`TestFirstFinanceWriteMigratesEncryptedSchemaFourPartiesAtomically` — schema-4 Sharma
Events keeps its `PTY-*` ID and every order/movement/return/sale reference; the first
finance write produces schema 5, removes the encrypted party-kind list, exposes only the
four allowed public party fields, retains encrypted actor/mobile/time audit and survives
restart.

`TestFirstFinanceWritePartyMigrationRollsBackOnFailure` — callback failure, invalid
cross-boundary party reference, encryption randomness failure and forced atomic-save
failure each leave schema-4 encrypted party rows, public register, main, backup and
in-memory state byte-identical.

`TestImportVaultPartyRenameAndMergeHistoryStillResolves` — import `Sharm Events` merged
into corrected `Sharma Tent House`; both old spellings and the current name resolve after
restart without rewriting a financial reference or exposing correction provenance.

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

`TestAccountDestructiveActionsRequireImpactConfirmation` — reset/disable first POST
shows the exact target, consequence and action above with byte-identical storage; only
`confirm=yes` writes and save-time guards still apply.

`TestSetupCodePageExplainsOneTimeTwentyFourHourUse` — exact heading, one-time/24-hour
instruction, activation path and own-password wording; the code occurs on one response.

`TestRecoveryKeyReplacementWarnsAndConfirmsTwice` — first step writes nothing; second
repeats the exact warning, requires current password/CSRF/`confirm=yes`, invalidates the
old key and enters the existing save-key gate.

`TestAuthorizationFormsAreReadableStructuredSections` — setup/login/activate/recover/
password pages have exact headings, visible labels in field order, one final primary
action and no pre-auth finance vocabulary.

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
ordinary route/API and assert only `Authorized login` and the intentionally public party
names/IDs/aliases appear; no protected navigation, amount, purpose, mode, account mobile,
financial relationship, audit provenance or timestamp appears.

## Acceptance criteria

1. Schema 1–4 migration, schema-5 save and downgrade warning procedure are tested.
2. AES-256-GCM and Go's `crypto/pbkdf2` are the only vault/password constructions; all
   random values come from `crypto/rand` and all authentication precedes parsing.
3. Raw main/backup/corrupt copies contain no decrypted financial field or value except
   the contracted public party names, aliases and IDs, with no financial relationship.
4. Every finance route passes the session/role/CSRF matrix and idle expiry tests.
5. Existing inventory tests and unauthenticated workflows remain unchanged.
6. No third-party dependency, network call, TLS listener or non-loopback listener exists.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/store/ ./internal/register/ -run 'TestSchemaTwo|TestFinance|TestFirstFinanceWrite|TestImportVaultParty|TestValidateParty|TestLinkInwardParties' -race -count=1 -v
go test ./internal/web/ -run 'TestFirstAccount|TestAdmin|TestAccountDestructive|TestSetupCode|TestRecoveryKey|TestAuthorizationForms|TestExpired|TestCannotDisable|TestFinanceAuthorization|TestFinanceSession|TestFinanceCSRF|TestPublicPages' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'crypto/(aes|cipher|pbkdf2|rand|sha256|subtle)' internal
rg -n 'math/rand|crypto/des|crypto/rc4|NewCBC|NewCTR' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n '"displayName"|"mobile"|"amountPaise":500000|"purposeId"|"modeId"|"createdById"|"byAccountId"' store-register.json 2>/dev/null # must print nothing; party names are intentionally public
rg -n '"parties"|"previousNames"|"mergedIntoId"' store-register.json 2>/dev/null # public party vocabulary may print
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
