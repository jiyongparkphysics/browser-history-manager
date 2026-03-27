// Package history - browser.go: browser detection and History DB path resolution
// for the Chrome History manager.
//
// # Responsibility
//
// This file owns everything related to locating a Chromium-based browser's
// History SQLite database on the host machine:
//
//   - ValidBrowserNames / ValidateBrowserName: allowlist for the --browser flag.
//   - BuildBrowserCandidates: OS-specific (Windows/macOS/Linux) profile path
//     construction. All platform branching is confined to this single function.
//   - FilterExistingPaths: reduces candidates to those that exist on disk.
//   - DetectBrowserPaths: top-level detection that combines the two helpers above.
//   - ResolveDBPath: resolves the final database path from --browser / --db flags,
//     falling back to Chrome then any detected browser.
//
// # OS-Specific Logic
//
// All platform-specific path construction is isolated inside
// BuildBrowserCandidates. It accepts goos, home, localAppData, and appData as
// plain string parameters so it can be tested deterministically without
// modifying real environment variables or relying on the current OS.
//
// # Dependency Notes
//
// ResolveDBPath calls ValidateDBPath (sanitize.go) for path traversal checks
// and ValidateChromeDB (db.go) for schema verification. These cross-file
// calls are intentional: path and DB validation are not browser-specific
// concerns and belong in their own layers.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ValidBrowserNames lists all accepted --browser flag values.
// This must stay in sync with the browser names handled by BuildBrowserCandidates.
var ValidBrowserNames = map[string]bool{
	"brave":    true,
	"chrome":   true,
	"chromium": true,
	"edge":     true,
	"opera":    true,
}

// ValidateBrowserName checks that the given browser name is a recognised
// --browser value. It returns an error listing valid names on failure.
func ValidateBrowserName(browser string) error {
	lower := strings.ToLower(browser)
	if !ValidBrowserNames[lower] {
		names := make([]string, 0, len(ValidBrowserNames))
		for n := range ValidBrowserNames {
			names = append(names, n)
		}
		sort.Strings(names) // deterministic error message
		return fmt.Errorf("invalid --browser value '%s'; must be one of: %s",
			browser, strings.Join(names, ", "))
	}
	return nil
}

// BuildBrowserCandidates returns candidate browser paths for the given OS,
// home directory, and Windows app-data directories. It does NOT check
// whether the paths actually exist on disk.
//
// profile specifies the Chrome-style profile directory name (e.g. "Default",
// "Profile 1"). An empty string defaults to "Default".
func BuildBrowserCandidates(goos, home, localAppData, appData, profile string) map[string]string {
	if profile == "" {
		profile = "Default"
	}

	browsers := make(map[string]string)

	switch goos {
	case "windows":
		browsers["brave"] = filepath.Join(braveWindowsUserDataDir(localAppData), profile, "History")
		browsers["chrome"] = filepath.Join(localAppData, "Google", "Chrome", "User Data", profile, "History")
		browsers["edge"] = filepath.Join(localAppData, "Microsoft", "Edge", "User Data", profile, "History")
		browsers["opera"] = filepath.Join(operaWindowsUserDataDir(appData), profile, "History")
	case "darwin":
		browsers["brave"] = filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", profile, "History")
		browsers["chrome"] = filepath.Join(home, "Library", "Application Support", "Google", "Chrome", profile, "History")
		browsers["edge"] = filepath.Join(home, "Library", "Application Support", "Microsoft Edge", profile, "History")
	case "linux":
		browsers["brave"] = filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", profile, "History")
		browsers["chrome"] = filepath.Join(home, ".config", "google-chrome", profile, "History")
		browsers["chromium"] = filepath.Join(home, ".config", "chromium", profile, "History")
		browsers["edge"] = filepath.Join(home, ".config", "microsoft-edge", profile, "History")
	}

	return browsers
}

// BrowserProfile describes a single Chrome-style profile directory that
// contains a History database.
type BrowserProfile struct {
	// Name is the profile directory name (e.g. "Default", "Profile 1").
	Name string
	// DBPath is the absolute path to the History file inside this profile.
	DBPath string
}

// BrowserInfo describes a detected browser and all its discovered profiles.
type BrowserInfo struct {
	// Name is the browser identifier (e.g. "chrome", "edge", "opera").
	Name string
	// Profiles lists all profile directories that contain a History database.
	Profiles []BrowserProfile
}

func operaWindowsUserDataDir(appData string) string {
	return filepath.Join(appData, "Opera Software", "Opera Stable")
}

func braveWindowsUserDataDir(localAppData string) string {
	return filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data")
}



