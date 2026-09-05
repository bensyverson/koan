package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/bensyverson/koan"
	"github.com/spf13/cobra"
)

// noValue is what a column holds when there is nothing true to put in it: a
// broken bag has no counts, and a sound bag has no problem.
const noValue = "-"

// bagRow is one bag as `koan bags` reports it, in both output shapes, so the
// plain columns and the JSON keys can never drift apart.
type bagRow struct {
	// Name is the bag name, which is the slot it binds to.
	Name string `json:"name"`
	// Format is "txt" or "csv", empty when the bag did not load.
	Format string `json:"format"`
	// Path is the file backing the bag, empty when there is no single one.
	Path string `json:"path"`
	// Values is the distinct value count: how many draws the bag can supply.
	Values int `json:"values"`
	// Used is how many of those values the used directory excludes.
	Used int `json:"used"`
	// Remaining is Values minus Used.
	Remaining int `json:"remaining"`
	// Problem is the error that stopped the bag loading, empty when there is
	// none.
	Problem string `json:"problem"`
}

// bagsCommand builds `koan bags`: a read, so a broken bag is a row rather than
// a failure. Only a directory that cannot be read at all is an error.
func (a *app) bagsCommand() *cobra.Command {
	var usedDir string
	var jsonl bool
	cmd := &cobra.Command{
		Use:     "bags [dir | template]",
		Short:   "List the bags in a directory with their value, used and remaining counts",
		GroupID: groupRead,
		Long: "List every bag in a directory: its format, how many distinct values it holds,\n" +
			"how many of them a used directory has already spent, and what is left. Given a\n" +
			"template rather than a directory, it lists only the bags that template's slots\n" +
			"bind, in slot order, from the directory the template sits in — so a template\n" +
			"kept beside its own bags does not list itself. A bag that will not load is a\n" +
			"row with its problem, not a failure; run koan lint when you want a broken bag\n" +
			"to fail a script.\n\n" +
			"USED counts values of the bag that the used directory holds; a used value no\n" +
			"longer in the bag is ignored.",
		Example: "  # Every bag beside a template, before a run.\n" +
			"  koan bags examples/art-director\n\n" +
			"  # Only the bags that template binds, in slot order.\n" +
			"  koan bags examples/art-director/art-director.txt\n\n" +
			"  # What is left after a fortnight of rounds.\n" +
			"  koan bags examples/art-director --used .koan-used\n\n" +
			"  # The machine shape: one JSON object per bag.\n" +
			"  koan bags examples/art-director --json",
		Args: atMostOneTarget,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return a.listBags(target, usedDir, jsonl)
		},
	}
	cmd.Flags().StringVar(&usedDir, "used", "",
		"directory of already-drawn values to count against each bag")
	cmd.Flags().BoolVar(&jsonl, "json", false, "print JSON Lines, one object per bag")
	return cmd
}

// atMostOneTarget accepts a single optional directory or template.
func atMostOneTarget(_ *cobra.Command, args []string) error {
	if len(args) <= 1 {
		return nil
	}
	return usagef("pass one directory or template, or none for the working directory",
		"koan bags reads one directory or template, but %d were given", len(args))
}

// listBags builds and prints the rows for target: every bag in it when it is a
// directory, and the bags one template binds when it is a regular file.
func (a *app) listBags(target, usedDir string, jsonl bool) error {
	if info, err := os.Stat(target); err == nil && info.Mode().IsRegular() {
		return a.listTemplateBags(target, usedDir, jsonl)
	}

	a.ctx = remedyContext{BagsDir: target, UsedDir: usedDir}
	listed, err := koan.ListBags(target)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(listed))
	for _, entry := range listed {
		names = append(names, entry.Name)
	}
	used, err := a.loadUsed(usedDir, names)
	if err != nil {
		return err
	}

	rows := make([]bagRow, 0, len(listed))
	for _, entry := range listed {
		rows = append(rows, rowFor(entry, used[entry.Name]))
	}
	if err := a.writeBags(rows, jsonl); err != nil {
		return err
	}
	if len(listed) == 0 {
		a.reportNoBags(target)
	}
	return nil
}

// listTemplateBags prints one row per slot of the template at path, resolved
// against the directory the template sits in. A slot with no bag is a row
// carrying that problem rather than a failure, so the listing is exactly the
// bags a draw would need and nothing else in the directory.
func (a *app) listTemplateBags(path, usedDir string, jsonl bool) error {
	tmpl, err := readTemplate(path, a.stdin)
	if err != nil {
		return &cliError{
			code:   exitTarget,
			what:   fmt.Sprintf("cannot read the template %s: %s", path, detail(err)),
			remedy: fmt.Sprintf("check the path, then run koan lint %s", path),
		}
	}
	dir := filepath.Dir(path)
	a.ctx = remedyContext{BagsDir: dir, UsedDir: usedDir}

	slots := tmpl.Slots()
	bags, err := koan.LoadBags(dir, slots)
	problems := map[string]error{}
	if err != nil {
		// A directory that cannot be read at all is a failure, not a column:
		// every row would otherwise carry a problem it did not have.
		if dirErr, ok := errors.AsType[*koan.ReadError](err); ok && dirErr.Path == dir {
			return err
		}
		problems = problemsBySlot(slots, bags, err)
	}
	used, err := a.loadUsed(usedDir, slots)
	if err != nil {
		return err
	}

	rows := make([]bagRow, 0, len(slots))
	for _, slot := range slots {
		entry := koan.Listed{Name: slot, Bag: bags[slot], Err: problems[slot]}
		rows = append(rows, rowFor(entry, used[slot]))
	}
	if err := a.writeBags(rows, jsonl); err != nil {
		return err
	}
	if len(slots) == 0 {
		a.reportNoSlots(path)
	}
	return nil
}

