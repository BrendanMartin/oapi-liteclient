package generator

import (
	"go/format"
	"strings"
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

func mustGenerateGo(t *testing.T, spec *ir.Spec, opts GoOptions) map[string]string {
	t.Helper()
	files, err := GenerateGo(spec, opts)
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}
	return files
}

func TestGoName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"petId", "PetId"},
		{"userId", "UserId"},
		{"id", "Id"},
		{"name", "Name"},
		{"display_name", "DisplayName"},
		{"isActive", "IsActive"},
		{"getURL", "GetURL"},
		{"pet_id", "PetId"},
		{"list_pets", "ListPets"},
		{"HTMLParser", "HTMLParser"},
		{"already_Pascal", "AlreadyPascal"},
		{"sort.field", "SortField"},
		{"sort.dir", "SortDir"},
		{"filter-name", "FilterName"},
		{"security-advisories/list-global-advisories", "SecurityAdvisoriesListGlobalAdvisories"},
		{"$ref", "Ref"},
		{"$schema", "Schema"},
		{"+1", "PlusOne"},
		{"-1", "MinusOne"},
	}
	for _, tt := range tests {
		got := goName(tt.in)
		if got != tt.want {
			t.Errorf("goName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGoFieldNameReservedWords(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"type", "Type_"},
		{"range", "Range_"},
		{"map", "Map_"},
		{"func", "Func_"},
		{"chan", "Chan_"},
		{"name", "Name"},
		{"status", "Status"},
	}
	for _, tt := range tests {
		got := goFieldName(tt.in)
		if got != tt.want {
			t.Errorf("goFieldName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGoType(t *testing.T) {
	tests := []struct {
		in   ir.Type
		want string
	}{
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, "string"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, "int"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}, "float64"},
		{ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}, "bool"},
		{ir.Type{Kind: ir.TypeRef, Ref: "Pet"}, "Pet"},
		{ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}, "[]string"},
		{ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}}, "[]Pet"},
	}
	for _, tt := range tests {
		got := goType(tt.in)
		if got != tt.want {
			t.Errorf("goType(%+v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGoFmtPath(t *testing.T) {
	tests := []struct {
		in       string
		wantFmt  string
		wantArgs []string
	}{
		{"/pets", "/pets", nil},
		{"/pets/{petId}", "/pets/%v", []string{"petId"}},
		{"/users/{userId}/orders", "/users/%v/orders", []string{"userId"}},
		{"/orders/{orderId}/items/{itemId}", "/orders/%v/items/%v", []string{"orderId", "itemId"}},
	}
	for _, tt := range tests {
		gotFmt, gotArgs := goFmtPath(tt.in)
		if gotFmt != tt.wantFmt {
			t.Errorf("goFmtPath(%q) fmt = %q, want %q", tt.in, gotFmt, tt.wantFmt)
		}
		if len(gotArgs) != len(tt.wantArgs) {
			t.Errorf("goFmtPath(%q) args len = %d, want %d", tt.in, len(gotArgs), len(tt.wantArgs))
			continue
		}
		for i := range gotArgs {
			if gotArgs[i] != tt.wantArgs[i] {
				t.Errorf("goFmtPath(%q) args[%d] = %q, want %q", tt.in, i, gotArgs[i], tt.wantArgs[i])
			}
		}
	}
}

func TestGenerateGoModels(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{
				Name: "Pet",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "tag", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
					{Name: "vaccinated", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}, Required: false},
				},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"package petstore",
		"type Pet struct {",
		"type APIError struct {",
		"StatusCode int",
		"func (e *APIError) Error() string {",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoNestedModels(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{
				Name: "Address",
				Fields: []ir.Field{
					{Name: "street", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "city", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
				},
			},
			{
				Name: "User",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "address", Type: ir.Type{Kind: ir.TypeRef, Ref: "Address"}, Required: false},
					{Name: "tags", Type: ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}}, Required: false},
					{Name: "scores", Type: ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}}, Required: false},
				},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"Address *Address",
		"Tags    *[]string",
		"Scores  *[]float64",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoClientNoAuth(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"type Client struct {",
		"BaseURL    string",
		"HTTPClient *http.Client",
		"func NewClient() *Client {",
		"http.DefaultClient",
		"DefaultBaseURL",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}

	noChecks := []string{"bearerToken", "apiKey", "authFunc", "idtoken"}
	for _, check := range noChecks {
		if strings.Contains(output, check) {
			t.Errorf("output should not contain %q for no-auth mode", check)
		}
	}
}

