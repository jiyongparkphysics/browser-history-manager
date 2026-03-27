package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSanitizePath_EmptyPath(t *testing.T) {
	_, err := sanitizePath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestSanitizePath_ResolvesAbsolute(t *testing.T) {
	result, err := sanitizePath("somefile.db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}

func TestSanitizePath_CleansTraversal(t *testing.T) {
	result, err := sanitizePath("foo/../bar/baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The ".." should be resolved away
	if strings.Contains(result, "..") {
		t.Errorf("path still contains traversal: %s", result)
	}
}

func TestValidateDBPath_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := validateDBPath(dir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got: %v", err)
	}
}

func TestValidateDBPath_RejectsNonexistent(t *testing.T) {
	_, err := validateDBPath("/nonexistent/path/to/db.sqlite")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestValidateDBPath_AcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "test.db")
	if err := os.WriteFile(dbFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := validateDBPath(dbFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}

func TestValidateOutputPath_RejectsTraversal(t *testing.T) {
	_, err := validateOutputPath("../../etc/evil.csv")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected escapes error, got: %v", err)
	}
}

func TestValidateOutputPath_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := validateOutputPath(dir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestValidateOutputPath_AcceptsLocalFile(t *testing.T) {
	// A simple filename in CWD should be accepted
	result, err := validateOutputPath("output.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}

func TestValidateOutputPath_AcceptsSubdirectoryFile(t *testing.T) {
	result, err := validateOutputPath("subdir/output.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}

func TestValidateFilterPath_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := validateFilterPath(dir)
	if err == nil {
		t.Fatal("expected error for directory path")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got: %v", err)
	}
}

func TestValidateFilterPath_AcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	filterFile := filepath.Join(dir, "filters.txt")
	if err := os.WriteFile(filterFile, []byte("google.com\nyoutube.com\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := validateFilterPath(filterFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %s", result)
	}
}

func TestValidateFilterPath_RejectsNonexistent(t *testing.T) {
	_, err := validateFilterPath("/nonexistent/filters.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestSanitizePath_RejectsNullByte(t *testing.T) {
	_, err := sanitizePath("some\x00path.db")
	if err == nil {
		t.Fatal("expected error for null byte in path")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected null byte error, got: %v", err)
	}
}

func TestSanitizePath_RejectsControlCharacters(t *testing.T) {
	for _, c := range []byte{0x01, 0x07, 0x0A, 0x1F} {
		path := "test" + string(c) + "file.db"
		_, err := sanitizePath(path)
		if err == nil {
			t.Errorf("expected error for control char 0x%02x in path", c)
		}
	}
}

func TestValidateDBPath_RejectsNullByte(t *testing.T) {
	_, err := validateDBPath("History\x00.db")
	if err == nil {
		t.Fatal("expected error for null byte in database path")
	}
}

func TestValidateOutputPath_RejectsNullByte(t *testing.T) {
	_, err := validateOutputPath("output\x00.csv")
	if err == nil {
		t.Fatal("expected error for null byte in output path")
	}
}

func TestValidateOutputPath_RejectsEmptyPath(t *testing.T) {
	_, err := validateOutputPath("")
	if err == nil {
		t.Fatal("expected error for empty output path")
	}
}

func TestValidateDBPath_RejectsEmptyPath(t *testing.T) {
	_, err := validateDBPath("")
	if err == nil {
		t.Fatal("expected error for empty database path")
	}
}

// --- validateSearchInput tests ---

func TestValidateSearchInput_Empty(t *testing.T) {
	if err := validateSearchInput(""); err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
}

func TestValidateSearchInput_ValidKeywords(t *testing.T) {
	valid := []string{
		"google.com",
		"foo, bar, baz",
		"*",
		"hello world",
		"café",
		"한국어",
		"foo\tbar",           // tabs allowed
		"line1\nline2\nend",  // newlines allowed (filter files)
		"line\r\nend",        // carriage returns allowed
	}
	for _, v := range valid {
		if err := validateSearchInput(v); err != nil {
			t.Errorf("unexpected error for %q: %v", v, err)
		}
	}
}

func TestValidateSearchInput_RejectsNullByte(t *testing.T) {
	err := validateSearchInput("foo\x00bar")
	if err == nil {
		t.Fatal("expected error for null byte")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected null byte error, got: %v", err)
	}
}

func TestValidateSearchInput_RejectsControlChars(t *testing.T) {
	for _, c := range []byte{0x01, 0x07, 0x1F} {
		err := validateSearchInput("test" + string(c) + "value")
		if err == nil {
			t.Errorf("expected error for control char 0x%02x", c)
		}
		if err != nil && !strings.Contains(err.Error(), "control character") {
			t.Errorf("expected control character error for 0x%02x, got: %v", c, err)
		}
	}
}

func TestValidateSearchInput_AllowsWhitespaceControls(t *testing.T) {
	// Tab (0x09), LF (0x0A), CR (0x0D) should be allowed
	if err := validateSearchInput("a\tb\nc\rd"); err != nil {
		t.Fatalf("expected whitespace controls to be allowed: %v", err)
	}
}

// --- validateFilterKeywords tests ---

func TestValidateFilterKeywords_Valid(t *testing.T) {
	keywords := []string{"google.com", "youtube.com", "github.com"}
	if err := validateFilterKeywords(keywords); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFilterKeywords_TooMany(t *testing.T) {
	keywords := make([]string, maxFilterKeywords+1)
	for i := range keywords {
		keywords[i] = "keyword"
	}
	err := validateFilterKeywords(keywords)
	if err == nil {
		t.Fatal("expected error for too many keywords")
	}
	if !strings.Contains(err.Error(), "too many") {
		t.Errorf("expected 'too many' error, got: %v", err)
	}
}

func TestValidateFilterKeywords_TooLong(t *testing.T) {
	longKeyword := strings.Repeat("a", maxFilterKeywordLen+1)
	err := validateFilterKeywords([]string{longKeyword})
	if err == nil {
		t.Fatal("expected error for too-long keyword")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected 'too long' error, got: %v", err)
	}
}

func TestValidateFilterKeywords_ExactLimit(t *testing.T) {
	keywords := make([]string, maxFilterKeywords)
	for i := range keywords {
		keywords[i] = "k"
	}
	if err := validateFilterKeywords(keywords); err != nil {
		t.Fatalf("unexpected error at exact limit: %v", err)
	}
}

// --- symlink rejection tests ---

// TestValidateDBPath_RejectsSymlink verifies that validateDBPath rejects a path
// that is a symbolic link, even if the link target is a valid regular file.
func TestValidateDBPath_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.db")
	if err := os.WriteFile(realFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	linkFile := filepath.Join(dir, "link.db")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := validateDBPath(linkFile)
	if err == nil {
		t.Fatal("expected error for symlink database path")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected symbolic link error, got: %v", err)
	}
}

// TestValidateOutputPath_RejectsSymlink verifies that validateOutputPath rejects
// a path that already exists as a symbolic link, preventing output from being
// redirected through an attacker-controlled symlink.
func TestValidateOutputPath_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	// Create a symlink within CWD pointing to a temp file outside CWD.
	dir := t.TempDir()
	target := filepath.Join(dir, "target.csv")
	if err := os.WriteFile(target, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	linkName := filepath.Join(cwd, "test_symlink_output.csv")
	// Clean up after test regardless of outcome.
	t.Cleanup(func() { os.Remove(linkName) })

	if err := os.Symlink(target, linkName); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err = validateOutputPath("test_symlink_output.csv")
	if err == nil {
		t.Fatal("expected error for symlink output path")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected symbolic link error, got: %v", err)
	}
}

// TestValidateFilterPath_RejectsSymlink verifies that validateFilterPath rejects
// a path that is a symbolic link.
func TestValidateFilterPath_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("google.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	linkFile := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := validateFilterPath(linkFile)
	if err == nil {
		t.Fatal("expected error for symlink filter path")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected symbolic link error, got: %v", err)
	}
}

// --- Windows reserved names test ---

// TestSanitizePath_WindowsReservedNames verifies that Windows reserved device
// names are rejected when running on Windows. Go 1.20+ converts bare reserved
// names to device paths inside filepath.Abs, so the check must run before Abs.
func TestSanitizePath_WindowsReservedNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows reserved name check only applies on Windows")
	}
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "LPT1"}
	for _, name := range reserved {
		// Bare name and name-with-extension must both be rejected.
		for _, candidate := range []string{name, name + ".txt", name + ".db"} {
			_, err := sanitizePath(candidate)
			if err == nil {
				t.Errorf("expected error for reserved name %q, got none", candidate)
				continue
			}
			// The error may say "reserved device name" (bare name caught before
			// Abs) or "Windows device path" (name-with-extension caught after Abs).
			if !strings.Contains(err.Error(), "reserved device name") &&
				!strings.Contains(err.Error(), "Windows device path") {
				t.Errorf("unexpected error for %q: %v", candidate, err)
			}
		}
	}
}

// TestSanitizePath_WindowsDevicePath verifies that explicit Windows device-path
// prefixes (\\.\) are rejected regardless of extension.
func TestSanitizePath_WindowsDevicePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows device path check only applies on Windows")
	}
	devicePaths := []string{`\\.\CON`, `\\.\NUL`, `\\?\Volume{abc}`}
	for _, p := range devicePaths {
		_, err := sanitizePath(p)
		if err == nil {
			t.Errorf("expected error for device path %q, got none", p)
		}
	}
}

// --- validateLimit tests ---

// TestValidateLimit_EmptyStringReturnsDefault verifies that an empty string
// returns the default preview limit without error.
func TestValidateLimit_EmptyStringReturnsDefault(t *testing.T) {
	n, err := validateLimit("")
	if err != nil {
		t.Fatalf("unexpected error for empty string: %v", err)
	}
	if n != defaultPreviewLimit {
		t.Errorf("expected defaultPreviewLimit (%d), got %d", defaultPreviewLimit, n)
	}
}

// TestValidateLimit_ValidPositiveIntegers verifies that typical positive integer
// strings are accepted and converted correctly.
func TestValidateLimit_ValidPositiveIntegers(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"1", 1},
		{"10", 10},
		{"50", 50},
		{"100", 100},
		{"500", 500},
		{"9999", 9999},
		{"10000", maxPreviewLimit},
	}
	for _, tc := range cases {
		n, err := validateLimit(tc.input)
		if err != nil {
			t.Errorf("validateLimit(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if n != tc.want {
			t.Errorf("validateLimit(%q) = %d, want %d", tc.input, n, tc.want)
		}
	}
}

// TestValidateLimit_RejectsNonInteger verifies that non-numeric strings return
// a clear error message.
func TestValidateLimit_RejectsNonInteger(t *testing.T) {
	cases := []string{"abc", "1.5", "ten", "50x", "--50", "1e5", " ", "\t", "∞"}
	for _, s := range cases {
		_, err := validateLimit(s)
		if err == nil {
			t.Errorf("validateLimit(%q): expected error, got nil", s)
			continue
		}
		if !strings.Contains(err.Error(), "must be a positive integer") {
			t.Errorf("validateLimit(%q): expected 'must be a positive integer' in error, got: %v", s, err)
		}
	}
}

// TestValidateLimit_RejectsZero verifies that 0 is rejected with a clear
// "must be at least 1" message.
func TestValidateLimit_RejectsZero(t *testing.T) {
	_, err := validateLimit("0")
	if err == nil {
		t.Fatal("expected error for limit=0")
	}
	if !strings.Contains(err.Error(), "must be at least 1") {
		t.Errorf("expected 'must be at least 1' error, got: %v", err)
	}
}

// TestValidateLimit_RejectsNegative verifies that negative integers are rejected
// with a clear "must be at least 1" message.
func TestValidateLimit_RejectsNegative(t *testing.T) {
	cases := []string{"-1", "-50", "-10000"}
	for _, s := range cases {
		_, err := validateLimit(s)
		if err == nil {
			t.Errorf("validateLimit(%q): expected error for negative value", s)
			continue
		}
		if !strings.Contains(err.Error(), "must be at least 1") {
			t.Errorf("validateLimit(%q): expected 'must be at least 1' error, got: %v", s, err)
		}
	}
}

// TestValidateLimit_RejectsAboveMax verifies that values exceeding maxPreviewLimit
// are rejected with a clear upper-bound error message.
func TestValidateLimit_RejectsAboveMax(t *testing.T) {
	cases := []string{"10001", "99999", "1000000"}
	for _, s := range cases {
		_, err := validateLimit(s)
		if err == nil {
			t.Errorf("validateLimit(%q): expected error for value above max", s)
			continue
		}
		if !strings.Contains(err.Error(), "must be at most") {
			t.Errorf("validateLimit(%q): expected 'must be at most' error, got: %v", s, err)
		}
	}
}

// TestValidateLimit_AcceptsExactBoundaries verifies that exactly 1 and maxPreviewLimit
// are both accepted (inclusive bounds).
func TestValidateLimit_AcceptsExactBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"1", 1},
		{"10000", maxPreviewLimit},
	} {
		n, err := validateLimit(tc.input)
		if err != nil {
			t.Errorf("validateLimit(%q) at boundary: unexpected error: %v", tc.input, err)
			continue
		}
		if n != tc.want {
			t.Errorf("validateLimit(%q) = %d, want %d", tc.input, n, tc.want)
		}
	}
}

