package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binary is the koan built once by TestMain; every test drives it as a
// subprocess, because the exit code and the stdout/stderr split are the
// contract under test.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "koan-cli-build")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot make a build directory:", err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "koan")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot build koan:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// result is one run of the binary.
type result struct {
	stdout string
	stderr string
	code   int
}

// cli executes the binary in dir with no stdin.
func cli(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return cliInput(t, dir, "", args...)
}

// cliInput executes the binary in dir with stdin bound to input.
func cliInput(t *testing.T, dir, input string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exit.ExitCode()
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// fixture copies the whole testdata tree into a fresh temp directory and
// returns it, so a run that writes (--used) never touches the repository.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata"))); err != nil {
		t.Fatalf("copying testdata: %v", err)
	}
	return dir
}

// wantCode fails unless the run exited with code.
func (r result) wantCode(t *testing.T, code int) result {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.stdout, r.stderr)
	}
	return r
}

// wantStdout fails unless stdout is exactly want.
func (r result) wantStdout(t *testing.T, want string) result {
	t.Helper()
	if r.stdout != want {
		t.Fatalf("stdout =\n%q\nwant\n%q", r.stdout, want)
	}
	return r
}

// wantStderrHas fails unless every fragment appears on stderr.
func (r result) wantStderrHas(t *testing.T, fragments ...string) result {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(r.stderr, fragment) {
			t.Fatalf("stderr does not contain %q\nstderr:\n%s", fragment, r.stderr)
		}
	}
	return r
}

// wantStdoutHas fails unless every fragment appears on stdout.
func (r result) wantStdoutHas(t *testing.T, fragments ...string) result {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(r.stdout, fragment) {
			t.Fatalf("stdout does not contain %q\nstdout:\n%s", fragment, r.stdout)
		}
	}
	return r
}

// lines splits output into lines, dropping the trailing empty one.
func lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// The golden draws below are pinned from the first green run of the built
// binary under seed 412; the library's stream is deterministic, so any change
// to them is a change to the contract.
const (
	goldenText = "Needs more a # in the middle of a line stays tuned, exactly. " +
		"Channel your inner lighthouse keeper. Constraint: No, really: no gradients"
	goldenPlain = goldenText + "\n"
	goldenJSON  = `{"seed":412,"i":0,"template":"cli/koan.txt",` +
		`"slots":{"adjective":"a # in the middle of a line stays",` +
		`"constraint":"No, really: no gradients","muse":"lighthouse keeper",` +
		`"quality":"tuned, exactly"},` +
		`"text":"` + goldenText + `"}` + "\n"
)

func TestBarePrintsPrimerAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := cli(t, dir).wantCode(t, 0)
	if r.stderr != "" {
		t.Fatalf("bare koan wrote to stderr:\n%s", r.stderr)
	}
	r.wantStdoutHas(t,
		"koan <template>",
		"bags",
		"lint",
		"recipes",
		"schema",
		// lint's one-line Short must not read as though a multi-line
		// template were something it would refuse.
		"a multi-line template is valid; it needs --json",
		// 0 is a seed like any other; the default only looks like "unset".
		"0 is a valid seed",
	)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("bare koan created %d entries in an empty directory", len(entries))
	}
}

func TestBareAndHelpAreTheSameDocument(t *testing.T) {
	dir := t.TempDir()
	bare := cli(t, dir).wantCode(t, 0)
	help := cli(t, dir, "--help").wantCode(t, 0)
	if bare.stdout != help.stdout {
		t.Fatalf("koan and koan --help differ:\n%q\n%q", bare.stdout, help.stdout)
	}
}

func TestRootHelpCarriesAWorkedExample(t *testing.T) {
	r := cli(t, t.TempDir(), "--help").wantCode(t, 0)
	r.wantStdoutHas(t, "Examples:", "koan ", "--seed")
}

func TestRootHelpNamesTheDotSlashBagsCase(t *testing.T) {
	cli(t, t.TempDir(), "--help").wantCode(t, 0).wantStdoutHas(t, "./bags")
}

func TestVersion(t *testing.T) {
	r := cli(t, t.TempDir(), "--version").wantCode(t, 0)
	if !strings.HasPrefix(r.stdout, "koan ") || !strings.HasSuffix(r.stdout, "\n") {
		t.Fatalf("--version printed %q, want \"koan <version>\\n\"", r.stdout)
	}
	if strings.TrimSpace(strings.TrimPrefix(r.stdout, "koan ")) == "" {
		t.Fatalf("--version printed no version: %q", r.stdout)
	}
}

func TestPlainDrawIsGolden(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", "412").wantCode(t, 0)
	r.wantStdout(t, goldenPlain)
	if r.stderr != "seed=412\n" {
		t.Fatalf("stderr = %q, want %q", r.stderr, "seed=412\n")
	}
}

func TestJSONDrawIsGolden(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", "412", "--json").wantCode(t, 0)
	r.wantStdout(t, goldenJSON)
	if r.stderr != "seed=412\n" {
		t.Fatalf("stderr = %q, want %q", r.stderr, "seed=412\n")
	}
	var got struct {
		Seed     uint64            `json:"seed"`
		Index    int               `json:"i"`
		Template string            `json:"template"`
		Slots    map[string]string `json:"slots"`
		Text     string            `json:"text"`
	}
	if err := json.Unmarshal([]byte(lines(r.stdout)[0]), &got); err != nil {
		t.Fatalf("the JSONL line does not parse: %v", err)
	}
	if got.Seed != 412 || got.Index != 0 || got.Template != "cli/koan.txt" ||
		got.Text == "" || len(got.Slots) != 4 {
		t.Fatalf("JSONL line is missing keys: %+v", got)
	}
}