func TestGenerateGoSimpleEndpoint(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{
				Name: "Pet",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
				},
			},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "getPet",
				Method:       "GET",
				Path:         "/pets/{petId}",
				Params:       []ir.Param{{Name: "petId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"type GetPetOp struct {",
		"func (c *Client) GetPet(ctx context.Context, petId int) *GetPetOp {",
		"func (r *GetPetOp) Do() (Pet, error) {",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoOptionalQueryParams(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{Name: "Pet", Fields: []ir.Field{{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID: "listPets",
				Method:      "GET",
				Path:        "/pets",
				Params: []ir.Param{
					{Name: "limit", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: false},
					{Name: "tag", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
				},
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"func (c *Client) ListPets(ctx context.Context) *ListPetsOp {",
		"func (r *ListPetsOp) Limit(v int) *ListPetsOp {",
		"func (r *ListPetsOp) Tag(v string) *ListPetsOp {",
		"func (r *ListPetsOp) Do() ([]Pet, error) {",
		"limit  *int",
		"tag    *string",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoRequiredQueryParams(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{Name: "User", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID: "listUsers",
				Method:      "GET",
				Path:        "/users",
				Params: []ir.Param{
					{Name: "is_active", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}, Required: true},
					{Name: "limit", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: false},
				},
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "User"}},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	if !strings.Contains(output, "func (c *Client) ListUsers(ctx context.Context, isActive bool) *ListUsersOp {") {
		t.Errorf("required query param should be positional arg\n\nFull output:\n%s", output)
	}

	if !strings.Contains(output, "func (r *ListUsersOp) Limit(v int) *ListUsersOp {") {
		t.Errorf("optional query param should be chained setter\n\nFull output:\n%s", output)
	}
}

func TestGenerateGoRequestBody(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Models: []ir.Model{
			{Name: "Pet", Fields: []ir.Field{{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}}},
			{Name: "PetCreate", Fields: []ir.Field{{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true}}},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID:  "createPet",
				Method:       "POST",
				Path:         "/pets",
				RequestBody:  &ir.Type{Kind: ir.TypeRef, Ref: "PetCreate"},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"func (c *Client) CreatePet(ctx context.Context, body PetCreate) *CreatePetOp {",
		"func (r *CreatePetOp) Do() (Pet, error) {",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoNoContent(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
		Endpoints: []ir.Endpoint{
			{
				OperationID: "deletePet",
				Method:      "DELETE",
				Path:        "/pets/{petId}",
				Params:      []ir.Param{{Name: "petId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	if !strings.Contains(output, "func (r *DeletePetOp) Do() error {") {
		t.Errorf("204 endpoint should return just error\n\nFull output:\n%s", output)
	}
}

func TestGenerateGoArrayResponse(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Test API",
		BaseURL: "https://api.test.com",
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

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	if !strings.Contains(output, "func (r *ListPetsOp) Do() ([]Pet, error) {") {
		t.Errorf("array response should return []Pet\n\nFull output:\n%s", output)
	}
}

func TestGenerateGoAuthBearerToken(t *testing.T) {
	spec := &ir.Spec{
		Title: "Bearer API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "bearer-token", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"bearerToken string",
		"func NewClient(bearerToken string) *Client {",
		`req.Header.Set("Authorization", "Bearer "+c.bearerToken)`,
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoAuthAPIKey(t *testing.T) {
	spec := &ir.Spec{
		Title: "API Key API",
		Auth:  &ir.Auth{Type: ir.AuthAPIKey, Name: "X-API-Key", In: "header"},
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "api-key", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"apiKey       string",
		"apiKeyHeader string",
		"func NewClient(apiKey string) *Client {",
		`apiKeyHeader: "X-API-Key"`,
		"req.Header.Set(c.apiKeyHeader, c.apiKey)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoAuthCustom(t *testing.T) {
	spec := &ir.Spec{
		Title: "Custom Auth API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "custom", Package: "testapi"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"authFunc func(req *http.Request)",
		"func NewClient(authFunc func(req *http.Request)) *Client {",
		"c.authFunc(req)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoAuthGCPIDToken(t *testing.T) {
	spec := &ir.Spec{
		Title: "GCP API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "gcp-id-token", Package: "testapi"})
	output := files["client.go"]

	checks := []string{
		`"google.golang.org/api/idtoken"`,
		"func NewClient(targetAudience string) (*Client, error) {",
		"idtoken.NewTokenSource",
		"c.tokenSource.Token()",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestGenerateGoAuthDefaultIsNone(t *testing.T) {
	spec := &ir.Spec{
		Title: "Default API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Package: "testapi"})
	output := files["client.go"]

	noChecks := []string{"bearerToken", "apiKey", "authFunc", "idtoken", "tokenSource"}
	for _, check := range noChecks {
		if strings.Contains(output, check) {
			t.Errorf("default auth should not contain %q", check)
		}
	}
}

func TestGenerateGoPetstore(t *testing.T) {
	spec := &ir.Spec{
		Title:   "Petstore",
		BaseURL: "https://petstore.example.com/v1",
		Auth:    &ir.Auth{Type: ir.AuthAPIKey, Name: "X-API-Key", In: "header"},
		Models: []ir.Model{
			{
				Name: "Pet",
				Fields: []ir.Field{
					{Name: "id", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true},
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "tag", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
				},
			},
			{
				Name: "PetCreate",
				Fields: []ir.Field{
					{Name: "name", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: true},
					{Name: "tag", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
				},
			},
		},
		Endpoints: []ir.Endpoint{
			{
				OperationID: "listPets",
				Summary:     "List all pets",
				Method:      "GET",
				Path:        "/pets",
				Params: []ir.Param{
					{Name: "limit", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: false},
					{Name: "tag", In: "query", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}, Required: false},
				},
				ResponseType: &ir.Type{Kind: ir.TypeArray, Elem: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"}},
			},
			{
				OperationID:  "createPet",
				Summary:      "Create a pet",
				Method:       "POST",
				Path:         "/pets",
				RequestBody:  &ir.Type{Kind: ir.TypeRef, Ref: "PetCreate"},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"},
			},
			{
				OperationID:  "getPet",
				Method:       "GET",
				Path:         "/pets/{petId}",
				Params:       []ir.Param{{Name: "petId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
				ResponseType: &ir.Type{Kind: ir.TypeRef, Ref: "Pet"},
			},
			{
				OperationID: "deletePet",
				Method:      "DELETE",
				Path:        "/pets/{petId}",
				Params:      []ir.Param{{Name: "petId", In: "path", Type: ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}, Required: true}},
			},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "api-key", Package: "petstore"})
	output := files["client.go"]

	if _, err := format.Source([]byte(output)); err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, output)
	}

	checks := []string{
		"package petstore",
		"type Pet struct {",
		"type PetCreate struct {",
		"type Client struct {",
		"type ListPetsOp struct {",
		"type CreatePetOp struct {",
		"type GetPetOp struct {",
		"type DeletePetOp struct {",
		"func (c *Client) ListPets(ctx context.Context) *ListPetsOp {",
		"func (r *ListPetsOp) Limit(v int) *ListPetsOp {",
		"func (r *ListPetsOp) Tag(v string) *ListPetsOp {",
		"func (r *ListPetsOp) Do() ([]Pet, error) {",
		"func (c *Client) CreatePet(ctx context.Context, body PetCreate) *CreatePetOp {",
		"func (r *CreatePetOp) Do() (Pet, error) {",
		"func (c *Client) GetPet(ctx context.Context, petId int) *GetPetOp {",
		"func (r *GetPetOp) Do() (Pet, error) {",
		"func (c *Client) DeletePet(ctx context.Context, petId int) *DeletePetOp {",
		"func (r *DeletePetOp) Do() error {",
		"apiKey",
		"apiKeyHeader",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

// --- Tag-based splitting tests ---

func TestGenerateGoTagSplit(t *testing.T) {
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

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "myapi"})

	expectedFiles := []string{"errors.go", "models.go", "client.go", "pets.go", "users.go"}
	for _, f := range expectedFiles {
		content, ok := files[f]
		if !ok {
			t.Errorf("missing expected file %q", f)
			continue
		}
		if _, err := format.Source([]byte(content)); err != nil {
			t.Errorf("%s is not valid Go: %v\n%s", f, err, content)
		}
		if !strings.Contains(content, "package myapi") {
			t.Errorf("%s missing package declaration", f)
		}
	}

	if errors, ok := files["errors.go"]; ok {
		if !strings.Contains(errors, "type APIError struct {") {
			t.Error("errors.go should contain APIError")
		}
	}

	if models, ok := files["models.go"]; ok {
		if !strings.Contains(models, "type Pet struct {") {
			t.Error("models.go should contain Pet struct")
		}
		if !strings.Contains(models, "type User struct {") {
			t.Error("models.go should contain User struct")
		}
	}

	if client, ok := files["client.go"]; ok {
		if !strings.Contains(client, "type Client struct {") {
			t.Error("client.go should contain Client struct")
		}
		if !strings.Contains(client, "func NewClient(") {
			t.Error("client.go should contain NewClient")
		}
		if !strings.Contains(client, "func (c *Client) do(") {
			t.Error("client.go should contain do method")
		}
	}

	if pets, ok := files["pets.go"]; ok {
		if !strings.Contains(pets, "type ListPetsOp struct {") {
			t.Error("pets.go should contain ListPetsOp")
		}
		if !strings.Contains(pets, "func (c *Client) ListPets(") {
			t.Error("pets.go should contain ListPets method")
		}
		if !strings.Contains(pets, "type CreatePetOp struct {") {
			t.Error("pets.go should contain CreatePetOp")
		}
	}

	if users, ok := files["users.go"]; ok {
		if !strings.Contains(users, "type GetUserOp struct {") {
			t.Error("users.go should contain GetUserOp")
		}
		if !strings.Contains(users, "func (c *Client) GetUser(") {
			t.Error("users.go should contain GetUser method")
		}
	}
}

func TestGenerateGoTagMerge(t *testing.T) {
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

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "testapi"})

	if _, ok := files["invoice_line_item.go"]; ok {
		t.Error("Invoice Line Item should be merged into invoice.go, not its own file")
	}

	invoice, ok := files["invoice.go"]
	if !ok {
		t.Fatal("missing invoice.go")
	}
	if !strings.Contains(invoice, "type ListInvoicesOp struct {") {
		t.Error("invoice.go should contain ListInvoicesOp")
	}
	if !strings.Contains(invoice, "type GetInvoiceLineItemOp struct {") {
		t.Error("invoice.go should contain GetInvoiceLineItemOp (merged from Invoice Line Item tag)")
	}
	if _, err := format.Source([]byte(invoice)); err != nil {
		t.Errorf("invoice.go is not valid Go: %v", err)
	}

	if _, ok := files["customer.go"]; !ok {
		t.Error("customer.go should still exist as a separate file")
	}
}

func TestGenerateGoNoTagsSingleFile(t *testing.T) {
	spec := &ir.Spec{
		Title: "Simple API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	files := mustGenerateGo(t, spec, GoOptions{Auth: "none", Package: "testapi"})

	if len(files) != 1 {
		t.Errorf("expected 1 file for no-tag spec, got %d", len(files))
	}
	if _, ok := files["client.go"]; !ok {
		t.Error("no-tag spec should produce client.go")
	}
}