// browserUserDataDir returns the "User Data" directory for a given browser
// on the specified OS. This is the parent directory that contains profile
// subdirectories (Default, Profile 1, Profile 2, etc.).
//
// Returns ("", false) when the browser/OS combination is not supported.
func browserUserDataDir(goos, home, localAppData, appData, browser string) (string, bool) {
	switch goos {
	case "windows":
		switch browser {
		case "brave":
			return braveWindowsUserDataDir(localAppData), true
		case "chrome":
			return filepath.Join(localAppData, "Google", "Chrome", "User Data"), true
		case "edge":
			return filepath.Join(localAppData, "Microsoft", "Edge", "User Data"), true
		case "opera":
			return operaWindowsUserDataDir(appData), true
		}
	case "darwin":
		switch browser {
		case "brave":
			return filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser"), true
		case "chrome":
			return filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), true
		case "edge":
			return filepath.Join(home, "Library", "Application Support", "Microsoft Edge"), true
		}
	case "linux":
		switch browser {
		case "brave":
			return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser"), true
		case "chrome":
			return filepath.Join(home, ".config", "google-chrome"), true
		case "chromium":
			return filepath.Join(home, ".config", "chromium"), true
		case "edge":
			return filepath.Join(home, ".config", "microsoft-edge"), true
		}
	}
	return "", false
}

// ListProfiles enumerates profile directories for a specific browser that
// contain a History database. The returned profiles are sorted by name.
func ListProfiles(goos, home, localAppData, appData, browser string) []BrowserProfile {
	userDataDir, hasProfiles := browserUserDataDir(goos, home, localAppData, appData, browser)
	if !hasProfiles {
		return nil
	}

	entries, err := os.ReadDir(userDataDir)
	if err != nil {
		return nil
	}

	var profiles []BrowserProfile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Chrome profiles are named "Default" or "Profile N".
		if name != "Default" && !isProfileDir(name) {
			continue
		}
		histPath := filepath.Join(userDataDir, name, "History")
		if _, err := os.Stat(histPath); err == nil {
			profiles = append(profiles, BrowserProfile{
				Name:   name,
				DBPath: histPath,
			})
		}
	}




	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles
}

// isProfileDir returns true if the directory name matches Chrome's profile
// naming convention: "Profile " followed by one or more digits.
func isProfileDir(name string) bool {
	const prefix = "Profile "
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	if rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ListBrowsersWithProfiles returns all detected browsers on the current
// system along with their discovered profile directories. Browsers with
// no profiles (i.e. no History files found) are omitted. The result is
// sorted alphabetically by browser name.
func ListBrowsersWithProfiles() []BrowserInfo {
	home, _ := os.UserHomeDir()
	safeHome := SanitizeEnvPath(home)
	safeLocal := SanitizeEnvPath(os.Getenv("LOCALAPPDATA"))
	safeAppData := SanitizeEnvPath(os.Getenv("APPDATA"))

	names := make([]string, 0, len(ValidBrowserNames))
	for n := range ValidBrowserNames {
		names = append(names, n)
	}
	sort.Strings(names)

	var result []BrowserInfo
	for _, name := range names {
		profiles := ListProfiles(runtime.GOOS, safeHome, safeLocal, safeAppData, name)
		if len(profiles) > 0 {
			result = append(result, BrowserInfo{
				Name:     name,
				Profiles: profiles,
			})
		}
	}
	return result
}

// FilterExistingPaths returns only the entries whose paths exist on disk.
func FilterExistingPaths(candidates map[string]string) map[string]string {
	found := make(map[string]string)
	for name, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			found[name] = path
		}
	}
	return found
}

// ValidateProfileDir checks that the given Chrome profile directory exists
// under at least one candidate browser installation on the current OS.
func ValidateProfileDir(goos, home, localAppData, appData, browser, profile string) error {
	candidates := BuildBrowserCandidates(goos, home, localAppData, appData, profile)

	checked := 0
	for name, histPath := range candidates {
		if browser != "" && strings.ToLower(browser) != name {
			continue
		}
		profileDir := filepath.Dir(histPath)
		checked++
		info, err := os.Stat(profileDir)
		if err == nil && info.IsDir() {
			return nil
		}
	}

	if checked == 0 {
		return fmt.Errorf("browser %q is not supported on this platform; cannot validate profile directory", browser)
	}

	if browser != "" {
		return fmt.Errorf(
			"profile directory %q not found for browser %q; "+
				"verify the profile name with 'browser-history-manager browsers' or check your installation",
			profile, browser)
	}
	return fmt.Errorf(
		"profile directory %q not found for any supported browser; "+
			"verify the profile name with 'browser-history-manager browsers' or check your installation",
		profile)
}

// DetectBrowserPaths returns a map of browser name to History DB path
// for all Chromium-based browsers found on the current platform using
// the default profile ("Default").
func DetectBrowserPaths() map[string]string {
	home, _ := os.UserHomeDir()
	candidates := BuildBrowserCandidates(
		runtime.GOOS,
		SanitizeEnvPath(home),
		SanitizeEnvPath(os.Getenv("LOCALAPPDATA")),
		SanitizeEnvPath(os.Getenv("APPDATA")),
		"", // use default profile
	)
	return FilterExistingPaths(candidates)
}

