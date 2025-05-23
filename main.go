
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func compileAndRun(lang, sourceFile, inputFile, outputFile, expectedOutputFile string) error {
	var execPath string
	var cmd *exec.Cmd
	baseName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))

	switch lang {
	case "go":
		execPath = "./" + baseName
		fmt.Println("Compiling Go...")
		cmd = exec.Command("go", "build", "-o", execPath, sourceFile)

	case "cpp":
		execPath = "./" + baseName
		fmt.Println("Compiling C++...")
		cmd = exec.Command("g++", "-o", execPath, sourceFile)

	case "rust":
		execPath = "./" + baseName
		fmt.Println("Compiling Rust...")
		cmd = exec.Command("rustc", "-o", execPath, sourceFile)

	case "java":
		fmt.Println("Compiling Java...")
		cmd = exec.Command("javac", sourceFile)
		execPath = "java"
	case "python":
		fmt.Println("Python doesn’t require compilation.")
	default:
		return fmt.Errorf("unsupported language: %s", lang)
	}

	if lang != "python" {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("compilation failed: %v", err)
		}
	}

	// Prepare to run
	fmt.Println("Executing...")
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

	switch lang {
	case "go", "cpp", "rust":
		cmd = exec.Command(execPath)
	case "java":
		cmd = exec.Command("java", baseName) // baseName without .java
	case "python":
		cmd = exec.Command("python3", sourceFile)
	}

	cmd.Stdin = inFile
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execution failed: %v", err)
	}

	// Compare outputs
	fmt.Println("Comparing outputs...")
	actual, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("cannot read actual output: %v", err)
	}
	expected, err := os.ReadFile(expectedOutputFile)
	if err != nil {
		return fmt.Errorf("cannot read expected output: %v", err)
	}

	if !bytes.Equal(bytes.TrimSpace(actual), bytes.TrimSpace(expected)) {
		fmt.Println("Output differs from expected:")
		diffLines(string(expected), string(actual))
	} else {
		fmt.Println("Output matches the expected output!")
	}

	// Cleanup
	if lang == "go" || lang == "cpp" || lang == "rust" {
		os.Remove(execPath)
	} else if lang == "java" {
		os.Remove(baseName + ".class")
	}
	return nil
}

func diffLines(expected, actual string) {
	expLines := strings.Split(expected, "\n")
	actLines := strings.Split(actual, "\n")

	fmt.Println("--- expected_output")
	fmt.Println("+++ actual_output")
	for i := 0; i < len(expLines) || i < len(actLines); i++ {
		var e, a string
		if i < len(expLines) {
			e = expLines[i]
		}
		if i < len(actLines) {
			a = actLines[i]
		}
		if e != a {
			fmt.Printf("- %s\n", e)
			fmt.Printf("+ %s\n", a)
		}
	}
}

func main() {
	if len(os.Args) != 6 {
		fmt.Println("Usage: go run multi_lang_runner.go <lang> <source_file> <input_file> <output_file> <expected_output_file>")
		os.Exit(1)
	}

	lang := strings.ToLower(os.Args[1])
	sourceFile := os.Args[2]
	inputFile := os.Args[3]
	outputFile := os.Args[4]
	expectedOutputFile := os.Args[5]

	files := []string{sourceFile, inputFile, expectedOutputFile}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			fmt.Printf("Error: %s does not exist.\n", f)
			os.Exit(1)
		}
	}

	if err := compileAndRun(lang, sourceFile, inputFile, outputFile, expectedOutputFile); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
