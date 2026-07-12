package flixbus

import (
	"strings"
	"time"
)

// dateFormat is the dotted wire format (DD.MM.YYYY) the search API and the
// shop's deeplink both expect.
const dateFormat = "02.01.2006"

// City is one autocomplete match.
type City struct {
	ID              string   `json:"id"` // UUID — the id the search endpoint wants (never legacy_id)
	Name            string   `json:"name"`
	Country         string   `json:"country"`
	IsFlixbusCity   bool     `json:"is_flixbus_city"`
	HasTrainStation bool     `json:"has_train_station"`
	Location        Location `json:"location"`
}

// Location is a WGS84 coordinate pair.
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// SearchParams are the inputs for SearchTrips.
type SearchParams struct {
	FromCityID    string
	ToCityID      string
	DepartureDate time.Time
	Adults        int
	Children      int
	Bikes         int
	Currency      string
	Locale        string
}

// SearchResponse is the v4 search payload. Trip.Results, Cities and Stations
// are keyed maps, not arrays.
type SearchResponse struct {
	Trips               []Trip              `json:"trips"`
	Cities              map[string]NamedRef `json:"cities"`
	Stations            map[string]NamedRef `json:"stations"`
	PlatformFeeRequired bool                `json:"platform_fee_in_price_required"`
}

// NamedRef resolves a UUID key to a display name.
type NamedRef struct {
	Name string `json:"name"`
}

// Trip groups the results for one from/to/date combination.
type Trip struct {
	Results map[string]TripResult `json:"results"`
}

// TripResult is a single bookable connection.
type TripResult struct {
	UID          string    `json:"uid"`
	Status       string    `json:"status"`
	TransferType string    `json:"transfer_type"`
	Departure    Stop      `json:"departure"`
	Arrival      Stop      `json:"arrival"`
	Duration     Duration  `json:"duration"`
	Price        Price     `json:"price"`
	Available    Available `json:"available"`
}

// IsDirect reports whether the connection has no transfer. The uid prefix
// ("direct:" vs "ic:") is the reliable signal — TransferType is a display
// label and single-leg rides can carry "Fast" instead of "Direct".
func (r TripResult) IsDirect() bool {
	return strings.HasPrefix(r.UID, "direct:")
}

// EffectivePrice returns the amount due at checkout: the fee-inclusive total
// when the response requires the platform fee in the price, else the base
// total. ok is false when the fee is required but the fee-inclusive total is
// missing — a sign the API shape changed; callers should not trust the value.
func (r TripResult) EffectivePrice(feeRequired bool) (price float64, ok bool) {
	if !feeRequired {
		return r.Price.Total, true
	}
	if r.Price.TotalWithPlatformFee > 0 {
		return r.Price.TotalWithPlatformFee, true
	}
	return r.Price.Total, false
}

// Stop is one end of a connection.
type Stop struct {
	Date      string `json:"date"` // ISO 8601 with offset, passed through verbatim
	CityID    string `json:"city_id"`
	StationID string `json:"station_id"`
}

// Duration is the advertised travel time.
type Duration struct {
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
}

// Price in the requested currency. Total is the fare without platform fee;
// TotalWithPlatformFee is the amount due at checkout when
// SearchResponse.PlatformFeeRequired is set. Values are pass-through only —
// convert before doing arithmetic on them.
type Price struct {
	Total                float64 `json:"total"`
	Original             float64 `json:"original"`
	TotalWithPlatformFee float64 `json:"total_with_platform_fee"`
}

// Available capacity on the connection.
type Available struct {
	Seats     int `json:"seats"`
	BikeSlots int `json:"bike_slots"`
}
