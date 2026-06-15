package ir

// Spec is the top-level intermediate representation of an OpenAPI spec.
type Spec struct {
	Title     string
	BaseURL   string
	Auth      *Auth
	Models    []Model
	Endpoints []Endpoint
}

// Auth describes the API's authentication scheme.
type Auth struct {
	Type AuthType
	Name string // header or query param name (e.g. "X-API-Key", "Authorization")
	In   string // "header" or "query"
}

type AuthType int

const (
	AuthAPIKey AuthType = iota
	AuthBearer
)

// Model is a named schema (typically from components/schemas).
type Model struct {
	Name   string
	Fields []Field
}

// Field is a single property on a model.
type Field struct {
	Name     string
	Alias    string // original spec name, set when it differs from Name (e.g. reserved words)
	Type     Type
	Required bool
	Default  *string // nil means no default; pointer to the string representation
}

// Type represents a data type in the IR.
type Type struct {
	Kind TypeKind
	Prim PrimKind // set when Kind == TypePrimitive
	Elem *Type    // set when Kind == TypeArray (element) or TypeMap (value)
	Ref  string   // model name, set when Kind == TypeRef
}

type TypeKind int

const (
	TypePrimitive TypeKind = iota
	TypeArray
	TypeRef
	TypeMap
)

type PrimKind int

const (
	PrimString PrimKind = iota
	PrimInt
	PrimFloat
	PrimBool
	PrimAny
	PrimBytes
)

func (t *Type) IsBytes() bool {
	return t != nil && t.Kind == TypePrimitive && t.Prim == PrimBytes
}

// Endpoint is a single API operation.
type Endpoint struct {
	OperationID  string
	Summary      string // short description from spec
	Description  string // longer description from spec
	Method       string // GET, POST, PUT, DELETE, PATCH
	Path         string // e.g. /pets/{petId}
	Tags         []string
	Params       []Param
	RequestBody  *Type       // nil if no body
	RequestCType string      // media type of the request body, e.g. "application/json-patch+json"; empty if no body
	FormFields   []FormField // multipart/form-data body fields; nil unless the request body is multipart/form-data
	ResponseType *Type       // nil if no response body (e.g. 204)
}

// FormField is one part of a multipart/form-data request body. Field keys may be
// flat dotted ASP.NET-style names (e.g. "Detail.Owner.Type"); Name is the
// derived method-parameter base name with any common container prefix stripped.
type FormField struct {
	Key      string // wire field name sent in the request, e.g. "Detail.Owner.Type" or "File"
	Name     string // method parameter base name (generators apply language casing), e.g. "Owner.Type"
	Type     Type
	Required bool
	IsFile   bool // binary upload sent as a file part; otherwise a plain form value
}

// Param is a path or query parameter.
type Param struct {
	Name     string
	In       string // "path" or "query"
	Type     Type
	Required bool
}
