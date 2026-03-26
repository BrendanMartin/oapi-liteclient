# oapi-liteclient

Generate minimal, single-file API clients from OpenAPI specs.

No custom HTTP frameworks, no exception hierarchies, no sprawling project structure. Just typed models and a thin client using the language's standard HTTP library.

## Install

**Pre-built binary** — download from [Releases](https://github.com/brendanmartin/oapi-liteclient/releases) for your OS.

**With Go:**

```bash
go install github.com/brendanmartin/oapi-liteclient/cmd@latest
```

## Usage

```
oapi-liteclient generate --spec <file-or-url> --lang <language> --out <dir>
```

```bash
# From a local file
oapi-liteclient generate --spec petstore.yaml --lang python --out ./client/

# From a URL
oapi-liteclient generate --spec https://api.example.com/openapi.json --lang python --out ./client/
```

This produces a single `client.py` in the output directory:

```python
from client import Client, Pet, PetCreate

with Client("https://petstore.example.com/v1", api_key="sk-...") as c:
    pets = c.list_pets(limit=10)
    new_pet = c.create_pet(PetCreate(name="Buddy"))
    pet = c.get_pet(pet_id=1)
```

## Supported Languages

| Language | Output | HTTP Library | Status |
|----------|--------|-------------|--------|
| Python | `client.py` — dataclasses + typed client | httpx | Available |
| TypeScript | `client.ts` — interfaces + typed fetch | fetch | Planned |
| Go | `client.go` — structs + net/http | net/http | Planned |

## What Gets Generated

For a typical 3-5 endpoint API, the output is a single file (~100-200 lines) containing:

- **Models** — typed data classes for request/response schemas
- **Client** — one method per endpoint, typed parameters and return values
- **Auth** — API key or bearer token injected via constructor
- **Errors** — exception raised on non-2xx responses with status code and body

## OpenAPI Support

- OpenAPI 3.0 and 3.1
- JSON request/response bodies
- Path and query parameters
- `$ref` to component schemas
- Arrays of primitives and refs
- API key and bearer token auth

## Development

```bash
go build -o oapi-liteclient ./cmd
go test ./...
```
