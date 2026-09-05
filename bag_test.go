package koan

import (
	"encoding/csv"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	goodDir      = "testdata/bags"
	ambiguousDir = "testdata/bags-ambiguous"
	brokenDir    = "testdata/bags-broken"
	crlfDir      = "testdata/bags-crlf"
	multilineDir = "testdata/bags-multiline"
)

func mustLoad(t *testing.T, dir, name string) *Bag {
	t.Helper()
	bag, err := LoadBag(dir, name)
	if err != nil {
		t.Fatalf("LoadBag(%q, %q) = error %v, want a bag", dir, name, err)
	}
	return bag
}

// wantErr asserts that err is of type E and returns it.
func wantErr[E error](t *testing.T, err error) E {
	t.Helper()
	var target E
	if !errors.As(err, &target) {
		t.Fatalf("error is %#v (%v), want %T", err, err, target)
	}
	return target
}

func wantContains(t *testing.T, msg string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
}

func TestFormatString(t *testing.T) {
	if got := FormatTxt.String(); got != "txt" {
		t.Errorf("FormatTxt.String() = %q, want %q", got, "txt")
	}
	if got := FormatCSV.String(); got != "csv" {
		t.Errorf("FormatCSV.String() = %q, want %q", got, "csv")
	}
	if got := Format(0).String(); got == "" {
		t.Error("Format(0).String() is empty, want something printable")
	}
}

// TestLoadTxtVerbatim pins the .txt contract: nothing is parsed, so commas,
// quotes and apostrophes survive; comments and blanks go; duplicates stay.
func TestLoadTxtVerbatim(t *testing.T) {
	bag := mustLoad(t, goodDir, "adjective")

	want := []string{
		"chalky",
		"brittle",
		"chalky",
		`Don't say "no", say, "maybe"`,
		"a # in the middle of a line stays",
	}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
	if bag.Name != "adjective" {
		t.Errorf("name = %q, want %q", bag.Name, "adjective")
	}
	if want := filepath.Join(goodDir, "adjective.txt"); bag.Path != want {
		t.Errorf("path = %q, want %q", bag.Path, want)
	}
	if bag.Format != FormatTxt {
		t.Errorf("format = %v, want %v", bag.Format, FormatTxt)
	}
}

// TestLoadTxtCommasAreLiteral is the criterion in its own right: a .txt line
// holding commas is one value, not several.
func TestLoadTxtCommasAreLiteral(t *testing.T) {
	bag := mustLoad(t, goodDir, "constraint")

	want := []string{"Only one typeface, only two sizes", "No, really: no gradients"}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
}

// TestLoadCSVFirstColumn pins the .csv contract: the first column is the
// value, further columns belong to the author, rows may be ragged, a quoted
// comma survives, and a `#` header is a comment rather than a record.
func TestLoadCSVFirstColumn(t *testing.T) {
	bag := mustLoad(t, goodDir, "quality")

	want := []string{"candor", "rigour", "tuned, exactly", "candor"}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
	if bag.Format != FormatCSV {
		t.Errorf("format = %v, want %v", bag.Format, FormatCSV)
	}
	if want := filepath.Join(goodDir, "quality.csv"); bag.Path != want {
		t.Errorf("path = %q, want %q", bag.Path, want)
	}
	for _, v := range bag.Values {
		if strings.HasPrefix(v, "#") {
			t.Errorf("value %q came from a comment line", v)
		}
	}
}

func TestLoadCSVTrimsAndIgnoresExtraColumns(t *testing.T) {
	bag := mustLoad(t, goodDir, "muse")

	want := []string{"cartographer", "lighthouse keeper"}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
}

// TestUnquotedCommaTruncates documents the .csv trap the README warns about:
// an unquoted comma ends the value, and the remainder becomes an ignored
// second column. This is intended behaviour, not a bug to be fixed with
// LazyQuotes or a heuristic.
func TestUnquotedCommaTruncates(t *testing.T) {
	bag := mustLoad(t, brokenDir, "truncated")

	want := []string{"Only one typeface", "Quoted, and whole"}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
}

func TestLoadCRLF(t *testing.T) {
	txt := mustLoad(t, crlfDir, "adjective")
	wantTxt := []string{"chalky", "brittle", "Do not, ever"}
	if !slices.Equal(txt.Values, wantTxt) {
		t.Errorf(".txt values = %q, want %q", txt.Values, wantTxt)
	}
	for _, v := range txt.Values {
		if strings.ContainsAny(v, "\r\n") {
			t.Errorf(".txt value %q still carries a line ending", v)
		}
	}

	csvBag := mustLoad(t, crlfDir, "quality")
	wantCSV := []string{"candor", "tuned, exactly", "rigour"}
	if !slices.Equal(csvBag.Values, wantCSV) {
		t.Errorf(".csv values = %q, want %q", csvBag.Values, wantCSV)
	}
}

