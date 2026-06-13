package main

// ─────────────────────────────────────────────────────────────────────────────
//  cf_config.go  –  Config file management
//
//  Two config files, plain key=value (# = comment, [sections] ignored):
//
//  1. $XDG_CONFIG_HOME/cfr/cf.conf        — global account settings
//     handle, api_key, api_secret, default_lang, default_lang_id
//
//  2. <contest_root>/contest_<id>.conf    — per-contest settings
//     contest_id, contest_name, root_dir, problems, lang, lang_id,
//     start_time, entered_at
// ─────────────────────────────────────────────────────────────────────────────

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type GlobalConfig struct {
	Handle        string // CF handle
	APIKey        string // CF API key
	APISecret     string // CF API secret
	DefaultLang   string // cpp | java | python | go | rust
	DefaultLangID string // CF programTypeId, e.g. "89"
	path          string // absolute path to cf.conf
}

type ContestConfig struct {
	ContestID   string   // "2232"
	ContestName string   // "Codeforces Round 1027 (Div. 3)"
	RootDir     string   // absolute path to contest root
	Problems    []string // ["A","B","C","D","E"]
	Lang        string   // "cpp"
	LangID      string   // "89"
	StartTime   string   // unix timestamp of contest start
	EnteredAt   string   // unix timestamp when cfr enter was run
	path        string   // absolute path to contest_<id>.conf
}

// ── Global config ─────────────────────────────────────────────────────────────

func xdgConfigDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "cfr")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create config dir %s: %w", dir, err)
	}
	return dir, nil
}

func GlobalConfigPath() (string, error) {
	dir, err := xdgConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cf.conf"), nil
}

// LoadGlobalConfig reads cf.conf.
// Returns an empty config (not an error) if the file doesn't exist yet.
func LoadGlobalConfig() (*GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}
	cfg := &GlobalConfig{path: path}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open cf.conf: %w", err)
	}
	defer f.Close()

	kv, err := parseINI(f)
	if err != nil {
		return nil, fmt.Errorf("parse cf.conf: %w", err)
	}
	cfg.Handle        = kv["handle"]
	cfg.APIKey        = kv["api_key"]
	cfg.APISecret     = kv["api_secret"]
	cfg.DefaultLang   = kv["default_lang"]
	cfg.DefaultLangID = kv["default_lang_id"]
	return cfg, nil
}

