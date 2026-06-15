package generator

import (
	"strings"
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

func mustGeneratePython(t *testing.T, spec *ir.Spec, opts PythonOptions) map[string]string {
	t.Helper()
	files, err := GeneratePython(spec, opts)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}
	return files
}

func TestPyName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"petId", "pet_id"},
		{"userId", "user_id"},
		{"id", "id"},
		{"name", "name"},
		{"display_name", "display_name"},
		{"isActive", "is_active"},
		{"HTMLParser", "htmlparser"},   // consecutive caps stay together
		{"getURL", "get_url"},          // consecutive caps stay together
		{"sort.field", "sort_field"},   // dotted param name
		{"sort.dir", "sort_dir"},       // dotted param name
		{"filter-name", "filter_name"}, // hyphenated param name
		{"security-advisories/list-global-advisories", "security_advisories_list_global_advisories"}, // slashed operationId
		{"$ref", "ref"}, // dollar-prefixed field
		{"$schema", "schema"},
		{"+1", "plus_one"}, // special emoji reaction field
		{"-1", "minus_one"},
	}
	for _, tt := range tests {
		got := pyName(tt.in)
		if got != tt.want {
			t.Errorf("pyName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPyNameReservedWords(t *testing.T) {
	reserved := []string{"class", "type", "from", "import", "global", "return", "def", "if", "in", "is"}
	for _, word := range reserved {
		got := pyName(word)
		if got != word+"_" {
			t.Errorf("pyName(%q) = %q, want %q", word, got, word+"_")
		}
	}
}

func TestPyNameNonReserved(t *testing.T) {
	nonReserved := []string{"name", "status", "count", "items", "data"}
	for _, word := range nonReserved {
		got := pyName(word)
		if got != word {
			t.Errorf("pyName(%q) = %q, want %q (should not be escaped)", word, got, word)
		}
	}
}

func TestPyType(t *testing.T) {
	tests := []struct {
		in   ir.Type
		want string
	}{
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, "str"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, "int"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}, "float"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}, "bool"},
		{ir.Type{Kind: ir.TypeRef, Ref: "Pet"}, "Pet"},
		{ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}, "list[str]"},
		{ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}}, "list[Pet]"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimAny}, "Any"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBytes}, "bytes"},
	}
	for _, tt := range tests {
		got := pyType(tt.in)
		if got != tt.want {
			t.Errorf("pyType(%+v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGeneratePythonBytesResponse(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Binary API",
		BaseURL: "https://api.example.com",
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "downloadQuotePdf",
				Method:       "GET",
				Path:         "/quotes/{quoteId}/pdf",
				Params:       []ir.Param{{Name: "quoteId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBytes},
			},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none"})["client.py"]

	checks := []string{
		"def download_quote_pdf(",
		"quote_id: int",
		") -> bytes:",
		"return resp.content",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
	if strings.Contains(output, "resp.json()") {
		t.Errorf("bytes endpoint should not call resp.json()\n\nFull output:\n%s", output)
	}
}

func TestGeneratePythonBytesResponseTagSplit(t *testing.T) {
	spec := &ir.Spec{
		Title: "Tagged Binary API",
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "downloadQuotePdf",
				Method:       "GET",
				Path:         "/quotes/{quoteId}/pdf",
				Tags:         []string{"Quote"},
				Params:       []ir.Param{{Name: "quoteId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBytes},
			},
		},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none"})
	quote := files["quote.py"]
	if !strings.Contains(quote, ") -> bytes:") {
		t.Errorf("tag client should type bytes response\n\nFull output:\n%s", quote)
	}
	if !strings.Contains(quote, "return resp.content") {
		t.Errorf("tag client should return resp.content\n\nFull output:\n%s", quote)
	}
}

func TestPyReturnRenderingUnchangedAcrossTemplates(t *testing.T) {
	cases := []struct {
		name string
		rt   ir.Type
		want string
	}{
		{name: "ref", rt: ir.Type{Kind: ir.TypeRef, Ref: "Quote"}, want: "return Quote.model_validate(resp.json())"},
		{name: "array_ref", rt: ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Quote"}}, want: "return [Quote.model_validate(item) for item in resp.json()]"},
		{name: "map", rt: ir.Type{Kind: ir.TypeMap, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}, want: "return resp.json()"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := &ir.Spec{
				Title:  "Ret API",
				Models: []ir.Model{{Name: "Quote", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}}}}},
				Endpoints: []ir.Endpoint{{
					OperationID:  "getThing",
					Method:       "GET",
					Path:         "/thing",
					ResponseType: &tc.rt,
				}},
			}
			out := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none"})["client.py"]
			if !strings.Contains(out, tc.want) {
				t.Errorf("output missing %q\n%s", tc.want, out)
			}
		})
	}
}

func TestFmtPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/pets", "/pets"},
		{"/pets/{petId}", "/pets/{pet_id}"},
		{"/users/{userId}/orders", "/users/{user_id}/orders"},
		{"/orders/{orderId}/items/{itemId}", "/orders/{order_id}/items/{item_id}"},
	}
	for _, tt := range tests {
		got := fmtPath(tt.in)
		if got != tt.want {
			t.Errorf("fmtPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGeneratePythonMinimal(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{
				Name: "Item",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "price", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}, Required: false},
				},
			},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "getItem",
				Method:       "GET",
				Path:         "/items/{itemId}",
				Params:       []ir.Param{{Name: "itemId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "Item"},
			},
		},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})
	output := files["client.py"]

	checks := []string{
		"class Item(BaseModel):",
		"id: int",
		"name: str",
		"price: Optional[float] = None",
		"def get_item(",
		"item_id: int",
		`f"/items/{item_id}"`,
		"return Item.model_validate(resp.json())",
		"class APIError(Exception):",
		"class Client:",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestGeneratePythonCustomContentType(t *testing.T) {
	spec := &ir.Spec{
		Title: "Patch API",
		Models: []ir.Model{
			{Name: "Item", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "patchItem",
				Method:       "PATCH",
				Path:         "/items/{id}",
				Params:       []ir.Param{{Name: "id", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}},
				RequestBody:  &ir.Type{Kind: ir.TypeRef, Ref: "Item"},
				RequestCType: "application/json-patch+json",
			},
			{
				OperationID:  "replaceItem",
				Method:       "PUT",
				Path:         "/items/{id}",
				Params:       []ir.Param{{Name: "id", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}},
				RequestBody:  &ir.Type{Kind: ir.TypeRef, Ref: "Item"},
				RequestCType: "application/json",
			},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none"})["client.py"]

	if !strings.Contains(output, `headers={"Content-Type": "application/json-patch+json"}`) {
		t.Error("PATCH endpoint should emit explicit Content-Type header for non-default media type")
	}
	if strings.Contains(output, `headers={"Content-Type": "application/json"}`) {
		t.Error("default application/json body should not emit an explicit Content-Type header")
	}
}

func multipartSpec() *ir.Spec {
	return &ir.Spec{
		Title: "Multipart API",
		Models: []ir.Model{
			{Name: "CreatedResponseDto", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "createAttachment",
				Method:       "POST",
				Path:         "/api/attachments",
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "CreatedResponseDto"},
				FormFields: []ir.FormField{
					{Key: "File", Name: "File", IsFile: true, Required: true},
					{Key: "Detail.Owner.Type", Name: "Owner.Type", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Key: "Detail.Owner.Id", Name: "Owner.Id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Key: "Detail.IsNoteAttachment", Name: "IsNoteAttachment", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}},
				},
			},
		},
	}
}

func TestGeneratePythonMultipart(t *testing.T) {
	output := mustGeneratePython(t, multipartSpec(), PythonOptions{Style: "pydantic", Auth: "none"})["client.py"]

	checks := []string{
		"def create_attachment(",
		"        file,",
		"        owner_type: str,",
		"        owner_id: str,",
		"        is_note_attachment: Optional[bool] = None,",
		`        files = {"File": file}`,
		`        data["Detail.Owner.Type"] = owner_type`,
		`        data["Detail.Owner.Id"] = owner_id`,
		"        if is_note_attachment is not None:",
		`            data["Detail.IsNoteAttachment"] = is_note_attachment`,
		"            files=files,",
		"            data=data,",
		"        return CreatedResponseDto.model_validate(resp.json())",
	}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("output missing %q", c)
		}
	}
	if strings.Contains(output, "json=") {
		t.Error("multipart endpoint should not serialize body as json=")
	}
}

func TestGeneratePythonMultipartDataclass(t *testing.T) {
	output := mustGeneratePython(t, multipartSpec(), PythonOptions{Style: "dataclass", Auth: "none"})["client.py"]
	if !strings.Contains(output, "return CreatedResponseDto(**resp.json())") {
		t.Error("dataclass multipart should construct response via (**resp.json())")
	}
}

func TestGeneratePythonMultipartTagSplitUsesBaseClientRequest(t *testing.T) {
	for _, style := range []string{"pydantic", "dataclass"} {
		t.Run(style, func(t *testing.T) {
			spec := multipartSpec()
			spec.Endpoints[0].Tags = []string{"Attachments"}

			files := mustGeneratePython(t, spec, PythonOptions{Style: style, Auth: "none"})
			attachments, ok := files["attachments.py"]
			if !ok {
				t.Fatalf("missing attachments.py, got files: %v", fileNames(files))
			}
			if !strings.Contains(attachments, "resp = self._client._request(") {
				t.Error("split multipart method should call through the BaseClient")
			}
			if strings.Contains(attachments, "resp = self._request(") {
				t.Error("split multipart method should not call an undefined sub-client _request method")
			}
		})
	}
}

func TestGeneratePythonPackageVersion(t *testing.T) {
	spec := &ir.Spec{
		Title:     "Versioned API",
		Endpoints: []ir.Endpoint{{OperationID: "ping", Method: "GET", Path: "/ping"}},
	}

	custom := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none", Package: "myclient", PackageVersion: "2.3.0"})["pyproject.toml"]
	if !strings.Contains(custom, `version = "2.3.0"`) {
		t.Errorf("pyproject.toml should use the supplied version\n%s", custom)
	}

	def := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none", Package: "myclient"})["pyproject.toml"]
	if !strings.Contains(def, `version = "0.1.0"`) {
		t.Errorf("pyproject.toml should default to 0.1.0 when version is empty\n%s", def)
	}
}

func TestGeneratePythonNoAuth(t *testing.T) {
	spec := &ir.Spec{
		Title: "No Auth API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "none"})["client.py"]

	if strings.Contains(output, "api_key") || strings.Contains(output, "bearer_token") {
		t.Error("output should not contain auth params for none mode")
	}
	if strings.Contains(output, "self._auth") {
		t.Error("output should not contain auth logic for none mode")
	}
	if strings.Contains(output, "from collections.abc import Callable") {
		t.Error("output should not import Callable for none mode")
	}
}

func TestGeneratePythonAuthCallable(t *testing.T) {
	spec := &ir.Spec{
		Title: "Auth API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "custom"})["client.py"]

	if !strings.Contains(output, "auth: Callable[[], dict[str, str]] | None = None") {
		t.Error("output should have auth callable parameter")
	}
	if !strings.Contains(output, "self._auth = auth") {
		t.Error("output should store auth callable")
	}
	if !strings.Contains(output, "self._auth()") {
		t.Error("output should call auth callable in _request")
	}
	if !strings.Contains(output, "from collections.abc import Callable") {
		t.Error("output should import Callable for custom auth mode")
	}
}

func TestGeneratePythonAuthBearerToken(t *testing.T) {
	spec := &ir.Spec{
		Title: "Bearer API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "bearer-token"})["client.py"]

	if !strings.Contains(output, "bearer_token: str = \"\"") {
		t.Error("output should have bearer_token parameter")
	}
	if !strings.Contains(output, "self._bearer_token = bearer_token") {
		t.Error("output should store bearer_token")
	}
	if !strings.Contains(output, `headers["Authorization"] = f"Bearer {self._bearer_token}"`) {
		t.Error("output should set Authorization header in _request")
	}
	if strings.Contains(output, "from collections.abc import Callable") {
		t.Error("output should not import Callable for bearer-token mode")
	}
}

func TestGeneratePythonAuthAPIKey(t *testing.T) {
	spec := &ir.Spec{
		Title: "API Key API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "api-key"})["client.py"]

	if !strings.Contains(output, "api_key: str = \"\"") {
		t.Error("output should have api_key parameter")
	}
	if !strings.Contains(output, `api_key_header: str = "X-API-Key"`) {
		t.Error("output should have api_key_header parameter")
	}
	if !strings.Contains(output, "self._api_key = api_key") {
		t.Error("output should store api_key")
	}
	if !strings.Contains(output, "headers[self._api_key_header] = self._api_key") {
		t.Error("output should set API key header in _request")
	}
}

func TestGeneratePythonAuthGCPIDToken(t *testing.T) {
	spec := &ir.Spec{
		Title: "GCP API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Auth: "gcp-id-token"})["client.py"]

	if !strings.Contains(output, "import time") {
		t.Error("output should import time for gcp-id-token mode")
	}
	if !strings.Contains(output, "import google.auth.transport.requests") {
		t.Error("output should import google.auth.transport.requests")
	}
	if !strings.Contains(output, "import google.oauth2.id_token") {
		t.Error("output should import google.oauth2.id_token")
	}
	if !strings.Contains(output, "self._token: str | None = None") {
		t.Error("output should initialize _token field")
	}
	if !strings.Contains(output, "self._token_expiry: float = 0") {
		t.Error("output should initialize _token_expiry field")
	}
	if !strings.Contains(output, "def _get_auth_headers(self)") {
		t.Error("output should have _get_auth_headers method")
	}
	if !strings.Contains(output, "self._token_expiry = now + 3300") {
		t.Error("output should cache token for 3300 seconds")
	}
	if !strings.Contains(output, "headers.update(self._get_auth_headers())") {
		t.Error("output should call _get_auth_headers in _request")
	}
	if strings.Contains(output, "from collections.abc import Callable") {
		t.Error("output should not import Callable for gcp-id-token mode")
	}
}

func TestGeneratePythonAuthDefaultIsNone(t *testing.T) {
	spec := &ir.Spec{
		Title: "Default API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})["client.py"]

	if strings.Contains(output, "self._auth") || strings.Contains(output, "bearer_token") ||
		strings.Contains(output, "api_key") || strings.Contains(output, "_get_auth_headers") {
		t.Error("default (empty) auth should produce no auth code")
	}
}

func TestResolveAuth(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		auth     *ir.Auth
		want     string
	}{
		{"explicit wins over spec", "custom", &ir.Auth{Type: ir.AuthBearer}, "custom"},
		{"explicit none wins over spec", "none", &ir.Auth{Type: ir.AuthBearer}, "none"},
		{"auto-detect bearer", "", &ir.Auth{Type: ir.AuthBearer, Name: "Authorization"}, "bearer-token"},
		{"auto-detect api-key", "", &ir.Auth{Type: ir.AuthAPIKey, Name: "X-API-Key", In: "header"}, "api-key"},
		{"no spec auth defaults to none", "", nil, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &ir.Spec{Auth: tt.auth}
			got := resolveAuth(tt.explicit, spec)
			if got != tt.want {
				t.Errorf("resolveAuth(%q, %+v) = %q, want %q", tt.explicit, tt.auth, got, tt.want)
			}
		})
	}
}

func TestGeneratePythonAutoDetectBearer(t *testing.T) {
	spec := &ir.Spec{
		Title: "Auto Bearer API",
		Auth:  &ir.Auth{Type: ir.AuthBearer, Name: "Authorization", In: "header"},
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})["client.py"]

	if !strings.Contains(output, "bearer_token: str") {
		t.Error("auto-detected bearer auth should produce bearer_token parameter")
	}
	if !strings.Contains(output, `"Authorization"`) {
		t.Error("auto-detected bearer auth should set Authorization header")
	}
}

func TestGeneratePythonArrayResponse(t *testing.T) {
	spec := &ir.Spec{
		Title: "Array API",
		Models: []ir.Model{
			{Name: "Pet", Fields: []ir.Field{{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "listPets",
				Method:       "GET",
				Path:         "/pets",
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}},
			},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})["client.py"]

	if !strings.Contains(output, "[Pet.model_validate(item) for item in resp.json()]") {
		t.Error("array of refs should deserialize with model_validate")
	}
}

func TestGeneratePythonArrayOfPrimitivesResponse(t *testing.T) {
	spec := &ir.Spec{
		Title: "Primitive Array API",
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "getTags",
				Method:       "GET",
				Path:         "/tags",
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}},
			},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})["client.py"]

	if !strings.Contains(output, "return resp.json()") {
		t.Error("array of primitives should return resp.json() directly")
	}
	if strings.Contains(output, "(**item)") {
		t.Error("array of primitives should not try to deserialize items into a model")
	}
}

