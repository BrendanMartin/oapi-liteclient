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

```python
from petstore.client import Client, Pet, PetCreate

with Client("https://petstore.example.com/v1") as c:
    pets = c.list_pets(limit=10)
    new_pet = c.create_pet(PetCreate(name="Buddy"))
    pet = c.get_pet(pet_id=1)
```

### Go

```go
import "myproject/petstore"

client := petstore.NewClient("https://petstore.example.com/v1", nil, "my-api-key")

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
| Python | `client.py` | Pydantic or dataclass | httpx | Available |
| Go | `client.go` | structs with JSON tags | net/http | Available |
| TypeScript | `client.ts` | interfaces | fetch | Planned |

## What Gets Generated

For a typical 3-5 endpoint API, the output is a single file (~100-200 lines) containing:

- **Models** — Pydantic `BaseModel` classes (Python) or structs with JSON tags (Go)
- **Client** — one method per endpoint with typed parameters and return values. Go uses a request builder pattern with chained setters and `Do()`
- **Auth** — baked-in strategy based on `--auth` flag
- **Errors** — `APIError` exception (Python) or `*APIError` implementing `error` (Go)

## OpenAPI Support

- OpenAPI 3.0 and 3.1
- JSON request/response bodies
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
