package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenBags is pinned from the first green run of `koan bags bags` on the
// testdata fixtures; the columns are tab-aligned by text/tabwriter, so the
// bytes are the same in a pipe and in a terminal.
const goldenBags = `NAME        FORMAT  VALUES  USED  REMAINING  PROBLEM
adjective   txt     4       0     4          -
constraint  txt     2       0     2          -
muse        csv     2       0     2          -
quality     csv     3       0     3          -
`

func TestBagsPlainIsGolden(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "bags").wantCode(t, 0)
	r.wantStdout(t, goldenBags)
	if r.stderr != "" {
		t.Fatalf("koan bags wrote to stderr:\n%s", r.stderr)
	}
}

func TestBagsDefaultsToTheWorkingDirectory(t *testing.T) {
	dir := fixture(t)
	here := cli(t, filepath.Join(dir, "bags"), "bags").wantCode(t, 0)
	here.wantStdout(t, goldenBags)
}

func TestBagsJSON(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "bags", "--json").wantCode(t, 0)
	got := lines(r.stdout)
	if len(got) != 4 {
		t.Fatalf("koan bags --json printed %d lines, want 4", len(got))
	}
	var row struct {
		Name      string `json:"name"`
		Format    string `json:"format"`
		Path      string `json:"path"`
		Values    int    `json:"values"`
		Used      int    `json:"used"`
		Remaining int    `json:"remaining"`
		Problem   string `json:"problem"`
	}
	if err := json.Unmarshal([]byte(got[0]), &row); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("bags", "adjective.txt")
	if row.Name != "adjective" || row.Format != "txt" || row.Path != want ||
		row.Values != 4 || row.Used != 0 || row.Remaining != 4 || row.Problem != "" {
		t.Fatalf("first row = %+v", row)
	}
}

func TestBagsReportsProblemsAndStillExitsZero(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "bags-broken").wantCode(t, 0)
	rows := map[string][]string{}
	for _, line := range lines(r.stdout)[1:] {
		fields := strings.Fields(line)
		rows[fields[0]] = fields
	}
	for _, broken := range []string{"badquote", "blankcell", "empty", "hollow", "void"} {
		fields, ok := rows[broken]
		if !ok {
			t.Fatalf("no row for %s:\n%s", broken, r.stdout)
		}
		if len(fields) < 7 {
			t.Fatalf("broken bag %s reported no problem: %v", broken, fields)
		}
		if fields[2] != "-" || fields[3] != "-" || fields[4] != "-" {
			t.Fatalf("broken bag %s carries counts: %v", broken, fields[:5])
		}
	}
	// truncated.csv parses: an unquoted comma shortens a value, it does not
	// break the file, so the row is clean and carries counts.
	if got := rows["truncated"]; len(got) != 6 || got[5] != "-" {
		t.Fatalf("truncated.csv reported a problem: %v", got)
	}
}

