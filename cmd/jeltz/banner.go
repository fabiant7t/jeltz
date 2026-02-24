package main

import (
	"fmt"
	"os"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
)

// useColor reports whether ANSI color codes should be emitted.
// Color is disabled when NO_COLOR is set or stderr is not a terminal.
func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// printBanner writes a human-readable startup summary to stderr.
func printBanner(listen, configFile, dataDir, caCertPath string, rules int, logLevel string, insecureUpstream, dumpTraffic bool) {
	color := useColor()

	bold := func(s string) string {
		if color {
			return ansiBold + s + ansiReset
		}
		return s
	}
	dim := func(s string) string {
		if color {
			return ansiDim + s + ansiReset
		}
		return s
	}
	cyan := func(s string) string {
		if color {
			return ansiCyan + s + ansiReset
		}
		return s
	}
	yellow := func(s string) string {
		if color {
			return ansiYellow + s + ansiReset
		}
		return s
	}

	label := func(s string) string {
		return dim(fmt.Sprintf("  %-10s", s))
	}

	w := os.Stderr

	// Header: name + listen address.
	fmt.Fprintf(w, "\n  %s  %s  %s\n",
		bold(cyan("jeltz")),
		dim("·"),
		bold(listen),
	)

	// Paths cluster.
	fmt.Fprintln(w)
	if configFile != "" {
		fmt.Fprintf(w, "%s%s\n", label("config"), configFile)
	}
	fmt.Fprintf(w, "%s%s\n", label("data"), dataDir)
	fmt.Fprintf(w, "%s%s\n", label("ca cert"), caCertPath)

	// Options cluster.
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s%d\n", label("rules"), rules)
	fmt.Fprintf(w, "%s%s\n", label("log"), logLevel)

	upstream := "verified"
	if insecureUpstream {
		upstream = yellow("⚠  insecure")
	}
	fmt.Fprintf(w, "%s%s\n", label("upstream"), upstream)

	dump := "off"
	if dumpTraffic {
		dump = "on"
	}
	fmt.Fprintf(w, "%s%s\n", label("dump"), dump)

	fmt.Fprintln(w)
}