// --- validateProfileName tests ---

// TestValidateProfileName_ValidNames verifies that standard Chrome profile
// directory names are accepted by validateProfileName.
func TestValidateProfileName_ValidNames(t *testing.T) {
	valid := []string{
		"Default",
		"Profile 1",
		"Profile 2",
		"Profile 10",
		"Profile 100",
		"Guest Profile",
		"System Profile",
		"MyProfile",
		"my-profile",
		"my_profile",
		"A",
		"Z",
		"profile1",
	}
	for _, name := range valid {
		if err := validateProfileName(name); err != nil {
			t.Errorf("validateProfileName(%q): unexpected error: %v", name, err)
		}
	}
}

// TestValidateProfileName_RejectsEmpty verifies that an empty string is rejected.
func TestValidateProfileName_RejectsEmpty(t *testing.T) {
	err := validateProfileName("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected 'must not be empty' error, got: %v", err)
	}
}

// TestValidateProfileName_RejectsTooLong verifies that names exceeding
// maxProfileNameLen bytes are rejected.
func TestValidateProfileName_RejectsTooLong(t *testing.T) {
	long := strings.Repeat("a", maxProfileNameLen+1)
	err := validateProfileName(long)
	if err == nil {
		t.Fatal("expected error for too-long profile name")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("expected 'too long' error, got: %v", err)
	}
}

