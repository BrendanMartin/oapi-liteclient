package generator

import (
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
