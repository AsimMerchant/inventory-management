# Handoff

## Active work — financial ledger (started 2 September 2026)

Work is in progress on branch `feature/financial-ledger`, based on released `v1.1.1`
commit `f619ab7`. No Go implementation existed at the initial checkpoint. The clean
baseline was independently reproduced on this branch with `go test ./... -race
-count=1`, `go vet ./...`, and the cgo-disabled Windows amd64 build; all passed.

The user has explicitly superseded the old "no authentication, money or supplier
return" product decisions for a protected financial area only. Ordinary inventory
work remains unauthenticated and must reveal no financial content. A neutral
`Authorized login` leads to individual mobile-number/password accounts; the first
account is administrator, administrators authorize later accounts, and financial
menus appear only after server-enforced login. Financial records require encryption,
audited corrections/voids, recovery, and atomic persistence.

Agreed ledger scope includes multi-product rent/purchase orders; optional estimated
or exact agreed totals; independently timed outgoing payments and incoming refunds or
sale proceeds; reusable typeahead values for supplier/payee, purpose and payment mode;
admin rename/merge and safe typo deletion; partial physical receipt through the
unchanged inward flow; deposit refunds; physical rented-goods returns to suppliers;
sales of purchased stock; and a protected printable transaction journal filtered by
day, inclusive date range, or exact local date/time range. Money movements and
physical settlement movements are separate because they may occur at different
times. The reviewed contracts are specs 17–21: protected vault/accounts, orders and
reusable values, money/audit/printable journal, supplier returns and sales, and the
integrated browser acceptance gate.

### Step one of the merge is built and driven in a browser — 3 September 2026

On branch `fix/product-pickers-at-scale`. **Deliberately additive: nothing was removed.**
*Record an order* is still on the menu and still works. Step two, taking it off and making
the order screens read-only, has not been done.

*Record money* now carries product **lines**. Each line is the single-product typeahead,
an **Agreed quantity** and **Rented / Bought**; once per money row there is an **Agreed
total (₹)** with no estimate/exact choice. On save the screen mints the `FinanceOrder`
itself and links the movement through `OrderID` and `OrderLineIDs` — the user's own idea,
*"what if record money also did record an order"*. Nothing new is stored and
`SchemaVersion` stays 3. `AgreedKind` is written as `exact` only when a total was typed;
writing it unconditionally fails `validateOrders` and would stop the vault decrypting.

The order dropdown was kept, reworded to *Part of an order already recorded* and demoted
below the lines. It is how instalments work: a later payment points at the agreement
already recorded instead of minting a second one. An order chosen **and** products typed
is refused rather than silently merged.

**Driven in a real browser against a real binary**, register seeded through the UI:

- money with one line saved, order created, journal shows *Order: Sharma Tent House ·
  Products: Chairs*, order reads *300 Chairs — rent · Agreed total ₹25,000.00*
- two lines in one row produced one order carrying both
- a brand-new product typed on the money screen was created and used in the same entry
- the both-halves refusal appeared verbatim
- instalment against the existing order minted no second order
- **with JavaScript switched off**, the `<noscript>` selects saved product, quantity,
  basis and agreed total, and the order was still created
- no JavaScript errors on any screen

**A pre-existing defect found and fixed in passing.** `FinanceLineIsReferenced` was a stub
returning `false`, its comment still waiting for spec 19 — which shipped. So an order line
a money entry pointed at was never locked: the order screen offered to remove it and the
save died in `validateMovements` behind the misleading *"the register file could not be
written"*. Rare before, normal now that every entry with a quantity mints an order.
Implemented properly, voided movements excluded, and confirmed in the browser: the edit
screen says *This product is already used by a ledger entry* and offers no remove button.

**Known limitations of step one, not defects to hunt:**

- Correcting an auto-ordered money entry does not offer the quantities again; the quantity
  is corrected on the order screen. Typing into the lines is refused, not silently ignored.
- `movementChanges` still labels corrections *Related order* and *Related products* while
  the screen now says *Part of an order already recorded* and *Products this money is for*.
- `templates/multi-picker.html` and `static/multi-picker.js` now have no consumer. Left in
  place rather than deleted.
- Coverage of `internal/register` measures 92.2% merged, 87.1% alone. HANDOFF's older
  claim of 95.4% could not be reproduced and predates this work. **92.2% does not meet the
  95% gate** and is recorded here rather than glossed.

### Step two is built: the third acquisition kind, and schema 4 — 3 September 2026

On branch `fix/product-pickers-at-scale`. **`SchemaVersion` is now 4.** Before the new
`.exe` is used: back up `store-register.json`, then delete the old `.exe` from the laptop
**and from the pen drive**. This is the same procedural fix the user chose on 1 September
and again for schema 3, and for the same reason — the defect is in the released reader
and cannot be fixed here. `v1.2.1` does not refuse a schema-4 file cleanly: it treats the
mismatch as damage, sets the real data aside as `store-register.json.corrupt-<timestamp>`,
falls back to `.bak` and shows pre-upgrade data behind the ordinary damage banner. The
new build reads schema 1, 2, 3 and 4, so upgrading is safe and no record is rewritten.

**Where the list lives, and why it is not in the vault.** `AcquisitionKinds` is a new
slice on the plain `register.Register`, beside `Disposals` and for the same reason: the
delivery desk is never logged in, so a list it has to read cannot be encrypted. A word
like "donated" is not financial data. The ledger reads the same list. IDs are `AKD-0001`.
`Inward.KindID` and `FinanceOrderLine.KindID` name the word; `Basis` gains `other`.
Both keys are `omitempty` and nothing normalises the slice, so a file written before this
existed is byte-identical after a no-op save.

**The control is a plain `<select>` plus an "or type a new word" box, deliberately.**
Not a typeahead. The list is three to six words, so the scale problem this branch exists
to fix does not apply, and there is no `data-picker`/`data-values` box to go dead without
its script tag — the trap this branch has already hit twice. It works with JavaScript off
by construction rather than by fallback. Do not "upgrade" it.

**Rule two is in the arithmetic, not in the screens.** `eligibleInwards` and `remaining`
now take a predicate instead of one basis: `CanGoBack` is rent plus every typed kind,
`CanBeSold` is purchase plus every typed kind. Both draw on one pool, and
`allocatedFromInward` already counted returns and sales together, so fifty donated chairs
sold are fifty that can no longer go back. There is a test for exactly that.

**Deliberate asymmetry, decided here and worth his eye:** `SupplierObligations` still
counts rent only. Donated goods may go back, but nobody is owed them, so they are not an
obligation. The return door being open and an obligation existing are two different
things.

**That asymmetry had a defect in it, found in review and fixed in the same commit.**
Opening the return door to typed kinds also changed what a return *draws from*, and
`SupplierObligations` reduced the debt by the whole return however it was sourced. So
sending back 30 donated tents after the 40 rented ones had already gone left Sharma at
minus 30, and an older donated delivery could soak up a return and leave the rented goods
still shown as owed. Two changes: `eligibleInwards` now takes from the basis the door
settles first — rent for a return, purchase for a sale — and `SupplierObligations` counts
only the part of a return that landed on rented deliveries. For a register with no typed
kinds both are exactly the old behaviour. Both are proved load-bearing: each was reverted
in turn and the test for it fails.

**Nothing validates that a `KindID` still exists.** `register.Validate` runs inside every
write and `ValidateFinance` on every vault decrypt, so a missing word must never stop the
file loading. An unresolvable kind displays as the plain word "Other".

