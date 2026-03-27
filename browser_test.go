package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- validateBrowserName tests ---

func TestValidateBrowserName_ValidNames(t *testing.T) {
	valid := []string{"brave", "chrome", "chromium", "edge", "opera"}
	for _, name := range valid {
		if err := validateBrowserName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}
}

func TestValidateBrowserName_CaseInsensitive(t *testing.T) {
	mixed := []string{"Brave", "Chrome", "CHROMIUM", "Edge", "Opera"}
	for _, name := range mixed {
		if err := validateBrowserName(name); err != nil {
			t.Errorf("expected %q (case-insensitive) to be valid, got error: %v", name, err)
		}
	}
}

func TestValidateBrowserName_InvalidName(t *testing.T) {
	invalid := []string{"firefox", "safari", "ie", "", "unknown-browser", "vivaldi"}
	for _, name := range invalid {
		if err := validateBrowserName(name); err == nil {
			t.Errorf("expected %q to be invalid, but got no error", name)
		}
	}
}

func TestValidateBrowserName_ErrorListsValidNames(t *testing.T) {
	err := validateBrowserName("firefox")
	if err == nil {
		t.Fatal("expected an error for 'firefox'")
	}
	msg := err.Error()
	// The error message must mention all valid browser names.
	for name := range validBrowserNames {
		if !strings.Contains(msg, name) {
			t.Errorf("error message should list %q as a valid name; got: %q", name, msg)
		}
	}
}

func TestValidateBrowserName_SyncWithCandidates(t *testing.T) {
	// Collect all browser names ever produced by buildBrowserCandidates
	// across all platforms and confirm each is in validBrowserNames.
	allOS := []string{"windows", "darwin", "linux"}
	for _, goos := range allOS {
		candidates := buildBrowserCandidates(goos, "/home/test", "/appdata/local", "/appdata/roaming", "")
		for name := range candidates {
			if !validBrowserNames[name] {
				t.Errorf("browser %q produced by buildBrowserCandidates on %s is not in validBrowserNames", name, goos)
			}
		}
	}
}

// --- buildBrowserCandidates tests ---

func TestBuildBrowserCandidates_Windows(t *testing.T) {
	candidates := buildBrowserCandidates("windows", `C:\Users\test`, `C:\Users\test\AppData\Local`, `C:\Users\test\AppData\Roaming`, "")

	expected := map[string]string{
		"brave":   filepath.Join(`C:\Users\test\AppData\Local`, "BraveSoftware", "Brave-Browser", "User Data", "Default", "History"),
		"chrome":  filepath.Join(`C:\Users\test\AppData\Local`, "Google", "Chrome", "User Data", "Default", "History"),
		"edge":    filepath.Join(`C:\Users\test\AppData\Local`, "Microsoft", "Edge", "User Data", "Default", "History"),
		"opera":   filepath.Join(`C:\Users\test\AppData\Roaming`, "Opera Software", "Opera Stable", "Default", "History"),
	}

	if len(candidates) != len(expected) {
		t.Fatalf("expected %d browsers, got %d", len(expected), len(candidates))
	}
	for name, wantPath := range expected {
		got, ok := candidates[name]
		if !ok {
			t.Errorf("missing browser %q", name)
			continue
		}
		if got != wantPath {
			t.Errorf("%s: expected %q, got %q", name, wantPath, got)
		}
	}
}

func TestBuildBrowserCandidates_Darwin(t *testing.T) {
	candidates := buildBrowserCandidates("darwin", "/Users/test", "", "", "")

	expected := map[string]string{
		"brave":   filepath.Join("/Users/test", "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "History"),
		"chrome":  filepath.Join("/Users/test", "Library", "Application Support", "Google", "Chrome", "Default", "History"),
		"edge":    filepath.Join("/Users/test", "Library", "Application Support", "Microsoft Edge", "Default", "History"),
	}

	if len(candidates) != len(expected) {
		t.Fatalf("expected %d browsers, got %d", len(expected), len(candidates))
	}
	for name, wantPath := range expected {
		got, ok := candidates[name]
		if !ok {
			t.Errorf("missing browser %q", name)
			continue
		}
		if got != wantPath {
			t.Errorf("%s: expected %q, got %q", name, wantPath, got)
		}
	}
}

func TestBuildBrowserCandidates_Linux(t *testing.T) {
	candidates := buildBrowserCandidates("linux", "/home/test", "", "", "")

	expected := map[string]string{
		"brave":    filepath.Join("/home/test", ".config", "BraveSoftware", "Brave-Browser", "Default", "History"),
		"chrome":   filepath.Join("/home/test", ".config", "google-chrome", "Default", "History"),
		"chromium": filepath.Join("/home/test", ".config", "chromium", "Default", "History"),
		"edge":     filepath.Join("/home/test", ".config", "microsoft-edge", "Default", "History"),
	}

	if len(candidates) != len(expected) {
		t.Fatalf("expected %d browsers, got %d", len(expected), len(candidates))
	}
	for name, wantPath := range expected {
		got, ok := candidates[name]
		if !ok {
			t.Errorf("missing browser %q", name)
			continue
		}
		if got != wantPath {
			t.Errorf("%s: expected %q, got %q", name, wantPath, got)
		}
	}
}

func TestBuildBrowserCandidates_LinuxHasChromium(t *testing.T) {
	candidates := buildBrowserCandidates("linux", "/home/test", "", "", "")
	if _, ok := candidates["chromium"]; !ok {
		t.Error("Linux should include chromium browser")
	}
}

func TestBuildBrowserCandidates_WindowsAndDarwinNoChromium(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		candidates := buildBrowserCandidates(goos, "/home/test", "/appdata/local", "/appdata/roaming", "")
		if _, ok := candidates["chromium"]; ok {
			t.Errorf("%s should not include chromium browser", goos)
		}
	}
}