func (g *GlobalConfig) Save() error {
	if g.path == "" {
		path, err := GlobalConfigPath()
		if err != nil {
			return err
		}
		g.path = path
	}
	lines := []string{
		"# cfr global config — edit manually or run `cfr setup`",
		"#",
		"# CF handle",
		"handle=" + g.Handle,
		"",
		"# API credentials (https://codeforces.com/settings/api)",
		"api_key=" + g.APIKey,
		"api_secret=" + g.APISecret,
		"",
		"# Default language for cfr enter (cpp | java | python | go | rust)",
		"default_lang=" + g.DefaultLang,
		"# CF programTypeId — run `cfr lang-ids` for the table",
		"default_lang_id=" + g.DefaultLangID,
	}
	return os.WriteFile(g.path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func (g *GlobalConfig) IsConfigured() bool {
	return g.Handle != ""
}

// ── Per-contest config ────────────────────────────────────────────────────────

func ContestConfigPath(rootDir, contestID string) string {
	return filepath.Join(rootDir, "contest_"+contestID+".conf")
}

func LoadContestConfig(rootDir, contestID string) (*ContestConfig, error) {
	path := ContestConfigPath(rootDir, contestID)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	kv, err := parseINI(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg := &ContestConfig{
		ContestID:   kv["contest_id"],
		ContestName: kv["contest_name"],
		RootDir:     kv["root_dir"],
		Lang:        kv["lang"],
		LangID:      kv["lang_id"],
		StartTime:   kv["start_time"],
		EnteredAt:   kv["entered_at"],
		path:        path,
	}
	for _, idx := range strings.Split(kv["problems"], ",") {
		if s := strings.TrimSpace(idx); s != "" {
			cfg.Problems = append(cfg.Problems, s)
		}
	}
	return cfg, nil
}

// FindContestConfig walks up from cwd looking for a contest_*.conf file.
// Works from the contest root or any problem subdirectory.
func FindContestConfig() (*ContestConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dir := cwd
	for {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if strings.HasPrefix(n, "contest_") && strings.HasSuffix(n, ".conf") {
				id := strings.TrimSuffix(strings.TrimPrefix(n, "contest_"), ".conf")
				return LoadContestConfig(dir, id)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, fmt.Errorf("no contest_*.conf found in %s or any parent", cwd)
}

func (cc *ContestConfig) Save() error {
	if cc.path == "" {
		if cc.RootDir == "" || cc.ContestID == "" {
			return fmt.Errorf("contest config: root_dir and contest_id are required")
		}
		cc.path = ContestConfigPath(cc.RootDir, cc.ContestID)
	}
	lines := []string{
		"# cfr contest config — auto-generated by `cfr enter`",
		"contest_id="   + cc.ContestID,
		"contest_name=" + cc.ContestName,
		"root_dir="     + cc.RootDir,
		"problems="     + strings.Join(cc.Problems, ","),
		"lang="         + cc.Lang,
		"lang_id="      + cc.LangID,
		"start_time="   + cc.StartTime,
		"entered_at="   + cc.EnteredAt,
	}
	return os.WriteFile(cc.path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// ── INI parser ────────────────────────────────────────────────────────────────

func parseINI(r io.Reader) (map[string]string, error) {
	kv := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		kv[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return kv, sc.Err()
}

// ── Language tables ───────────────────────────────────────────────────────────

// CFLangID maps a normalised language name to CF's programTypeId.
// IDs verified on codeforces.com/contest/<id>/submit (2025).
var CFLangID = map[string]string{
	"cpp":     "89", // C++23 (GCC 14-64, msys2)
	"cpp17":   "54", // C++17 (GCC 7-32)
	"cpp20":   "73", // C++20 (GCC 11-64)
	"java":    "60", // Java 21 64bit
	"python":  "71", // PyPy 3-64
	"python3": "71", // PyPy 3-64
	"cpython": "31", // CPython 3.8
	"go":      "75", // Go 1.22.6
	"rust":    "49", // Rust 2021
}

// langToExt maps a normalised language name to its source file extension.
var langToExt = map[string]string{
	"cpp":    ".cpp",
	"java":   ".java",
	"python": ".py",
	"go":     ".go",
	"rust":   ".rs",
}

// normalizeLang canonicalises user-supplied language strings.
func normalizeLang(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cpp", "c++", "cpp17", "cpp20", "cpp23":
		return "cpp"
	case "java", "java21":
		return "java"
	case "python", "python3", "py", "pypy", "pypy3":
		return "python"
	case "go", "golang":
		return "go"
	case "rust", "rs":
		return "rust"
	default:
		return "cpp"
	}
}

func solutionExtension(lang string) string {
	if ext := langToExt[normalizeLang(lang)]; ext != "" {
		return ext
	}
	return ".cpp"
}

func PrintLangIDs() {
	fmt.Println("\n┌─ CF language IDs  (programTypeId)")
	fmt.Printf("│  %-14s  %-6s  %s\n", "Key", "CF ID", "Description")
	fmt.Printf("│  %-14s  %-6s  %s\n", "──────────────", "──────", "───────────────────────────────")
	for _, r := range []struct{ key, id, desc string }{
		{"cpp",     "89", "C++23 (GCC 14-64, msys2)  ← default"},
		{"cpp20",   "73", "C++20 (GCC 11-64)"},
		{"cpp17",   "54", "C++17 (GCC 7-32)"},
		{"java",    "60", "Java 21 64bit"},
		{"python",  "71", "PyPy 3-64"},
		{"cpython", "31", "Python 3.8 (CPython)"},
		{"go",      "75", "Go 1.22.6"},
		{"rust",    "49", "Rust 2021"},
	} {
		fmt.Printf("│  %-14s  %-6s  %s\n", r.key, r.id, r.desc)
	}
	fmt.Println("│")
	fmt.Println("│  Set in cf.conf:  default_lang=cpp  default_lang_id=89")
	fmt.Println("└────────────────────────────────────────────────────────────────────────────")
}