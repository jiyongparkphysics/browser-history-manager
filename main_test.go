package main

import (
	"strings"
	"testing"
)

// --- parseFlags tests ---

func TestParseFlags_Empty(t *testing.T) {
	flags, boolFlags, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 0 || len(boolFlags) != 0 {
		t.Fatal("expected empty maps for nil args")
	}
}

func TestParseFlags_BoolFlag(t *testing.T) {
	_, boolFlags, err := parseFlags([]string{"--yes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !boolFlags["yes"] {
		t.Fatal("expected yes=true")
	}
}

func TestParseFlags_BoolFlagShort(t *testing.T) {
	_, boolFlags, err := parseFlags([]string{"-y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !boolFlags["yes"] {
		t.Fatal("expected yes=true from -y")
	}
}

func TestParseFlags_ValueFlag(t *testing.T) {
	flags, _, err := parseFlags([]string{"--include", "foo,bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags["include"] != "foo,bar" {
		t.Fatalf("expected 'foo,bar', got %q", flags["include"])
	}
}

func TestParseFlags_MultipleFlags(t *testing.T) {
	flags, boolFlags, err := parseFlags([]string{
		"--include", "test", "--exclude", "keep", "--browser", "chrome", "--yes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags["include"] != "test" || flags["exclude"] != "keep" || flags["browser"] != "chrome" {
		t.Fatalf("unexpected flags: %v", flags)
	}
	if !boolFlags["yes"] {
		t.Fatal("expected yes=true")
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	_, _, err := parseFlags([]string{"--unknown", "value"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected 'unknown flag' error, got: %v", err)
	}
}

func TestParseFlags_MissingValue(t *testing.T) {
	_, _, err := parseFlags([]string{"--include"})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("expected 'requires a value' error, got: %v", err)
	}
}

func TestParseFlags_FlagAsValue(t *testing.T) {
	_, _, err := parseFlags([]string{"--include", "--exclude"})
	if err == nil {
		t.Fatal("expected error when flag used as value")
	}
	if !strings.Contains(err.Error(), "missing its value") {
		t.Fatalf("expected 'missing its value' error, got: %v", err)
	}
}

func TestParseFlags_EmptyFlag(t *testing.T) {
	_, _, err := parseFlags([]string{"--"})
	if err == nil {
		t.Fatal("expected error for empty flag")
	}
}

func TestParseFlags_UnexpectedArg(t *testing.T) {
	_, _, err := parseFlags([]string{"foobar"})
	if err == nil {
		t.Fatal("expected error for unexpected argument")
	}
	if !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("expected 'unexpected argument' error, got: %v", err)
	}
}

// --- validateFlagValues tests ---

func TestValidateFlagValues_ValidBrowser(t *testing.T) {
	for _, b := range []string{"chrome", "chromium", "edge", "opera"} {
		err := validateFlagValues(map[string]string{"browser": b})
		if err != nil {
			t.Fatalf("expected valid browser %q, got error: %v", b, err)
		}
	}
}

func TestValidateFlagValues_InvalidBrowser(t *testing.T) {
	err := validateFlagValues(map[string]string{"browser": "firefox"})
	if err == nil {
		t.Fatal("expected error for invalid browser")
	}
	if !strings.Contains(err.Error(), "invalid --browser") {
		t.Fatalf("expected browser validation error, got: %v", err)
	}
}

func TestValidateFlagValues_ValidOutExtension(t *testing.T) {
	err := validateFlagValues(map[string]string{"out": "export.csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlagValues_OutNoExtension(t *testing.T) {
	// No extension is allowed (defaults to csv internally).
	err := validateFlagValues(map[string]string{"out": "myexport"})
	if err != nil {
		t.Fatalf("unexpected error for no extension: %v", err)
	}
}

func TestValidateFlagValues_InvalidOutExtension(t *testing.T) {
	err := validateFlagValues(map[string]string{"out": "output.json"})
	if err == nil {
		t.Fatal("expected error for non-csv extension")
	}
	if !strings.Contains(err.Error(), ".csv extension") {
		t.Fatalf("expected extension error, got: %v", err)
	}
}

func TestValidateFlagValues_TooLongValue(t *testing.T) {
	long := strings.Repeat("a", maxFlagValueLen+1)
	err := validateFlagValues(map[string]string{"include": long})
	if err == nil {
		t.Fatal("expected error for too-long value")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Fatalf("expected 'too long' error, got: %v", err)
	}
}

func TestValidateFlagValues_EmptyMap(t *testing.T) {
	err := validateFlagValues(map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error for empty flags: %v", err)
	}
}

func TestValidateFlagValues_BrowserCaseInsensitive(t *testing.T) {
	err := validateFlagValues(map[string]string{"browser": "Chrome"})
	if err != nil {
		t.Fatalf("expected case-insensitive browser match, got: %v", err)
	}
}

// --- --limit flag parsing and validation tests ---

// TestParseFlags_LimitFlag verifies that --limit is recognized and stored.
func TestParseFlags_LimitFlag(t *testing.T) {
	flags, _, err := parseFlags([]string{"--limit", "25"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags["limit"] != "25" {
		t.Fatalf("expected flags[\"limit\"]=\"25\", got %q", flags["limit"])
	}
}

// TestParseFlags_LimitMissingValue verifies that --limit without a value
// returns a clear "requires a value" error.
func TestParseFlags_LimitMissingValue(t *testing.T) {
	_, _, err := parseFlags([]string{"--limit"})
	if err == nil {
		t.Fatal("expected error for --limit with no value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Errorf("expected 'requires a value' error, got: %v", err)
	}
}

// TestValidateFlagValues_ValidLimit verifies that valid integer strings for
// --limit are accepted without error.
func TestValidateFlagValues_ValidLimit(t *testing.T) {
	cases := []string{"1", "10", "50", "100", "10000"}
	for _, v := range cases {
		err := validateFlagValues(map[string]string{"limit": v})
		if err != nil {
			t.Errorf("validateFlagValues(limit=%q): unexpected error: %v", v, err)
		}
	}
}

// TestValidateFlagValues_LimitZero verifies that --limit 0 is rejected.
func TestValidateFlagValues_LimitZero(t *testing.T) {
	err := validateFlagValues(map[string]string{"limit": "0"})
	if err == nil {
		t.Fatal("expected error for --limit 0")
	}
	if !strings.Contains(err.Error(), "must be at least 1") {
		t.Errorf("expected 'must be at least 1' error, got: %v", err)
	}
}

// TestValidateFlagValues_LimitNegative verifies that negative --limit values
// are rejected with a clear error message.
func TestValidateFlagValues_LimitNegative(t *testing.T) {
	err := validateFlagValues(map[string]string{"limit": "-5"})
	if err == nil {
		t.Fatal("expected error for --limit -5")
	}
	if !strings.Contains(err.Error(), "must be at least 1") {
		t.Errorf("expected 'must be at least 1' error, got: %v", err)
	}
}

// TestValidateFlagValues_LimitNonInteger verifies that non-integer --limit
// values are rejected with a clear error message.
func TestValidateFlagValues_LimitNonInteger(t *testing.T) {
	cases := []string{"abc", "1.5", "ten", "50x"}
	for _, v := range cases {
		err := validateFlagValues(map[string]string{"limit": v})
		if err == nil {
			t.Errorf("expected error for --limit %q, got nil", v)
			continue
		}
		if !strings.Contains(err.Error(), "must be a positive integer") {
			t.Errorf("validateFlagValues(limit=%q): expected 'must be a positive integer' error, got: %v", v, err)
		}
	}
}

// TestValidateFlagValues_LimitExceedsMax verifies that --limit values above
// maxPreviewLimit are rejected.
func TestValidateFlagValues_LimitExceedsMax(t *testing.T) {
	err := validateFlagValues(map[string]string{"limit": "10001"})
	if err == nil {
		t.Fatal("expected error for --limit 10001")
	}
	if !strings.Contains(err.Error(), "must be at most") {
		t.Errorf("expected 'must be at most' error, got: %v", err)
	}
}

// TestValidateFlagValues_LimitAbsentIsValid verifies that omitting --limit
// entirely (no key) does not trigger validation errors.
func TestValidateFlagValues_LimitAbsentIsValid(t *testing.T) {
	err := validateFlagValues(map[string]string{"include": "google.com"})
	if err != nil {
		t.Fatalf("unexpected error when --limit is absent: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Search-input validation inside validateFlagValues (--match / --protect)
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateFlagValues_MatchNullByteRejected verifies that a null byte in
// the --match value is caught at flag-validation time, before any command
// execution or loadFilters call.
func TestValidateFlagValues_MatchNullByteRejected(t *testing.T) {
	err := validateFlagValues(map[string]string{"include": "google.com\x00evil"})
	if err == nil {
		t.Fatal("expected error for null byte in --match value")
	}
	if !strings.Contains(err.Error(), "--include") {
		t.Errorf("expected error to mention '--match', got: %v", err)
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

// TestValidateFlagValues_ProtectNullByteRejected verifies that a null byte in
// the --protect value is caught at flag-validation time.
func TestValidateFlagValues_ProtectNullByteRejected(t *testing.T) {
	err := validateFlagValues(map[string]string{"exclude": "safe\x00injection"})
	if err == nil {
		t.Fatal("expected error for null byte in --protect value")
	}
	if !strings.Contains(err.Error(), "--exclude") {
		t.Errorf("expected error to mention '--protect', got: %v", err)
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

// TestValidateFlagValues_MatchControlCharRejected verifies that shell-injection-
// prone control characters (0x01–0x08, 0x0B, 0x0C, 0x0E–0x1F) in --match are
// caught at flag-validation time. These characters can cause unexpected
// behaviour in terminal emulators and C-string syscalls.
func TestValidateFlagValues_MatchControlCharRejected(t *testing.T) {
	shellDangerousControls := []byte{
		0x01, // SOH – terminal control
		0x07, // BEL – terminal bell
		0x08, // BS  – terminal backspace
		0x0B, // VT  – vertical tab
		0x0C, // FF  – form feed
		0x0E, // SO  – shift-out (terminal mode switch)
		0x1B, // ESC – terminal escape sequences
		0x1F, // US  – unit separator
	}
	for _, c := range shellDangerousControls {
		value := "search\x00" // reuse loop structure with varying char
		value = "search" + string([]byte{c}) + "query"
		err := validateFlagValues(map[string]string{"include": value})
		if err == nil {
			t.Errorf("expected error for control char 0x%02x in --match, got nil", c)
			continue
		}
		if !strings.Contains(err.Error(), "--include") {
			t.Errorf("0x%02x: expected '--match' in error, got: %v", c, err)
		}
	}
}

// TestValidateFlagValues_ProtectControlCharRejected verifies that disallowed
// control characters in --protect are caught at flag-validation time.
func TestValidateFlagValues_ProtectControlCharRejected(t *testing.T) {
	for _, c := range []byte{0x01, 0x07, 0x1F} {
		value := "protect" + string([]byte{c}) + "value"
		err := validateFlagValues(map[string]string{"exclude": value})
		if err == nil {
			t.Errorf("expected error for control char 0x%02x in --protect, got nil", c)
			continue
		}
		if !strings.Contains(err.Error(), "--exclude") {
			t.Errorf("0x%02x: expected '--protect' in error, got: %v", c, err)
		}
	}
}

// TestValidateFlagValues_MatchAllowsValidSearchTerms verifies that typical and
// edge-case search query strings (including URL-safe special characters, Unicode,
// wildcards, and comma-separated lists) are accepted without error.
func TestValidateFlagValues_MatchAllowsValidSearchTerms(t *testing.T) {
	validMatches := []string{
		"google.com",
		"google.com,youtube.com,github.com",
		"*",
		"https://example.com/path?query=foo&bar=baz",
		"hello world",
		"café résumé",
		"한국어 검색",
		"foo\tbar",           // tab is allowed
		"line1\nline2",       // newline allowed (filter files)
		"url with $pecial & chars",
		"search; query | pipe",
		"(parentheses) and [brackets]",
		"'quoted' and \"double-quoted\"",
		"back`tick`",
		"gt>lt< and bang!",
		"hash#mark and percent%20",
		"at@sign and caret^",
	}
	for _, v := range validMatches {
		err := validateFlagValues(map[string]string{"include": v})
		if err != nil {
			t.Errorf("validateFlagValues(match=%q) unexpected error: %v", v, err)
		}
	}
}

// TestValidateFlagValues_ProtectAllowsValidSearchTerms mirrors the match test
// for the --protect flag.
func TestValidateFlagValues_ProtectAllowsValidSearchTerms(t *testing.T) {
	validProtects := []string{
		"bank.example.com",
		"private,confidential",
		"mailto:user@example.com",
		"path/with/slashes",
		"(grouping) [brackets] {braces}",
	}
	for _, v := range validProtects {
		err := validateFlagValues(map[string]string{"exclude": v})
		if err != nil {
			t.Errorf("validateFlagValues(protect=%q) unexpected error: %v", v, err)
		}
	}
}

// TestValidateFlagValues_BothMatchAndProtectValidated verifies that when both
// --match and --protect are provided, both are validated and the first invalid
// one causes rejection.
func TestValidateFlagValues_BothMatchAndProtectValidated(t *testing.T) {
	// Valid match, invalid protect (null byte)
	err := validateFlagValues(map[string]string{
		"include":   "google.com",
		"exclude": "safe\x00poisoned",
	})
	if err == nil {
		t.Fatal("expected error when --protect contains null byte")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}

	// Invalid match (control char), no protect
	err = validateFlagValues(map[string]string{
		"include": "google\x07com",
	})
	if err == nil {
		t.Fatal("expected error when --match contains control char")
	}
	if !strings.Contains(err.Error(), "control character") {
		t.Errorf("expected 'control character' in error, got: %v", err)
	}
}

// TestValidateFlagValues_MatchAllowsWhitespaceControls verifies that the
// permitted whitespace control characters (tab 0x09, LF 0x0A, CR 0x0D) are
// accepted in search queries, since they may appear in multi-line filter files.
func TestValidateFlagValues_MatchAllowsWhitespaceControls(t *testing.T) {
	allowedControls := []struct {
		name  string
		value string
	}{
		{"tab", "foo\tbar"},
		{"LF", "line1\nline2"},
		{"CR", "line\rend"},
		{"CRLF", "line\r\nend"},
	}
	for _, tc := range allowedControls {
		err := validateFlagValues(map[string]string{"include": tc.value})
		if err != nil {
			t.Errorf("validateFlagValues(match with %s) unexpected error: %v", tc.name, err)
		}
	}
}

// TestValidateFlagValues_MatchLengthLimit verifies that --match values exceeding
// maxFlagValueLen are rejected. This is the outermost length guard, applied
// before any per-keyword validation.
func TestValidateFlagValues_MatchLengthLimit(t *testing.T) {
	// Exactly at limit: should be accepted
	atLimit := strings.Repeat("a", maxFlagValueLen)
	if err := validateFlagValues(map[string]string{"include": atLimit}); err != nil {
		t.Errorf("expected match at maxFlagValueLen to be accepted, got: %v", err)
	}

	// One byte over limit: must be rejected
	overLimit := strings.Repeat("a", maxFlagValueLen+1)
	err := validateFlagValues(map[string]string{"include": overLimit})
	if err == nil {
		t.Fatal("expected error for --match value exceeding maxFlagValueLen")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected 'too long' in error, got: %v", err)
	}
}

// TestValidateFlagValues_ProtectLengthLimit mirrors the length-limit test for
// the --protect flag.
func TestValidateFlagValues_ProtectLengthLimit(t *testing.T) {
	overLimit := strings.Repeat("b", maxFlagValueLen+1)
	err := validateFlagValues(map[string]string{"exclude": overLimit})
	if err == nil {
		t.Fatal("expected error for --protect value exceeding maxFlagValueLen")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected 'too long' in error, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Browser flag character allowlist
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateBrowserName_RejectsShellMetachars verifies that the browser-name
// allowlist implicitly rejects all shell-injection-prone characters. Since the
// allowlist contains only lowercase alphabetic browser names, any string
// containing special characters or spaces must be rejected.
func TestValidateBrowserName_RejectsShellMetachars(t *testing.T) {
	shellPayloads := []struct {
		name  string
		input string
	}{
		{"semicolon", "chrome;rm -rf /"},
		{"pipe", "edge|cat /etc/passwd"},
		{"ampersand", "brave&whoami"},
		{"backtick", "chrome`id`"},
		{"dollar", "chrome$(id)"},
		{"single quote", "chrome'"},
		{"double quote", `chrome"`},
		{"gt redirect", "chrome>out.txt"},
		{"lt redirect", "chrome</etc/passwd"},
		{"backslash escape", `chrome\n`},
		{"space padding", "chrome "},
		{"null byte", "chrome\x00"},
		{"newline", "chrome\n"},
		{"tab", "chrome\t"},
		{"carriage return", "chrome\r"},
		{"parentheses", "chrome()"},
		{"curly braces", "chrome{}"},
	}
	for _, tc := range shellPayloads {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			err := validateBrowserName(tc.input)
			if err == nil {
				t.Errorf("validateBrowserName(%q) should reject shell metachar payload", tc.input)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Early path validation in validateFlagValues (--db / --out)
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateFlagValues_DBNullByteRejected verifies that a null byte embedded
// in the --db flag value is caught at flag-validation time, before resolveDBPath
// or any SQL open is attempted. Null bytes can truncate C-string paths and must
// be rejected as the first line of defence.
func TestValidateFlagValues_DBNullByteRejected(t *testing.T) {
	err := validateFlagValues(map[string]string{"db": "History\x00.db"})
	if err == nil {
		t.Fatal("expected error for null byte in --db value")
	}
	if !strings.Contains(err.Error(), "--db") {
		t.Errorf("expected error to mention '--db', got: %v", err)
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

// TestValidateFlagValues_DBControlCharRejected verifies that control characters
// (0x01–0x1F) embedded in the --db path are caught at flag-validation time.
func TestValidateFlagValues_DBControlCharRejected(t *testing.T) {
	for _, c := range []byte{0x01, 0x07, 0x1B, 0x1F} {
		value := "History" + string([]byte{c}) + ".db"
		err := validateFlagValues(map[string]string{"db": value})
		if err == nil {
			t.Errorf("expected error for control char 0x%02x in --db, got nil", c)
			continue
		}
		if !strings.Contains(err.Error(), "--db") {
			t.Errorf("0x%02x: expected '--db' in error, got: %v", c, err)
		}
	}
}

// TestValidateFlagValues_DBValidPath verifies that a syntactically valid path
// (even if it does not exist on disk) passes validateFlagValues without error.
// Existence checks happen later in resolveDBPath.
func TestValidateFlagValues_DBValidPath(t *testing.T) {
	// A simple file name is well-formed: no null bytes or control chars.
	err := validateFlagValues(map[string]string{"db": "History"})
	if err != nil {
		t.Fatalf("expected valid db path to pass validateFlagValues, got: %v", err)
	}
}

// TestValidateFlagValues_OutNullByteRejected verifies that a null byte embedded
// in the --out flag value is caught at flag-validation time.
func TestValidateFlagValues_OutNullByteRejected(t *testing.T) {
	err := validateFlagValues(map[string]string{"out": "output\x00.csv"})
	if err == nil {
		t.Fatal("expected error for null byte in --out value")
	}
	if !strings.Contains(err.Error(), "--out") {
		t.Errorf("expected error to mention '--out', got: %v", err)
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

// TestValidateFlagValues_OutTraversalRejected verifies that path traversal
// sequences in the --out flag value are caught at flag-validation time,
// before cmdExport is invoked. Traversal paths must not escape the CWD.
func TestValidateFlagValues_OutTraversalRejected(t *testing.T) {
	traversals := []string{
		"../../etc/evil.csv",
		"../output.csv",
		"../../../../tmp/stolen.csv",
		"subdir/../../out.csv",
	}
	for _, p := range traversals {
		err := validateFlagValues(map[string]string{"out": p})
		if err == nil {
			t.Errorf("expected error for traversal --out path %q, got nil", p)
			continue
		}
		if !strings.Contains(err.Error(), "--out") {
			t.Errorf("path %q: expected error to mention '--out', got: %v", p, err)
		}
		// The error should indicate either traversal escape or path invalidity.
		if !strings.Contains(err.Error(), "escapes") && !strings.Contains(err.Error(), "invalid") {
			t.Errorf("path %q: expected 'escapes' or 'invalid' in error, got: %v", p, err)
		}
	}
}

// TestValidateFlagValues_OutControlCharRejected verifies that control characters
// in the --out path value are caught at flag-validation time.
func TestValidateFlagValues_OutControlCharRejected(t *testing.T) {
	value := "output\x01file.csv"
	err := validateFlagValues(map[string]string{"out": value})
	if err == nil {
		t.Fatal("expected error for control char in --out value")
	}
	if !strings.Contains(err.Error(), "--out") {
		t.Errorf("expected error to mention '--out', got: %v", err)
	}
}

// TestValidateFlagValues_OutValidLocalPath verifies that a simple local CSV
// filename is accepted by validateFlagValues with no error. The path must
// resolve within the current working directory.
func TestValidateFlagValues_OutValidLocalPath(t *testing.T) {
	err := validateFlagValues(map[string]string{"out": "report.csv"})
	if err != nil {
		t.Fatalf("expected valid local out path to be accepted, got: %v", err)
	}
}

// TestValidateFlagValues_OutSubdirLocalPath verifies that a subdirectory
// path within the CWD is accepted (even if the subdirectory doesn't exist,
// since we only check the path itself, not directory creation).
func TestValidateFlagValues_OutSubdirLocalPath(t *testing.T) {
	// subdir/report.csv does not escape CWD, so it should be accepted
	// regardless of whether the subdirectory exists on disk.
	err := validateFlagValues(map[string]string{"out": "subdir/report.csv"})
	if err != nil {
		t.Fatalf("expected subdirectory out path within CWD to be accepted, got: %v", err)
	}
}

// --- --profile flag parsing and validation tests ---

// TestParseFlags_ProfileFlag verifies that --profile is recognized and stored.
func TestParseFlags_ProfileFlag(t *testing.T) {
	flags, _, err := parseFlags([]string{"--profile", "Profile 1"})
	if err != nil {
		t.Fatalf("parseFlags(--profile 'Profile 1'): unexpected error: %v", err)
	}
	if flags["profile"] != "Profile 1" {
		t.Errorf("expected profile='Profile 1', got %q", flags["profile"])
	}
}

// TestParseFlags_ProfileMissingValue verifies that --profile without a value
// returns a descriptive error.
func TestParseFlags_ProfileMissingValue(t *testing.T) {
	_, _, err := parseFlags([]string{"--profile"})
	if err == nil {
		t.Fatal("expected error for --profile with no value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Errorf("expected 'requires a value' error, got: %v", err)
	}
}

// TestValidateFlagValues_ProfileValid verifies that standard Chrome profile names
// pass validateFlagValues without error.
func TestValidateFlagValues_ProfileValid(t *testing.T) {
	valid := []string{"Default", "Profile 1", "Profile 2", "Guest Profile", "MyProfile"}
	for _, name := range valid {
		if err := validateFlagValues(map[string]string{"profile": name}); err != nil {
			t.Errorf("validateFlagValues(profile=%q): unexpected error: %v", name, err)
		}
	}
}

// TestValidateFlagValues_ProfileInvalidChars verifies that profile names with
// path-traversal sequences or special characters are rejected.
func TestValidateFlagValues_ProfileInvalidChars(t *testing.T) {
	invalid := []string{
		"../etc/passwd",
		"Profile/1",
		`Profile\1`,
		"Profile\x00",
		"Profil\x01e",
		"Profile!",
		"프로파일",
	}
	for _, name := range invalid {
		err := validateFlagValues(map[string]string{"profile": name})
		if err == nil {
			t.Errorf("validateFlagValues(profile=%q): expected error, got nil", name)
		}
	}
}

// TestValidateFlagValues_ProfileLeadingSpecialChar verifies that profile names
// starting with a space, hyphen, or underscore are rejected.
func TestValidateFlagValues_ProfileLeadingSpecialChar(t *testing.T) {
	leading := []string{" Profile", "-Profile", "_Profile"}
	for _, name := range leading {
		err := validateFlagValues(map[string]string{"profile": name})
		if err == nil {
			t.Errorf("validateFlagValues(profile=%q): expected error for leading special char, got nil", name)
		}
	}
}

// TestValidateFlagValues_ProfileAbsentIsValid verifies that omitting --profile
// does not cause a validation error.
func TestValidateFlagValues_ProfileAbsentIsValid(t *testing.T) {
	if err := validateFlagValues(map[string]string{}); err != nil {
		t.Fatalf("expected no error when --profile is absent: %v", err)
	}
}

// TestParseFlags_ShortNFlag verifies that -n is accepted as a shorthand for
// --limit and stores the value under the "limit" key.
func TestParseFlags_ShortNFlag(t *testing.T) {
	flags, _, err := parseFlags([]string{"-n", "25"})
	if err != nil {
		t.Fatalf("parseFlags(-n 25): unexpected error: %v", err)
	}
	if flags["limit"] != "25" {
		t.Errorf("expected limit='25' from -n 25, got %q", flags["limit"])
	}
}

// TestParseFlags_ShortNFlagMissingValue verifies that -n without a value returns
// a descriptive error.
func TestParseFlags_ShortNFlagMissingValue(t *testing.T) {
	_, _, err := parseFlags([]string{"-n"})
	if err == nil {
		t.Fatal("expected error for -n with no value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Errorf("expected 'requires a value' error, got: %v", err)
	}
}

// TestParseFlags_ShortNFlagAndLongLimit verifies that -n and --limit are
// equivalent: both store under the "limit" key and produce valid results.
func TestParseFlags_ShortNFlagAndLongLimit(t *testing.T) {
	for _, args := range [][]string{
		{"-n", "10"},
		{"--limit", "10"},
	} {
		flags, _, err := parseFlags(args)
		if err != nil {
			t.Fatalf("parseFlags(%v): unexpected error: %v", args, err)
		}
		if flags["limit"] != "10" {
			t.Errorf("parseFlags(%v): expected limit='10', got %q", args, flags["limit"])
		}
	}
}

// TestValidateFlagValues_ShortNFlagValidation verifies that values stored via
// -n are subject to the same validation as --limit.
func TestValidateFlagValues_ShortNFlagValidation(t *testing.T) {
	// Invalid value via -n shorthand must be rejected.
	if err := validateFlagValues(map[string]string{"limit": "0"}); err == nil {
		t.Error("expected error for limit=0 (via -n shorthand), got nil")
	}
	if err := validateFlagValues(map[string]string{"limit": "abc"}); err == nil {
		t.Error("expected error for limit=abc (via -n shorthand), got nil")
	}
	// Valid value must be accepted.
	if err := validateFlagValues(map[string]string{"limit": "42"}); err != nil {
		t.Errorf("expected no error for limit=42, got: %v", err)
	}
}

// TestValidateBrowserName_AllowlistOnlyAcceptsExactNames verifies the complete
// set of valid browser names and that no variant with extra characters is accepted.
func TestValidateBrowserName_AllowlistOnlyAcceptsExactNames(t *testing.T) {
	valid := []string{"brave", "chrome", "chromium", "edge", "opera"}
	for _, name := range valid {
		if err := validateBrowserName(name); err != nil {
			t.Errorf("validateBrowserName(%q) should be valid, got: %v", name, err)
		}
		// Upper-cased version should also be accepted (case-insensitive)
		if err := validateBrowserName(strings.ToUpper(name)); err != nil {
			t.Errorf("validateBrowserName(%q uppercase) should be valid, got: %v", name, err)
		}
	}

	// Near-matches with appended special chars must be rejected
	nearMatches := []string{
		"chrome!", "chrome.", "chrome-stable", "chrome2",
		"_edge", "brave-beta", "vivaldi", "opera-browser", " opera",
	}
	for _, name := range nearMatches {
		if err := validateBrowserName(name); err == nil {
			t.Errorf("validateBrowserName(%q) should be invalid (not in allowlist)", name)
		}
	}
}