func TestBuildBrowserCandidates_UnknownOS(t *testing.T) {
	candidates := buildBrowserCandidates("freebsd", "/home/test", "", "", "")
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates for unknown OS, got %d", len(candidates))
	}
}

func TestBuildBrowserCandidates_EmptyHome(t *testing.T) {
	// With empty home, paths are still built (just relative); no panic.
	candidates := buildBrowserCandidates("linux", "", "", "", "")
	if len(candidates) == 0 {
		t.Fatal("expected candidates even with empty home")
	}
	// Paths should not start with a path separator (they're relative).
	for name, p := range candidates {
		if filepath.IsAbs(p) {
			t.Errorf("%s: expected relative path with empty home, got %q", name, p)
		}
	}
}

func TestBuildBrowserCandidates_AllPathsEndWithHistory(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		candidates := buildBrowserCandidates(goos, "/home/test", "/appdata/local", "/appdata/roaming", "")
		for name, p := range candidates {
			if filepath.Base(p) != "History" {
				t.Errorf("%s/%s: expected path ending in 'History', got %q", goos, name, p)
			}
		}
	}
}

func TestBuildBrowserCandidates_WindowsUsesLocalAppData(t *testing.T) {
	localAppData := `C:\Users\test\AppData\Local`
	candidates := buildBrowserCandidates("windows", "", localAppData, "", "")

	// Chrome and Edge should use LOCALAPPDATA.
	for _, name := range []string{"chrome", "edge"} {
		p, ok := candidates[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if !strings.HasPrefix(p, localAppData) {
			t.Errorf("%s should use LOCALAPPDATA, got %q", name, p)
		}
	}
}

func TestBuildBrowserCandidates_WindowsOperaUsesAppData(t *testing.T) {
	appData := `C:\Users\test\AppData\Roaming`
	candidates := buildBrowserCandidates("windows", "", "", appData, "")

	p, ok := candidates["opera"]
	if !ok {
		t.Fatal("missing opera")
	}
	if !strings.HasPrefix(p, appData) {
		t.Errorf("opera should use APPDATA, got %q", p)
	}
}

// --- filterExistingPaths tests ---

func TestFilterExistingPaths_AllExist(t *testing.T) {
	dir := t.TempDir()

	// Create fake History files.
	chromePath := filepath.Join(dir, "chrome", "History")
	edgePath := filepath.Join(dir, "edge", "History")
	os.MkdirAll(filepath.Dir(chromePath), 0755)
	os.MkdirAll(filepath.Dir(edgePath), 0755)
	os.WriteFile(chromePath, []byte("db"), 0644)
	os.WriteFile(edgePath, []byte("db"), 0644)

	candidates := map[string]string{
		"chrome": chromePath,
		"edge":   edgePath,
	}
	found := filterExistingPaths(candidates)
	if len(found) != 2 {
		t.Fatalf("expected 2, got %d", len(found))
	}
}

func TestFilterExistingPaths_NoneExist(t *testing.T) {
	candidates := map[string]string{
		"chrome": "/nonexistent/path/to/chrome/History",
		"edge":   "/nonexistent/path/to/edge/History",
	}
	found := filterExistingPaths(candidates)
	if len(found) != 0 {
		t.Fatalf("expected 0, got %d", len(found))
	}
}

func TestFilterExistingPaths_PartialExist(t *testing.T) {
	dir := t.TempDir()

	chromePath := filepath.Join(dir, "chrome", "History")
	os.MkdirAll(filepath.Dir(chromePath), 0755)
	os.WriteFile(chromePath, []byte("db"), 0644)

	candidates := map[string]string{
		"chrome": chromePath,
		"edge":   filepath.Join(dir, "nonexistent", "History"),
	}
	found := filterExistingPaths(candidates)
	if len(found) != 1 {
		t.Fatalf("expected 1, got %d", len(found))
	}
	if _, ok := found["chrome"]; !ok {
		t.Fatal("expected chrome to be found")
	}
}

func TestFilterExistingPaths_EmptyCandidates(t *testing.T) {
	found := filterExistingPaths(map[string]string{})
	if len(found) != 0 {
		t.Fatalf("expected 0, got %d", len(found))
	}
}

func TestFilterExistingPaths_DirectoryNotFile(t *testing.T) {
	dir := t.TempDir()
	// Create a directory named "History" instead of a file.
	histDir := filepath.Join(dir, "chrome", "History")
	os.MkdirAll(histDir, 0755)

	candidates := map[string]string{
		"chrome": histDir,
	}
	// os.Stat succeeds for directories too, so this should be found.
	found := filterExistingPaths(candidates)
	if len(found) != 1 {
		t.Fatalf("expected 1 (directory still passes os.Stat), got %d", len(found))
	}
}

// --- detectBrowserPaths integration test ---

func TestDetectBrowserPaths_ReturnsMap(t *testing.T) {
	// Basic integration test: detectBrowserPaths should return a non-nil map
	// without panicking, regardless of which browsers are installed.
	result := detectBrowserPaths()
	if result == nil {
		t.Fatal("expected non-nil map")
	}
	// On CI or machines without browsers, the map may be empty; that's fine.
	// Just verify all values are non-empty paths.
	for name, path := range result {
		if path == "" {
			t.Errorf("browser %q has empty path", name)
		}
	}
}

func TestDetectBrowserPaths_PathsEndWithHistory(t *testing.T) {
	result := detectBrowserPaths()
	for name, path := range result {
		if filepath.Base(path) != "History" {
			t.Errorf("browser %q path does not end with 'History': %q", name, path)
		}
	}
}

// simulateBrowserInstall creates a fake Chrome History file in dir using the
// path layout expected by the current platform (runtime.GOOS). It returns the
// browser name and the full path to the created History file.
// The caller is responsible for pointing the relevant environment variables
// at dir before calling detectBrowserPaths.
func simulateBrowserInstall(t *testing.T, dir string) (browserName, historyPath string) {
	t.Helper()
	browserName = "chrome"

	switch runtime.GOOS {
	case "windows":
		historyPath = filepath.Join(dir, "Google", "Chrome", "User Data", "Default", "History")
	case "darwin":
		historyPath = filepath.Join(dir, "Library", "Application Support", "Google", "Chrome", "Default", "History")
	case "linux":
		historyPath = filepath.Join(dir, ".config", "google-chrome", "Default", "History")
	default:
		t.Skipf("simulateBrowserInstall: unsupported platform %s", runtime.GOOS)
	}

	if err := os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil {
		t.Fatalf("simulateBrowserInstall: mkdir: %v", err)
	}
	if err := os.WriteFile(historyPath, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatalf("simulateBrowserInstall: write History: %v", err)
	}
	return browserName, historyPath
}

// overridePlatformEnv redirects the environment variables that detectBrowserPaths
// reads (LOCALAPPDATA/APPDATA on Windows, HOME on Linux/macOS) to dir so the
// test is isolated from real browser installations on the machine.
// On Windows both LOCALAPPDATA and APPDATA are pointed at dir because different
// browsers use different variables (e.g. Opera reads APPDATA while Chrome reads
// LOCALAPPDATA). t.Setenv ensures all changes are reverted after the test.
func overridePlatformEnv(t *testing.T, dir string) {
	t.Helper()

	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", dir)
		t.Setenv("APPDATA", dir)
		// os.UserHomeDir() on Windows checks USERPROFILE first.
		t.Setenv("USERPROFILE", t.TempDir())
	case "darwin", "linux":
		t.Setenv("HOME", dir)
	default:
		t.Skipf("overridePlatformEnv: unsupported platform %s", runtime.GOOS)
	}
}