// ResolveDBPath determines the History DB path based on the browser name,
// profile name, or a custom path provided by the user. Falls back to Chrome,
// then any detected browser if no specific option is given.
//
// profile specifies the Chrome profile directory name (e.g. "Default",
// "Profile 1"). An empty string falls back to "Default". profile is ignored
// when customPath is non-empty (the user supplied the exact DB path).
//
// All returned paths, whether from --db, --browser, or auto-detection, are
// run through SanitizePath so that callers always receive a canonical absolute
// path free of traversal sequences, null bytes, and control characters.
func ResolveDBPath(browser string, customPath string, profile string) (string, error) {
	if customPath != "" {
		safePath, err := ValidateDBPath(customPath)
		if err != nil {
			return "", err
		}
		if err := ValidateChromeDB(safePath); err != nil {
			return "", fmt.Errorf("invalid --db file: %w", err)
		}
		return safePath, nil
	}

	home, _ := os.UserHomeDir()
	safeHome := SanitizeEnvPath(home)
	safeLocal := SanitizeEnvPath(os.Getenv("LOCALAPPDATA"))
	safeAppData := SanitizeEnvPath(os.Getenv("APPDATA"))

	if profile != "" {
		if err := ValidateProfileDir(runtime.GOOS, safeHome, safeLocal, safeAppData, browser, profile); err != nil {
			return "", err
		}
	}

	candidates := BuildBrowserCandidates(
		runtime.GOOS,
		safeHome,
		safeLocal,
		safeAppData,
		profile,
	)
	browsers := FilterExistingPaths(candidates)

	if browser != "" {
		path, ok := browsers[strings.ToLower(browser)]
		if !ok {
			return "", fmt.Errorf("browser '%s' not found. check installation or use --db", browser)
		}
		safePath, err := SanitizePath(path)
		if err != nil {
			return "", fmt.Errorf("browser path is invalid: %w", err)
		}
		if err := ValidateChromeDB(safePath); err != nil {
			return "", fmt.Errorf("detected browser database is not a valid Chrome History file: %w", err)
		}
		return safePath, nil
	}

	if path, ok := browsers["chrome"]; ok {
		safePath, err := SanitizePath(path)
		if err != nil {
			return "", fmt.Errorf("browser path is invalid: %w", err)
		}
		if err := ValidateChromeDB(safePath); err != nil {
			return "", fmt.Errorf("detected Chrome database is not a valid Chrome History file: %w", err)
		}
		return safePath, nil
	}

	for name, path := range browsers {
		fmt.Printf("Chrome not found, using %s instead.\n", name)
		safePath, err := SanitizePath(path)
		if err != nil {
			return "", fmt.Errorf("browser path is invalid: %w", err)
		}
		if err := ValidateChromeDB(safePath); err != nil {
			return "", fmt.Errorf("detected %s database is not a valid Chrome History file: %w", name, err)
		}
		return safePath, nil
	}

	return "", fmt.Errorf("no Chromium-based browser found")
}

// BrowserDetection holds the result of auto-detecting a browser.
type BrowserDetection struct {
	// Name is the browser identifier (e.g. "chrome", "edge", "opera").
	Name string
	// DBPath is the absolute path to the browser's History SQLite database.
	DBPath string
}

// AutoDetectBrowser discovers installed Chromium-based browsers on the
// current system, prioritises Chrome, and returns the detected browser's
// name and History database path. If Chrome is not installed, the first
// available alternative is returned. An error is returned when no
// compatible browser can be found.
//
// Unlike ResolveDBPath, this function does not perform schema validation
// or path sanitisation; it is a lightweight "which browser is available?"
// check intended for the GUI's initial startup where the user can then
// refine the selection.
func AutoDetectBrowser() (*BrowserDetection, error) {
	browsers := DetectBrowserPaths()
	if len(browsers) == 0 {
		return nil, fmt.Errorf("no Chromium-based browser found")
	}

	// Prefer Chrome when available.
	if path, ok := browsers["chrome"]; ok {
		return &BrowserDetection{Name: "chrome", DBPath: path}, nil
	}

	// Deterministic fallback: pick the first browser alphabetically.
	names := make([]string, 0, len(browsers))
	for name := range browsers {
		names = append(names, name)
	}
	sort.Strings(names)

	return &BrowserDetection{Name: names[0], DBPath: browsers[names[0]]}, nil
}

// AutoDetectBrowserFrom is a testable variant of AutoDetectBrowser that
// accepts pre-built candidate paths instead of reading the environment.
// It applies the same Chrome-first priority logic.
func AutoDetectBrowserFrom(candidates map[string]string) (*BrowserDetection, error) {
	existing := FilterExistingPaths(candidates)
	if len(existing) == 0 {
		return nil, fmt.Errorf("no Chromium-based browser found")
	}

	if path, ok := existing["chrome"]; ok {
		return &BrowserDetection{Name: "chrome", DBPath: path}, nil
	}

	names := make([]string, 0, len(existing))
	for name := range existing {
		names = append(names, name)
	}
	sort.Strings(names)

	return &BrowserDetection{Name: names[0], DBPath: existing[names[0]]}, nil
}

