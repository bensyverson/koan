package koan

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// commentPrefix marks a comment: a line whose first non-blank character is
// this is skipped in both formats, and is how a .csv carries a header.
const commentPrefix = "#"

// Format is a bag file's format, decided by its extension.
type Format int

const (
	// FormatTxt is a .txt bag: one line, one verbatim value.
	FormatTxt Format = iota + 1
	// FormatCSV is a .csv bag: RFC 4180 rows whose first column is the value.
	FormatCSV
)

// ext returns the file extension the format is stored under, dot included.
func (f Format) ext() string {
	switch f {
	case FormatTxt:
		return ".txt"
	case FormatCSV:
		return ".csv"
	default:
		return ""
	}
}

// String returns the format's extension without the dot: "txt" or "csv".
func (f Format) String() string {
	if e := f.ext(); e != "" {
		return e[1:]
	}
	return "Format(" + strconv.Itoa(int(f)) + ")"
}

// Bag is one loaded bag: its values in file order, duplicates kept, because a
// duplicated line is a weight.
type Bag struct {
	// Name is the slot name: the file's basename without its extension.
	Name string
	// Path is the file the values were loaded from.
	Path string
	// Format is the format Path was read as.
	Format Format
	// Values are the bag's values, trimmed, in file order.
	Values []string
}

// LoadBag resolves name in dir and loads it. Resolution looks for <name>.txt
// and <name>.csv, matching directory entries exactly and case-sensitively;
// both present is an *AmbiguousBagError, neither a *MissingBagError.
func LoadBag(dir, name string) (*Bag, error) {
	files, err := bagFiles(dir)
	if err != nil {
		return nil, err
	}
	return loadResolved(dir, name, files)
}

// LoadBags loads every name. It keeps going after a failure and returns every
// error joined with errors.Join, so lint can report all problems at once; the
// map holds every bag that loaded. Errors are in names order. A name repeated
// in names is loaded, and reported, once.
func LoadBags(dir string, names []string) (map[string]*Bag, error) {
	bags := make(map[string]*Bag, len(names))
	files, err := bagFiles(dir)
	if err != nil {
		return bags, err
	}
	var errs []error
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		bag, err := loadResolved(dir, name, files)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		bags[name] = bag
	}
	return bags, errors.Join(errs...)
}

// Listed is one entry from ListBags: a bag that loaded, or the error that
// stopped it. Exactly one of Bag and Err is set.
type Listed struct {
	// Name is the bag name.
	Name string
	// Bag is the loaded bag, nil if Err is set.
	Bag *Bag
	// Err is what stopped the bag loading, nil if Bag is set.
	Err error
}

// ListBags discovers every *.txt and *.csv in dir (non-recursive), sorted by
// name, loading each; an ambiguous pair is one entry carrying the error. It
// errors only if dir itself cannot be read.
func ListBags(dir string) ([]Listed, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, &ReadError{Path: dir, Err: err}
	}
	files := make(map[string]bool, len(entries))
	var names []string
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files[entry.Name()] = true
		ext := filepath.Ext(entry.Name())
		if ext != FormatTxt.ext() && ext != FormatCSV.ext() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ext)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	slices.Sort(names)

	listed := make([]Listed, 0, len(names))
	for _, name := range names {
		bag, err := loadResolved(dir, name, files)
		listed = append(listed, Listed{Name: name, Bag: bag, Err: err})
	}
	return listed, nil
}

// bagFiles is the set of names of the regular files in dir. Resolution reads
// the directory rather than stat-ing a path so that names match exactly and
// case-sensitively even on a case-insensitive filesystem. A directory that
// does not exist yields an empty set, so every name in it reports as missing.
func bagFiles(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, &ReadError{Path: dir, Err: err}
	}
	files := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files[entry.Name()] = true
	}
	return files, nil
}

// loadResolved picks the one file backing name among the files in dir.
func loadResolved(dir, name string, files map[string]bool) (*Bag, error) {
	txtPath := filepath.Join(dir, name+FormatTxt.ext())
	csvPath := filepath.Join(dir, name+FormatCSV.ext())
	hasTxt := files[name+FormatTxt.ext()]
	hasCSV := files[name+FormatCSV.ext()]

	switch {
	case hasTxt && hasCSV:
		return nil, &AmbiguousBagError{Name: name, TxtPath: txtPath, CSVPath: csvPath}
	case hasTxt:
		return loadFile(name, txtPath, FormatTxt)
	case hasCSV:
		return loadFile(name, csvPath, FormatCSV)
	default:
		return nil, &MissingBagError{Name: name, Dir: dir, TxtPath: txtPath, CSVPath: csvPath}
	}
}

// loadFile reads and parses one bag file.
func loadFile(name, path string, format Format) (*Bag, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ReadError{Path: path, Err: err}
	}
	var values []string
	if format == FormatCSV {
		if values, err = parseCSV(path, data); err != nil {
			return nil, err
		}
	} else {
		values = parseTxt(data)
	}
	if len(values) == 0 {
		return nil, &EmptyBagError{Path: path}
	}
	return &Bag{Name: name, Path: path, Format: format, Values: values}, nil
}

// parseTxt takes each line verbatim. Nothing is parsed, so commas, quotes and
// apostrophes are literal; comments and blank lines go, and CRLF endings fall
// out of the trim.
func parseTxt(data []byte) []string {
	var values []string
	for line := range strings.SplitSeq(string(data), "\n") {
		value := strings.TrimSpace(line)
		if value == "" || strings.HasPrefix(value, commentPrefix) {
			continue
		}
		values = append(values, value)
	}
	return values
}

// parseCSV reads RFC 4180 rows and keeps the first column of each. Ragged rows
// are allowed and further columns belong to the author. LazyQuotes stays off,
// so an unquoted comma truncates the value at the comma rather than producing
// something subtly different — a trap the README warns about, pinned by a test.
func parseCSV(path string, data []byte) ([]string, error) {
	reader := csv.NewReader(bytes.NewReader(maskComments(data)))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = false

	var values []string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &CSVError{Path: path, Line: csvErrorLine(err), Err: err}
		}
		if len(record) == 0 {
			continue
		}
		line, _ := reader.FieldPos(0)
		value := strings.TrimSpace(record[0])
		if value == "" {
			return nil, &EmptyValueError{Path: path, Line: line}
		}
		values = append(values, value)
	}
	return values, nil
}

// csvErrorLine is the 1-based line a csv error stopped on, 0 if unknown.
func csvErrorLine(err error) int {
	if parse, ok := errors.AsType[*csv.ParseError](err); ok {
		return parse.Line
	}
	return 0
}

// maskComments blanks every comment and whitespace-only line that is not
// inside a quoted field, keeping the newline so encoding/csv skips the line
// and every remaining record reports its true line number. The quote tracking
// is what keeps a `#` inside a multi-line quoted value part of that value.
func maskComments(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))
	inQuotes := false
	for line := range strings.SplitAfterSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		if !inQuotes {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, commentPrefix) {
				out.WriteByte('\n')
				continue
			}
		}
		out.WriteString(line)
		if strings.Count(line, `"`)%2 == 1 {
			inQuotes = !inQuotes
		}
	}
	return out.Bytes()
}