// TestCSVMultilineFieldSurvivesCommentStripping guards the seam between the
// line-oriented `#` rule and RFC 4180: a line inside a quoted field is part of
// the value even when it opens with a hash.
func TestCSVMultilineFieldSurvivesCommentStripping(t *testing.T) {
	bag := mustLoad(t, multilineDir, "verse")

	want := []string{
		"first line\n# not a comment, it is inside a quoted field\n\nstill the same value",
		"plain",
	}
	if !slices.Equal(bag.Values, want) {
		t.Errorf("values = %q, want %q", bag.Values, want)
	}
}

// TestResolutionIsExact covers what is actually observable about resolution:
// each format resolves on its own, and a name matches a basename exactly.
// The .txt-then-.csv order is not observable, because a name backed by both
// is an error rather than a precedence.
func TestResolutionIsExact(t *testing.T) {
	if bag := mustLoad(t, goodDir, "adjective"); bag.Format != FormatTxt {
		t.Errorf("adjective resolved to %v, want txt", bag.Format)
	}
	if bag := mustLoad(t, goodDir, "quality"); bag.Format != FormatCSV {
		t.Errorf("quality resolved to %v, want csv", bag.Format)
	}

	// Case-sensitive, and no plural or singular guessing, on every platform:
	// resolution matches directory entries exactly rather than stat-ing a
	// path, which a case-insensitive filesystem would answer for.
	for _, name := range []string{"Adjective", "ADJECTIVE", "adjectives", "adjectiv"} {
		if _, err := LoadBag(goodDir, name); err == nil {
			t.Errorf("LoadBag(%q, %q) succeeded, want a missing-bag error", goodDir, name)
		} else {
			wantErr[*MissingBagError](t, err)
		}
	}

	// A directory whose name ends in .txt is not a bag.
	if _, err := LoadBag(goodDir, "sub"); err == nil {
		t.Error("LoadBag resolved the directory sub.txt as a bag")
	} else {
		wantErr[*MissingBagError](t, err)
	}
}

func TestMissingBagError(t *testing.T) {
	_, err := LoadBag(goodDir, "nowhere")
	e := wantErr[*MissingBagError](t, err)

	if e.Name != "nowhere" {
		t.Errorf("Name = %q, want %q", e.Name, "nowhere")
	}
	if e.Dir != goodDir {
		t.Errorf("Dir = %q, want %q", e.Dir, goodDir)
	}
	wantTxt := filepath.Join(goodDir, "nowhere.txt")
	wantCSV := filepath.Join(goodDir, "nowhere.csv")
	if e.TxtPath != wantTxt || e.CSVPath != wantCSV {
		t.Errorf("paths = %q, %q; want %q, %q", e.TxtPath, e.CSVPath, wantTxt, wantCSV)
	}
	wantContains(t, e.Error(), "nowhere", wantTxt, wantCSV)
}

func TestMissingBagErrorOnAbsentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-bags-dir")
	_, err := LoadBag(dir, "adjective")
	e := wantErr[*MissingBagError](t, err)
	wantContains(t, e.Error(), filepath.Join(dir, "adjective.txt"))
}

func TestAmbiguousBagError(t *testing.T) {
	_, err := LoadBag(ambiguousDir, "adjective")
	e := wantErr[*AmbiguousBagError](t, err)

	wantTxt := filepath.Join(ambiguousDir, "adjective.txt")
	wantCSV := filepath.Join(ambiguousDir, "adjective.csv")
	if e.TxtPath != wantTxt || e.CSVPath != wantCSV {
		t.Errorf("paths = %q, %q; want %q, %q", e.TxtPath, e.CSVPath, wantTxt, wantCSV)
	}
	// Both paths, and the remedy, never a precedence.
	wantContains(t, e.Error(), wantTxt, wantCSV, "remove one of them")
}

func TestEmptyBagError(t *testing.T) {
	for _, name := range []string{"empty", "hollow", "void"} {
		_, err := LoadBag(brokenDir, name)
		if err == nil {
			t.Fatalf("LoadBag(%q, %q) succeeded, want an empty-bag error", brokenDir, name)
		}
		e := wantErr[*EmptyBagError](t, err)
		if !strings.Contains(e.Path, name) {
			t.Errorf("Path = %q, want it to name %q", e.Path, name)
		}
		wantContains(t, e.Error(), e.Path)
	}
}