// TestDetectBrowserPaths_SimulatedInstallation verifies that detectBrowserPaths
// finds a browser whose History file exists in the platform-correct location.
// Env vars are overridden to point to a temp dir, isolating the test from any
// real browser installations on the host machine.
func TestDetectBrowserPaths_SimulatedInstallation(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	browserName, _ := simulateBrowserInstall(t, dir)

	result := detectBrowserPaths()
	if _, ok := result[browserName]; !ok {
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		t.Errorf("expected %q to be detected; result: %v", browserName, keys)
	}
}

// TestDetectBrowserPaths_MissingHistoryFile verifies that a browser whose
// Default profile directory exists but whose History file is absent is NOT
// returned by detectBrowserPaths. This models Chrome being installed but
// never launched (no profile data written yet).
func TestDetectBrowserPaths_MissingHistoryFile(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	// Build the profile directory path but deliberately omit the History file.
	var profileDir string
	switch runtime.GOOS {
	case "windows":
		profileDir = filepath.Join(dir, "Google", "Chrome", "User Data", "Default")
	case "darwin":
		profileDir = filepath.Join(dir, "Library", "Application Support", "Google", "Chrome", "Default")
	case "linux":
		profileDir = filepath.Join(dir, ".config", "google-chrome", "Default")
	default:
		t.Skipf("unsupported platform: %s", runtime.GOOS)
	}

	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatalf("failed to create profile directory: %v", err)
	}
	// History file intentionally NOT created.

	result := detectBrowserPaths()
	if path, ok := result["chrome"]; ok {
		t.Errorf("chrome must NOT be detected when History file is absent; got %q", path)
	}
}

// TestDetectBrowserPaths_MissingDefaultProfile verifies that a browser whose
// top-level application-data directory exists but whose Default/ profile
// subdirectory is completely absent is NOT returned by detectBrowserPaths.
// This models a partial or interrupted browser installation.
func TestDetectBrowserPaths_MissingDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	// Create only the top-level app directory, not Default/ or History.
	var appDir string
	switch runtime.GOOS {
	case "windows":
		appDir = filepath.Join(dir, "Google", "Chrome", "User Data")
	case "darwin":
		appDir = filepath.Join(dir, "Library", "Application Support", "Google", "Chrome")
	case "linux":
		appDir = filepath.Join(dir, ".config", "google-chrome")
	default:
		t.Skipf("unsupported platform: %s", runtime.GOOS)
	}

	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("failed to create app directory: %v", err)
	}

	result := detectBrowserPaths()
	if path, ok := result["chrome"]; ok {
		t.Errorf("chrome must NOT be detected when Default profile directory is absent; got %q", path)
	}
}

