package generator

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

// resolveAuth returns the auth mode string. If explicit is set, it wins;
// otherwise the mode is inferred from the spec's security schemes.
func resolveAuth(explicit string, spec *ir.Spec) string {
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

// PythonOptions configures the Python code generator.
type PythonOptions struct {
	Style          string // "pydantic" (default) or "dataclass"
	Auth           string // "none", "custom", "bearer-token", "gcp-id-token", "api-key"
	Package        string // package name for pyproject.toml (defaults to output dir name)
	PackageVersion string // version for pyproject.toml (defaults to "0.1.0")
	Lenient        bool   // make all model fields Optional (tolerates null from inaccurate specs)
}

// pythonData is passed to the template.
type pythonData struct {
	*ir.Spec
	AuthMode string
}

var funcMap = template.FuncMap{
	"pyType": pyType,
	"pyTypeDeref": func(t *ir.Type) string {
		if t == nil {
			return "str"
		}
		return pyType(*t)
	},
	"pyName":       pyName,
	"pyMethodName": pyMethodName,
	"hasDefault":   func(f ir.Field) bool { return f.Default != nil },
	"needsAlias":   func(f ir.Field) bool { return pyName(f.Name) != f.Name },
	"modelHasAlias": func(m ir.Model) bool {
		for _, f := range m.Fields {
			if pyName(f.Name) != f.Name {
				return true
			}
		}
		return false
	},
	"pyDefaultVal":               pyDefaultVal,
	"hasBody":                    func(t *ir.Type) bool { return t != nil },
	"isArrayBody":                func(t *ir.Type) bool { return t != nil && t.Kind == ir.TypeArray },
	"hasResponse":                func(t *ir.Type) bool { return t != nil },
	"hasParams":                  func(ep ir.Endpoint) bool { return len(ep.Params) > 0 || ep.RequestBody != nil },
	"customCType":                customContentType,
	"isMultipart":                isMultipart,
	"pyMultipartMethod":          pyMultipartMethod,
	"pyMultipartSubclientMethod": pyMultipartSubclientMethod,
	"docstring":                  docstring,
	"sortedFields":               sortedFields,
	"pathParams":                 pathParams,
	"queryParams":                queryParams,
	"requiredQueryParams":        requiredQueryParams,
	"optionalQueryParams":        optionalQueryParams,
	"fmtPath":                    fmtPath,
}

var pydanticTmpl = template.Must(template.New("pydantic").Funcs(funcMap).Parse(pydanticTemplate))
var dataclassTmpl = template.Must(template.New("dataclass").Funcs(funcMap).Parse(dataclassTemplate))

// Split-mode templates (pydantic)
var pyBaseTmpl = template.Must(template.New("pyBase").Funcs(funcMap).Parse(pyBaseTemplate))
var pyModelsTmpl = template.Must(template.New("pyModels").Funcs(funcMap).Parse(pyModelsTemplate))
var pyTagTmpl = template.Must(template.New("pyTag").Funcs(funcMap).Parse(pyTagTemplate))
var pyClientTmpl = template.Must(template.New("pyClient").Funcs(funcMap).Parse(pyClientTemplate))
var pyInitTmpl = template.Must(template.New("pyInit").Funcs(funcMap).Parse(pyInitTemplate))

// Split-mode templates (dataclass)
var pyBaseDcTmpl = template.Must(template.New("pyBaseDc").Funcs(funcMap).Parse(pyBaseDcTemplate))
var pyModelsDcTmpl = template.Must(template.New("pyModelsDc").Funcs(funcMap).Parse(pyModelsDcTemplate))
var pyTagDcTmpl = template.Must(template.New("pyTagDc").Funcs(funcMap).Parse(pyTagDcTemplate))

// GeneratePython generates a Python client from the IR spec.
// Returns a map of filename → content. Single-file when no tags are present.
func GeneratePython(spec *ir.Spec, opts PythonOptions) (map[string]string, error) {
	if opts.Lenient {
		spec = makeLenient(spec)
	}
	authMode := resolveAuth(opts.Auth, spec)

	groups, hasTags := groupEndpointsByTag(spec.Endpoints)
	if !hasTags {
		tmpl := pydanticTmpl
		if opts.Style == "dataclass" {
			tmpl = dataclassTmpl
		}
		data := pythonData{Spec: spec, AuthMode: authMode}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("executing template: %w", err)
		}
		files := map[string]string{"client.py": buf.String()}
		if opts.Package != "" {
			files["pyproject.toml"] = pyProjectTOML(opts.Package, opts.PackageVersion, opts.Style, authMode)
		}
		return files, nil
	}

	groups = mergeTagsByPrefix(groups)

	if err := validateTagFilenames(groups); err != nil {
		return nil, err
	}

	isPydantic := opts.Style != "dataclass"
	files := make(map[string]string)

	// _base.py
	baseData := pythonData{Spec: spec, AuthMode: authMode}
	var buf bytes.Buffer
	baseTmpl := pyBaseTmpl
	if !isPydantic {
		baseTmpl = pyBaseDcTmpl
	}
	if err := baseTmpl.Execute(&buf, baseData); err != nil {
		return nil, fmt.Errorf("executing base template: %w", err)
	}
	files["_base.py"] = buf.String()

	// models.py
	buf.Reset()
	modelsTmpl := pyModelsTmpl
	if !isPydantic {
		modelsTmpl = pyModelsDcTmpl
	}
	if err := modelsTmpl.Execute(&buf, spec); err != nil {
		return nil, fmt.Errorf("executing models template: %w", err)
	}
	files["models.py"] = buf.String()

	// Per-tag files
	type tagFileData struct {
		ClassName string
		Endpoints []ir.Endpoint
	}
	tags := sortedTags(groups)
	var tagClassNames []struct{ Attr, ClassName, Module string }
	for _, tag := range tags {
		fn := tagToFilename(tag)
		cn := tagToClassName(tag) + "Client"
		tagClassNames = append(tagClassNames, struct{ Attr, ClassName, Module string }{
			Attr:      fn,
			ClassName: cn,
			Module:    fn,
		})
		buf.Reset()
		td := tagFileData{ClassName: cn, Endpoints: groups[tag]}
		tagTmpl := pyTagTmpl
		if !isPydantic {
			tagTmpl = pyTagDcTmpl
		}
		if err := tagTmpl.Execute(&buf, td); err != nil {
			return nil, fmt.Errorf("executing tag template for %q: %w", tag, err)
		}
		files[fn+".py"] = buf.String()
	}

	// client.py
	buf.Reset()
	clientData := struct {
		Title    string
		AuthMode string
		Tags     []struct{ Attr, ClassName, Module string }
	}{
		Title:    spec.Title,
		AuthMode: authMode,
		Tags:     tagClassNames,
	}
	if err := pyClientTmpl.Execute(&buf, clientData); err != nil {
		return nil, fmt.Errorf("executing client template: %w", err)
	}
	files["client.py"] = buf.String()

	// __init__.py
	buf.Reset()
	if err := pyInitTmpl.Execute(&buf, clientData); err != nil {
		return nil, fmt.Errorf("executing init template: %w", err)
	}
	files["__init__.py"] = buf.String()

	if opts.Package != "" {
		files["pyproject.toml"] = pyProjectTOML(opts.Package, opts.PackageVersion, opts.Style, authMode)
	}

	return files, nil
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
		case ir.PrimAny:
			return "Any"
		}
	case ir.TypeArray:
		if t.Elem != nil {
			return "list[" + pyType(*t.Elem) + "]"
		}
		return "list"
	case ir.TypeRef:
		return t.Ref
	case ir.TypeMap:
		if t.Elem != nil {
			return "dict[str, " + pyType(*t.Elem) + "]"
		}
		return "dict[str, Any]"
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

