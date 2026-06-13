# cfr

A lightweight Codeforces CLI for contest workspace generation, local testing, verdict tracking, standings monitoring, and Codeforces API integration.

---

## Features

- Contest workspace generation (`cfr enter`)
- Automatic contest Makefile generation
- Git repository initialization
- Local compile, run, and test workflow
- Standalone runner mode
- Live contest standings
- Submission verdict tracking
- User profile lookup
- Contest metadata inspection
- Problem lookup by contest/problem ID
- Problem search by tags
- Global and per-contest configuration
- Multi-language support
- Output diff visualization
- Verbose execution mode

---

## Supported Languages

| Language | CF ProgramTypeId |
|-----------|------------------|
| C++23 | 89 |
| C++20 | 73 |
| C++17 | 54 |
| Java 21 | 60 |
| PyPy 3 | 71 |
| CPython 3.8 | 31 |
| Go | 75 |
| Rust 2021 | 49 |

Display the full table:

```bash
cfr lang-ids
```

---

# Installation

## Requirements

- Go 1.20+
- GNU Make
- Git (optional, used during contest initialization)

Verify:

```bash
go version
make --version
git --version
```

---

## Build

```bash
make build
```

Output:

```text
build/cfr
```

---

## Install

Install into your Go binary directory:

```bash
make install
```

Binary location:

```text
$GOPATH/bin/cfr
```

Ensure your PATH contains:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Alternatively:

```bash
go install ./...
```

Verify:

```bash
cfr version
```

---

# Quick Start

Initialize a contest workspace:

```bash
cfr enter 2101
```

Enter the workspace:

```bash
cd contest_2101
```

Run a solution:

```bash
cfr run A
```

Check latest verdict:

```bash
cfr status A
```

Watch standings:

```bash
cfr sts
```

---

# Contest Workflow

## Create Contest Workspace

```bash
cfr enter <contestId>
```

Example:

```bash
cfr enter 2101
```

Generated structure:

```text
contest_2101/
├── contest_2101.conf
├── Makefile
├── .gitignore
├── A/
│   ├── solution.cpp
│   ├── in.txt
│   ├── out.txt
│   └── exp.txt
├── B/
├── C/
└── ...
```

The command:

- Fetches contest metadata
- Creates problem folders
- Generates solution templates
- Generates a contest Makefile
- Creates contest configuration
- Initializes Git
- Creates an initial commit

---

## Run a Problem

Compile and execute a problem locally:

```bash
cfr run A
```

Verbose mode:

```bash
cfr run A -v
```

The command:

1. Locates the contest workspace
2. Invokes:

```bash
make test PROB=A
```

3. Compiles the solution
4. Runs it against `in.txt`
5. Compares output against `exp.txt`

---

## Check Submission Status

Poll Codeforces for the latest verdict:

```bash
cfr status A
```

Verbose:

```bash
cfr status A -v
```

Useful after submitting from the browser.

---

## Live Standings

Display:

- Contest phase
- Remaining time
- Your ranking
- Top standings
- Recent submissions

```bash
cfr sts
```

Verbose:

```bash
cfr sts -v
```

---

# Contest Makefile

Inside a contest workspace:

```bash
cd contest_2101
```

Available commands:

---

## Compile

```bash
make compile PROB=A
```

---

## Run

```bash
make run PROB=A
```

Input:

```text
A/in.txt
```

Output:

```text
A/out.txt
```

---

## Test

```bash
make test PROB=A
```

Compares:

```text
A/out.txt
```

against

```text
A/exp.txt
```

---

## Clean

```bash
make clean
```

---

# Standalone Runner

You can use cfr without creating a contest workspace.

## Syntax

```bash
cfr <source> <input> <output> <expected>
```

Example:

```bash
cfr solution.cpp in.txt out.txt exp.txt
```

Workflow:

```text
Compile
   ↓
Execute
   ↓
Write output
   ↓
Compare expected output
```

---

## Verbose Mode

```bash
cfr solution.cpp in.txt out.txt exp.txt --verbose
```

---

## Cleanup Build Artifacts

```bash
cfr solution.cpp in.txt out.txt exp.txt --cleanup
```

---

# Codeforces API

---

## User Information

```bash
cfr --cf-user tourist
```

Displays:

- Rank
- Rating
- Max rating
- Contribution
- Registration date
- Recent rating changes
- Recent submissions

---

## Contest Information

```bash
cfr --cf-contest 2101
```

Displays:

- Contest metadata
- Duration
- Problems
- Top standings
- Rating change summary

---

## Problem Lookup

Lookup a specific problem:

```bash
cfr --cf-problem 2232F
```

Displays:

- Contest
- Problem name
- Rating
- Tags
- URL

---

## Search by Tags

Single tag:

```bash
cfr --cf-tags dp
```

Multiple tags:

```bash
cfr --cf-tags dp,greedy
```

Displays recent matching problems.

---

## Latest Contest Submission

```bash
cfr --cf-verdict 2101
```

Displays:

- Submission ID
- Author
- Problem
- Language
- Verdict
- Time usage
- Memory usage

---

# API Authentication

Public API queries work without credentials.

Authenticated requests support:

- user.status
- contest.status
- verdict polling

---

## Environment Variables

Recommended:

```bash
export CF_API_KEY="your-key"
export CF_API_SECRET="your-secret"
```

---

## Command Line

```bash
cfr \
  --cf-key your-key \
  --cf-secret your-secret \
  --cf-user tourist
```

---

# Configuration

## Global Configuration

Location:

```text
$XDG_CONFIG_HOME/cfr/cf.conf
```

Generate interactively:

```bash
cfr setup
```

Example:

```ini
handle=tourist

api_key=xxxxxxxx
api_secret=xxxxxxxx

default_lang=cpp
default_lang_id=89
```

---

## Contest Configuration

Generated automatically:

```text
contest_<id>/contest_<id>.conf
```

Example:

```ini
contest_id=2101
contest_name=Codeforces Round #2101

root_dir=/home/user/contest_2101

problems=A,B,C,D,E,F

lang=cpp
lang_id=89

start_time=1740000000
entered_at=1740001234
```

---

# Development

## Format

```bash
make fmt
```

---

## Vet

```bash
make vet
```

---

## Lint

```bash
make lint
```

Requires:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## Run Tests

```bash
make test
```

---

## Coverage Report

```bash
make coverage
```

Generates:

```text
coverage.out
coverage.html
```

---

## Download Dependencies

```bash
make deps
```

---

## Tidy Modules

```bash
make tidy
```

---

## Build

```bash
make build
```

---

## Run

```bash
make run
```

---

## Clean

```bash
make clean
```

---

## Full Pipeline

Runs:

```text
clean
fmt
vet
test
build
```

Command:

```bash
make all
```

---

# Project Structure

```text
cfr/
├── build/
├── cfapi.go
├── cf_config.go
├── cf_contest.go
├── main.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

Generated contest workspace:

```text
contest_2101/
├── contest_2101.conf
├── Makefile
├── .gitignore
├── A/
│   ├── solution.cpp
│   ├── in.txt
│   ├── out.txt
│   └── exp.txt
├── B/
├── C/
└── ...
```

---

# Example Contest Session

```bash
# Configure account
cfr setup

# Enter contest
cfr enter 2101

cd contest_2101

# Solve problem A
vim A/solution.cpp

# Run local tests
cfr run A

# Submit via browser

# Monitor verdict
cfr status A

# View standings
cfr sts
```

---

# Version

Display current version:

```bash
cfr version
```

Versions are injected during build:

```bash
git describe --tags --always --dirty
```

---

# License

MIT License