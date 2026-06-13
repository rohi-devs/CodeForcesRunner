package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Version is injected at build time via -X main.Version=<tag>.
var Version = "dev"

// ── Global flags ──────────────────────────────────────────────────────────────

var (
	verboseFlag = false
	cleanupFlag = false
)

func logVerbose(format string, args ...interface{}) {
	if verboseFlag {
		fmt.Printf("[verbose] "+format+"\n", args...)
	}
}

// ── Language detection ────────────────────────────────────────────────────────

func detectLang(sourceFile string) (string, error) {
	switch filepath.Ext(sourceFile) {
	case ".go":
		return "go", nil
	case ".cpp", ".cc", ".cxx":
		return "cpp", nil
	case ".rs":
		return "rust", nil
	case ".java":
		return "java", nil
	case ".py":
		return "python", nil
	default:
		return "", fmt.Errorf("unsupported extension: %s", filepath.Ext(sourceFile))
	}
}

// ── Compile + run pipeline ────────────────────────────────────────────────────

func compileAndRun(lang, sourceFile, inputFile, outputFile, expectedOutputFile string) error {
	baseName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	buildDir := "build"

	if err := os.MkdirAll(buildDir, os.ModePerm); err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}

	var execPath string
	var compileCmd *exec.Cmd

	switch lang {
	case "go":
		execPath = filepath.Join(buildDir, baseName)
		compileCmd = exec.Command("go", "build", "-o", execPath, sourceFile)
	case "cpp":
		execPath = filepath.Join(buildDir, baseName)
		compileCmd = exec.Command("g++", "-O2", "-std=c++23", "-o", execPath, sourceFile)
	case "rust":
		execPath = filepath.Join(buildDir, baseName)
		compileCmd = exec.Command("rustc", "-O", "-o", execPath, sourceFile)
	case "java":
		compileCmd = exec.Command("javac", "-d", buildDir, sourceFile)
		execPath = "java"
	case "python":
		// no compile step
	default:
		return fmt.Errorf("unsupported language: %s", lang)
	}

	if compileCmd != nil {
		logVerbose("compile: %s", strings.Join(compileCmd.Args, " "))
		compileCmd.Stdout = os.Stdout
		compileCmd.Stderr = os.Stderr
		if err := compileCmd.Run(); err != nil {
			return fmt.Errorf("compilation failed: %w", err)
		}
	}

	inFile, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	var runCmd *exec.Cmd
	switch lang {
	case "go", "cpp", "rust":
		runCmd = exec.Command(execPath)
	case "java":
		runCmd = exec.Command("java", "-cp", buildDir, baseName)
	case "python":
		runCmd = exec.Command("python3", sourceFile)
	}
	logVerbose("run: %s", strings.Join(runCmd.Args, " "))
	runCmd.Stdin = inFile
	runCmd.Stdout = outFile
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	actual, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}
	expected, err := os.ReadFile(expectedOutputFile)
	if err != nil {
		return fmt.Errorf("read expected: %w", err)
	}

	if !bytes.Equal(bytes.TrimSpace(actual), bytes.TrimSpace(expected)) {
		fmt.Println("✗ Output differs:")
		diffLines(string(expected), string(actual))
	} else {
		fmt.Println("✓ Output matches expected")
	}

	if cleanupFlag {
		os.RemoveAll(buildDir)
	}
	return nil
}

// ── Diff display ──────────────────────────────────────────────────────────────

func diffLines(expected, actual string) {
	expLines := strings.Split(strings.TrimRight(expected, "\n"), "\n")
	actLines := strings.Split(strings.TrimRight(actual, "\n"), "\n")

	const colW = 40
	sep := strings.Repeat("═", colW)
	fmt.Printf("╔%s╦%s╗\n", sep, sep)
	fmt.Printf("║ %-*s ║ %-*s ║\n", colW-2, "Expected", colW-2, "Actual")
	fmt.Printf("╠%s╬%s╣\n", sep, sep)

	n := len(expLines)
	if len(actLines) > n {
		n = len(actLines)
	}
	for i := 0; i < n; i++ {
		e, a := "", ""
		if i < len(expLines) {
			e = expLines[i]
		}
		if i < len(actLines) {
			a = actLines[i]
		}
		if e != a {
			fmt.Printf("║ \033[31m%-*s\033[0m ║ \033[32m%-*s\033[0m ║\n",
				colW-2, truncate(e, colW-2), colW-2, truncate(a, colW-2))
		} else {
			fmt.Printf("║ %-*s ║ %-*s ║\n",
				colW-2, truncate(e, colW-2), colW-2, truncate(a, colW-2))
		}
	}
	fmt.Printf("╚%s╩%s╝\n", sep, sep)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

// ── CLI parsing ───────────────────────────────────────────────────────────────

type cliInvocation struct {
	command string
	args    []string

	// --cf-* flags
	cfUser    string
	cfContest int
	cfProblem string
	cfTags    string
	cfVerdict int
	cfKey     string
	cfSecret  string
}

var subcommands = map[string]bool{
	"enter":    true,
	"run":      true,   // compile + run a problem via make
	"status":   true,   // fetch latest verdict from CF API
	"sts":      true,   // live standings
	"setup":    true,
	"lang-ids": true,
	"help":     true,
	"version":  true,
}

func parseCLI(argv []string) (cliInvocation, error) {
	var inv cliInvocation
	for i := 0; i < len(argv); i++ {
		arg := argv[i]

		next := func(flag string) (string, error) {
			i++
			if i >= len(argv) {
				return "", fmt.Errorf("%s requires an argument", flag)
			}
			return argv[i], nil
		}

		switch arg {
		case "-v", "--verbose":
			verboseFlag = true
		case "--cleanup":
			cleanupFlag = true

		case "--cf-user":
			v, err := next(arg); if err != nil { return inv, err }
			inv.cfUser = v
		case "--cf-contest":
			v, err := next(arg); if err != nil { return inv, err }
			n, err := strconv.Atoi(v); if err != nil { return inv, fmt.Errorf("--cf-contest: %w", err) }
			inv.cfContest = n
		case "--cf-problem":
			v, err := next(arg); if err != nil { return inv, err }
			inv.cfProblem = v
		case "--cf-tags":
			v, err := next(arg); if err != nil { return inv, err }
			inv.cfTags = v
		case "--cf-verdict":
			v, err := next(arg); if err != nil { return inv, err }
			n, err := strconv.Atoi(v); if err != nil { return inv, fmt.Errorf("--cf-verdict: %w", err) }
			inv.cfVerdict = n
		case "--cf-key":
			v, err := next(arg); if err != nil { return inv, err }
			inv.cfKey = v
		case "--cf-secret":
			v, err := next(arg); if err != nil { return inv, err }
			inv.cfSecret = v

		default:
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return inv, fmt.Errorf("unknown flag: %s  (run `cfr help`)", arg)
			}
			if inv.command == "" && subcommands[arg] {
				inv.command = arg
			} else {
				inv.args = append(inv.args, arg)
			}
		}
	}
	return inv, nil
}

