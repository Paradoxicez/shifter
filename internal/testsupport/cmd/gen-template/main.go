// Command gen-template emits the canonical Phase 3 bulk-import XLSX template
// to the path provided as the first CLI argument. The template is identical
// to what `/admin/imports` will serve to operators (Wave 1 implementation
// regenerates this via internal/import/template.go).
//
// Usage:
//
//	go run ./internal/testsupport/cmd/gen-template web/tests/fixtures/import-template.xlsx
//
// This binary is test-only — not built into the production Shifter binary.
package main

import (
	"fmt"
	"os"

	"github.com/shifter-io/shifter/internal/testsupport"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen-template OUTPUT_PATH")
		os.Exit(2)
	}
	dst := os.Args[1]

	data, err := testsupport.ImportTemplateBytes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build template: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", dst, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(data), dst)
}