// TestDetectBrowserPaths_MultipleSimulatedBrowsers creates History files for
// every browser candidate on the current platform and verifies all are detected.
func TestDetectBrowserPaths_MultipleSimulatedBrowsers(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	// Get all candidates for the current platform using dir as every base dir.
	candidates := buildBrowserCandidates(runtime.GOOS, dir, dir, dir, "")
	if len(candidates) == 0 {
		t.Skipf("no browser candidates defined for platform %s", runtime.GOOS)
	}

	for name, p := range candidates {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("browser %s: mkdir: %v", name, err)
		}
		if err := os.WriteFile(p, []byte("fake"), 0644); err != nil {
			t.Fatalf("browser %s: write History: %v", name, err)
		}
	}

	result := detectBrowserPaths()
	for name := range candidates {
		if _, ok := result[name]; !ok {
			t.Errorf("browser %q was not detected, expected it to be found", name)
		}
	}
	if len(result) != len(candidates) {
		t.Errorf("expected %d browsers detected, got %d: %v", len(candidates), len(result), result)
	}
}

// TestDetectBrowserPaths_OnlyExistingBrowsersReturned verifies that when
// multiple browsers are candidates but only one has a History file, only that
// browser appears in the result.
func TestDetectBrowserPaths_OnlyExistingBrowsersReturned(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	candidates := buildBrowserCandidates(runtime.GOOS, dir, dir, dir, "")
	if len(candidates) < 2 {
		t.Skipf("need at least 2 browser candidates on %s (got %d)", runtime.GOOS, len(candidates))
	}

	// Collect and sort candidate names for deterministic selection.
	names := make([]string, 0, len(candidates))
	for n := range candidates {
		names = append(names, n)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}

	// Create a History file only for the first candidate.
	installedName := names[0]
	installedPath := candidates[installedName]
	if err := os.MkdirAll(filepath.Dir(installedPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(installedPath, []byte("fake"), 0644); err != nil {
		t.Fatalf("write History: %v", err)
	}

	result := detectBrowserPaths()

	if _, ok := result[installedName]; !ok {
		t.Errorf("expected %q to be detected", installedName)
	}
	for _, name := range names[1:] {
		if _, ok := result[name]; ok {
			t.Errorf("browser %q must NOT be detected (no History file created)", name)
		}
	}
}

// TestDetectBrowserPaths_NoBrowsersInstalled verifies that detectBrowserPaths
// returns a non-nil empty map when the isolated environment has no History files.
func TestDetectBrowserPaths_NoBrowsersInstalled(t *testing.T) {
	emptyDir := t.TempDir()
	overridePlatformEnv(t, emptyDir)

	result := detectBrowserPaths()
	if result == nil {
		t.Fatal("expected non-nil map even when no browsers are installed")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map when no browsers are installed, got: %v", result)
	}
}

// TestDetectBrowserPaths_ResultContainsOnlyKnownBrowserNames is an end-to-end
// check that every key returned by detectBrowserPaths is a recognised browser
// name present in validBrowserNames; the detection pipeline should never
// produce unexpected keys.
func TestDetectBrowserPaths_ResultContainsOnlyKnownBrowserNames(t *testing.T) {
	result := detectBrowserPaths()
	for name := range result {
		if !validBrowserNames[name] {
			t.Errorf("detectBrowserPaths returned unknown browser name %q", name)
		}
	}
}

// --- resolveDBPath tests ---

func TestResolveDBPath_CustomPathWithValidDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")
	createTestChromeDB(t, dbPath)

	result, err := resolveDBPath("", dbPath, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != dbPath {
		t.Fatalf("expected %q, got %q", dbPath, result)
	}
}

func TestResolveDBPath_CustomPathInvalid(t *testing.T) {
	_, err := resolveDBPath("", "/nonexistent/path/History", "")
	if err == nil {
		t.Fatal("expected error for nonexistent custom path")
	}
}

func TestResolveDBPath_BrowserNotFound(t *testing.T) {
	_, err := resolveDBPath("nonexistent-browser", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent browser")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got %q", err.Error())
	}
}

// --- buildBrowserCandidates with custom profile ---

// TestBuildBrowserCandidates_CustomProfile verifies that passing a non-empty
// profile name replaces "Default" in the constructed paths.
func TestBuildBrowserCandidates_CustomProfile(t *testing.T) {
	candidates := buildBrowserCandidates("linux", "/home/test", "", "", "Profile 1")
	chromePath, ok := candidates["chrome"]
	if !ok {
		t.Fatal("missing chrome in linux candidates")
	}
	if !strings.Contains(chromePath, "Profile 1") {
		t.Errorf("expected 'Profile 1' in path, got %q", chromePath)
	}
	if strings.Contains(chromePath, "Default") {
		t.Errorf("path should not contain 'Default' when profile='Profile 1', got %q", chromePath)
	}
}

// TestBuildBrowserCandidates_EmptyProfileDefaultsToDefault verifies that an
// empty profile string produces paths using "Default" as the profile directory.
func TestBuildBrowserCandidates_EmptyProfileDefaultsToDefault(t *testing.T) {
	candidates := buildBrowserCandidates("linux", "/home/test", "", "", "")
	chromePath, ok := candidates["chrome"]
	if !ok {
		t.Fatal("missing chrome in linux candidates")
	}
	if !strings.Contains(chromePath, "Default") {
		t.Errorf("expected 'Default' in path when profile is empty, got %q", chromePath)
	}
}

// TestBuildBrowserCandidates_WindowsCustomProfile verifies that a custom
// profile is applied to Windows Chrome, Edge, and Opera paths.
func TestBuildBrowserCandidates_WindowsCustomProfile(t *testing.T) {
	candidates := buildBrowserCandidates("windows", "", `C:\AppData\Local`, `C:\AppData\Roaming`, "Profile 2")

	for _, name := range []string{"chrome", "edge", "opera"} {
		p, ok := candidates[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if !strings.Contains(p, "Profile 2") {
			t.Errorf("%s: expected 'Profile 2' in path, got %q", name, p)
		}
	}
}

// --- Environment variable sanitization tests ---

// TestDetectBrowserPaths_PoisonedHomeNullByte verifies that detectBrowserPaths
// returns a non-nil empty map (no panic, no crash) when HOME contains a null
// byte. sanitizeEnvPath should reject the poisoned value and fall back to an
// empty base path, causing all candidates to be relative and therefore not
// found by filterExistingPaths.
//
// This test is skipped on Windows because os.Setenv with a null byte is
// rejected by the Windows API, so the scenario cannot be constructed there.
func TestDetectBrowserPaths_PoisonedHomeNullByte(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows API rejects null bytes in environment variables")
	}

	emptyDir := t.TempDir()
	overridePlatformEnv(t, emptyDir)

	// Override HOME with a null-byte-poisoned value. sanitizeEnvPath should
	// discard this and treat HOME as unset.
	if err := os.Setenv("HOME", "/home/user\x00evil"); err != nil {
		t.Skipf("cannot set null-byte env var on this platform: %v", err)
	}

	result := detectBrowserPaths()
	if result == nil {
		t.Fatal("detectBrowserPaths must return non-nil map even with poisoned HOME")
	}
	// No browsers should be detected because the base path was discarded.
	if len(result) != 0 {
		t.Errorf("expected no browsers detected with poisoned HOME, got: %v", result)
	}
}

// TestDetectBrowserPaths_ResultPathsAreAbsolute verifies that all paths
// returned by detectBrowserPaths are absolute. Because sanitizeEnvPath is
// applied to the environment variables before building candidates, and
// filepath.Join with an absolute base always produces absolute results, every
// detected path must be absolute.
func TestDetectBrowserPaths_ResultPathsAreAbsolute(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	simulateBrowserInstall(t, dir)

	result := detectBrowserPaths()
	for name, path := range result {
		if !filepath.IsAbs(path) {
			t.Errorf("browser %q: detected path is not absolute: %q", name, path)
		}
	}
}

// TestResolveDBPath_AutoDetectedPathsAreAbsolute verifies that paths returned
// by resolveDBPath via browser auto-detection (no --db flag) are always
// canonical absolute paths. The sanitizePath call on each auto-detected path
// guarantees this property.
//
// resolveDBPath now calls validateChromeDB on browser-detected paths, so the
// simulated History file must contain a real Chrome schema (not just a
// placeholder byte sequence).
func TestResolveDBPath_AutoDetectedPathsAreAbsolute(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	browserName, historyPath := simulateBrowserInstall(t, dir)

	// simulateBrowserInstall writes a placeholder file (non-SQLite bytes).
	// Remove it first so createTestChromeDB can create a proper SQLite file
	// at the same path. validateChromeDB (called inside resolveDBPath) requires
	// a real Chrome-schema database, not a placeholder.
	os.Remove(historyPath)
	createTestChromeDB(t, historyPath)

	// Ensure the simulated browser is listed as valid so resolveDBPath can
	// look it up. If it is not in validBrowserNames the test would fail for
	// an unrelated reason.
	if !validBrowserNames[browserName] {
		t.Skipf("simulated browser %q not in validBrowserNames", browserName)
	}

	result, err := resolveDBPath(browserName, "", "")
	if err != nil {
		t.Fatalf("resolveDBPath(%q): unexpected error: %v", browserName, err)
	}
	if !filepath.IsAbs(result) {
		t.Errorf("resolveDBPath returned non-absolute path: %q", result)
	}
	if strings.Contains(result, "..") {
		t.Errorf("resolveDBPath returned path with traversal component: %q", result)
	}
}

// TestResolveDBPath_BrowserDetectionRejectsNonChromeDB verifies that
// resolveDBPath calls validateChromeDB for auto-detected browser paths.
// A valid SQLite file that does NOT have the Chrome History schema must be
// rejected with a clear error rather than silently accepted and later failing
// at query time.
func TestResolveDBPath_BrowserDetectionRejectsNonChromeDB(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	browserName, _ := simulateBrowserInstall(t, dir)

	// The file created by simulateBrowserInstall is not a valid SQLite
	// database. resolveDBPath must reject it via validateChromeDB.
	if !validBrowserNames[browserName] {
		t.Skipf("simulated browser %q not in validBrowserNames", browserName)
	}

	_, err := resolveDBPath(browserName, "", "")
	if err == nil {
		t.Fatal("expected resolveDBPath to reject a non-Chrome-schema database, got nil error")
	}
	// The error should mention Chrome History (from validateChromeDB).
	if !strings.Contains(strings.ToLower(err.Error()), "chrome") &&
		!strings.Contains(strings.ToLower(err.Error()), "database") &&
		!strings.Contains(strings.ToLower(err.Error()), "sqlite") {
		t.Errorf("expected error message to describe Chrome/DB problem, got: %v", err)
	}
}

// --- Additional detectBrowserPaths unit tests ---

// TestDetectBrowserPaths_OsSpecificSubdirInPath verifies that the Chrome path
// returned by detectBrowserPaths contains OS-appropriate directory components.
//
//   - Windows: "User Data" (Chrome / Edge use this dir)
//   - macOS:   "Library" and "Application Support"
//   - Linux:   ".config"
func TestDetectBrowserPaths_OsSpecificSubdirInPath(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	simulateBrowserInstall(t, dir)

	result := detectBrowserPaths()

	chromePath, ok := result["chrome"]
	if !ok {
		t.Skip("chrome not detected; skipping OS-specific path structure check")
	}

	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(chromePath, "User Data") {
			t.Errorf("Windows Chrome path must contain 'User Data', got %q", chromePath)
		}
	case "darwin":
		if !strings.Contains(chromePath, "Library") {
			t.Errorf("macOS Chrome path must contain 'Library', got %q", chromePath)
		}
		if !strings.Contains(chromePath, "Application Support") {
			t.Errorf("macOS Chrome path must contain 'Application Support', got %q", chromePath)
		}
	case "linux":
		if !strings.Contains(chromePath, ".config") {
			t.Errorf("Linux Chrome path must contain '.config', got %q", chromePath)
		}
	default:
		t.Skipf("no OS-specific subdirectory assertion defined for %s", runtime.GOOS)
	}
}