func TestEmptyValueError(t *testing.T) {
	_, err := LoadBag(brokenDir, "blankcell")
	e := wantErr[*EmptyValueError](t, err)

	if want := filepath.Join(brokenDir, "blankcell.csv"); e.Path != want {
		t.Errorf("Path = %q, want %q", e.Path, want)
	}
	// Line 3 of the file: the `#` header on line 1 must not shift the count.
	if e.Line != 3 {
		t.Errorf("Line = %d, want 3", e.Line)
	}
	wantContains(t, e.Error(), e.Path, "line 3", "quote the value or delete the row")
}

func TestCSVError(t *testing.T) {
	_, err := LoadBag(brokenDir, "badquote")
	e := wantErr[*CSVError](t, err)

	if want := filepath.Join(brokenDir, "badquote.csv"); e.Path != want {
		t.Errorf("Path = %q, want %q", e.Path, want)
	}
	if e.Line != 3 {
		t.Errorf("Line = %d, want 3", e.Line)
	}
	if !errors.Is(err, csv.ErrBareQuote) {
		t.Errorf("error does not unwrap to csv.ErrBareQuote: %v", err)
	}
	if _, ok := errors.AsType[*csv.ParseError](err); !ok {
		t.Error("error does not unwrap to *csv.ParseError")
	}
	wantContains(t, e.Error(), e.Path, "line 3")
}

func TestReadErrorOnUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permissions do not deny a read")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "adjective.txt")
	if err := os.WriteFile(path, []byte("chalky\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("the filesystem does not enforce mode 000")
	}

	_, err := LoadBag(dir, "adjective")
	e := wantErr[*ReadError](t, err)
	if e.Path != path {
		t.Errorf("Path = %q, want %q", e.Path, path)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error does not unwrap to fs.ErrPermission: %v", err)
	}
	wantContains(t, e.Error(), path)
}

func TestReadErrorOnUnreadableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permissions do not deny a read")
	}
	dir := filepath.Join(t.TempDir(), "bags")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("the filesystem does not enforce mode 000")
	}

	_, err := LoadBag(dir, "adjective")
	e := wantErr[*ReadError](t, err)
	if e.Path != dir {
		t.Errorf("Path = %q, want the directory %q", e.Path, dir)
	}
}

func TestLoadBags(t *testing.T) {
	bags, err := LoadBags(goodDir, []string{"adjective", "quality"})
	if err != nil {
		t.Fatalf("LoadBags returned %v, want nil", err)
	}
	if len(bags) != 2 {
		t.Fatalf("loaded %d bags, want 2", len(bags))
	}
	if bags["adjective"].Format != FormatTxt || bags["quality"].Format != FormatCSV {
		t.Error("LoadBags lost a format")
	}
}

// TestLoadBagsKeepsGoingAfterAFailure is what lint needs: every problem at
// once, in names order, with the bags that did load still returned.
func TestLoadBagsKeepsGoingAfterAFailure(t *testing.T) {
	names := []string{"adjective", "nowhere", "quality", "alsonowhere"}
	bags, err := LoadBags(goodDir, names)
	if err == nil {
		t.Fatal("LoadBags succeeded, want joined errors")
	}
	if len(bags) != 2 || bags["adjective"] == nil || bags["quality"] == nil {
		t.Errorf("loaded %d bags, want adjective and quality", len(bags))
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error %T is not an errors.Join result", err)
	}
	parts := joined.Unwrap()
	if len(parts) != 2 {
		t.Fatalf("joined %d errors, want 2", len(parts))
	}
	if !strings.Contains(parts[0].Error(), "nowhere") || !strings.Contains(parts[1].Error(), "alsonowhere") {
		t.Errorf("errors are out of names order: %v", parts)
	}
}

func TestLoadBagsReportsEveryKindOfFailure(t *testing.T) {
	_, err := LoadBags(brokenDir, []string{"blankcell", "badquote", "empty"})
	if err == nil {
		t.Fatal("LoadBags succeeded, want joined errors")
	}
	wantErr[*EmptyValueError](t, err)
	wantErr[*CSVError](t, err)
	wantErr[*EmptyBagError](t, err)
}

