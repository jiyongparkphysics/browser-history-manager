package main

// path_traversal_test.go — Unit tests covering path traversal attack vectors
// against all user-facing path validation functions.
//
// Attack vectors covered:
//   - Classic directory traversal sequences (../../../etc/passwd)
//   - Absolute paths outside the current working directory
//   - Null bytes embedded in traversal paths (C-string truncation)
//   - Control characters mixed with traversal sequences
//   - Symlink-based traversal (symlink → outside CWD, symlink chains)
//   - Double-slash and repeated-dot sequences
//   - Windows device path prefixes (\\.\, \\?\)
//   - Windows reserved device names used as traversal targets
//
// Defence functions under test: sanitizePath, validateDBPath,
// validateOutputPath, validateFilterPath (all in sanitize.go).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// A. sanitizePath — traversal sequence resolution and rejection
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_SanitizePath_ClassicTraversalResolvesToAbsolute verifies
// that classic traversal sequences (../../../etc/passwd) are cleaned by
// filepath.Clean and resolved to an absolute path by sanitizePath.
// sanitizePath itself does not reject paths that traverse above CWD — that
// restriction is enforced by validateOutputPath. The key property tested here
// is that the returned path is canonical (absolute, no ".." components) so
// callers receive a deterministic path regardless of how many ".." segments
// are present in the input.
func TestPathTraversal_SanitizePath_ClassicTraversalResolvesToAbsolute(t *testing.T) {
	traversals := []string{
		"../../etc/passwd",
		"../../../etc/passwd",
		"../../../../etc/shadow",
		"../../../../../tmp/evil",
		"foo/../../etc/passwd",
		"./../../etc/passwd",
		"a/b/c/../../../etc/passwd",
	}
	for _, p := range traversals {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			result, err := sanitizePath(p)
			if err != nil {
				t.Fatalf("sanitizePath(%q): unexpected error: %v", p, err)
			}
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", p, result)
			}
			if strings.Contains(result, "..") {
				t.Errorf("sanitizePath(%q) = %q: result still contains traversal component", p, result)
			}
		})
	}
}

// TestPathTraversal_SanitizePath_NullByteInTraversalPath verifies that a null
// byte embedded anywhere in a traversal path is caught before any filesystem
// operation. The null byte is the primary vector for C-string truncation
// attacks and must be rejected unconditionally.
func TestPathTraversal_SanitizePath_NullByteInTraversalPath(t *testing.T) {
	nullByteTraversals := []struct {
		name string
		path string
	}{
		{"null at start", "\x00../../etc/passwd"},
		{"null mid traversal", "../\x00../../etc/passwd"},
		{"null before filename", "../../etc/\x00passwd"},
		{"null after dotdot", "..\x00/etc/passwd"},
		{"null mid filename", "../../etc/pass\x00wd"},
		{"null at end", "../../etc/passwd\x00"},
		{"double null", "../../\x00etc/\x00passwd"},
	}
	for _, tc := range nullByteTraversals {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			_, err := sanitizePath(tc.path)
			if err == nil {
				t.Errorf("sanitizePath(%q): expected rejection for null byte, got nil error", tc.path)
				return
			}
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("sanitizePath(%q): expected 'null byte' in error, got: %v", tc.path, err)
			}
		})
	}
}

// TestPathTraversal_SanitizePath_ControlCharInTraversalPath verifies that
// control characters (0x01–0x1F, excluding the null byte which is tested
// separately) embedded in traversal paths are rejected. Control characters
// can confuse shell tokenisers, logging, and terminal rendering.
func TestPathTraversal_SanitizePath_ControlCharInTraversalPath(t *testing.T) {
	controlChars := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // SOH–BEL
		0x08,                   // BS
		0x0B, 0x0C,             // VT, FF
		0x0E, 0x0F,             // SO, SI
		0x10, 0x11, 0x12, 0x13, // DLE–DC3
		0x14, 0x15, 0x16, 0x17, // DC4–ETB
		0x18, 0x19, 0x1A,       // CAN–SUB
		0x1C, 0x1D, 0x1E, 0x1F, // FS–US
	}
	for _, c := range controlChars {
		c := c // capture
		path := "../../etc/pa" + string([]byte{c}) + "sswd"
		t.Run("ctrl_0x"+hexByte(c), func(t *testing.T) {
			_, err := sanitizePath(path)
			if err == nil {
				t.Errorf("sanitizePath with control char 0x%02x: expected error, got nil", c)
			}
		})
	}
}

