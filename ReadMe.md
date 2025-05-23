# Multi-Language Code Runner

A versatile Go-based tool to **compile, execute, and verify** the output of source code files written in multiple programming languages — **Go, C++, Rust, Java, and Python** — against expected outputs.

---

## Features

- **Automatic language detection** based on source file extension
- Supports **compilation** and **execution** for:
  - Go (`.go`)
  - C++ (`.cpp`, `.cc`, `.cxx`)
  - Rust (`.rs`)
  - Java (`.java`)
  - Python (`.py`)
- **Input/output redirection** from/to files
- Compares actual output with expected output and shows **colorful, aligned diffs in terminal**
- Optional **build directory cleanup** (`--cleanup`)
- Verbose logging for debugging (`--verbose`)
- Stores compiled binaries/class files inside a `build` folder

---

## Compile & Install
```bash
go build -o cf-runner main.go
sudo mv ./cf-runner /usr/bin
```

## Usage

```bash
cf-runner [--cleanup] [--verbose] <source_file> <input_file> <output_file> <expected_output_file>
```
 ---

## ScreenShots

![image](https://github.com/user-attachments/assets/35f18c24-6586-4a02-8aa6-0f07a24aee51)
![image](https://github.com/user-attachments/assets/57fd9f89-d22e-4789-9620-ca9e918210fb)

