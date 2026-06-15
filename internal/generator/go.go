package generator

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
	"golang.org/x/tools/imports"
)

// GoOptions configures the Go code generator.
type GoOptions struct {
	Auth    string // "none", "custom", "bearer-token", "api-key", "gcp-id-token"
	Package string // Go package name for generated code
	Lenient bool   // make all model fields pointer types (tolerates null from inaccurate specs)
}

// goName converts camelCase or snake_case to PascalCase.
var goSpecialFieldNames = map[string]string{
	"+1": "PlusOne",
	"-1": "MinusOne",
}

func goName(name string) string {
	if mapped, ok := goSpecialFieldNames[name]; ok {
		return mapped
	}
	var result []rune
	upper := true
	for i, r := range name {
		if r == '_' || r == '.' || r == '-' || r == ' ' || r == '/' || r == '$' || r == '+' {
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
	s := string(result)
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "N" + s
	}
	return s
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
		case ir.PrimAny:
			return "interface{}"
		}
	case ir.TypeArray:
		if t.Elem != nil {
			return "[]" + goType(*t.Elem)
		}
		return "[]interface{}"
	case ir.TypeRef:
		return t.Ref
	case ir.TypeMap:
		if t.Elem != nil {
			return "map[string]" + goType(*t.Elem)
		}
		return "map[string]interface{}"
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

type goData struct {
	*ir.Spec
	AuthMode string
	Package  string
}

var goFuncMap = template.FuncMap{
	"goType":      goType,
	"goName":      goName,
	"goFieldName": goFieldName,
	"goParamName": goParamName,
	"goFmtPath": func(path string) string {
		f, _ := goFmtPath(path)
		return f
	},
	"goFmtPathArgs": func(path string) []string {
		_, args := goFmtPath(path)
		return args
	},
	"goTypeDeref": func(t *ir.Type) string {
		if t == nil {
			return "string"
		}
		return goType(*t)
	},
	"hasBody":             func(t *ir.Type) bool { return t != nil },
	"isMultipart":         isMultipart,
	"goMultipartMethod":   goMultipartMethod,
	"isArrayBody":         func(t *ir.Type) bool { return t != nil && t.Kind == ir.TypeArray },
	"hasResponse":         func(t *ir.Type) bool { return t != nil },
	"isArrayResponse":     func(t *ir.Type) bool { return t != nil && t.Kind == ir.TypeArray },
	"isRefResponse":       func(t *ir.Type) bool { return t != nil && t.Kind == ir.TypeRef },
	"pathParams":          pathParams,
	"queryParams":         queryParams,
	"requiredQueryParams": requiredQueryParams,
	"optionalQueryParams": optionalQueryParams,
	"hasOptionalQuery": func(params []ir.Param) bool {
		for _, p := range params {
			if p.In == "query" && !p.Required {
				return true
			}
		}
		return false
	},
	"docstring": docstring,
	"isStringType": func(t ir.Type) bool {
		return t.Kind == ir.TypePrimitive && t.Prim == ir.PrimString
	},
}

var goTmpl = template.Must(template.New("go").Funcs(goFuncMap).Parse(goTemplate))

// Split-mode templates
var goErrorsTmpl = template.Must(template.New("goErrors").Funcs(goFuncMap).Parse(goErrorsTemplate))
var goModelsTmpl = template.Must(template.New("goModels").Funcs(goFuncMap).Parse(goModelsTemplate))
var goClientSplitTmpl = template.Must(template.New("goClientSplit").Funcs(goFuncMap).Parse(goClientSplitTemplate))
var goTagTmpl = template.Must(template.New("goTag").Funcs(goFuncMap).Parse(goTagTemplate))

func goFormat(src []byte) (string, error) {
	formatted, err := imports.Process("", src, nil)
	if err != nil {
		return string(src), fmt.Errorf("formatting output: %w (raw output may have syntax errors)", err)
	}
	return string(formatted), nil
}

// GenerateGo generates a Go client from the IR spec.
// Returns a map of filename → content. Single-file when no tags are present.
func GenerateGo(spec *ir.Spec, opts GoOptions) (map[string]string, error) {
	if opts.Lenient {
		spec = makeLenient(spec)
	}
	authMode := resolveAuth(opts.Auth, spec)
	pkg := opts.Package
	if pkg == "" {
		pkg = "client"
	}
	data := goData{Spec: spec, AuthMode: authMode, Package: pkg}

	groups, hasTags := groupEndpointsByTag(spec.Endpoints)
	if !hasTags {
		var buf bytes.Buffer
		if err := goTmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("executing template: %w", err)
		}
		s, err := goFormat(buf.Bytes())
		if err != nil {
			return nil, err
		}
		return map[string]string{"client.go": s}, nil
	}

	groups = mergeTagsByPrefix(groups)

	if err := validateTagFilenames(groups); err != nil {
		return nil, err
	}

	files := make(map[string]string)
	var buf bytes.Buffer

	// errors.go
	if err := goErrorsTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing errors template: %w", err)
	}
	s, err := goFormat(buf.Bytes())
	if err != nil {
		return nil, err
	}
	files["errors.go"] = s

	// models.go
	buf.Reset()
	if err := goModelsTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing models template: %w", err)
	}
	s, err = goFormat(buf.Bytes())
	if err != nil {
		return nil, err
	}
	files["models.go"] = s

	// client.go
	buf.Reset()
	if err := goClientSplitTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing client template: %w", err)
	}
	s, err = goFormat(buf.Bytes())
	if err != nil {
		return nil, err
	}
	files["client.go"] = s

	// Per-tag files
	type tagData struct {
		Package   string
		AuthMode  string
		Auth      *ir.Auth
		Endpoints []ir.Endpoint
	}
	for _, tag := range sortedTags(groups) {
		buf.Reset()
		td := tagData{
			Package:   pkg,
			AuthMode:  authMode,
			Auth:      spec.Auth,
			Endpoints: groups[tag],
		}
		if err := goTagTmpl.Execute(&buf, td); err != nil {
			return nil, fmt.Errorf("executing tag template for %q: %w", tag, err)
		}
		s, err = goFormat(buf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("formatting %s.go (tag %q): %w", tagToFilename(tag), tag, err)
		}
		fn := tagToFilename(tag)
		files[fn+".go"] = s
	}

	return files, nil
}