// pyName converts a field name to a valid Python snake_case identifier,
// appending _ for reserved words.
var pySpecialFieldNames = map[string]string{
	"+1": "plus_one",
	"-1": "minus_one",
}

func pyName(name string) string {
	if mapped, ok := pySpecialFieldNames[name]; ok {
		return mapped
	}
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
		} else if r == '.' || r == '-' || r == ' ' || r == '/' || r == '$' || r == '+' {
			result = append(result, '_')
		} else {
			result = append(result, r)
		}
	}
	s := strings.TrimLeft(string(result), "_")
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "n" + s
	}
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

// pyMultipartMethod renders a complete Python method for a multipart/form-data
// endpoint: file parts go through httpx files=, other fields through data= using
// their original (possibly dotted) wire keys. The body is identical for pydantic
// and dataclass styles except for the response-decode statement.
func pyMultipartMethod(ep ir.Endpoint, style string) string {
	return pyMultipartMethodWithRequest(ep, style, "self._request")
}

func pyMultipartSubclientMethod(ep ir.Endpoint, style string) string {
	return pyMultipartMethodWithRequest(ep, style, "self._client._request")
}

func pyMultipartMethodWithRequest(ep ir.Endpoint, style, requestExpr string) string {
	files := formFileFields(ep.FormFields)
	required := formRequiredFields(ep.FormFields)
	optional := formOptionalFields(ep.FormFields)

	var b strings.Builder
	b.WriteString("\n    def ")
	b.WriteString(pyMethodName(ep.OperationID))
	b.WriteString("(\n        self,\n")
	for _, f := range files {
		fmt.Fprintf(&b, "        %s,\n", pyName(f.Name))
	}
	for _, f := range required {
		fmt.Fprintf(&b, "        %s: %s,\n", pyName(f.Name), pyType(f.Type))
	}
	for _, f := range optional {
		fmt.Fprintf(&b, "        %s: Optional[%s] = None,\n", pyName(f.Name), pyType(f.Type))
	}
	b.WriteString("    )")
	if ep.ResponseType != nil {
		fmt.Fprintf(&b, " -> %s", pyType(*ep.ResponseType))
	}
	b.WriteString(":\n")
	if doc := docstring(ep); doc != "" {
		fmt.Fprintf(&b, "        \"\"\"%s\"\"\"\n", doc)
	}

	b.WriteString("        files = {")
	for i, f := range files {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q: %s", f.Key, pyName(f.Name))
	}
	b.WriteString("}\n        data = {}\n")
	for _, f := range required {
		fmt.Fprintf(&b, "        data[%q] = %s\n", f.Key, pyName(f.Name))
	}
	for _, f := range optional {
		fmt.Fprintf(&b, "        if %s is not None:\n            data[%q] = %s\n", pyName(f.Name), f.Key, pyName(f.Name))
	}

	fmt.Fprintf(&b, "        resp = %s(\n            %q,\n            f\"%s\",\n            files=files,\n            data=data,\n        )\n", requestExpr, ep.Method, fmtPath(ep.Path))
	fmt.Fprintf(&b, "        %s\n", pyReturnStmt(ep, style))
	return b.String()
}