func TestBagsCountsUsed(t *testing.T) {
	dir := fixture(t)
	used := filepath.Join(dir, "u")
	if err := os.MkdirAll(used, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(used, "adjective.txt"), []byte("chalky\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "bags", "bags", "--used", "u", "--json").wantCode(t, 0)
	var row struct {
		Name      string `json:"name"`
		Values    int    `json:"values"`
		Used      int    `json:"used"`
		Remaining int    `json:"remaining"`
	}
	if err := json.Unmarshal([]byte(lines(r.stdout)[0]), &row); err != nil {
		t.Fatal(err)
	}
	if row.Name != "adjective" || row.Values != 4 || row.Used != 1 || row.Remaining != 3 {
		t.Fatalf("adjective row = %+v, want 4 values, 1 used, 3 remaining", row)
	}
}

func TestBagsMissingUsedDirectoryCountsNothing(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "bags", "bags", "--used", "never-created").wantCode(t, 0).wantStdout(t, goldenBags)
	if _, err := os.Stat(filepath.Join(dir, "never-created")); !os.IsNotExist(err) {
		t.Fatalf("koan bags created the used directory")
	}
}

// TestBagsInAnEmptyDirectorySaysSoOnStderr pins that an empty listing is not
// silent. The header on stdout is a faithful empty table, so the exit stays 0
// and a pipe still parses; the notice goes to stderr, where a reader sees it.
func TestBagsInAnEmptyDirectorySaysSoOnStderr(t *testing.T) {
	r := cli(t, t.TempDir(), "bags").wantCode(t, 0)
	got := lines(r.stdout)
	if len(got) != 1 || !strings.HasPrefix(got[0], "NAME") {
		t.Fatalf("koan bags in an empty directory printed:\n%s", r.stdout)
	}
	r.wantStderrHas(t, "koan: no .txt or .csv bags in .")
	r.wantRemedy(t, "koan bags examples/art-director")
}

// bagsForATemplate is the template written beside the fixture bags, so that
// `koan bags <template>` is exercised in the layout that produced the defect:
// a template sitting in its own bags directory.
const bagsForATemplate = "Needs more {adjective} {quality}. " +
	"Channel your inner {muse}. Constraint: {constraint}\n"

// goldenTemplateBags is the listing for bagsForATemplate: one row per slot, in
// slot order rather than alphabetical, and no row for the template itself.
const goldenTemplateBags = `NAME        FORMAT  VALUES  USED  REMAINING  PROBLEM
adjective   txt     4       0     4          -
quality     csv     3       0     3          -
muse        csv     2       0     2          -
constraint  txt     2       0     2          -
`

func TestBagsForATemplateListsOnlyItsSlots(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "bags", "order.txt")
	if err := os.WriteFile(path, []byte(bagsForATemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "bags", filepath.Join("bags", "order.txt")).wantCode(t, 0)
	r.wantStdout(t, goldenTemplateBags)
	if strings.Contains(r.stdout, "order") {
		t.Fatalf("the template listed itself as a bag:\n%s", r.stdout)
	}
}

// TestBagsForATemplateFlagsAnUnboundSlot pins that a slot with no bag is a row
// carrying the problem rather than a failure, and that each problem lands on
// its own slot: the bound and unbound slots interleave, so a listing that
// paired them by position would put the finding on the wrong row.
func TestBagsForATemplateFlagsAnUnboundSlot(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "bags", "gap.txt")
	source := "A {adjective} {nonesuch} {quality} {alsomissing}.\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "bags", filepath.Join("bags", "gap.txt")).wantCode(t, 0)
	got := lines(r.stdout)
	if len(got) != 5 {
		t.Fatalf("a four-slot template printed %d lines, want a header and four rows:\n%s",
			len(got), r.stdout)
	}
	for i, want := range []string{"adjective", "nonesuch", "quality", "alsomissing"} {
		if !strings.HasPrefix(got[i+1], want) {
			t.Fatalf("row %d is %q, want the row for %q", i, got[i+1], want)
		}
	}
	if !strings.Contains(got[2], filepath.Join("bags", "nonesuch.txt")) {
		t.Fatalf("the nonesuch row carries the wrong problem: %q", got[2])
	}
	if !strings.Contains(got[4], filepath.Join("bags", "alsomissing.txt")) {
		t.Fatalf("the alsomissing row carries the wrong problem: %q", got[4])
	}
	if strings.Contains(got[1], "nonesuch") || strings.Contains(got[3], "alsomissing") {
		t.Fatalf("a problem landed on a bound slot's row:\n%s", r.stdout)
	}
}

// TestBagsForASlotlessTemplateSaysSoOnStderr pins that the empty listing a
// template with no {slot} produces is not silent either: the bag file handed
// over as a template is exactly the mistake the listing exists to catch.
func TestBagsForASlotlessTemplateSaysSoOnStderr(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "cli/slotless.txt").wantCode(t, 0)
	got := lines(r.stdout)
	if len(got) != 1 || !strings.HasPrefix(got[0], "NAME") {
		t.Fatalf("koan bags on a slotless template printed:\n%s", r.stdout)
	}
	r.wantStderrHas(t, "koan: cli/slotless.txt has no {slot}")
	r.wantRemedy(t, "koan lint cli/slotless.txt")
}