// TestValidateProfileName_AcceptsExactMaxLength verifies that a name of exactly
// maxProfileNameLen bytes is accepted.
func TestValidateProfileName_AcceptsExactMaxLength(t *testing.T) {
	exact := strings.Repeat("a", maxProfileNameLen)
	if err := validateProfileName(exact); err != nil {
		t.Errorf("expected no error at exact length, got: %v", err)
	}
}

// TestValidateProfileName_RejectsTraversalSequences verifies that path traversal
// characters are rejected; the profile name must never contain '/' or '\'.
func TestValidateProfileName_RejectsTraversalSequences(t *testing.T) {
	traversal := []string{
		"../evil",
		"Default/../etc/passwd",
		"foo/bar",
		`foo\bar`,
		"Profile\x00Null",
	}
	for _, name := range traversal {
		if err := validateProfileName(name); err == nil {
			t.Errorf("validateProfileName(%q): expected error for traversal, got nil", name)
		}
	}
}

// TestValidateProfileName_RejectsInvalidCharacters verifies that characters
// outside the allowed ASCII set (letters, digits, space, hyphen, underscore)
// are rejected.
func TestValidateProfileName_RejectsInvalidCharacters(t *testing.T) {
	invalid := []string{
		"Profile!",
		"Prof@ile",
		"Profile#1",
		"Profile$",
		"Profile%",
		"Profile^",
		"Profile&",
		"Profile*",
		"Profile(1)",
		"Profile=1",
		"Profil\x00e",  // null byte
		"Profil\x01e",  // control character
		"Profil\x1Fe",  // control character
		"Profilé",       // non-ASCII letter
		"프로파일",       // non-ASCII (Korean)
	}
	for _, name := range invalid {
		if err := validateProfileName(name); err == nil {
			t.Errorf("validateProfileName(%q): expected error, got nil", name)
		}
	}
}

