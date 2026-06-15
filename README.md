# oapi-liteclient

Generate minimal API clients from OpenAPI specs.

No custom HTTP frameworks, no exception hierarchies, no sprawling project structure. Just typed models and a thin client using the language's standard HTTP library.

## Install

**Pre-built binary** — download from [Releases](https://github.com/brendanmartin/oapi-liteclient/releases) for your OS.

**With Go:**

```bash
go install github.com/brendanmartin/oapi-liteclient/cmd/oapi-liteclient@latest
```

## Usage

```bash
oapi-liteclient --spec <file-or-url> --lang python --out ./myclient/
```

```bash
# From a local file
oapi-liteclient --spec petstore.yaml --lang python --out ./petstore/

# From a URL
oapi-liteclient --spec https://api.example.com/openapi.json --lang python --out ./client/

# With auth strategy
oapi-liteclient --spec spec.yaml --lang python --auth gcp-id-token --out ./client/

# With dataclasses instead of Pydantic
oapi-liteclient --spec spec.yaml --lang python --style dataclass --out ./client/

# Generate a Go client
oapi-liteclient --spec petstore.yaml --lang go --auth api-key --out ./petstore/
```

### Python

The generated output includes a `pyproject.toml`, so you can install it directly:

```bash
pip install ./petstore
```

```python
from petstore import Client, Pet, PetCreate

with Client("https://petstore.example.com/v1", bearer_token="sk-...") as c:
    pets = c.list_pets(limit=10)
    new_pet = c.create_pet(PetCreate(name="Buddy"))
    pet = c.get_pet(pet_id=1)
```

### Go

```go
import "myproject/petstore"

client := petstore.NewClient("my-api-key")

pets, err := client.ListPets(ctx).Limit(10).Do()
pet, err := client.CreatePet(ctx, petstore.PetCreate{Name: "Buddy"}).Do()
pet, err := client.GetPet(ctx, 1).Do()
err := client.DeletePet(ctx, 1).Do()
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--spec` | (required) | Path or URL to OpenAPI spec (YAML or JSON) |
| `--lang` | `python` | Target language: `python` or `go` |
| `--style` | `pydantic` | Model style (Python only): `pydantic` or `dataclass` |
| `--auth` | `none` | Auth strategy (see below) |
| `--out` | `./client` | Output directory |
| `--force` | `false` | Overwrite output directory if it exists |
| `--lenient` | `false` | Make all model fields optional (tolerates null values from inaccurate specs) |
| `--merge <file>` | | Supplemental OpenAPI YAML/JSON fragment to deep-merge into the base spec before parsing. Repeat to apply multiple fragments in order. This is a plain recursive merge (maps merge; scalars and arrays replace), **not** OpenAPI Overlay 1.0 — do not pass `overlay:`/`actions:` documents. |
| `--package-version` | `0.1.0` | Version written to the generated Python `pyproject.toml` |
| `--version` | | Print version and exit |

### Supplemental Spec Merge

Use `--merge` when the upstream OpenAPI spec omits endpoints or needs small local patches. Fragments are YAML or JSON using the same OpenAPI shape as the base spec. The merge is recursive and fragment-wins: maps are merged, while scalars and arrays replace the base value. Repeat `--merge` to apply fragments in order.

This is intentionally **not** OpenAPI Overlay 1.0 (`overlay:`/`actions:`/JSONPath). It is a plain structural merge of spec-shaped fragments, which is simpler for the common case of adding a missing endpoint. Because arrays replace wholesale, prefer fragments that add *new* paths/operations rather than patching an operation that already declares `parameters`/`tags`/`security` (you would have to restate the whole array).

```bash
oapi-liteclient --spec upstream.yaml --merge fulcrum-extra.yaml --out ./fulcrum_client
```

Example fragment for a PDF endpoint:

```yaml
paths:
  /api/v2/quotes/{quoteId}/pdf:
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
          description: Quote PDF
          content:
            application/pdf:
              schema:
                type: string
                format: binary
```

## Auth Strategies

| Strategy | Python | Go |
|----------|--------|-----|
| `none` | No auth code | No auth code |
| `custom` | `auth: Callable` returning headers | `authFunc func(req *http.Request)` |
| `bearer-token` | `bearer_token: str` | `bearerToken string` |
| `api-key` | `api_key: str` with configurable header | `apiKey string` with configurable header |
| `gcp-id-token` | `google.oauth2.id_token` with 55 min cache | `google.golang.org/api/idtoken` TokenSource |

## Supported Languages

| Language | Output | Models | HTTP Library | Status |
|----------|--------|--------|-------------|--------|
| Python | `client.py` + `pyproject.toml` | Pydantic or dataclass | httpx | Available |
| Go | `client.go` | structs with JSON tags | net/http | Available |
| TypeScript | `client.ts` | interfaces | fetch | Planned |

## Size Comparison

Generating a Python client from two large public OpenAPI specs, compared with [openapi-generator](https://github.com/OpenAPITools/openapi-generator):

| Spec | openapi-generator | oapi-liteclient | Reduction |
|------|-------------------|-----------------|-----------|
| **GitHub REST API** (12.5 MB, 1183 endpoints) | 4,114 files / 31 MB / 34.6s | 50 files / 1.0 MB / 1.9s | 98% fewer files, 97% smaller, 18x faster |
| **Kubernetes API** (3.9 MB, 1111 endpoints) | 1,652 files / 47 MB / 20.1s | 68 files / 1.4 MB / 0.8s | 96% fewer files, 97% smaller, 25x faster |

For large specs with tags, endpoints are grouped by tag prefix into one file per logical area (e.g. all `Invoice`, `Invoice Line Item`, and `Invoice Tax Line Item` endpoints go into `invoice.py`). Specs without tags produce a single file.

## What Gets Generated

For a typical 3-5 endpoint API, the output is a single file (~100-200 lines). For larger specs with tagged endpoints, the output splits into one file per tag group:

- **Models** — Pydantic `BaseModel` classes (Python) or structs with JSON tags (Go)
- **Client** — one method per endpoint with typed parameters and return values. Go uses a request builder pattern with chained setters and `Do()`
- **Auth** — baked-in strategy based on `--auth` flag
- **Errors** — `APIError` exception (Python) or `*APIError` implementing `error` (Go)

## OpenAPI Support

- OpenAPI 3.0 and 3.1
- JSON request/response bodies, including JSON-Patch (`application/json-patch+json`) — the request body's media type is sent as the `Content-Type`
- A 2xx response body whose schema is `type: string, format: binary` (or Swagger 2 `type: file`) generates as `bytes` in Python and `[]byte` in Go. JSON responses take precedence when an operation offers both. Non-binary, non-JSON bodies (e.g. `text/plain`) are not decoded — the method returns no body, as before.
- `multipart/form-data` uploads — binary fields become file parts, other fields are sent as form values under their original (possibly dotted) keys; a container prefix common to all keys is stripped from parameter names
- Path and query parameters
- `$ref` to component schemas
- Arrays of primitives and refs
- Nested object deserialization (via Pydantic `model_validate`)
- Default values from spec
- Field aliases for camelCase and reserved words

## Development

```bash
go build -o oapi-liteclient ./cmd/oapi-liteclient
go test ./...
golangci-lint run ./...
```

Pre-commit hook runs gofmt, go fix, go vet, golangci-lint, and tests automatically.
