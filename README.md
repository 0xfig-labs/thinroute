# thinroute

thinroute is a local-first, OpenAI-compatible LLM gateway: a single entry point that routes requests across multiple AI providers, with model discovery, exact-match response caching, lightweight usage tracking, and Prometheus metrics.

> Single-machine, single-user, SQLite-only. Configuration lives in `config.yaml` and system environment variables — no `.env` files, no dashboard. The gateway API uses inbound API keys; `/health` and `/health/ready` remain public.

Requires Go 1.26 or later.

```bash
git clone https://github.com/0xfig-labs/thinroute.git
cd thinroute
go run ./cmd/thinroute config init
# Set provider credentials via environment variables, e.g.:
#   export OPENAI_API_KEY=sk-...
#   export ANTHROPIC_API_KEY=sk-ant-...
go run ./cmd/thinroute
```

The API server listens on `127.0.0.1:52180` by default. Change `server.listen` in `config.yaml` only when you need other devices to reach the gateway, and configure network boundaries yourself.

Build:

```bash
make build
```

## Configuration

- Default: `~/.config/thinroute/config.yaml` (or `$XDG_CONFIG_HOME/thinroute/config.yaml`)
- Example: [`config.example.yaml`](config.example.yaml)
- Create: `thinroute config init`
- Edit: `thinroute config edit`
- Print path: `thinroute config path`
- Print example: `thinroute config example`
- Validate: `thinroute config validate`
- Provider credentials are set via system environment variables, referenced in `config.yaml` as `${VAR}`.

Create and manage inbound API keys:

```bash
thinroute auth keys create --name my-client
thinroute auth keys list
thinroute auth keys revoke <id>
```

Use the generated key as a standard OpenAI-compatible client key:

```bash
export OPENAI_API_KEY=tr_...
```

`/health` and `/health/ready` are public. Other gateway API routes require
`Authorization: Bearer <API_KEY>`.

Do not commit `config.yaml`, API keys, database files, or build artifacts.

Other useful commands:

```bash
thinroute usage [--days 7] [--json]
thinroute providers status
thinroute models list [--json]
thinroute virtual-models list
thinroute providers benchmark --model gpt-4o-mini --runs 3
```

CLI commands initialize local provider and storage state directly; they do not
connect to a running gateway process.

## Supported OpenAI-Compatible Endpoints

thinroute implements a subset of the OpenAI API. The following endpoints are supported:

| Endpoint | Description |
|----------|-------------|
| `GET /v1/models` | List available models |
| `POST /v1/chat/completions` | Chat completions (streaming and non-streaming) |
| `POST /v1/responses` | Responses API |
| `GET /v1/responses/{id}` | Retrieve a response |
| `POST /v1/responses/{id}/cancel` | Cancel a running response |
| `GET /v1/responses/{id}/input_items` | List input items for a response |
| `POST /v1/files` | Upload a file |
| `GET /v1/files` | List files |
| `GET /v1/files/{id}` | Retrieve file metadata |
| `GET /v1/files/{id}/content` | Download file content |
| `DELETE /v1/files/{id}` | Delete a file |
| `POST /v1/batches` | Create a batch |
| `GET /v1/batches` | List batches |
| `GET /v1/batches/{id}` | Retrieve batch status |
| `POST /v1/batches/{id}/cancel` | Cancel a batch |
| `GET /v1/conversations` | List conversations |
| `POST /v1/conversations` | Create a conversation |
| `GET /v1/conversations/{id}` | Retrieve a conversation |
| `DELETE /v1/conversations/{id}` | Delete a conversation |

Endpoints not listed above (for example fine-tuning, assistants, and moderations) are not supported. thinroute does not claim full OpenAI API compatibility.

## Client Integration

thinroute's OpenAI-compatible API can be used directly with these clients:

### LiteLLM Proxy

```yaml
# litellm_config.yaml
model_list:
  - model_name: gpt-5-mini
    litellm_params:
      model: openai/gpt-5-mini
      api_base: http://localhost:52180/v1
      api_key: unused
```

### Open WebUI

Admin Settings → Connections → OpenAI API:
- URL: `http://localhost:52180/v1`
- Key: any non-empty value (e.g. `unused`)

### LibreChat

```yaml
# librechat.yaml
endpoints:
  custom:
    - name: thinroute
      apiKey: "unused"
      baseURL: "http://localhost:52180/v1"
      models:
        default: ["deepseek/deepseek-v4-flash"]
        fetch: true
```

## Development

```bash
go test ./...
go test -race ./...
make lint
```

## Security Boundaries

thinroute binds to loopback (`127.0.0.1`) by default. `/health` and `/health/ready` are public; other gateway API routes require a generated inbound API key. Binding to a non-loopback address still exposes the gateway to the network, so use a trusted or reverse-proxied environment and manage downstream provider credentials carefully.

When reporting security issues, do not include API keys, logs, or request bodies.

## License

See [`LICENSE`](LICENSE).
