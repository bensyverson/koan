package koan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeUsed puts a used file on disk verbatim, so a test can pin what an
// existing or hand-edited file does to the append.
func writeUsed(t *testing.T, dir, file, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func readUsed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// mustLoadUsed is LoadUsed with the error fatal.
func mustLoadUsed(t *testing.T, dir string, names []string) map[string]*Bag {
	t.Helper()
	used, err := LoadUsed(dir, names)
	if err != nil {
		t.Fatalf("LoadUsed(%q, %q) = error %v, want bags", dir, names, err)
	}
	return used
}

// usedValues is the values LoadUsed found for a name, empty if none.
func usedValues(used map[string]*Bag, name string) []string {
	if bag, ok := used[name]; ok {
		return bag.Values
	}
	return nil
}

// TestLoadUsedMissingDir pins that a used directory that does not exist yet
// means nothing used, not an error: the directory is created on first record.
func TestLoadUsedMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	used, err := LoadUsed(dir, []string{"adjective"})
	if err != nil {
		t.Fatalf("LoadUsed on a missing dir = error %v, want nothing used", err)
	}
	if used == nil {
		t.Error("used map is nil, want an empty map")
	}
	if len(used) != 0 {
		t.Errorf("used = %v, want empty", used)
	}
}

// TestLoadUsedPerName pins that a name with no file is simply not used, while
// its neighbour with a file is.
func TestLoadUsedPerName(t *testing.T) {
	dir := t.TempDir()
	writeUsed(t, dir, "adjective.txt", "chalky\nbrittle\n")

	used := mustLoadUsed(t, dir, []string{"adjective", "muse"})
	if got := usedValues(used, "adjective"); !slices.Equal(got, []string{"chalky", "brittle"}) {
		t.Errorf("adjective used = %q, want chalky and brittle", got)
	}
	if _, ok := used["muse"]; ok {
		t.Errorf("muse is in the used map (%v), want absent", used["muse"])
	}
}

// TestLoadUsedEmptyFile pins that an empty used file means nothing used rather
// than the empty-bag error the loader raises for a real bag.
func TestLoadUsedEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeUsed(t, dir, "adjective.txt", "# nothing used yet\n\n")

	used := mustLoadUsed(t, dir, []string{"adjective"})
	if len(used) != 0 {
		t.Errorf("used = %v, want empty", used)
	}
}

// TestLoadUsedAmbiguous pins that the loader's real errors still propagate: a
// used directory holding both files for one name is as broken as a bags one.
func TestLoadUsedAmbiguous(t *testing.T) {
	dir := t.TempDir()
	writeUsed(t, dir, "adjective.txt", "chalky\n")
	writeUsed(t, dir, "adjective.csv", "brittle\n")

	_, err := LoadUsed(dir, []string{"adjective"})
	e := wantErr[*AmbiguousBagError](t, err)
	if e.Name != "adjective" {
		t.Errorf("Name = %q, want %q", e.Name, "adjective")
	}
}

// TestLoadUsedCSV pins that a .csv used file is read as a bag, so a value
// holding a comma comes back whole.
func TestLoadUsedCSV(t *testing.T) {
	dir := t.TempDir()
	writeUsed(t, dir, "constraint.csv", "# used\n\"Only one typeface, only two sizes\"\nNo photographs\n")

	used := mustLoadUsed(t, dir, []string{"constraint"})
	want := []string{"Only one typeface, only two sizes", "No photographs"}
	if got := usedValues(used, "constraint"); !slices.Equal(got, want) {
		t.Errorf("constraint used = %q, want %q", got, want)
	}
}

// TestExcludeRemovesEveryCopy pins exclusion by value: a used value goes
// however many times the bag repeats it, and the inputs survive.
func TestExcludeRemovesEveryCopy(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "chalky", "brittle", "molten")
	used := bagsOf(inlineBag("adjective", "chalky"))
	bagBefore := slices.Clone(bag.Values)
	usedBefore := slices.Clone(used["adjective"].Values)

	got := Exclude(bagsOf(bag), used)

	if !slices.Equal(got["adjective"].Values, []string{"brittle", "molten"}) {
		t.Errorf("values = %q, want brittle and molten", got["adjective"].Values)
	}
	if !slices.Equal(bag.Values, bagBefore) {
		t.Errorf("input bag = %q, want %q untouched", bag.Values, bagBefore)
	}
	if !slices.Equal(used["adjective"].Values, usedBefore) {
		t.Errorf("input used = %q, want %q untouched", used["adjective"].Values, usedBefore)
	}
}

