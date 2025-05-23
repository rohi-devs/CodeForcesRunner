package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	cleanupFlag = flag.Bool("cleanup", false, "Remove the build directory after execution")
	verboseFlag = flag.Bool("verbose", false, "Print detailed logs")
)

func logVerbose(format string, args ...interface{}) {
	if *verboseFlag {
		fmt.Printf("[verbose] "+format+"\n", args...)
	}
}

func detectLang(sourceFile string) (string, error) {
	ext := filepath.Ext(sourceFile)
	switch ext {
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
		return "", fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func compileAndRun(lang, sourceFile, inputFile, outputFile, expectedOutputFile string, cleanup bool) error {
	var execPath string
	var cmd *exec.Cmd
	baseName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	buildDir := "build"

	if err := os.MkdirAll(buildDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create build directory: %v", err)
	}
	logVerbose("Created build directory: %s", buildDir)

	switch lang {
	case "go":
		execPath = filepath.Join(buildDir, baseName)
		logVerbose("Compiling Go to %s", execPath)
		cmd = exec.Command("go", "build", "-o", execPath, sourceFile)

	case "cpp":
		execPath = filepath.Join(buildDir, baseName)
		logVerbose("Compiling C++ to %s", execPath)
		cmd = exec.Command("g++", "-o", execPath, sourceFile)

	case "rust":
		execPath = filepath.Join(buildDir, baseName)
		logVerbose("Compiling Rust to %s", execPath)
		cmd = exec.Command("rustc", "-o", execPath, sourceFile)

	case "java":
		logVerbose("Compiling Java to %s", buildDir)
		cmd = exec.Command("javac", "-d", buildDir, sourceFile)
		execPath = "java"

	case "python":
		logVerbose("Python script: %s", sourceFile)

	default:
		return fmt.Errorf("unsupported language: %s", lang)
	}

	if lang != "python" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		logVerbose("Running command: %s", strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("compilation failed: %v", err)
		}
	}

	inFile, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("cannot open input file: %v", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("cannot create output file: %v", err)
	}
	defer outFile.Close()

	fmt.Println("Executing...")
	switch lang {
	case "go", "cpp", "rust":
		cmd = exec.Command(execPath)
	case "java":
		cmd = exec.Command("java", "-cp", buildDir, baseName)
	case "python":
		cmd = exec.Command("python3", sourceFile)
	}
	logVerbose("Running command: %s", strings.Join(cmd.Args, " "))
	cmd.Stdin = inFile
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execution failed: %v", err)
	}

	fmt.Println("Comparing outputs...")
	actual, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("cannot read actual output file: %v", err)
	}
	expected, err := os.ReadFile(expectedOutputFile)
	if err != nil {
		return fmt.Errorf("cannot read expected output file: %v", err)
	}

	if !bytes.Equal(bytes.TrimSpace(actual), bytes.TrimSpace(expected)) {
		fmt.Println("Output differs from expected:")
		diffLines(string(expected), string(actual))
	} else {
		fmt.Println("Output matches the expected output!")
	}

	if cleanup {
		logVerbose("Removing build directory: %s", buildDir)
		if err := os.RemoveAll(buildDir); err != nil {
			fmt.Printf("Warning: failed to remove build directory: %v\n", err)
		}
	}

	return nil
}

func diffLines(expected, actual string) {
	expLines := strings.Split(expected, "\n")
	actLines := strings.Split(actual, "\n")

	const width = 40
	sep := strings.Repeat("═", width)
	fmt.Printf("╔%s╦%s╗\n", sep, sep)
	fmt.Printf("║ %-*s ║ %-*s ║\n", width-2, "Expected (-)", width-2, "Actual (+)")
	fmt.Printf("╠%s╬%s╣\n", sep, sep)

	max := len(expLines)
	if len(actLines) > max {
		max = len(actLines)
	}

	for i := 0; i < max; i++ {
		var e, a string
		if i < len(expLines) {
			e = expLines[i]
		}
		if i < len(actLines) {
			a = actLines[i]
		}
		if e != a {
			printColoredRow(e, a, width-2)
		} else {
			fmt.Printf("║ %-*s ║ %-*s ║\n", width-2, truncate(e, width-2), width-2, truncate(a, width-2))
		}
	}

	fmt.Printf("╚%s╩%s╝\n", sep, sep)
}

func printColoredRow(expected, actual string, width int) {
	red := "\033[31m"
	green := "\033[32m"
	reset := "\033[0m"

	truncE := truncate(expected, width)
	truncA := truncate(actual, width)

	// ANSI codes do not consume width, so padding is safe
	fmt.Printf("║ %s%-*s%s ║ %s%-*s%s ║\n",
		red, width, truncE, reset,
		green, width, truncA, reset)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) != 4 {
		fmt.Println("Usage: go run multi_lang_runner.go [--cleanup] [--verbose] <source_file> <input_file> <output_file> <expected_output_file>")
		os.Exit(1)
	}

	sourceFile := args[0]
	inputFile := args[1]
	outputFile := args[2]
	expectedOutputFile := args[3]

	lang, err := detectLang(sourceFile)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	logVerbose("Detected language: %s", lang)

	files := []string{sourceFile, inputFile, expectedOutputFile}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			fmt.Printf("Error: %s does not exist.\n", f)
			os.Exit(1)
		}
	}

	if err := compileAndRun(lang, sourceFile, inputFile, outputFile, expectedOutputFile, *cleanupFlag); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