const goTemplate = `// Code generated by oapi-liteclient. DO NOT EDIT.
package {{.Package}}

import (
{{- if .Endpoints}}
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
{{- if eq .AuthMode "gcp-id-token"}}
	"google.golang.org/api/idtoken"
{{- end}}
{{- else}}
	"fmt"
{{- end}}
)

// APIError is returned for non-2xx HTTP responses.
type APIError struct {
	StatusCode int
	Body       string
	Method     string
	Path       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}
{{range .Models}}
type {{.Name}} struct {
{{- range .Fields}}
{{- if .Required}}
	{{goFieldName .Name}} {{goType .Type}} ` + "`" + `json:"{{if .Alias}}{{.Alias}}{{else}}{{.Name}}{{end}}"` + "`" + `
{{- else}}
	{{goFieldName .Name}} *{{goType .Type}} ` + "`" + `json:"{{if .Alias}}{{.Alias}}{{else}}{{.Name}}{{end}},omitempty"` + "`" + `
{{- end}}
{{- end}}
}
{{end}}
{{- if .Endpoints}}

const DefaultBaseURL = "{{.BaseURL}}"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
{{- if eq .AuthMode "bearer-token"}}
	bearerToken string
{{- end}}
{{- if eq .AuthMode "api-key"}}
	apiKey       string
	apiKeyHeader string
{{- end}}
{{- if eq .AuthMode "custom"}}
	authFunc func(req *http.Request)
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
	tokenSource *idtoken.TokenSource
{{- end}}
}

{{- if eq .AuthMode "none"}}

func NewClient() *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient}
}
{{- end}}
{{- if eq .AuthMode "bearer-token"}}

func NewClient(bearerToken string) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, bearerToken: bearerToken}
}
{{- end}}
{{- if eq .AuthMode "api-key"}}

func NewClient(apiKey string) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, apiKey: apiKey, apiKeyHeader: "{{.Auth.Name}}"}
}
{{- end}}
{{- if eq .AuthMode "custom"}}

func NewClient(authFunc func(req *http.Request)) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, authFunc: authFunc}
}
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}

func NewClient(targetAudience string) (*Client, error) {
	ts, err := idtoken.NewTokenSource(context.Background(), targetAudience)
	if err != nil {
		return nil, fmt.Errorf("creating token source: %w", err)
	}
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, tokenSource: ts}, nil
}
{{- end}}

func (c *Client) do(ctx context.Context, method, path, contentType string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
		if contentType == "" {
			contentType = "application/json"
		}
	}
	return c.doRaw(ctx, method, path, contentType, reqBody)
}

func (c *Client) doRaw(ctx context.Context, method, path, contentType string, body io.Reader) (*http.Response, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
{{- if eq .AuthMode "bearer-token"}}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
{{- end}}
{{- if eq .AuthMode "api-key"}}
	req.Header.Set(c.apiKeyHeader, c.apiKey)
{{- end}}
{{- if eq .AuthMode "custom"}}
	if c.authFunc != nil {
		c.authFunc(req)
	}
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
	token, err := c.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("getting ID token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
{{- end}}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(b), Method: method, Path: path}
	}
	return resp, nil
}
{{range .Endpoints}}
{{- if .OperationID}}
{{- if isMultipart .}}{{goMultipartMethod .}}
{{- else}}
{{- $opName := goName .OperationID}}

type {{$opName}}Op struct {
	client *Client
	ctx    context.Context
{{- range pathParams .Params}}
	{{goParamName .Name}} {{goType .Type}}
{{- end}}
{{- if hasBody .RequestBody}}
	body {{goTypeDeref .RequestBody}}
{{- end}}
{{- range requiredQueryParams .Params}}
	{{goParamName .Name}} {{goType .Type}}
{{- end}}
{{- range optionalQueryParams .Params}}
	{{goParamName .Name}} *{{goType .Type}}
{{- end}}
}

// {{$opName}} {{docstring .}}
func (c *Client) {{$opName}}(ctx context.Context
{{- range pathParams .Params}}, {{goParamName .Name}} {{goType .Type}}{{end}}
{{- if hasBody .RequestBody}}, body {{goTypeDeref .RequestBody}}{{end}}
{{- range requiredQueryParams .Params}}, {{goParamName .Name}} {{goType .Type}}{{end}}) *{{$opName}}Op {
	return &{{$opName}}Op{
		client: c,
		ctx:    ctx,
{{- range pathParams .Params}}
		{{goParamName .Name}}: {{goParamName .Name}},
{{- end}}
{{- if hasBody .RequestBody}}
		body: body,
{{- end}}
{{- range requiredQueryParams .Params}}
		{{goParamName .Name}}: {{goParamName .Name}},
{{- end}}
	}
}
{{range optionalQueryParams .Params}}
func (r *{{$opName}}Op) {{goName .Name}}(v {{goType .Type}}) *{{$opName}}Op {
	r.{{goParamName .Name}} = &v
	return r
}
{{end}}
{{- if hasResponse .ResponseType}}
{{- if isArrayResponse .ResponseType}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- else if isRefResponse .ResponseType}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- else}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- end}}
	query := url.Values{}
{{- range requiredQueryParams .Params}}
{{- if isStringType .Type}}
	query.Set("{{.Name}}", r.{{goParamName .Name}})
{{- else}}
	query.Set("{{.Name}}", fmt.Sprint(r.{{goParamName .Name}}))
{{- end}}
{{- end}}
{{- range optionalQueryParams .Params}}
	if r.{{goParamName .Name}} != nil {
{{- if isStringType .Type}}
		query.Set("{{.Name}}", *r.{{goParamName .Name}})
{{- else}}
		query.Set("{{.Name}}", fmt.Sprint(*r.{{goParamName .Name}}))
{{- end}}
	}
{{- end}}
	path := fmt.Sprintf("{{goFmtPath .Path}}"{{range goFmtPathArgs .Path}}, r.{{.}}{{end}})
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
{{- if hasBody .RequestBody}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "{{.RequestCType}}", r.body)
{{- else}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "", nil)
{{- end}}
	if err != nil {
{{- if isArrayResponse .ResponseType}}
		return nil, err
{{- else if isRefResponse .ResponseType}}
		return {{goTypeDeref .ResponseType}}{}, err
{{- else}}
		var zero {{goTypeDeref .ResponseType}}
		return zero, err
{{- end}}
	}
	defer resp.Body.Close()
	var result {{goTypeDeref .ResponseType}}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
{{- if isArrayResponse .ResponseType}}
		return nil, fmt.Errorf("decoding response: %w", err)
{{- else if isRefResponse .ResponseType}}
		return {{goTypeDeref .ResponseType}}{}, fmt.Errorf("decoding response: %w", err)
{{- else}}
		var zero {{goTypeDeref .ResponseType}}
		return zero, fmt.Errorf("decoding response: %w", err)
{{- end}}
	}
	return result, nil
}
{{- else}}
func (r *{{$opName}}Op) Do() error {
	query := url.Values{}
{{- range requiredQueryParams .Params}}
{{- if isStringType .Type}}
	query.Set("{{.Name}}", r.{{goParamName .Name}})
{{- else}}
	query.Set("{{.Name}}", fmt.Sprint(r.{{goParamName .Name}}))
{{- end}}
{{- end}}
{{- range optionalQueryParams .Params}}
	if r.{{goParamName .Name}} != nil {
{{- if isStringType .Type}}
		query.Set("{{.Name}}", *r.{{goParamName .Name}})
{{- else}}
		query.Set("{{.Name}}", fmt.Sprint(*r.{{goParamName .Name}}))
{{- end}}
	}
{{- end}}
	path := fmt.Sprintf("{{goFmtPath .Path}}"{{range goFmtPathArgs .Path}}, r.{{.}}{{end}})
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
{{- if hasBody .RequestBody}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "{{.RequestCType}}", r.body)
{{- else}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "", nil)
{{- end}}
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
{{- end}}
{{- end}}
{{- end}}
{{end}}
{{- end}}
`

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
	s := string(runes)
	if goReserved[s] {
		return s + "_"
	}
	return s
}