// TestExcludeKeepsBagIdentity pins that the copies still know which file they
// came from, so an exhaustion error after exclusion names a real path.
func TestExcludeKeepsBagIdentity(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "brittle")
	got := Exclude(bagsOf(bag), bagsOf(inlineBag("adjective", "chalky")))["adjective"]

	if got.Name != bag.Name || got.Path != bag.Path || got.Format != bag.Format {
		t.Errorf("copy = %+v, want name/path/format of %+v", got, bag)
	}
}

// TestExcludeIgnoresUnknownValues pins that a used value the bag never held is
// not an error and changes nothing.
func TestExcludeIgnoresUnknownValues(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "brittle")
	got := Exclude(bagsOf(bag), bagsOf(inlineBag("adjective", "sopping")))

	if !slices.Equal(got["adjective"].Values, bag.Values) {
		t.Errorf("values = %q, want %q", got["adjective"].Values, bag.Values)
	}
}

// TestExcludeCopiesUnusedBag pins that a bag with no used entry comes through
// unchanged, and as a copy: editing the result cannot reach the input.
func TestExcludeCopiesUnusedBag(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "brittle")
	got := Exclude(bagsOf(bag), map[string]*Bag{})["adjective"]

	if !slices.Equal(got.Values, bag.Values) {
		t.Errorf("values = %q, want %q", got.Values, bag.Values)
	}
	got.Values[0] = "scribbled"
	if bag.Values[0] != "chalky" {
		t.Error("the copy shares its values with the input bag, want a fresh slice")
	}
}

// TestExcludeCanEmptyABag pins that exclusion is allowed to leave nothing: the
// shortfall is Draw's to report, so the pool is not special-cased here.
func TestExcludeCanEmptyABag(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "brittle")
	got := Exclude(bagsOf(bag), bagsOf(inlineBag("adjective", "chalky", "brittle")))

	if len(got["adjective"].Values) != 0 {
		t.Errorf("values = %q, want none left", got["adjective"].Values)
	}
	_, err := Draw(Request{
		Template: ParseTemplate("{adjective}"),
		Name:     "-",
		Bags:     got,
		Seed:     1,
		Count:    1,
	})
	if e := wantErr[*ExhaustedError](t, err); e.Available != 0 {
		t.Errorf("Available = %d, want 0", e.Available)
	}
}

// recordFixture draws n koans from two small bags into a fresh used directory
// path that does not exist yet.
func recordFixture(t *testing.T, seed uint64, n int) (dir string, koans []Koan) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "used")
	koans = mustDraw(t, Request{
		Template: ParseTemplate("{adjective} {muse}"),
		Name:     "art-director.txt",
		Bags: bagsOf(
			inlineBag("adjective", "chalky", "brittle", "molten", "damp", "loud", "thin"),
			inlineBag("muse", "cartographer", "welder", "archivist", "diver", "printer", "smith"),
		),
		Seed:  seed,
		Count: n,
	})
	return dir, koans
}

// TestRecordCreatesDirectoryAndFiles pins that the used directory is state the
// tool creates when first named, one .txt per bag.
func TestRecordCreatesDirectoryAndFiles(t *testing.T) {
	dir, koans := recordFixture(t, 412, 3)

	if err := Record(dir, koans); err != nil {
		t.Fatalf("Record: %v", err)
	}
	for _, name := range []string{"adjective", "muse"} {
		path := filepath.Join(dir, name+".txt")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("stat %s: %v", path, err)
		}
	}
}

// TestRecordAppendsInKoanOrder pins that each drawn value lands once, in the
// order it was drawn, so the file reads as the run did.
func TestRecordAppendsInKoanOrder(t *testing.T) {
	dir, koans := recordFixture(t, 412, 3)
	if err := Record(dir, koans); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var want []string
	for _, koan := range koans {
		want = append(want, koan.Slots["adjective"])
	}
	got := strings.Split(strings.TrimSuffix(readUsed(t, filepath.Join(dir, "adjective.txt")), "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Errorf("adjective.txt = %q, want %q in koan order", got, want)
	}
}

// TestRecordAppendsAcrossRuns pins that a second run adds to the file rather
// than replacing it: the used directory accumulates.
func TestRecordAppendsAcrossRuns(t *testing.T) {
	dir, koans := recordFixture(t, 412, 2)
	if err := Record(dir, koans[:1]); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if err := Record(dir, koans[1:]); err != nil {
		t.Fatalf("second Record: %v", err)
	}

	want := koans[0].Slots["adjective"] + "\n" + koans[1].Slots["adjective"] + "\n"
	if got := readUsed(t, filepath.Join(dir, "adjective.txt")); got != want {
		t.Errorf("adjective.txt = %q, want %q", got, want)
	}
}

// TestRecordAppendsToExistingCSV pins that a bag already recorded as .csv stays
// .csv — manufacturing the ambiguous pair would break the next run — and that
// a value holding a comma is quoted on the way in.
func TestRecordAppendsToExistingCSV(t *testing.T) {
	dir := t.TempDir()
	writeUsed(t, dir, "constraint.csv", "No photographs\n")
	koan := Koan{
		Seed:     1,
		Template: "-",
		Slots:    map[string]string{"constraint": "Only one typeface, only two sizes"},
		Text:     "Only one typeface, only two sizes",
	}

	if err := Record(dir, []Koan{koan}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "constraint.txt")); err == nil {
		t.Error("Record created constraint.txt beside constraint.csv, want the .csv appended to")
	}
	want := "No photographs\n\"Only one typeface, only two sizes\"\n"
	if got := readUsed(t, filepath.Join(dir, "constraint.csv")); got != want {
		t.Errorf("constraint.csv = %q, want %q", got, want)
	}

	used := mustLoadUsed(t, dir, []string{"constraint"})
	wantValues := []string{"No photographs", "Only one typeface, only two sizes"}
	if got := usedValues(used, "constraint"); !slices.Equal(got, wantValues) {
		t.Errorf("reloaded = %q, want %q", got, wantValues)
	}
}

