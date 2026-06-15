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

// Parse reads an OpenAPI spec (v2 or v3) from a file path or URL, deep-merges
// any supplemental fragments, and returns the IR.
func Parse(specPath string, mergePaths ...string) (*ir.Spec, error) {
	data, err := loadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("loading spec: %w", err)
	}
	if len(mergePaths) > 0 {
		doc, err := decodeSpec(data)
		if err != nil {
			return nil, err
		}
		for _, mergePath := range mergePaths {
			fragData, err := loadSpec(mergePath)
			if err != nil {
				return nil, fmt.Errorf("merge %s: %w", mergePath, err)
			}
			fragDoc, err := decodeSpec(fragData)
			if err != nil {
				return nil, fmt.Errorf("merge %s: %w", mergePath, err)
			}
			doc = deepMerge(doc, fragDoc)
		}
		data, err = encodeSpec(doc)
		if err != nil {
			return nil, err
		}
	}

	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing spec: %w", err)
	}

	if strings.HasPrefix(doc.GetVersion(), "2") {
		model, err := doc.BuildV2Model()
		if err != nil {
			return nil, fmt.Errorf("building v2 model: %w", err)
		}
		return buildIRFromV2(model), nil
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
			if !isObjectSchema(schema) {
				continue
			}
			m := buildModel(sanitizeName(name), schema)
			spec.Models = append(spec.Models, m)
		}
	}

	// Endpoints from paths. Two paths can share one operationId (e.g.
	// /custom-fields and /customFields); dedupe deterministically by keeping the
	// lexicographically smallest path so regeneration is stable regardless of
	// map order — the kebab-case form wins, as its '-' sorts before the
	// camelCase capital.
	opIndex := make(map[string]int) // operationId -> index into spec.Endpoints
	if model.Model.Paths != nil && model.Model.Paths.PathItems != nil {
		for path, pathItem := range model.Model.Paths.PathItems.FromOldest() {
			for _, ep := range buildEndpoints(path, pathItem) {
				spec.Endpoints = dedupeEndpoint(spec.Endpoints, opIndex, ep)
			}
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

// buildFormFields flattens a multipart/form-data object schema into ordered form
// fields: file (binary) parts first, then required values, then optional values.
// A container segment common to every dotted field key is stripped from the
// derived parameter names (e.g. Detail.Owner.Type -> Owner.Type).
func buildFormFields(proxy *base.SchemaProxy) []ir.FormField {
	schema := proxy.Schema()
	if schema == nil || schema.Properties == nil {
		return nil
	}

	requiredSet := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	var keys []string
	for key := range schema.Properties.FromOldest() {
		keys = append(keys, key)
	}
	names := stripCommonPrefix(keys)

	var files, required, optional []ir.FormField
	for key, fieldProxy := range schema.Properties.FromOldest() {
		fieldSchema := fieldProxy.Schema()
		field := ir.FormField{
			Key:      key,
			Name:     names[key],
			Type:     schemaToType(fieldProxy),
			Required: requiredSet[key],
			IsFile:   fieldSchema != nil && len(fieldSchema.Type) > 0 && fieldSchema.Type[0] == "string" && fieldSchema.Format == "binary",
		}
		switch {
		case field.IsFile:
			files = append(files, field)
		case field.Required:
			required = append(required, field)
		default:
			optional = append(optional, field)
		}
	}
	return append(append(files, required...), optional...)
}

// stripCommonPrefix maps each form-field key to a parameter base name with the
// leading dotted segments shared by all multi-segment keys removed. Keys without
// a dot (e.g. "File") are returned unchanged. At least one trailing segment is
// always kept.
func stripCommonPrefix(keys []string) map[string]string {
	var multi [][]string
	for _, k := range keys {
		if segs := strings.Split(k, "."); len(segs) > 1 {
			multi = append(multi, segs)
		}
	}

	common := 0
	for len(multi) > 0 {
		seg := ""
		ok := true
		for i, segs := range multi {
			if common >= len(segs)-1 { // keep at least one trailing segment
				ok = false
				break
			}
			if i == 0 {
				seg = segs[common]
			} else if segs[common] != seg {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		common++
	}

	out := make(map[string]string, len(keys))
	for _, k := range keys {
		segs := strings.Split(k, ".")
		if len(segs) > 1 && common > 0 && common < len(segs) {
			out[k] = strings.Join(segs[common:], ".")
		} else {
			out[k] = k
		}
	}
	return out
}

func schemaToType(proxy *base.SchemaProxy) ir.Type {
	schema := proxy.Schema()
	if schema == nil {
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimString}
	}

	// If this is a $ref to an object schema with properties, use TypeRef.
	// Otherwise fall through to resolve the actual type from the schema.
	ref := proxy.GetReference()
	if ref != "" && isObjectSchema(schema) {
		parts := strings.Split(ref, "/")
		refName := sanitizeName(parts[len(parts)-1])
		return ir.Type{Kind: ir.TypeRef, Ref: refName}
	}

	types := schema.Type
	if len(types) == 0 {
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimAny}
	}
	typeName := types[0]

	switch typeName {
	case "file":
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBytes}
	case "string":
		// format: binary is raw bytes (files). format: byte is a base64 string,
		// which the client does not decode, so it stays a string.
		if schema.Format == "binary" {
			return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimBytes}
		}
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
		if schema.AdditionalProperties != nil && schema.AdditionalProperties.IsA() {
			valType := schemaToType(schema.AdditionalProperties.A)
			return ir.Type{Kind: ir.TypeMap, Elem: &valType}
		}
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimAny}
	default:
		return ir.Type{Kind: ir.TypePrimitive, Prim: ir.PrimAny}
	}
}

