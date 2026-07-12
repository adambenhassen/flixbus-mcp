// Package tools registers the MCP tool surface for flixbus-mcp.
package tools

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/adambenhassen/flixbus-mcp/internal/config"
	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
)

const (
	defaultMaxResults = 20
	capMaxResults     = 100
	maxPassengers     = 9
)

// Deps carries the shared dependencies for all tool handlers.
type Deps struct {
	Client *flixbus.Client
	Cfg    config.Config
}

// Register adds all FlixBus tools to the MCP server.
func Register(server *mcp.Server, d *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_cities",
		Description: "Search FlixBus cities by name. Returns city ids (UUIDs) usable with search_trips.",
	}, d.searchCities)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_trips",
		Description: "Search FlixBus trips between two cities on a date: times, duration, prices, seats and a booking link. Accepts city names (auto-resolved) or city UUIDs from search_cities.",
	}, d.searchTrips)
}

// SearchCitiesIn is the input for the search_cities tool.
type SearchCitiesIn struct {
	Query string `json:"query"          jsonschema:"city name or prefix, e.g. Berlin"`
	Lang  string `json:"lang,omitempty" jsonschema:"ISO 639-1 language for results, defaults to the server locale"`
}

// CityOut is one city match.
type CityOut struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Country         string  `json:"country"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	IsFlixbusCity   bool    `json:"is_flixbus_city"`
	HasTrainStation bool    `json:"has_train_station"`
}

// SearchCitiesOut is the output of the search_cities tool.
type SearchCitiesOut struct {
	Cities []CityOut `json:"cities"`
}

func (d *Deps) searchCities(ctx context.Context, _ *mcp.CallToolRequest, in SearchCitiesIn) (*mcp.CallToolResult, SearchCitiesOut, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, SearchCitiesOut{}, errors.New("query is required")
	}
	cities, err := d.Client.AutocompleteCities(ctx, in.Query, d.lang(in.Lang))
	if err != nil {
		return nil, SearchCitiesOut{}, err
	}
	out := SearchCitiesOut{Cities: make([]CityOut, 0, len(cities))}
	for _, c := range cities {
		out.Cities = append(out.Cities, CityOut{
			ID:              c.ID,
			Name:            c.Name,
			Country:         strings.ToUpper(c.Country),
			Latitude:        c.Location.Lat,
			Longitude:       c.Location.Lon,
			IsFlixbusCity:   c.IsFlixbusCity,
			HasTrainStation: c.HasTrainStation,
		})
	}
	return nil, out, nil
}

// SearchTripsIn is the input for the search_trips tool.
type SearchTripsIn struct {
	From       string `json:"from"                  jsonschema:"origin city name or FlixBus city UUID"`
	To         string `json:"to"                    jsonschema:"destination city name or FlixBus city UUID"`
	Date       string `json:"date"                  jsonschema:"departure date, YYYY-MM-DD"`
	Adults     int    `json:"adults,omitempty"      jsonschema:"adult passengers, default 1"`
	Children   int    `json:"children,omitempty"    jsonschema:"child passengers, default 0"`
	Bikes      int    `json:"bikes,omitempty"       jsonschema:"bike slots, default 0"`
	Currency   string `json:"currency,omitempty"    jsonschema:"ISO 4217 price currency, defaults to the server currency (EUR unless configured)"`
	Lang       string `json:"lang,omitempty"        jsonschema:"ISO 639-1 language for resolving city names, defaults to the server locale"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum trips returned, default 20"`
}

// TripOut is one bookable connection.
type TripOut struct {
	Departure        string  `json:"departure"` // ISO 8601 with offset
	Arrival          string  `json:"arrival"`
	DepartureStation string  `json:"departure_station,omitempty"`
	ArrivalStation   string  `json:"arrival_station,omitempty"`
	Duration         string  `json:"duration"` // e.g. 10h20m
	TransferType     string  `json:"transfer_type,omitempty"`
	Direct           bool    `json:"direct"`
	Price            float64 `json:"price"`
	SeatsRemaining   int     `json:"seats_remaining"`
	Status           string  `json:"status"`
}

// SearchTripsOut is the output of the search_trips tool. Currency and
// BookingURL apply to the whole result set, so they live here rather than
// repeated on every trip.
type SearchTripsOut struct {
	FromCity   string    `json:"from_city"`
	ToCity     string    `json:"to_city"`
	Currency   string    `json:"currency"`
	BookingURL string    `json:"booking_url"`
	Trips      []TripOut `json:"trips"`
}