// TestDetectBrowserPaths_DefaultProfileInDetectedPath verifies that the path
// returned by detectBrowserPaths for Chrome contains the "Default" profile
// directory. detectBrowserPaths always uses the default profile; this test
// confirms that the Default directory name is present in the detected path.
func TestDetectBrowserPaths_DefaultProfileInDetectedPath(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	simulateBrowserInstall(t, dir)

	result := detectBrowserPaths()

	chromePath, ok := result["chrome"]
	if !ok {
		t.Skip("chrome not detected; skipping profile-directory check")
	}
	if !strings.Contains(chromePath, "Default") {
		t.Errorf("detected Chrome path should contain 'Default' profile directory, got %q", chromePath)
	}
}

// TestDetectBrowserPaths_OnlyDefaultProfileDetected verifies that when
// History files exist for both the Default profile and an alternate profile
// directory (e.g. "Profile 1"), only the Default path is returned by
// detectBrowserPaths. The function does not enumerate alternate profiles;
// it always searches the Default directory exclusively.
func TestDetectBrowserPaths_OnlyDefaultProfileDetected(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)

	// Install Chrome with its Default profile (the standard simulated install).
	_, defaultPath := simulateBrowserInstall(t, dir)

	// Derive the "Profile 1" path using the same logic as buildBrowserCandidates
	// so the path layout is always correct for the current platform.
	profile1Candidates := buildBrowserCandidates(runtime.GOOS, dir, dir, dir, "Profile 1")
	profile1Path, ok := profile1Candidates["chrome"]
	if !ok {
		t.Skipf("chrome has no Profile 1 candidate on %s", runtime.GOOS)
	}

	// Verify the two paths are actually different (sanity guard).
	if profile1Path == defaultPath {
		t.Skipf("Default and Profile 1 paths are identical (%q); cannot distinguish profiles", defaultPath)
	}

	// Create History file in Profile 1 as well.
	if err := os.MkdirAll(filepath.Dir(profile1Path), 0755); err != nil {
		t.Fatalf("mkdir Profile 1: %v", err)
	}
	if err := os.WriteFile(profile1Path, []byte("fake"), 0644); err != nil {
		t.Fatalf("write Profile 1 History: %v", err)
	}

	result := detectBrowserPaths()

	// detectBrowserPaths should still report Chrome via Default.
	chromePath, ok := result["chrome"]
	if !ok {
		t.Fatal("expected chrome to be detected via Default profile")
	}
	if !strings.Contains(chromePath, "Default") {
		t.Errorf("detected Chrome path should point to Default profile, got %q", chromePath)
	}
	if strings.Contains(chromePath, "Profile 1") {
		t.Errorf("detected Chrome path must not reference 'Profile 1' (detectBrowserPaths is Default-only), got %q", chromePath)
	}
}

