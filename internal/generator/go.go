package generator

import (
	"strings"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

// GoOptions configures the Go code generator.
type GoOptions struct {
	Auth    string // "none", "custom", "bearer-token", "api-key", "gcp-id-token"
	Package string // Go package name for generated code
}

// goName converts camelCase or snake_case to PascalCase.
func goName(name string) string {
	var result []rune
	upper := true
	for i, r := range name {
		if r == '_' {
			upper = true
			continue
		}
		if upper {
			result = append(result, unicode.ToUpper(r))
			upper = false
		} else if unicode.IsUpper(r) && i > 0 {
			result = append(result, r)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

var goReserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// goFieldName converts a name to PascalCase, appending _ for Go reserved words.
func goFieldName(name string) string {
	n := goName(name)
	if goReserved[strings.ToLower(n)] {
		return n + "_"
	}
	return n
}

func goType(t ir.Type) string {
	switch t.Kind {
	case ir.TypePrimitive:
		switch t.Prim {
		case ir.PrimString:
			return "string"
		case ir.PrimInt:
			return "int"
		case ir.PrimFloat:
			return "float64"
		case ir.PrimBool:
			return "bool"
		}
	case ir.TypeArray:
		if t.Elem != nil {
			return "[]" + goType(*t.Elem)
		}
		return "[]interface{}"
	case ir.TypeRef:
		return t.Ref
	}
	return "string"
}

// goFmtPath converts /pets/{petId} to ("/pets/%v", ["petID"]) for fmt.Sprintf.
func goFmtPath(path string) (string, []string) {
	var fmtStr []byte
	var args []string
	inBrace := false
	var braceContent []byte
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			inBrace = true
			braceContent = braceContent[:0]
			fmtStr = append(fmtStr, '%', 'v')
		} else if path[i] == '}' {
			inBrace = false
			args = append(args, goParamName(string(braceContent)))
		} else if inBrace {
			braceContent = append(braceContent, path[i])
		} else {
			fmtStr = append(fmtStr, path[i])
		}
	}
	return string(fmtStr), args
}

// goParamName converts a path param name to a Go parameter name (unexported camelCase).
func goParamName(name string) string {
	pascal := goName(name)
	if len(pascal) == 0 {
		return pascal
	}
	runes := []rune(pascal)
	if len(runes) == 1 {
		return strings.ToLower(string(runes))
	}
	allUpper := true
	for _, r := range runes {
		if !unicode.IsUpper(r) {
			allUpper = false
			break
		}
	}
	if allUpper {
		return strings.ToLower(string(runes))
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
