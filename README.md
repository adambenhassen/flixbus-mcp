# flixbus-mcp

MCP server for FlixBus trip search. Talks to FlixBus's public JSON API directly — no API key, no browser, no scraping. Supports stdio and streamable HTTP transports.

## Tools

### `search_cities`

Find FlixBus cities and their ids.

| Input | Type | Description |
|-------|------|-------------|
| `query` | string | city name or prefix, e.g. `Berlin` (required) |
| `lang` | string | ISO 639-1 language for results, default `en` |

Returns `cities`: id (UUID), name, country, latitude/longitude, `is_flixbus_city`, `has_train_station`.

### `search_trips`

Search trips between two cities on a date.

| Input | Type | Description |
|-------|------|-------------|
| `from` | string | origin city name or FlixBus city UUID (required) |
| `to` | string | destination city name or FlixBus city UUID (required) |
| `date` | string | departure date, `YYYY-MM-DD` (required) |
| `adults` | int | adult passengers, default 1 |
| `children` | int | child passengers, default 0 |
| `bikes` | int | bike slots, default 0 |
| `currency` | string | ISO 4217 price currency, default `EUR` |
| `max_results` | int | maximum trips returned, default 20 |

City names are resolved automatically; if a name is ambiguous the error lists candidate ids. Returns trips sorted by departure: times (ISO 8601), duration, transfer type, price, seats remaining, status, and a `shop.flixbus.com` booking link.

## Run

```sh
go build ./cmd/flixbus-mcp

# stdio (default)
./flixbus-mcp

# http
./flixbus-mcp -transport http -addr :8748

# docker
docker run -p 8748:8748 ghcr.io/adambenhassen/flixbus-mcp
```

Client config (stdio):

```json
{
  "mcpServers": {
    "flixbus": { "command": "/path/to/flixbus-mcp" }
  }
}
```

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `FLIXBUS_CURRENCY` | `EUR` | default price currency |
| `FLIXBUS_LOCALE` | `en_US` | locale for search results |
| `FLIXBUS_TIMEOUT` | `30s` | HTTP client timeout |
| `FLIXBUS_API_KEY` | _(empty)_ | optional, sent as `X-Api-Key` if set |
| `FLIXBUS_BASE_URL` | `https://global.api.flixbus.com` | API base URL (tests) |

## Notes

This uses the unofficial API that flixbus.com itself calls. It requires no credentials today, but endpoints may change without notice. Be polite with request volume.

## Development

```sh
go test ./...
golangci-lint run
```
