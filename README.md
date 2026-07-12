<div align="center">

<img src="assets/banner.svg" alt="flixbus-mcp — FlixBus trip search for AI agents" width="100%">

# flixbus-mcp

**FlixBus trip search for AI agents — a Model Context Protocol server in Go.**

[![CI](https://github.com/adambenhassen/flixbus-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/adambenhassen/flixbus-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/adambenhassen/flixbus-mcp)](https://github.com/adambenhassen/flixbus-mcp/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/adambenhassen/flixbus-mcp)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/adambenhassen/flixbus-mcp)](https://goreportcard.com/report/github.com/adambenhassen/flixbus-mcp)

</div>

Talks to FlixBus's public JSON API directly — no API key, no browser, no scraping. City names resolve automatically, prices are checkout totals (platform fee included), and every search comes with a ready-to-open booking link. Serves MCP over **stdio** and **streamable HTTP**.

## Features

- 🚌 **Trip search** — times, stations, duration, direct/transfer, live prices and seat availability
- 🏙️ **City resolution** — pass `"Berlin"`, not a UUID; ambiguous names return candidates instead of guessing
- 💶 **Honest prices** — checkout totals including FlixBus's mandatory platform fee, in your currency
- 🔗 **Booking deeplinks** — one click from search result to the FlixBus shop with the full passenger selection
- 🔌 **Two transports** — stdio for MCP clients, streamable HTTP for daemons (loopback by default)
- 📦 **Zero config** — a single static binary or a 10 MB distroless container; one direct dependency

## Quick start

### Claude Code

```sh
claude mcp add flixbus -- /path/to/flixbus-mcp -transport stdio
```

### Claude Desktop / other MCP clients

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

### HTTP

```sh
# binds 127.0.0.1:8748 — the server has no auth of its own, so only
# expose it on networks you trust or behind a reverse proxy
flixbus-mcp
flixbus-mcp -addr :8748   # all interfaces
```

### Docker

```sh
docker run --rm -p 8748:8748 ghcr.io/adambenhassen/flixbus-mcp
```

### From source

```sh
go install github.com/adambenhassen/flixbus-mcp/cmd/flixbus-mcp@latest
```


## Tools

### `search_trips`

Search trips between two cities on a date.

| Input | Type | Description |
|-------|------|-------------|
| `from` | string | origin city name or FlixBus city UUID **(required)** |
| `to` | string | destination city name or FlixBus city UUID **(required)** |
| `date` | string | departure date, `YYYY-MM-DD` **(required)** |
| `adults` | int | adult passengers, default 1 (max 9) |
| `children` | int | child passengers, default 0 (max 9) |
| `bikes` | int | bike slots, default 0 (max 9) |
| `currency` | string | ISO 4217 price currency, defaults to the server currency |
| `lang` | string | ISO 639-1 language for resolving city names, defaults to the server locale |
| `max_results` | int | maximum trips returned, default 20 (max 100) |

<details>
<summary>Example response</summary>

```json
{
  "from_city": "Berlin",
  "to_city": "Munich",
  "currency": "EUR",
  "booking_url": "https://shop.flixbus.com/search?adult=1&arrivalCity=…&currency=EUR&departureCity=…&rideDate=20.07.2026",
  "trips": [
    {
      "departure": "2026-07-20T07:15:00+02:00",
      "arrival": "2026-07-20T17:35:00+02:00",
      "departure_station": "Berlin Südkreuz",
      "arrival_station": "Munich Fröttmaning",
      "duration": "10h20m",
      "transfer_type": "Transfer",
      "direct": false,
      "price": 78.97,
      "seats_remaining": 17,
      "status": "available"
    }
  ]
}
```

</details>

Trips come back in chronological departure order. If a city name is ambiguous (two Birminghams), the error lists the candidate ids to pass instead.

### `search_cities`

Find FlixBus cities and their ids.

| Input | Type | Description |
|-------|------|-------------|
| `query` | string | city name or prefix, e.g. `Berlin` **(required)** |
| `lang` | string | ISO 639-1 language for results, defaults to the server locale |

Returns matches with the id (UUID), name, country, coordinates, and whether the city is bookable (`is_flixbus_city`) or has a train station.

## Configuration

All configuration is via environment variables — none are required.

| Variable | Default | Description |
|----------|---------|-------------|
| `FLIXBUS_CURRENCY` | `EUR` | default price currency |
| `FLIXBUS_LOCALE` | `en_US` | locale for search results (`en-US` is normalized) |
| `FLIXBUS_TIMEOUT` | `30s` | HTTP client timeout, must be positive |
| `FLIXBUS_API_KEY` | _(empty)_ | optional, sent as `X-Api-Key` if ever needed |

| Flag | Default | Description |
|------|---------|-------------|
| `-transport` | `http` | `stdio` or `http` |
| `-addr` | `127.0.0.1:8748` | listen address for the http transport |

## Development

```sh
go test ./...                              # unit tests (recorded API fixtures)
go test -tags smoke -timeout 5m ./smoke    # end-to-end against the live API
golangci-lint run
```

CI runs vet, race-enabled tests, the linter, and a Docker build on every push and PR; live smoke tests gate master and releases. Publishing a GitHub release pushes the container image to `ghcr.io`.

## How it works

The server calls the same unofficial JSON API that flixbus.com itself uses (`global.api.flixbus.com`): one endpoint to resolve city names, one to search trips. It requires no credentials today, but it is not a contracted API — endpoints may change without notice, and this project may break when they do. Be polite with request volume. Not affiliated with Flix SE.
