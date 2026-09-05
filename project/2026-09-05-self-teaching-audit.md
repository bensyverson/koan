# 2026-09-05 — Self-teaching audit: a fresh agent, the binary, and a round to draw

The kick-the-tires step from `project/agents/cli-design.md`, run at the end of v1. A fresh agent
was given the `koan` binary built from `613be13`, a copy of `examples/art-director/` renamed
`vocabulary/`, and this task, with orders to read nothing but the tool's own output:

> Draw three koans for tonight's design round against a used directory called `used/`, log the
> draws to `round.jsonl`, report the coverage per bag, show how to reproduce one of the three
> draws tomorrow from the log alone, and check whether the vocabulary would still support a
> round of five.

The main task ran correctly on the first try, from bare `koan`, `koan schema` and `koan recipes`
alone. The agent then probed the edges and found ten things, three of them real defects. Each
finding below is paired with the rule it implies and with what was done about it. Findings fixed
in the tool are in commit `wt/audit`'s squash (see the git log after this document's date);
findings parked are in [backlog.md](backlog.md).

## Findings

### 1. The exhaustion remedy named the wrong number — fixed

`-n 100` said `draw -n 48 or fewer`; `-n 48` then failed on a different bag; four runs later the
real ceiling turned out to be 28. `Draw` reported the first bag in slot order that fell short,
not the tightest one, while `koan bags` already had all four counts.

**Rule:** a remedy that names a number must name the binding one. Compute over every
constraint before reporting the first, or the tool states a falsehood in the one place a user is
guaranteed to trust it.

**Done:** `Draw` now checks every slot and reports the bag with the smallest remaining count.

### 2. `lint --used` advertised a guard it did not provide — fixed

`koan lint --help` showed `koan lint t.txt --used d && koan t.txt --used d` as "would tonight's
round still have values left?". With a bag down to two values, `lint` said `ok` and `-n 5`
exited 3. Lint only knew about a fully spent bag; the question it advertised was about a round
of *n*.

**Rule:** an assertion verb accepts every parameter that changes the act's outcome, or its
`&&` idiom is a lie. If the draw takes `-n`, the check takes `-n`.

**Done:** `koan lint -n <k>`; a bag with fewer than k distinct values left is a finding.

### 3. The reproducibility promise omitted its precondition — fixed in the help

`schema` and `recipes` said: same seed, same bags, omit `--used`. That is true for a round drawn
against an empty pool and false for every round after it, because the pool the seed indexes has
shrunk. The agent replayed a second-round seed without `--used`, got two of four slots matching,
and noted that a user eyeballing the sentence would believe they had reproduced it. The plan's
own reproducibility section already says the honest thing: once the JSONL exists, the line holds
the draw and reproduction no longer needs the seed. The help did not.

**Rule:** state a reproducibility promise with its precondition in the same sentence. A partial
match is more dangerous than a mismatch, because it looks like success.

**Done:** `schema` now says a draw reproduces from its seed only against the same pool, that a
draw made under `--used` against a non-empty pool will not come back from `--seed` alone, and
that the log line is the history. `schema` also says values draw in file order, so reordering a
bag changes what a seed draws, which the agent asked and nothing answered.

### 4. `koan bags` listed the template as a bag — fixed

The default layout keeps the template beside its bags, so `koan bags vocabulary` listed
`art-director` as a one-value `.txt` bag with no problem flag, and the agent's coverage report
carried the bogus row. The row is a true statement about a `.txt` file and a wrong answer to the
question asked.

**Rule:** when the tool's own default layout puts a non-target next to the targets, the read
verb needs a target-scoped form. Here the template is the scope.

**Done:** `koan bags <template>` lists only the bags that template binds, in slot order, with
a missing bag as a problem row. `koan bags <dir>` is unchanged.

### 5. No way to ask "would a round of n fit?" without spending the pool — partly fixed

`--used` reads and writes; there is no dry run. The agent copied the used directory and drew
against the copy to answer the question.

**Rule:** every act with a side effect has a read that answers "would it succeed". An assertion
verb is the cheapest form.

**Done:** finding 2 makes `koan lint -n 5 --used d` that read. A `--dry-run` on the draw itself
is parked in the backlog with its trigger.

### 6. Draw *i* cannot be asked for directly — recipe added, flag parked

The JSONL carries `i`; the tool does not accept one. The agent guessed `--index 1`, got exit 2,
and then inferred the `-n <i+1>` plus `sed -n '<i+1>p'` idiom from the recipes line about one
stream extending.

**Rule:** when the output carries an index, either accept it as input or show the idiom that
stands in for it.

**Done:** `recipes` shows the idiom for reproducing draw *i*; `schema` explains `i`. `--index`
is in the backlog.

### 7. The primer's one-line summary of `lint` contradicted its behaviour — fixed

"Report everything a draw would refuse" while a multi-line template lints `ok` and plain-mode
draw refuses it. `lint --help` disclosed the carve-out; the primer did not.

**Rule:** a verb's one-line summary must be true without its long help.

**Done:** the primer line names the carve-out.

### 8. `lint` said `ok` to a bag file — fixed

`lint vocabulary/constraint.txt` passed a 46-line bag with no slots. A template with no slot
renders the same line every draw, which is never what anyone meant.

**Rule:** an assertion verb flags what is valid but never intended.

**Done:** a slotless template is a finding that asks whether the file is a bag.

### 9. `koan bags` on an empty directory was a silent no-op — fixed

The header printed and the exit was 0. `lint` with no argument got this right.

**Rule:** empty is a result, but say so. Never a silent no-op.

**Done:** stdout keeps the faithful empty table; stderr says no bags were found and names the
command to try.

### 10. The used directory writes `.txt` for a `.csv` bag — no change

Documented in `schema`; the agent was not surprised only because it had read `schema` end to end
moments before. Recorded as a near-miss. The rule it points at is already in the plan: the used
directory is a pool subtraction, not a mirror of the bags.

### Questions no verb answered

- How many rounds of *n* remain? Computed by hand from `koan bags`. Not added; `lint -n` answers
  the binary form of the question.
- Does `USED` count matches or lines? Matches. Now stated in `bags --help`.
- Is `--seed 0` valid? Yes. Now stated in the flag help.

## What worked, so it is not removed by accident

- Bare `koan` was enough to run the main task first time: the two file kinds, ephemeral unless
  `--used`, seed to stderr, no repeats within a run, and which verb to run next.
- `recipes` in goal-then-command shape; the `--json | tee -a` idiom was used unmodified.
- The JSONL line is self-contained; the reproduction command came from its fields alone.
- The unknown-slot error names both files tried, identically from `lint` and from the draw.
- The exit table in `schema` meant no exit code was ever guessed at.

## The single change that helped most

Reporting the tightest bag in the exhaustion error, and giving `lint` an `-n`. Together they
remove four wasted runs, make the advertised guard true, and remove most of the need for a dry
run. The more dangerous finding was the third: it costs nothing today and returns the wrong koan
later, looking like success.

## Method

One agent, Opus tier, no access to the repository, source or documentation; the binary and the
bags copied to a scratch directory outside the repo. The agent's full report is in the `job`
notes on leaf `au45tT`. Reproduce the setup with `go build -o <scratch>/bin/koan ./cmd/koan`
and `cp -R examples/art-director <scratch>/work/vocabulary`.