func TestLoadBagsLoadsARepeatedNameOnce(t *testing.T) {
	bags, err := LoadBags(goodDir, []string{"adjective", "adjective"})
	if err != nil {
		t.Fatalf("LoadBags returned %v, want nil", err)
	}
	if len(bags) != 1 {
		t.Errorf("loaded %d bags, want 1", len(bags))
	}

	_, err = LoadBags(goodDir, []string{"nowhere", "nowhere"})
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error %T is not an errors.Join result", err)
	}
	if n := len(joined.Unwrap()); n != 1 {
		t.Errorf("joined %d errors for one repeated name, want 1", n)
	}
}

func TestListBags(t *testing.T) {
	listed, err := ListBags(goodDir)
	if err != nil {
		t.Fatalf("ListBags returned %v, want nil", err)
	}

	var names []string
	for _, l := range listed {
		if l.Err != nil {
			t.Errorf("%s: unexpected error %v", l.Name, l.Err)
		}
		if l.Bag == nil {
			t.Errorf("%s: no bag and no error", l.Name)
			continue
		}
		if l.Bag.Name != l.Name {
			t.Errorf("entry %q carries bag %q", l.Name, l.Bag.Name)
		}
		names = append(names, l.Name)
	}

	// Sorted by name, mixed formats, notes.md ignored, sub.txt/ not recursed.
	want := []string{"adjective", "constraint", "muse", "quality"}
	if !slices.Equal(names, want) {
		t.Errorf("names = %q, want %q", names, want)
	}
	if listed[0].Bag.Format != FormatTxt || listed[2].Bag.Format != FormatCSV {
		t.Error("ListBags lost a format")
	}
}

func TestListBagsCarriesAnAmbiguousPairAsOneEntry(t *testing.T) {
	listed, err := ListBags(ambiguousDir)
	if err != nil {
		t.Fatalf("ListBags returned %v, want nil", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d entries, want 2 (the pair counts once)", len(listed))
	}
	if listed[0].Name != "adjective" || listed[0].Bag != nil {
		t.Errorf("entry 0 = %+v, want the ambiguous adjective with no bag", listed[0])
	}
	wantErr[*AmbiguousBagError](t, listed[0].Err)
	if listed[1].Name != "zebra" || listed[1].Bag == nil || listed[1].Err != nil {
		t.Errorf("entry 1 = %+v, want zebra loaded", listed[1])
	}
}

func TestListBagsReportsEveryBrokenBag(t *testing.T) {
	listed, err := ListBags(brokenDir)
	if err != nil {
		t.Fatalf("ListBags returned %v, want nil", err)
	}
	byName := map[string]Listed{}
	for _, l := range listed {
		byName[l.Name] = l
	}
	for _, name := range []string{"empty", "hollow", "void", "blankcell", "badquote"} {
		l, ok := byName[name]
		if !ok {
			t.Errorf("%s is missing from the listing", name)
			continue
		}
		if l.Err == nil {
			t.Errorf("%s listed without an error", name)
		}
	}
	if l := byName["truncated"]; l.Err != nil || l.Bag == nil {
		t.Errorf("truncated = %+v, want it to load", l)
	}
}

func TestListBagsErrorsOnlyOnTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-dir")
	_, err := ListBags(dir)
	e := wantErr[*ReadError](t, err)
	if e.Path != dir {
		t.Errorf("Path = %q, want %q", e.Path, dir)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error does not unwrap to fs.ErrNotExist: %v", err)
	}
}

// TestEveryErrorNamesItsPath is the cli-design standard applied to the whole
// error set: an error names the thing by its path and says what happened.
func TestEveryErrorNamesItsPath(t *testing.T) {
	cases := []struct {
		err  error
		path string
	}{
		{&MissingBagError{Name: "a", Dir: "d", TxtPath: "d/a.txt", CSVPath: "d/a.csv"}, "d/a.txt"},
		{&AmbiguousBagError{Name: "a", TxtPath: "d/a.txt", CSVPath: "d/a.csv"}, "d/a.csv"},
		{&EmptyBagError{Path: "d/a.txt"}, "d/a.txt"},
		{&EmptyValueError{Path: "d/a.csv", Line: 7}, "d/a.csv"},
		{&CSVError{Path: "d/a.csv", Line: 7, Err: csv.ErrBareQuote}, "d/a.csv"},
		{&ReadError{Path: "d/a.csv", Err: fs.ErrPermission}, "d/a.csv"},
	}
	for _, c := range cases {
		msg := c.err.Error()
		if !strings.Contains(msg, c.path) {
			t.Errorf("%T message %q does not name %q", c.err, msg, c.path)
		}
		if strings.TrimSpace(msg) == "" {
			t.Errorf("%T has an empty message", c.err)
		}
	}
}