// TestDetectBrowserPaths_TraversalInHomeEnvResolvesCleanly verifies that
// setting HOME (or equivalent) to a path containing ".." traversal sequences
// does not cause a panic or crash. sanitizeEnvPath resolves the traversal via
// filepath.Clean; the resulting path is then used for candidate construction.
// Because the cleaned traversal path points to an empty temp directory, no
// browsers should be detected.
//
// This test is skipped on Windows where USERPROFILE is used instead of HOME.
func TestDetectBrowserPaths_TraversalInHomeEnvResolvesCleanly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses USERPROFILE; traversal test is covered by overridePlatformEnv on other platforms")
	}

	// Create an isolated temp dir; all browser-related env vars point here.
	baseDir := t.TempDir()
	overridePlatformEnv(t, baseDir)

	// Override HOME with a path that contains a ".." traversal component.
	// The sub-directory must exist for filepath.Abs to succeed.
	sub := filepath.Join(baseDir, "subdir")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	// sub/.. cleans to baseDir; it is still a valid path, just with a traversal.
	traversalHome := filepath.Join(sub, "..")
	t.Setenv("HOME", traversalHome)

	// Must not panic; must return a non-nil map.
	result := detectBrowserPaths()
	if result == nil {
		t.Fatal("detectBrowserPaths must return non-nil map even when HOME contains '..' traversal")
	}
	// No browser History files were created, so the result must be empty.
	if len(result) != 0 {
		t.Errorf("expected no browsers detected when HOME traversal resolves to empty dir, got: %v", result)
	}
}

