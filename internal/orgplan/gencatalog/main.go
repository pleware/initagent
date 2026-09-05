// Command gencatalog writes TypeScript for the hosted plan catalogue.
//
//	go run ./internal/orgplan/gencatalog
//	go run ./internal/orgplan/gencatalog -check
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pleware/initagent/internal/orgplan"
)

func main() {
	check := flag.Bool("check", false, "exit 1 unless generated files match the catalogue")
	flag.Parse()
	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gencatalog: %v\n", err)
		os.Exit(1)
	}
	want := []byte(orgplan.TypeScript())
	paths := []string{
		filepath.Join(root, "site", "src", "lib", "org-plans.gen.ts"),
		filepath.Join(root, "ui", "src", "lib", "org-plans.gen.ts"),
	}
	if *check {
		var stale int
		for _, path := range paths {
			got, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "gencatalog: %v\n", err)
				stale++
				continue
			}
			if !bytes.Equal(got, want) {
				fmt.Fprintf(os.Stderr, "gencatalog: stale %s (run go run ./internal/orgplan/gencatalog)\n", path)
				stale++
			}
		}
		if stale > 0 {
			os.Exit(1)
		}
		return
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "gencatalog: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gencatalog: %v\n", err)
			os.Exit(1)
		}
	}
}

func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("no caller")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
