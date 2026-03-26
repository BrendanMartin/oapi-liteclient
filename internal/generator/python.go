package generator

import (
	"bytes"
	"fmt"
	"text/template"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

var pythonTmpl = template.Must(template.New("python").Funcs(template.FuncMap{
	"pyType":       pyType,
	"pyTypeDeref":  func(t *ir.Type) string { if t == nil { return "str" }; return pyType(*t) },
	"pyName":       pyName,
	"pyMethodName": pyMethodName,
	"pyDefault":    pyDefault,
	"hasBody":      func(t *ir.Type) bool { return t != nil },
	"hasResponse":  func(t *ir.Type) bool { return t != nil },
	"pathParams":         pathParams,
	"queryParams":        queryParams,
	"requiredQueryParams": requiredQueryParams,
	"optionalQueryParams": optionalQueryParams,
	"fmtPath":      fmtPath,
}).Parse(pythonTemplate))

// GeneratePython generates a Python client from the IR spec.
func GeneratePython(spec *ir.Spec) (string, error) {
	var buf bytes.Buffer
	if err := pythonTmpl.Execute(&buf, spec); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

func pyType(t ir.Type) string {
	switch t.Kind {
	case ir.TypePrimitive:
		switch t.Prim {
		case ir.PrimString:
			return "str"
		case ir.PrimInt:
			return "int"
		case ir.PrimFloat:
			return "float"
		case ir.PrimBool:
			return "bool"
		}
	case ir.TypeArray:
		if t.Elem != nil {
			return "list[" + pyType(*t.Elem) + "]"
		}
		return "list"
	case ir.TypeRef:
		return t.Ref
	}
	return "str"
}

var pythonReserved = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true,
	"assert": true, "async": true, "await": true, "break": true, "class": true,
	"continue": true, "def": true, "del": true, "elif": true, "else": true,
	"except": true, "finally": true, "for": true, "from": true, "global": true,
	"if": true, "import": true, "in": true, "is": true, "lambda": true,
	"nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
	"type": true,
}

// pyName converts a field name to snake_case, appending _ for reserved words.
func pyName(name string) string {
	var result []rune
	for i, r := range name {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(name[i-1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					result = append(result, '_')
				}
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}
	s := string(result)
	if pythonReserved[s] {
		s += "_"
	}
	return s
}

// pyMethodName converts an operationId to a Python method name.
func pyMethodName(opID string) string {
	if opID == "" {
		return "unknown"
	}
	return pyName(opID)
}

func pyDefault(t ir.Type) string {
	switch t.Kind {
	case ir.TypePrimitive:
		switch t.Prim {
		case ir.PrimString:
			return `""`
		case ir.PrimInt:
			return "0"
		case ir.PrimFloat:
			return "0.0"
		case ir.PrimBool:
			return "False"
		}
	case ir.TypeArray:
		return "field(default_factory=list)"
	case ir.TypeRef:
		return "None"
	}
	return "None"
}

func pathParams(params []ir.Param) []ir.Param {
	var out []ir.Param
	for _, p := range params {
		if p.In == "path" {
			out = append(out, p)
		}
	}
	return out
}

func queryParams(params []ir.Param) []ir.Param {
	var out []ir.Param
	for _, p := range params {
		if p.In == "query" {
			out = append(out, p)
		}
	}
	return out
}

func requiredQueryParams(params []ir.Param) []ir.Param {
	var out []ir.Param
	for _, p := range params {
		if p.In == "query" && p.Required {
			out = append(out, p)
		}
	}
	return out
}

func optionalQueryParams(params []ir.Param) []ir.Param {
	var out []ir.Param
	for _, p := range params {
		if p.In == "query" && !p.Required {
			out = append(out, p)
		}
	}
	return out
}

// fmtPath converts /pets/{petId} to /pets/{pet_id} for Python f-strings.
func fmtPath(path string) string {
	var result []byte
	inBrace := false
	braceContent := []byte{}
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			inBrace = true
			braceContent = braceContent[:0]
			result = append(result, '{')
		} else if path[i] == '}' {
			inBrace = false
			result = append(result, []byte(pyName(string(braceContent)))...)
			result = append(result, '}')
		} else if inBrace {
			braceContent = append(braceContent, path[i])
		} else {
			result = append(result, path[i])
		}
	}
	return string(result)
}

