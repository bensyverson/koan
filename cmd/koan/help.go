package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// primer is the root command's Long text, so bare `koan` and `koan --help`
// are the same document: what the tool is for, the shape of the two files it
// reads, and the one command to run first.
const primer = `koan draws seeded random values from hand-curated bags of lines and renders them
into a template. It exists to push a language model out of the centre of its
distribution by handing it something it would never have chosen. It knows about
lines, files, seeds and templates, and nothing else.

Two kinds of file, both hand-written:

  a bag       vocabulary/adjective.txt — one value per line, verbatim
  a template  a line of text with {slot} names that bind to bags by basename,
              as in "Needs more {adjective} {quality}."

Drawing renders the template once per draw, taking one value per slot. Within a
run a bag never repeats a value, so three draws cannot share a constraint. The
seed goes to stderr as seed=N, so a piped draw stays clean and a log always has
the number that reproduces it.

koan is ephemeral by default: it writes nothing unless you pass --used, which
names the directory that both excludes what has been drawn and records what
this run draws.

The verbs, by role:

  act   koan <template> [--bags dir] [--used dir] [--seed n] [-n k] [--json]
  read  koan bags, koan lint — they inspect, they never draw and never write
  help  koan recipes, koan schema — they need no files and no setup

Run this first:

  koan schema     the template grammar, the bag file rules, and the exit table

then koan recipes for a goal and the command that reaches it.`

// rootExample is the root command's worked example. It also settles the one
// ambiguity in the grammar: a template named like a verb.
const rootExample = `  # Draw one koan. --bags defaults to the template's own directory.
  koan art-director.txt

  # Reproduce a draw from a log: same seed, same bags, no --used.
  koan art-director.txt --seed 412

  # Three designers in one round, no shared values, as a machine shape.
  koan art-director.txt -n 3 --json

  # A template file named bags is addressed as ./bags, so it is not the verb.
  koan ./bags`

// recipesText routes a goal to the command that reaches it. It is the second
// step of the progressive help: the primer, then a verb's --help, then this.
const recipesText = `Goal                                      Command
────                                      ───────

Draw one koan                             koan art-director.txt

Draw three for a round, no repeats        koan art-director.txt -n 3
  (one seeded stream; -n 5 extends -n 3 rather than replacing it)

Reproduce a draw from a log               koan art-director.txt --seed 412
  (same seed, same bags, and omit --used: the used pool shrinks over time)

Keep rounds from repeating across days    koan art-director.txt --used .koan-used
  (excludes what is in .koan-used, then appends this run's draws to it;
   the directory is created the first time it is named)

Check a bags directory before a run       koan bags vocabulary
  (name, format, values, used, remaining, and a problem flag per bag)

See what is left after a fortnight        koan bags vocabulary --used .koan-used

Check every template in a directory       koan lint patterns/*.txt --bags vocabulary
  (exit 1 on a finding, so: koan lint p.txt && koan p.txt)

Log the draws                             koan p.txt --json | tee -a log.jsonl
  (one self-contained JSON object per draw; seed=N goes to stderr, not the log)

Pipe a template in                        echo 'A {adjective} thing.' | koan - --bags vocabulary
  (- reads the template from standard input; --bags then defaults to ".")

Write a new bag                           put prose in vocabulary/muse.txt
  (.txt is verbatim: commas, quotes and apostrophes are literal. Use .csv only
   when you want further columns for comments or tags, and then a value holding
   a comma must be wrapped in double quotes. Never both files for one name.)`

