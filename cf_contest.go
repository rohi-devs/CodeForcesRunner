package main

// ─────────────────────────────────────────────────────────────────────────────
//  cf_contest.go  –  Contest workspace, local runner, and live status
//
//  EnterContest   → cfr enter <id>
//    Fetches contest metadata (anonymous standings), creates:
//      contest_<id>/
//        contest_<id>.conf   Makefile   .gitignore
//        A/  solution.cpp  in.txt  out.txt  exp.txt
//        B/  …
//    Then: git init + initial commit.
//
//  RunProblem     → cfr run <A|B|…> [-v]
//    Delegates to `make run PROB=<index>` in the contest root, which
//    compiles and runs solution.<ext> < in.txt and diffs against exp.txt.
//    Falls back to the built-in compileAndRun pipeline if no Makefile.
//
//  ContestStatus  → cfr sts [-v]
//    Anonymous standings: phase/timer, your row, top-10, your recent subs.
//
//  FetchStatus    → cfr status <A|B|…>
//    Polls user.status for the latest verdict of a specific problem in the
//    active contest (useful after a manual web submission).
//
//  SetupConfig    → cfr setup
//    Interactive wizard for $XDG_CONFIG_HOME/cfr/cf.conf.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ── EnterContest ──────────────────────────────────────────────────────────────

func EnterContest(contestIDStr string, gcfg *GlobalConfig) error {
	contestIDStr = strings.TrimSpace(contestIDStr)
	if _, err := strconv.Atoi(contestIDStr); err != nil {
		return fmt.Errorf("invalid contest ID %q: must be an integer", contestIDStr)
	}

	// Fetch contest metadata + problem list — always anonymous.
	client := newCFClient()
	raw, err := client.getAnon("contest.standings", map[string]string{
		"contestId": contestIDStr,
	})
	if err != nil {
		return fmt.Errorf("fetch contest %s: %w", contestIDStr, err)
	}

	var standings CFStandingsResult
	if err := json.Unmarshal(raw, &standings); err != nil {
		return fmt.Errorf("parse standings: %w", err)
	}
	if len(standings.Problems) == 0 {
		return fmt.Errorf("contest %s has no problems (may not have started yet)", contestIDStr)
	}

	lang := normalizeLang(gcfg.DefaultLang)
	if lang == "" {
		lang = "cpp"
	}
	langID := gcfg.DefaultLangID
	if langID == "" {
		langID = CFLangID[lang]
	}
	if langID == "" {
		langID = "89"
	}
	ext := solutionExtension(lang)

	rootDir, err := filepath.Abs("contest_" + contestIDStr)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create contest root: %w", err)
	}

	ct := standings.Contest
	fmt.Printf("┌─ cfr enter #%s — %s\n", contestIDStr, ct.Name)
	fmt.Printf("│  Root     : %s\n", rootDir)
	fmt.Printf("│  Language : %s  (lang_id=%s)\n", lang, langID)
	fmt.Printf("│  Problems : %d\n│\n", len(standings.Problems))

	indices := make([]string, 0, len(standings.Problems))
	for _, p := range standings.Problems {
		indices = append(indices, p.Index)

		dir := filepath.Join(rootDir, p.Index)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s/: %w", p.Index, err)
		}
		writeIfMissing(filepath.Join(dir, "solution"+ext), []byte(solutionStub(lang, p, contestIDStr)))
		writeIfMissing(filepath.Join(dir, "in.txt"),
			[]byte(fmt.Sprintf("# %s%s — %s\n# Paste sample input here\n", contestIDStr, p.Index, p.Name)))
		writeIfMissing(filepath.Join(dir, "out.txt"), []byte(""))
		writeIfMissing(filepath.Join(dir, "exp.txt"),
			[]byte(fmt.Sprintf("# %s%s — %s\n# Paste expected output here\n", contestIDStr, p.Index, p.Name)))

		rating := ""
		if p.Rating > 0 {
			rating = fmt.Sprintf("  ★%d", p.Rating)
		}
		fmt.Printf("│  [%s] %s%s\n", p.Index, p.Name, rating)
	}

	writeIfMissing(filepath.Join(rootDir, ".gitignore"), []byte("build/\n*.class\n*.o\n*.out\n*.log\n"))
	writeMakefile(rootDir, lang, ext, indices)

	startTime := ""
	if ct.StartTimeSeconds > 0 {
		startTime = strconv.FormatInt(ct.StartTimeSeconds, 10)
	}
	ccfg := &ContestConfig{
		ContestID:   contestIDStr,
		ContestName: ct.Name,
		RootDir:     rootDir,
		Problems:    indices,
		Lang:        lang,
		LangID:      langID,
		StartTime:   startTime,
		EnteredAt:   strconv.FormatInt(time.Now().Unix(), 10),
		path:        ContestConfigPath(rootDir, contestIDStr),
	}
	if err := ccfg.Save(); err != nil {
		return fmt.Errorf("save contest config: %w", err)
	}
	fmt.Printf("│\n│  Wrote: contest_%s.conf\n│  Wrote: Makefile\n", contestIDStr)

	// git init + initial commit
	fmt.Println("│")
	if err := runGit(rootDir, "init", "-q"); err != nil {
		fmt.Printf("│  ⚠  git init: %v\n", err)
	} else {
		runGit(rootDir, "add", ".")
		msg := fmt.Sprintf("cfr: enter contest %s — %s", contestIDStr, ct.Name)
		if err := runGit(rootDir, "commit", "-q", "-m", msg); err != nil {
			fmt.Printf("│  ⚠  git commit: %v\n", err)
		} else {
			fmt.Println("│  git: init + initial commit done")
		}
	}

	fmt.Printf("│\n│  Next steps:\n")
	fmt.Printf("│    cd contest_%s\n", contestIDStr)
	fmt.Printf("│    make run PROB=A        # compile + run A against in.txt\n")
	fmt.Printf("│    make test PROB=A       # compile + run + diff against exp.txt\n")
	fmt.Printf("│    cfr sts                # live standings\n")
	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// writeMakefile writes the contest Makefile into rootDir.