func TestBagsForATemplateCountsUsed(t *testing.T) {
	dir := fixture(t)
	path := filepath.Join(dir, "bags", "solo.txt")
	if err := os.WriteFile(path, []byte("A {adjective} thing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	used := filepath.Join(dir, "u")
	if err := os.MkdirAll(used, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(used, "adjective.txt"), []byte("chalky\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "bags", filepath.Join("bags", "solo.txt"), "--used", "u", "--json").
		wantCode(t, 0)
	var row struct {
		Name      string `json:"name"`
		Values    int    `json:"values"`
		Used      int    `json:"used"`
		Remaining int    `json:"remaining"`
	}
	if err := json.Unmarshal([]byte(lines(r.stdout)[0]), &row); err != nil {
		t.Fatal(err)
	}
	if row.Name != "adjective" || row.Values != 4 || row.Used != 1 || row.Remaining != 3 {
		t.Fatalf("adjective row = %+v, want 4 values, 1 used, 3 remaining", row)
	}
}

func TestBagsForAnUnreadableFileIsFour(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode-000 file, so there is nothing to refuse")
	}
	dir := fixture(t)
	path := filepath.Join(dir, "bags", "locked.txt")
	if err := os.WriteFile(path, []byte("A {adjective} thing.\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "bags", filepath.Join("bags", "locked.txt")).wantCode(t, 4)
	// A regular file is read as a template, so the failure says so and sends
	// the caller to lint rather than telling them to create a directory.
	r.wantStderrHas(t, "cannot read the template", "locked.txt")
	r.wantRemedy(t, "koan lint", "locked.txt")
}

func TestBagsBrokenUsedDirectoryIsFour(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "bags", "--used", "bags-ambiguous").wantCode(t, 4)
	r.wantStderrHas(t, "ambiguous")
	r.wantRemedy(t, "remove one")
}

func TestBagsMissingDirectoryIsFour(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "bags", "nowhere").wantCode(t, 4)
	r.wantStderrHas(t, "nowhere")
	r.wantRemedy(t, "nowhere")
}

func TestLintCleanIsZero(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "lint", "cli/koan.txt", "--bags", "bags").
		wantCode(t, 0).
		wantStdout(t, "cli/koan.txt: ok\n")
}

func TestLintMultiLineTemplateIsNotAFinding(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "cli/multi.txt", "--bags", "bags").
		wantCode(t, 0).
		wantStdout(t, "cli/multi.txt: ok\n")
	if strings.Contains(r.stdout, "--json") {
		t.Fatalf("lint mentioned --json:\n%s", r.stdout)
	}
}

func TestLintUnboundSlotIsOne(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "cli/unbound.txt", "--bags", "bags").wantCode(t, 1)
	r.wantStdoutHas(t,
		"cli/unbound.txt: ",
		"nonesuch",
		filepath.Join("bags", "nonesuch.txt"),
		filepath.Join("bags", "nonesuch.csv"),
	)
}

func TestLintReportsEveryFindingForEveryTemplate(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "cli/koan.txt", "cli/unbound.txt", "cli/broken.txt",
		"--bags", "bags").wantCode(t, 1)
	got := lines(r.stdout)
	if len(got) < 3 {
		t.Fatalf("lint printed %d lines for three templates:\n%s", len(got), r.stdout)
	}
	if got[0] != "cli/koan.txt: ok" {
		t.Fatalf("the clean template did not report ok: %q", got[0])
	}
	for _, want := range []string{"cli/unbound.txt: ", "cli/broken.txt: "} {
		found := false
		for _, line := range got {
			if strings.HasPrefix(line, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no finding for %q:\n%s", want, r.stdout)
		}
	}
	// broken.txt names three bags and every one of them is missing from bags/.
	findings := 0
	for _, line := range got {
		if strings.HasPrefix(line, "cli/broken.txt: ") {
			findings++
		}
	}
	if findings != 3 {
		t.Fatalf("cli/broken.txt produced %d findings, want 3:\n%s", findings, r.stdout)
	}
}

func TestLintMissingTemplateIsAFinding(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "nope.txt").wantCode(t, 1)
	r.wantStdoutHas(t, "nope.txt: ")
}

