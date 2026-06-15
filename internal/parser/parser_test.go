package parser

import (
	"os"
	"path/filepath"
	"strings"
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
	if createPet.RequestCType != "application/json" {
		t.Errorf("createPet.RequestCType = %q, want application/json", createPet.RequestCType)
	}

	deletePet := findEndpoint(spec.Endpoints, "deletePet")
	if deletePet == nil {
		t.Fatal("deletePet endpoint not found")
	}
	if deletePet.ResponseType != nil {
		t.Error("deletePet.ResponseType should be nil (204)")
	}
}

func TestParseSwaggo(t *testing.T) {
	spec, err := Parse(testdataPath("swaggo-example.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	task := findModel(spec.Models, "TaskResponse")
	if task == nil {
		t.Fatal("TaskResponse model not found (dotted name not sanitized?)")
	}

	list := findModel(spec.Models, "ListResponse")
	if list == nil {
		t.Fatal("ListResponse model not found")
	}
	itemsField := findField(list.Fields, "items")
	if itemsField == nil {
		t.Fatal("ListResponse.items not found")
	}
	if itemsField.Type.Kind != ir.TypeArray {
		t.Fatalf("ListResponse.items kind = %v, want TypeArray", itemsField.Type.Kind)
	}
	if itemsField.Type.Elem.Ref != "TaskResponse" {
		t.Errorf("ListResponse.items elem ref = %q, want TaskResponse", itemsField.Type.Elem.Ref)
	}

	createTask := findEndpoint(spec.Endpoints, "createTask")
	if createTask == nil {
		t.Fatal("createTask endpoint not found")
	}
	if createTask.RequestBody == nil {
		t.Fatal("createTask.RequestBody is nil")
	}
	if createTask.RequestBody.Ref != "CreateTaskRequest" {
		t.Errorf("createTask.RequestBody.Ref = %q, want CreateTaskRequest", createTask.RequestBody.Ref)
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
	if createPet.RequestCType != "application/json" {
		t.Errorf("createPet.RequestCType = %q, want application/json", createPet.RequestCType)
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

	// Scalar enum schema should not produce a model
	if findModel(spec.Models, "OrderStatus") != nil {
		t.Error("OrderStatus should not be emitted as a model (it's a scalar enum)")
	}

	// Field referencing scalar enum should resolve to string, not TypeRef
	statusField := findField(order.Fields, "status")
	if statusField == nil {
		t.Fatal("Order.status not found")
	}
	if statusField.Type.Kind != ir.TypePrimitive || statusField.Type.Prim != ir.PrimString {
		t.Errorf("Order.status type = %+v, want PrimString (scalar enum ref)", statusField.Type)
	}

	// Array-type schema should not produce a model
	if findModel(spec.Models, "PatchDocument") != nil {
		t.Error("PatchDocument should not be emitted as a model (it's an array type)")
	}

	// PatchOperation should still be emitted (it's an object)
	patchOp := findModel(spec.Models, "PatchOperation")
	if patchOp == nil {
		t.Fatal("PatchOperation model not found")
	}

	// Field with no type should resolve to PrimAny
	valueField := findField(patchOp.Fields, "value")
	if valueField == nil {
		t.Fatal("PatchOperation.value not found")
	}
	if valueField.Type.Kind != ir.TypePrimitive || valueField.Type.Prim != ir.PrimAny {
		t.Errorf("PatchOperation.value type = %+v, want PrimAny", valueField.Type)
	}

	// Request body referencing array schema should resolve to TypeArray
	patchUser := findEndpoint(spec.Endpoints, "patchUser")
	if patchUser == nil {
		t.Fatal("patchUser endpoint not found")
	}
	if patchUser.RequestBody == nil {
		t.Fatal("patchUser.RequestBody is nil")
	}
	if patchUser.RequestBody.Kind != ir.TypeArray {
		t.Errorf("patchUser.RequestBody kind = %v, want TypeArray", patchUser.RequestBody.Kind)
	}
	if patchUser.RequestBody.Elem == nil || patchUser.RequestBody.Elem.Ref != "PatchOperation" {
		t.Errorf("patchUser.RequestBody.Elem = %+v, want TypeRef to PatchOperation", patchUser.RequestBody.Elem)
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

func TestParseMultipart(t *testing.T) {
	spec, err := Parse(testdataPath("multipart.yaml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	ep := findEndpoint(spec.Endpoints, "createAttachment")
	if ep == nil {
		t.Fatal("createAttachment endpoint not found")
	}
	if ep.RequestBody != nil {
		t.Error("multipart endpoint should not set RequestBody")
	}

	// Expect: file part first, then required values, then optional values; the
	// shared "Detail." container prefix is stripped from parameter names.
	want := []ir.FormField{
		{Key: "File", Name: "File", Required: true, IsFile: true},
		{Key: "Detail.Owner.Type", Name: "Owner.Type", Required: true},
		{Key: "Detail.Owner.Id", Name: "Owner.Id", Required: true},
		{Key: "Detail.Description", Name: "Description"},
		{Key: "Detail.IsNoteAttachment", Name: "IsNoteAttachment"},
	}
	if len(ep.FormFields) != len(want) {
		t.Fatalf("FormFields len = %d, want %d: %+v", len(ep.FormFields), len(want), ep.FormFields)
	}
	for i, w := range want {
		got := ep.FormFields[i]
		if got.Key != w.Key || got.Name != w.Name || got.Required != w.Required || got.IsFile != w.IsFile {
			t.Errorf("FormFields[%d] = {Key:%q Name:%q Required:%v IsFile:%v}, want {Key:%q Name:%q Required:%v IsFile:%v}",
				i, got.Key, got.Name, got.Required, got.IsFile, w.Key, w.Name, w.Required, w.IsFile)
		}
	}
	if pk := ep.FormFields[4].Type.Prim; pk != ir.PrimBool {
		t.Errorf("IsNoteAttachment prim = %v, want PrimBool", pk)
	}
}

func TestDeepMerge(t *testing.T) {
	base := map[string]any{
		"openapi": "3.0.0",
		"paths": map[string]any{
			"/pets": map[string]any{
				"get": map[string]any{
					"summary":    "List pets",
					"parameters": []any{"base-param"},
				},
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Pet": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
	fragment := map[string]any{
		"paths": map[string]any{
			"/pets": map[string]any{
				"get": map[string]any{
					"summary":    "Fragment list pets",
					"parameters": []any{"fragment-param"},
				},
			},
			"/quotes/{quoteId}/pdf": map[string]any{
				"get": map[string]any{"operationId": "downloadQuotePdf"},
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"Pet": map[string]any{
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
				"Quote": map[string]any{"type": "object"},
			},
		},
	}

	got := deepMerge(base, fragment)

	paths := got["paths"].(map[string]any)
	petGet := paths["/pets"].(map[string]any)["get"].(map[string]any)
	if petGet["summary"] != "Fragment list pets" {
		t.Fatalf("summary = %v, want fragment value", petGet["summary"])
	}
	params := petGet["parameters"].([]any)
	if len(params) != 1 || params[0] != "fragment-param" {
		t.Fatalf("parameters = %#v, want fragment array replacement", params)
	}
	if _, ok := paths["/quotes/{quoteId}/pdf"]; !ok {
		t.Fatal("new fragment path was not added")
	}
	schemas := got["components"].(map[string]any)["schemas"].(map[string]any)
	petProps := schemas["Pet"].(map[string]any)["properties"].(map[string]any)
	if _, ok := petProps["id"]; !ok {
		t.Fatal("base nested property id was not preserved")
	}
	if _, ok := petProps["name"]; !ok {
		t.Fatal("fragment nested property name was not added")
	}
	if _, ok := schemas["Quote"]; !ok {
		t.Fatal("fragment schema Quote was not added")
	}
}

func TestDeepMergeTypeMismatchReplaces(t *testing.T) {
	base := map[string]any{"x": map[string]any{"nested": true}}
	fragment := map[string]any{"x": "replacement"}
	got := deepMerge(base, fragment)
	if got["x"] != "replacement" {
		t.Fatalf("x = %#v, want replacement", got["x"])
	}
}

func TestDecodeEncodeSpecRoundTrip(t *testing.T) {
	doc, err := decodeSpec([]byte("openapi: 3.0.0\npaths: {}\n"))
	if err != nil {
		t.Fatalf("decodeSpec: %v", err)
	}
	if doc["openapi"] != "3.0.0" {
		t.Fatalf("openapi = %v, want 3.0.0", doc["openapi"])
	}
	encoded, err := encodeSpec(doc)
	if err != nil {
		t.Fatalf("encodeSpec: %v", err)
	}
	decoded, err := decodeSpec(encoded)
	if err != nil {
		t.Fatalf("decodeSpec(encoded): %v", err)
	}
	if decoded["openapi"] != "3.0.0" {
		t.Fatalf("round-trip openapi = %v, want 3.0.0", decoded["openapi"])
	}
}

func TestParseWithMergeAddsEndpointAndSchema(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	fragmentPath := filepath.Join(dir, "fragment.yaml")

	base := `openapi: 3.0.0
info:
  title: Merge API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /quotes/{quoteId}:
    get:
      operationId: getQuote
      tags: [Quote]
      parameters:
        - name: quoteId
          in: path
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Quote"
components:
  schemas:
    Quote:
      type: object
      required: [id]
      properties:
        id:
          type: integer
`
	fragment := `paths:
  /quotes/{quoteId}/pdf:
    get:
      operationId: downloadQuotePdf
      tags: [Quote]
      parameters:
        - name: quoteId
          in: path
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: PDF
          content:
            application/pdf:
              schema:
                type: string
                format: binary
components:
  schemas:
    Quote:
      properties:
        status:
          type: string
`
	if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, []byte(fragment), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(basePath, fragmentPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if findEndpoint(spec.Endpoints, "getQuote") == nil {
		t.Fatal("base endpoint getQuote missing after merge")
	}
	pdf := findEndpoint(spec.Endpoints, "downloadQuotePdf")
	if pdf == nil {
		t.Fatal("fragment endpoint downloadQuotePdf missing")
	}
	if len(pdf.Params) != 1 || pdf.Params[0].Name != "quoteId" || pdf.Params[0].Type.Prim != ir.PrimInt {
		t.Fatalf("pdf params = %+v, want quoteId int", pdf.Params)
	}
	quote := findModel(spec.Models, "Quote")
	if quote == nil {
		t.Fatal("Quote model missing")
	}
	if findField(quote.Fields, "id") == nil {
		t.Fatal("base Quote.id field missing after merge")
	}
	if findField(quote.Fields, "status") == nil {
		t.Fatal("fragment Quote.status field missing")
	}
}

func TestParseMalformedFragmentIncludesPath(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	fragmentPath := filepath.Join(dir, "bad.yaml")
	base := `openapi: 3.0.0
info: {title: Bad Fragment API, version: "1.0"}
paths: {}
`
	if err := os.WriteFile(basePath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fragmentPath, []byte("paths: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Parse(basePath, fragmentPath)
	if err == nil {
		t.Fatal("Parse succeeded, want malformed fragment error")
	}
	if !strings.Contains(err.Error(), "merge "+fragmentPath+":") {
		t.Fatalf("error = %q, want merge path prefix", err.Error())
	}
}

func TestParseBinaryResponseV3(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "binary.yaml")
	specYAML := `openapi: 3.0.0
info: {title: Binary API, version: "1.0"}
paths:
  /quotes/{quoteId}/pdf:
    get:
      operationId: downloadQuotePdf
      parameters:
        - name: quoteId
          in: path
          required: true
          schema: {type: integer}
      responses:
        "200":
          description: PDF
          content:
            application/pdf:
              schema:
                type: string
                format: binary
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ep := findEndpoint(spec.Endpoints, "downloadQuotePdf")
	if ep == nil {
		t.Fatal("downloadQuotePdf endpoint missing")
	}
	if ep.ResponseType == nil || !ep.ResponseType.IsBytes() {
		t.Fatalf("ResponseType = %+v, want PrimBytes", ep.ResponseType)
	}
}

func TestParseJSONResponseWinsOverBinaryV3(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "json-wins.yaml")
	specYAML := `openapi: 3.0.0
info: {title: JSON Wins API, version: "1.0"}
paths:
  /quotes/{quoteId}:
    get:
      operationId: getQuote
      responses:
        "200":
          description: JSON or PDF
          content:
            application/pdf:
              schema: {type: string, format: binary}
            application/json:
              schema:
                type: object
                properties:
                  id: {type: integer}
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ep := findEndpoint(spec.Endpoints, "getQuote")
	if ep == nil {
		t.Fatal("getQuote endpoint missing")
	}
	if ep.ResponseType == nil || ep.ResponseType.IsBytes() {
		t.Fatalf("ResponseType = %+v, want JSON-derived non-bytes type", ep.ResponseType)
	}
}

func TestParseTextResponseIsNotBytesV3(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "text.yaml")
	specYAML := `openapi: 3.0.0
info: {title: Text API, version: "1.0"}
paths:
  /health:
    get:
      operationId: health
      responses:
        "200":
          description: plain text
          content:
            text/plain:
              schema: {type: string}
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ep := findEndpoint(spec.Endpoints, "health")
	if ep == nil {
		t.Fatal("health endpoint missing")
	}
	if ep.ResponseType != nil && ep.ResponseType.IsBytes() {
		t.Fatalf("ResponseType = %+v, text/plain must not be bytes", ep.ResponseType)
	}
}

func TestParseBinaryResponseV2(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "binary-v2.yaml")
	specYAML := `swagger: "2.0"
info: {title: Binary V2 API, version: "1.0"}
host: api.example.com
schemes: [https]
paths:
  /quotes/{quoteId}/pdf:
    get:
      operationId: downloadQuotePdf
      produces: [application/pdf]
      parameters:
        - name: quoteId
          in: path
          required: true
          type: integer
      responses:
        "200":
          description: PDF
          schema:
            type: file
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ep := findEndpoint(spec.Endpoints, "downloadQuotePdf")
	if ep == nil {
		t.Fatal("downloadQuotePdf endpoint missing")
	}
	if ep.ResponseType == nil || !ep.ResponseType.IsBytes() {
		t.Fatalf("ResponseType = %+v, want PrimBytes", ep.ResponseType)
	}
}

func TestParseTextResponseIsNotBytesV2(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "text-v2.yaml")
	specYAML := `swagger: "2.0"
info: {title: Text V2 API, version: "1.0"}
host: api.example.com
schemes: [https]
paths:
  /health:
    get:
      operationId: health
      produces: [text/plain]
      responses:
        "200":
          description: plain text
          schema:
            type: string
`
	if err := os.WriteFile(specPath, []byte(specYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := Parse(specPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ep := findEndpoint(spec.Endpoints, "health")
	if ep == nil {
		t.Fatal("health endpoint missing")
	}
	if ep.ResponseType != nil {
		t.Fatalf("ResponseType = %+v, text/plain must not be decoded", ep.ResponseType)
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
