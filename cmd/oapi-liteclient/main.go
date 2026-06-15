package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/brendanmartin/oapi-liteclient/internal/generator"
	"github.com/brendanmartin/oapi-liteclient/internal/ir"
	"github.com/brendanmartin/oapi-liteclient/internal/parser"
)

type mergeFlags []string

func (m *mergeFlags) String() string {
	return strings.Join(*m, ",")
}

func (m *mergeFlags) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func main() {
	spec := flag.String("spec", "", "Path or URL to OpenAPI spec (YAML or JSON)")
	lang := flag.String("lang", "python", "Target language (python, go)")
	style := flag.String("style", "pydantic", "Model style for Python: pydantic (default) or dataclass")
	auth := flag.String("auth", "", "Auth strategy: none, custom, bearer-token, gcp-id-token, api-key (auto-detected from spec if omitted)")
	out := flag.String("out", "./client", "Output directory")
	force := flag.Bool("force", false, "Overwrite output directory if it exists")
	lenient := flag.Bool("lenient", false, "Make all model fields optional (tolerates null values from inaccurate specs)")
	packageVersion := flag.String("package-version", "0.1.0", "Version for the generated Python pyproject.toml")
	version := flag.Bool("version", false, "Print version and exit")
	var merges mergeFlags
	flag.Var(&merges, "merge", "Supplemental OpenAPI fragment (YAML/JSON) to deep-merge into the base spec before parsing; repeatable. Plain recursive merge, not OpenAPI Overlay 1.0.")
	flag.Parse()

	if *version {
		printVersion()
		return
	}

	if *spec == "" {
		fmt.Fprintln(os.Stderr, "error: --spec is required")
		flag.Usage()
		os.Exit(1)
	}

	irSpec, err := parser.Parse(*spec, merges...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var files map[string]string

	switch *lang {
	case "python":
		opts := generator.PythonOptions{Style: *style, Auth: *auth, Package: filepath.Base(*out), PackageVersion: *packageVersion, Lenient: *lenient}
		files, err = generator.GeneratePython(irSpec, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if _, hasInit := files["__init__.py"]; !hasInit {
			files["__init__.py"] = ""
		}
	case "go":
		pkg := filepath.Base(*out)
		pkg = strings.ReplaceAll(pkg, "-", "_")
		if len(pkg) > 0 && pkg[0] >= '0' && pkg[0] <= '9' {
			pkg = "pkg" + pkg
		}
		opts := generator.GoOptions{Auth: *auth, Package: pkg, Lenient: *lenient}
		files, err = generator.GenerateGo(irSpec, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported language %q (supported: python, go)\n", *lang)
		os.Exit(1)
	}

	if info, err := os.Stat(*out); err == nil && info.IsDir() {
		if !*force {
			fmt.Fprintf(os.Stderr, "error: output directory %q already exists (use --force to overwrite)\n", *out)
			os.Exit(1)
		}
		if err := os.RemoveAll(*out); err != nil {
			fmt.Fprintf(os.Stderr, "error cleaning output dir: %v\n", err)
			os.Exit(1)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	for filename, content := range files {
		outPath := filepath.Join(*out, filename)
		if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("Generated %s\n", outPath)
	}

	fmt.Println()
	authMode := resolveAuthMode(*auth, irSpec)
	switch *lang {
	case "python":
		module := strings.ReplaceAll(filepath.Base(*out), "-", "_")
		fmt.Printf("Install:\n")
		fmt.Printf("  pip install %s\n\n", *out)
		fmt.Printf("Usage:\n")
		fmt.Printf("  from %s import Client\n", module)
		fmt.Printf("  client = Client(%s)\n", pyAuthArgsOnly(authMode))
	case "go":
		pkg := filepath.Base(*out)
		pkg = strings.ReplaceAll(pkg, "-", "_")
		fmt.Printf("Usage:\n")
		fmt.Printf("  client := %s.NewClient(%s)\n", pkg, goAuthArgsOnly(authMode))
	}
}

func resolveAuthMode(explicit string, spec *ir.Spec) string {
	if explicit != "" {
		return explicit
	}
	if spec.Auth != nil {
		switch spec.Auth.Type {
		case ir.AuthBearer:
			return "bearer-token"
		case ir.AuthAPIKey:
			return "api-key"
		}
	}
	return "none"
}

func pyAuthArgsOnly(authMode string) string {
	switch authMode {
	case "bearer-token":
		return `bearer_token="..."`
	case "api-key":
		return `api_key="..."`
	case "custom":
		return `auth=lambda: {"Authorization": "..."}`
	default:
		return ""
	}
}

func goAuthArgsOnly(authMode string) string {
	switch authMode {
	case "bearer-token":
		return `"your-token"`
	case "api-key":
		return `"your-api-key"`
	case "custom":
		return `func(r *http.Request) { r.Header.Set("Authorization", "...") }`
	case "gcp-id-token":
		return `"https://your-audience"`
	default:
		return ""
	}
}

func printVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("oapi-liteclient (unknown version)")
		return
	}
	version := info.Main.Version
	var revision, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = " (dirty)"
			}
		}
	}
	if revision != "" && len(revision) > 7 {
		revision = revision[:7]
	}
	if revision != "" {
		if version == "(devel)" || strings.Contains(version, "-0.") {
			version = revision
		} else {
			version = version + " " + revision
		}
	}
	fmt.Printf("oapi-liteclient %s%s\n", version, dirty)
}
