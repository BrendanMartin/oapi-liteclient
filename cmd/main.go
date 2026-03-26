package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brendanmartin/oapi-liteclient/internal/generator"
	"github.com/brendanmartin/oapi-liteclient/internal/parser"
)

func main() {
	spec := flag.String("spec", "", "Path or URL to OpenAPI spec (YAML or JSON)")
	lang := flag.String("lang", "python", "Target language (python)")
	out := flag.String("out", "./client", "Output directory")
	flag.Parse()

	if *spec == "" {
		fmt.Fprintln(os.Stderr, "error: --spec is required")
		flag.Usage()
		os.Exit(1)
	}

	irSpec, err := parser.Parse(*spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var output string
	var filename string

	switch *lang {
	case "python":
		output, err = generator.GeneratePython(irSpec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		filename = "client.py"
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported language %q (supported: python)\n", *lang)
		os.Exit(1)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(*out, filename)
	if err := os.WriteFile(outPath, []byte(output), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s\n", outPath)
}