const pythonTemplate = `"""Auto-generated API client for {{.Title}}."""
from __future__ import annotations

import httpx
from dataclasses import dataclass, field
from typing import Optional


class APIError(Exception):
    """Raised on non-2xx responses."""

    def __init__(self, status_code: int, body: str, method: str, path: str):
        self.status_code = status_code
        self.body = body
        self.method = method
        self.path = path
        super().__init__(f"{method} {path} returned {status_code}: {body}")

{{range .Models}}
@dataclass
class {{.Name}}:
{{- range .Fields}}
{{- if .Required}}
    {{pyName .Name}}: {{pyType .Type}}
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = None
{{- end}}
{{- end}}
{{if not .Fields}}    pass{{end}}
{{end}}

class Client:
    """API client for {{.Title}}."""

    def __init__(
        self,
        base_url: str{{if .Auth}},{{end}}
{{- if .Auth}}
{{- if eq .Auth.Type 0}}
        api_key: str = "",
{{- else}}
        bearer_token: str = "",
{{- end}}
{{- end}}
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
        headers = {}
{{- if .Auth}}
{{- if eq .Auth.Type 0}}
{{- if eq .Auth.In "header"}}
        if api_key:
            headers["{{.Auth.Name}}"] = api_key
{{- end}}
{{- else}}
        if bearer_token:
            headers["Authorization"] = f"Bearer {bearer_token}"
{{- end}}
{{- end}}
        self._client = httpx.Client(
            base_url=self.base_url,
            headers=headers,
            timeout=timeout,
        )

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _request(self, method: str, path: str, **kwargs) -> httpx.Response:
        resp = self._client.request(method, path, **kwargs)
        if resp.status_code >= 400:
            raise APIError(resp.status_code, resp.text, method, path)
        return resp
{{range .Endpoints}}
    def {{pyMethodName .OperationID}}(
        self,
{{- range pathParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- if hasBody .RequestBody}}
        body: {{pyType .RequestBody}},
{{- end}}
{{- range requiredQueryParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- range optionalQueryParams .Params}}
        {{pyName .Name}}: Optional[{{pyType .Type}}] = None,
{{- end}}
    ){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- if queryParams .Params}}
        params = {}
{{- range requiredQueryParams .Params}}
        params["{{.Name}}"] = {{pyName .Name}}
{{- end}}
{{- range optionalQueryParams .Params}}
        if {{pyName .Name}} is not None:
            params["{{.Name}}"] = {{pyName .Name}}
{{- end}}
{{- end}}
        resp = self._request(
            "{{.Method}}",
            f"{{fmtPath .Path}}",
{{- if hasBody .RequestBody}}
            json=body.__dict__ if hasattr(body, "__dict__") else body,
{{- end}}
{{- if queryParams .Params}}
            params=params,
{{- end}}
        )
{{- if hasResponse .ResponseType}}
{{- if eq .ResponseType.Kind 2}}
        return {{pyType .ResponseType}}(**resp.json())
{{- else if eq .ResponseType.Kind 1}}
{{- if .ResponseType.Elem}}{{- if eq .ResponseType.Elem.Kind 2}}
        return [{{pyTypeDeref .ResponseType.Elem}}(**item) for item in resp.json()]
{{- else}}
        return resp.json()
{{- end}}{{- else}}
        return resp.json()
{{- end}}
{{- else}}
        return resp.json()
{{- end}}
{{- else}}
        return None
{{- end}}
{{end}}`