// TestDetectBrowserPaths_WindowsSeparateLocalAppDataAndAppData verifies that
// on Windows, browsers that use LOCALAPPDATA (Chrome, Edge)
// are not detected when LOCALAPPDATA points to an empty directory, even though
// APPDATA is populated with Opera's History file (Opera uses APPDATA).
//
// This test is Windows-only; it is skipped on other platforms.
func TestDetectBrowserPaths_WindowsSeparateLocalAppDataAndAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LOCALAPPDATA / APPDATA distinction is Windows-only")
	}

	// localDir is empty; no browsers installed here.
	localDir := t.TempDir()
	// appDataDir hosts Opera's History file.
	appDataDir := t.TempDir()

	t.Setenv("LOCALAPPDATA", localDir)
	t.Setenv("APPDATA", appDataDir)
	t.Setenv("USERPROFILE", t.TempDir()) // isolate home dir too

	// Create Opera's History file under APPDATA (current Opera uses a Default profile dir on Windows).
	operaPath := filepath.Join(appDataDir, "Opera Software", "Opera Stable", "Default", "History")
	if err := os.MkdirAll(filepath.Dir(operaPath), 0755); err != nil {
		t.Fatalf("mkdir opera: %v", err)
	}
	if err := os.WriteFile(operaPath, []byte("fake"), 0644); err != nil {
		t.Fatalf("write opera History: %v", err)
	}

	result := detectBrowserPaths()

	// Opera should be detected because it reads from APPDATA.
	if _, ok := result["opera"]; !ok {
		t.Error("expected opera to be detected from APPDATA")
	}

	// Chrome and Edge use LOCALAPPDATA, which is empty;
	// none of them should be detected.
	for _, name := range []string{"chrome", "edge"} {
		if path, ok := result[name]; ok {
			t.Errorf("browser %q must NOT be detected when LOCALAPPDATA is empty, got path %q", name, path)
		}
	}
}

// -----------------------------------------------------------------------------
// validateProfileDir tests
// -----------------------------------------------------------------------------

// TestValidateProfileDir_ExistingProfileFound verifies that validateProfileDir
// returns nil when the profile directory exists under at least one candidate
// browser path (no browser filter applied).
func TestValidateProfileDir_ExistingProfileFound(t *testing.T) {
	dir := t.TempDir()

	// Create the Chrome profile directory for Linux under dir.
	profileName := "Profile 1"
	profileDir := filepath.Join(dir, ".config", "google-chrome", profileName)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := validateProfileDir("linux", dir, "", "", "", profileName); err != nil {
		t.Fatalf("unexpected error for existing profile directory: %v", err)
	}
}

// TestValidateProfileDir_ExistingProfileWithBrowserFilter verifies that
// validateProfileDir returns nil when the profile directory exists under the
// specified browser's candidate path.
func TestValidateProfileDir_ExistingProfileWithBrowserFilter(t *testing.T) {
	dir := t.TempDir()

	profileName := "Profile 1"
	profileDir := filepath.Join(dir, ".config", "google-chrome", profileName)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := validateProfileDir("linux", dir, "", "", "chrome", profileName); err != nil {
		t.Fatalf("unexpected error for existing chrome Profile 1 directory: %v", err)
	}
}

// TestValidateProfileDir_MissingProfileDirectory verifies that validateProfileDir
// returns an error when no profile directory with the given name exists, and
// that the error message contains the profile name.
func TestValidateProfileDir_MissingProfileDirectory(t *testing.T) {
	dir := t.TempDir()
	// No profile directory created.

	err := validateProfileDir("linux", dir, "", "", "", "Profile 99")
	if err == nil {
		t.Fatal("expected error for missing profile directory")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Profile 99") {
		t.Errorf("expected profile name 'Profile 99' in error message, got: %v", err)
	}
}

