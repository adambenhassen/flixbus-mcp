// Package tools registers the MCP tool surface for flixbus-mcp.
package tools

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
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
	Lang  string `json:"lang,omitempty" jsonschema:"ISO 639-1 language for results, default en"`
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
	lang := cmp.Or(in.Lang, "en")
	cities, err := d.Client.AutocompleteCities(ctx, in.Query, lang)
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
	Currency   string `json:"currency,omitempty"    jsonschema:"ISO 4217 price currency, default EUR"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum trips returned, default 20"`
}

// TripOut is one bookable connection.
type TripOut struct {
	Departure      string  `json:"departure"` // ISO 8601 with offset
	Arrival        string  `json:"arrival"`
	Duration       string  `json:"duration"` // e.g. 10h20m
	TransferType   string  `json:"transfer_type,omitempty"`
	Direct         bool    `json:"direct"`
	Price          float64 `json:"price"`
	Currency       string  `json:"currency"`
	SeatsRemaining int     `json:"seats_remaining"`
	Status         string  `json:"status"`
	BookingURL     string  `json:"booking_url"`
}

// SearchTripsOut is the output of the search_trips tool.
type SearchTripsOut struct {
	FromCity string    `json:"from_city"`
	ToCity   string    `json:"to_city"`
	Trips    []TripOut `json:"trips"`
}

func (d *Deps) searchTrips(ctx context.Context, _ *mcp.CallToolRequest, in SearchTripsIn) (*mcp.CallToolResult, SearchTripsOut, error) {
	var zero SearchTripsOut
	if strings.TrimSpace(in.From) == "" {
		return nil, zero, errors.New("from is required")
	}
	if strings.TrimSpace(in.To) == "" {
		return nil, zero, errors.New("to is required")
	}
	if in.Date == "" {
		return nil, zero, errors.New("date is required")
	}
	day, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		return nil, zero, fmt.Errorf("date %q: want YYYY-MM-DD", in.Date)
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
		return nil, zero, fmt.Errorf("currency %q: want 3-letter ISO 4217 code", in.Currency)
	}
	maxResults := cmp.Or(in.MaxResults, defaultMaxResults)
	if maxResults < 1 || maxResults > capMaxResults {
		return nil, zero, fmt.Errorf("max_results must be 1-%d", capMaxResults)
	}

	from, err := resolveCity(ctx, d.Client, in.From)
	if err != nil {
		return nil, zero, fmt.Errorf("from: %w", err)
	}
	to, err := resolveCity(ctx, d.Client, in.To)
	if err != nil {
		return nil, zero, fmt.Errorf("to: %w", err)
	}

	apiDate := day.Format("02.01.2006") // FlixBus wants DD.MM.YYYY
	resp, err := d.Client.SearchTrips(ctx, flixbus.SearchParams{
		FromCityID:    from.ID,
		ToCityID:      to.ID,
		DepartureDate: apiDate,
		Adults:        adults,
		Children:      in.Children,
		Bikes:         in.Bikes,
		Currency:      currency,
		Locale:        d.Cfg.Locale,
	})
	if err != nil {
		return nil, zero, err
	}

	booking := bookingURL(from.ID, to.ID, apiDate, adults)
	var trips []TripOut
	for _, trip := range resp.Trips {
		for _, r := range trip.Results {
			trips = append(trips, TripOut{
				Departure:      r.Departure.Date,
				Arrival:        r.Arrival.Date,
				Duration:       fmt.Sprintf("%dh%02dm", r.Duration.Hours, r.Duration.Minutes),
				TransferType:   r.TransferType,
				Direct:         strings.EqualFold(r.TransferType, "direct"),
				Price:          r.Price.Total,
				Currency:       currency,
				SeatsRemaining: r.Available.Seats,
				Status:         r.Status,
				BookingURL:     booking,
			})
		}
	}
	// ponytail: lexicographic sort — same route+date shares one UTC offset in practice
	slices.SortFunc(trips, func(a, b TripOut) int { return cmp.Compare(a.Departure, b.Departure) })
	if len(trips) > maxResults {
		trips = trips[:maxResults]
	}

	out := SearchTripsOut{
		FromCity: cmp.Or(resp.Cities[from.ID].Name, from.Name, in.From),
		ToCity:   cmp.Or(resp.Cities[to.ID].Name, to.Name, in.To),
		Trips:    trips,
	}
	return nil, out, nil
}

// bookingURL builds a shop.flixbus.com deeplink (verified URL shape).
func bookingURL(fromID, toID, apiDate string, adults int) string {
	q := url.Values{}
	q.Set("departureCity", fromID)
	q.Set("arrivalCity", toID)
	q.Set("rideDate", apiDate)
	q.Set("adult", strconv.Itoa(adults))
	return "https://shop.flixbus.com/search?" + q.Encode()
}
