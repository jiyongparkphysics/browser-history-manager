package history

import (
	"os"
	"path/filepath"
	"testing"
)

// --- isProfileDir tests ---

func TestIsProfileDir(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Default", false},
		{"Profile 1", true},
		{"Profile 2", true},
		{"Profile 10", true},
		{"Profile 123", true},
		{"Profile ", false},  // no digits
		{"Profile X", false}, // non-digit suffix
		{"ProfileX", false},  // missing space
		{"System Profile", false},
		{"Guest Profile", false},
		{"", false},
		{"SomeDir", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProfileDir(tt.name)
			if got != tt.want {
				t.Errorf("isProfileDir(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// --- browserUserDataDir tests ---

func TestBrowserUserDataDir_Windows(t *testing.T) {
	dir, ok := browserUserDataDir("windows", `C:\Users\test`, `C:\Users\test\AppData\Local`, `C:\Users\test\AppData\Roaming`, "brave")
	if !ok {
		t.Fatal("expected ok=true for brave on windows")
	}
	expected := filepath.Join(`C:\Users\test\AppData\Local`, "BraveSoftware", "Brave-Browser", "User Data")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}

	dir, ok = browserUserDataDir("windows", `C:\Users\test`, `C:\Users\test\AppData\Local`, `C:\Users\test\AppData\Roaming`, "chrome")
	if !ok {
		t.Fatal("expected ok=true for chrome on windows")
	}
	expected = filepath.Join(`C:\Users\test\AppData\Local`, "Google", "Chrome", "User Data")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}

	dir, ok = browserUserDataDir("windows", `C:\Users\test`, `C:\Users\test\AppData\Local`, `C:\Users\test\AppData\Roaming`, "opera")
	if !ok {
		t.Fatal("expected ok=true for opera on windows")
	}
	expected = filepath.Join(`C:\Users\test\AppData\Roaming`, "Opera Software", "Opera Stable")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}
}

func TestBrowserUserDataDir_Linux(t *testing.T) {
	dir, ok := browserUserDataDir("linux", "/home/user", "", "", "brave")
	if !ok {
		t.Fatal("expected ok=true for brave on linux")
	}
	expected := filepath.Join("/home/user", ".config", "BraveSoftware", "Brave-Browser")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}

	dir, ok = browserUserDataDir("linux", "/home/user", "", "", "chrome")
	if !ok {
		t.Fatal("expected ok=true for chrome on linux")
	}
	expected = filepath.Join("/home/user", ".config", "google-chrome")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}

	dir, ok = browserUserDataDir("linux", "/home/user", "", "", "chromium")
	if !ok {
		t.Fatal("expected ok=true for chromium on linux")
	}
	expected = filepath.Join("/home/user", ".config", "chromium")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}
}

func TestBrowserUserDataDir_Darwin(t *testing.T) {
	dir, ok := browserUserDataDir("darwin", "/Users/user", "", "", "brave")
	if !ok {
		t.Fatal("expected ok=true for brave on darwin")
	}
	expected := filepath.Join("/Users/user", "Library", "Application Support", "BraveSoftware", "Brave-Browser")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}

	dir, ok = browserUserDataDir("darwin", "/Users/user", "", "", "chrome")
	if !ok {
		t.Fatal("expected ok=true for chrome on darwin")
	}
	expected = filepath.Join("/Users/user", "Library", "Application Support", "Google", "Chrome")
	if dir != expected {
		t.Errorf("got %q, want %q", dir, expected)
	}
}

func TestBrowserUserDataDir_UnsupportedOS(t *testing.T) {
	_, ok := browserUserDataDir("freebsd", "/home/user", "", "", "chrome")
	if ok {
		t.Error("expected ok=false for unsupported OS")
	}
}

func TestBrowserUserDataDir_UnknownBrowser(t *testing.T) {
	_, ok := browserUserDataDir("windows", `C:\Users\test`, `C:\local`, `C:\roaming`, "firefox")
	if ok {
		t.Error("expected ok=false for unsupported browser")
	}
}

// --- ListProfiles tests ---

