package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		Use:     "bags [dir]",
		Short:   "List the bags in a directory with their value, used and remaining counts",
		GroupID: groupRead,
		Long: "List every bag in a directory: its format, how many distinct values it holds,\n" +
			"how many of them a used directory has already spent, and what is left. A bag\n" +
			"that will not load is a row with its problem, not a failure; run koan lint when\n" +
			"you want a broken bag to fail a script.",
		Example: "  # Every bag beside a template, before a run.\n" +
			"  koan bags examples/art-director\n\n" +
			"  # What is left after a fortnight of rounds.\n" +
			"  koan bags examples/art-director --used .koan-used\n\n" +
			"  # The machine shape: one JSON object per bag.\n" +
			"  koan bags examples/art-director --json",
		Args: atMostOneDir,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return a.listBags(dir, usedDir, jsonl)
		},
	}
	cmd.Flags().StringVar(&usedDir, "used", "",
		"directory of already-drawn values to count against each bag")
	cmd.Flags().BoolVar(&jsonl, "json", false, "print JSON Lines, one object per bag")
	return cmd
}

// atMostOneDir accepts a single optional directory.
func atMostOneDir(_ *cobra.Command, args []string) error {
	if len(args) <= 1 {
		return nil
	}
	return usagef("pass one directory, or none for the working directory",
		"koan bags reads one directory, but %d were given", len(args))
}

// listBags builds and prints the rows for dir.
func (a *app) listBags(dir, usedDir string, jsonl bool) error {
	a.ctx = remedyContext{BagsDir: dir, UsedDir: usedDir}
	listed, err := koan.ListBags(dir)
	if err != nil {
		return err
	}

	used := map[string]*koan.Bag{}
	if usedDir != "" {
		names := make([]string, 0, len(listed))
		for _, entry := range listed {
			names = append(names, entry.Name)
		}
		if used, err = koan.LoadUsed(usedDir, names); err != nil {
			// A used directory is read as a bags directory, so it is the one
			// `koan bags` should be pointed at to see what is wrong with it.
			a.ctx.BagsDir = usedDir
			return err
		}
	}

	rows := make([]bagRow, 0, len(listed))
	for _, entry := range listed {
		rows = append(rows, rowFor(entry, used[entry.Name]))
	}
	if jsonl {
		return writeBagsJSON(a.stdout, rows)
	}
	return writeBagsPlain(a.stdout, rows)
}

// rowFor turns one listed bag, and whatever the used directory holds for it,
// into a row.
func rowFor(entry koan.Listed, spent *koan.Bag) bagRow {
	row := bagRow{Name: entry.Name}
	if entry.Err != nil {
		row.Problem = entry.Err.Error()
		row.Path = pathOf(entry.Err)
		row.Format = formatOf(row.Path)
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