func TestSeedDefaultsToTheClockAndReproduces(t *testing.T) {
	dir := fixture(t)
	first := cli(t, dir, "cli/koan.txt", "--bags", "bags").wantCode(t, 0)
	seed, ok := strings.CutPrefix(strings.TrimSpace(first.stderr), "seed=")
	if !ok {
		t.Fatalf("stderr = %q, want a seed= line", first.stderr)
	}
	if len(seed) > 6 {
		t.Fatalf("clock seed %q is more than six digits", seed)
	}
	again := cli(t, dir, "cli/koan.txt", "--bags", "bags", "--seed", seed).wantCode(t, 0)
	again.wantStdout(t, first.stdout)
}

// TestClockSeedIsNotQuantised guards against a clock that ticks coarser than
// the seed's modulus. macOS reports wall-clock time in microseconds, so a seed
// taken as nanoseconds modulo a million always ended in 000 and only a
// thousand seeds were ever possible. Five runs all landing on a multiple of
// 1000 by chance is a one-in-10^15 event.
func TestClockSeedIsNotQuantised(t *testing.T) {
	dir := fixture(t)
	for range 5 {
		run := cli(t, dir, "cli/koan.txt", "--bags", "bags").wantCode(t, 0)
		seed, ok := strings.CutPrefix(strings.TrimSpace(run.stderr), "seed=")
		if !ok {
			t.Fatalf("stderr = %q, want a seed= line", run.stderr)
		}
		if !strings.HasSuffix(seed, "000") {
			return
		}
	}
	t.Fatal("five clock seeds in a row were multiples of 1000: the seed is quantised by the clock")
}

func TestCountDrawsWithoutRepeatingAValue(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "3", "--json").wantCode(t, 0)
	got := lines(r.stdout)
	if len(got) != 3 {
		t.Fatalf("-n 3 printed %d lines, want 3", len(got))
	}
	seen := map[string]map[string]bool{}
	for _, line := range got {
		var k struct {
			Slots map[string]string `json:"slots"`
		}
		if err := json.Unmarshal([]byte(line), &k); err != nil {
			t.Fatal(err)
		}
		for slot, value := range k.Slots {
			if seen[slot] == nil {
				seen[slot] = map[string]bool{}
			}
			if seen[slot][value] {
				t.Fatalf("bag %s repeated %q across the run", slot, value)
			}
			seen[slot][value] = true
		}
	}
}

func TestCountExtendsRatherThanReplaces(t *testing.T) {
	dir := fixture(t)
	three := cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "3").wantCode(t, 0)
	five := cli(t, dir, "cli/round/round.txt", "--seed", "7", "-n", "5").wantCode(t, 0)
	if len(lines(five.stdout)) != 5 {
		t.Fatalf("-n 5 printed %d lines, want 5", len(lines(five.stdout)))
	}
	if !strings.HasPrefix(five.stdout, three.stdout) {
		t.Fatalf("-n 5 does not extend -n 3:\n%s\n%s", three.stdout, five.stdout)
	}
}

func TestStdinTemplate(t *testing.T) {
	dir := fixture(t)
	r := cliInput(t, dir, "A {adjective} thing.\n", "-", "--bags", "bags", "--seed", "412", "--json").
		wantCode(t, 0)
	if !strings.Contains(r.stdout, `"template":"-"`) {
		t.Fatalf("stdin draw did not name the template \"-\":\n%s", r.stdout)
	}
}

func TestStdinBagsDefaultToTheWorkingDirectory(t *testing.T) {
	dir := fixture(t)
	r := cliInput(t, filepath.Join(dir, "bags"), "A {adjective} thing.\n", "-", "--seed", "412").
		wantCode(t, 0)
	if !strings.HasPrefix(r.stdout, "A ") {
		t.Fatalf("stdin draw with default bags printed %q", r.stdout)
	}
}

func TestBagsDefaultToTheTemplateDirectory(t *testing.T) {
	dir := fixture(t)
	elsewhere := t.TempDir()
	r := cli(t, elsewhere, filepath.Join(dir, "cli", "round", "round.txt"), "--seed", "7").
		wantCode(t, 0)
	if !strings.HasPrefix(r.stdout, "The ") {
		t.Fatalf("default --bags did not find the bags beside the template: %q", r.stdout)
	}
}

func TestRepeatedSlotRepeatsItsValue(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/repeat.txt", "--bags", "bags", "--seed", "412").wantCode(t, 0)
	line := lines(r.stdout)[0]
	first, rest, _ := strings.Cut(line, ", and again ")
	value, _, _ := strings.Cut(rest, ", with ")
	if first != value {
		t.Fatalf("a repeated slot drew twice: %q vs %q in %q", first, value, line)
	}
}

func TestMultiLineTemplateWithJSON(t *testing.T) {
	dir := fixture(t)
	r := cli(t, dir, "cli/multi.txt", "--bags", "bags", "--seed", "412", "--json").wantCode(t, 0)
	if len(lines(r.stdout)) != 1 {
		t.Fatalf("a multi-line template under --json printed %d lines, want 1", len(lines(r.stdout)))
	}
	if !strings.Contains(r.stdout, `\n`) {
		t.Fatalf("the JSON text lost its newlines:\n%s", r.stdout)
	}
}