**Wording changed, and it needs his judgement.** The label over the three choices was
`Rent or purchase` and is now `How these came in` on *Stuff came in*, *Fix this*, *Record
money* and the order form. Two refusals changed with it: `Choose rent or purchase.`
became `Choose how these came in.`, and the money screen's `Say whether each product
with an agreed quantity is rented or bought.` became `Say how each product with an agreed
quantity came in.` Specs 07, 11 and 18 still carry the old wording and were **not**
edited. This has not been through `plain_language_reviewer`.

Verified: `go test ./... -race -count=1` green, `go vet` clean, PE32+ cross-compile with
`-trimpath -ldflags="-s -w"`. `internal/register` coverage 87.7% alone. No browser
testing was done — the user said he would drive it himself.

### The agreed order of work, settled with the user on 3 September 2026

Four things to clear, then one to plan. This order is his, and the reasoning is his:
the merge is the thing he has asked for repeatedly, it is the smaller change, and it is
the screen the balances work will live next to.

1. **Merge *Record an order* into *Record money*.** The design section below.
2. **The third acquisition kind** — Rent, Purchase or a typed word, on the money screen
   and at the inward desk, with the shared list getting the same rename / merge /
   safe-delete tools as the other reusable values. The schema-4 decision belongs here.
3. **The stock list's PURCHASE default** — the defect immediately below.
4. **Merge branch `fix/product-pickers-at-scale`** — eleven commits, built and tested,
   waiting only on his word.

Then **balances**, which needs planning with him before a line is written.

**Priority changed on 3 September 2026, late in the day.** The stakeholders are moving the
money to Tally *for now* and will stay there unless the ledger gets simpler. They are not
hitting the product-picker problem this branch fixes, because those controls are on the
ledger screens they have stopped opening — the desk's own picker was fixed in the released
build. Meanwhile the register itself is a success: over a hundred product types are being
run through it.

So: **balances stops being urgent** — Tally answers "where do I stand". What decides
whether the ledger survives is **simplification**, and the user has confirmed the
two-screen split was the worst of it, which the merge has now removed. Further
simplification comes after the work already in the pipeline, and should be planned by
watching the person who gave up use the merged screen, not by guessing which of login,
vocabulary or missing balances put them off.

### Supplier names move into the open file — decided 3 September 2026, NOT BUILT

The user spotted the asymmetry himself: *"i think supplier name should also be plain since
inward person will also be entering it."* He is right, and the desk is the unprotected
side.

**Today the desk types the supplier as free text.** `Inward.Supplier` is a plain string
filled from an ordinary text box on *Stuff came in*, with no list and no picker. So
"Sharma Tent House", "Sharma tent house" and "Sharma Tents" become three suppliers on the
Suppliers tab. Meanwhile the ledger keeps a tidy `FinanceParty` list of the same real
names inside the encrypted vault. Two vocabularies for one supplier, and only the ledger's
is protected from drift — the exact duplicate problem the reusable-value pattern exists to
prevent.

**Decided: move the whole party list into the plain `Register`,** beside products and the
new acquisition kinds. The desk picks a supplier instead of typing it; the ledger offers
the same list. One vocabulary, both sides, with the administrator rename / merge /
safe-delete already built for it.

**The exposure was put to him plainly and he accepted it.** The list is "supplier or other
party", so it also holds people who are paid but never deliver — a labour contractor, a
transporter. In the open, those names become visible on ordinary desk screens. His answer:
*"go with the simple one, move the whole list, we want money portion protected, which is
served."* Names carry no amount, no purpose and no payment mode; every figure stays in the
vault. The alternative — keeping pure payees private and publishing only names used as a
delivery supplier — was offered and rejected as more machinery than it is worth.

Do after the acquisition-kinds work: both touch `Register`, `finance_values.go` and the
inward screen. Migration is the open question to settle when it is built — existing
`Inward.Supplier` strings have to become references to list entries, and two spellings
already in a real file are two entries until somebody merges them.

### The main fields must be searchable everywhere — asked 3 September 2026, NOT BUILT

The user, 3 September 2026: *"basically every field should be searchable everywhere, the
main fields, like supplier, inward challan, issue challan, etc."* Same failure mode he
already met in the product pickers: a screen that works with three rows and is useless
with four hundred. To be planned with him, not built now.

**What exists today is two search boxes in the entire program.** Challan on *Someone is
returning* (`templates/return.html:11`) and challan on *Who did what*
(`templates/log.html:24`). Nothing else. *Stock*, *Who is holding what*, *What came in*
and *Suppliers* have no search at all — the reader scrolls. The typeahead on products and
parties is a pick-list for filling in a form, not a way to find records.

The main fields, as he named them: supplier, inward challan, issue challan, product,
person. Findable wherever they appear, not only on the one screen that happens to have a
box today.

**The rule that constrains it**, from the correction above: the supplier's challan and our
issue challan are different papers and are searched separately. "Searchable everywhere"
never means one box that matches both numbers.

Two things follow that are worth settling when this is planned:

- Searching a delivery by the supplier's challan does not exist in any form today.
  `FindOutstandingByChallan` walks issues only. It has to be written.
- The read-only views were built to be read whole. Adding search to them is not one
  change but four screens, and it should probably follow one pattern rather than four.

### Challan numbers belong on the ledger side too — asked 3 September 2026, NOT BUILT

The user's ask, to be planned with him later, not built now. The same paper challan that
the desk copies onto a delivery is often in the hands of the person who placed the order
or paid the money. Today only the physical side records it — `Inward.ChallanNo` and
`Issue.ChallanNo` — and the ledger has no challan field at all, so the two halves of the
same piece of paper cannot be brought together.

**The shape he proposed, and it matches how the desk already works.** One challan can
cover five products. The desk does not record it once for five things; it records five
deliveries, each carrying the same challan number typed again. So the merged money and
order screen should do exactly the same: **the challan number sits on each product line,
not once per entry.** Five lines under one challan repeat the number, five lines under
five challans each carry their own. His words: *"if 5 products get delivered under same
challan then inward guy will do the needful 1 by 1 for each product, same should be done
on record money and order screen."*

Keeping it per line rather than per entry is what makes the two sides comparable at all,
since the desk's records are per product too.

**There are two different challans and they must never be mixed.** Corrected by the user
on 3 September 2026 against a suggestion of mine to search one number across everything,
which would have been a defect:

- **The supplier's challan** — the vendor's own bill or delivery note, their number,
  written by them. This is `Inward.ChallanNo`, entered on *Stuff came in*. We do not
  control the numbering and two suppliers may easily use the same number.
- **Our issue challan** — torn from our own book when goods are handed to somebody. Our
  number, our sequence. This is `Issue.ChallanNo`, entered on *Someone is taking*, and it
  is the one the return screen searches through `FindOutstandingByChallan`.

They are unrelated pieces of paper. A supplier challan `447` and one of our issue slips
`447` have nothing to do with each other, so a search that returns both is wrong. Never
match them, never search them together, never share a field.

**The ledger's challan is the supplier's one — usually the same number the desk types,
but not always.** Confirmed and then qualified by the user on 3 September 2026: *"ledger
challan and inward challan for same product might be different sometimes, not always, but
sometimes, so inward person should be allowed to enter different challan number for same
product."* Our issue book still has no business in the ledger.

So these are **two independent fields that usually agree**, not one number stored twice.
The desk types what is on the paper in their hand; the ledger person types what is on
theirs. Neither is ever pre-filled from the other, neither overwrites the other, and the
desk is never blocked or warned for entering a different one. This is the same rule the
user already set for rent versus purchase: the ledger offers, the desk decides.

**One product can carry many supplier challans, and always could.** The user, 3 September
2026: *"1 product can have more than 1 challan, more than 1 supplier/vendor challan."*
This is already how the register behaves — chairs arrive from Sharma under `STH/4390`,
again under `STH/4471`, again from another supplier, and the stock is one pooled number,
which is his original decision that a chair is a chair. A challan is therefore never a
property of a *product*; it belongs to one delivery, and on the ledger side to one line.

**The screen already handles this, and an earlier note here overstated a constraint.**
Corrected 3 September 2026 after the user pushed back and the code was read.

*Record money* already has **Add another amount**, and `financeMoneyNew` builds one
separate `MoneyMovement` per row (`web/finance_ledger.go:361-370`). So ₹5,000 for 30
chairs and ₹6,000 for 30 more chairs, same supplier and same challan, go in together
today as two rows in one submission. Under the merge each row creates its own order, so
chairs appear once in each of two orders.

`finance_validate.go:315-318` does refuse an order that repeats one product on the same
basis, and that check runs on every vault decrypt — but with one order per row it is
never reached. It bites only if a single row lists the same product twice among its own
lines, and the natural answer there is a second row.

So: do not build the merged screen on an assumption of one line per product, and do not
widen that validator, but no special shape is needed either. The rows already do it.

**A payment cannot be tied to a delivery by matching challan numbers.** They are
allowed to differ, so equality is not a key. Any such linking has to work on product and
supplier, with the challan as a strong hint a human can read, never as an identifier the
program relies on.

**Settled 3 September 2026: the challan is on the approved money form, and it lives outside
the vault.** The form carries `Challan no.` once per entry, the supplier's own number. The
user asked for it to be kept out of the encrypted half so the desk sees the same numbers in
its suggestions: *"we need to keep this also out of vault since inward folks will be able to
see this in suggestions."* Same reasoning as products, acquisition kinds and supplier names.

The desk sees these numbers and can still type a different one on a delivery — the two
fields stay independent, as recorded above. There is nothing further to decide here. An
earlier note in this section raised the visibility of a bill's challan as though it were a
question; it was not, a challan number is not financial data, and it has been removed.

Worth deciding when this is planned, none of it assumed:

- Whether typing a supplier challan on the ledger should show what came in under it. That
  search does not exist today: `FindOutstandingByChallan` walks issues only, so nothing
  anywhere finds a delivery by the vendor's number. It would have to be written, and it
  must stay strictly on the supplier side.
- Whether repeating the number down five lines should be a single press, given the
  standing rule that nobody types an identifier from memory. Typing the same challan five
  times is exactly the kind of remembering that rule exists to prevent.
- Whether the ledger's challan stays free text, as both are on the desk side, or becomes
  something the two halves must agree on. Free text matches today's behaviour and is the
  smaller change.

Follows the merge and the third kind. Nothing here is started.

### Three amount columns, and the bill is what creates a debt — designed 3 September 2026, NOT BUILT

Worked out with the user on 3 September 2026 after he rejected two of my proposals in a
row. Both rejections were right and both are recorded here, because the reasoning is the
valuable part.

**The bill creates the debt, not the agreement.** Researched rather than assumed: standard
practice is to record a payable when the supplier's **invoice arrives**, not when an order
is placed. Before the bill, nothing is owed — that is the correct state, not a gap. See
[Accounts Payable explained](https://www.accountingcoach.com/accounts-payable/explanation)
and [Single-entry bookkeeping](https://en.wikipedia.org/wiki/Single-entry_bookkeeping).

This removes the question I had put to the user and which he could not answer — whether
the agreed amount is known when money is recorded. It does not need to be. The bill
arrives with the goods, on the supplier's challan, which the desk already handles. It also
explains why the order felt like pointless paperwork all day: **an estimate is not a debt**,
and accounting does not treat it as one either.

So the *Agreed total* field becomes a **bill amount**, entered when the bill comes. A
supplier who has not billed shows their payments and says plainly that no bill has arrived,
rather than implying a balance nobody can compute.

**Rejected: a separate *Record a bill* screen.** The user, immediately: *"record a bill?
record money? why 2 forms purpose?"* He was right — it walks straight back into the
two-screen split this whole day was spent removing.

**Rejected: one screen with a three-way radio** (billed / paid / received). It handles the
common case only by making the person add a second row, and his case is one event:
*"day 1 at same fucking time i pay 5k i get bill of 20k, same exact time."*

**His proposal, adopted: three amount boxes on one line.** The whole screen, shown to him
in plain text on 3 September 2026 and approved — *"so simple"*. This is the form. Build
this, do not redesign it:

    Record something

      Date and time      3 September 2026, 12:40 pm
      Supplier           Sharma Tent House
      Payment mode       Cash
      Challan no.        STH/4390

      What this is for
        Product          Chairs
        How many         500
        How these came in   [On rent]  Bought   Some other way

        + Add another product

      Billed     20,000
      Paid        5,000
      Received        —

      Reference          —
      Remarks            —

      + Add another amount            [ Save ]

Fill in whichever applies. Bill only, advance with no bill, both at once, or a refund
received. One save, and behind it two records are written. This is also what a hand-written
cash book looks like — parallel columns — so it matches the paper it replaces. It removes
the radio, and removes *Add another amount* from the ordinary case.

**What it costs, and the user was told:** one line carries one payment mode, so two
payments the same day in different modes still need a second line. *Add another amount*
stays for that.

### A page per supplier with a running balance — designed 3 September 2026, NOT BUILT

This is the shape the balances gap below should be built in. Worked out with the user late
on 3 September 2026 and recognised by him immediately: *"aaaaaaaaaaaaaaaaaaaaaah, this is
much simpler no?"*

**The framing that unlocked it.** Bookkeeping has two different books. The *journal* is the
chronological list — what happened, in order. The *ledger* is the same entries regrouped by
**account**, one page per party, with a running balance. The journal answers *what
happened*; the ledger answers *where do I stand with this person*. The screen in this
program is already correctly called **Transaction journal**. The journal was built, the
ledger never was, and the whole thing was called a ledger. That is precisely why the user
said the centre piece was missing.

**Decided: build accounts with running balances. Do not build debits and credits.** The
double-entry machinery is what accountants need and what makes Tally hard; the store needs
two of the four things a ledger has — a page per supplier, and the balance on it.

**The page is per supplier, never per product.** You pay a person, not a chair. Products
are what the money was for and are written on the lines. Per product would be useless:
"you owe chairs ₹39,000" answers nothing, and those chairs may have come from two
suppliers.

The worked example put to the user, four products and three challans under one supplier:

    Sharma Tent House — you owe ₹31,000

    1 Sep  Agreed: 500 chairs on rent · challan STH/4390   25,000   25,000
    1 Sep  Paid — advance, cash                            -5,000   20,000
    2 Sep  Agreed: 60 round tables · challan STH/4471       6,000   26,000
    2 Sep  Agreed: 12 shamiana poles · challan STH/4471     3,000   29,000
    3 Sep  Paid — cash                                    -12,000   17,000
    4 Sep  Agreed: 200 more chairs · challan STH/4502      14,000   31,000

Chairs appear twice, two deliveries under two challans — on this page they are simply two
lines, which is the "one product, many challans" case handled with no special machinery.

Above it, one line per supplier — Sharma ₹31,000, Gupta ₹3,500, Verma settled, and a
total. Tap a name, get their page.

**Every figure is already in the file.** Agreed totals sit on the orders, payments sit on
the movements. Nothing new to type: this is grouping and addition, which is the work the
book was supposed to save them.

**It also gives the order a job.** *Record an order* felt like pointless paperwork because
the agreed figure went nowhere. Here it is the line that creates the balance.

**Held back deliberately.** The goods side — 500 agreed, 300 delivered, 200 pending — is a
different table with different arithmetic. Mixing it into the money page is how this gets
complicated again. Money first, separately.

**Honest limit, told to the user.** This does not reduce what anybody types; *Record money*
still asks fourteen things. It changes why they would bother, because every entry now moves
a number they care about. Reducing the typing is the separate simplification item below.

### The ledger cannot say where you stand — named 3 September 2026, NOT BUILT

The user's words after being shown what an estimate does today: *"so we didnt actually
ever build a real ledger, this is a fake ledger after all"*, and later, *"the main centre
piece of ledger is missing from the fucking ledger"*. He is right, and this was never in
specs 17–21, so it is a gap in what was asked for, not a defect against the contract.

What exists is a cash book and it is honest: every movement in and out, audited,
correctable, voidable, encrypted, with a filtered printable journal and totals. What does
not exist is any notion of a **balance**.

Demonstrated, not assumed. Record an order with an estimated total of ₹25,000, then pay
₹23,000 against it: the order detail goes on showing *Estimated total ₹25,000*
(`web/finance_orders.go:405-410`), the journal shows ₹23,000, and nothing anywhere
compares the two. No warning, no outstanding figure, no flag. `MoneyForOrder` at
`register/finance_ledger.go:121` would compute exactly that comparison and **has no
caller outside tests**.

So the program answers *what money moved?* and cannot answer *where do I stand?* — and
the second is the question a ledger is opened to ask. It is also why the order screen
felt like pointless paperwork to him this morning: an order compared to nothing is a note
to yourself.

The two pieces, both from figures already in the file, needing no new file format:

- **Per order** — agreed, paid, and the difference still to pay, shown on the order.
- **Per party** — everything agreed with one supplier against everything paid to them,
  and the standing position, on one screen.

**Plan this with him before building.** He has watched three fixes introduce three
defects in one day and explicitly refused a wing-it approach: *"just wing it then 40
issues show up then fix that and forget the actual purpose of software at all"*.

### The stock list calls a product PURCHASE before anything has arrived

Live in `v1.2.1`, found 3 September 2026, small and self-contained. `StockRows` in
`register/arith.go:139-141` starts every product at `Purchase` and only lifts it to
`Rent` if some live inward says so — so a product created before its goods arrive is
displayed as bought, with no delivery anywhere to justify it.

It matters more once the merged money screen can create a product: record money for
donated goods, no inward yet, and the desk's stock list asserts PURCHASE.

Intended fix, agreed in principle with the user and not yet built: with no deliveries the
product has no acquisition kind, and the row shows a dash or *not received yet* rather
than claiming one. The kind becomes a fact when the desk records the delivery.

Not to be confused with a bigger idea raised and deliberately deferred the same day: a
column showing what the ledger *expects* to arrive. That one leads straight to
partial-receipt tracking and is its own piece of work, worth doing after the merge lands.

### Design the user dictated on 3 September 2026 — NOT BUILT, no code written

Recorded verbatim in intent at the user's request after an evening in which the split
between *Record an order* and *Record money* was the thing that finally lost him. He had
just asked, correctly, what an order is even for: nothing in the program compares agreed
against paid, an order changes no stock and enters no total, so as built it is optional
paperwork. This is his answer to that.

**One screen.** *Record money* absorbs *Record an order*. There is no separate screen for
saying what was agreed. Money and the agreement are recorded together, in one place.

**The estimated/exact distinction goes.** An agreed amount, or none. Nothing else.

**Per product on that screen: the agreed quantity, and what kind of acquisition it is —
Rent, Purchase, or a typed word.** The typed word is the user's example of `donated`, and
this is the same reusable-value pattern as party, purpose and payment mode: type a new
one once, it is offered to everyone afterwards. This is the continuation of the deferred
acquisition-basis decision recorded below, now with a concrete shape.

**The inward screen gains the same third choice, and the desk still decides.** Corrected
by the user on 3 September 2026 against an earlier reading of mine, which had the ledger
telling the desk what it had recorded. It does not. There is no "the ledger says donated"
message anywhere, nothing pre-selected and nothing enforced.

Instead the inward screen's basis becomes three radios — **Rent / Purchase / Other** —
and Other opens the list of typed kinds: sponsored, donated, borrowed, whatever is there.
That list is one shared vocabulary. A kind typed on *Record money* is in the list at the
desk afterwards; a kind typed at the desk is in the ledger's list too. It is the same
reusable-value pattern as party, purpose and payment mode, and it is what the deferred
acquisition-basis item below described — the user has folded that work into this design
rather than leaving it separate.

The ledger therefore contributes vocabulary, never an answer. The person holding the
delivery note picks, exactly as they do today.

This is still aimed at a live defect: the desk's Rent / Purchase tick is the only place
that fact is ever decided, and a wrong one silently makes goods unreturnable. The user
hit it himself — cooking vessels ticked `PURCHASE` never appeared on *Return rented
goods*.

**A wrong tick is already recoverable, and this was checked rather than assumed.** *Stuff
came in* → *Fix this* on the delivery carries the Rent / Purchase radios. Driven in a
browser against `INW-0027`: purchase changed to rent, after which Cooking vessels
appeared on *Return rented goods* at 249 and vanished from *Record a sale*, and *Who did
what* recorded `Fixed this entry: 249 cooking vessels from Verma Sound & Light`. When the
third choice is built it has to reach this screen too, not only the new-inward screen.

Noted in passing: the reason field on that page belongs to the *delete* form, not the fix
form, so a correction records what changed and who but never why. Deletions do ask.

**Goods of a typed kind appear on both settlement screens.** Answered by the user on
3 September 2026. Rent stays exclusive to *Return rented goods* and Purchase exclusive to
*Record a sale*, but every typed kind — donated, sponsored, borrowed — is offered on
both. A borrowed thing may well go back; a donated thing may well be sold; the program
has no business deciding which and should offer the person both doors.

So the lists become Rent plus every typed kind for a return, and Purchase plus every
typed kind for a sale. Both draw on the same physical stock, so fifty donated chairs sold
are fifty that can no longer be sent back — which is already how the on-hand cap behaves
and is right, because the goods left the store either way.

Worth raising with him when this is built, not now: *Return rented goods* stops being an
accurate name for a screen that also sends back borrowed and donated things.

**Money is still real when the goods are free.** His words: goods may be donated while
the labour for delivery is still payable. The kind describes how the goods arrived; it
says nothing about whether money moved. The screen must allow a money entry against
donated goods without treating the donation as a payment, and must allow an agreed
quantity with no agreed amount at all.

**The kinds list gets the same administrator maintenance as the other reusable
values.** Decided by the user on 3 September 2026, asked and answered in plain terms: a
typed kind rots the same way a typed supplier name does — `donatd` beside `donated`, and
from then on deliveries split across both. So the kinds list gets the rename, merge and
safe-delete-if-unused screen that supplier/payee, purpose and payment mode already have,
built the same way rather than as a new mechanism.

### Can this be built without disturbing the file? Checked 3 September 2026

Asked by the user before any building, because real inventory is already recorded and
the json cannot be dropped. Read from the code, not assumed. Conclusion: **yes, going
forward. No record already written is rewritten, and no migration step is needed.** What
stops being safe is going *backwards*, and only once a typed kind is actually used.

**Keeping the four existing orders is a hard constraint, not a preference.**
`finance_validate.go:182-187` resolves every movement's `orderId` through
`FinanceOrderByID`, and `store/finance.go:173` runs `ValidateFinance` on *every vault
decrypt*, not only on save. So converting or deleting those orders makes the vault
refuse to load. Leave `FinanceOrder` in place, retire only the screen that creates new
ones, show the existing rows read-only.

**Dropping the estimated/exact distinction needs no migration.**
`finance_validate.go:285-295` demands `estimated|exact` only when a total is present. Old
rows keep their `agreedKind` and load unchanged; nothing new has to write one.

**The third acquisition kind is additive on disk and a bounded code audit off it.** No
field is removed or repurposed: `Inward.Basis` gains a value and a companion kind ID.
Nothing in `internal/store` validates `Basis` on load — it is checked only at the form
boundary, `web/inward.go:165`. But every place that asks `== Rent` treats anything else
as a purchase, so each must be visited in the same change:

- `arith.go:129, 141, 486, 509` — `arith.go:118` says it outright, *"Rent if any inward
  for the product is rent, else Purchase"*, so donated goods would show as **Purchase**
  on the stock view and group with purchases in the suppliers view until this is fixed.
- `finance_settlement.go:400, 421, 470, 530` — the settlement caps.
- `web/inward.go:165`, `web/views.go:48`, `web/corrections.go` — form guard and pills.

Two traps in the same area, both silent if missed. `finance_validate.go:312` refuses an
order line whose basis is not rent or purchase, and that runs on load — widen it in the
same commit. And `ValueKindPrefix` returns `""` for a kind it does not know, which
`validateReusableValues` then rejects as *"kind and id do not agree"* — so the new
`FinanceValueKind` constant and its prefix case must land together.

**Retired 3 September 2026: whether an agreement with no money needs recording.** Asked
three times and never answered, because the question stopped existing. Under the rule the
user approved — a bill creates the debt, not an agreement — nothing is owed before the
supplier bills, so there is nothing to write down and no back door is needed.

**Open, and explicitly his call, not to be assumed:**

- *Return rented goods* is now a slightly untrue name: borrowed and donated goods go back
  through it too. *Send goods back to supplier* would cover all three. Cosmetic, nothing
  breaks either way, and the user has not decided.

**Decided 3 September 2026, and shipped the same day: `SchemaVersion` goes to 4, and only
when the third kind ships.** It has shipped; see the step-two section above for the
upgrade procedure. The user delegated the call — *"u decide that"* — under one requirement: the new
exe must open the existing `v1.2.1` file with every order intact. That requirement is met
either way and is not what the number affects. The reader accepts schema 1, 2 and 3 and
will accept 4, so upgrading is unaffected; the version number governs only an **old** exe
opening a **new** file.

Left at 3, `v1.2.1` opens a file containing typed kinds and silently reads every one of
them as a purchase — recreating the exact defect the design exists to remove, in the
user's live register, with no warning. Bumped to 4 it refuses instead. The refusal is bad
— `copyAside` plus a stale `.bak` behind the damage banner, `store.go:190`, unfixable in
a released reader — but it is visible, and the handling is the procedure the user already
chose on 1 September: back up the json, then delete the old exe from the laptop and the
pen drive.

A loud, already-documented failure beats a quiet wrong number in the register.

**The merge on its own needs no bump.** It stores nothing new — it reuses `FinanceOrder`
exactly as schema 3 already defines it. Only the third acquisition kind changes what a
file can contain.

**Do not start this without him.** It changes the shape of the ledger's central screen,
touches the register's acquisition basis, and reaches the ordinary desk — the one part of
the program he trusts and that currently works.

### Post-release fix in flight — branch `fix/product-pickers-at-scale`

Branched from `master` at `v1.2.1` + docs. **Not merged.** It exists because the user
tried the released build with a realistic catalogue and found two product controls that
work with three fixtures and fail with forty.

**What was wrong**

1. *Record money → Related products* was a `<select multiple size="4">`. Choosing more
   than one product required knowing to hold Ctrl; an ordinary click silently threw away
   everything already chosen; forty products sat in a four-row scrolling window. The
   user found it by asking how it worked, which is the whole answer. Ctrl+click *did*
   record against several products correctly — the defect was that nobody could discover
   it.
2. *Return rented goods / Record a sale → Product* was a `<select>` of every returnable
   product, with no way to type and no availability on the row.

Both broke the rule the design rests on: nothing is remembered, everything is picked.

**What replaced them**

- `internal/web/templates/multi-picker.html` + `static/multi-picker.js` — type, press,
  and the product joins a visible list of removable tags. Hidden inputs keep the field
  name (`productIds-N`), so the handler is unchanged. `<noscript>` keeps the old
  multi-select as the fallback.
- The settlement form now uses the ordinary `picker.html`, pointed at
  `/finance/api/products` with `data-mode="return"`/`"sale"`, `data-party-from` (the
  supplier decides what may go back) and `data-except` (an edit must not count itself).
  `financeAPIProducts` grew `settlementSuggestions`, which labels rows
  `"Tents — 60 available"`.
- CSS: `.chosen`, `.chosen-tag`, `.chosen-off` in `app.css`.

**The trap this fix hit, and it is the same one twice**

A picker is markup *plus its script*. `finance-settlement-form.html` got the picker
markup and no `<script src="/static/picker.js">`. Every Go test passed and the box found
nothing in a real browser. `finance_picker_test.go` now asserts the script tag on both
forms, and that assertion was proved by deleting the tags and watching both tests fail.

**Verification done**

- `go test ./... -race` green (`internal/web` 225s), `go vet` clean.
- `finance_picker_test.go`: three new tests — settlement picker narrows and carries the
  number, another supplier's goods never appear, no supplier offers nothing rather than
  everything; the sale picker offers only purchased goods; the money control is not a
  multi-select outside `<noscript>`, saves several products, and shows them back as
  removable tags on the correction form.
- Real browser, real binary, 40 products / 5 suppliers / 4 orders / 8 money entries
  (`/tmp/claude-1000/seed.sh`, port 8766): typed three products onto one money entry
  with no modifier key, removed one, put it back, saved — journal read
  `Products: Tents; Water drums (20L); Chairs`. Supplier return: typed `ch`, got only
  `Chafing dishes — 165 available` from that supplier, saved as `SRN-0002`.
- JavaScript off: money form still offers the 32-option fallback; the settlement form's
  `Show what can go back to this supplier` button still fills the plain select with the
  availability labels.

**Second pass, after review**

- `picker.html` also posts a free-text `productName` and can render an "add as a
  brand-new product" row. Neither reaches the settlement screens: `AllowNew` is left
  false so `data-newlabel` is empty and no create row is drawn, and
  `settlementFields` derives the product name from `productId` alone, refusing with
  "Pick the product from the list." if nothing was pressed. The catalogue cannot be
  written from a settlement screen.
- Orphans this change stranded, now removed: `moneyDraft.Products`, `moneyRow.Products`
  and `settlementDraft.Products` (the templates read `.Picker`/`.ProductBox` instead),
  and the whole `except` path through `pickerData`, `picker.html`, `picker.js` and
  `settlementSuggestions` — the settlement picker renders only under `{{if not
  .Editing}}`, where the ID is empty, so it was unreachable. `fillSettlement` still uses
  the `…Excluding` register functions, which the edit path does reach.
- `TestSplitMoneyEntryKeepsEachRowsProducts`: adding a second amount redraws the page
  without losing the first row's chip, and the two saved entries carry one product each.
- `TestCorrectionCanTakeEveryProductOff`: taking every chip off submits no `productIds`
  at all; the entry ends with none and the audit records `Chairs; Tents → Blank`.
- Browser, same seeded instance: both of the above driven for real, plus removing a
  server-drawn chip (which uses the `data-multi-off` delegation, not the listener on a
  tag the script just added) and replacing it with another.

**Third pass — the availability box, found by the user on the sale screen**

`Available to sell` read `0` beside a correctly chosen product with 427 available. The
number is server-rendered from `d.Available`, which is worked out for whatever product
was submitted, so choosing one in the browser never touched it. The old `<select>` had
the same staleness; the type-to-find picker made it visible, because now the picker's
own rows carry the right number while the box beside them says nothing.

- `pickerData.CountInto` names an element (`[data-available]` on both settlement forms);
  `picker.js` writes the chosen row's `onHand` into it.
- Its neighbour, fixed at the same time: choosing a product for one supplier and then
  changing the supplier left both the product and a confident wrong number on screen.
  The server refuses such a save, so nothing could be corrupted, but the screen lied.
  Changing the party now clears the product and the count. `value-picker.js` had to
  announce itself for this — it sets `.value` directly, and firing `input` would trip
  its own handler and blank the ID it had just set, so it dispatches a bubbling
  `valuepicked` CustomEvent instead. `picker.js` listens for both.
- `TestAvailabilityBoxIsKeptInStepWithThePicker` asserts the box is named and the picker
  is pointed at it on both forms, and that the number the picker would write is real.
- Browser: sale screen fills 427 and saves; switching between products updates it;
  changing the supplier on a return clears both, whether the supplier is typed or
  pressed from its list; `/inward/new` still picks products and still offers
  `+ Add "Chai" as a brand-new product`, so the shared `value-picker.js` change did not
  disturb the ordinary desk.

**Mobile numbers, asked and answered:** `register.MobileKey` keeps only digits, so
`98861 40023`, `9886140023`, `98861-40023` and `988 6140 023` are the same account.
`+91 98861 40023` is not — the digits become `919886140023`. Verified by four real
logins against a running binary.

**Fourth pass — the supplier stops filtering the product list**

The user filled the sale form product-first and watched the product vanish when they
named the buyer. Two faults behind one symptom:

1. *Mine.* I wired `PartyFrom` on the sale form out of habit. A sale's product list does
   not depend on who is buying — `settlementSuggestions` ignores the party for
   `mode=sale` — so the clearing was pure damage. The sale form no longer sets it.
2. *The design.* Filtering the return list by supplier forced one filling order and
   discarded work when anybody used the other. The spec calls this a "parent
   orchestration decision", not a user decision. The user was asked and said they were
   too confused to choose, so the call was made for them and stated plainly.

**The rule now: the supplier narrows the number, never the list.** A return offers every
product physically in the store. With no supplier named the row reads `— 155 on hand`;
naming one turns it into `— 150 available`, including `— 0 available` for goods that
supplier never sent. Nothing is ever removed from the form, and `Store.RecordSettlement`
still refuses at save: `Only 0 Chafing dishes can be returned to this supplier.`

Removed with it: `pickerData.WaitFor`, the `data-wait` attribute, the hint row and its
`.sug .wait` CSS, and the clearing branch of `recheck` — which now only ever moves the
number. `value-picker.js` still dispatches `valuepicked`, because the number genuinely
does depend on the supplier and has to follow it.

Browser, both orders and the refusal: product-then-supplier saved `SRN-0003`;
supplier-then-product still narrows to `Chafing dishes — 150 available`; a supplier who
never sent the goods keeps the product on screen, shows `0`, and refuses on save.

**Note for whoever picks this up.** Six defects surfaced in this branch's life, three
already in released `v1.2.1` and three introduced while fixing those. Every one lived in
the browser, between fields, or in a missing script tag — none in a handler. The Go
suite posts complete correct forms and cannot see any of it. Treat a green suite here as
evidence about handlers only, and drive the screen.

**Fifth pass — the ask-first button, and the sweep**

The user spotted that `Show what can go back to this supplier` no longer had a job: the
list stopped depending on the supplier, so there was nothing to ask for. Removed, along
with the `refresh` branch in `settlementNew`. The `<noscript>` fallback now follows the
same rule as the picker — everything physically in the store, labelled `on hand` until a
supplier is named.

That removal exposed a defect **introduced in this branch** that only a browser with
JavaScript off can see. `picker.html` submits a hidden `productId` *and* a `<noscript>`
select of the same name, so with no script both arrive and the empty hidden one is
first. `readSettlementDraft` used `r.FormValue`, which takes the first, so every
scriptless save was refused with "Pick the product from the list." while a product was
plainly chosen. `formProductID` exists for exactly this and the settlement draft was not
using it. Fixed, and covered by `TestSettlementReadsTheProductWithNoScript`, which posts
`productId` twice the way a scriptless browser does.

**The sweep, across all 14 protected screens at 40-product scale:**

- Every screen returns 200, with no unrendered template output.
- Every `data-picker`, `data-values` and `data-multi` box on every screen has its script
  loaded — the check that would have caught the missing `picker.js` in one second.
- Every product picker returns rows when typed into; every party/purpose/mode list does
  too.
- The money form's order dropdown lists the seeded orders and choosing one offers its
  four products.
- The journal's date filters work: 17 entries unfiltered, 1 for `day=2026-08-30`, 3 for
  `from=to=2026-09-01`, 1 for a one-hour `fromTime`/`toTime` window.
- No JavaScript errors anywhere in the pass.

The sweep raised two hits, **both faults in the sweep script itself, not the app**:
`mode=sale&q=ch` returning nothing is correct (in this seed Charcoal sacks is a rental,
so there is nothing sellable matching "ch"), and "the date filter changed nothing" was
the script clicking the last submit button on the page — a void form — instead of the
filter form. Nothing was voided; the two-step guard held.

**Sixth pass — the user overrode the supplier rule outright**

The fourth pass had the diagnosis wrong. Filtering the return list by supplier was not a
usability wrinkle to soften; the rule underneath it was wrong. The user, in their own
words: goods are handed to a transporter or to whoever is doing the rounds, and *that*
person returns them to the vendor. The register was refusing something the store does
every day.

**Spec 20's "parent orchestration decision" is superseded and marked so in the spec.**
The party on a supplier return records who took the goods and limits nothing.

- `supplierReturnAvailable` ignores its `partyID` argument: the cap is the smaller of
  what is physically here and what came in on rent for that product, whoever sent it.
- `allocateSupplierReturn` spreads across the oldest rented receipts for the product,
  `nil` aliases, same as the sale path already did for purchases.
- New `register.SupplierSentRented` answers "how much of this did that party send on
  rent" through renames and merges. It caps nothing; it exists only to narrow a list.
- The picker offers everything that may leave the store. `pickerData.OnlyPartyLabel`
  renders a tick, *Show only this supplier's goods*, which adds `onlyParty=yes` to the
  request. The API applies the party filter only when that flag is present.
- Refusal reworded: `Only 60 Round tables can be sent back.`

Tests changed with the rule, not around it: `TestSupplierReturnAllocatesOldestEligible
Inwards` now expects allocation across suppliers and its alias assertions moved to
`SupplierSentRented`; `TestSupplierReturnRefusesMoreThanTheTwoCaps` now asserts that a
party who sent nothing *may* take goods back; `TestSettlementProductIsTypedNotScrolled`
asserts the list is identical before and after a supplier is named, and that the tick
shortens it.

Browser: Patel chosen, typing `c` gives 8 rows; ticking the box gives 3; unticking gives
8 back. Chairs saved against Patel, who never sent a chair — `SRN-0007`.

**Do not re-narrow this without asking.** Two passes were spent moving between filtered
and unfiltered because the underlying rule was never questioned. It has been now, by the
person who owns the decision.

**Still to do before merge:** nothing known. Full gate green after the change
(`-race` all packages, vet, PE32+), and the 14-screen sweep re-run clean. The sweep
script is at `/tmp/claude-1000/sweep.js` and is worth rewriting into the repo as a real
acceptance pass; it is the only check that looks at the layer where every defect in this
branch lived. Its two standing hits are faults in the script, not the app: `mode=sale&
q=ch` correctly returns nothing in this seed, and the journal-filter check clicks the
wrong button (the filter works — 17 entries unfiltered, 1 for `day=2026-08-30`).

### Deferred acquisition-basis follow-up — now folded into the design above

**Superseded 3 September 2026.** The user has folded this into the merged money-and-order
design recorded above, rather than leaving it as separate later work. The reasoning below
still stands and is why that design has a third acquisition choice at all.

The user later clarified that acquisition basis should not become one fixed `Borrowed`
enum. A future, separately specified feature should keep built-in suggestions for Rent
and Bought while allowing a user to type a new acquisition-basis value once and select
it from suggestions everywhere thereafter. Reusable basis values will need the same
administrator typo maintenance as other shared financial values: rename, merge, and
safe deletion when unused. No current implementation or spec contract changes for this
follow-up.

The user then settled the important semantic boundary: acquisition basis and final
disposition are separate. Inventory staff only receive stock and ensure issued goods
return to the inventory pool. After the event, an authorized user explicitly records
what finally happened—returned, sold, transferred, discarded, donated, or a future
custom outcome—and records related money separately. No return/sale/keep behavior may
ever be inferred from a custom acquisition-basis name such as `Borrowed`, `Lent free`,
or `Sponsored`.

### Build progress against specs 17–21

`.agent-handoff/` is gitignored, so it does not survive a fresh clone. **This section is
the resume artifact.** Keep it current after every committed slice.

| Spec | Slice | Commit | State |
|---|---|---|---|
| 17 | Vault, accounts, sessions | `cdc431c` | **Done.** All 16 required tests pass under `-race`. |
| 18 | 1. Order/value model, IDs, validation, rupee parser | `0b9cbb9` | **Done.** register-level tests pass. |
| 18 | 2. Reusable values: typeahead, admin `/finance/lists` | `f162427` | **Done.** 3 required tests pass. |
| 18 | 3. Orders: create, list, detail, `POST /finance/product/new` | `c484970` | **Done.** 6 required tests pass. |
| 18 | 4. Order edit and cancel, the six-row `FinanceChange` table, suggestion ordering, docs | `b97023b` | **Done.** All 12 required tests pass. |
| 19 | 1. Movement model, totals, journal filters, checked int64 money | `f7a831e` | **Done.** register-level tests pass. |
| 19 | 2. Recording money, single and batch | `f70d75d` | **Done.** 7 required tests pass. |
| 19 | 3. Corrections and voids | `d6551ea` | **Done.** |
| 19 | 4. Dashboard, journal, print view, financial activity | `d6551ea` | **Done.** All 13 required tests pass. |
| 20 | 0. `Register.Disposals` and the new `OnHand` — **alone, as a regression canary** | `7e01240` | **Done.** Whole suite green with an empty slice. |
| 20 | 1. Settlement types and pairing validation | `206e705` | **Done.** |
| 20 | 2. Allocation, party aliases and supplier obligations | `206e705` | **Done.** 6 required register tests pass. |
| 20 | 3. Supplier returns, sales, settlement and obligation screens | `47a1758` | **Done.** |
| 20 | 4. Settlement edit/void, inward guards, product cascade | `47a1758` | **Done.** All 13 spec-20 web tests plus 6 register tests pass. |
| 21 | 1. Protected navigation, route and header matrix | `9dcda6e` | **Done.** All 6 required tests pass. |
| 21 | 2. Plain-language review, two-step confirmations, paired spec/test amendments | `643b159` | **Done.** 13 new contract tests; all 74 required tests across 17–21 exist and pass. |
| 21 | 3. Browser acceptance run, release gate, docs | `6570dce` | **Done.** Scenario passed on a real binary; independent `release_gate` reports **READY**. |

**Spec 21 makes two things blocking that spec 18 did not.** Its acceptance criterion 5
requires an independent `plain_language_reviewer` with no blocking finding *and* an
independent `release_gate` reporting `READY`; neither can be replaced by the
implementer's own report. Criterion 2 requires the browser scenario actually executed
against a real native binary, in normal mode, again with JavaScript off, and again after
a restart.

**Decision taken before spec 20 starts:** spec 20's central invariant pairs a protected
settlement with a public disposal, which `ValidateFinance(f)` cannot see. Rather than
widen that signature across every spec-17 call site mid-feature, the pairing check lives
in its own `ValidatePairing(reg, f)` called beside the two existing validators inside
`UpdateFinance`.

Spec 17 arrived from the previous session mid-refactor and did not compile: the request
was being threaded into `Server.page` so the chrome can tell whether a finance session is
live, and four `render*` helpers still referred to an `r` they did not take. Completing
that threading was the whole fix; no spec-17 behaviour was changed.

**The financial screens are placeholder HTML.** They work and are asserted by tests, but
they are unstyled and have not been through `plain_language_reviewer`. Spec 21 makes them
real. Do not judge the feature by opening it before then.

#### Browser acceptance run record (spec 21 criterion 2)

Binary built from `fede98f` plus the fixes below, run from a fresh `mktemp -d`,
Chromium via Playwright 1.62.1, discovered URL `http://127.0.0.1:8765`.

