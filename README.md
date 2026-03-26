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
```

This produces a Python package in the output directory:

```python
from petstore.client import Client, Pet, PetCreate

with Client("https://petstore.example.com/v1") as c:
    pets = c.list_pets(limit=10)
    new_pet = c.create_pet(PetCreate(name="Buddy"))
    pet = c.get_pet(pet_id=1)
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--spec` | (required) | Path or URL to OpenAPI spec (YAML or JSON) |
| `--lang` | `python` | Target language |
| `--style` | `pydantic` | Model style: `pydantic` or `dataclass` |
| `--auth` | `none` | Auth strategy (see below) |
| `--out` | `./client` | Output directory |

## Auth Strategies

| Strategy | Description |
|----------|-------------|
| `none` | No auth code generated |
| `custom` | `auth: Callable[[], dict[str, str]]` parameter — caller provides a function returning headers |
| `bearer-token` | `bearer_token: str` parameter, sent as `Authorization: Bearer <token>` |
| `api-key` | `api_key: str` parameter with configurable header name (default `X-API-Key`) |
| `gcp-id-token` | Google Cloud ID token with automatic caching (55 min). For calling Cloud Run services |

## Supported Languages

| Language | Output | Models | HTTP Library | Status |
|----------|--------|--------|-------------|--------|
| Python | `client.py` | Pydantic or dataclass | httpx | Available |
| TypeScript | `client.ts` | interfaces | fetch | Planned |
| Go | `client.go` | structs | net/http | Planned |

## What Gets Generated

For a typical 3-5 endpoint API, the output is a single file (~100-200 lines) containing:

- **Models** — Pydantic `BaseModel` classes (or dataclasses) with typed fields, defaults, and aliases for camelCase/reserved words
- **Client** — one method per endpoint with typed parameters and return values
- **Docstrings** — from operation `summary` or `description` in the spec
- **Auth** — baked-in strategy based on `--auth` flag
- **Errors** — exception raised on non-2xx responses

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