// TestValidateProfileName_RejectsLeadingSpecialChar verifies that names
// starting with a space, hyphen, or underscore are rejected.
func TestValidateProfileName_RejectsLeadingSpecialChar(t *testing.T) {
	leadingSpecial := []string{
		" Profile",
		"-Profile",
		"_Profile",
	}
	for _, name := range leadingSpecial {
		err := validateProfileName(name)
		if err == nil {
			t.Errorf("validateProfileName(%q): expected error for leading special char, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "must start with a letter or digit") {
			t.Errorf("validateProfileName(%q): expected 'must start with a letter or digit' error, got: %v", name, err)
		}
	}
}

// --- sanitizeEnvPath tests ---

// TestSanitizeEnvPath_EmptyReturnsEmpty verifies that an empty environment
// variable value (the variable is unset or blank) is silently returned as an
// empty string rather than an error.
func TestSanitizeEnvPath_EmptyReturnsEmpty(t *testing.T) {
	if got := sanitizeEnvPath(""); got != "" {
		t.Errorf("sanitizeEnvPath(%q) = %q; want empty string", "", got)
	}
}

// TestSanitizeEnvPath_ValidAbsolutePathReturnsClean verifies that a normal
// absolute path is returned unchanged (already clean and absolute).
func TestSanitizeEnvPath_ValidAbsolutePathReturnsClean(t *testing.T) {
	dir := t.TempDir()
	got := sanitizeEnvPath(dir)
	if got == "" {
		t.Fatalf("sanitizeEnvPath(%q): expected non-empty result for valid path", dir)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("sanitizeEnvPath(%q) = %q: expected absolute path", dir, got)
	}
}

// TestSanitizeEnvPath_NullByteReturnsEmpty verifies that a path containing a
// null byte (a potential C-string truncation attack) is silently rejected and
// an empty string is returned. Callers treat an empty result as "env var unset".
func TestSanitizeEnvPath_NullByteReturnsEmpty(t *testing.T) {
	poisoned := "/home/user\x00evil"
	if got := sanitizeEnvPath(poisoned); got != "" {
		t.Errorf("sanitizeEnvPath(%q): expected empty string for null-byte path, got %q", poisoned, got)
	}
}

// TestSanitizeEnvPath_ControlCharReturnsEmpty verifies that a path containing
// control characters (ASCII 0x01–0x1F) is silently rejected.
func TestSanitizeEnvPath_ControlCharReturnsEmpty(t *testing.T) {
	for _, c := range []byte{0x01, 0x07, 0x0B, 0x1F} {
		poisoned := "/home/user" + string([]byte{c}) + "evil"
		if got := sanitizeEnvPath(poisoned); got != "" {
			t.Errorf("sanitizeEnvPath with ctrl 0x%02x: expected empty, got %q", c, got)
		}
	}
}

// TestSanitizeEnvPath_TraversalSequencesCleaned verifies that traversal
// sequences (../../) embedded in an environment variable value are cleaned
// away by filepath.Clean + filepath.Abs so the resulting base directory is
// a legitimate absolute path without ".." components.
func TestSanitizeEnvPath_TraversalSequencesCleaned(t *testing.T) {
	traversals := []string{
		"/home/user/../../tmp",
		"/home/user/../../../etc",
	}
	for _, raw := range traversals {
		got := sanitizeEnvPath(raw)
		if got == "" {
			// An error-level rejection is not expected for these inputs; they
			// contain no null bytes or control chars — just ".." components
			// that filepath.Clean resolves deterministically.
			t.Errorf("sanitizeEnvPath(%q): unexpected empty result", raw)
			continue
		}
		if !filepath.IsAbs(got) {
			t.Errorf("sanitizeEnvPath(%q) = %q: expected absolute path", raw, got)
		}
		if strings.Contains(got, "..") {
			t.Errorf("sanitizeEnvPath(%q) = %q: result still contains '..' component", raw, got)
		}
	}
}

// TestSanitizeEnvPath_RelativePathConvertsToAbsolute verifies that a relative
// path (unusual for env vars like HOME, but possible) is resolved to an
// absolute path rather than rejected.
func TestSanitizeEnvPath_RelativePathConvertsToAbsolute(t *testing.T) {
	// "some/relative/dir" is unusual for HOME but should not cause a crash.
	got := sanitizeEnvPath("some/relative/dir")
	if got == "" {
		t.Fatal("sanitizeEnvPath: unexpected empty result for relative path without invalid chars")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("sanitizeEnvPath returned non-absolute path %q", got)
	}
}