// Targets: compile, run, test — all take PROB=<index>.
// Also delegates to the top-level cfr Makefile for build/install.
func writeMakefile(rootDir, lang, ext string, indices []string) {
	// Compiler + flags per language
	type langMeta struct {
		compile string // compile command, %SRC% and %BIN% are placeholders
		run     string // run command, %BIN% = binary / main class / script
	}
	meta := map[string]langMeta{
		"cpp":    {`g++ -O2 -std=c++23 -o build/%BIN% %SRC%`, `./build/%BIN%`},
		"java":   {`javac -d build %SRC%`, `java -cp build %BIN%`},
		"python": {``, `python3 %SRC%`},
		"go":     {`go build -o build/%BIN% %SRC%`, `./build/%BIN%`},
		"rust":   {`rustc -O -o build/%BIN% %SRC%`, `./build/%BIN%`},
	}
	m, ok := meta[lang]
	if !ok {
		m = meta["cpp"]
	}

	// For Java the "binary" is the class name (solution), not a file path.
	binPlaceholder := "$(PROB)"
	if lang == "java" {
		binPlaceholder = "solution"
	}

	compileRule := strings.NewReplacer("%SRC%", "$(PROB)/solution"+ext, "%BIN%", binPlaceholder).Replace(m.compile)
	runRule := strings.NewReplacer("%SRC%", "$(PROB)/solution"+ext, "%BIN%", binPlaceholder).Replace(m.run)

	// Python has no compile step
	compileTarget := ""
	if lang != "python" {
		compileTarget = fmt.Sprintf(`
## compile: Compile solution for PROB (e.g. make compile PROB=A)
compile:
	@mkdir -p build
	%s
`, compileRule)
	}

	content := fmt.Sprintf(`# contest Makefile — generated by cfr enter
# Usage:
#   make compile PROB=A
#   make run     PROB=A      (compiles then runs < in.txt, writes out.txt)
#   make test    PROB=A      (run + diff out.txt vs exp.txt)
#   make clean
#   make build               (build cfr binary itself, delegates up)

PROB ?= A
PROBLEMS := %s
%s
## run: Compile + run PROB < in.txt, write out.txt
run:%s
	%s < $(PROB)/in.txt > $(PROB)/out.txt
	@echo "--- output ($(PROB)/out.txt) ---"
	@cat $(PROB)/out.txt

## test: run + diff out.txt against exp.txt
test: run
	@echo "--- diff ---"
	@if diff -q $(PROB)/out.txt $(PROB)/exp.txt > /dev/null 2>&1; then \
		echo "✓  $(PROB): output matches expected"; \
	else \
		diff --color=always $(PROB)/out.txt $(PROB)/exp.txt || true; \
		echo "✗  $(PROB): output differs from expected"; \
	fi

## clean: Remove build artefacts
clean:
	@rm -rf build
	@echo "cleaned"

## build: Build the cfr binary (delegates to parent Makefile if present)
build:
	@if [ -f ../Makefile ]; then $(MAKE) -C .. build; else echo "no parent Makefile found"; fi

.PHONY: compile run test clean build
.DEFAULT_GOAL := run
`,
		strings.Join(indices, " "),
		compileTarget,
		func() string {
			if lang == "python" {
				return ""
			}
			return " compile"
		}(),
		runRule,
	)

	writeIfMissing(filepath.Join(rootDir, "Makefile"), []byte(content))
}