| Pass | Result |
|---|---|
| Steps 1–13, normal JavaScript | **85 checks, 0 failures, 0 console errors, 0 external requests** |
| Step 14, file on disk after stopping the server | schema 3; products, inwards and disposals readable; `finance` holds only `vaultVersion`, `keySlots`, `recovery`, `nonce`, `ciphertext` |
| Step 14, restart with nobody logged in | Tents **60**, Chairs **30** — correct after a supplier return, a sale and a void — and no finance menu anywhere |
| Step 15, JavaScript disabled | **17 checks, 0 failures**: login, an existing-value payment through the selects, a brand-new purpose through `Or add a new one`, exact-time filter, print view and logout |
| Step 12, cancel a paid undelivered order | **12 checks, 0 failures**: the two-step cancellation warns that payments will not change, the original ₹10,000.00 stays on the journal, the equal refund is its own incoming row, and Chairs stock is unchanged throughout |

**Two names are in the public file, and both are deliberate.** Do not "fix" them:
`inwards[].supplier` is ordinary inventory data the desk types, and
`products[].createdBy` carries the authenticated display name because spec 18's contract
requires it when a product is created through the finance route. No money, mobile,
party list, settlement kind or record ID appears in plaintext. Everything else on the
protected-value list was checked and is absent.