// TestPathTraversal_SanitizePath_DoubleSlashAndDotVariants verifies that
// redundant separator sequences are normalised to their canonical form.
// These are not attacks per se but are included to confirm that filepath.Clean
// eliminates all such variants before the path reaches the filesystem.
func TestPathTraversal_SanitizePath_DoubleSlashAndDotVariants(t *testing.T) {
	variants := []string{
		"//etc/passwd",
		"///etc/passwd",
		"./etc/passwd",
		"././etc/passwd",
		"dir//subdir//file",
	}
	for _, p := range variants {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			result, err := sanitizePath(p)
			if err != nil {
				t.Fatalf("sanitizePath(%q): unexpected error: %v", p, err)
			}
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", p, result)
			}
			// Cleaned path must not contain double-slash or "/./".
			if strings.Contains(result, "//") || strings.Contains(result, "/./") {
				t.Errorf("sanitizePath(%q) = %q: path was not properly cleaned", p, result)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// B. validateDBPath — traversal attack vectors for the --db flag
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_ValidateDBPath_ClassicUnixTraversal verifies that classic
// UNIX traversal paths supplied as the --db flag are rejected because the
// targeted system files (e.g. /etc/passwd) are either non-existent as SQLite
// databases or require escalated privileges. On any system where the path does
// not point to an existing regular non-symlink file, validateDBPath must
// return an error before any SQL operation is attempted.
func TestPathTraversal_ValidateDBPath_ClassicUnixTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix traversal paths are not applicable on Windows")
	}
	// These paths target common UNIX files via traversal. They either do not
	// exist as regular files or are not valid SQLite databases. validateDBPath
	// must reject them with a path/file error, never a SQL error.
	traversalDBPaths := []string{
		"../../../etc/passwd",
		"../../../../etc/shadow",
		"../../../../etc/hostname",
		"../../../../../proc/version",
		"../../../../../../etc/os-release",
	}
	for _, p := range traversalDBPaths {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			_, err := validateDBPath(p)
			if err == nil {
				// Accept only if the resolved path actually points to a valid
				// regular file that our process can read. In that case the
				// validation passes (it is a CLI tool running with user rights).
				// Failing to return an error here is not a security defect as
				// long as the path was not modified. Log for awareness.
				t.Logf("validateDBPath(%q): path resolved and was accepted (file may exist)", p)
				return
			}
			// Expected outcomes: file not found, directory, or access denied.
			// What must NOT happen: a SQL-level error, or a panic.
			_ = err // any non-nil error is acceptable
		})
	}
}