// goMultipartMethod renders the full Op struct, constructor, optional-field
// setters, and Do() for a multipart/form-data endpoint. File parts are written
// with multipart.CreateFormFile; other fields use their original wire keys.
// Output is fed through gofmt by the caller, so spacing here need not be exact.
func goMultipartMethod(ep ir.Endpoint) string {
	opName := goName(ep.OperationID)
	pathPs := pathParams(ep.Params)
	files := formFileFields(ep.FormFields)
	required := formRequiredFields(ep.FormFields)
	optional := formOptionalFields(ep.FormFields)

	var b strings.Builder

	// Op struct.
	fmt.Fprintf(&b, "\ntype %sOp struct {\n", opName)
	b.WriteString("\tclient *Client\n\tctx    context.Context\n")
	for _, p := range pathPs {
		fmt.Fprintf(&b, "\t%s %s\n", goParamName(p.Name), goType(p.Type))
	}
	for _, f := range files {
		fmt.Fprintf(&b, "\t%s io.Reader\n", goParamName(f.Name))
	}
	for _, f := range required {
		fmt.Fprintf(&b, "\t%s %s\n", goParamName(f.Name), goType(f.Type))
	}
	for _, f := range optional {
		fmt.Fprintf(&b, "\t%s *%s\n", goParamName(f.Name), goType(f.Type))
	}
	b.WriteString("}\n")

	// Constructor.
	fmt.Fprintf(&b, "\nfunc (c *Client) %s(ctx context.Context", opName)
	for _, p := range pathPs {
		fmt.Fprintf(&b, ", %s %s", goParamName(p.Name), goType(p.Type))
	}
	for _, f := range files {
		fmt.Fprintf(&b, ", %s io.Reader", goParamName(f.Name))
	}
	for _, f := range required {
		fmt.Fprintf(&b, ", %s %s", goParamName(f.Name), goType(f.Type))
	}
	fmt.Fprintf(&b, ") *%sOp {\n\treturn &%sOp{client: c, ctx: ctx", opName, opName)
	for _, p := range pathPs {
		fmt.Fprintf(&b, ", %s: %s", goParamName(p.Name), goParamName(p.Name))
	}
	for _, f := range files {
		fmt.Fprintf(&b, ", %s: %s", goParamName(f.Name), goParamName(f.Name))
	}
	for _, f := range required {
		fmt.Fprintf(&b, ", %s: %s", goParamName(f.Name), goParamName(f.Name))
	}
	b.WriteString("}\n}\n")

	// Optional-field setters.
	for _, f := range optional {
		fmt.Fprintf(&b, "\nfunc (r *%sOp) %s(v %s) *%sOp {\n\tr.%s = &v\n\treturn r\n}\n",
			opName, goName(f.Name), goType(f.Type), opName, goParamName(f.Name))
	}

	// Do().
	hasResp := ep.ResponseType != nil
	errRet := "err"
	if hasResp {
		retType := goType(*ep.ResponseType)
		fmt.Fprintf(&b, "\nfunc (r *%sOp) Do() (%s, error) {\n\tvar zero %s\n", opName, retType, retType)
		errRet = "zero, err"
	} else {
		fmt.Fprintf(&b, "\nfunc (r *%sOp) Do() error {\n", opName)
	}
	b.WriteString("\tvar buf bytes.Buffer\n\tw := multipart.NewWriter(&buf)\n")
	for _, f := range files {
		fmt.Fprintf(&b, "\tfw, err := w.CreateFormFile(%q, %q)\n\tif err != nil {\n\t\treturn %s\n\t}\n", f.Key, f.Key, errRet)
		fmt.Fprintf(&b, "\tif _, err := io.Copy(fw, r.%s); err != nil {\n\t\treturn %s\n\t}\n", goParamName(f.Name), errRet)
	}
	for _, f := range required {
		fmt.Fprintf(&b, "\tif err := w.WriteField(%q, %s); err != nil {\n\t\treturn %s\n\t}\n", f.Key, goFormValueExpr(f, false), errRet)
	}
	for _, f := range optional {
		fmt.Fprintf(&b, "\tif r.%s != nil {\n\t\tif err := w.WriteField(%q, %s); err != nil {\n\t\t\treturn %s\n\t\t}\n\t}\n", goParamName(f.Name), f.Key, goFormValueExpr(f, true), errRet)
	}
	fmt.Fprintf(&b, "\tif err := w.Close(); err != nil {\n\t\treturn %s\n\t}\n", errRet)

	fmtStr, args := goFmtPath(ep.Path)
	if len(args) > 0 {
		fmt.Fprintf(&b, "\tpath := fmt.Sprintf(%q", fmtStr)
		for _, a := range args {
			fmt.Fprintf(&b, ", r.%s", a)
		}
		b.WriteString(")\n")
	} else {
		fmt.Fprintf(&b, "\tpath := %q\n", fmtStr)
	}

	fmt.Fprintf(&b, "\tresp, err := r.client.doRaw(r.ctx, %q, path, w.FormDataContentType(), &buf)\n", ep.Method)
	if hasResp {
		retType := goType(*ep.ResponseType)
		b.WriteString("\tif err != nil {\n\t\treturn zero, err\n\t}\n\tdefer resp.Body.Close()\n")
		fmt.Fprintf(&b, "\tvar result %s\n", retType)
		b.WriteString("\tif err := json.NewDecoder(resp.Body).Decode(&result); err != nil {\n\t\treturn zero, fmt.Errorf(\"decoding response: %w\", err)\n\t}\n\treturn result, nil\n}\n")
	} else {
		b.WriteString("\tif err != nil {\n\t\treturn err\n\t}\n\tresp.Body.Close()\n\treturn nil\n}\n")
	}
	return b.String()
}

