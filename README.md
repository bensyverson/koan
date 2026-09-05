# koan

A tool that picks random words from lists you wrote and drops them into a sentence, so an AI gets a prompt it would never have chosen for itself.

`koan` is a single Go binary. A **bag** is a file of values, one per line. A **template** is a line with `{slots}`. A **draw** pulls one value per slot from the bag of the same name, seeded, and renders the template. One seed governs every bag at once and is always printed, so any draw can be reproduced. A used directory subtracts past draws with no hidden state.

## Install

```
go install github.com/bensyverson/koan/cmd/koan@latest
```

## Quick start

_To be written when the examples land._

## Two things to know

- **Commas in `.csv` bags must be quoted.** A `.csv` bag is parsed as CSV and the first column is the value, so an unquoted comma silently shortens the value. Put prose with commas in a `.txt` bag, where a line is a value verbatim.
- **Reproduce without `--used`.** Under `--used` the pool shrinks over time, so the same seed on a later day gives a different draw. Pass the same seed against the same bags and omit `--used`.

## More

The design, the contract and the history live in [project/](project/), starting with [the plan](project/2026-09-05-koan-vision-and-plan.md).

## Authorship and license

Ben Syverson. [MIT](LICENSE).