func TestGeneratePythonRequiredQueryParams(t *testing.T) {
	spec := &ir.Spec{
		Title: "Params API",
		Endpoints: []ir.Endpoint{
			{
				OperationID: "search",
				Method:      "GET",
				Path:        "/search",
				Params: []ir.Param{
					{Name: "q", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "limit", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: false},
				},
			},
		},
	}

	output := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})["client.py"]

	qIdx := strings.Index(output, "q: str,")
	limitIdx := strings.Index(output, "limit: Optional[int]")
	if qIdx == -1 {
		t.Fatal("missing required param 'q: str'")
	}
	if limitIdx == -1 {
		t.Fatal("missing optional param 'limit'")
	}
	if qIdx > limitIdx {
		t.Error("required query param should come before optional")
	}

	if strings.Contains(output, "if q is not None") {
		t.Error("required param should not have 'is not None' check")
	}
}

// --- Tag-based splitting tests ---

func TestGeneratePythonTagSplit(t *testing.T) {
	spec := &ir.Spec{
		Title: "Tagged API",
		Models: []ir.Model{
			{Name: "Pet", Fields: []ir.Field{{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}}},
			{Name: "User", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{OperationID: "listPets", Method: "GET", Path: "/pets", Tags: []string{"Pets"},
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}}},
			{OperationID: "createPet", Method: "POST", Path: "/pets", Tags: []string{"Pets"},
				RequestBody: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}},
			{OperationID: "getUser", Method: "GET", Path: "/users/{userId}", Tags: []string{"Users"},
				Params:       []ir.Param{{Name: "userId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "User"}},
		},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})

	expectedFiles := []string{"__init__.py", "_base.py", "models.py", "client.py", "pets.py", "users.py"}
	for _, f := range expectedFiles {
		if _, ok := files[f]; !ok {
			t.Errorf("missing expected file %q, got files: %v", f, fileNames(files))
		}
	}

	if _, ok := files["client.py"]; ok {
		if !strings.Contains(files["client.py"], "class Client(BaseClient):") {
			t.Error("client.py should contain Client(BaseClient)")
		}
		if !strings.Contains(files["client.py"], "self.pets = PetsClient(self)") {
			t.Error("client.py should compose PetsClient")
		}
		if !strings.Contains(files["client.py"], "self.users = UsersClient(self)") {
			t.Error("client.py should compose UsersClient")
		}
	}

	if pets, ok := files["pets.py"]; ok {
		if !strings.Contains(pets, "class PetsClient:") {
			t.Error("pets.py should contain PetsClient class")
		}
		if !strings.Contains(pets, "def list_pets(") {
			t.Error("pets.py should contain list_pets method")
		}
		if !strings.Contains(pets, "def create_pet(") {
			t.Error("pets.py should contain create_pet method")
		}
	}

	if users, ok := files["users.py"]; ok {
		if !strings.Contains(users, "class UsersClient:") {
			t.Error("users.py should contain UsersClient class")
		}
		if !strings.Contains(users, "def get_user(") {
			t.Error("users.py should contain get_user method")
		}
	}

	if base, ok := files["_base.py"]; ok {
		if !strings.Contains(base, "class APIError") {
			t.Error("_base.py should contain APIError")
		}
		if !strings.Contains(base, "class BaseClient:") {
			t.Error("_base.py should contain BaseClient")
		}
	}

	if models, ok := files["models.py"]; ok {
		if !strings.Contains(models, "class Pet(BaseModel):") {
			t.Error("models.py should contain Pet model")
		}
		if !strings.Contains(models, "class User(BaseModel):") {
			t.Error("models.py should contain User model")
		}
	}

	if init, ok := files["__init__.py"]; ok {
		if !strings.Contains(init, "from .client import Client") {
			t.Error("__init__.py should re-export Client")
		}
		if !strings.Contains(init, "from ._base import APIError") {
			t.Error("__init__.py should re-export APIError")
		}
	}
}

