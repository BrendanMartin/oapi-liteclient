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
)

// Endpoint is a single API operation.
type Endpoint struct {
	OperationID  string
	Summary      string // short description from spec
	Description  string // longer description from spec
	Method       string // GET, POST, PUT, DELETE, PATCH
	Path         string // e.g. /pets/{petId}
	Params       []Param
	RequestBody  *Type // nil if no body
	ResponseType *Type // nil if no response body (e.g. 204)
}

// Param is a path or query parameter.
type Param struct {
	Name     string
	In       string // "path" or "query"
	Type     Type
	Required bool
}