// ── Usage ─────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Printf("cfr %s — Codeforces CLI\n\n", Version)
	fmt.Println("Contest workflow:")
	fmt.Println("  cfr enter <id>           scaffold workspace, Makefile, git init")
	fmt.Println("  cfr run <A|B|…> [-v]     compile + run via make, diff vs exp.txt")
	fmt.Println("  cfr status <A|B|…> [-v]  fetch latest verdict from CF API")
	fmt.Println("  cfr sts [-v]             live contest standings")
	fmt.Println("  cfr setup                configure ~/.config/cfr/cf.conf")
	fmt.Println("  cfr lang-ids             print CF language ID table")
	fmt.Println()
	fmt.Println("  Inside contest_<id>/  you can also call make directly:")
	fmt.Println("    make compile PROB=A")
	fmt.Println("    make run     PROB=A    (compile + run < in.txt → out.txt)")
	fmt.Println("    make test    PROB=A    (run + diff out.txt vs exp.txt)")
	fmt.Println()
	fmt.Println("Standalone local runner (no contest context needed):")
	fmt.Println("  cfr <source> <in> <out> <exp>   compile, run, diff")
	fmt.Println("    flags: --cleanup  --verbose")
	fmt.Println()
	fmt.Println("CF API queries:")
	fmt.Println("  cfr --cf-user <handle>")
	fmt.Println("  cfr --cf-contest <id>")
	fmt.Println("  cfr --cf-problem <2232F>       look up a specific problem")
	fmt.Println("  cfr --cf-tags <dp,greedy>      search by tags")
	fmt.Println("  cfr --cf-verdict <contestId>")
	fmt.Println("  cfr --cf-key <k> --cf-secret <s>  (or CF_API_KEY / CF_API_SECRET env)")
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	inv, err := parseCLI(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	switch inv.command {
	case "help":
		printUsage()
		return

	case "version":
		fmt.Println(Version)
		return

	case "enter":
		if len(inv.args) != 1 {
			fatalf("usage: cfr enter <contestId>")
		}
		gcfg, err := LoadGlobalConfig()
		if err != nil {
			fatalf("load config: %v", err)
		}
		if err := EnterContest(inv.args[0], gcfg); err != nil {
			fatalf("%v", err)
		}
		return

	case "run":
		if len(inv.args) != 1 {
			fatalf("usage: cfr run <A|B|C|…> [-v]")
		}
		if err := RunProblem(inv.args[0], verboseFlag); err != nil {
			fatalf("%v", err)
		}
		return

	case "status":
		if len(inv.args) != 1 {
			fatalf("usage: cfr status <A|B|C|…> [-v]")
		}
		if err := FetchStatus(inv.args[0], verboseFlag); err != nil {
			fatalf("%v", err)
		}
		return

	case "sts":
		if err := ContestStatus(verboseFlag); err != nil {
			fatalf("%v", err)
		}
		return

	case "setup":
		if err := SetupConfig(); err != nil {
			fatalf("%v", err)
		}
		return

	case "lang-ids":
		PrintLangIDs()
		return
	}

	// CF API flag mode
	if cfRan := runCFCommands(inv); cfRan {
		return
	}

	// Standalone local runner: cfr <source> <in> <out> <exp>
	if len(inv.args) == 0 {
		printUsage()
		os.Exit(1)
	}
	if len(inv.args) != 4 {
		fatalf("runner expects <source> <input> <output> <expected>")
	}

	src, in, out, exp := inv.args[0], inv.args[1], inv.args[2], inv.args[3]
	lang, err := detectLang(src)
	if err != nil {
		fatalf("%v", err)
	}
	for _, f := range []string{src, in, exp} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			fatalf("file not found: %s", f)
		}
	}
	if err := compileAndRun(lang, src, in, out, exp); err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}