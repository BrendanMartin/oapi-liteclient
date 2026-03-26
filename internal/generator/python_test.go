package generator

import (
	"strings"
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

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
		{"HTMLParser", "htmlparser"}, // consecutive caps stay together
		{"getURL", "get_url"},       // consecutive caps stay together
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
	}
	for _, tt := range tests {
		got := pyType(tt.in)
		if got != tt.want {
			t.Errorf("pyType(%+v) = %q, want %q", tt.in, got, tt.want)
		}
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

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

	// Check key parts are present
	checks := []string{
		"class Item:",
		"id: int",
		"name: str",
		"price: Optional[float] = None",
		"def get_item(",
		"item_id: int",
		`f"/items/{item_id}"`,
		"return Item(**resp.json())",
		"class APIError(Exception):",
		"class Client:",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestGeneratePythonNoAuth(t *testing.T) {
	spec := &ir.Spec{
		Title: "No Auth API",
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

	if strings.Contains(output, "api_key") || strings.Contains(output, "bearer_token") {
		t.Error("output should not contain auth params when spec has no auth")
	}
}

func TestGeneratePythonBearerAuth(t *testing.T) {
	spec := &ir.Spec{
		Title: "Bearer API",
		Auth:  &ir.Auth{Type: ir.AuthBearer, Name: "Authorization", In: "header"},
		Endpoints: []ir.Endpoint{
			{OperationID: "ping", Method: "GET", Path: "/ping"},
		},
	}

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

	if !strings.Contains(output, "bearer_token") {
		t.Error("output should contain bearer_token param")
	}
	if !strings.Contains(output, `f"Bearer {bearer_token}"`) {
		t.Error("output should set Authorization header with bearer token")
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

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

	if !strings.Contains(output, "[Pet(**item) for item in resp.json()]") {
		t.Error("array of refs should deserialize with list comprehension")
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

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

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

	output, err := GeneratePython(spec)
	if err != nil {
		t.Fatalf("GeneratePython: %v", err)
	}

	// Required param should come before optional
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

	// Required param should not have None check
	if strings.Contains(output, "if q is not None") {
		t.Error("required param should not have 'is not None' check")
	}
}