// goFormValueExpr renders the Go expression that stringifies a form field value
// for multipart.WriteField. Non-string scalars are wrapped in fmt.Sprint.
func goFormValueExpr(f ir.FormField, optional bool) string {
	ref := "r." + goParamName(f.Name)
	if optional {
		ref = "*" + ref
	}
	if f.Type.Kind == ir.TypePrimitive && f.Type.Prim == ir.PrimString {
		return ref
	}
	return fmt.Sprintf("fmt.Sprint(%s)", ref)
}

// --- Split-mode Go templates ---

const goErrorsTemplate = `// Code generated by oapi-liteclient. DO NOT EDIT.
package {{.Package}}

import "fmt"

// APIError is returned for non-2xx HTTP responses.
type APIError struct {
	StatusCode int
	Body       string
	Method     string
	Path       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}
`

const goModelsTemplate = `// Code generated by oapi-liteclient. DO NOT EDIT.
package {{.Package}}
{{range .Models}}
type {{.Name}} struct {
{{- range .Fields}}
{{- if .Required}}
	{{goFieldName .Name}} {{goType .Type}} ` + "`" + `json:"{{if .Alias}}{{.Alias}}{{else}}{{.Name}}{{end}}"` + "`" + `
{{- else}}
	{{goFieldName .Name}} *{{goType .Type}} ` + "`" + `json:"{{if .Alias}}{{.Alias}}{{else}}{{.Name}}{{end}},omitempty"` + "`" + `
{{- end}}
{{- end}}
}
{{end}}`