// pyReturnStmt renders the response-decode return statement for an endpoint,
// matching the chosen model style (pydantic vs dataclass construction).
func pyReturnStmt(ep ir.Endpoint, style string) string {
	rt := ep.ResponseType
	if rt == nil {
		return "return None"
	}
	switch rt.Kind {
	case ir.TypeRef:
		if style == "dataclass" {
			return fmt.Sprintf("return %s(**resp.json())", pyType(*rt))
		}
		return fmt.Sprintf("return %s.model_validate(resp.json())", pyType(*rt))
	case ir.TypeArray:
		if rt.Elem != nil && rt.Elem.Kind == ir.TypeRef {
			if style == "dataclass" {
				return fmt.Sprintf("return [%s(**item) for item in resp.json()]", pyType(*rt.Elem))
			}
			return fmt.Sprintf("return [%s.model_validate(item) for item in resp.json()]", pyType(*rt.Elem))
		}
		return "return resp.json()"
	default:
		return "return resp.json()"
	}
}

// pyDefaultVal converts a raw default string to a Python literal based on the field type.
func pyDefaultVal(f ir.Field) string {
	if f.Default == nil {
		return "None"
	}
	val := *f.Default
	switch f.Type.Kind {
	case ir.TypePrimitive:
		switch f.Type.Prim {
		case ir.PrimString:
			return `"` + val + `"`
		case ir.PrimBool:
			if val == "true" {
				return "True"
			}
			return "False"
		case ir.PrimInt, ir.PrimFloat:
			return val
		}
	}
	return `"` + val + `"`
}

// docstring returns a single-line docstring with METHOD /path and summary/description.
func docstring(ep ir.Endpoint) string {
	prefix := ep.Method + " " + ep.Path
	text := ep.Summary
	if text == "" {
		text = ep.Description
	}
	if text != "" {
		if i := strings.IndexAny(text, "\r\n"); i >= 0 {
			text = text[:i]
		}
		return prefix + " — " + text
	}
	return prefix
}

func makeLenient(spec *ir.Spec) *ir.Spec {
	out := *spec
	out.Models = make([]ir.Model, len(spec.Models))
	for i, m := range spec.Models {
		model := m
		model.Fields = make([]ir.Field, len(m.Fields))
		for j, f := range m.Fields {
			f.Required = false
			model.Fields[j] = f
		}
		out.Models[i] = model
	}
	return &out
}

