package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wantRemedy fails unless stderr carries a remedy line holding every fragment.
func (r result) wantRemedy(t *testing.T, fragments ...string) result {
	t.Helper()
	var remedy string
	for _, line := range lines(r.stderr) {
		if after, ok := strings.CutPrefix(line, remedyPrefix); ok {
			remedy = after
		}
	}
	if remedy == "" {
		t.Fatalf("stderr carries no %q line:\n%s", remedyPrefix, r.stderr)
	}
	for _, fragment := range fragments {
		if !strings.Contains(remedy, fragment) {
			t.Fatalf("remedy %q does not mention %q", remedy, fragment)
		}
	}
	return r
}

// wantNoSeedLine fails if a failed run leaked the seed onto stderr.
func (r result) wantNoSeedLine(t *testing.T) result {
	t.Helper()
	if strings.Contains(r.stderr, "seed=") {
		t.Fatalf("a failed run printed the seed:\n%s", r.stderr)
	}
	return r
}

func TestUsageUnknownFlag(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/koan.txt", "--nope").
		wantCode(t, 2).
		wantStderrHas(t, "koan: ").
		wantRemedy(t, "koan --help")
}

func TestUsageBadSeed(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", "not-a-number").
		wantCode(t, 2).
		wantRemedy(t, "koan --help")
	cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", "-1").
		wantCode(t, 2).
		wantRemedy(t, "koan --help")
}

func TestUsageCountBelowOne(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/koan.txt", "--bags", "bags", "-n", "0").
		wantCode(t, 2).
		wantNoSeedLine(t)
	r.wantStderrHas(t, "-n")
	r.wantRemedy(t, "-n")
}

func TestUsageMoreThanOneTemplate(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/koan.txt", "cli/repeat.txt", "--bags", "bags").
		wantCode(t, 2).
		wantStderrHas(t, "koan: ").
		wantRemedy(t, "koan lint")
}

func TestUsageMultiLineTemplateWithoutJSON(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/multi.txt", "--bags", "bags", "--seed", "412").
		wantCode(t, 2).
		wantNoSeedLine(t)
	r.wantStderrHas(t, "cli/multi.txt", "3 lines")
	r.wantRemedy(t, "--json")
}

func TestUsageNoTemplateButFlags(t *testing.T) {
	cli(t, t.TempDir(), "--seed", "412").
		wantCode(t, 2).
		wantRemedy(t, "koan --help")
}

func TestTargetTemplateUnreadable(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "nope.txt").wantCode(t, 4).wantNoSeedLine(t)
	r.wantStderrHas(t, "nope.txt")
	r.wantRemedy(t, "nope.txt")
}

func TestTargetMissingBag(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/unbound.txt", "--bags", "bags").wantCode(t, 4)
	r.wantStderrHas(t, "nonesuch")
	r.wantRemedy(t,
		filepath.Join("bags", "nonesuch.txt"),
		filepath.Join("bags", "nonesuch.csv"),
		"koan bags bags")
}

func TestTargetAmbiguousBag(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/solo.txt", "--bags", "bags-ambiguous").wantCode(t, 4)
	r.wantStderrHas(t, "ambiguous")
	r.wantRemedy(t, "remove one",
		filepath.Join("bags-ambiguous", "adjective.txt"),
		filepath.Join("bags-ambiguous", "adjective.csv"))
}

func TestTargetEmptyBag(t *testing.T) {
	dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, "cli", "one-empty.txt"), []byte("{empty}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "cli/one-empty.txt", "--bags", "bags-broken").wantCode(t, 4)
	r.wantStderrHas(t, filepath.Join("bags-broken", "empty.txt"))
	r.wantRemedy(t, "koan bags bags-broken")
}

func TestTargetEmptyCell(t *testing.T) {
	dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, "cli", "one-cell.txt"), []byte("{blankcell}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "cli/one-cell.txt", "--bags", "bags-broken").wantCode(t, 4)
	r.wantStderrHas(t, "first column is empty")
	r.wantRemedy(t, "koan bags bags-broken")
}