// TestRecordClosesAnUnterminatedFile pins the guard against a hand-edited file
// with no trailing newline: appending must not glue two values together.
func TestRecordClosesAnUnterminatedFile(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"adjective.txt", "chalky\nbrittle\n"},
		{"adjective.csv", "chalky\nbrittle\n"},
	} {
		dir := t.TempDir()
		writeUsed(t, dir, tc.file, "chalky")
		koan := Koan{Slots: map[string]string{"adjective": "brittle"}}

		if err := Record(dir, []Koan{koan}); err != nil {
			t.Fatalf("Record into %s: %v", tc.file, err)
		}
		if got := readUsed(t, filepath.Join(dir, tc.file)); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.file, got, tc.want)
		}
	}
}

// TestRecordUnwritableDirectory pins the environment failure the CLI maps to
// exit 5, naming the path that could not be made.
func TestRecordUnwritableDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "used")
	if err := os.WriteFile(dir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
	koan := Koan{Slots: map[string]string{"adjective": "chalky"}}

	err := Record(dir, []Koan{koan})
	e := wantErr[*RecordError](t, err)
	if e.Path != dir {
		t.Errorf("Path = %q, want %q", e.Path, dir)
	}
	if e.Unwrap() == nil {
		t.Error("Unwrap() is nil, want the filesystem error")
	}
	wantContains(t, e.Error(), dir)
}

// TestRecordOfNothingWritesNothing pins the other half of "a failed run writes
// nothing": Draw hands back no koans when it fails, and no koans means the used
// directory is not even created.
func TestRecordOfNothingWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "used")

	koans, err := Draw(Request{
		Template: ParseTemplate("{adjective}"),
		Name:     "-",
		Bags:     bagsOf(inlineBag("adjective", "chalky", "brittle")),
		Seed:     1,
		Count:    3,
	})
	wantErr[*ExhaustedError](t, err)
	if koans != nil {
		t.Fatalf("koans = %+v, want none", koans)
	}
	if err := Record(dir, koans); err != nil {
		t.Fatalf("Record of nothing: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want the directory not to exist", dir, err)
	}
}

// TestUsedRoundTrip walks the whole --used loop: draw, record, reload, exclude,
// draw again under the same seed, and pin that nothing recurs.
func TestUsedRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "used")
	names := []string{"adjective", "muse"}
	bags := func() map[string]*Bag {
		return bagsOf(
			inlineBag("adjective", "chalky", "brittle", "molten", "damp", "loud", "thin"),
			inlineBag("muse", "cartographer", "welder", "archivist", "diver", "printer", "smith"),
		)
	}
	request := func(available map[string]*Bag) Request {
		return Request{
			Template: ParseTemplate("{adjective} {muse}"),
			Name:     "art-director.txt",
			Bags:     available,
			Seed:     412,
			Count:    3,
		}
	}

	first := mustDraw(t, request(bags()))
	if err := Record(dir, first); err != nil {
		t.Fatalf("Record: %v", err)
	}

	used := mustLoadUsed(t, dir, names)
	for _, name := range names {
		var want []string
		for _, koan := range first {
			want = append(want, koan.Slots[name])
		}
		if got := usedValues(used, name); !slices.Equal(got, want) {
			t.Errorf("%s used = %q, want the first run's %q", name, got, want)
		}
	}

	second := mustDraw(t, request(Exclude(bags(), used)))
	for _, name := range names {
		for _, before := range first {
			for _, after := range second {
				if after.Slots[name] == before.Slots[name] {
					t.Errorf("%s value %q recurred after being recorded as used",
						name, after.Slots[name])
				}
			}
		}
	}
}
