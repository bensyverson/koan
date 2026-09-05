# koan

A tool that picks random words from lists you wrote and drops them into a sentence, so an AI gets a prompt it would never have chosen for itself.

`koan` is a single Go binary. A **bag** is a file of values, one per line. A **template** is a line with `{slots}`. A **draw** pulls one value per slot from the bag of the same name, seeded, and renders the template. One seed governs every bag at once and is always printed, so any draw can be reproduced. A used directory subtracts past draws with no hidden state. It knows about lines, files, seeds and templates, and nothing else; anything that consumes a draw is a script in the repo that needs it.

## Install

```
go install github.com/bensyverson/koan/cmd/koan@latest
```

## Quick start

The repo ships a starter set in `examples/art-director/`: three `.csv` bags, one `.txt` bag and the template that binds them. Draw one:

```
$ koan examples/art-director/art-director.txt --seed 412
seed=412
Needs more tidal stillness. Channel your inner fennec fox. Constraint: Everything looks made by hand with a pen or a brush; only the captions are set in type
```

The seed goes to stderr, the draw to stdout. Leave `--seed` off and the clock picks one; feed the printed number back to get the same line again. Three for a round, as JSON Lines you can log:

```
$ koan examples/art-director/art-director.txt -n 3 --json | tee -a log.jsonl
```

Within a run no bag repeats a value, so three designers cannot share a constraint. To keep rounds from repeating across days, name a used directory. It is created on first use and is the only state the tool keeps:

```
$ koan examples/art-director/art-director.txt --used .koan-used
$ koan bags examples/art-director --used .koan-used
```

Write your own bag by dropping `muse.txt` beside the template, one value per line, and refer to it as `{muse}`. Bare `koan` prints the primer, `koan recipes` maps goals to commands, and `koan schema` carries the full grammar and the exit table.

## Two things to know

- **Commas in `.csv` bags must be quoted.** A `.csv` bag is parsed as CSV and the first column is the value, so an unquoted comma silently shortens the value. Put prose with commas in a `.txt` bag, where a line is a value verbatim.
- **Reproduce without `--used`.** Under `--used` the pool shrinks over time, so the same seed on a later day gives a different draw. Pass the same seed against the same bags and omit `--used`.

## More

The design, the contract and the history live in [project/](project/), starting with [the plan](project/2026-09-05-koan-vision-and-plan.md).

## Authorship and license

Ben Syverson. [MIT](LICENSE).