func buildEndpoints(path string, pathItem *v3.PathItem) []ir.Endpoint {
	var endpoints []ir.Endpoint

	// Fixed method order so endpoint output is deterministic (a map would
	// iterate in random order, reordering methods between regenerations).
	ops := []struct {
		method string
		op     *v3.Operation
	}{
		{"GET", pathItem.Get},
		{"POST", pathItem.Post},
		{"PUT", pathItem.Put},
		{"DELETE", pathItem.Delete},
		{"PATCH", pathItem.Patch},
	}

	for _, mo := range ops {
		method, op := mo.method, mo.op
		if op == nil {
			continue
		}
		opID := op.OperationId
		if opID == "" {
			opID = deriveOperationID(method, path)
		}
		ep := ir.Endpoint{
			OperationID: opID,
			Summary:     op.Summary,
			Description: op.Description,
			Method:      method,
			Path:        path,
			Tags:        op.Tags,
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

		// Request body: prefer a concrete JSON media type (never the
		// application/*+json wildcard or text/json — servers bind concrete
		// types and 415 those); otherwise fall back to multipart/form-data,
		// modeled as flattened form fields.
		if op.RequestBody != nil && op.RequestBody.Content != nil {
			bestRank, bestType := -1, ""
			for mediaType, content := range op.RequestBody.Content.FromOldest() {
				if content.Schema == nil {
					continue
				}
				rank := jsonRequestTypeRank(mediaType)
				if rank < 0 {
					continue
				}
				// Higher rank wins; ties broken lexicographically so the
				// choice is independent of content-map order.
				if rank > bestRank || (rank == bestRank && mediaType < bestType) {
					bestRank, bestType = rank, mediaType
					t := schemaToType(content.Schema)
					ep.RequestBody = &t
					ep.RequestCType = mediaType
				}
			}
			if ep.RequestBody == nil {
				if content, ok := op.RequestBody.Content.Get("multipart/form-data"); ok && content.Schema != nil {
					ep.FormFields = buildFormFields(content.Schema)
				}
			}
		}

		// Response type (first 2xx response with JSON body; otherwise binary body)
		if op.Responses != nil && op.Responses.Codes != nil {
			for code, resp := range op.Responses.Codes.FromOldest() {
				if !strings.HasPrefix(code, "2") {
					continue
				}
				if resp.Content != nil {
					// JSON wins when an operation offers both JSON and binary bodies.
					for mediaType, content := range resp.Content.FromOldest() {
						if isJSONMediaType(mediaType) && content.Schema != nil {
							t := schemaToType(content.Schema)
							ep.ResponseType = &t
							break
						}
					}
					if ep.ResponseType == nil {
						for mediaType, content := range resp.Content.FromOldest() {
							if isJSONMediaType(mediaType) || content.Schema == nil {
								continue
							}
							if t := schemaToType(content.Schema); t.IsBytes() {
								ep.ResponseType = &t
								break
							}
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

// deriveOperationID generates an operationId from HTTP method and path
// (e.g. "GET", "/documents/{id}" → "getDocumentsById",
//
//	"POST", "/tenants/{id}/rotate-key" → "postTenantsIdRotateKey").
func deriveOperationID(method, path string) string {
	method = strings.ToLower(method)
	var parts []string
	parts = append(parts, method)
	for seg := range strings.SplitSeq(strings.Trim(path, "/"), "/") {
		if seg == "" {
			continue
		}
		seg = strings.TrimPrefix(seg, "{")
		seg = strings.TrimSuffix(seg, "}")
		for word := range strings.SplitSeq(seg, "-") {
			if word == "" {
				continue
			}
			parts = append(parts, strings.ToUpper(word[:1])+word[1:])
		}
	}
	return strings.Join(parts, "")
}

// consumesContentType picks the request media type for a Swagger 2.0 body
// parameter from the operation-level "consumes" list (which overrides global),
// preferring a concrete JSON type and defaulting to application/json.
func consumesContentType(opConsumes, globalConsumes []string) string {
	for _, list := range [][]string{opConsumes, globalConsumes} {
		if mt := bestJSONRequestType(list); mt != "" {
			return mt
		}
	}
	return "application/json"
}

// bestJSONRequestType returns the highest-ranked concrete JSON media type from
// the list (see jsonRequestTypeRank), or "" if none qualifies.
func bestJSONRequestType(types []string) string {
	bestRank, bestType := -1, ""
	for _, mt := range types {
		rank := jsonRequestTypeRank(mt)
		if rank < 0 {
			continue
		}
		if rank > bestRank || (rank == bestRank && mt < bestType) {
			bestRank, bestType = rank, mt
		}
	}
	return bestType
}

// jsonRequestTypeRank scores a media type for use as a request Content-Type.
// Higher wins. Concrete application/json is preferred over other concrete JSON
// subtypes (e.g. application/json-patch+json); the application/*+json wildcard
// and text/json are rejected (rank -1) because servers bind concrete media
// types and reject those. Non-JSON types also score -1.
func jsonRequestTypeRank(mt string) int {
	switch {
	case mt == "application/json":
		return 2
	case strings.ContainsRune(mt, '*'), mt == "text/json":
		return -1
	case strings.HasSuffix(mt, "+json"):
		return 1
	default:
		return -1
	}
}

// isJSONMediaType returns true for media types that carry JSON content. Used
// for response parsing, where text/json and +json subtypes are valid JSON.
func isJSONMediaType(mt string) bool {
	return mt == "application/json" ||
		mt == "text/json" ||
		strings.HasSuffix(mt, "+json")
}

// dedupeEndpoint adds ep to endpoints unless its operationId already appears.
// On collision it keeps the endpoint with the lexicographically smallest path
// (deterministic regardless of map order) and warns to stderr. opIndex maps
// operationId to the endpoint's index in the slice.
func dedupeEndpoint(endpoints []ir.Endpoint, opIndex map[string]int, ep ir.Endpoint) []ir.Endpoint {
	if i, ok := opIndex[ep.OperationID]; ok {
		prev := endpoints[i]
		kept, dropped := prev, ep
		if ep.Path < prev.Path {
			kept, dropped = ep, prev
			endpoints[i] = ep
		}
		fmt.Fprintf(os.Stderr, "warning: operationId %q maps to multiple paths; keeping %q, dropping %q\n",
			ep.OperationID, kept.Path, dropped.Path)
		return endpoints
	}
	opIndex[ep.OperationID] = len(endpoints)
	return append(endpoints, ep)
}

func isObjectSchema(schema *base.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		return true
	}
	if len(schema.Type) > 0 && schema.Type[0] == "object" {
		return true
	}
	return false
}

// sanitizeName produces a valid identifier from a schema name by stripping
// dotted package prefixes and removing characters that aren't letters, digits,
// or underscores.
func sanitizeName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' || r == '-' || r == ' ' || r == '/' || r == '$' || r == '+' {
			upper = true
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if upper {
				if r >= 'a' && r <= 'z' {
					b.WriteRune(r - 32)
				} else {
					b.WriteRune(r)
				}
				upper = false
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
