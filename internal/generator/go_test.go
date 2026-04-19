package generator

import (
	"go/format"
	"strings"
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

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

	output, err := GenerateGo(spec, GoOptions{Auth: "none", Package: "petstore"})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

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

	output, err := GenerateGo(spec, GoOptions{Auth: "none", Package: "testapi"})
	if err != nil {
		t.Fatalf("GenerateGo: %v", err)
	}

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
