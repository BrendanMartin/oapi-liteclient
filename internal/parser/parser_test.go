package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
)

func TestParsePetstore(t *testing.T) {
	spec, err := Parse(testdataPath("petstore.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if spec.Title != "Petstore" {
		t.Errorf("Title = %q, want %q", spec.Title, "Petstore")
	}

	if spec.BaseURL != "https://petstore.example.com/v1" {
		t.Errorf("BaseURL = %q, want %q", spec.BaseURL, "https://petstore.example.com/v1")
	}

	// Auth
	if spec.Auth == nil {
		t.Fatal("Auth is nil")
	}
	if spec.Auth.Type != ir.AuthAPIKey {
		t.Errorf("Auth.Type = %v, want AuthAPIKey", spec.Auth.Type)
	}
	if spec.Auth.Name != "X-API-Key" {
		t.Errorf("Auth.Name = %q, want %q", spec.Auth.Name, "X-API-Key")
	}

	// Models
	if len(spec.Models) != 3 {
		t.Fatalf("len(Models) = %d, want 3", len(spec.Models))
	}

	pet := findModel(spec.Models, "Pet")
	if pet == nil {
		t.Fatal("Pet model not found")
	}
	if len(pet.Fields) != 5 {
		t.Errorf("Pet has %d fields, want 5", len(pet.Fields))
	}

	idField := findField(pet.Fields, "id")
	if idField == nil {
		t.Fatal("Pet.id not found")
	}
	if !idField.Required {
		t.Error("Pet.id should be required")
	}
	if idField.Type.Prim != ir.PrimInt {
		t.Errorf("Pet.id type = %v, want PrimInt", idField.Type.Prim)
	}

	tagField := findField(pet.Fields, "tag")
	if tagField == nil {
		t.Fatal("Pet.tag not found")
	}
	if tagField.Required {
		t.Error("Pet.tag should not be required")
	}

	// Endpoints
	if len(spec.Endpoints) != 4 {
		t.Fatalf("len(Endpoints) = %d, want 4", len(spec.Endpoints))
	}

	listPets := findEndpoint(spec.Endpoints, "listPets")
	if listPets == nil {
		t.Fatal("listPets endpoint not found")
	}
	if listPets.Method != "GET" {
		t.Errorf("listPets.Method = %q, want GET", listPets.Method)
	}
	if len(listPets.Params) != 2 {
		t.Errorf("listPets has %d params, want 2", len(listPets.Params))
	}
	if listPets.ResponseType == nil {
		t.Fatal("listPets.ResponseType is nil")
	}
	if listPets.ResponseType.Kind != ir.TypeArray {
		t.Errorf("listPets response kind = %v, want TypeArray", listPets.ResponseType.Kind)
	}

	createPet := findEndpoint(spec.Endpoints, "createPet")
	if createPet == nil {
		t.Fatal("createPet endpoint not found")
	}
	if createPet.RequestBody == nil {
		t.Fatal("createPet.RequestBody is nil")
	}
	if createPet.RequestBody.Ref != "PetCreate" {
		t.Errorf("createPet.RequestBody.Ref = %q, want PetCreate", createPet.RequestBody.Ref)
	}

	deletePet := findEndpoint(spec.Endpoints, "deletePet")
	if deletePet == nil {
		t.Fatal("deletePet endpoint not found")
	}
	if deletePet.ResponseType != nil {
		t.Error("deletePet.ResponseType should be nil (204)")
	}
}

func TestParseSwaggerV2(t *testing.T) {
	spec, err := Parse(testdataPath("petstore-v2.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if spec.Title != "Petstore" {
		t.Errorf("Title = %q, want %q", spec.Title, "Petstore")
	}

	if spec.BaseURL != "https://petstore.example.com/v1" {
		t.Errorf("BaseURL = %q, want %q", spec.BaseURL, "https://petstore.example.com/v1")
	}

	if spec.Auth == nil {
		t.Fatal("Auth is nil")
	}
	if spec.Auth.Type != ir.AuthAPIKey {
		t.Errorf("Auth.Type = %v, want AuthAPIKey", spec.Auth.Type)
	}
	if spec.Auth.Name != "X-API-Key" {
		t.Errorf("Auth.Name = %q, want %q", spec.Auth.Name, "X-API-Key")
	}

	if len(spec.Models) != 3 {
		t.Fatalf("len(Models) = %d, want 3", len(spec.Models))
	}

	pet := findModel(spec.Models, "Pet")
	if pet == nil {
		t.Fatal("Pet model not found")
	}
	if len(pet.Fields) != 5 {
		t.Errorf("Pet has %d fields, want 5", len(pet.Fields))
	}

	idField := findField(pet.Fields, "id")
	if idField == nil {
		t.Fatal("Pet.id not found")
	}
	if !idField.Required {
		t.Error("Pet.id should be required")
	}
	if idField.Type.Prim != ir.PrimInt {
		t.Errorf("Pet.id type = %v, want PrimInt", idField.Type.Prim)
	}

	tagField := findField(pet.Fields, "tag")
	if tagField == nil {
		t.Fatal("Pet.tag not found")
	}
	if tagField.Required {
		t.Error("Pet.tag should not be required")
	}

	if len(spec.Endpoints) != 4 {
		t.Fatalf("len(Endpoints) = %d, want 4", len(spec.Endpoints))
	}

	listPets := findEndpoint(spec.Endpoints, "listPets")
	if listPets == nil {
		t.Fatal("listPets endpoint not found")
	}
	if listPets.Method != "GET" {
		t.Errorf("listPets.Method = %q, want GET", listPets.Method)
	}
	if len(listPets.Params) != 2 {
		t.Errorf("listPets has %d params, want 2", len(listPets.Params))
	}
	if listPets.ResponseType == nil {
		t.Fatal("listPets.ResponseType is nil")
	}
	if listPets.ResponseType.Kind != ir.TypeArray {
		t.Errorf("listPets response kind = %v, want TypeArray", listPets.ResponseType.Kind)
	}

	createPet := findEndpoint(spec.Endpoints, "createPet")
	if createPet == nil {
		t.Fatal("createPet endpoint not found")
	}
	if createPet.RequestBody == nil {
		t.Fatal("createPet.RequestBody is nil")
	}
	if createPet.RequestBody.Ref != "PetCreate" {
		t.Errorf("createPet.RequestBody.Ref = %q, want PetCreate", createPet.RequestBody.Ref)
	}

	deletePet := findEndpoint(spec.Endpoints, "deletePet")
	if deletePet == nil {
		t.Fatal("deletePet endpoint not found")
	}
	if deletePet.ResponseType != nil {
		t.Error("deletePet.ResponseType should be nil (204)")
	}
}

func TestParseComplex(t *testing.T) {
	spec, err := Parse(testdataPath("complex.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Bearer auth
	if spec.Auth == nil {
		t.Fatal("Auth is nil")
	}
	if spec.Auth.Type != ir.AuthBearer {
		t.Errorf("Auth.Type = %v, want AuthBearer", spec.Auth.Type)
	}

	// Nested ref: User.address -> Address
	user := findModel(spec.Models, "User")
	if user == nil {
		t.Fatal("User model not found")
	}
	addrField := findField(user.Fields, "address")
	if addrField == nil {
		t.Fatal("User.address not found")
	}
	if addrField.Type.Kind != ir.TypeRef || addrField.Type.Ref != "Address" {
		t.Errorf("User.address type = %+v, want TypeRef to Address", addrField.Type)
	}

	// Array of primitives: User.tags -> []string
	tagsField := findField(user.Fields, "tags")
	if tagsField == nil {
		t.Fatal("User.tags not found")
	}
	if tagsField.Type.Kind != ir.TypeArray {
		t.Errorf("User.tags kind = %v, want TypeArray", tagsField.Type.Kind)
	}
	if tagsField.Type.Elem.Prim != ir.PrimString {
		t.Errorf("User.tags elem = %v, want PrimString", tagsField.Type.Elem.Prim)
	}

	// Array of refs: Order.items -> []OrderItem
	order := findModel(spec.Models, "Order")
	if order == nil {
		t.Fatal("Order model not found")
	}
	itemsField := findField(order.Fields, "items")
	if itemsField == nil {
		t.Fatal("Order.items not found")
	}
	if itemsField.Type.Kind != ir.TypeArray {
		t.Fatalf("Order.items kind = %v, want TypeArray", itemsField.Type.Kind)
	}
	if itemsField.Type.Elem.Ref != "OrderItem" {
		t.Errorf("Order.items elem ref = %q, want OrderItem", itemsField.Type.Elem.Ref)
	}

	// Multiple path params
	getOrderItem := findEndpoint(spec.Endpoints, "getOrderItem")
	if getOrderItem == nil {
		t.Fatal("getOrderItem endpoint not found")
	}
	pathParams := 0
	for _, p := range getOrderItem.Params {
		if p.In == "path" {
			pathParams++
		}
	}
	if pathParams != 2 {
		t.Errorf("getOrderItem has %d path params, want 2", pathParams)
	}

	// Required query param
	listUsers := findEndpoint(spec.Endpoints, "listUsers")
	if listUsers == nil {
		t.Fatal("listUsers endpoint not found")
	}
	isActiveParam := findParam(listUsers.Params, "is_active")
	if isActiveParam == nil {
		t.Fatal("listUsers.is_active param not found")
	}
	if !isActiveParam.Required {
		t.Error("listUsers.is_active should be required")
	}

	// All-optional model
	filters := findModel(spec.Models, "SearchFilters")
	if filters == nil {
		t.Fatal("SearchFilters model not found")
	}
	for _, f := range filters.Fields {
		if f.Required {
			t.Errorf("SearchFilters.%s should not be required", f.Name)
		}
	}
}

func testdataPath(name string) string {
	// Walk up from internal/parser to project root
	wd, _ := os.Getwd()
	return filepath.Join(wd, "..", "..", "testdata", name)
}

func findModel(models []ir.Model, name string) *ir.Model {
	for i := range models {
		if models[i].Name == name {
			return &models[i]
		}
	}
	return nil
}

func findField(fields []ir.Field, name string) *ir.Field {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}

func findEndpoint(endpoints []ir.Endpoint, opID string) *ir.Endpoint {
	for i := range endpoints {
		if endpoints[i].OperationID == opID {
			return &endpoints[i]
		}
	}
	return nil
}

func findParam(params []ir.Param, name string) *ir.Param {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}