// createFakeProfile creates a profile directory with a fake History file
// under the given user data directory.
func createFakeProfile(t *testing.T, userDataDir, profileName string) string {
	t.Helper()
	histPath := filepath.Join(userDataDir, profileName, "History")
	if err := os.MkdirAll(filepath.Dir(histPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(histPath, []byte("fake-history"), 0o644); err != nil {
		t.Fatal(err)
	}
	return histPath
}

func TestListProfiles_MultipleProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	// Simulate Windows layout: tmpDir acts as LOCALAPPDATA.
	userData := filepath.Join(tmpDir, "Google", "Chrome", "User Data")

	defaultPath := createFakeProfile(t, userData, "Default")
	profile1Path := createFakeProfile(t, userData, "Profile 1")
	profile2Path := createFakeProfile(t, userData, "Profile 2")

	// Create a non-profile directory that should be ignored.
	os.MkdirAll(filepath.Join(userData, "Crashpad"), 0o755)
	os.MkdirAll(filepath.Join(userData, "System Profile"), 0o755)

	profiles := ListProfiles("windows", "", tmpDir, "", "chrome")
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d: %+v", len(profiles), profiles)
	}

	// Should be sorted: Default, Profile 1, Profile 2.
	wantNames := []string{"Default", "Profile 1", "Profile 2"}
	wantPaths := []string{defaultPath, profile1Path, profile2Path}
	for i, p := range profiles {
		if p.Name != wantNames[i] {
			t.Errorf("profiles[%d].Name = %q, want %q", i, p.Name, wantNames[i])
		}
		if p.DBPath != wantPaths[i] {
			t.Errorf("profiles[%d].DBPath = %q, want %q", i, p.DBPath, wantPaths[i])
		}
	}
}

func TestListProfiles_NoProfilesFound(t *testing.T) {
	tmpDir := t.TempDir()
	// Create user data dir but no profiles with History files.
	userData := filepath.Join(tmpDir, "Google", "Chrome", "User Data")
	os.MkdirAll(userData, 0o755)

	profiles := ListProfiles("windows", "", tmpDir, "", "chrome")
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestListProfiles_NonExistentUserDataDir(t *testing.T) {
	profiles := ListProfiles("windows", "", "/nonexistent/path", "", "chrome")
	if profiles != nil {
		t.Errorf("expected nil for nonexistent dir, got %+v", profiles)
	}
}

func TestListProfiles_ProfileDirWithoutHistory(t *testing.T) {
	tmpDir := t.TempDir()
	userData := filepath.Join(tmpDir, "Google", "Chrome", "User Data")

	// Create Default profile with History.
	createFakeProfile(t, userData, "Default")
	// Create Profile 1 without History file.
	os.MkdirAll(filepath.Join(userData, "Profile 1"), 0o755)

	profiles := ListProfiles("windows", "", tmpDir, "", "chrome")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile (only Default has History), got %d", len(profiles))
	}
	if profiles[0].Name != "Default" {
		t.Errorf("expected Default profile, got %q", profiles[0].Name)
	}
}

func TestListProfiles_LinuxChrome(t *testing.T) {
	tmpDir := t.TempDir()
	userData := filepath.Join(tmpDir, ".config", "google-chrome")

	createFakeProfile(t, userData, "Default")
	createFakeProfile(t, userData, "Profile 1")

	profiles := ListProfiles("linux", tmpDir, "", "", "chrome")
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestListProfiles_DarwinEdge(t *testing.T) {
	tmpDir := t.TempDir()
	userData := filepath.Join(tmpDir, "Library", "Application Support", "Microsoft Edge")

	createFakeProfile(t, userData, "Default")

	profiles := ListProfiles("darwin", tmpDir, "", "", "edge")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "Default" {
		t.Errorf("expected Default, got %q", profiles[0].Name)
	}
}

func TestListProfiles_OperaWindows(t *testing.T) {
	tmpDir := t.TempDir()
	operaDir := filepath.Join(tmpDir, "Opera Software", "Opera Stable")
	histPath := filepath.Join(operaDir, "Default", "History")
	if err := os.MkdirAll(filepath.Dir(histPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(histPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles := ListProfiles("windows", "", "", tmpDir, "opera")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile for Opera, got %d", len(profiles))
	}
	if profiles[0].Name != "Default" {
		t.Errorf("expected profile name Default, got %q", profiles[0].Name)
	}
	if profiles[0].DBPath != histPath {
		t.Errorf("expected DBPath %q, got %q", histPath, profiles[0].DBPath)
	}
}

func TestListProfiles_OperaWindowsLegacyFlatPathIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	operaDir := filepath.Join(tmpDir, "Opera Software", "Opera Stable")
	histPath := filepath.Join(operaDir, "History")
	if err := os.MkdirAll(operaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(histPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles := ListProfiles("windows", "", "", tmpDir, "opera")
	if len(profiles) != 0 {
		t.Fatalf("expected legacy flat Opera path to be ignored, got %d profiles", len(profiles))
	}
}

// --- App-level ListBrowsersWithProfiles test is in cmd/gui/app_test.go ---