// schemaText is every grammar koan imports, in one page: the template, the two
// bag formats, the JSON Lines shape, the stderr line, the used directory and
// the exit table.
const schemaText = `TEMPLATE
  A slot is {slot}: a brace, a name matching [A-Za-z0-9_.-]+, a brace.
  Anything else is literal text. There is no escape syntax, so {} and {a b}
  are literal and {{a}} is a brace, the slot a, and a brace.
  The same slot twice in one template repeats the same drawn value.
  Slots draw in order of first appearance.
  Trailing newlines are stripped, so a one-line file that ends in a newline is
  a single-line template.
  Plain output needs a single-line template; a multi-line one needs --json.

BAGS
  A slot binds to <name>.txt or <name>.csv in the bags directory, matched
  exactly and case-sensitively. No plural or singular guessing.
  Both files present for one name is an error, never a precedence.
  .txt  one line, one value, verbatim: commas, quotes and apostrophes literal.
  .csv  RFC 4180; the first column is the value, further columns are yours.
        A value holding a comma or a quote must be wrapped in double quotes,
        with each inner quote doubled; an unquoted comma silently shortens the
        value at the comma.
  Both formats: a line whose first non-blank character is # is a comment (a
  .csv header goes on one), blank lines are skipped, values are trimmed.
  A duplicated line is a weight: it is drawn more often, but drawing it retires
  every copy, so it can be drawn only once in a run.
  An empty bag, an empty first cell and a parse error are hard errors naming
  the file and, where there is one, the line.

OUTPUT
  Plain: one rendered line per draw on stdout.
  --json: JSON Lines, one self-contained object per draw, in order:
  {"seed":412,"i":0,"template":"art-director.txt","slots":{"adjective":"chalky"},"text":"Needs more chalky."}
    seed      the seed the run drew under
    i         the 0-based position of the draw in the run
    template  the template path as typed, or "-" for standard input
    slots     the value drawn for each slot
    text      the rendered template
  stderr always carries one line, seed=N, printed once just before the draws,
  so --json piping stays clean and a failed run leaves only its error there.

USED DIRECTORY
  --used <dir> is read as a bags directory and is the only state koan keeps.
  Values found in <dir>/<bag>.txt or <dir>/<bag>.csv are removed from the pool
  before drawing; after a fully successful run every drawn value is appended
  there, to whichever file already backs the bag, and to <bag>.txt for a bag
  with no used file yet. The directory is created when it is first named.
  A failed run records nothing. A used directory is a pool subtraction, not a
  history: the history is the JSONL you tee into a log.
  Reproducing a draw means passing the same seed against the same bags and
  omitting --used, because the used pool shrinks over time.

EXIT CODES
  The status koan exits with, from the table every tool in the house shares.
  koan lint exits 1 on a finding, so a script branches with && and ||.
  0  drew; lint found nothing; a help verb ran
  1  lint found something
  2  malformed invocation: a bad or unknown flag, -n under 1, more than one
     template, a multi-line template without --json
  3  a bag ran out: --used or -n exhausted it; the message names the counts
  4  a target could not be loaded: a missing, ambiguous, empty or unparseable
     bag, an unreadable template, an unreadable used directory
  5  environment: the used directory could not be created or written`

// recipesCommand builds `koan recipes`.
func recipesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "recipes",
		Short:   "A goal, and the command that reaches it",
		GroupID: groupHelp,
		Long: "Routing from what you want to the invocation that does it. Needs no files and\n" +
			"no setup; see koan schema for the grammars these commands speak.",
		Example: "  # The whole table; it is short enough to read.\n" +
			"  koan recipes\n\n" +
			"  # Just the line about the used directory.\n" +
			"  koan recipes | grep -- --used",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), recipesText)
			return err
		},
	}
}

// schemaCommand builds `koan schema`.
func schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "The template grammar, the bag file rules, the JSON shape and the exit table",
		GroupID: groupHelp,
		Long: "Every grammar koan reads or writes, in one page. Needs no files and no setup;\n" +
			"see koan recipes for which command to reach for.",
		Example: "  # The whole page.\n" +
			"  koan schema\n\n" +
			"  # Just the exit table, to script against.\n" +
			"  koan schema | sed -n '/^EXIT CODES/,$p'",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), schemaText)
			return err
		},
	}
}