All three passes were re-run against `d99bd7b` after the protected-chrome fix, because
that change alters what ordinary pages emit: 85 + 12 + 17 checks, still zero failures,
zero console errors and zero external requests. The file after that run still shows
schema 3, an envelope of nothing but ciphertext and key slots, disposals carrying only
the six contracted fields, and Tents 60 / Chairs 30 after a restart with nobody logged
in.

**Every step of the scenario has now been driven in a real browser.** The idle-expiry
boundary is deliberately left to the deterministic clock test, as spec 21 step 13
itself directs: the browser run must not claim a real-time idle test without waiting
fifteen minutes.

One more selector fact: the order detail's `Review cancellation` and the settlement
list's void control both sit inside a `<details class="danger">` that starts closed.

#### What the first release gate found

Spec 21 criterion 5 exists because an implementer cannot certify their own work, and
this proved it. The gate reproduced everything independently — build, vet, the race
suite, all 74 required tests by exact anchored name, the Windows PE32+ build, zero
third-party dependencies, `ss -ltnp` showing the listener bound to 127.0.0.1 only,
schema-1 and schema-2 migrations with sha256-identical `.bak` bytes, both settlement
caps, six-way concurrency accepting exactly the safe number with on-hand never
negative, an attempt to construct an orphan pairing that failed as it should, a
plaintext scan of the real file, and the rupee parser refusing rather than overflowing.