func TestGeneratePythonTagMerge(t *testing.T) {
	spec := &ir.Spec{
		Title: "Invoice API",
		Models: []ir.Model{
			{Name: "Invoice", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}}},
			{Name: "InvoiceLineItem", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{OperationID: "listInvoices", Method: "GET", Path: "/invoices", Tags: []string{"Invoice"},
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Invoice"}}},
			{OperationID: "getInvoiceLineItem", Method: "GET", Path: "/invoices/{id}/line-items/{lineId}", Tags: []string{"Invoice Line Item"},
				Params: []ir.Param{
					{Name: "id", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "lineId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
				},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "InvoiceLineItem"}},
			{OperationID: "listCustomers", Method: "GET", Path: "/customers", Tags: []string{"Customer"}},
		},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})

	if _, ok := files["invoice_line_item.py"]; ok {
		t.Error("Invoice Line Item should be merged into invoice.py, not its own file")
	}

	invoice, ok := files["invoice.py"]
	if !ok {
		t.Fatal("missing invoice.py")
	}
	if !strings.Contains(invoice, "class InvoiceClient:") {
		t.Error("invoice.py should contain InvoiceClient class")
	}
	if !strings.Contains(invoice, "def list_invoices(") {
		t.Error("invoice.py should contain list_invoices method")
	}
	if !strings.Contains(invoice, "def get_invoice_line_item(") {
		t.Error("invoice.py should contain get_invoice_line_item (merged from Invoice Line Item tag)")
	}

	if _, ok := files["customer.py"]; !ok {
		t.Error("customer.py should still exist as a separate file")
	}

	if client, ok := files["client.py"]; ok {
		if !strings.Contains(client, "self.invoice = InvoiceClient(self)") {
			t.Error("client.py should have invoice attribute, not invoice_line_item")
		}
		if strings.Contains(client, "InvoiceLineItemClient") {
			t.Error("client.py should not have a separate InvoiceLineItemClient")
		}
	}
}

