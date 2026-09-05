# 2026-09-05 — koan: a reproducible random draw over hand-curated lists

`koan` is a single Go binary, usable on any project, that draws random values from lists a
human wrote and renders them into a template. Its purpose is to push a language model out of
the centre of its distribution by handing it something it would never have chosen. It knows
about lines, files, seeds and templates, and nothing else: not design, not critics, not rounds.

It grows out of a one-off `koan.go` script and the four-round slide-deck design loop it served on
a client project (2026-09-03 to 2026-09-04). Those rounds established the facts this tool is
built on: adjectives retune an artefact while a hard constraint moves it; a printed
seed makes any draw reproducible; the vocabulary must be human-curated, because a model-written
list sits at the centre of the distribution the draw is trying to leave; and a constraint that
collides with a ruling is the brief's problem, not the generator's.

Decisions in this document are Ben's (2026-09-05) unless marked *open*.

## The one thing

A **bag** is a file of values. A **template** is a line with `{slots}`. A **draw** pulls one
value per slot from the bag of the same name, seeded, and renders the template. That is the
whole tool. Everything that made the design loop work — designer roles, media, critics,
ledgers — is a shell script that calls `koan`, written in the repo that needs it. The loop is
the tool's first consumer and the proof it is agnostic.

`shuf -n 1` already does the atom. `koan` earns its place on exactly these things and nothing
else: one seed governing several bags at once, printed so a log can carry it; a template bound
to bags by name; a used-directory that subtracts past draws with no hidden state; and errors
that name the file and the fix.

## The contract

### Invocation

```
koan <template> [--bags <dir>] [--used <dir>] [--seed <n>] [-n <k>] [--json]
```

- `<template>` is a file path, or `-` for stdin. Its `{slot}` names bind to bags by basename.
- `--bags` defaults to the template's own directory (the working directory for stdin), so a repo
  that keeps patterns beside vocabulary runs `koan art-director.txt` with nothing else.
- `--seed` reproduces a draw. Omitted, the seed comes from the clock. Either way stderr carries
  `seed=N`, so `--json` piping stays clean and a log always has the number.
- `-n` draws k times from one seeded stream: `-n 5` extends `-n 3` rather than replacing it.
  Within one invocation a bag never repeats a value across the k draws, so three designers in a
  round cannot share a constraint.
- `--used <dir>` both excludes and records. Values found in `<dir>/<bag>.txt` are removed from
  the pool before drawing; after a fully successful run, every drawn value is appended there.
  No flag, no exclusion, no recording: the tool is ephemeral by default, and the directory is
  the only state, named on every call, created when first named.
  > **Corrected 2026-09-05, while building the used leaf.** The used directory is read as a
  > bags directory, so `<dir>/<bag>.csv` is honoured on exclusion as well as `.txt`, with the
  > same exact, case-sensitive resolution. Recording appends to whichever file already backs the
  > bag, through the CSV encoder when it is `.csv`, and creates `<bag>.txt` only for a bag with no
  > used file yet; appending `.txt` beside an existing `.csv` would manufacture the ambiguous pair
  > the loader refuses. Decided on the used leaf (`hGODxR`) in `job`.
- `--json` switches the output shape (below). `--version` prints the build.

### Bags

- A bag lives in the bags directory as `<name>.txt` or `<name>.csv`. `{adjective}` looks for
  `adjective.txt` then `adjective.csv`. Names match basenames exactly, case-sensitive, with no
  plural or singular guessing.
- **Both present is an error**, naming both paths, never a precedence.
- **`.txt`: a line is a value, verbatim.** Nothing is parsed. Commas, quotes and apostrophes are
  literal. This is where prose belongs.
- **`.csv`: RFC 4180 through `encoding/csv`, first column is the value**, further columns are
  ignored and reserved for the author (comments today, tags or attribution tomorrow). Ragged
  rows are allowed. A comma inside a value must be quoted, and the README says so up front,
  because an unquoted comma silently produces a shorter value that still reads as valid.
- In both formats a line whose first non-blank character is `#` is a comment, blank lines are
  skipped, and values are trimmed. A `#` line is also how a `.csv` carries a header.
- **A duplicated line is a weight.** Under `--used`, exclusion is by value, so a used value is
  removed however many times it appears.
- A missing bag, an empty bag, an empty first cell and a CSV parse error are all hard errors
  naming the path and, where there is one, the line. A bag can never hold a hole and a template
  can never render short.

### Templates

- A slot is `{name}` where name is `[A-Za-z0-9_.-]+`. Anything else is literal text; there is no
  escape syntax. The same slot twice in one template repeats the value.