It returned **NEEDS WORK** on one defect the suite could not see, described in the trap
above. It is fixed at `d99bd7b`, and the fix was proved by reinstating the old branch
and watching the amended test fail on all nine symptoms before restoring.

Its non-blocking observations, worth keeping:

- `keySlots[].accountId` is plaintext (`FAC-0001`). It is key-slot lookup metadata; the
  mobile is a SHA-256 hash and no display name appears, and the slot count reveals the
  account count regardless. Not a financial leak.
- Spec 20's verification block greps `internal/web/finance_settlements.go` for the
  allocation calls, but the writes live in `internal/store/finance_settlement.go`.
  Spec-doc drift only: both caps really are recomputed inside `UpdateFinance` under the
  store lock.
- Spec 21's verification block uses `rg --exclude`, which ripgrep does not have, so that
  command errors as written. The corrected form is silent.
- Spec 17's grep of `store-register.json` passes vacuously: that file is gitignored and
  absent from the repo root, so it proves nothing. Encryption evidence has to come from
  a file generated by driving the binary.

#### The re-gate: READY

The second `release_gate` on `d99bd7b` returned **READY**. It did not take the fix on
trust: it reinstated the old branch itself, on two different ordinary paths, and
confirmed the amended test fails on all nine symptoms and names the patched path
specifically, so no assertion is vacuous. It also re-checked the bind address from the
live process, the Windows build, the crash and `.bak` paths, and drove over-issue and
negative-quantity refusals against a running binary.