// ── RunProblem ────────────────────────────────────────────────────────────────

// RunProblem runs `make test PROB=<index>` in the contest root.
// Falls back to the built-in compileAndRun if no Makefile is found.
func RunProblem(index string, verbose bool) error {
	index = strings.ToUpper(strings.TrimSpace(index))
	if index == "" {
		return fmt.Errorf("problem index required (e.g. A, B, C1)")
	}

	ccfg, err := FindContestConfig()
	if err != nil {
		return fmt.Errorf("not inside a contest workspace: %w\n  Run: cfr enter <contestId>", err)
	}

	makefile := filepath.Join(ccfg.RootDir, "Makefile")
	if _, err := os.Stat(makefile); err == nil {
		// Delegate to make test PROB=<index>
		target := "test"
		args := []string{target, "PROB=" + index}
		if verbose {
			fmt.Printf("┌─ make %s\n", strings.Join(args, " "))
		}
		cmd := exec.Command("make", args...)
		cmd.Dir = ccfg.RootDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("make test failed: %w", err)
		}
		return nil
	}

	// Fallback: built-in pipeline
	ext := solutionExtension(ccfg.Lang)
	src := filepath.Join(ccfg.RootDir, index, "solution"+ext)
	in  := filepath.Join(ccfg.RootDir, index, "in.txt")
	out := filepath.Join(ccfg.RootDir, index, "out.txt")
	exp := filepath.Join(ccfg.RootDir, index, "exp.txt")

	for _, f := range []string{src, in, exp} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", f)
		}
	}

	lang, err := detectLang(src)
	if err != nil {
		return err
	}
	fmt.Printf("┌─ cfr run %s  (built-in runner)\n", index)
	return compileAndRun(lang, src, in, out, exp)
}

// ── FetchStatus ───────────────────────────────────────────────────────────────

