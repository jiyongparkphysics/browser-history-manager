package history

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// --- AutoDetectBrowser / AutoDetectBrowserFrom tests ---

func TestAutoDetectBrowserFrom_PrefersChrome(t *testing.T) {
	dir := t.TempDir()

	// Create fake History files for chrome and edge.
	chromePath := filepath.Join(dir, "chrome", "History")
	edgePath := filepath.Join(dir, "edge", "History")
	for _, p := range []string{chromePath, edgePath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := map[string]string{
		"chrome": chromePath,
		"edge":   edgePath,
	}

	result, err := AutoDetectBrowserFrom(candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "chrome" {
		t.Errorf("expected chrome to be prioritised, got %q", result.Name)
	}
	if result.DBPath != chromePath {
		t.Errorf("expected DBPath %q, got %q", chromePath, result.DBPath)
	}
}

func TestAutoDetectBrowserFrom_FallsBackAlphabetically(t *testing.T) {
	dir := t.TempDir()

	// Create fake History files for edge and opera (no chrome).
	edgePath := filepath.Join(dir, "edge", "History")
	operaPath := filepath.Join(dir, "opera", "History")
	for _, p := range []string{edgePath, operaPath} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	candidates := map[string]string{
		"edge":  edgePath,
		"opera": operaPath,
	}

	result, err := AutoDetectBrowserFrom(candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "edge" < "opera" alphabetically.
	if result.Name != "edge" {
		t.Errorf("expected edge (first alphabetically), got %q", result.Name)
	}
	if result.DBPath != edgePath {
		t.Errorf("expected DBPath %q, got %q", edgePath, result.DBPath)
	}
}

func TestAutoDetectBrowserFrom_NoCandidates(t *testing.T) {
	candidates := map[string]string{}
	_, err := AutoDetectBrowserFrom(candidates)
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
}

func TestAutoDetectBrowserFrom_NonExistentPaths(t *testing.T) {
	candidates := map[string]string{
		"chrome": "/nonexistent/path/History",
		"edge":   "/also/nonexistent/History",
	}
	_, err := AutoDetectBrowserFrom(candidates)
	if err == nil {
		t.Fatal("expected error when no paths exist on disk")
	}
}

func TestAutoDetectBrowserFrom_SingleBrowser(t *testing.T) {
	dir := t.TempDir()
	operaPath := filepath.Join(dir, "opera", "History")
	if err := os.MkdirAll(filepath.Dir(operaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(operaPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := map[string]string{
		"opera": operaPath,
	}

	result, err := AutoDetectBrowserFrom(candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "opera" {
		t.Errorf("expected opera, got %q", result.Name)
	}
}

func TestAutoDetectBrowser_DoesNotPanic(t *testing.T) {
	// AutoDetectBrowser reads the real filesystem. On CI it may return
	// an error (no browser installed) but must not panic.
	result, err := AutoDetectBrowser()
	if err != nil {
		// Expected on CI; just verify it's the right error.
		if result != nil {
			t.Error("expected nil result when error is returned")
		}
		return
	}
	if result.Name == "" {
		t.Error("non-error result should have a browser name")
	}
	if result.DBPath == "" {
		t.Error("non-error result should have a DBPath")
	}
}

// --- BrowserDetection struct tests ---

func TestBrowserDetection_Fields(t *testing.T) {
	d := &BrowserDetection{Name: "chrome", DBPath: "/some/path/History"}
	if d.Name != "chrome" {
		t.Errorf("Name = %q, want chrome", d.Name)
	}
	if d.DBPath != "/some/path/History" {
		t.Errorf("DBPath = %q, want /some/path/History", d.DBPath)
	}
}

// --- Existing function tests (unit tests for shared code) ---

func TestValidateBrowserName_Valid(t *testing.T) {
	for _, name := range []string{"brave", "chrome", "chromium", "edge", "opera"} {
		if err := ValidateBrowserName(name); err != nil {
			t.Errorf("expected %q to be valid, got: %v", name, err)
		}
	}
}

func TestValidateBrowserName_Invalid(t *testing.T) {
	for _, name := range []string{"firefox", "safari", "", "unknown"} {
		if err := ValidateBrowserName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestBuildBrowserCandidates_DefaultProfile(t *testing.T) {
	c := BuildBrowserCandidates("windows", `C:\Users\test`, `C:\Users\test\AppData\Local`, `C:\Users\test\AppData\Roaming`, "")
	if c["brave"] == "" {
		t.Error("expected brave candidate on windows")
	}
	if c["chrome"] == "" {
		t.Error("expected chrome candidate on windows")
	}
	if c["opera"] == "" {
		t.Error("expected opera candidate on windows")
	}
	// Should use "Default" profile.
	if !contains(c["brave"], filepath.Join("BraveSoftware", "Brave-Browser", "User Data", "Default", "History")) {
		t.Errorf("expected Brave Default profile path, got %q", c["brave"])
	}
	if !contains(c["chrome"], "Default") {
		t.Errorf("expected Default profile in path, got %q", c["chrome"])
	}
	if !contains(c["opera"], filepath.Join("Opera Stable", "Default", "History")) {
		t.Errorf("expected Opera Default profile path, got %q", c["opera"])
	}
}

func TestBuildBrowserCandidates_CustomProfile(t *testing.T) {
	c := BuildBrowserCandidates("linux", "/home/user", "", "", "Profile 1")
	if !contains(c["chrome"], "Profile 1") {
		t.Errorf("expected 'Profile 1' in path, got %q", c["chrome"])
	}
}

func TestFilterExistingPaths_FiltersNonExistent(t *testing.T) {
	dir := t.TempDir()
	existingPath := filepath.Join(dir, "History")
	if err := os.WriteFile(existingPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := map[string]string{
		"exists":    existingPath,
		"notexists": filepath.Join(dir, "nonexistent"),
	}

	result := FilterExistingPaths(candidates)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if _, ok := result["exists"]; !ok {
		t.Error("expected 'exists' key in result")
	}
}

func TestDetectBrowserPaths_DoesNotPanic(t *testing.T) {
	// Just ensure it doesn't panic on any platform.
	paths := DetectBrowserPaths()
	if paths == nil {
		t.Error("DetectBrowserPaths returned nil (should return empty map)")
	}
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && sort.SearchStrings([]string{s}, substr) >= 0 || containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