func (d *Deps) searchTrips(ctx context.Context, _ *mcp.CallToolRequest, in SearchTripsIn) (*mcp.CallToolResult, SearchTripsOut, error) {
	var zero SearchTripsOut
	if strings.TrimSpace(in.From) == "" {
		return nil, zero, errors.New("from is required")
	}
	if strings.TrimSpace(in.To) == "" {
		return nil, zero, errors.New("to is required")
	}
	date := strings.TrimSpace(in.Date)
	if date == "" {
		return nil, zero, errors.New("date is required")
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, zero, fmt.Errorf("date %q: want YYYY-MM-DD: %w", in.Date, err)
	}
	adults := cmp.Or(in.Adults, 1)
	if adults < 1 || adults > maxPassengers {
		return nil, zero, fmt.Errorf("adults must be 1-%d", maxPassengers)
	}
	if in.Children < 0 || in.Children > maxPassengers || in.Bikes < 0 || in.Bikes > maxPassengers {
		return nil, zero, fmt.Errorf("children and bikes must be 0-%d", maxPassengers)
	}
	currency := strings.ToUpper(cmp.Or(in.Currency, d.Cfg.Currency))
	if !config.ValidCurrency(currency) {
		return nil, zero, fmt.Errorf("currency %q: want 3-letter ISO 4217 code", currency)
	}
	maxResults := cmp.Or(in.MaxResults, defaultMaxResults)
	if maxResults < 1 || maxResults > capMaxResults {
		return nil, zero, fmt.Errorf("max_results must be 1-%d", capMaxResults)
	}

	lang := d.lang(in.Lang)
	fromID, fromName, err := resolveCity(ctx, d.Client, in.From, lang)
	if err != nil {
		return nil, zero, fmt.Errorf("from: %w", err)
	}
	toID, toName, err := resolveCity(ctx, d.Client, in.To, lang)
	if err != nil {
		return nil, zero, fmt.Errorf("to: %w", err)
	}

	params := flixbus.SearchParams{
		FromCityID:    fromID,
		ToCityID:      toID,
		DepartureDate: day,
		Adults:        adults,
		Children:      in.Children,
		Bikes:         in.Bikes,
		Currency:      currency,
		Locale:        d.Cfg.Locale,
	}
	resp, err := d.Client.SearchTrips(ctx, params)
	if err != nil {
		return nil, zero, err
	}

	total := 0
	for _, trip := range resp.Trips {
		total += len(trip.Results)
	}
	trips := make([]TripOut, 0, total) // non-nil so empty results marshal as [], matching search_cities
	for _, trip := range resp.Trips {
		for _, r := range trip.Results {
			// a bookable result with no dates or price means the API shape
			// changed under us — fail loud rather than emit zeros
			if r.Status == "available" && (r.Departure.Date == "" || r.Arrival.Date == "" || r.Price.Total <= 0) {
				return nil, zero, fmt.Errorf("unexpected flixbus response shape (empty dates or price on available trip %s) — the API may have changed", r.UID)
			}
			price, ok := r.EffectivePrice(resp.PlatformFeeRequired)
			if !ok {
				return nil, zero, fmt.Errorf("flixbus requires the platform fee in prices but trip %s has no fee-inclusive total — the API may have changed", r.UID)
			}
			trips = append(trips, TripOut{
				Departure:        r.Departure.Date,
				Arrival:          r.Arrival.Date,
				DepartureStation: resp.Stations[r.Departure.StationID].Name,
				ArrivalStation:   resp.Stations[r.Arrival.StationID].Name,
				Duration:         fmt.Sprintf("%dh%02dm", r.Duration.Hours, r.Duration.Minutes),
				TransferType:     r.TransferType,
				Direct:           r.IsDirect(),
				Price:            price,
				SeatsRemaining:   r.Available.Seats,
				Status:           r.Status,
			})
		}
	}
	slices.SortFunc(trips, compareTrips)
	if len(trips) > maxResults {
		trips = trips[:maxResults]
	}

	out := SearchTripsOut{
		FromCity:   cmp.Or(resp.Cities[fromID].Name, fromName, in.From),
		ToCity:     cmp.Or(resp.Cities[toID].Name, toName, in.To),
		Currency:   currency,
		BookingURL: flixbus.BookingURL(params),
		Trips:      trips,
	}
	return nil, out, nil
}

// lang picks the autocomplete language: an explicit per-call override, else
// the language part of the configured locale.
func (d *Deps) lang(override string) string {
	return cmp.Or(override, langFromLocale(d.Cfg.Locale))
}

// compareTrips orders chronologically by departure instant (offset-aware, so
// mixed UTC offsets on DST nights still sort correctly), falling back to the
// raw string if a timestamp doesn't parse; price then arrival break ties so
// the order — and any max_results cut — is deterministic.
func compareTrips(a, b TripOut) int {
	ta, errA := time.Parse(time.RFC3339, a.Departure)
	tb, errB := time.Parse(time.RFC3339, b.Departure)
	var c int
	if errA == nil && errB == nil {
		c = ta.Compare(tb)
	} else {
		c = cmp.Compare(a.Departure, b.Departure)
	}
	if c != 0 {
		return c
	}
	if c := cmp.Compare(a.Price, b.Price); c != 0 {
		return c
	}
	return cmp.Compare(a.Arrival, b.Arrival)
}