// FetchStatus polls user.status for the latest verdict of problem <index>
// in the active contest. Useful after a manual web submission.
func FetchStatus(index string, verbose bool) error {
	index = strings.ToUpper(strings.TrimSpace(index))

	ccfg, err := FindContestConfig()
	if err != nil {
		return fmt.Errorf("not inside a contest workspace: %w", err)
	}
	gcfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}
	if gcfg.Handle == "" {
		return fmt.Errorf("handle not set in cf.conf — run `cfr setup`")
	}

	fmt.Printf("┌─ Fetching status  %s%s — %s\n", ccfg.ContestID, index, ccfg.ContestName)
	fmt.Printf("│  Handle: %s\n│\n", gcfg.Handle)

	client := newCFClientWithCreds(gcfg.APIKey, gcfg.APISecret)

	deadline := time.Now().Add(3 * time.Minute)
	attempt := 0
	for {
		attempt++
		raw, err := client.get("user.status", map[string]string{
			"handle": gcfg.Handle,
			"from":   "1",
			"count":  "10",
		})
		if err != nil {
			fmt.Printf("│  ⚠  poll #%d: %v\n", attempt, err)
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(2 * time.Second)
			continue
		}

		var subs []CFSubmission
		if err := json.Unmarshal(raw, &subs); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// Find the most recent submission for this problem in this contest
		for _, s := range subs {
			if strconv.Itoa(s.Problem.ContestId) != ccfg.ContestID {
				continue
			}
			if index != "" && !strings.EqualFold(s.Problem.Index, index) {
				continue
			}

			verdict := s.Verdict
			if verdict == "" {
				verdict = "TESTING"
			}

			ts := time.Unix(s.CreationTimeSeconds, 0).Format("15:04:05")
			if verbose {
				fmt.Printf("│  [poll #%d @ %s]  #%d  %s\n", attempt, ts, s.Id, verdict)
			} else {
				fmt.Printf("│  … %-22s\r", verdict)
			}

			if verdict != "TESTING" && verdict != "SUBMITTED" {
				fmt.Print("│                               \r")
				fmt.Printf("│  Submission #%d  [%s]\n", s.Id, ts)
				fmt.Printf("│  Problem  : %s%s — %s\n",
					ccfg.ContestID, s.Problem.Index, s.Problem.Name)
				fmt.Printf("│  Language : %s\n", s.ProgrammingLanguage)
				fmt.Printf("│  Verdict  : %s\n", verdictColor(verdict))
				if verdict == "OK" || verdict == "PARTIAL" {
					fmt.Printf("│  Tests    : %d passed\n", s.PassedTestCount)
				}
				fmt.Printf("│  Time     : %d ms    Memory: %d KB\n",
					s.TimeConsumedMillis, s.MemoryConsumedBytes/1024)
				fmt.Println("└────────────────────────────────────────────────────────────────────────────")
				return nil
			}
			break // still testing, loop again
		}

		if time.Now().After(deadline) {
			fmt.Println("│  timed out after 3 minutes")
			break
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// ── ContestStatus ─────────────────────────────────────────────────────────────

func ContestStatus(verbose bool) error {
	ccfg, err := FindContestConfig()
	if err != nil {
		return fmt.Errorf("not inside a contest workspace: %w\n  Run: cfr enter <contestId>", err)
	}
	gcfg, _ := LoadGlobalConfig() // non-fatal if missing

	client := newCFClient()
	raw, err := client.getAnon("contest.standings", map[string]string{
		"contestId": ccfg.ContestID,
	})
	if err != nil {
		return err
	}

	var result CFStandingsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse standings: %w", err)
	}

	ct := result.Contest
	fmt.Printf("\n┌─ Contest #%s — %s\n", ccfg.ContestID, ct.Name)

	// Phase + elapsed/remaining timer
	fmt.Printf("│  Phase : %s", ct.Phase)
	if ct.StartTimeSeconds > 0 {
		dur     := time.Duration(ct.DurationSeconds) * time.Second
		elapsed := time.Since(time.Unix(ct.StartTimeSeconds, 0))
		if elapsed < 0 {
			elapsed = 0
		}
		remaining := dur - elapsed
		if remaining < 0 {
			remaining = 0
		}
		fmt.Printf("   elapsed %s / %s remaining", fmtDuration(elapsed), fmtDuration(remaining))
	}
	fmt.Println()

	// Problem index row
	fmt.Printf("│  Problems:")
	for _, p := range result.Problems {
		fmt.Printf("  %s", p.Index)
	}
	fmt.Println()

	// User's own row
	handle := ""
	if gcfg != nil {
		handle = gcfg.Handle
	}
	if handle != "" {
		found := false
		for _, row := range result.Rows {
			for _, m := range row.Party.Members {
				if strings.EqualFold(m.Handle, handle) {
					fmt.Printf("│\n│  Your standing:\n")
					printStandingRow("│    ", row, result.Problems)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			fmt.Printf("│\n│  (%s not yet in standings)\n", handle)
		}
	}

	// Top 10
	rows := result.Rows
	if len(rows) > 10 {
		rows = rows[:10]
	}
	fmt.Printf("│\n│  Top %d of %d:\n", len(rows), len(result.Rows))
	fmt.Printf("│  %-5s %-20s", "Rank", "Handle")
	for _, p := range result.Problems {
		fmt.Printf("  %-5s", p.Index)
	}
	fmt.Printf("  Points  Penalty\n")
	for _, row := range rows {
		printStandingRow("│  ", row, result.Problems)
	}

	// User's recent subs in this contest
	if gcfg != nil && gcfg.Handle != "" {
		apiClient := newCFClientWithCreds(gcfg.APIKey, gcfg.APISecret)
		if raw2, err := apiClient.get("user.status", map[string]string{
			"handle": gcfg.Handle,
			"from":   "1",
			"count":  "20",
		}); err == nil {
			var subs []CFSubmission
			var mine []CFSubmission
			if json.Unmarshal(raw2, &subs) == nil {
				for _, s := range subs {
					if strconv.Itoa(s.Problem.ContestId) == ccfg.ContestID {
						mine = append(mine, s)
					}
				}
			}
			if len(mine) > 0 {
				fmt.Printf("│\n│  Your submissions:\n")
				for _, s := range mine {
					ts := time.Unix(s.CreationTimeSeconds, 0).Format("15:04:05")
					v := s.Verdict
					if v == "" {
						v = "TESTING"
					}
					fmt.Printf("│    [%s] %s%-2s  %-26s  %s\n",
						ts, ccfg.ContestID, s.Problem.Index,
						truncate(s.ProgrammingLanguage, 26),
						verdictColor(v))
				}
			}
		}
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

func printStandingRow(prefix string, row CFRanklistRow, problems []CFProblem) {
	handle := "?"
	if len(row.Party.Members) > 0 {
		handle = row.Party.Members[0].Handle
	}
	fmt.Printf("%s%-5d %-20s", prefix, row.Rank, truncate(handle, 20))
	for i := range problems {
		cell := "  -  "
		if i < len(row.ProblemResults) {
			pr := row.ProblemResults[i]
			switch {
			case pr.Points > 0:
				cell = fmt.Sprintf("+%-4.0f", pr.Points)
			case pr.RejectedAttemptCount > 0:
				cell = fmt.Sprintf("-%d   ", pr.RejectedAttemptCount)
			}
		}
		fmt.Printf("  %-5s", cell)
	}
	fmt.Printf("  %-7.0f %d\n", row.Points, row.Penalty)
}

// ── SetupConfig ───────────────────────────────────────────────────────────────

func SetupConfig() error {
	gcfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}
	path, _ := GlobalConfigPath()
	fmt.Printf("┌─ cfr setup — %s\n│\n", path)

	r := bufio.NewReader(os.Stdin)
	prompt := func(label, current string) string {
		if current != "" {
			fmt.Printf("│  %s [%s]: ", label, current)
		} else {
			fmt.Printf("│  %s: ", label)
		}
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return current
		}
		return line
	}

	gcfg.Handle    = prompt("CF handle", gcfg.Handle)
	gcfg.APIKey    = prompt("API key (codeforces.com/settings/api)", gcfg.APIKey)
	gcfg.APISecret = prompt("API secret", gcfg.APISecret)

	fmt.Println("│")
	PrintLangIDs()
	rawLang := prompt("Default language (cpp/java/python/go/rust)", gcfg.DefaultLang)
	gcfg.DefaultLang = normalizeLang(rawLang)
	if gcfg.DefaultLang == "" {
		gcfg.DefaultLang = "cpp"
	}
	if gcfg.DefaultLangID == "" {
		gcfg.DefaultLangID = CFLangID[gcfg.DefaultLang]
	}
	gcfg.DefaultLangID = prompt("Default lang_id", gcfg.DefaultLangID)

	if err := gcfg.Save(); err != nil {
		return err
	}
	fmt.Printf("│\n│  Saved → %s\n", path)
	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
	return nil
}

// ── Solution stubs ────────────────────────────────────────────────────────────

func solutionStub(lang string, p CFProblem, contestID string) string {
	title := fmt.Sprintf("%s%s — %s", contestID, p.Index, p.Name)
	switch normalizeLang(lang) {
	case "cpp":
		return fmt.Sprintf(`// %s
#include <bits/stdc++.h>
using namespace std;

int main() {
    ios_base::sync_with_stdio(false);
    cin.tie(NULL);

    

    return 0;
}
`, title)
	case "java":
		return fmt.Sprintf(`// %s
import java.util.*;
import java.io.*;

public class solution {
    static BufferedReader br = new BufferedReader(new InputStreamReader(System.in));
    static PrintWriter out = new PrintWriter(new BufferedWriter(new OutputStreamWriter(System.out)));

    public static void main(String[] args) throws IOException {
        

        out.flush();
    }
}
`, title)
	case "python":
		return fmt.Sprintf(`# %s
import sys
input = sys.stdin.readline

def solve():
    pass

T = int(input())
for _ in range(T):
    solve()
`, title)
	case "go":
		return fmt.Sprintf(`// %s
package main

import (
	"bufio"
	"fmt"
	"os"
)

var reader *bufio.Reader
var writer *bufio.Writer

func main() {
	reader = bufio.NewReader(os.Stdin)
	writer = bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	

	_ = fmt.Fscan
}
`, title)
	case "rust":
		return fmt.Sprintf(`// %s
use std::io::{self, BufRead, Write, BufWriter};

fn main() {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut out = BufWriter::new(stdout.lock());
    let mut lines = stdin.lock().lines();

    // TODO: solve %s
    let _ = writeln!(out, "");
}
`, title, p.Index)
	default:
		return fmt.Sprintf("// %s\n// TODO\n", title)
	}
}

// ── Git helper ────────────────────────────────────────────────────────────────

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(out.String()))
	}
	logVerbose("git %v → OK", args)
	return nil
}

// ── File helpers ──────────────────────────────────────────────────────────────

func writeIfMissing(path string, content []byte) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, content, 0o644)
}

func findSourceFile(ccfg *ContestConfig, index string) (string, error) {
	ext := solutionExtension(ccfg.Lang)
	candidates := []string{
		filepath.Join(ccfg.RootDir, index, "solution"+ext),
		filepath.Join(ccfg.RootDir, index, "main"+ext),
	}
	for _, e := range []string{".cpp", ".java", ".py", ".go", ".rs"} {
		if e == ext {
			continue
		}
		candidates = append(candidates, filepath.Join(ccfg.RootDir, index, "solution"+e))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("source not found for %s — looked in %s/%s/solution%s",
		index, ccfg.RootDir, index, ext)
}