func TestGeneratePythonNoTagsSingleFile(t *testing.T) {
	spec := &ir.Spec{
		Title: "Simple API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})

	if len(files) != 1 {
		t.Errorf("expected 1 file for no-tag spec, got %d: %v", len(files), fileNames(files))
	}
	if _, ok := files["client.py"]; !ok {
		t.Error("no-tag spec should produce client.py")
	}
}

func TestPyProjectTOML(t *testing.T) {
	spec := &ir.Spec{
		Title:     "Test API",
		Endpoints: []ir.Endpoint{{OperationID: "ping", Method: "GET", Path: "/ping"}},
	}

	files := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic", Package: "my-client"})
	toml, ok := files["pyproject.toml"]
	if !ok {
		t.Fatal("expected pyproject.toml in output")
	}
	if !strings.Contains(toml, `name = "my_client"`) {
		t.Error("pyproject.toml should sanitize hyphens to underscores in name")
	}
	if !strings.Contains(toml, `"pydantic>=2"`) {
		t.Error("pydantic style should include pydantic dependency")
	}
	if !strings.Contains(toml, `"httpx>=0.27"`) {
		t.Error("should include httpx dependency")
	}
	if !strings.Contains(toml, `packages = ["."]`) {
		t.Error("should include hatch packages config")
	}
	if !strings.Contains(toml, `build-backend = "hatchling.build"`) {
		t.Error("should use hatchling build backend")
	}

	dcFiles := mustGeneratePython(t, spec, PythonOptions{Style: "dataclass", Package: "my-client"})
	dcToml := dcFiles["pyproject.toml"]
	if strings.Contains(dcToml, "pydantic") {
		t.Error("dataclass style should not include pydantic dependency")
	}

	noPackage := mustGeneratePython(t, spec, PythonOptions{Style: "pydantic"})
	if _, ok := noPackage["pyproject.toml"]; ok {
		t.Error("should not generate pyproject.toml when Package is empty")
	}
}

func TestGeneratePythonLenient(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.example.com",
		Models: []ir.Model{
			{
				Name: "Pet",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "tag", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
				},
			},
		},
		Endpoints: []ir.Endpoint{{OperationID: "getPet", Method: "GET", Path: "/pets/{petId}"}},
	}

	strict := mustGeneratePython(t, spec, PythonOptions{})
	code := strict["client.py"]
	if !strings.Contains(code, "id: int") {
		t.Error("strict mode: required field 'id' should not be Optional")
	}

	lenient := mustGeneratePython(t, spec, PythonOptions{Lenient: true})
	code = lenient["client.py"]
	if strings.Contains(code, "id: int\n") {
		t.Error("lenient mode: required field 'id' should become Optional")
	}
	if !strings.Contains(code, "Optional[int]") {
		t.Error("lenient mode: 'id' should be Optional[int]")
	}
	if !strings.Contains(code, "Optional[str]") {
		t.Error("lenient mode: all fields should be Optional")
	}
}

func fileNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	return names
}
