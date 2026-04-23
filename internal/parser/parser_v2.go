package parser

import (
	"strings"

	"github.com/brendanmartin/oapi-liteclient/internal/ir"
	"github.com/pb33f/libopenapi"
	v2 "github.com/pb33f/libopenapi/datamodel/high/v2"
)

func buildIRFromV2(model *libopenapi.DocumentModel[v2.Swagger]) *ir.Spec {
	spec := &ir.Spec{
		Title: model.Model.Info.Title,
	}

	if model.Model.Host != "" {
		scheme := "https"
		if len(model.Model.Schemes) > 0 {
			scheme = model.Model.Schemes[0]
		}
		spec.BaseURL = scheme + "://" + model.Model.Host + model.Model.BasePath
	}

	spec.Auth = extractAuthV2(model)

	if model.Model.Definitions != nil && model.Model.Definitions.Definitions != nil {
		for name, schemaProxy := range model.Model.Definitions.Definitions.FromOldest() {
			schema := schemaProxy.Schema()
			if schema == nil {
				continue
			}
			m := buildModel(name, schema)
			spec.Models = append(spec.Models, m)
		}
	}

	if model.Model.Paths != nil && model.Model.Paths.PathItems != nil {
		for path, pathItem := range model.Model.Paths.PathItems.FromOldest() {
			endpoints := buildEndpointsV2(path, pathItem)
			spec.Endpoints = append(spec.Endpoints, endpoints...)
		}
	}

	return spec
}

func extractAuthV2(model *libopenapi.DocumentModel[v2.Swagger]) *ir.Auth {
	if model.Model.SecurityDefinitions == nil || model.Model.SecurityDefinitions.Definitions == nil {
		return nil
	}

	for _, scheme := range model.Model.SecurityDefinitions.Definitions.FromOldest() {
		switch scheme.Type {
		case "apiKey":
			return &ir.Auth{
				Type: ir.AuthAPIKey,
				Name: scheme.Name,
				In:   scheme.In,
			}
		case "basic":
			return &ir.Auth{
				Type: ir.AuthBearer,
				Name: "Authorization",
				In:   "header",
			}
		}
	}

	return nil
}

func buildEndpointsV2(path string, pathItem *v2.PathItem) []ir.Endpoint {
	var endpoints []ir.Endpoint

	ops := map[string]*v2.Operation{
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

		for _, param := range op.Parameters {
			if param.In == "body" {
				if param.Schema != nil {
					t := schemaToType(param.Schema)
					ep.RequestBody = &t
				}
				continue
			}
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
			} else {
				p.Type = primitiveType(param.Type)
			}
			ep.Params = append(ep.Params, p)
		}

		if op.Responses != nil && op.Responses.Codes != nil {
			for code, resp := range op.Responses.Codes.FromOldest() {
				if !strings.HasPrefix(code, "2") {
					continue
				}
				if resp.Schema != nil {
					t := schemaToType(resp.Schema)
					ep.ResponseType = &t
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

func primitiveType(typeName string) ir.Type {
	switch typeName {
	case "integer":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimInt}
	case "number":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimFloat}
	case "boolean":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBool}
	default:
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	}
}