const goClientSplitTemplate = `// Code generated by oapi-liteclient. DO NOT EDIT.
package {{.Package}}

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
{{- if eq .AuthMode "gcp-id-token"}}
	"google.golang.org/api/idtoken"
{{- end}}
)

const DefaultBaseURL = "{{.BaseURL}}"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
{{- if eq .AuthMode "bearer-token"}}
	bearerToken string
{{- end}}
{{- if eq .AuthMode "api-key"}}
	apiKey       string
	apiKeyHeader string
{{- end}}
{{- if eq .AuthMode "custom"}}
	authFunc func(req *http.Request)
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
	tokenSource *idtoken.TokenSource
{{- end}}
}

{{- if eq .AuthMode "none"}}

func NewClient() *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient}
}
{{- end}}
{{- if eq .AuthMode "bearer-token"}}

func NewClient(bearerToken string) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, bearerToken: bearerToken}
}
{{- end}}
{{- if eq .AuthMode "api-key"}}

func NewClient(apiKey string) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, apiKey: apiKey, apiKeyHeader: "{{.Auth.Name}}"}
}
{{- end}}
{{- if eq .AuthMode "custom"}}

func NewClient(authFunc func(req *http.Request)) *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, authFunc: authFunc}
}
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}

func NewClient(targetAudience string) (*Client, error) {
	ts, err := idtoken.NewTokenSource(context.Background(), targetAudience)
	if err != nil {
		return nil, fmt.Errorf("creating token source: %w", err)
	}
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient, tokenSource: ts}, nil
}
{{- end}}

func (c *Client) do(ctx context.Context, method, path, contentType string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
		if contentType == "" {
			contentType = "application/json"
		}
	}
	return c.doRaw(ctx, method, path, contentType, reqBody)
}

func (c *Client) doRaw(ctx context.Context, method, path, contentType string, body io.Reader) (*http.Response, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
{{- if eq .AuthMode "bearer-token"}}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
{{- end}}
{{- if eq .AuthMode "api-key"}}
	req.Header.Set(c.apiKeyHeader, c.apiKey)
{{- end}}
{{- if eq .AuthMode "custom"}}
	if c.authFunc != nil {
		c.authFunc(req)
	}
{{- end}}
{{- if eq .AuthMode "gcp-id-token"}}
	token, err := c.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("getting ID token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
{{- end}}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(b), Method: method, Path: path}
	}
	return resp, nil
}
`

