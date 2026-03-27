package main

import (
	"fmt"
	"os"
	"time"

	"chrome-history-manager/internal/history"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cmd := os.Args[1]

	if cmd == "--version" || cmd == "-v" {
		fmt.Println(version)
		return
	}

	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		printUsage()
		return
	}

	if !validCommands[cmd] {
		exitErr("unknown command: %s\nRun 'browser-history-manager --help' for usage.", cmd)
	}

	flags, boolFlags, err := parseFlags(os.Args[2:])
	if err != nil {
		exitErr("%v", err)
	}

	if err := validateFlagValues(flags); err != nil {
		exitErr("%v", err)
	}

	var dbPath string

	if cmd != "browsers" {
		dbPath, err = history.ResolveDBPath(flags["browser"], flags["db"], flags["profile"])
		if err != nil {
			exitErr("%v", err)
		}
	}

	// Parse --limit, using default when absent. Validation already ran above.
	limit, _ := validateLimit(flags["limit"])

	// Parse --since and --until date range flags. Validation already ran above.
	var sinceTime, untilTime time.Time
	sinceTime, _ = history.ValidateDateFlag(flags["since"])
	untilTime, _ = history.ValidateDateFlag(flags["until"])

	switch cmd {
	case "preview":
		cmdPreview(dbPath, flags["include"], flags["exclude"], limit, sinceTime, untilTime)
	case "delete":
		cmdDelete(dbPath, flags["include"], flags["exclude"], boolFlags["yes"], sinceTime, untilTime)
	case "export":
		cmdExport(dbPath, flags["include"], flags["exclude"], flags["out"], sinceTime, untilTime)
	case "browsers":
		cmdBrowsers()
	}
}