- Slots draw in order of first appearance, so a template's draw is a pure function of the seed,
  the bags and the used directory.
- **Plain output requires a single-line template.** One draw per line means `head`, `grep`,
  `while read` and `xargs` all work with no ceremony. A multi-line template in plain mode is a
  usage error whose message names `--json` as the fix.

### Output

Plain: one rendered line per draw on stdout, `seed=N` on stderr.

`--json`: JSON Lines, one object per draw, each self-contained so a line cut out of a log can be
reproduced without its neighbours.

```
{"seed":412,"i":0,"template":"art-director.txt","slots":{"adjective":"chalky","quality":"candor","muse":"cartographer","constraint":"Only one typeface, only two sizes"},"text":"Needs more chalky candor. Channel your inner cartographer. Constraint: Only one typeface, only two sizes."}
```

The used directory is a pool subtraction, not a history. The history is the JSONL a caller
chooses to `tee` or `>>` into a log.

### Reproducibility, stated honestly

A draw is a pure function of the seed and the pool. Under `--used` the pool shrinks over time,
so the same seed on a later day gives a different draw. To reproduce, pass the same seed against
the same bags and omit `--used`. Once a used directory or a JSONL log exists, the values are
already recorded and reproduction no longer needs the seed at all.

### Exit codes

The shared house table from `cli-design.md`:

| Code | When |
|---|---|
| 0 | Drew, or `lint` found nothing |
| 1 | `lint` found something |
| 2 | Malformed invocation: bad flag, `-n` under 1, multi-line template without `--json` |
| 3 | A bag ran out: `--used` or `-n` exhausted it. The message names the bag, the counts and the remedy |
| 4 | A target could not be loaded: missing or empty bag, unbound slot, ambiguous `.txt`/`.csv` pair, CSV parse error, unreadable template |
| 5 | Environment: the used directory cannot be created or written |

Every error names the thing by its path, says what happened, and says the next command.

### Verbs

`koan` has one act, so the act is the root command: `koan <template>`. Bare `koan` is the primer.
The subcommands are all reads and all work with nothing set up:

| Verb | Class | Does |
|---|---|---|
| `koan` | help | The primer: what the tool is for, the verbs by role, the one command to run first |
| `koan <template> …` | act | Draw |
| `koan bags [dir] [--used dir]` | read | One row per bag: name, format, values, used, remaining, a problem flag. `--json` for the same |
| `koan lint <template>… [--bags] [--used]` | assert | Everything draw would refuse, reported for every template at once without drawing; exit 1 on a finding |
| `koan recipes` | help | Goal → command routing |
| `koan schema` | help | The template grammar, the JSONL shape and the exit table |

`<verb> --help` carries one worked example each. The aim is that an agent running `koan` bare
and then `koan --help` needs no other document to use it correctly. A template file that happens
to be named `bags` is addressed as `./bags`.

Cobra provides the grammar (`github.com/spf13/cobra`, already the house choice in `agents`,
`Jobs`, `kura`). It is the only dependency.

## Shape of the repository

```
koan/
  go.mod                      module path: open
  README.md                   what it is, install, quick start, the .csv comma warning
  bag.go bag_test.go          resolve, load, comments, weights, ambiguity, errors
  template.go template_test.go
  draw.go draw_test.go        seeded stream, -n, without-replacement, exhaustion
  used.go used_test.go        exclude, record after success
  cmd/koan/                   thin Cobra adapter: root, bags, lint, recipes, schema
  cmd/koan/cli_test.go        drives the built binary on fixture copies in a temp dir;
                              asserts bytes and exit codes; every error path reads its remedy
  testdata/                   fixture bags and templates, including the unquoted-comma case
  examples/art-director/      the original vocabulary as a starter set, with the template
  project/                    this document and what follows it
```

Library + thin executable: the root package exposes `LoadBags`, `ParseTemplate`, `Draw` and the
used-directory functions; `cmd/koan` holds no decision the library does not.

## What is deliberately not in v1

Each is an entry in [backlog.md](backlog.md) with what would un-park it; the headlines:

- **Two different draws from one bag in a single template** (`{noun} with {noun}` meaning two
  nouns). Repeating a slot repeats the value. Un-parked by a real template that needs it, with
  the syntax decided then.
- **Shuffle, pick k of n, integer ranges.** Tempting because they would share the seed. Un-parked
  by a consuming script that needs one and cannot get it from `-n`.
- **Directories as bags, and stdin as a bag.** `ls refs/ | shuf -n 1` already covers a random
  file, and stdin is reserved for the template.
- **A hidden history or a well-known state directory.** The used directory is the only state
  and is always named.
- **Locking the used directory.** Two concurrent `--used` runs can interleave appends. Un-parked
  by an observed collision.