func TestLintUsedExhaustion(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "6", "--used", "u").wantCode(t, 0)
	r := cli(t, dir, "lint", "cli/round/round.txt", "--used", "u").wantCode(t, 1)
	r.wantStdoutHas(t, "cli/round/round.txt: ", "animal", "0 distinct values")
}

func TestLintBagsFlagAppliesToEveryTemplate(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "lint", "cli/koan.txt", "cli/repeat.txt", "--bags", "bags").
		wantCode(t, 0).
		wantStdout(t, "cli/koan.txt: ok\ncli/repeat.txt: ok\n")
}

func TestLintWithoutBagsUsesEachTemplateDirectory(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "lint", "cli/round/round.txt").
		wantCode(t, 0).
		wantStdout(t, "cli/round/round.txt: ok\n")
}

func TestLintWithNoTemplateIsUsage(t *testing.T) {
	cli(t, t.TempDir(), "lint").wantCode(t, 2).wantRemedy(t, "koan lint")
}

// TestLintCountChecksTheRoundYouWillDraw pins the guard the lint --help
// advertises: without -n, lint only sees a bag with nothing left, so a round of
// three against a bag with two left passed lint and then failed the draw.
func TestLintCountChecksTheRoundYouWillDraw(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "4", "--used", "u").wantCode(t, 0)

	tight := cli(t, dir, "lint", "cli/round/round.txt", "--used", "u", "-n", "3").wantCode(t, 1)
	tight.wantStdoutHas(t, "cli/round/round.txt: ", "animal", "2 distinct values", "3 draws")

	cli(t, dir, "lint", "cli/round/round.txt", "--used", "u", "-n", "2").
		wantCode(t, 0).
		wantStdout(t, "cli/round/round.txt: ok\n")
}

func TestLintCountBelowOneIsUsage(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "cli/round/round.txt", "-n", "0").wantCode(t, 2)
	r.wantStderrHas(t, "-n")
	r.wantRemedy(t, "-n")
}

// TestLintSlotlessTemplateIsAFinding pins that a template with no {slot} is
// reported: it renders the same line every draw, which is never what a caller
// meant, and it is usually a bag file passed as a template.
func TestLintSlotlessTemplateIsAFinding(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "lint", "cli/slotless.txt").wantCode(t, 1)
	r.wantStdout(t, "cli/slotless.txt: has no {slot}: every draw would render the same line; "+
		"is this a bag rather than a template?\n")
}

func TestRecipesNeedsNothing(t *testing.T) {
	r := cli(t, t.TempDir(), "recipes").wantCode(t, 0)
	r.wantStdoutHas(t,
		"--seed",
		"--used",
		"--json",
		"koan lint",
		"koan bags",
		"koan -",
		".csv",
		".txt",
		"-n 3",
		// Reproducing a draw is stated against the pool it was drawn from,
		// and a draw past the first is reached by extending the run.
		"against the same used state, if any",
		"sed -n '3p'",
	)
}

func TestSchemaNeedsNothing(t *testing.T) {
	r := cli(t, t.TempDir(), "schema").wantCode(t, 0)
	r.wantStdoutHas(t,
		"{slot}",
		"[A-Za-z0-9_.-]+",
		".txt",
		".csv",
		"seed=",
		"exit",
		`{"seed":`,
		"--used",
		// The three facts an agent got wrong reading the old page: that a
		// seed reproduces only against the same pool, that i is an index
		// within one run, and that a bag's order is part of what a seed means.
		"only against the same pool",
		"-n <i+1>",
		"file order",
	)
}

func TestEveryVerbHelpCarriesAnExample(t *testing.T) {
	for _, verb := range []string{"bags", "lint", "recipes", "schema"} {
		r := cli(t, t.TempDir(), verb, "--help").wantCode(t, 0)
		r.wantStdoutHas(t, "Examples:", "koan "+verb)
	}
}

func TestUnknownVerbIsTreatedAsATemplate(t *testing.T) {
	// koan has one act, so a bare word is a template path, not a verb: a
	// template file named bags is addressed as ./bags.
	dir := fixture(t)
	r := cli(t, dir, "no-such-template").wantCode(t, 4)
	r.wantStderrHas(t, "no-such-template")
}