// TestValidateProfileDir_MissingProfileWithBrowserFilter verifies that the
// error message mentions both the profile name and browser name when a
// browser filter is specified and the profile directory does not exist.
func TestValidateProfileDir_MissingProfileWithBrowserFilter(t *testing.T) {
	dir := t.TempDir()
	// No profile directory created.

	err := validateProfileDir("linux", dir, "", "", "chrome", "Profile 99")
	if err == nil {
		t.Fatal("expected error for missing profile directory with browser filter")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Profile 99") {
		t.Errorf("expected profile name in error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "chrome") {
		t.Errorf("expected browser name in error message, got: %v", err)
	}
}

// TestValidateProfileDir_UnsupportedBrowserOnPlatform verifies that
// validateProfileDir returns a "not supported on this platform" error when
// the specified browser has no candidate paths on the given OS.
// "chromium" is only supported on Linux; using it on "windows" produces no
// candidate entries in buildBrowserCandidates.
func TestValidateProfileDir_UnsupportedBrowserOnPlatform(t *testing.T) {
	dir := t.TempDir()

	err := validateProfileDir("windows", "", dir, dir, "chromium", "Default")
	if err == nil {
		t.Fatal("expected error for browser not supported on platform")
	}
	if !strings.Contains(err.Error(), "not supported on this platform") {
		t.Errorf("expected 'not supported on this platform' in error, got: %v", err)
	}
}

// TestValidateProfileDir_DefaultProfileExists verifies that the "Default"
// profile name is found correctly on Linux.
func TestValidateProfileDir_DefaultProfileExists(t *testing.T) {
	dir := t.TempDir()
	profileDir := filepath.Join(dir, ".config", "google-chrome", "Default")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := validateProfileDir("linux", dir, "", "", "chrome", "Default"); err != nil {
		t.Fatalf("unexpected error for existing Default profile: %v", err)
	}
}

// TestValidateProfileDir_FileInsteadOfDirectory verifies that a file at the
// profile path (rather than a directory) is treated as "not found". Chrome
// profile directories must be real directories.
func TestValidateProfileDir_FileInsteadOfDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a file at the profile directory path instead of a directory.
	profileParent := filepath.Join(dir, ".config", "google-chrome")
	if err := os.MkdirAll(profileParent, 0755); err != nil {
		t.Fatal(err)
	}
	profileFile := filepath.Join(profileParent, "Profile 1")
	if err := os.WriteFile(profileFile, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	err := validateProfileDir("linux", dir, "", "", "chrome", "Profile 1")
	if err == nil {
		t.Fatal("expected error when profile path is a file, not a directory")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error for file-at-profile-path, got: %v", err)
	}
}

// TestValidateProfileDir_WindowsProfileExists verifies that validateProfileDir
// correctly resolves Chrome profile paths on Windows using LOCALAPPDATA-based
// path construction.
func TestValidateProfileDir_WindowsProfileExists(t *testing.T) {
	dir := t.TempDir()
	profileName := "Profile 1"

	// On Windows, Chrome profile is under <LOCALAPPDATA>\Google\Chrome\User Data\<profile>.
	profileDir := filepath.Join(dir, "Google", "Chrome", "User Data", profileName)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := validateProfileDir("windows", "", dir, dir, "chrome", profileName); err != nil {
		t.Fatalf("unexpected error for Windows Chrome Profile 1: %v", err)
	}
}

// TestValidateProfileDir_DarwinProfileExists verifies that validateProfileDir
// correctly resolves Chrome profile paths on macOS (darwin).
func TestValidateProfileDir_DarwinProfileExists(t *testing.T) {
	dir := t.TempDir()
	profileName := "Profile 1"

	// On macOS, Chrome profile is under ~/Library/Application Support/Google/Chrome/<profile>.
	profileDir := filepath.Join(dir, "Library", "Application Support", "Google", "Chrome", profileName)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := validateProfileDir("darwin", dir, "", "", "chrome", profileName); err != nil {
		t.Fatalf("unexpected error for macOS Chrome Profile 1: %v", err)
	}
}

// TestValidateProfileDir_AnyBrowserMatchSuffices verifies that when no browser
// filter is given, a profile directory found under ANY supported browser is
// sufficient for validateProfileDir to return nil.
func TestValidateProfileDir_AnyBrowserMatchSuffices(t *testing.T) {
	dir := t.TempDir()
	profileName := "Profile 1"

	// Create the Edge profile directory but not the Chrome one.
	edgeProfileDir := filepath.Join(dir, ".config", "microsoft-edge", profileName)
	if err := os.MkdirAll(edgeProfileDir, 0755); err != nil {
		t.Fatal(err)
	}

	// With no browser filter, finding Edge's profile directory is enough.
	if err := validateProfileDir("linux", dir, "", "", "", profileName); err != nil {
		t.Fatalf("unexpected error: expected Edge profile directory to satisfy the check: %v", err)
	}
}

// TestResolveDBPath_NonExistentProfileReturnsProfileError is an integration
// test verifying that resolveDBPath returns a clear, profile-specific error
// when --profile is specified but the directory does not exist on disk, rather
// than the confusing generic "browser not found" error.
func TestResolveDBPath_NonExistentProfileReturnsProfileError(t *testing.T) {
	dir := t.TempDir()
	overridePlatformEnv(t, dir)
	// No browser directory structure created; profile does not exist.

	_, err := resolveDBPath("", "", "Profile 99")
	if err == nil {
		t.Fatal("expected error for non-existent profile directory")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Profile 99") {
		t.Errorf("expected profile name 'Profile 99' in error message, got: %v", err)
	}
}

// TestResolveDBPath_ProfileIgnoredWhenCustomDBSupplied verifies that when the
// user supplies --db (customPath), the --profile flag is ignored and no profile
// directory check is performed. The custom path takes precedence.
func TestResolveDBPath_ProfileIgnoredWhenCustomDBSupplied(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "History")
	createTestChromeDB(t, dbPath)

	// Pass a non-existent profile name; with --db the profile check must be
	// skipped and the custom path used directly.
	result, err := resolveDBPath("", dbPath, "NonExistentProfile")
	if err != nil {
		t.Fatalf("unexpected error: profile should be ignored when --db is supplied: %v", err)
	}
	if result != dbPath {
		t.Errorf("expected custom DB path %q, got %q", dbPath, result)
	}
}
