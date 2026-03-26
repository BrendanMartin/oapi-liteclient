package parser

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Parse reads an OpenAPI spec from a file path or URL and returns the IR.
func Parse(specPath string) (*ir.Spec, error) {
	data, err := loadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("loading spec: %w", err)
	}

	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}

	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("building model: %w", err)
	}

	return buildIR(model), nil
}

func loadSpec(specPath string) ([]byte, error) {
	if strings.HasPrefix(specPath, "http://") || strings.HasPrefix(specPath, "https://") {
		resp, err := http.Get(specPath)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d fetching spec", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(specPath)
}

func buildIR(model *libopenapi.DocumentModel[v3.Document]) *ir.Spec {
	spec := &ir.Spec{
		Title: model.Model.Info.Title,
	}

	// Base URL from first server
	if len(model.Model.Servers) > 0 {
		spec.BaseURL = model.Model.Servers[0].URL
	}

	// Auth from security schemes
	spec.Auth = extractAuth(model)

	// Models from components/schemas
	if model.Model.Components != nil && model.Model.Components.Schemas != nil {
		for name, schemaProxy := range model.Model.Components.Schemas.FromOldest() {
			schema := schemaProxy.Schema()
			if schema == nil {
				continue
			}
			m := buildModel(name, schema)
			spec.Models = append(spec.Models, m)
		}
	}

	// Endpoints from paths
	if model.Model.Paths != nil && model.Model.Paths.PathItems != nil {
		for path, pathItem := range model.Model.Paths.PathItems.FromOldest() {
			endpoints := buildEndpoints(path, pathItem)
			spec.Endpoints = append(spec.Endpoints, endpoints...)
		}
	}

	return spec
}

func extractAuth(model *libopenapi.DocumentModel[v3.Document]) *ir.Auth {
	if model.Model.Components == nil || model.Model.Components.SecuritySchemes == nil {
		return nil
	}

	for _, scheme := range model.Model.Components.SecuritySchemes.FromOldest() {
		switch scheme.Type {
		case "apiKey":
			return &ir.Auth{
				Type: ir.AuthAPIKey,
				Name: scheme.Name,
				In:   scheme.In,
			}
		case "http":
			if strings.ToLower(scheme.Scheme) == "bearer" {
				return &ir.Auth{
					Type: ir.AuthBearer,
					Name: "Authorization",
					In:   "header",
				}
			}
		}
	}

	return nil
}

func buildModel(name string, schema *base.Schema) ir.Model {
	m := ir.Model{Name: name}

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	if schema.Properties != nil {
		for fieldName, fieldProxy := range schema.Properties.FromOldest() {
			fieldSchema := fieldProxy.Schema()
			if fieldSchema == nil {
				continue
			}
			f := ir.Field{
				Name:     fieldName,
				Type:     schemaToType(fieldProxy),
				Required: requiredSet[fieldName],
				Default:  extractDefault(fieldSchema),
			}
			m.Fields = append(m.Fields, f)
		}
	}

	return m
}

func schemaToType(proxy *base.SchemaProxy) ir.Type {
	schema := proxy.Schema()
	if schema == nil {
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	}

	// Check if this is a $ref to a named schema
	ref := proxy.GetReference()
	if ref != "" {
		parts := strings.Split(ref, "/")
		refName := parts[len(parts)-1]
		return ir.Type{Kind: ir.TypeRef, Ref: refName}
	}

	// Determine type from schema
	types := schema.Type
	if len(types) == 0 {
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	}
	typeName := types[0]

	switch typeName {
	case "string":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	case "integer":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}
	case "number":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}
	case "boolean":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}
	case "array":
		elemType := ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
		if schema.Items != nil && schema.Items.IsA() {
			elemType = schemaToType(schema.Items.A)
		}
		return ir.Type{Kind: ir.TypeArray, Elem: &elemType}
	case "object":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	default:
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	}
}

func buildEndpoints(path string, pathItem *v3.PathItem) []ir.Endpoint {
	var endpoints []ir.Endpoint

	ops := map[string]*v3.Operation{
		"GET":    pathItem.Get,
		"POST":   pathItem.Post,
		"PUT":    pathItem.Put,
		"DELETE": pathItem.Delete,
		"PATCH":  pathItem.Patch,
	}

	for method, op := range ops {
		if op == nil {
			continue
		}
		ep := ir.Endpoint{
			OperationID: op.OperationId,
			Summary:     op.Summary,
			Description: op.Description,
			Method:      method,
			Path:        path,
		}

		// Parameters
		for _, param := range op.Parameters {
			if param.In != "path" && param.In != "query" {
				continue
			}
			p := ir.Param{
				Name:     param.Name,
				In:       param.In,
				Required: boolVal(param.Required),
			}
			if param.Schema != nil {
				p.Type = schemaToType(param.Schema)
			}
			ep.Params = append(ep.Params, p)
		}

		// Request body
		if op.RequestBody != nil && op.RequestBody.Content != nil {
			for mediaType, content := range op.RequestBody.Content.FromOldest() {
				if mediaType == "application/json" && content.Schema != nil {
					t := schemaToType(content.Schema)
					ep.RequestBody = &t
					break
				}
			}
		}

		// Response type (first 2xx response with JSON body)
		if op.Responses != nil && op.Responses.Codes != nil {
			for code, resp := range op.Responses.Codes.FromOldest() {
				if !strings.HasPrefix(code, "2") {
					continue
				}
				if resp.Content != nil {
					for mediaType, content := range resp.Content.FromOldest() {
						if mediaType == "application/json" && content.Schema != nil {
							t := schemaToType(content.Schema)
							ep.ResponseType = &t
							break
						}
					}
				}
				if ep.ResponseType != nil {
					break
				}
			}
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints
}

func extractDefault(schema *base.Schema) *string {
	if schema.Default == nil {
		return nil
	}
	val := schema.Default.Value
	if val == "" {
		return nil
	}
	return &val
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
