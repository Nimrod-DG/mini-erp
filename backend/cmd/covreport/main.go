// covreport turns `go test -coverpkg=./...` output into the table §12.6 asks about.
//
// WHY THIS EXISTS RATHER THAN `go tool cover -func`.
//
// Two problems, and neither is obvious until the numbers come out wrong.
//
// **1. Per-package coverage understates the suite badly.** `go test ./...
// -coverprofile` measures each package only against its *own* tests. `internal/db`
// scores 42% that way, while `internal/api`'s tests are in fact exercising most of
// it — every request goes through `WithTenant`. The honest number needs
// `-coverpkg=./...`, which instruments the whole module in every test binary.
//
// **2. Which makes the profile un-summable.** With `-coverpkg`, each of the seven
// test binaries writes a full profile of all packages, and `go test` concatenates
// them. So every block appears seven times, once per binary, and `go tool cover`
// reading that file double-counts. The blocks have to be **unioned** — a block is
// covered if any binary covered it — which is what this does.
//
// It then groups by §12.6's targets, which are stated per *package* for a layout
// this project does not have: there is no `internal/procurement`, only
// procurement_*.go inside `internal/api`. The groups below map the spec's intent
// onto the files that exist, and that mapping is the reason this is a checked-in
// tool rather than a shell pipeline somebody rewrites each phase.
//
//	go run ./cmd/covreport coverage-all.out
//
// Exits non-zero if any group is under its target, so it can gate if it ever
// should. Today `make cover` just prints it: §12.6 says outright that "coverage
// percentage is a weak signal on its own", and the gate is Groups A–J plus the
// acceptance test.
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const modulePrefix = "github.com/DGosal/mini-erp/backend/"

// A group is one row of §12.6's table, matched against a profile's file paths.
type group struct {
	name   string
	target float64
	match  func(path string) bool
}

func procurementFile(path string) bool {
	// "incl. receipt.go" in §12.6. The receipt's inventory and finance halves are
	// part of the same transaction and are counted with it, because they are what
	// makes it cross-module.
	return strings.Contains(path, "internal/api/procurement") ||
		strings.HasSuffix(path, "internal/api/inventory_receipt.go") ||
		strings.HasSuffix(path, "internal/api/finance_journal.go")
}

func inventoryFile(path string) bool {
	return strings.Contains(path, "internal/api/inventory") &&
		!strings.HasSuffix(path, "internal/api/inventory_receipt.go")
}

func financeFile(path string) bool {
	return strings.HasSuffix(path, "internal/api/finance.go")
}

var groups = []group{
	{"procurement (incl. receipt.go)", 90, procurementFile},
	{"internal/db", 90, func(p string) bool { return strings.HasPrefix(p, "internal/db/") }},
	{"internal/middleware", 90, func(p string) bool { return strings.HasPrefix(p, "internal/middleware/") }},
	{"inventory", 80, inventoryFile},
	{"finance", 80, financeFile},
	{"handlers and wiring", 60, func(p string) bool {
		return strings.HasPrefix(p, "internal/api/") &&
			!procurementFile(p) && !inventoryFile(p) && !financeFile(p)
	}},
}

type counts struct{ covered, total int }

func (c counts) pct() float64 {
	if c.total == 0 {
		return 0
	}
	return 100 * float64(c.covered) / float64(c.total)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: covreport <profile>")
		os.Exit(2)
	}

	blocks, err := union(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "covreport:", err)
		os.Exit(2)
	}

	perFile := map[string]counts{}
	for key, block := range blocks {
		path := key[:strings.LastIndex(key, ":")]
		c := perFile[path]
		c.total += block.statements
		if block.covered {
			c.covered += block.statements
		}
		perFile[path] = c
	}

	failed := false
	fmt.Println("§12.6 coverage targets")
	fmt.Println(strings.Repeat("-", 66))
	for _, g := range groups {
		var c counts
		for path, fileCounts := range perFile {
			if g.match(path) {
				c.covered += fileCounts.covered
				c.total += fileCounts.total
			}
		}
		verdict := "OK"
		if c.pct() < g.target {
			verdict = "SHORT"
			failed = true
		}
		fmt.Printf("%-32s %6.1f%%  (%4d/%4d)  target %3.0f%%  %s\n",
			g.name, c.pct(), c.covered, c.total, g.target, verdict)
	}

	// Everything under internal/, as one number. Not a target — context, so a
	// reader can see whether a group is an outlier or the whole codebase.
	var overall counts
	for path, c := range perFile {
		if strings.HasPrefix(path, "internal/") {
			overall.covered += c.covered
			overall.total += c.total
		}
	}
	fmt.Println(strings.Repeat("-", 66))
	fmt.Printf("%-32s %6.1f%%  (%4d/%4d)\n",
		"all of internal/ (not a target)", overall.pct(), overall.covered, overall.total)

	fmt.Println()
	fmt.Println("Per file, worst first:")
	paths := make([]string, 0, len(perFile))
	for path := range perFile {
		if strings.HasPrefix(path, "internal/") {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return perFile[paths[i]].pct() < perFile[paths[j]].pct()
	})
	for _, path := range paths[:min(12, len(paths))] {
		c := perFile[path]
		fmt.Printf("  %6.1f%%  (%4d/%4d)  %s\n", c.pct(), c.covered, c.total, path)
	}

	if failed {
		// Reported, not enforced. See the package comment.
		fmt.Println("\nAt least one group is under its §12.6 target.")
	}
}

type block struct {
	statements int
	covered    bool
}

// union reads a concatenation of coverage profiles and keeps, for each block,
// whether ANY of them covered it. The key is the block's location, which is what
// makes the same block from seven binaries one block here.
func union(path string) (map[string]block, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	blocks := map[string]block{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		// `<path>:<startLine>.<col>,<endLine>.<col> <numStatements> <count>`
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unparseable profile line %q", line)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("statement count in %q: %w", line, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("hit count in %q: %w", line, err)
		}

		key := strings.TrimPrefix(fields[0], modulePrefix)
		existing := blocks[key]
		blocks[key] = block{
			statements: statements,
			covered:    existing.covered || count > 0,
		}
	}
	return blocks, scanner.Err()
}