// reportNoSlots says on stderr that a template bound no bags, for the same
// reason reportNoBags exists: an empty table is a faithful answer and a silent
// one reads as success. A template with no {slot} is usually a bag file handed
// over as a template, which is what lint says in as many words.
func (a *app) reportNoSlots(path string) {
	fmt.Fprintf(a.stderr, "%s%s has no {slot}, so it binds no bags\n", messagePrefix, path)
	fmt.Fprintf(a.stderr, "%srun koan lint %s; a template with no {slot} renders the same "+
		"line every draw\n", remedyPrefix, path)
}

// problemsBySlot pairs each slot that did not load with the error that stopped
// it. LoadBags reports one error per failing name, in the order the names were
// given, so the leaves and the slots missing from bags line up.
func problemsBySlot(slots []string, bags map[string]*koan.Bag, err error) map[string]error {
	leaves := flatten(err)
	problems := make(map[string]error, len(leaves))
	next := 0
	for _, slot := range slots {
		if _, ok := bags[slot]; ok || next >= len(leaves) {
			continue
		}
		problems[slot] = leaves[next]
		next++
	}
	return problems
}

// loadUsed is what the used directory has spent for names, and nothing when
// --used was not given.
func (a *app) loadUsed(usedDir string, names []string) (map[string]*koan.Bag, error) {
	if usedDir == "" {
		return map[string]*koan.Bag{}, nil
	}
	used, err := koan.LoadUsed(usedDir, names)
	if err != nil {
		// A used directory is read as a bags directory, so it is the one
		// `koan bags` should be pointed at to see what is wrong with it.
		a.ctx.BagsDir = usedDir
		return nil, err
	}
	return used, nil
}

// writeBags prints the rows in the shape the flags asked for.
func (a *app) writeBags(rows []bagRow, jsonl bool) error {
	if jsonl {
		return writeBagsJSON(a.stdout, rows)
	}
	return writeBagsPlain(a.stdout, rows)
}

// reportNoBags says on stderr that a directory held nothing to list. The
// header on stdout is a faithful empty table and the exit stays 0, so a pipe
// still parses; the notice is there because silence reads as success.
func (a *app) reportNoBags(dir string) {
	fmt.Fprintf(a.stderr, "%sno .txt or .csv bags in %s\n", messagePrefix, dir)
	fmt.Fprintf(a.stderr, "%spass a directory of bags, as in koan bags examples/art-director\n",
		remedyPrefix)
}

// rowFor turns one listed bag, and whatever the used directory holds for it,
// into a row.
func rowFor(entry koan.Listed, spent *koan.Bag) bagRow {
	row := bagRow{Name: entry.Name}
	if entry.Bag == nil {
		if entry.Err != nil {
			row.Problem = entry.Err.Error()
			row.Path = pathOf(entry.Err)
			row.Format = formatOf(row.Path)
		}
		return row
	}
	row.Format = entry.Bag.Format.String()
	row.Path = entry.Bag.Path
	values := distinct(entry.Bag.Values)
	row.Values = len(values)
	if spent != nil {
		for value := range distinct(spent.Values) {
			if values[value] {
				row.Used++
			}
		}
	}
	row.Remaining = row.Values - row.Used
	return row
}

// distinct is the set of values in a bag: how many draws it can supply, since
// drawing a value retires every copy of it.
func distinct(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// pathOf is the file a bag error names, empty for an error that names two
// files (an ambiguous pair) or none.
func pathOf(err error) string {
	if empty, ok := errors.AsType[*koan.EmptyBagError](err); ok {
		return empty.Path
	}
	if cell, ok := errors.AsType[*koan.EmptyValueError](err); ok {
		return cell.Path
	}
	if bad, ok := errors.AsType[*koan.CSVError](err); ok {
		return bad.Path
	}
	if unreadable, ok := errors.AsType[*koan.ReadError](err); ok {
		return unreadable.Path
	}
	return ""
}

// formatOf is the format a path is stored in, empty for anything else.
func formatOf(path string) string {
	switch filepath.Ext(path) {
	case ".txt":
		return koan.FormatTxt.String()
	case ".csv":
		return koan.FormatCSV.String()
	default:
		return ""
	}
}

// writeBagsPlain prints a header and one tab-aligned row per bag. The columns
// are laid out by text/tabwriter, so the bytes are the same in a pipe and in a
// terminal.
func writeBagsPlain(w io.Writer, rows []bagRow) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tFORMAT\tVALUES\tUSED\tREMAINING\tPROBLEM")
	for _, row := range rows {
		if row.Problem != "" {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
				row.Name, orDash(row.Format), noValue, noValue, noValue,
				strings.ReplaceAll(row.Problem, "\t", " "))
			continue
		}
		fmt.Fprintf(table, "%s\t%s\t%d\t%d\t%d\t%s\n",
			row.Name, row.Format, row.Values, row.Used, row.Remaining, noValue)
	}
	return table.Flush()
}

// writeBagsJSON prints one object per bag.
func writeBagsJSON(w io.Writer, rows []bagRow) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

// orDash is a column value, or the dash that stands for nothing true to say.
func orDash(value string) string {
	if value == "" {
		return noValue
	}
	return value
}