const goTagTemplate = `// Code generated by oapi-liteclient. DO NOT EDIT.
package {{.Package}}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)
{{range .Endpoints}}
{{- if .OperationID}}
{{- if isMultipart .}}{{goMultipartMethod .}}
{{- else}}
{{- $opName := goName .OperationID}}

type {{$opName}}Op struct {
	client *Client
	ctx    context.Context
{{- range pathParams .Params}}
	{{goParamName .Name}} {{goType .Type}}
{{- end}}
{{- if hasBody .RequestBody}}
	body {{goTypeDeref .RequestBody}}
{{- end}}
{{- range requiredQueryParams .Params}}
	{{goParamName .Name}} {{goType .Type}}
{{- end}}
{{- range optionalQueryParams .Params}}
	{{goParamName .Name}} *{{goType .Type}}
{{- end}}
}

// {{$opName}} {{docstring .}}
func (c *Client) {{$opName}}(ctx context.Context
{{- range pathParams .Params}}, {{goParamName .Name}} {{goType .Type}}{{end}}
{{- if hasBody .RequestBody}}, body {{goTypeDeref .RequestBody}}{{end}}
{{- range requiredQueryParams .Params}}, {{goParamName .Name}} {{goType .Type}}{{end}}) *{{$opName}}Op {
	return &{{$opName}}Op{
		client: c,
		ctx:    ctx,
{{- range pathParams .Params}}
		{{goParamName .Name}}: {{goParamName .Name}},
{{- end}}
{{- if hasBody .RequestBody}}
		body: body,
{{- end}}
{{- range requiredQueryParams .Params}}
		{{goParamName .Name}}: {{goParamName .Name}},
{{- end}}
	}
}
{{range optionalQueryParams .Params}}
func (r *{{$opName}}Op) {{goName .Name}}(v {{goType .Type}}) *{{$opName}}Op {
	r.{{goParamName .Name}} = &v
	return r
}
{{end}}
{{- if hasResponse .ResponseType}}
{{- if isArrayResponse .ResponseType}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- else if isRefResponse .ResponseType}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- else}}
func (r *{{$opName}}Op) Do() ({{goTypeDeref .ResponseType}}, error) {
{{- end}}
	query := url.Values{}
{{- range requiredQueryParams .Params}}
{{- if isStringType .Type}}
	query.Set("{{.Name}}", r.{{goParamName .Name}})
{{- else}}
	query.Set("{{.Name}}", fmt.Sprint(r.{{goParamName .Name}}))
{{- end}}
{{- end}}
{{- range optionalQueryParams .Params}}
	if r.{{goParamName .Name}} != nil {
{{- if isStringType .Type}}
		query.Set("{{.Name}}", *r.{{goParamName .Name}})
{{- else}}
		query.Set("{{.Name}}", fmt.Sprint(*r.{{goParamName .Name}}))
{{- end}}
	}
{{- end}}
	path := fmt.Sprintf("{{goFmtPath .Path}}"{{range goFmtPathArgs .Path}}, r.{{.}}{{end}})
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
{{- if hasBody .RequestBody}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "{{.RequestCType}}", r.body)
{{- else}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "", nil)
{{- end}}
	if err != nil {
{{- if isArrayResponse .ResponseType}}
		return nil, err
{{- else if isRefResponse .ResponseType}}
		return {{goTypeDeref .ResponseType}}{}, err
{{- else}}
		var zero {{goTypeDeref .ResponseType}}
		return zero, err
{{- end}}
	}
	defer resp.Body.Close()
	var result {{goTypeDeref .ResponseType}}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
{{- if isArrayResponse .ResponseType}}
		return nil, fmt.Errorf("decoding response: %w", err)
{{- else if isRefResponse .ResponseType}}
		return {{goTypeDeref .ResponseType}}{}, fmt.Errorf("decoding response: %w", err)
{{- else}}
		var zero {{goTypeDeref .ResponseType}}
		return zero, fmt.Errorf("decoding response: %w", err)
{{- end}}
	}
	return result, nil
}
{{- else}}
func (r *{{$opName}}Op) Do() error {
	query := url.Values{}
{{- range requiredQueryParams .Params}}
{{- if isStringType .Type}}
	query.Set("{{.Name}}", r.{{goParamName .Name}})
{{- else}}
	query.Set("{{.Name}}", fmt.Sprint(r.{{goParamName .Name}}))
{{- end}}
{{- end}}
{{- range optionalQueryParams .Params}}
	if r.{{goParamName .Name}} != nil {
{{- if isStringType .Type}}
		query.Set("{{.Name}}", *r.{{goParamName .Name}})
{{- else}}
		query.Set("{{.Name}}", fmt.Sprint(*r.{{goParamName .Name}}))
{{- end}}
	}
{{- end}}
	path := fmt.Sprintf("{{goFmtPath .Path}}"{{range goFmtPathArgs .Path}}, r.{{.}}{{end}})
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
{{- if hasBody .RequestBody}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "{{.RequestCType}}", r.body)
{{- else}}
	resp, err := r.client.do(r.ctx, "{{.Method}}", path, "", nil)
{{- end}}
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
{{- end}}
{{- end}}
{{- end}}
{{end}}`
