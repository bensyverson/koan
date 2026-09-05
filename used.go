package koan

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// LoadUsed reads dir as a bags directory and returns the used values for
// names, keyed by name. Resolution is the bag loader's, so <name>.csv is
// honoured and names match exactly and case-sensitively.
//
// A used directory that does not exist, a name with no file in it, and a file
// holding no values all mean nothing used, and leave that name out of the
// result. Every other bag failure — an ambiguous .txt/.csv pair, a CSV parse
// error, an empty first cell, an unreadable file — propagates as it would from
// [LoadBags], joined so one call reports them all.
func LoadUsed(dir string, names []string) (map[string]*Bag, error) {
	used := make(map[string]*Bag, len(names))
	files, err := bagFiles(dir)
	if err != nil {
		return used, err
	}

	var errs []error
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		bag, err := loadResolved(dir, name, files)
		switch {
		case err == nil:
			used[name] = bag
		case isNothingUsed(err):
		default:
			errs = append(errs, err)
		}
	}
	return used, errors.Join(errs...)
}

// isNothingUsed reports whether err is a bag failure that, in a used
// directory, only means the bag has nothing used yet.
func isNothingUsed(err error) bool {
	var missing *MissingBagError
	var empty *EmptyBagError
	return errors.As(err, &missing) || errors.As(err, &empty)
}

// Exclude returns fresh bags with every value present in used removed — every
// copy, by exact value, because a duplicated line is a weight rather than a
// second chance. Neither argument is mutated, and a name absent from used is
// copied unchanged.
//
// An excluded bag may end up with no values at all. That is not an error here:
// [Draw] reports it as exhaustion with Available 0, which is where the counts
// and the remedy belong.
func Exclude(bags, used map[string]*Bag) map[string]*Bag {
	out := make(map[string]*Bag, len(bags))
	for name, bag := range bags {
		values := slices.Clone(bag.Values)
		if spent, ok := used[name]; ok {
			gone := make(map[string]bool, len(spent.Values))
			for _, value := range spent.Values {
				gone[value] = true
			}
			values = slices.DeleteFunc(values, func(v string) bool { return gone[v] })
		}
		out[name] = &Bag{Name: bag.Name, Path: bag.Path, Format: bag.Format, Values: values}
	}
	return out
}

// Record appends each koan's drawn values to the used directory, creating it
// if it is not there yet. A bag already recorded as <name>.csv is appended to
// as CSV, so a comma or a quote is escaped and the next run still resolves one
// file for the name; every other bag gets <name>.txt.
//
// Values are written in koan order, so a file reads as the run did. Where a
// file does not already end in a newline — a hand edit, say — a newline is
// written first, so an append can never glue two values together.
//
// Record is called only after every draw succeeded, which is what makes a
// failed run write nothing; the caller guarantees that. Recording nothing
// writes nothing and does not create the directory. Any failure to create or
// write is a *[RecordError] naming the directory or file that failed, which
// the CLI maps to exit 5.
func Record(dir string, koans []Koan) error {
	if len(koans) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &RecordError{Path: dir, Err: err}
	}
	files, err := bagFiles(dir)
	if err != nil {
		return &RecordError{Path: dir, Err: err}
	}

	open := make(map[string]*usedFile)
	defer func() {
		for _, w := range open {
			w.close()
		}
	}()

	for _, koan := range koans {
		for _, name := range sortedSlotNames(koan.Slots) {
			w, ok := open[name]
			if !ok {
				if w, err = openUsedFile(dir, name, files); err != nil {
					return err
				}
				open[name] = w
			}
			if err := w.append(koan.Slots[name]); err != nil {
				return err
			}
		}
	}
	for _, w := range open {
		if err := w.close(); err != nil {
			return err
		}
	}
	return nil
}

// sortedSlotNames is a koan's slot names in sorted order, so a run touches its
// files in the same sequence every time.
func sortedSlotNames(slots map[string]string) []string {
	names := make([]string, 0, len(slots))
	for name := range slots {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// usedFile is one open used file, appending in its own format.
type usedFile struct {
	path string
	file *os.File
	csv  *csv.Writer
}

// openUsedFile opens the file that already backs name, preferring .txt when
// both exist. That pair is a state the loader refuses, so a run never reaches
// here with one; picking .txt rather than re-checking keeps the ambiguity rule
// in one place.
func openUsedFile(dir, name string, files map[string]bool) (*usedFile, error) {
	format := FormatTxt
	if !files[name+FormatTxt.ext()] && files[name+FormatCSV.ext()] {
		format = FormatCSV
	}
	path := filepath.Join(dir, name+format.ext())

	unterminated, err := lacksFinalNewline(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, &RecordError{Path: path, Err: err}
	}
	w := &usedFile{path: path, file: file}
	if format == FormatCSV {
		w.csv = csv.NewWriter(file)
	}
	if unterminated {
		if _, err := file.WriteString("\n"); err != nil {
			file.Close()
			return nil, &RecordError{Path: path, Err: err}
		}
	}
	return w, nil
}

// lacksFinalNewline reports whether path exists, holds something, and does not
// end in a newline. It reads the last byte rather than the file, since a used
// file grows without bound.
func lacksFinalNewline(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, &RecordError{Path: path, Err: err}
	}
	if info.Size() == 0 {
		return false, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return false, &RecordError{Path: path, Err: err}
	}
	defer file.Close()

	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return false, &RecordError{Path: path, Err: err}
	}
	return last[0] != '\n', nil
}

// append writes one value as a whole line, or as a one-field CSV record so
// that a comma or a quote is escaped.
func (w *usedFile) append(value string) error {
	if w.csv != nil {
		if err := w.csv.Write([]string{value}); err != nil {
			return &RecordError{Path: w.path, Err: err}
		}
		return nil
	}
	if _, err := w.file.WriteString(value + "\n"); err != nil {
		return &RecordError{Path: w.path, Err: err}
	}
	return nil
}

// close flushes and closes, reporting whichever failed. Calling it twice is
// harmless, so the deferred cleanup after an early return costs nothing.
func (w *usedFile) close() error {
	if w.file == nil {
		return nil
	}
	file := w.file
	w.file = nil
	if w.csv != nil {
		w.csv.Flush()
		if err := w.csv.Error(); err != nil {
			file.Close()
			return &RecordError{Path: w.path, Err: err}
		}
	}
	if err := file.Close(); err != nil {
		return &RecordError{Path: w.path, Err: err}
	}
	return nil
}

// RecordError reports a used directory or file that could not be created or
// written. The CLI maps it to exit 5.
type RecordError struct {
	// Path is the directory or file that failed.
	Path string
	// Err is the underlying filesystem error.
	Err error
}

// Error implements error.
func (e *RecordError) Error() string {
	// An *fs.PathError restates the path, which this message already names.
	detail := e.Err.Error()
	var pathErr *fs.PathError
	if errors.As(e.Err, &pathErr) && pathErr.Err != nil {
		detail = pathErr.Err.Error()
	}
	return fmt.Sprintf("cannot record used values to %s: %s", e.Path, detail)
}

// Unwrap returns the underlying filesystem error.
func (e *RecordError) Unwrap() error { return e.Err }
