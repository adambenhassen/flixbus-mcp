# flixbus-mcp

MCP server for FlixBus trip search. Talks to FlixBus's public JSON API directly — no API key, no browser, no scraping. Supports stdio and streamable HTTP transports.

## Tools

### `search_cities`

Find FlixBus cities and their ids.

| Input | Type | Description |
|-------|------|-------------|
| `query` | string | city name or prefix, e.g. `Berlin` (required) |
| `lang` | string | ISO 639-1 language for results, defaults to the server locale's language (`en` unless `FLIXBUS_LOCALE` is set) |

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
| `bikes` | int | bike slots, default 0 (max 9 per category) |
| `currency` | string | ISO 4217 price currency, defaults to the server currency (`EUR` unless `FLIXBUS_CURRENCY` is set) |
| `lang` | string | ISO 639-1 language for resolving city names, defaults to the server locale's language |
| `max_results` | int | maximum trips returned, default 20 (max 100) |

City names are resolved automatically; if a name is ambiguous the error lists candidate ids. Returns the currency, a `shop.flixbus.com` booking link carrying the full passenger selection, and trips in chronological departure order: times (ISO 8601), station names, duration, transfer type, price (checkout total, including FlixBus's platform fee when it is mandatory), seats remaining, and status.

## Run

```sh
go build ./cmd/flixbus-mcp

# http (default; binds loopback — the server has no auth of its own,
# so only expose it on networks you trust or behind a reverse proxy)
./flixbus-mcp
./flixbus-mcp -addr :8748   # all interfaces

# stdio
./flixbus-mcp -transport stdio

# docker
docker run -p 8748:8748 ghcr.io/adambenhassen/flixbus-mcp
```

Client config (stdio):

```json
{
  "mcpServers": {
    "flixbus": {
      "command": "/path/to/flixbus-mcp",
      "args": ["-transport", "stdio"]
    }
  }
}
```

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `FLIXBUS_CURRENCY` | `EUR` | default price currency |
| `FLIXBUS_LOCALE` | `en_US` | locale for search results |
| `FLIXBUS_TIMEOUT` | `30s` | HTTP client timeout (must be positive) |
| `FLIXBUS_API_KEY` | _(empty)_ | optional, sent as `X-Api-Key` if set |

## Notes

This uses the unofficial API that flixbus.com itself calls. It requires no credentials today, but endpoints may change without notice. Be polite with request volume.

## Development

```sh
go test ./...
golangci-lint run
```
