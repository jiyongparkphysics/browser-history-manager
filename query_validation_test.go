package main

// query_validation_test.go — Tests for search/query string parameter
// validation: length limits, control-character rejection, and SQL wildcard
// abuse prevention (% and _ density limits).
//
// Security model context (see also db.go and sanitize.go):
//   - Search keywords (--match / --protect) are used exclusively for
//     in-memory substring matching inside filterEntries. They never appear in
//     any SQL query.
//   - SQL wildcard limits are therefore defence-in-depth: they prevent
//     wildcard bombing in the current system and ensure safe behaviour if
//     SQL LIKE-based searches are ever introduced.
//   - validateKeyword is called for every keyword after splitting, covering
//     both the comma-separated inline path and the single-keyword path in
//     loadFilters.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// validateKeyword — basic acceptance tests
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateKeyword_EmptyStringAccepted verifies that an empty keyword is
// accepted. Empty strings arise when a comma-separated filter value contains
// a trailing comma; callers filter them out before calling validateKeyword,
// but the function must not panic or error on an empty input.
func TestValidateKeyword_EmptyStringAccepted(t *testing.T) {
	if err := validateKeyword(""); err != nil {
		t.Fatalf("validateKeyword(%q): unexpected error: %v", "", err)
	}
}

// TestValidateKeyword_ValidKeywordsAccepted verifies that a broad set of
// typical search keywords (domain names, URL fragments, mixed-case strings,
// Unicode, and keywords containing a small number of % or _ characters) are
// all accepted.
func TestValidateKeyword_ValidKeywordsAccepted(t *testing.T) {
	valid := []string{
		"google.com",
		"github.com/user/repo",
		"search?q=golang",
		"hello world",
		"*",
		"café",
		"한국어",
		"my_profile",            // single underscore — common in URLs
		"file%20name",           // single percent-encoded space — common in URLs
		"a%20b%20c%20d",         // four percent signs — well within limit
		strings.Repeat("_", 20), // exactly maxSQLWildcardCount underscores
		strings.Repeat("%", 20), // exactly maxSQLWildcardCount percent signs
		"google_%",              // mix within limit
		"example.com/path_with_underscores_and%20spaces",
	}
	for _, kw := range valid {
		if err := validateKeyword(kw); err != nil {
			t.Errorf("validateKeyword(%q): unexpected error: %v", kw, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// validateKeyword — null byte and control character rejection
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateKeyword_RejectsNullByte verifies that a keyword containing a
// null byte is rejected with a clear error.
func TestValidateKeyword_RejectsNullByte(t *testing.T) {
	kw := "foo\x00bar"
	err := validateKeyword(kw)
	if err == nil {
		t.Fatal("validateKeyword: expected error for null byte in keyword")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("validateKeyword: expected 'null byte' in error, got: %v", err)
	}
}

// TestValidateKeyword_RejectsDisallowedControlCharacters verifies that
// keywords containing disallowed control characters (0x01–0x08, 0x0B–0x0C,
// 0x0E–0x1F) are rejected.
func TestValidateKeyword_RejectsDisallowedControlCharacters(t *testing.T) {
	disallowed := []byte{0x01, 0x02, 0x03, 0x07, 0x08, 0x0B, 0x0C, 0x0E, 0x1F}
	for _, c := range disallowed {
		kw := "keyword" + string([]byte{c}) + "value"
		err := validateKeyword(kw)
		if err == nil {
			t.Errorf("validateKeyword: expected error for control char 0x%02x in keyword", c)
		}
	}
}

// TestValidateKeyword_AllowsWhitespaceControlCharacters verifies that common
// whitespace control characters (tab 0x09, LF 0x0A, CR 0x0D) are permitted,
// as they may appear in keywords loaded from filter files.
func TestValidateKeyword_AllowsWhitespaceControlCharacters(t *testing.T) {
	// Note: in practice these characters are stripped by TrimSpace/Split
	// before reaching validateKeyword, but the function must not reject them.
	allowed := []string{
		"a\tb",  // horizontal tab
		"a\nb",  // line feed
		"a\r\nb", // CR+LF
	}
	for _, kw := range allowed {
		if err := validateKeyword(kw); err != nil {
			t.Errorf("validateKeyword(%q): unexpected error for allowed whitespace: %v", kw, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// validateKeyword — length limits
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateKeyword_RejectsTooLong verifies that a keyword exceeding
// maxFilterKeywordLen bytes is rejected with a "too long" error.
func TestValidateKeyword_RejectsTooLong(t *testing.T) {
	longKW := strings.Repeat("a", maxFilterKeywordLen+1)
	err := validateKeyword(longKW)
	if err == nil {
		t.Fatal("validateKeyword: expected error for keyword exceeding maxFilterKeywordLen")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("validateKeyword: expected 'too long' in error, got: %v", err)
	}
}

// TestValidateKeyword_AcceptsExactMaxLength verifies that a keyword of exactly
// maxFilterKeywordLen bytes is accepted (inclusive upper bound).
func TestValidateKeyword_AcceptsExactMaxLength(t *testing.T) {
	exactKW := strings.Repeat("a", maxFilterKeywordLen)
	if err := validateKeyword(exactKW); err != nil {
		t.Errorf("validateKeyword: unexpected error at exact maxFilterKeywordLen: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// validateKeyword — SQL wildcard density limits
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateKeyword_AcceptsExactWildcardLimit verifies that a keyword
// containing exactly maxSQLWildcardCount wildcard characters is accepted
// (inclusive upper bound).
func TestValidateKeyword_AcceptsExactWildcardLimit(t *testing.T) {
	// maxSQLWildcardCount percent signs
	kw := strings.Repeat("%", maxSQLWildcardCount)
	if err := validateKeyword(kw); err != nil {
		t.Errorf("validateKeyword(%q): unexpected error at exact wildcard limit: %v", kw, err)
	}

	// maxSQLWildcardCount underscores
	kw = strings.Repeat("_", maxSQLWildcardCount)
	if err := validateKeyword(kw); err != nil {
		t.Errorf("validateKeyword(%q): unexpected error at exact wildcard limit: %v", kw, err)
	}
}

// TestValidateKeyword_RejectsExcessivePercentWildcards verifies that a keyword
// containing more than maxSQLWildcardCount percent signs (%) is rejected with
// a clear "wildcard" error message.
func TestValidateKeyword_RejectsExcessivePercentWildcards(t *testing.T) {
	kw := strings.Repeat("%", maxSQLWildcardCount+1)
	err := validateKeyword(kw)
	if err == nil {
		t.Fatalf("validateKeyword: expected error for %d %% characters (max %d)",
			maxSQLWildcardCount+1, maxSQLWildcardCount)
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("validateKeyword: expected 'wildcard' in error, got: %v", err)
	}
}

// TestValidateKeyword_RejectsExcessiveUnderscoreWildcards verifies that a
// keyword containing more than maxSQLWildcardCount underscore (_) characters
// is rejected.
func TestValidateKeyword_RejectsExcessiveUnderscoreWildcards(t *testing.T) {
	kw := strings.Repeat("_", maxSQLWildcardCount+1)
	err := validateKeyword(kw)
	if err == nil {
		t.Fatalf("validateKeyword: expected error for %d _ characters (max %d)",
			maxSQLWildcardCount+1, maxSQLWildcardCount)
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("validateKeyword: expected 'wildcard' in error, got: %v", err)
	}
}

// TestValidateKeyword_RejectsMixedExcessiveWildcards verifies that a keyword
// whose combined % and _ count exceeds maxSQLWildcardCount is rejected, even
// when neither character individually exceeds the limit.
func TestValidateKeyword_RejectsMixedExcessiveWildcards(t *testing.T) {
	// 11 % + 11 _ = 22, which exceeds maxSQLWildcardCount (20).
	kw := strings.Repeat("%", 11) + strings.Repeat("_", 11)
	err := validateKeyword(kw)
	if err == nil {
		t.Fatalf("validateKeyword: expected error for mixed wildcards totalling %d (max %d)",
			22, maxSQLWildcardCount)
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("validateKeyword: expected 'wildcard' in error, got: %v", err)
	}
}

// TestValidateKeyword_WildcardBombingKeyword verifies that a keyword consisting
// entirely of % characters far exceeding the limit is rejected. This simulates
// a "wildcard bombing" attack where the intent is to make a LIKE expression
// match every row in the database.
func TestValidateKeyword_WildcardBombingKeyword(t *testing.T) {
	cases := []string{
		strings.Repeat("%", 100),
		strings.Repeat("%", 500),
		strings.Repeat("_", 100),
		"%" + strings.Repeat("_", 50) + "%",
	}
	for _, kw := range cases {
		err := validateKeyword(kw)
		if err == nil {
			t.Errorf("validateKeyword(%q...): expected error for wildcard bombing keyword",
				kw[:min(len(kw), 20)])
		}
	}
}

// TestValidateKeyword_SQLPayloadsWithFewWildcards verifies that SQL injection
// payload strings that contain at most maxSQLWildcardCount wildcard characters
// are accepted by validateKeyword. These payloads contain no null bytes or
// control characters; they are treated as plain literal strings by the
// in-memory filter and must not be wrongly rejected at the keyword level.
func TestValidateKeyword_SQLPayloadsWithFewWildcards(t *testing.T) {
	// These are the payloads from sql_injection_test.go; none have excessive
	// wildcard characters.
	payloads := []string{
		"'; drop table urls; --",
		"' or '1'='1",
		"' or 1=1 --",
		"' union select id,url,title,visit_count,last_visit_time from urls --",
		"robert'); drop table students;--",
		"/**/union/**/select/**/1,2,3--",
		"'; pragma journal_mode=delete; --",
	}
	for _, p := range payloads {
		if err := validateKeyword(p); err != nil {
			t.Errorf("validateKeyword(%q): unexpected rejection of SQL payload: %v", p, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// validateFilterKeywords — propagates validateKeyword errors
// ─────────────────────────────────────────────────────────────────────────────

// TestValidateFilterKeywords_RejectsKeywordWithExcessiveWildcards verifies
// that validateFilterKeywords propagates validateKeyword errors when any
// keyword in the list exceeds the wildcard limit.
func TestValidateFilterKeywords_RejectsKeywordWithExcessiveWildcards(t *testing.T) {
	keywords := []string{
		"google.com",
		"github.com",
		strings.Repeat("%", maxSQLWildcardCount+5), // this one is over the limit
		"youtube.com",
	}
	err := validateFilterKeywords(keywords)
	if err == nil {
		t.Fatal("validateFilterKeywords: expected error when one keyword exceeds wildcard limit")
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("validateFilterKeywords: expected 'wildcard' in error, got: %v", err)
	}
}

// TestValidateFilterKeywords_RejectsKeywordWithNullByte verifies that
// validateFilterKeywords rejects a list containing a keyword with a null byte.
func TestValidateFilterKeywords_RejectsKeywordWithNullByte(t *testing.T) {
	keywords := []string{"google.com", "foo\x00bar", "youtube.com"}
	err := validateFilterKeywords(keywords)
	if err == nil {
		t.Fatal("validateFilterKeywords: expected error for null byte in keyword")
	}
}

// TestValidateFilterKeywords_RejectsKeywordTooLong verifies that
// validateFilterKeywords rejects a list containing a keyword that exceeds
// maxFilterKeywordLen bytes.
func TestValidateFilterKeywords_RejectsKeywordTooLong(t *testing.T) {
	keywords := []string{"google.com", strings.Repeat("a", maxFilterKeywordLen+1)}
	err := validateFilterKeywords(keywords)
	if err == nil {
		t.Fatal("validateFilterKeywords: expected error for keyword exceeding length limit")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("validateFilterKeywords: expected 'too long' in error, got: %v", err)
	}
}

// TestValidateFilterKeywords_AllValidKeywordsPass verifies that a list of
// well-formed keywords passes without error.
func TestValidateFilterKeywords_AllValidKeywordsPass(t *testing.T) {
	keywords := []string{
		"google.com",
		"github.com",
		"youtube.com",
		"search?q=golang",
		"my_profile",
		"file%20name",
	}
	if err := validateFilterKeywords(keywords); err != nil {
		t.Fatalf("validateFilterKeywords: unexpected error for valid keywords: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// loadFilters — end-to-end wildcard validation through all code paths
// ─────────────────────────────────────────────────────────────────────────────

// TestLoadFilters_RejectsSingleKeywordWithExcessiveWildcards verifies that the
// single-keyword code path in loadFilters rejects a keyword with too many
// wildcard characters. This path previously only checked the byte length, so
// it required explicit extension to call validateKeyword.
func TestLoadFilters_RejectsSingleKeywordWithExcessiveWildcards(t *testing.T) {
	// A single keyword (no commas, not a file path) with too many %s.
	kw := strings.Repeat("%", maxSQLWildcardCount+1)
	_, err := loadFilters(kw)
	if err == nil {
		t.Fatalf("loadFilters: expected error for single keyword with %d wildcard chars (max %d)",
			maxSQLWildcardCount+1, maxSQLWildcardCount)
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("loadFilters: expected 'wildcard' in error, got: %v", err)
	}
}

// TestLoadFilters_RejectsCommaSeparatedWithWildcardBombing verifies that the
// comma-separated inline path in loadFilters rejects a list where one keyword
// contains too many wildcard characters.
func TestLoadFilters_RejectsCommaSeparatedWithWildcardBombing(t *testing.T) {
	value := "google.com," + strings.Repeat("%", maxSQLWildcardCount+5) + ",youtube.com"
	_, err := loadFilters(value)
	if err == nil {
		t.Fatalf("loadFilters: expected error for comma-separated value with wildcard bombing")
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("loadFilters: expected 'wildcard' in error, got: %v", err)
	}
}

// TestLoadFilters_RejectsFileContainingWildcardBombingKeyword verifies that
// the file code path in loadFilters rejects a filter file that contains a
// keyword with an excessive number of SQL wildcard characters.
func TestLoadFilters_RejectsFileContainingWildcardBombingKeyword(t *testing.T) {
	dir := t.TempDir()
	filterFile := filepath.Join(dir, "wildcards.txt")

	content := "google.com\n" + strings.Repeat("%", maxSQLWildcardCount+10) + "\nyoutube.com\n"
	if err := os.WriteFile(filterFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write filter file: %v", err)
	}

	_, err := loadFilters(filterFile)
	if err == nil {
		t.Fatal("loadFilters: expected error for filter file containing wildcard bombing keyword")
	}
	if !strings.Contains(err.Error(), "wildcard") {
		t.Errorf("loadFilters: expected 'wildcard' in error, got: %v", err)
	}
}

// TestLoadFilters_AcceptsSingleKeywordWithinWildcardLimit verifies that a
// single keyword with exactly maxSQLWildcardCount wildcards passes loadFilters.
func TestLoadFilters_AcceptsSingleKeywordWithinWildcardLimit(t *testing.T) {
	kw := strings.Repeat("%", maxSQLWildcardCount)
	filters, err := loadFilters(kw)
	if err != nil {
		t.Fatalf("loadFilters: unexpected error for keyword with exactly %d wildcards: %v",
			maxSQLWildcardCount, err)
	}
	if len(filters) != 1 {
		t.Fatalf("loadFilters: expected 1 filter, got %d", len(filters))
	}
}

// TestLoadFilters_AcceptsCommaSeparatedWithReasonableWildcards verifies that
// comma-separated keywords each having a small number of wildcards are accepted.
func TestLoadFilters_AcceptsCommaSeparatedWithReasonableWildcards(t *testing.T) {
	value := "file%20name,search%20query,my_profile,path_with_underscores"
	filters, err := loadFilters(value)
	if err != nil {
		t.Fatalf("loadFilters: unexpected error for comma-separated value with few wildcards: %v", err)
	}
	if len(filters) != 4 {
		t.Fatalf("loadFilters: expected 4 filters, got %d: %v", len(filters), filters)
	}
}

// TestLoadFilters_AcceptsFileWithReasonableWildcards verifies that a filter
// file whose keywords each have a small number of wildcard characters is
// accepted and loaded correctly.
func TestLoadFilters_AcceptsFileWithReasonableWildcards(t *testing.T) {
	dir := t.TempDir()
	filterFile := filepath.Join(dir, "filters.txt")

	content := "google.com\nfile%20name\nmy_profile\nsearch?q=test\n"
	if err := os.WriteFile(filterFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write filter file: %v", err)
	}

	filters, err := loadFilters(filterFile)
	if err != nil {
		t.Fatalf("loadFilters: unexpected error for filter file with reasonable wildcards: %v", err)
	}
	if len(filters) != 4 {
		t.Fatalf("loadFilters: expected 4 filters, got %d", len(filters))
	}
}

// TestLoadFilters_WildcardCountIsPerKeywordNotPerValue verifies that the
// wildcard limit is applied per keyword (after splitting), not per the entire
// comma-separated value. A value where each individual keyword is within the
// limit must be accepted even if the total wildcard count across all keywords
// would exceed the limit.
func TestLoadFilters_WildcardCountIsPerKeywordNotPerValue(t *testing.T) {
	// 5 keywords, each with 10 wildcards = 50 total, but only 10 per keyword.
	// Each keyword is within the maxSQLWildcardCount (20) limit.
	parts := make([]string, 5)
	for i := range parts {
		parts[i] = strings.Repeat("%", 10)
	}
	value := strings.Join(parts, ",")
	filters, err := loadFilters(value)
	if err != nil {
		t.Fatalf("loadFilters: unexpected error — wildcard limit must apply per keyword, not per value: %v", err)
	}
	if len(filters) != 5 {
		t.Fatalf("loadFilters: expected 5 filters, got %d", len(filters))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// loadFilters — filter file size limit
// ─────────────────────────────────────────────────────────────────────────────

// TestLoadFilters_RejectsOversizedFilterFile verifies that loadFilters rejects
// a filter file whose size exceeds maxFilterFileBytes. This prevents memory
// exhaustion when an abnormally large file is supplied as a --match or
// --protect value.
func TestLoadFilters_RejectsOversizedFilterFile(t *testing.T) {
	dir := t.TempDir()
	filterFile := filepath.Join(dir, "huge.txt")

	// Write a file that is one byte over the limit. We repeat a short keyword
	// followed by a newline so the content looks like a valid filter file;
	// the file is simply too large.
	chunk := []byte("google.com\n")
	needed := maxFilterFileBytes + 1
	var content []byte
	for len(content) < needed {
		content = append(content, chunk...)
	}
	content = content[:needed] // trim to exactly one byte over

	if err := os.WriteFile(filterFile, content, 0600); err != nil {
		t.Fatalf("failed to write oversized filter file: %v", err)
	}

	_, err := loadFilters(filterFile)
	if err == nil {
		t.Fatalf("loadFilters: expected error for filter file exceeding maxFilterFileBytes (%d bytes), got nil",
			maxFilterFileBytes)
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("loadFilters: expected 'too large' in error message, got: %v", err)
	}
}

// TestLoadFilters_AcceptsFilterFileAtSizeLimit verifies that a filter file
// whose size is exactly at maxFilterFileBytes is not rejected by the size
// check (the limit is exclusive: files > maxFilterFileBytes are rejected).
func TestLoadFilters_AcceptsFilterFileAtSizeLimit(t *testing.T) {
	dir := t.TempDir()
	filterFile := filepath.Join(dir, "exactly_limit.txt")

	// A file whose size equals the limit must not be rejected by the size
	// check. We write a single keyword padded to exactly maxFilterFileBytes
	// using a long line. The content is not necessarily valid (it may exceed
	// per-keyword length), but the size check must pass.
	content := make([]byte, maxFilterFileBytes)
	copy(content, []byte("google.com"))
	// Fill remaining bytes with 'a' to make a single long line.
	for i := len("google.com"); i < maxFilterFileBytes; i++ {
		content[i] = 'a'
	}

	if err := os.WriteFile(filterFile, content, 0600); err != nil {
		t.Fatalf("failed to write file at size limit: %v", err)
	}

	// The file size is exactly maxFilterFileBytes, so the size check must
	// not reject it. The keyword validation may return an error (keyword too
	// long), but the error must NOT be a "too large" file-size error.
	_, err := loadFilters(filterFile)
	if err != nil && strings.Contains(err.Error(), "too large") {
		t.Errorf("loadFilters: file at exact size limit must not be rejected by size check, got: %v", err)
	}
}

// TestMaxFilterFileBytes_IsPositive verifies that maxFilterFileBytes is a
// positive constant so the size check is actually enforced.
func TestMaxFilterFileBytes_IsPositive(t *testing.T) {
	if maxFilterFileBytes <= 0 {
		t.Fatalf("maxFilterFileBytes must be positive, got %d", maxFilterFileBytes)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// maxSQLWildcardCount constant sanity checks
// ─────────────────────────────────────────────────────────────────────────────

// TestMaxSQLWildcardCount_IsPositive verifies that maxSQLWildcardCount is a
// positive integer, ensuring the limit is actually enforced (a value of 0
// would reject all keywords containing any wildcard character).
func TestMaxSQLWildcardCount_IsPositive(t *testing.T) {
	if maxSQLWildcardCount <= 0 {
		t.Fatalf("maxSQLWildcardCount must be positive, got %d", maxSQLWildcardCount)
	}
}

// TestMaxSQLWildcardCount_AllowsTypicalURLPercents verifies that a URL with
// several percent-encoded characters (common in browser history) passes the
// wildcard limit. This guards against the limit being set too low and
// accidentally rejecting legitimate URL fragments.
func TestMaxSQLWildcardCount_AllowsTypicalURLPercents(t *testing.T) {
	// A URL fragment with 10 percent-encoded characters — a realistic maximum.
	kw := "search%20results%20for%20golang%20programming%20on%20google%2Fexample%20page%20one"
	if err := validateKeyword(kw); err != nil {
		t.Errorf("validateKeyword: typical URL fragment rejected unexpectedly: %v (keyword: %s)", err, kw)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper: min for older Go versions without built-in min
// ─────────────────────────────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
