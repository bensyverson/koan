# Backlog

Work decided against, parked rather than dropped in silence. Nothing here is scheduled or blocking — active work lives in `job`. Read it before proposing something that sounds novel; it may already have been weighed and parked.

- **One dated H2 per item:** what it is, why it's parked, and *what would un-park it*.
- **Delete an item when it lands, or when it stops being plausible.** A long list is one nobody reads.
- **Something parked that turns out to be needed becomes a task in `job`** — move it, don't work it from here.

Format: one dated H2 per entry, a headline, then what it is, why it's parked, and the trigger that would revive it.

---

## 2026-09-05 — Two different draws from one bag in a single template

`{noun} with {noun}` meaning two nouns. In v1 a repeated slot repeats the value, because a slot is a
variable. Parked because no template needs it yet and the syntax for a second draw is a decision
worth making against a real case. Un-parked by a real template that needs it, with the syntax
decided then. Source: [2026-09-05-koan-vision-and-plan.md](2026-09-05-koan-vision-and-plan.md).

## 2026-09-05 — Shuffle, pick k of n, integer ranges

Tempting because they would share the seed with the draw, and one seed per round was the
strongest idea from the design loop that motivated the tool. Parked because `-n` covers the cases seen so far and each
primitive is a shape the tool would then have to explain. Un-parked by a consuming script that
needs one and cannot get it from `-n`.

## 2026-09-05 — Directories or stdin as bags

`ls refs/ | shuf -n 1` already picks a random file, and stdin is reserved for the template.
Un-parked by a consumer that needs a directory listing to share the draw's seed.

## 2026-09-05 — A hidden history or well-known state directory

The used directory is the only state and is always named on the command line, per
`cli-design.md`'s "ephemeral by default, state by name". Un-parked by nothing foreseeable; the
JSONL a caller tees into a log is the history.

## 2026-09-05 — Locking the used directory

Two concurrent `--used` runs can interleave appends. Parked until an observed collision, because
the loop that consumes this draws once per round from one process.

## 2026-09-05 — Header detection in `.csv` bags

A `#` line is the header. Guessing that the first row is a header is the same lie as guessing
plurals. Un-parked by nothing foreseeable.

## 2026-09-05 — Atomic recording to the used directory

`Record` appends to one file per bag as it goes, so a write that fails on the third bag leaves
the first two appended: "a failed run writes nothing" holds, "a failed *record* writes nothing"
does not. All-or-nothing wants a temp file and a rename per bag, which sits beside the locking
question above. Parked until a record failure is observed in practice; a full disk or a
permission change mid-run is the trigger, and the fix is the same as for locking.

## 2026-09-05 — Multi-line `.csv` values in a `.txt` used file

RFC 4180 allows a quoted value to span lines, and the loader honours that. Recording such a
value into a fresh `<bag>.txt` used file writes it across several lines, so the next run's
exclusion by value cannot match it and excludes its fragments instead. Parked because no
curated bag has a multi-line value and the register the tool serves is one line per idea.
Un-parked by a real bag that needs multi-line values under `--used`; the fix is for `Record`
to create `.csv` rather than `.txt` when any value for that bag carries a newline.