- **Header detection in `.csv`.** A `#` line is the header.

## Open decisions

- **Module path** for `go.mod` (a GitHub path under Ben's account, presumably).
- **License**, for the README's last line.
- **Whether `lint` ships in v1.** It is draw's validation without the draw, so it is nearly
  free, and it is the only way to check every template in a directory at once. The plan
  includes it; cut it if it drags.

> **Decided 2026-09-05, the same day, before the scaffold leaf.** Module path
> `github.com/bensyverson/koan`; MIT licence; `lint` ships in v1. Recorded on the scaffold leaf
> in `job`.

## Follow-ups outside this plan

- In the client project: retire the one-off script and its test, move the vocabulary to
  `.txt` where lines carry commas (all ten are in `constraints`), rename the bags to the
  singular so the template reads `{adjective}`, and point the design-loop learnings at this
  repo. Filed in that project's own tracker when v1 ships.
- The learnings doc's "playbook" becomes an `agents` module whose tooling step names `koan`.

## Plan

Leaves are units of work grouped by the files they touch; TDD is strict red/green throughout,
and the CLI leaf's tests drive the built binary. Import with `job import <this file> --dry-run`
first, then for real.

```yaml
tasks:
  - title: "koan v1: a reproducible random draw over hand-curated lists"
    desc: |
      A single Go binary, usable on any project, that draws seeded random values from
      hand-curated bags and renders them into a template, to push a language model out of
      the centre of its distribution. The contract, the exit table and the verbs are in
      project/2026-09-05-koan-vision-and-plan.md; this tree implements that document and
      nothing beyond it. The list of what v1 deliberately leaves out is in the same document.
    labels: [koan, v1]
    children:
      - title: "Scaffold the repository"
        ref: scaffold
        desc: |
          go.mod on the agreed module path, Cobra as the only dependency, `job init`, and a
          README skeleton with the one-line description and the quick start left to be filled
          by the README leaf. AGENTS.md (via the `agents` tool), .gitignore and project/ were
          set up on 2026-09-05 with the plan and need no work. Module path and license are
          open decisions in the plan doc; ask Ben rather than guessing.
        criteria:
          - "`go build ./...` and `go vet ./...` pass on an empty main"
          - "`agents check` passes: no region is stale or hand-edited"
          - "The module path and license were confirmed by Ben, not assumed"
      - title: "Bags: resolve, load and validate .txt and .csv"
        ref: bags
        blockedBy: [scaffold]
        desc: |
          The library's bag loader. A slot name resolves to `<name>.txt` then `<name>.csv`
          in the bags directory, exact and case-sensitive. Both present is an error naming
          both paths. `.txt` is a line per value, verbatim, nothing parsed. `.csv` is RFC 4180
          via encoding/csv with ragged rows allowed and the first column as the value. Both
          skip `#` lines and blanks and trim values. Duplicates are kept, because a duplicate
          is a weight. Missing bag, empty bag, empty first cell and CSV parse error are typed
          errors carrying the path and line so the CLI can map each to exit 4 and a remedy.

          Tests first, all red: each format, comments, weights, ambiguity, every error, and
          the unquoted-comma case in `.csv` asserted as the documented truncation so it is
          intended rather than discovered.
        criteria:
          - "Every error type is a distinct Go type carrying path and, where there is one, line"
          - "An unquoted comma in a .csv value is covered by a test that documents the truncation"
          - "A .txt value containing commas and quotes loads verbatim"
          - "adjective.txt beside adjective.csv is refused naming both paths"
      - title: "Templates: parse slots and render"
        ref: template
        blockedBy: [scaffold]
        desc: |
          Parse a template into literal runs and `{slot}` names matching [A-Za-z0-9_.-]+,
          preserving order of first appearance and recording whether the template spans more
          than one line. Render from a map of slot to value; a repeated slot repeats the value.
          No escape syntax: unmatched braces are literal. Unbound slots are the draw's concern,
          but the parser exposes the ordered slot list so lint and draw share it.
        criteria:
          - "Slot order is order of first appearance and a repeated slot appears once in the list"
          - "A template with a newline reports multi-line"
          - "Braces that do not form a slot render literally"
      - title: "Draw: seeded stream, -n and without-replacement"
        ref: draw
        blockedBy: [bags, template]
        desc: |
          Given bags, a template, a seed and n, produce n draws from one PCG stream seeded
          (seed, 0), pulling slots in template order, and never repeating a value within a bag
          across the n draws of one invocation. The n+1th draw of a run equals the extra draw
          of a longer run with the same seed. When a bag cannot supply a further distinct
          value, return a typed exhaustion error naming the bag, the pool size and how many
          were needed, so the CLI can map it to exit 3 with the remedy. Bind unbound slots to
          a typed error for exit 4.
        criteria:
          - "The same seed and bags produce byte-identical draws across runs"
          - "-n 5 begins with the three draws of -n 3 under the same seed"
          - "A bag of two values with -n 3 fails with the exhaustion error, not a repeat"
          - "A slot with no bag fails naming the slot and both paths tried"
      - title: "Used directory: exclude before, record after"
        ref: used
        blockedBy: [draw]
        desc: |
          `--used <dir>` reads the directory as a bags directory and subtracts every value
          found in `<name>.txt` or `<name>.csv` from the matching pool, by value, however many
          times the bag repeats it. After a fully successful run it appends each drawn value to
          `<dir>/<name>.txt`, creating the directory and files as needed; nothing is written if
          any draw failed. Failure to create or write maps to exit 5. No locking in v1, noted in
          the plan doc's not-in-v1 list.
        criteria:
          - "A value present in used/<bag>.txt is never drawn"
          - "After a run of -n 3 each drawn value appears once in the used file"
          - "A run that fails on exhaustion writes nothing"
          - "Exclusion of a weighted value removes every copy"
      - title: "CLI: root draw, flags, output modes and exit codes"
        ref: cli
        blockedBy: [used]
        desc: |
          The Cobra adapter. Bare `koan` prints the primer. `koan <template>` draws, with
          `--bags` defaulting to the template's directory (cwd for `-`), `--used`, `--seed`,
          `-n` and `--json`. stderr always carries `seed=N`. Plain output is one line per draw
          and refuses a multi-line template with a message naming `--json`. `--json` is JSON
          Lines with seed, i, template, slots and text per line. Every library error maps to
          the plan doc's exit table with a message that names the path, what happened and the
          next command. Tests drive the built binary on fixture copies in a temp dir and assert
          bytes and exit codes; every error path has a test that reads its remedy.
        criteria:
          - "Bare koan exits 0 with the primer and no file, network or state"
          - "Golden tests pin plain and JSONL output for a fixed seed"
          - "Each row of the exit table has a binary test producing that code"
          - "The multi-line-without-json error names --json"
      - title: "Read verbs: bags, lint, recipes, schema and per-verb help"
        ref: verbs
        blockedBy: [cli]
        desc: |
          `koan bags [dir] [--used dir]` prints one row per bag with name, format, values,
          used, remaining and a problem flag, and `--json` for the same. `koan lint <template>…`
          reports everything draw would refuse, for every template given, without drawing, and
          exits 1 on a finding. `koan recipes` routes goals to commands. `koan schema` prints
          the template grammar, the JSONL shape and the exit table. Every verb's `--help`
          carries one worked example. The acceptance test is an agent reading only the tool's
          own output: run `koan`, then `koan --help`, then each verb's help, and check that the
          quick start in the README adds nothing the help did not already say.
        criteria:
          - "koan bags on the examples directory prints one row per bag with counts"
          - "koan lint exits 1 on a template with an unbound slot and 0 on a clean one"
          - "Every verb's --help includes a worked example"
          - "recipes and schema run with no arguments and no files present"
      - title: "Starter examples and README"
        ref: readme
        blockedBy: [verbs]
        desc: |
          Copy the original vocabulary into examples/art-director/ as singular bags
          (adjective, quality, muse as .csv with an empty comment column; constraint as .txt
          because ten of its lines carry commas), with the art-director template beside them
          so `koan examples/art-director/art-director.txt` runs bare. Write the README to the
          house shape: name and one-line description a non-technical reader understands, one
          paragraph of what it is, install, a quick start with two examples, the .csv comma
          warning, the reproducibility caveat, links to project/, authorship and license.
          Verify the whole quick start by running it.
        criteria:
          - "Every command in the README's quick start was run and its output matches"
          - "The README states the .csv comma rule and the reproduce-without---used rule"
          - "examples/art-director draws with no flags from the repo root"
      - title: "Self-teaching audit as an agent would use it"
        ref: audit
        blockedBy: [readme]
        desc: |
          The kick-the-tires step from cli-design.md, run at the end because the nearest
          existing tool was already used for four real rounds. Give a fresh agent the binary
          and a task (draw three koans for a round against a used directory, then report
          coverage) with no document but the tool's own output. Record what it had to guess in
          a dated DX note under project/, pair each finding with the rule it implies, and fix
          the help or the errors rather than the note.
        criteria:
          - "A dated DX note under project/ pairs each finding with a rule"
          - "Every finding was fixed in the tool's own output, or filed in backlog.md with what un-parks it"
```