Two of its non-blocking observations are now fixed:

- The protected 404 rendered the same empty ` · · ` identity line, because
  `financeNotFound` never filled the name fields in. It now calls `whoAmI` like every
  other protected page, and the route-matrix test asserts the identity is there.
- `financeGET` in the test helper read the body with one `Read` into a buffer, which is
  not guaranteed to fill it: a large page could be truncated and quietly turn a presence
  assertion into one that can never fail. It uses `io.ReadAll` now.

Two are recorded and deliberately left:

- `inward.go:162` and `corrections.go:487` say "Type the date like this: 03-09-2026."
  while the parser wants `2006-01-02`. Pre-existing, and unreachable through the native
  `type="date"` input, which submits ISO. Only a hand-built POST can see it.
- A logged-in financial user on an ordinary page still sees `Authorized login`, and
  pressing it shows the login form rather than returning them to the ledger. Their
  session is untouched. This follows directly from spec 21 fixing public chrome as the
  inventory controls plus that one link, so changing it means amending the spec.

#### Browser acceptance: how to run it

The script is scratchpad-only (spec 21 forbids committing one). Rebuild, copy into a
fresh `mktemp -d`, run it there, and drive `http://127.0.0.1:8765`. Playwright 1.62.1
lives in the npx cache; set
`NODE_PATH=/home/asim/.npm/_npx/e41f203b7505f1fb/node_modules`.

Selector facts learned the hard way, all of which cost a run each:

- **The chrome's Logout is the first submit button on every protected page.** Any bare
  `click('button[type=submit]')` logs you out. Name the form, or click by button text.
- The order form's **Save order** is *not* the first submit in its form either — the
  per-line "add this as a new product" button comes first and carries `formaction`.
  Click `button:has-text("Save order")`.
- Setup, login and activate forms **post to the current URL and carry no `action`**, so
  select them by a field only they have.
- Basis radios are **indexed per row**: `basis-0`, `basis-1`.
- `<details class="add">` on `/shift` is **already open** on an empty register; clicking
  the summary closes it. Check `.open` first.
- Adding a person is not putting them on duty: **select the radio, then press Start
  shift**, or every ordinary page keeps redirecting to `/shift`.
- With script on there is **no `<select>` in the DOM at all** for pickers — the
  `<noscript>` fallback is not parsed. Drive the real picker: type, wait, click the row.
- A fresh binary has **an empty catalogue**. Spec 21 step 4 assumes Chairs exists; it
  does not, so create both products through the deliberate finance route.

#### Traps this build has already hit

- **The finance session cookie is `Path=/`, so it rides every ordinary request too.**
  The shared page helper saw it and rendered the protected navigation *and a live
  session CSRF token* onto `/stock`, `/log` and the rest — pages served with no
  `no-store`, no content policy and no frame protection, because the header regime is
  gated on the `/finance` prefix. It also produced a stray `<span class="who"> · · </span>`
  because the identity fields were never filled in on that path. Ordinary pages now
  carry no protected chrome at all, which is what spec 21 says public chrome is.
  **Any test asserting what an ordinary page does *not* show must use an authenticated
  client**: the two that named this contract used an anonymous one, so the branch never
  fired and they passed throughout. The independent release gate found it, not the
  suite.