func TestTargetCSVParseError(t *testing.T) {
	dir := fixture(t)
	if err := os.WriteFile(filepath.Join(dir, "cli", "one-quote.txt"), []byte("{badquote}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "cli/one-quote.txt", "--bags", "bags-broken").wantCode(t, 4)
	r.wantStderrHas(t, "cannot parse the row")
	r.wantRemedy(t, "koan bags bags-broken")
}

func TestTargetUnreadableBagsDirectory(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/koan.txt", "--bags", filepath.Join("bags", "adjective.txt")).wantCode(t, 4)
	r.wantStderrHas(t, "cannot read")
}

func TestJoinedBagErrorsPrintEveryMessage(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/broken.txt", "--bags", "bags-broken").wantCode(t, 4)
	r.wantStderrHas(t,
		filepath.Join("bags-broken", "empty.txt"),
		filepath.Join("bags-broken", "badquote.csv"),
		"nosuch",
	)
	problems := 0
	for _, line := range lines(r.stderr) {
		if strings.HasPrefix(line, "koan: ") {
			problems++
		}
	}
	if problems != 3 {
		t.Fatalf("a joined error printed %d koan: lines, want 3\nstderr:\n%s", problems, r.stderr)
	}
}

func TestTargetUsedDirectoryError(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/repeat.txt", "--bags", "bags", "--used", "bags-ambiguous").wantCode(t, 4)
	r.wantStderrHas(t, "ambiguous")
}

func TestExhaustedBag(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", "412", "-n", "3").
		wantCode(t, 3).
		wantNoSeedLine(t)
	r.wantStderrHas(t, "muse", "2", "3")
	r.wantRemedy(t, "-n 2", filepath.Join("bags", "muse.csv"))
}

func TestExhaustedUnderUsedNamesTheUsedFile(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "6", "--used", "u").wantCode(t, 0)
	r := cli(t, dir, "cli/round/round.txt", "--seed", "7", "--used", "u").wantCode(t, 3)
	r.wantStderrHas(t, "0 distinct values")
	r.wantRemedy(t, filepath.Join("u", "animal.txt"))
}

func TestRecordFailureIsFive(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes to a read-only directory, so there is nothing to refuse")
	}
	dir := fixture(t)
	// The used directory reads fine and cannot be written: LoadUsed succeeds,
	// the draw succeeds and is printed, and only Record fails.
	readOnly := filepath.Join(dir, "locked")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "cli/round/round.txt", "--seed", "7", "--used", "locked").wantCode(t, 5)
	if !strings.HasPrefix(r.stdout, "The ") {
		t.Fatalf("the draws were not printed before the record failed: %q", r.stdout)
	}
	r.wantStderrHas(t, "locked")
	r.wantRemedy(t, "--used")
}

func TestUnreadableUsedDirectoryIsFour(t *testing.T) {
	dir := fixture(t)
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := cli(t, dir, "cli/round/round.txt", "--seed", "7", "--used", filepath.Join("not-a-dir", "u")).
		wantCode(t, 4)
	r.wantStderrHas(t, "not-a-dir")
	if r.stdout != "" {
		t.Fatalf("a run that could not read --used still drew: %q", r.stdout)
	}
}

func TestUsedExcludesAndRecords(t *testing.T) {
	dir := fixture(t)
	first := cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "2", "--used", "u").wantCode(t, 0)
	animals, err := os.ReadFile(filepath.Join(dir, "u", "animal.txt"))
	if err != nil {
		t.Fatalf("the used directory was not created: %v", err)
	}
	if len(lines(string(animals))) != 2 {
		t.Fatalf("u/animal.txt holds %q, want two values", animals)
	}
	second := cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "2", "--used", "u").wantCode(t, 0)
	for _, line := range lines(first.stdout) {
		if strings.Contains(second.stdout, line) {
			t.Fatalf("the second run repeated %q despite --used:\n%s", line, second.stdout)
		}
	}
}

func TestFailedRunWritesNothingToUsed(t *testing.T) {
	dir := fixture(t)
	cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "7", "--used", "u").wantCode(t, 3)
	if _, err := os.Stat(filepath.Join(dir, "u")); !os.IsNotExist(err) {
		t.Fatalf("a failed run created the used directory (err=%v)", err)
	}
}