// sortedFields returns fields ordered for valid dataclass definitions:
// required without default, required with default, optional (all have defaults).
func sortedFields(fields []ir.Field) []ir.Field {
	var noDefault, hasDefault []ir.Field
	for _, f := range fields {
		if f.Required && f.Default == nil {
			noDefault = append(noDefault, f)
		} else {
			hasDefault = append(hasDefault, f)
		}
	}
	return append(noDefault, hasDefault...)
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

func pyProjectTOML(pkg, version, style, authMode string) string {
	safePkg := strings.ReplaceAll(pkg, "-", "_")
	if version == "" {
		version = "0.1.0"
	}
	var deps []string
	deps = append(deps, `    "httpx>=0.27"`)
	if style != "dataclass" {
		deps = append(deps, `    "pydantic>=2"`)
	}
	if authMode == "gcp-id-token" {
		deps = append(deps, `    "google-auth>=2"`)
	}
	return fmt.Sprintf(`[project]
name = %q
version = %q
requires-python = ">=3.10"
dependencies = [
%s,
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["."]

[tool.hatch.build.targets.wheel.sources]
"." = %q
`, safePkg, version, strings.Join(deps, ",\n"), safePkg)
}

const pydanticTemplate = `{{- $style := "pydantic" -}}
"""Auto-generated API client for {{.Title}}."""
from __future__ import annotations

import httpx
{{- if eq .AuthMode "custom"}}
from collections.abc import Callable
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
import time
import google.auth.transport.requests
import google.oauth2.id_token
{{- end}}
from pydantic import BaseModel, Field
from typing import Any, Optional


class APIError(Exception):
    """Raised on non-2xx responses."""

    def __init__(self, status_code: int, body: str, method: str, path: str):
        self.status_code = status_code
        self.body = body
        self.method = method
        self.path = path
        super().__init__(f"{method} {path} returned {status_code}: {body}")

{{range .Models}}
class {{.Name}}(BaseModel):
{{- range .Fields}}
{{- if needsAlias .}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = Field({{pyDefaultVal .}}, alias="{{.Name}}")
{{- else}}
    {{pyName .Name}}: {{pyType .Type}} = Field(alias="{{.Name}}")
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = Field({{pyDefaultVal .}}, alias="{{.Name}}")
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = Field(None, alias="{{.Name}}")
{{- end}}
{{- end}}
{{- else}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: {{pyType .Type}}
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = None
{{- end}}
{{- end}}
{{- end}}
{{- end}}
{{- if modelHasAlias .}}

    model_config = {"populate_by_name": True}
{{- end}}
{{if not .Fields}}    pass{{end}}
{{end}}

class Client:
    """API client for {{.Title}}."""

    def __init__(
        self,
        base_url: str = "{{.BaseURL}}",
{{- if eq .AuthMode "custom"}}
        auth: Callable[[], dict[str, str]] | None = None,
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        bearer_token: str = "",
{{- end}}
{{- if eq .AuthMode "api-key"}}
        api_key: str = "",
        api_key_header: str = "X-API-Key",
{{- end}}
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
{{- if eq .AuthMode "custom"}}
        self._auth = auth
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        self._bearer_token = bearer_token
{{- end}}
{{- if eq .AuthMode "api-key"}}
        self._api_key = api_key
        self._api_key_header = api_key_header
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        self._token: str | None = None
        self._token_expiry: float = 0
{{- end}}
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
        )
{{- if eq .AuthMode "gcp-id-token"}}

    def _get_auth_headers(self) -> dict[str, str]:
        """Get a cached ID token for Cloud Run IAM authentication."""
        now = time.time()
        if self._token is None or now >= self._token_expiry:
            auth_req = google.auth.transport.requests.Request()
            self._token = google.oauth2.id_token.fetch_id_token(auth_req, self.base_url)
            self._token_expiry = now + 3300  # Refresh 5 min before 1h expiry
        return {"Authorization": f"Bearer {self._token}"}
{{- end}}

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _request(self, method: str, path: str, **kwargs) -> httpx.Response:
{{- if eq .AuthMode "custom"}}
        if self._auth:
            headers = kwargs.pop("headers", {})
            headers.update(self._auth())
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        if self._bearer_token:
            headers = kwargs.pop("headers", {})
            headers["Authorization"] = f"Bearer {self._bearer_token}"
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "api-key"}}
        if self._api_key:
            headers = kwargs.pop("headers", {})
            headers[self._api_key_header] = self._api_key
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        headers = kwargs.pop("headers", {})
        headers.update(self._get_auth_headers())
        kwargs["headers"] = headers
{{- end}}
        resp = self._client.request(method, path, **kwargs)
        if resp.status_code >= 400:
            raise APIError(resp.status_code, resp.text, method, path)
        return resp
{{range .Endpoints}}
{{- if isMultipart .}}{{pyMultipartMethod . $style}}
{{- else}}
{{- if hasParams .}}
    def {{pyMethodName .OperationID}}(
        self,
{{- range pathParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- if hasBody .RequestBody}}
        req: {{pyType .RequestBody}},
{{- end}}
{{- range requiredQueryParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- range optionalQueryParams .Params}}
        {{pyName .Name}}: Optional[{{pyType .Type}}] = None,
{{- end}}
    ){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- else}}
    def {{pyMethodName .OperationID}}(self){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- end}}
{{- if docstring .}}
        """{{docstring .}}"""
{{- end}}
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
{{- if isArrayBody .RequestBody}}
            json=[item.model_dump(exclude_none=True, by_alias=True) for item in req],
{{- else}}
            json=req.model_dump(exclude_none=True, by_alias=True),
{{- end}}
{{- if customCType .}}
            headers={"Content-Type": "{{.RequestCType}}"},
{{- end}}
{{- end}}
{{- if queryParams .Params}}
            params=params,
{{- end}}
        )
{{- if hasResponse .ResponseType}}
{{- if eq .ResponseType.Kind 2}}
        return {{pyType .ResponseType}}.model_validate(resp.json())
{{- else if eq .ResponseType.Kind 1}}
{{- if .ResponseType.Elem}}{{- if eq .ResponseType.Elem.Kind 2}}
        return [{{pyTypeDeref .ResponseType.Elem}}.model_validate(item) for item in resp.json()]
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
{{- end}}
{{end}}`

const dataclassTemplate = `{{- $style := "dataclass" -}}
"""Auto-generated API client for {{.Title}}."""
from __future__ import annotations

import httpx
{{- if eq .AuthMode "custom"}}
from collections.abc import Callable
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
import time
import google.auth.transport.requests
import google.oauth2.id_token
{{- end}}
from dataclasses import dataclass, field
from typing import Any, Optional


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
{{- range sortedFields .Fields}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: {{pyType .Type}}
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = None
{{- end}}
{{- end}}
{{- end}}
{{if not .Fields}}    pass{{end}}
{{end}}

class Client:
    """API client for {{.Title}}."""

    def __init__(
        self,
        base_url: str = "{{.BaseURL}}",
{{- if eq .AuthMode "custom"}}
        auth: Callable[[], dict[str, str]] | None = None,
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        bearer_token: str = "",
{{- end}}
{{- if eq .AuthMode "api-key"}}
        api_key: str = "",
        api_key_header: str = "X-API-Key",
{{- end}}
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
{{- if eq .AuthMode "custom"}}
        self._auth = auth
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        self._bearer_token = bearer_token
{{- end}}
{{- if eq .AuthMode "api-key"}}
        self._api_key = api_key
        self._api_key_header = api_key_header
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        self._token: str | None = None
        self._token_expiry: float = 0
{{- end}}
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
        )
{{- if eq .AuthMode "gcp-id-token"}}

    def _get_auth_headers(self) -> dict[str, str]:
        """Get a cached ID token for Cloud Run IAM authentication."""
        now = time.time()
        if self._token is None or now >= self._token_expiry:
            auth_req = google.auth.transport.requests.Request()
            self._token = google.oauth2.id_token.fetch_id_token(auth_req, self.base_url)
            self._token_expiry = now + 3300  # Refresh 5 min before 1h expiry
        return {"Authorization": f"Bearer {self._token}"}
{{- end}}

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _request(self, method: str, path: str, **kwargs) -> httpx.Response:
{{- if eq .AuthMode "custom"}}
        if self._auth:
            headers = kwargs.pop("headers", {})
            headers.update(self._auth())
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        if self._bearer_token:
            headers = kwargs.pop("headers", {})
            headers["Authorization"] = f"Bearer {self._bearer_token}"
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "api-key"}}
        if self._api_key:
            headers = kwargs.pop("headers", {})
            headers[self._api_key_header] = self._api_key
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        headers = kwargs.pop("headers", {})
        headers.update(self._get_auth_headers())
        kwargs["headers"] = headers
{{- end}}
        resp = self._client.request(method, path, **kwargs)
        if resp.status_code >= 400:
            raise APIError(resp.status_code, resp.text, method, path)
        return resp
{{range .Endpoints}}
{{- if isMultipart .}}{{pyMultipartMethod . $style}}
{{- else}}
{{- if hasParams .}}
    def {{pyMethodName .OperationID}}(
        self,
{{- range pathParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- if hasBody .RequestBody}}
        req: {{pyType .RequestBody}},
{{- end}}
{{- range requiredQueryParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- range optionalQueryParams .Params}}
        {{pyName .Name}}: Optional[{{pyType .Type}}] = None,
{{- end}}
    ){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- else}}
    def {{pyMethodName .OperationID}}(self){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- end}}
{{- if docstring .}}
        """{{docstring .}}"""
{{- end}}
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
{{- if isArrayBody .RequestBody}}
            json=[item.__dict__ for item in req] if req and hasattr(req[0], "__dict__") else req,
{{- else}}
            json=req.__dict__ if hasattr(req, "__dict__") else req,
{{- end}}
{{- if customCType .}}
            headers={"Content-Type": "{{.RequestCType}}"},
{{- end}}
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
{{- end}}
{{end}}`

// --- Split-mode template strings (pydantic) ---

const pyBaseTemplate = `"""Auto-generated base client for {{.Title}}."""
from __future__ import annotations

import httpx
{{- if eq .AuthMode "custom"}}
from collections.abc import Callable
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
import time
import google.auth.transport.requests
import google.oauth2.id_token
{{- end}}
from typing import Any, Optional


class APIError(Exception):
    """Raised on non-2xx responses."""

    def __init__(self, status_code: int, body: str, method: str, path: str):
        self.status_code = status_code
        self.body = body
        self.method = method
        self.path = path
        super().__init__(f"{method} {path} returned {status_code}: {body}")


class BaseClient:
    """Base API client with auth and HTTP handling."""

    def __init__(
        self,
        base_url: str = "{{.BaseURL}}",
{{- if eq .AuthMode "custom"}}
        auth: Callable[[], dict[str, str]] | None = None,
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        bearer_token: str = "",
{{- end}}
{{- if eq .AuthMode "api-key"}}
        api_key: str = "",
        api_key_header: str = "X-API-Key",
{{- end}}
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
{{- if eq .AuthMode "custom"}}
        self._auth = auth
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        self._bearer_token = bearer_token
{{- end}}
{{- if eq .AuthMode "api-key"}}
        self._api_key = api_key
        self._api_key_header = api_key_header
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        self._token: str | None = None
        self._token_expiry: float = 0
{{- end}}
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
        )
{{- if eq .AuthMode "gcp-id-token"}}

    def _get_auth_headers(self) -> dict[str, str]:
        """Get a cached ID token for Cloud Run IAM authentication."""
        now = time.time()
        if self._token is None or now >= self._token_expiry:
            auth_req = google.auth.transport.requests.Request()
            self._token = google.oauth2.id_token.fetch_id_token(auth_req, self.base_url)
            self._token_expiry = now + 3300  # Refresh 5 min before 1h expiry
        return {"Authorization": f"Bearer {self._token}"}
{{- end}}

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _request(self, method: str, path: str, **kwargs) -> httpx.Response:
{{- if eq .AuthMode "custom"}}
        if self._auth:
            headers = kwargs.pop("headers", {})
            headers.update(self._auth())
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        if self._bearer_token:
            headers = kwargs.pop("headers", {})
            headers["Authorization"] = f"Bearer {self._bearer_token}"
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "api-key"}}
        if self._api_key:
            headers = kwargs.pop("headers", {})
            headers[self._api_key_header] = self._api_key
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        headers = kwargs.pop("headers", {})
        headers.update(self._get_auth_headers())
        kwargs["headers"] = headers
{{- end}}
        resp = self._client.request(method, path, **kwargs)
        if resp.status_code >= 400:
            raise APIError(resp.status_code, resp.text, method, path)
        return resp
`

const pyModelsTemplate = `"""Auto-generated models."""
from __future__ import annotations

from pydantic import BaseModel, Field
from typing import Any, Optional

{{range .Models}}
class {{.Name}}(BaseModel):
{{- range .Fields}}
{{- if needsAlias .}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = Field({{pyDefaultVal .}}, alias="{{.Name}}")
{{- else}}
    {{pyName .Name}}: {{pyType .Type}} = Field(alias="{{.Name}}")
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = Field({{pyDefaultVal .}}, alias="{{.Name}}")
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = Field(None, alias="{{.Name}}")
{{- end}}
{{- end}}
{{- else}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: {{pyType .Type}}
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = None
{{- end}}
{{- end}}
{{- end}}
{{- end}}
{{- if modelHasAlias .}}

    model_config = {"populate_by_name": True}
{{- end}}
{{if not .Fields}}    pass{{end}}
{{end}}`

const pyTagTemplate = `{{- $style := "pydantic" -}}
"""Auto-generated tag client."""
from __future__ import annotations

from typing import Any, Optional

from ._base import BaseClient
from .models import *


class {{.ClassName}}:
    """Sub-client for {{.ClassName}} endpoints."""

    def __init__(self, client: BaseClient):
        self._client = client
{{range .Endpoints}}
{{- if isMultipart .}}{{pyMultipartSubclientMethod . $style}}
{{- else}}
{{- if hasParams .}}
    def {{pyMethodName .OperationID}}(
        self,
{{- range pathParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- if hasBody .RequestBody}}
        req: {{pyType .RequestBody}},
{{- end}}
{{- range requiredQueryParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- range optionalQueryParams .Params}}
        {{pyName .Name}}: Optional[{{pyType .Type}}] = None,
{{- end}}
    ){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- else}}
    def {{pyMethodName .OperationID}}(self){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- end}}
{{- if docstring .}}
        """{{docstring .}}"""
{{- end}}
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
        resp = self._client._request(
            "{{.Method}}",
            f"{{fmtPath .Path}}",
{{- if hasBody .RequestBody}}
{{- if isArrayBody .RequestBody}}
            json=[item.model_dump(exclude_none=True, by_alias=True) for item in req],
{{- else}}
            json=req.model_dump(exclude_none=True, by_alias=True),
{{- end}}
{{- if customCType .}}
            headers={"Content-Type": "{{.RequestCType}}"},
{{- end}}
{{- end}}
{{- if queryParams .Params}}
            params=params,
{{- end}}
        )
{{- if hasResponse .ResponseType}}
{{- if eq .ResponseType.Kind 2}}
        return {{pyType .ResponseType}}.model_validate(resp.json())
{{- else if eq .ResponseType.Kind 1}}
{{- if .ResponseType.Elem}}{{- if eq .ResponseType.Elem.Kind 2}}
        return [{{pyTypeDeref .ResponseType.Elem}}.model_validate(item) for item in resp.json()]
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
{{- end}}
{{end}}`

const pyClientTemplate = `"""Auto-generated client for {{.Title}}."""
from __future__ import annotations

from ._base import BaseClient
{{- range .Tags}}
from .{{.Module}} import {{.ClassName}}
{{- end}}


class Client(BaseClient):
    """API client for {{.Title}}."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
{{- range .Tags}}
        self.{{.Attr}} = {{.ClassName}}(self)
{{- end}}
`

const pyInitTemplate = `"""Auto-generated API client for {{.Title}}."""
from .client import Client
from ._base import APIError

__all__ = ["Client", "APIError"]
`

// --- Split-mode template strings (dataclass) ---

const pyBaseDcTemplate = `"""Auto-generated base client for {{.Title}}."""
from __future__ import annotations

import httpx
{{- if eq .AuthMode "custom"}}
from collections.abc import Callable
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
import time
import google.auth.transport.requests
import google.oauth2.id_token
{{- end}}
from typing import Any, Optional


class APIError(Exception):
    """Raised on non-2xx responses."""

    def __init__(self, status_code: int, body: str, method: str, path: str):
        self.status_code = status_code
        self.body = body
        self.method = method
        self.path = path
        super().__init__(f"{method} {path} returned {status_code}: {body}")


class BaseClient:
    """Base API client with auth and HTTP handling."""

    def __init__(
        self,
        base_url: str = "{{.BaseURL}}",
{{- if eq .AuthMode "custom"}}
        auth: Callable[[], dict[str, str]] | None = None,
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        bearer_token: str = "",
{{- end}}
{{- if eq .AuthMode "api-key"}}
        api_key: str = "",
        api_key_header: str = "X-API-Key",
{{- end}}
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
{{- if eq .AuthMode "custom"}}
        self._auth = auth
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        self._bearer_token = bearer_token
{{- end}}
{{- if eq .AuthMode "api-key"}}
        self._api_key = api_key
        self._api_key_header = api_key_header
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        self._token: str | None = None
        self._token_expiry: float = 0
{{- end}}
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=timeout,
        )
{{- if eq .AuthMode "gcp-id-token"}}

    def _get_auth_headers(self) -> dict[str, str]:
        """Get a cached ID token for Cloud Run IAM authentication."""
        now = time.time()
        if self._token is None or now >= self._token_expiry:
            auth_req = google.auth.transport.requests.Request()
            self._token = google.oauth2.id_token.fetch_id_token(auth_req, self.base_url)
            self._token_expiry = now + 3300  # Refresh 5 min before 1h expiry
        return {"Authorization": f"Bearer {self._token}"}
{{- end}}

    def close(self):
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _request(self, method: str, path: str, **kwargs) -> httpx.Response:
{{- if eq .AuthMode "custom"}}
        if self._auth:
            headers = kwargs.pop("headers", {})
            headers.update(self._auth())
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "bearer-token"}}
        if self._bearer_token:
            headers = kwargs.pop("headers", {})
            headers["Authorization"] = f"Bearer {self._bearer_token}"
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "api-key"}}
        if self._api_key:
            headers = kwargs.pop("headers", {})
            headers[self._api_key_header] = self._api_key
            kwargs["headers"] = headers
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
        headers = kwargs.pop("headers", {})
        headers.update(self._get_auth_headers())
        kwargs["headers"] = headers
{{- end}}
        resp = self._client.request(method, path, **kwargs)
        if resp.status_code >= 400:
            raise APIError(resp.status_code, resp.text, method, path)
        return resp
`

const pyModelsDcTemplate = `"""Auto-generated models."""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Optional

{{range .Models}}
@dataclass
class {{.Name}}:
{{- range sortedFields .Fields}}
{{- if .Required}}
{{- if hasDefault .}}
    {{pyName .Name}}: {{pyType .Type}} = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: {{pyType .Type}}
{{- end}}
{{- else}}
{{- if hasDefault .}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = {{pyDefaultVal .}}
{{- else}}
    {{pyName .Name}}: Optional[{{pyType .Type}}] = None
{{- end}}
{{- end}}
{{- end}}
{{if not .Fields}}    pass{{end}}
{{end}}`

const pyTagDcTemplate = `{{- $style := "dataclass" -}}
"""Auto-generated tag client."""
from __future__ import annotations

from typing import Any, Optional

from ._base import BaseClient
from .models import *


class {{.ClassName}}:
    """Sub-client for {{.ClassName}} endpoints."""

    def __init__(self, client: BaseClient):
        self._client = client
{{range .Endpoints}}
{{- if isMultipart .}}{{pyMultipartSubclientMethod . $style}}
{{- else}}
{{- if hasParams .}}
    def {{pyMethodName .OperationID}}(
        self,
{{- range pathParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- if hasBody .RequestBody}}
        req: {{pyType .RequestBody}},
{{- end}}
{{- range requiredQueryParams .Params}}
        {{pyName .Name}}: {{pyType .Type}},
{{- end}}
{{- range optionalQueryParams .Params}}
        {{pyName .Name}}: Optional[{{pyType .Type}}] = None,
{{- end}}
    ){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- else}}
    def {{pyMethodName .OperationID}}(self){{- if hasResponse .ResponseType}} -> {{pyType .ResponseType}}{{end}}:
{{- end}}
{{- if docstring .}}
        """{{docstring .}}"""
{{- end}}
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
        resp = self._client._request(
            "{{.Method}}",
            f"{{fmtPath .Path}}",
{{- if hasBody .RequestBody}}
{{- if isArrayBody .RequestBody}}
            json=[item.__dict__ for item in req] if req and hasattr(req[0], "__dict__") else req,
{{- else}}
            json=req.__dict__ if hasattr(req, "__dict__") else req,
{{- end}}
{{- if customCType .}}
            headers={"Content-Type": "{{.RequestCType}}"},
{{- end}}
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
{{- end}}
{{end}}`