- **A screen can be impossible to use while every handler test passes.** The supplier
  return form renders its product list from the server, and that list depends on which
  supplier the goods came from. Choosing a supplier in the browser cannot refill it, so
  the screen could never be completed. The handler tests missed it because they post the
  supplier and product together in one request, which no person can do. There is now a
  plain submit, `Show what can go back to this supplier`, that redraws the form with the
  list and writes nothing; `TestSupplierReturnListsProductsAfterChoosingSupplier` pins
  it, including that merely asking creates no party.
- **The whole handler suite can miss a defect that only a running binary shows.**
  Every web fixture starts from `WalkthroughT0()`, which has a live shift, so nothing
  caught that the finance order form's product picker asked `/api/products` — a route
  behind the *inventory* shift guard. With nobody on duty at the desk, a financial user
  saw an empty picker and could not choose a product at all. Fixed by giving the picker
  a `data-endpoint` and adding the session-gated `/finance/api/products`;
  `TestFinanceProductPickerWorksWithNoInventoryShift` pins it with the shift cleared.
  **Run the real binary as well as the suite.**

- `deepCopyFinance` was a shallow copy. `FinanceOrder` carries `Lines`, `Changes` and a
  `*int64`; sharing those backing arrays would let a **refused** transaction leave
  half-applied edits in the live decrypted data. It now deep-copies all three.
- Spec 18's float grep has no `!**/*_test.go` exclusion, unlike its `Products = append`
  grep on the line above. A float literal in a *test* therefore fails the gate. The tree
  is clean of floats today, tests included; keep it that way rather than amending the
  spec.
- Resolution of a typed party/purpose/mode must happen **inside** the `UpdateFinance`
  closure, not before it, or two simultaneous posts create two rows saying the same
  thing.
- `FinanceOrderLine.ID` is unique across the whole vault, not within one order, so
  `NextID("OLN")` scans every line of every order.
- **Resolving a shared value in its own transaction saves the file.** The settlement
  screens first resolved the party in a separate `UpdateFinance`, then recorded the
  settlement. A settlement refused for exceeding its cap had therefore already rewritten
  the register and could leave a newly created party behind with nothing pointing at it.
  Resolve inside the same write that stores the record.
- **A supplier can have goods here before finance has ever heard of them.** Inventory
  staff type the supplier straight onto the inward, so the return screen must work out
  availability from the typed name (`SupplierReturnAvailableByName`) as well as from a
  selected party, or the first return to any supplier reads "Only 0 can be returned".
- **Adding a field to `Register` breaks three things at once**, and the canary commit is
  how you find out cheaply: the golden `internal/store/testdata/walkthrough-t0.json` has
  to be regenerated, `register.WalkthroughT0()` must set the new slice to empty or a
  register in memory stops comparing equal to the same register read back off disk, and
  `TestEmptyRegisterEncodesEmptyArrays` catches a nil slice encoding as `null` in a file
  that is meant to be human-readable. Regenerate the golden file by running a throwaway
  `main` inside the module (an external one cannot import `internal/`).
- **`Store.Read` and `Store.ReadFinance` both take the same non-reentrant mutex.**
  Nesting them deadlocks the request with no error and no test failure — the run just
  hangs. Any screen needing the inventory record and the vault together must use
  `Store.ReadBoth`, which hands over both under one lock. This bit the order list and
  detail screens.

#### What spec 20 inherits

- `register.FinanceLineIsReferenced` still returns `false`. Spec 19 chose not to fill it
  in because a movement points at an order *line* only through `OrderLineIDs`, and spec
  18's guard is about a line disappearing under a ledger entry. **Spec 20 or a follow-up
  must make it report `len(m.OrderLineIDs) > 0 && contains(m.OrderLineIDs, lineID)` over
  live movements**, and the two partial spec-18 tests then become complete.
- `register.FinanceValueIsUsed` now covers order party and movement party, purpose and
  mode. Every settlement field naming a value must be added to it in spec 20.
- `Store.ReadBoth` is the only safe way to read the inventory record and the vault
  together.
- The placeholder handlers in `internal/web/finance_settlements.go` exist so the
  dashboard has no dead links. Spec 20 replaces all three.

#### What spec 19 inherited

- `register.FinanceLineIsReferenced` returns `false` and must start reporting whether a
  movement points at an order line. `lineRefusal` in `internal/web/finance_orders.go`
  already calls it and already carries the exact wording.
- `register.FinanceValueIsUsed` currently checks only `FinanceOrder.PartyID`. Every new
  movement field naming a party, purpose or payment mode must be added to it, or an
  administrator will be able to delete a value a movement still points at.
- `Store.ReadBoth` is the way to read the inventory record and the vault together.
- `financeAuditFor` in `internal/web/finance_orders.go` and `store.FinanceAudit` do the
  same job at two layers; spec 19 should use whichever layer it writes in rather than
  adding a third.

#### Known gap: two spec-18 tests depend on spec 19

`TestCancelPaidUndeliveredOrderKeepsHistory` and
`TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear` both require a money movement
pointing at an order line, and spec 18 explicitly forbids declaring the movement type.
The refusal path and its exact wording `This product is already used by a ledger entry.`
ship in spec 18 behind `register.FinanceLineIsReferenced`, which returns `false` until
spec 19 fills it in. Both tests are written and cover everything reachable today; their
movement-linked assertions are the named gap. **Spec 19 must implement that predicate and
complete both tests.**

Continuation details are also kept in `.agent-handoff/latest.md` after each milestone,
but that file is gitignored: never make it the only record of anything.

You are picking up a build in progress. This file tells you what exists, what is left,
how to check the work is sound, and which mistakes are easy to make here.

Read `CLAUDE.md` first — it holds the product decisions and the working agreements. This
file is the engineering state; that one is the why.

## What this is, in three lines

A single self-contained Windows `.exe` that replaces a handwritten inventory ledger at a
large gathering. It runs on **one Windows 11 laptop**, no installer and no runtime, as a
local HTTP server on `127.0.0.1` that serves its screens to the browser already on the
machine. All state is one human-readable JSON file beside the executable.

The people using it are not technical and have had no training. That constraint decides
more design questions than any other.

## Read in this order

1. `CLAUDE.md` — product decisions, constraints, working agreements.
2. `design/store-register.html` — the approved design, with screen mockups. **This is the
   source of truth for screens and wording.** Open it in a browser; it is a real page.
3. `specs/00-index.spec.md` — build order, binding conventions, every settled decision.
4. The numbered specs, `01` through `13`.

The specs are precise and have been reviewed twice: once for correctness, once by a
plain-language pass over every user-visible string. **Follow them exactly.** Where a spec
and your instinct disagree, the spec wins. Where a spec is genuinely wrong, implement the
Contract section and *report the defect* — every previous pass found real ones that way,
and that reporting has been more valuable than the code.

## State

| Specs | What | Status |
|---|---|---|
| 01–09 | Core model, persistence, arithmetic and entry flows | **Released in `v1.0.0`** |
| 10–12 | Read-only views, corrections and activity log | **Released in `v1.0.0`** (`c0adcdc`) |
| 13 | One issue may name multiple joint recipients | **Released in `v1.0.1`** (`77a40f0`) |
| 14–16 | Global desk controls, product rename and cascading delete, issue challans and return search | **Released in `v1.1.1`** (`3bc9535`) |

### Before the new `.exe` goes on the laptop — do this in order

`v1.1.1` writes schema 2. `v1.0.1` cannot read it, and **does not refuse it cleanly**:
its `store.Open` treats a version mismatch exactly like a damaged file, copies the main
file aside and falls back to `.bak`, which directly after the first `v1.1.1` save is the
schema-1 pre-upgrade file. The old program therefore opens on pre-upgrade data behind
the ordinary "Today's register was damaged" banner, and its next save writes schema 1
back over the main file. Nothing is destroyed — the schema-2 file is kept as
`store-register.json.corrupt-<timestamp>` — but the banner blames damage when the real
cause is that somebody ran the old program, and nothing on screen says so.

This cannot be fixed in `v1.1.1`; the defect is in the already-released `v1.0.1` reader.
The user's decision, 1 September 2026, is to handle it procedurally:

1. Copy `store-register.json` somewhere safe.
2. Delete the old `.exe` from the laptop **and** from the pen drive.
3. Put the new `.exe` in its place.

Verified against real binaries by the `v1.1.1` release gate. With no `.bak` beside it,
`v1.0.1` refuses correctly and touches nothing; after two `v1.1.1` saves both files are
schema 2 and it refuses correctly again. The exposure is the window in between.

### `v1.1.1` verification, all reproduced independently by the release gate

372 tests pass under `-race -count=1` with no failures and no skips; `go vet` is clean;
the cgo-disabled Windows amd64 build produces a PE32+ executable; all 57 tests named as
required by specs 14, 15 and 16 exist and pass; every must-print-nothing grep is silent.
`internal/register` statement coverage is 95.4% against the documented 95% minimum —
down from 97.1%, and the uncovered statements are guard returns the web layer checks
before calling, plus the `Validate` abort at `product.go:145`.

Spec 16's browser acceptance scenario passed in full: all 9 steps through Playwright
against a real native binary, 67 checks plus 11 more after restarting the binary against
the saved file, and again server-rendered with no JavaScript at all. Under real
concurrency at a live binary, 40 simultaneous issues of 10 against 100 on hand accepted
exactly 10 and refused 30; 20 simultaneous returns against a 10-chair holding accepted
exactly 1; a delete racing 15 issues on the same product left no live record pointing at
a deleted product.

`v1.0.1` is the selected version for spec 13. The strict release matrix passes on the
current tree: the full race suite, vet, Windows amd64 cross-compile, store and feature
tests, architecture/vocabulary greps, and 97.1% `internal/register` statement coverage
(the documented minimum is 95%).

