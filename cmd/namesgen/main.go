// Command namesgen regenerates the artifacts derived from names/names.yaml.
//
// It is deliberately thin: read a path, call internal/names, write two files.
// Everything worth testing — decoding, every refusal, both renderings — is in
// the package, where a test can reach it without a filesystem.
//
//	go generate ./internal/id
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pleware/initagent/internal/names"
)

func main() {
	source := flag.String("source", "names/names.yaml", "path to the naming registry")
	goOut := flag.String("go", "internal/id/registry_gen.go", "path for the generated Go registry")
	jsonOut := flag.String("json", "names/names.json", "path for the generated JSON artifact")
	flag.Parse()

	if err := run(*source, *goOut, *jsonOut); err != nil {
		fmt.Fprintln(os.Stderr, "namesgen:", err)
		os.Exit(1)
	}
}

func run(source, goOut, jsonOut string) error {
	registry, err := names.Load(source)
	if err != nil {
		return err
	}
	goSrc, err := names.EmitGo(registry)
	if err != nil {
		return err
	}
	jsonSrc, err := names.EmitJSON(registry)
	if err != nil {
		return err
	}
	if err := os.WriteFile(goOut, goSrc, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", goOut, err)
	}
	if err := os.WriteFile(jsonOut, jsonSrc, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", jsonOut, err)
	}
	return nil
}