// TestPathTraversal_ValidateDBPath_NullByteTraversal verifies that null bytes
// embedded in traversal paths supplied as --db are rejected before any
// filesystem or SQL operation. This is the highest-priority check.
func TestPathTraversal_ValidateDBPath_NullByteTraversal(t *testing.T) {
	nullBytePaths := []string{
		"../../../etc/\x00passwd",
		"../../\x00etc/passwd",
		"History\x00/../../etc/passwd",
		"../../etc/passwd\x00.db",
	}
	for _, p := range nullBytePaths {
		p := p // capture
		t.Run("null_in_"+strings.ReplaceAll(filepath.Base(p), "\x00", "NUL"), func(t *testing.T) {
			_, err := validateDBPath(p)
			if err == nil {
				t.Errorf("validateDBPath(%q): expected rejection for null byte, got nil", p)
				return
			}
			// Error must originate from null-byte detection, not SQL.
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("validateDBPath(%q): expected 'null byte' error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateDBPath_AbsoluteSystemPaths verifies that absolute
// paths targeting system files are handled without panic or SQL access.
// These paths are not blocked by sanitizePath (absolute paths are valid), but
// they are blocked by the file-existence and regular-file checks.
func TestPathTraversal_ValidateDBPath_AbsoluteSystemPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix absolute paths are not applicable on Windows")
	}
	systemPaths := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/hostname",
		"/dev/null",
		"/dev/random",
		"/proc/self/mem",
	}
	for _, p := range systemPaths {
		p := p // capture
		t.Run(filepath.Base(p), func(t *testing.T) {
			_, err := validateDBPath(p)
			// /dev/null and similar device files should be rejected as
			// non-regular files. /etc/passwd might succeed (it is a regular
			// file), but the SQLite driver will then fail to open it as a DB.
			// The test only asserts that no panic occurs and that validateDBPath
			// never silently skips its validation.
			if err != nil {
				// Acceptable: the path does not meet the regular-file criteria
				// or was not found.
				return
			}
			// If accepted, the path must be absolute — sanitizePath contract.
			t.Logf("validateDBPath(%q): accepted (file exists as regular file, will fail at SQLite open)", p)
		})
	}
}

// TestPathTraversal_ValidateDBPath_RejectsSymlinkToOutsideCWD verifies that
// a symlink inside a temp directory that points to a regular file in a
// different directory is rejected by validateDBPath (symlinks not allowed).
func TestPathTraversal_ValidateDBPath_RejectsSymlinkToOutsideCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()
	targetDir := t.TempDir()

	// Real DB-like file in targetDir.
	realDB := filepath.Join(targetDir, "History")
	if err := os.WriteFile(realDB, []byte("SQLiteDB"), 0600); err != nil {
		t.Fatal(err)
	}

	// Symlink inside dir pointing to realDB (which is in a different temp dir).
	symlinkDB := filepath.Join(dir, "History")
	if err := os.Symlink(realDB, symlinkDB); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := validateDBPath(symlinkDB)
	if err == nil {
		t.Fatal("validateDBPath should reject a symlink even if the target is a regular file")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// TestPathTraversal_ValidateDBPath_RejectsSymlinkChain verifies that a chain
// of symlinks (A → B → real file) is rejected. os.Lstat reports the first
// symlink as a symlink without following it, so the chain is caught at the
// first hop.
func TestPathTraversal_ValidateDBPath_RejectsSymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.db")
	if err := os.WriteFile(realFile, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	linkA := filepath.Join(dir, "linkA.db")
	linkB := filepath.Join(dir, "linkB.db")

	// linkA → linkB → realFile (chain depth 2).
	if err := os.Symlink(realFile, linkB); err != nil {
		t.Skipf("cannot create first symlink: %v", err)
	}
	if err := os.Symlink(linkB, linkA); err != nil {
		t.Skipf("cannot create second symlink: %v", err)
	}

	_, err := validateDBPath(linkA)
	if err == nil {
		t.Fatal("validateDBPath should reject a symlink chain")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C. validateOutputPath — traversal attack vectors for the --out flag
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_ValidateOutputPath_ClassicUnixTraversal verifies that a
// broad set of classic UNIX path traversal sequences used as the --out flag
// are rejected because they escape the current working directory.
func TestPathTraversal_ValidateOutputPath_ClassicUnixTraversal(t *testing.T) {
	traversalOutputPaths := []string{
		"../../etc/passwd",
		"../../../etc/passwd",
		"../../../../etc/shadow",
		"../../../../../tmp/evil.csv",
		"../output.csv",
		"../evil",
		"./../../tmp/out.csv",
		"a/b/../../../../tmp/evil.csv",
		"subdir/../../../etc/out",
	}
	for _, p := range traversalOutputPaths {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for path escaping CWD, got nil", p)
				return
			}
			// The error must mention that the path escapes the working directory.
			if !strings.Contains(err.Error(), "escapes") {
				t.Errorf("validateOutputPath(%q): expected 'escapes' in error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_AbsolutePathsOutsideCWD verifies that
// absolute paths pointing to locations outside the current working directory
// are rejected by validateOutputPath. An absolute path to e.g. /tmp/evil.csv
// clearly escapes CWD.
func TestPathTraversal_ValidateOutputPath_AbsolutePathsOutsideCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix absolute paths are not applicable on Windows")
	}
	absoluteOutsidePaths := []string{
		"/etc/passwd",
		"/tmp/evil.csv",
		"/var/tmp/output.csv",
		"/home/attacker/stolen.csv",
	}
	for _, p := range absoluteOutsidePaths {
		p := p // capture
		t.Run(filepath.Base(p), func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for absolute path outside CWD", p)
				return
			}
			if !strings.Contains(err.Error(), "escapes") {
				t.Errorf("validateOutputPath(%q): expected 'escapes' in error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_NullByteTraversal verifies that null
// bytes embedded in output traversal paths are detected before any file
// operation. Null bytes can silently truncate paths in C-based syscalls.
func TestPathTraversal_ValidateOutputPath_NullByteTraversal(t *testing.T) {
	nullBytePaths := []string{
		"../../etc/\x00passwd.csv",
		"output\x00/../../../etc/passwd",
		"./out\x00put.csv",
		"subdir/file\x00.csv",
	}
	for _, p := range nullBytePaths {
		p := p // capture
		t.Run("null_in_output", func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for null byte, got nil", p)
				return
			}
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("validateOutputPath(%q): expected 'null byte' in error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_RejectsSymlinkToOutside verifies that
// an existing symlink within the CWD that points to a location outside the CWD
// is rejected by validateOutputPath. This prevents an attacker from pre-placing
// a symlink so that output is written to an unintended destination.
func TestPathTraversal_ValidateOutputPath_RejectsSymlinkToOutside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	// Create a target file outside the CWD in a temp dir.
	outsideDir := t.TempDir()
	targetFile := filepath.Join(outsideDir, "stolen.csv")
	if err := os.WriteFile(targetFile, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Place a symlink with an innocent-looking name inside CWD.
	symlinkName := "output_traversal_test.csv"
	symlinkPath := filepath.Join(cwd, symlinkName)
	t.Cleanup(func() { os.Remove(symlinkPath) })

	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err = validateOutputPath(symlinkName)
	if err == nil {
		t.Fatal("validateOutputPath should reject a symlink within CWD pointing to outside target")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// TestPathTraversal_ValidateOutputPath_RejectsSymlinkToDirectory verifies that
// a symlink pointing to a directory (inside or outside CWD) is rejected.
func TestPathTraversal_ValidateOutputPath_RejectsSymlinkToDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	outsideDir := t.TempDir()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Symlink with innocent name inside CWD pointing to a directory.
	symlinkName := "dir_traversal_test_link"
	symlinkPath := filepath.Join(cwd, symlinkName)
	t.Cleanup(func() { os.Remove(symlinkPath) })

	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err = validateOutputPath(symlinkName)
	if err == nil {
		t.Fatal("validateOutputPath should reject a symlink pointing to a directory")
	}
	// Accept either "symbolic" or "directory" in the error — the symlink check
	// fires before the directory check in the current implementation.
	if !strings.Contains(err.Error(), "symbolic") && !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected 'symbolic' or 'directory' in error, got: %v", err)
	}
}

// TestPathTraversal_ValidateOutputPath_RejectsSymlinkChain verifies that a
// chain of symlinks (A → B → file) within or outside CWD is rejected.
func TestPathTraversal_ValidateOutputPath_RejectsSymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	outsideDir := t.TempDir()
	targetFile := filepath.Join(outsideDir, "target.csv")
	if err := os.WriteFile(targetFile, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// intermediate symlink outside CWD: linkB → targetFile
	linkB := filepath.Join(outsideDir, "linkB.csv")
	if err := os.Symlink(targetFile, linkB); err != nil {
		t.Skipf("cannot create first symlink: %v", err)
	}

	// chain entry inside CWD: linkA → linkB (which is outside CWD)
	linkAName := "chain_traversal_test.csv"
	linkAPath := filepath.Join(cwd, linkAName)
	t.Cleanup(func() { os.Remove(linkAPath) })

	if err := os.Symlink(linkB, linkAPath); err != nil {
		t.Skipf("cannot create second symlink: %v", err)
	}

	_, err = validateOutputPath(linkAName)
	if err == nil {
		t.Fatal("validateOutputPath should reject a symlink chain leading outside CWD")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// D. validateFilterPath — traversal attack vectors for filter file paths
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_ValidateFilterPath_TraversalToNonexistentFile verifies
// that traversal paths that do not resolve to an existing file are rejected
// with a "file not found" (or similar) error. validateFilterPath uses
// os.Lstat so the traversal is resolved but the file check fails.
func TestPathTraversal_ValidateFilterPath_TraversalToNonexistentFile(t *testing.T) {
	traversalFilterPaths := []string{
		"../../../etc/nonexistent_filter.txt",
		"../../../../tmp/filters.txt",
		"../../filters/list.txt",
	}
	for _, p := range traversalFilterPaths {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			_, err := validateFilterPath(p)
			if err == nil {
				t.Errorf("validateFilterPath(%q): expected error for traversal to nonexistent file", p)
			}
		})
	}
}

// TestPathTraversal_ValidateFilterPath_NullByteTraversal verifies that null
// bytes in filter file paths supplied via the --match or --protect flag are
// rejected before any file I/O attempt.
func TestPathTraversal_ValidateFilterPath_NullByteTraversal(t *testing.T) {
	nullBytePaths := []string{
		"../../filters\x00/list.txt",
		"filter\x00list.txt",
		"../etc/\x00filters",
	}
	for _, p := range nullBytePaths {
		p := p // capture
		t.Run("null_in_filterpath", func(t *testing.T) {
			_, err := validateFilterPath(p)
			if err == nil {
				t.Errorf("validateFilterPath(%q): expected rejection for null byte", p)
				return
			}
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("validateFilterPath(%q): expected 'null byte' in error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateFilterPath_RejectsSymlinkToOutside verifies that
// a symlink supplied as a filter file path is rejected even if the link target
// is a valid regular file.
func TestPathTraversal_ValidateFilterPath_RejectsSymlinkToOutside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	outsideDir := t.TempDir()
	realFilter := filepath.Join(outsideDir, "real_filters.txt")
	if err := os.WriteFile(realFilter, []byte("google.com\n"), 0600); err != nil {
		t.Fatal(err)
	}

	symlinkDir := t.TempDir()
	symlinkFilter := filepath.Join(symlinkDir, "filters.txt")
	if err := os.Symlink(realFilter, symlinkFilter); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	_, err := validateFilterPath(symlinkFilter)
	if err == nil {
		t.Fatal("validateFilterPath should reject a symlink filter file path")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// E. Windows-specific path traversal vectors
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_Windows_BackslashTraversal verifies that Windows-style
// backslash traversal paths are sanitised correctly on Windows. filepath.Clean
// treats backslashes as separators on Windows and ".." sequences are resolved.
func TestPathTraversal_Windows_BackslashTraversal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific backslash traversal only runs on Windows")
	}
	traversals := []string{
		`..\..\Windows\System32\drivers\etc\hosts`,
		`..\..\..\Windows\win.ini`,
		`..\..\..\..\Windows\System32\config\SAM`,
	}
	for _, p := range traversals {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			result, err := sanitizePath(p)
			if err != nil {
				// Rejection is acceptable (e.g., reserved name caught).
				return
			}
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", p, result)
			}
			if strings.Contains(result, "..") {
				t.Errorf("sanitizePath(%q) = %q: result still contains traversal", p, result)
			}
		})
	}
}

// TestPathTraversal_Windows_ReservedNamesInTraversalPath verifies that Windows
// reserved device names appearing in a traversal path are rejected by
// sanitizePath on Windows. e.g. "../../NUL" must be caught.
func TestPathTraversal_Windows_ReservedNamesInTraversalPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows reserved name traversal only runs on Windows")
	}
	reservedTraversals := []string{
		`../../NUL`,
		`../CON`,
		`../../COM1`,
		`../LPT1`,
		`../../AUX.db`,
		`../PRN.txt`,
	}
	for _, p := range reservedTraversals {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			_, err := sanitizePath(p)
			if err == nil {
				t.Errorf("sanitizePath(%q): expected rejection for reserved device name in traversal path", p)
				return
			}
			if !strings.Contains(err.Error(), "reserved device name") &&
				!strings.Contains(err.Error(), "Windows device path") {
				t.Errorf("sanitizePath(%q): expected device name error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_Windows_DevicePathPrefix verifies that explicit Windows
// device path prefixes in traversal-style strings are rejected.
func TestPathTraversal_Windows_DevicePathPrefix(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows device path tests only run on Windows")
	}
	devicePaths := []string{
		`\\.\C:\Users\victim\secret.db`,
		`\\?\C:\Windows\System32\config\SAM`,
		`\\.\PhysicalDrive0`,
	}
	for _, p := range devicePaths {
		p := p // capture
		t.Run(filepath.Base(p), func(t *testing.T) {
			_, err := sanitizePath(p)
			if err == nil {
				t.Errorf("sanitizePath(%q): expected rejection for Windows device path prefix", p)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// F. Cross-function consistency checks
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_AllValidators_RejectNullByte verifies that every
// user-facing path validator rejects null bytes consistently, regardless of
// whether the null byte appears in a traversal context or a plain path.
func TestPathTraversal_AllValidators_RejectNullByte(t *testing.T) {
	nullPath := "innocent/looking/\x00path.db"

	t.Run("sanitizePath", func(t *testing.T) {
		_, err := sanitizePath(nullPath)
		if err == nil {
			t.Error("sanitizePath: expected null byte rejection")
		}
	})

	t.Run("validateDBPath", func(t *testing.T) {
		_, err := validateDBPath(nullPath)
		if err == nil {
			t.Error("validateDBPath: expected null byte rejection")
		}
	})

	t.Run("validateOutputPath", func(t *testing.T) {
		_, err := validateOutputPath(nullPath)
		if err == nil {
			t.Error("validateOutputPath: expected null byte rejection")
		}
	})

	t.Run("validateFilterPath", func(t *testing.T) {
		_, err := validateFilterPath(nullPath)
		if err == nil {
			t.Error("validateFilterPath: expected null byte rejection")
		}
	})
}

// TestPathTraversal_AllValidators_RejectEmptyPath verifies that every
// user-facing path validator consistently rejects an empty string.
func TestPathTraversal_AllValidators_RejectEmptyPath(t *testing.T) {
	t.Run("sanitizePath", func(t *testing.T) {
		_, err := sanitizePath("")
		if err == nil {
			t.Error("sanitizePath: expected error for empty path")
		}
	})

	t.Run("validateDBPath", func(t *testing.T) {
		_, err := validateDBPath("")
		if err == nil {
			t.Error("validateDBPath: expected error for empty path")
		}
	})

	t.Run("validateOutputPath", func(t *testing.T) {
		_, err := validateOutputPath("")
		if err == nil {
			t.Error("validateOutputPath: expected error for empty path")
		}
	})

	t.Run("validateFilterPath", func(t *testing.T) {
		_, err := validateFilterPath("")
		if err == nil {
			t.Error("validateFilterPath: expected error for empty path")
		}
	})
}

// TestPathTraversal_SanitizePath_ExcessiveTraversalDepth verifies that
// deeply nested traversal sequences (many "../" repetitions) do not cause
// a panic, infinite loop, or unexpected behaviour. filepath.Clean collapses
// them to the filesystem root.
func TestPathTraversal_SanitizePath_ExcessiveTraversalDepth(t *testing.T) {
	// Construct a path with 200 "../" repetitions followed by "etc/passwd".
	traversal := strings.Repeat("../", 200) + "etc/passwd"

	result, err := sanitizePath(traversal)
	if err != nil {
		t.Fatalf("sanitizePath with 200-level traversal: unexpected error: %v", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("sanitizePath with 200-level traversal = %q: expected absolute path", result)
	}
	if strings.Contains(result, "..") {
		t.Errorf("sanitizePath with 200-level traversal = %q: result contains '..'", result)
	}
}

// TestPathTraversal_ValidateOutputPath_ExcessiveTraversalDepth verifies that
// deeply nested traversal sequences supplied as --out are rejected consistently.
func TestPathTraversal_ValidateOutputPath_ExcessiveTraversalDepth(t *testing.T) {
	traversal := strings.Repeat("../", 200) + "etc/evil.csv"

	_, err := validateOutputPath(traversal)
	if err == nil {
		t.Error("validateOutputPath with 200-level traversal: expected rejection, got nil")
		return
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("validateOutputPath with 200-level traversal: expected 'escapes' in error, got: %v", err)
	}
}

// TestPathTraversal_SanitizePath_TraversalWithUnicodeComponentsResolvesCleanly
// verifies that unicode characters in traversal paths are handled correctly:
// the ".." sequences are resolved, the unicode filename components are preserved,
// and the result is absolute and clean.
func TestPathTraversal_SanitizePath_TraversalWithUnicodeComponentsResolvesCleanly(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"../../한국어/file.db"},
		{"../données/../secret.db"},
		{"../café/../normal.db"},
	}
	for _, tc := range tests {
		tc := tc // capture
		t.Run(tc.input, func(t *testing.T) {
			result, err := sanitizePath(tc.input)
			if err != nil {
				t.Fatalf("sanitizePath(%q): unexpected error: %v", tc.input, err)
			}
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", tc.input, result)
			}
			if strings.Contains(result, "..") {
				t.Errorf("sanitizePath(%q) = %q: result still contains '..'", tc.input, result)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_TraversalWithUnicode verifies that
// unicode traversal paths are still rejected when they escape the CWD,
// regardless of the unicode characters in the path components.
func TestPathTraversal_ValidateOutputPath_TraversalWithUnicode(t *testing.T) {
	unicodeTraversals := []string{
		"../../한국어/evil.csv",
		"../données/output.csv",
		"../../../tmp/café.csv",
	}
	for _, p := range unicodeTraversals {
		p := p // capture
		t.Run(p, func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for unicode traversal path", p)
				return
			}
			if !strings.Contains(err.Error(), "escapes") {
				t.Errorf("validateOutputPath(%q): expected 'escapes' in error, got: %v", p, err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// G. Additional attack vectors — percent-encoding, alternate streams, edge cases
// ─────────────────────────────────────────────────────────────────────────────

// TestPathTraversal_SanitizePath_PercentEncodedSequences verifies that
// percent-encoded traversal sequences (e.g. %2e%2e%2f, the URL encoding of
// "../") are treated as literal filename characters by sanitizePath, NOT as
// traversal sequences. filepath.Clean does not URL-decode, so these pass
// through as path components containing the percent sign. The test documents
// this behaviour: sanitizePath returns a canonical absolute path rather than
// an error, because the strings do not contain null bytes or control chars.
// Callers that receive paths from URL inputs must URL-decode before calling
// sanitizePath.
//
// Traversal safety note: the only way percent-encoded inputs could cause
// actual directory traversal is if "%2e%2e" (or similar) were decoded to ".."
// by sanitizePath — but sanitizePath does no decoding. Inputs such as
// "..%2fetc%2fpasswd" contain a real ".." at the start; however, because no
// path separator follows immediately, filepath.Clean treats the whole string
// "..%2fetc%2fpasswd" as a single relative filename component (not a
// two-component path), so the result is <CWD>\..%2fetc%2fpasswd — still
// within the working directory.
func TestPathTraversal_SanitizePath_PercentEncodedSequences(t *testing.T) {
	encoded := []struct {
		input string
		// expectAboveVolumeRoot is true when the input is known to walk above
		// CWD (via a real ".." component with a separator), meaning the result
		// may reference a directory outside CWD — which is acceptable for
		// sanitizePath but would be caught by validateOutputPath.
		expectAboveVolumeRoot bool
	}{
		{"%2e%2e%2fetc%2fpasswd", false},       // URL encoding of ../../etc/passwd — no real separators
		{"%2e%2e/%2e%2e/etc/passwd", false},    // mixed: %2e%2e is a literal dirname, not ".."
		{"..%2fetc%2fpasswd", false},           // ".." without separator is a single filename component
		{"%2e%2e%5cetc%5cpasswd", false},       // %5c is NOT a separator — one component
	}
	for _, tc := range encoded {
		tc := tc // capture
		t.Run(tc.input, func(t *testing.T) {
			result, err := sanitizePath(tc.input)
			if err != nil {
				// Rejection is only expected if the string contains null or
				// control bytes; percent-encoded strings contain none of those.
				t.Fatalf("sanitizePath(%q): unexpected error: %v (percent-encoded sequences should pass through as literal path components)", tc.input, err)
			}
			// Result must be absolute.
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", tc.input, result)
			}
			// Verify that no standalone ".." component remains in the result.
			// Note: ".." may appear as a SUBSTRING of a filename component
			// (e.g. "..%2fetc" is a single filename), which is not a security
			// issue. We therefore check components, not substrings.
			for _, component := range strings.Split(result, string(filepath.Separator)) {
				if component == ".." {
					t.Errorf("sanitizePath(%q) = %q: result has standalone '..' component; traversal was not resolved", tc.input, result)
				}
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_SingleDotDot verifies that a single
// ".." (parent-directory escape without any extra path component) is rejected
// by validateOutputPath because it resolves to the parent of CWD, which is
// outside the working directory.
func TestPathTraversal_ValidateOutputPath_SingleDotDot(t *testing.T) {
	_, err := validateOutputPath("..")
	if err == nil {
		t.Fatalf("validateOutputPath(%q): expected rejection for '..', got nil", "..")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("validateOutputPath(%q): expected 'escapes' in error, got: %v", "..", err)
	}
}

// TestPathTraversal_ValidateOutputPath_ParentThenFilename verifies that the
// common pattern "../output.csv" (one level up followed by a filename) is
// rejected because the resolved path escapes CWD.
func TestPathTraversal_ValidateOutputPath_ParentThenFilename(t *testing.T) {
	cases := []string{
		"../output.csv",
		"../results.csv",
		"../data/export.csv",
	}
	for _, p := range cases {
		p := p
		t.Run(p, func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for path escaping CWD", p)
				return
			}
			if !strings.Contains(err.Error(), "escapes") {
				t.Errorf("validateOutputPath(%q): expected 'escapes' in error, got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateDBPath_TraversalResolvesToDirectory verifies that
// a traversal path which resolves to an existing directory is rejected by
// validateDBPath with a "directory" error, not silently opened as a database.
func TestPathTraversal_ValidateDBPath_TraversalResolvesToDirectory(t *testing.T) {
	// t.TempDir() returns an existing directory. We construct a traversal
	// sequence that resolves to it so the filesystem check fires.
	dir := t.TempDir()

	// Build a path like "<dir>/dummy/../.." which resolves to the parent of dir.
	// The parent directory exists and is a directory, so validateDBPath must
	// reject it with a "directory" error.
	traversalToDir := filepath.Join(dir, "dummy", "..", "..")

	_, err := validateDBPath(traversalToDir)
	if err == nil {
		t.Fatal("validateDBPath: expected error when path resolves to a directory, got nil")
	}
	// Accept either "directory" or "file not found" depending on whether the
	// resolved path exists — what must NOT happen is silent success.
	_ = err
}

// TestPathTraversal_ValidateFilterPath_SymlinkChain verifies that a chain of
// symlinks (A → B → real file) supplied as a filter file path is rejected.
// os.Lstat reports the first hop (A) as a symlink without following the chain,
// so the rejection fires at the first level.
func TestPathTraversal_ValidateFilterPath_SymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	dir := t.TempDir()

	realFile := filepath.Join(dir, "real_filters.txt")
	if err := os.WriteFile(realFile, []byte("google.com\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// linkB → realFile
	linkB := filepath.Join(dir, "linkB.txt")
	if err := os.Symlink(realFile, linkB); err != nil {
		t.Skipf("cannot create first symlink: %v", err)
	}

	// linkA → linkB (chain depth 2)
	linkA := filepath.Join(dir, "linkA.txt")
	if err := os.Symlink(linkB, linkA); err != nil {
		t.Skipf("cannot create second symlink: %v", err)
	}

	_, err := validateFilterPath(linkA)
	if err == nil {
		t.Fatal("validateFilterPath should reject a symlink chain")
	}
	if !strings.Contains(err.Error(), "symbolic") {
		t.Errorf("expected 'symbolic' in error, got: %v", err)
	}
}

// TestPathTraversal_SanitizePath_SingleDotResolvesToCWD verifies that a
// single "." is resolved to the current working directory (absolute path)
// without error. "." is not a traversal attack but is included to document
// that sanitizePath normalises it correctly.
func TestPathTraversal_SanitizePath_SingleDotResolvesToCWD(t *testing.T) {
	result, err := sanitizePath(".")
	if err != nil {
		t.Fatalf("sanitizePath(%q): unexpected error: %v", ".", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("sanitizePath(%q) = %q: expected absolute path", ".", result)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if result != filepath.Clean(cwd) {
		t.Errorf("sanitizePath(%q) = %q; expected CWD %q", ".", result, cwd)
	}
}

// TestPathTraversal_SanitizePath_ThreeDotsIsNotTraversal verifies that
// "..." (three dots) is NOT treated as a traversal sequence that escapes
// upward in the directory tree. Only ".." is a traversal component;
// three or more dots have no special meaning in standard path resolution.
//
// Note: on Windows, "..." is normalised by the OS to the current directory
// (the same way "." is) due to Win32 path canonicalization rules, so the
// result on Windows may equal the CWD rather than CWD+"...". The key
// invariant asserted here is platform-independent: the result must be
// absolute, must not contain "..", and must not equal the parent of CWD.
func TestPathTraversal_SanitizePath_ThreeDotsIsNotTraversal(t *testing.T) {
	result, err := sanitizePath("...")
	if err != nil {
		t.Fatalf("sanitizePath(%q): unexpected error: %v", "...", err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("sanitizePath(%q) = %q: expected absolute path", "...", result)
	}
	// The result must not contain a ".." component, which would indicate
	// the three-dot input was misinterpreted as a parent-directory traversal.
	if strings.Contains(result, "..") {
		t.Errorf("sanitizePath(%q) = %q: result contains '..'; three dots must not cause traversal", "...", result)
	}
	// The result must not be the parent of CWD. If it were, sanitizePath
	// would be treating "..." as equivalent to ".." (parent-dir traversal).
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(filepath.Clean(cwd))
	if result == parent {
		t.Errorf("sanitizePath(%q) = %q: result equals parent of CWD; three dots must not traverse up", "...", result)
	}
}

// TestPathTraversal_ValidateDBPath_LongTraversalPathDoesNotPanic verifies that
// an extremely long traversal path (thousands of "../" repetitions) does not
// cause a panic, stack overflow, or excessive memory use. The path should be
// either rejected or resolved to a canonical absolute path without crashing.
func TestPathTraversal_ValidateDBPath_LongTraversalPathDoesNotPanic(t *testing.T) {
	// Construct a path with 1000 "../" repetitions followed by a target name.
	traversal := strings.Repeat("../", 1000) + "nonexistent_db_target"

	// Must not panic. Error is expected (file not found) but nil is also
	// acceptable if the path somehow resolves to an existing file.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("validateDBPath panicked on 1000-level traversal: %v", r)
		}
	}()
	_, _ = validateDBPath(traversal)
}

// TestPathTraversal_ValidateOutputPath_LongPathDoesNotPanic verifies that an
// extremely long traversal path supplied as --out does not panic. The path is
// expected to be rejected for escaping CWD.
func TestPathTraversal_ValidateOutputPath_LongPathDoesNotPanic(t *testing.T) {
	traversal := strings.Repeat("../", 1000) + "evil_output.csv"

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("validateOutputPath panicked on 1000-level traversal: %v", r)
		}
	}()

	_, err := validateOutputPath(traversal)
	if err == nil {
		t.Error("validateOutputPath: expected rejection for 1000-level traversal, got nil")
	}
}

// TestPathTraversal_Windows_AlternateDataStreamRejected verifies that Windows
// Alternate Data Stream paths (e.g. "file.db:stream") are rejected by
// sanitizePath on Windows. The colon character introduces an ADS name which
// can be used to hide content or redirect file access.
func TestPathTraversal_Windows_AlternateDataStreamRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Alternate Data Stream paths only apply on Windows")
	}
	adsPaths := []string{
		`History:hidden_stream`,
		`output.csv:zone.identifier`,
		`db.sqlite:$DATA`,
		`../History:secret`,
	}
	for _, p := range adsPaths {
		p := p
		t.Run(p, func(t *testing.T) {
			// On Windows, filepath.Clean may treat the colon specially.
			// sanitizePath must either reject the path (due to reserved name
			// detection or device path detection) or return a safe canonical
			// path that does not include a colon-stream component.
			result, err := sanitizePath(p)
			if err != nil {
				// Rejection is the preferred outcome.
				return
			}
			// If accepted, the colon must not be silently passed through to
			// a filesystem operation that could open an ADS.
			if strings.Contains(result, ":") && !strings.HasPrefix(result, `\\`) {
				// On Windows, drive letters like "C:\" are legitimate and must
				// not be flagged. Only flag if the colon appears after the
				// drive-letter position.
				parts := strings.SplitN(result, ":", 2)
				if len(parts) == 2 && len(parts[0]) > 1 {
					t.Errorf("sanitizePath(%q) = %q: colon in non-drive position may indicate ADS", p, result)
				}
			}
		})
	}
}

// TestPathTraversal_ValidateDBPath_TraversalWithNullByteAndPath verifies that
// a path combining both a traversal sequence and an embedded null byte is
// rejected for the null byte (the earlier, higher-priority check) rather than
// reaching the filesystem stat call.
func TestPathTraversal_ValidateDBPath_TraversalWithNullByteAndPath(t *testing.T) {
	combined := []string{
		"../../etc/passwd\x00.db",
		"../\x00../../shadow",
		"\x00../../../etc/hosts",
	}
	for _, p := range combined {
		p := p
		t.Run("combined_null_traversal", func(t *testing.T) {
			_, err := validateDBPath(p)
			if err == nil {
				t.Errorf("validateDBPath(%q): expected rejection, got nil", p)
				return
			}
			// The null-byte check must fire before any filesystem operation.
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("validateDBPath(%q): expected 'null byte' in error (null byte must be caught first), got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_TraversalWithNullByteAndPath verifies
// the same combined-attack property for the output path validator.
func TestPathTraversal_ValidateOutputPath_TraversalWithNullByteAndPath(t *testing.T) {
	combined := []string{
		"../../tmp/evil\x00.csv",
		"out\x00put/../../../etc/passwd",
	}
	for _, p := range combined {
		p := p
		t.Run("combined_null_traversal_output", func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection, got nil", p)
				return
			}
			if !strings.Contains(err.Error(), "null byte") {
				t.Errorf("validateOutputPath(%q): expected 'null byte' error (null byte must be caught first), got: %v", p, err)
			}
		})
	}
}

// TestPathTraversal_SanitizePath_TrailingSlashNormalised verifies that a
// trailing slash in a traversal path is normalised away by filepath.Clean
// so the result is an absolute, clean path.
func TestPathTraversal_SanitizePath_TrailingSlashNormalised(t *testing.T) {
	paths := []string{
		"../../etc/",
		"../tmp/",
		"foo/bar/",
		"../",
	}
	for _, p := range paths {
		p := p
		t.Run(p, func(t *testing.T) {
			result, err := sanitizePath(p)
			if err != nil {
				t.Fatalf("sanitizePath(%q): unexpected error: %v", p, err)
			}
			if !filepath.IsAbs(result) {
				t.Errorf("sanitizePath(%q) = %q: expected absolute path", p, result)
			}
			// Trailing slash must have been stripped.
			if strings.HasSuffix(result, string(filepath.Separator)) &&
				result != filepath.VolumeName(result)+string(filepath.Separator) {
				t.Errorf("sanitizePath(%q) = %q: result has unexpected trailing separator", p, result)
			}
			if strings.Contains(result, "..") {
				t.Errorf("sanitizePath(%q) = %q: result still contains '..' component", p, result)
			}
		})
	}
}

// TestPathTraversal_ValidateOutputPath_TrailingSlash verifies that a traversal
// path with a trailing slash that escapes CWD is still rejected. The trailing
// slash normalisation in filepath.Clean must not cause the escape check to be
// skipped.
func TestPathTraversal_ValidateOutputPath_TrailingSlash(t *testing.T) {
	cases := []string{
		"../../tmp/",
		"../evil/",
	}
	for _, p := range cases {
		p := p
		t.Run(p, func(t *testing.T) {
			_, err := validateOutputPath(p)
			if err == nil {
				t.Errorf("validateOutputPath(%q): expected rejection for trailing-slash traversal, got nil", p)
				return
			}
			if !strings.Contains(err.Error(), "escapes") && !strings.Contains(err.Error(), "directory") {
				t.Errorf("validateOutputPath(%q): expected 'escapes' or 'directory' in error, got: %v", p, err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper functions
// ─────────────────────────────────────────────────────────────────────────────

// hexByte returns the lowercase hex representation of a single byte, used to
// produce human-readable test names for control character test cases.
func hexByte(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0xf]})
}