The read-only `plain_language_reviewer` checked 175 built strings and found only the
known no-supplier wording conflict recorded below; no code change is warranted while
the two specs disagree. Native Linux binaries passed both the original end-to-end smoke
and a spec 13 smoke that stored one 30-chair issue for Ravi, Amit and Suresh, counted it
once, found it through Amit, and preserved the ordered recipients in JSON. The spec 13
contributor reports that the enhanced workflow passed on Windows; Codex independently
verified the cross-build and automated matrix but did not reproduce that manual run.

Known spec-text defects found while implementing the required tests:

- Spec 15 stated that a `v1.0.1` executable refuses a schema-2 file. It does not; see
  the upgrade steps above. Corrected in `83f0a0b`.
- Spec 15's verification commands grepped `internal/web/products.go` for `store.Update`,
  which prints nothing because the code calls `s.st.Update`. The guard was present the
  whole time; the grep could not see it. Corrected in `83f0a0b`.
- Spec 00's greps used unquoted `--include=*.go` globs, which this project's fish shell
  expands before `grep` sees them, so they failed outright rather than passing.
  Corrected in `83f0a0b`.

- Spec 11 shortens the fixture's full return remark in two examples. The implementation
  preserves the complete stored remark.
- Spec 12 says deleting `INW-0002` from T1 leaves 390 chairs received, but live
  `INW-0007` contributes another 500; the correct total is 890.
- Specs 11 and 12 prescribe different no-supplier wording for the shared `entryName`
  helper. The implementation conservatively preserves spec 11's existing-screen wording.
- Several spec 12 required-test paragraphs retain older wording that contradicts the
  later normative Contract. Tests and implementation follow the Contract.
- Spec 00's original forbidden-word greps scanned `design/store-register.html` and
  test names, so they failed on historical, non-shipped prose and tests that prove the
  forbidden UI is absent. The verification commands now scan shipped code only and
  exclude `*_test.go`.

## How to check the work

These are not optional and they are how every pass has been verified:

```
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .   # note the '.', not './...'
```

Plus the whole-build block at the foot of `specs/00-index.spec.md`, which greps for the
things that must never appear: `0.0.0.0`, money vocabulary, authentication vocabulary,
and any third-party dependency.

**Do not trust a report that the tests pass. Run them.** Two of the verification commands
in the specs were themselves broken and could never have passed as written; they were only
found because someone ran them.

## Running it for real

Static tests are not enough for this program. Build it into an empty directory and drive
it, the way each pass has been checked:

```
mkdir /tmp/e2e && go build -o /tmp/e2e/register . && cd /tmp/e2e && ./register &
curl -s http://127.0.0.1:8765/shift
```

**The trap:** the program hunts ports 8765→8785 when one is busy. A leftover process from
an earlier test keeps 8765, your new binary silently takes 8766, and you spend twenty
minutes debugging a stale server. Kill by PID from the socket table before each run:

```
for p in 8765 8766 8767; do
  pid=$(ss -ltnp | grep ":$p " | grep -oP 'pid=\K[0-9]+' | head -1)
  [ -n "$pid" ] && kill -9 $pid
done
```

A full working scenario, useful as a smoke test — add a person, start a shift, add a
product, 500 chairs in, refuse 900 out of 500, issue 40 then 10 to Ravi, return 45 of his
50 via a different person, and confirm the shortfall is attributed to Ravi and not to the
returner. Every one of those steps has been driven successfully against the real binary.

## Architecture, and the lines not to cross

```
main.go                startup, port hunt, browser launch
internal/register/     model, ids, fixture, arithmetic, allocation, Validate
internal/store/        atomic save and load
internal/web/          handlers, templates, static assets
```

- **Standard library only.** Zero third-party dependencies, and `go list -deps` proves it.
  If you think you need one, you have misread the problem.
- **No cgo, ever.** It would break the Windows cross-compile, which is the whole delivery
  mechanism. This rules out `mattn/go-sqlite3` and every native GUI toolkit.
- `internal/register` and `internal/store` must not import `net/http`.
- `internal/web` must not do arithmetic. It calls `internal/register`.
- **Bind `127.0.0.1` only.** Never `0.0.0.0`, never `:port`. `0.0.0.0` raises the Windows
  Defender Firewall dialog, and a non-technical user facing a security prompt is a failed
  handover.
- `internal/register/arith.go` must never see deleted records; `internal/register/log.go`
  must. That is why the log builder lives in its own file, and spec 03 has a grep
  asserting the separation. Do not merge them.
- Times in fixtures use `time.FixedZone("IST", ...)`, never `time.LoadLocation` — the
  tzdata files are absent from a `CGO_ENABLED=0 GOOS=windows` binary.

## Things that will bite you

- **Data loss is the worst possible outcome.** A day of entries at a live event cannot be
  recreated. The save path is temp file → fsync → rename current to `.bak` → rename temp
  over the real path, and a crash at any point must leave one good file or the other.
  Treat any change to `internal/store` as high risk and read its tests first.
- **A duplicate product silently halves the on-hand count** and nobody notices until it is
  too late. That is why product names are picked and never free-typed. A duplicate
  *person* is fine and must never be blocked — the asymmetry is deliberate and explained
  in `00-index.spec.md`.
- **The taker is not the returner.** When Suresh hands back chairs Ravi took, the
  shortfall stays against Ravi and every sentence says Ravi. Easy to get subtly wrong.
- **Over-issue and over-return re-check inside the `store.Update` closure**, against the
  register at that instant, not the number rendered on the page. Two browser tabs must not
  both spend the last ten chairs.
- **Deleting an issue that a live return points at** is invisible to `Validate` — the
  over-allocation check walks live issues while the return still counts its orphaned
  allocation, inflating on-hand. Spec 03 implements three invariants by design; this guard
  belongs in spec 11's delete path. Confirm it exists and is tested.
- **Never reword a user-visible string.** They have been through a plain-language review
  and are asserted verbatim in tests. If one reads badly, change the spec and the test
  together, and say why.

## Known open, nothing blocking

- **Nine wording findings against `v1.1.1` strings are unactioned.** The
  `plain_language_reviewer` flagged `Dashboard` (a second, differently-named link to
  `/stock`, which the tab bar already labels `Stock`), `Change person` (reads as "change
  who is taking this" mid-entry), the cascading-delete impact block (`Received entries:`
  / `Issue entries:` / `Return entries:`, and "the working register", a phrase on no
  other screen), `Outstanding entries with this challan` and `No outstanding issue
  matches that challan number.` ("outstanding" is a finance word that reads as
  "excellent" to a second-language reader, while the row below says plain `20 chairs
  out`), and `Pick this holding` (does not say what tapping it does). The release gate
  added a tenth: `Currently out: 1 chairs` on the delete screen violates normative
  decision 10. All are wording, none are defects; each needs its spec and its verbatim
  test amended together. Put to the user 1 September 2026, deferred, not declined.
- **The save-time recheck tests are not load-bearing.** The release gate disabled, one
  at a time, the in-closure `impactVersion` recheck, the in-closure rename duplicate
  check, `Validate` inside `DeleteProductCascade`, the in-closure `CheckIssue`, and
  `Product.Changes` deep-copying — and the full suite stayed green for all five. The
  code is correct and was proved so empirically under real concurrency, but every
  "rechecks at save time" test stages its conflict *before* the POST, so nothing
  distinguishes a check inside the `store.Update` closure from one outside it. A future
  refactor could move one out and no test would notice. This is the code that stops two
  browser tabs spending the last ten chairs; worth strengthening.
- **Quantities have no upper bound**, in the field since `v1.0.0` and untouched by
  `v1.1.1`. The inward field is `min="1"` with no `max` and there is no server-side
  limit; the gate overflowed int64 and drove on-hand to a large negative number. The
  realistic failure is not the overflow but one fat-fingered `50000000` inward silently
  corrupting the headline number. Needs its own spec.
- **The activity log's person filter records the returner, not the taker**
  (`internal/register/log.go:114`). Searching the log for the taker misses the return
  that put their goods back. The release gate confirmed this is what
  `specs/12-activity-log.spec.md:115` actually prescribes, and that `/out` attributes
  the shortfall correctly, so no number is wrong — it is discoverability only. Changing
  it means amending spec 12.

- **Two copies of the program can run at once**, on 8765 and 8766, showing two views of
  one file. Harmless in tests, plausible at a gathering if somebody double-clicks the icon
  twice. Worth deciding whether the second copy should open the first instead of starting
  its own. Not yet specced.
- **Unverifiable from Linux**, and needing a real Windows check before the event: browser
  launch via `rundll32`, `os.Executable()` resolving the data file beside the `.exe`,
  first-run from a folder the user cannot write to, how Edge renders `<input type="date">`
  and `datetime-local`, and the four JavaScript files in a real browser. Every scripted
  path has a server-rendered equivalent that the tests cover, so the register works with
  scripting off.
- `specs/07-inward.spec.md` open item 4: a supplier typed as `Sharma tent house` still
  becomes a second supplier row. Products are guarded, suppliers are not. Unconfirmed.
- The date hint says `Type the date like this: 03-09-2026.` while the field parses
  `YYYY-MM-DD`. Rendered as specced; someone should decide which is wrong.

## Working with this user

Recorded properly in `CLAUDE.md`, but the two that matter most:

- **Propose, do not interrogate.** One round of genuinely blocking questions, then act on
  stated assumptions. Repeated clarifying rounds read to him as going backwards, and he
  has said so.
- **Review agent output yourself and surface only what needs his judgement.** He said
  explicitly he will not review anything if nothing is flagged. Settle what you can,
  record it as a decision, and bring him the two or three that genuinely need him.

Keep messages short.
