# Browser History Manager

A CLI + GUI tool to preview, export, and delete browsing history from Chrome-family History databases.

> This project is not affiliated with Google.

## Verification and Browser Targets

- Windows is the primary verified environment for this release.
- Current named browser targets in code: Brave, Chrome, Edge, Chromium on Linux, and Opera on Windows.
- Other Chrome-family browsers may work when a compatible History database is provided directly via `--db`, but they are not runtime-verified.

## Installation

Download the binary for your OS from the Releases page and run it directly.

| OS | File |
|----|------|
| Windows | `browser-history-manager-windows-amd64.exe` |
| macOS (Apple Silicon) | `browser-history-manager-darwin-arm64` |
| macOS (Intel) | `browser-history-manager-darwin-amd64` |
| Linux (x64) | `browser-history-manager-linux-amd64` |
| Linux (ARM) | `browser-history-manager-linux-arm64` |

macOS and Linux CLI binaries are included as convenience builds, but were not runtime-verified in this release.

For Windows, the Releases page also includes `browser-history-manager-gui-windows-amd64.exe`.
The Windows GUI build embeds the WebView2 bootstrapper. If WebView2 is missing on first launch, Windows may prompt to install it.

## Before You Start

- **Close the browser before running any command.** All commands copy the database before reading it. On Windows, an open browser commonly keeps the History file locked, so even `preview` and `export` may fail until the browser is closed. Closing the browser first is the safest option on every platform.
- A backup is automatically created before any destructive operation (`delete`).

## CLI Usage

### Detect Browsers

```bash
browser-history-manager browsers
```

### Preview

```bash
# Inline keywords (comma-separated)
browser-history-manager preview --include "google.com,youtube.com"

# From file
browser-history-manager preview --include include.txt --exclude exclude.txt

# All entries
browser-history-manager preview --include "*"

# Limit output to first 10 entries
browser-history-manager preview --include "*" --limit 10
browser-history-manager preview --include "*" -n 10

# Restrict by last-visit date (inclusive)
browser-history-manager preview --include "*" --since 2026-01-01 --until 2026-01-31

# Target a specific browser
browser-history-manager preview --include "google.com" --browser edge

# Target a non-default Chrome profile
browser-history-manager preview --include "*" --profile "Profile 1"
```

### Export to CSV

```bash
# Filtered export
browser-history-manager export --include "google.com" --out result.csv

# Export all
browser-history-manager export --include "*" --out all.csv

# Export entries visited since a date
browser-history-manager export --include "*" --since 2026-01-01 --out recent.csv

# Export all (--include defaults to "*", --out defaults to history_export.csv)
browser-history-manager export
```

### Delete

```bash
# Delete google.com entries, but exclude those containing "scholar"
browser-history-manager delete --include "google.com" --exclude "scholar"

# Delete all, except excluded keywords
browser-history-manager delete --include "*" --exclude exclude.txt

# Delete only entries visited during a date range
browser-history-manager delete --include "*" --since 2026-01-01 --until 2026-01-31

# Skip confirmation (for scripting)
browser-history-manager delete --include "ads.com" --yes
```

## Filters

### Include Filter (`--include`)

Items whose URL or title contains any of the keywords are targeted.

- File: one keyword per line
- Inline: comma-separated
- `*`: match all

### Exclude Filter (`--exclude`)

Items matching the exclude keywords are excluded, even if they match the include filter. Exclude always takes precedence.

Legacy aliases: `--match` (= `--include`), `--protect` (= `--exclude`)

### Examples

| include | exclude | result |
|---------|---------|--------|
| `google.com` | - | matches items containing google.com |
| `google.com` | `scholar` | matches google.com, excludes if contains scholar |
| `*` | `scholar` | matches all, excludes if contains scholar |

## GUI

The GUI is split into peer `History | Backups` tabs that share the same active browser/database selection.

- **History tab**: Search, include/exclude keyword filters, selection toolbar, pagination, and history table
- **Backups tab**: Search backup file names, then browse a backup table with `File Name`, `Created`, `Size`, and `Items` columns for the selected `History` database folder
- **Shared utilities**: Help in the tab header, plus Refresh and Open DB folder in the active browser block
- Include/Exclude keywords are applied automatically when enabled on the History tab
- Clicking a filter chip toggles it on or off; the close icon removes it
- The Backups tab reuses the same page navigation pattern as History: page input, previous/next controls, and rows-per-page selector
- `Restore` is enabled only when exactly one backup is selected
- `Delete` is enabled when one or more backups are selected and removes matching `-wal` / `-shm` sidecars with each backup
- The Help dialog shows the current GUI version
- Date-range filtering is currently available in the CLI.

## CLI Options

| Option | Short | Description |
|--------|-------|-------------|
| `--include` | | Include filter (file path or comma-separated keywords, `*` = all) |
| `--exclude` | | Exclude filter (hide entries matching these keywords) |
| `--browser` | | Named browser target (brave, chrome, chromium, edge, opera) |
| `--profile` | | Browser profile directory name (default: `Default`, e.g. `"Profile 1"`) |
| `--db` | | History DB path (direct) |
| `--limit` | `-n` | Maximum entries to display in preview (1-10000, default: 50) |
| `--out` | | CSV output filename (default: `history_export.csv`); must be within the current working directory |
| `--since` | | Lower date bound for last visit (inclusive, `YYYY-MM-DD`) |
| `--until` | | Upper date bound for last visit (inclusive, `YYYY-MM-DD`) |
| `--yes` | `-y` | Skip confirmation prompts |
| `--version` | `-v` | Show version |
| `--help` | `-h` | Show usage information |

## Build from Source

Requires **Go 1.26+**. No C compiler (CGO) is needed because the SQLite driver is pure Go.

```bash
# Install dependencies
go mod download

# Build CLI
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=1.0.0" -o browser-history-manager .

# Build GUI (requires Wails)
cd cmd/gui && wails build
```

### Cross-compile for all platforms

The included `build.sh` script cross-compiles CLI binaries for Windows, macOS (Intel + Apple Silicon), and Linux (x64 + ARM). The GUI must be built separately with `wails build` on each target platform.

```bash
./build.sh 1.0.0
# Outputs to dist/
```

Cross-compiled CLI binaries for macOS and Linux are included for convenience, but were not runtime-verified in this release.

## Running Tests

```bash
go test ./...
```

All automated tests are self-contained and create temporary SQLite databases in the system temp directory; no real browser installation is required.

These tests validate core logic and regression behavior, but they do not substitute for runtime verification on every browser and operating system combination.

On Windows, the Go race detector needs CGO plus a C compiler. After installing a GCC toolchain, you can use the included helper:

```powershell
.\test-race.ps1
```

The script enables `CGO_ENABLED=1`, locates `gcc.exe` (including common WinGet WinLibs installs), and runs `go test -race ./...`. You can also pass specific package/test arguments, for example:

```powershell
.\test-race.ps1 ./cmd/gui -run SearchHistoryAll
```

## Project Structure

| Directory | Responsibility |
|-----------|----------------|
| `main.go`, `commands.go` | CLI entry point, flag parsing, subcommand handlers |
| `internal/history/` | Core library: DB access, filtering, export, delete queue, validation |
| `cmd/gui/` | Wails GUI application |
| `cmd/gui/frontend/` | React frontend (Vite + Manrope/JetBrains Mono/Material Symbols) |

## License

MIT License - see [LICENSE](LICENSE) for details.
