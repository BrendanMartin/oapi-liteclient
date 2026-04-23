package generator

import (
	"bytes"
	"fmt"
	"text/template"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

// PythonOptions configures the Python code generator.
type PythonOptions struct {
	Style string // "pydantic" (default) or "dataclass"
	Auth  string // "none", "custom", "bearer-token", "gcp-id-token", "api-key"
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
	"pyDefaultVal":        pyDefaultVal,
	"hasBody":             func(t *ir.Type) bool { return t != nil },
	"isArrayBody":         func(t *ir.Type) bool { return t != nil && t.Kind == ir.TypeArray },
	"hasResponse":         func(t *ir.Type) bool { return t != nil },
	"hasParams":           func(ep ir.Endpoint) bool { return len(ep.Params) > 0 || ep.RequestBody != nil },
	"docstring":           docstring,
	"sortedFields":        sortedFields,
	"pathParams":          pathParams,
	"queryParams":         queryParams,
	"requiredQueryParams": requiredQueryParams,
	"optionalQueryParams": optionalQueryParams,
	"fmtPath":             fmtPath,
}

var pydanticTmpl = template.Must(template.New("pydantic").Funcs(funcMap).Parse(pydanticTemplate))
var dataclassTmpl = template.Must(template.New("dataclass").Funcs(funcMap).Parse(dataclassTemplate))

// GeneratePython generates a Python client from the IR spec.
func GeneratePython(spec *ir.Spec, opts PythonOptions) (string, error) {
	tmpl := pydanticTmpl
	if opts.Style == "dataclass" {
		tmpl = dataclassTmpl
	}
	authMode := opts.Auth
	if authMode == "" {
		authMode = "none"
	}
	data := pythonData{Spec: spec, AuthMode: authMode}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
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

// docstring returns a Python docstring from summary/description, or empty string.
func docstring(ep ir.Endpoint) string {
	text := ep.Summary
	if text == "" {
		text = ep.Description
	}
	return text
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

const pydanticTemplate = `"""Auto-generated API client for {{.Title}}."""
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
        base_url: str,
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
            json=[item.model_dump(exclude_none=True) for item in req],
{{- else}}
            json=req.model_dump(exclude_none=True),
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
{{end}}`

const dataclassTemplate = `"""Auto-generated API client for {{.Title}}."""
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
        base_url: str,
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
