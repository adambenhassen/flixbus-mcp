package flixbus

// City is one autocomplete match.
type City struct {
	ID              string   `json:"id"` // UUID — the id the search endpoint wants (never legacy_id)
	Name            string   `json:"name"`
	Country         string   `json:"country"`
	Score           float64  `json:"score"`
	IsFlixbusCity   bool     `json:"is_flixbus_city"`
	HasTrainStation bool     `json:"has_train_station"`
	Location        Location `json:"location"`
}

// Location is a WGS84 coordinate pair.
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// SearchParams are the inputs for SearchTrips. DepartureDate must already be
// in the API's DD.MM.YYYY format.
type SearchParams struct {
	FromCityID    string
	ToCityID      string
	DepartureDate string
	Adults        int
	Children      int
	Bikes         int
	Currency      string
	Locale        string
}

// SearchResponse is the v4 search payload. Results and Cities are keyed maps,
// not arrays.
type SearchResponse struct {
	Trips  []Trip              `json:"trips"`
	Cities map[string]NamedRef `json:"cities"`
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

// Price in the requested currency. Total is the fare without platform fee.
type Price struct {
	Total    float64 `json:"total"`
	Original float64 `json:"original"`
}

// Available capacity on the connection.
type Available struct {
	Seats     int `json:"seats"`
	BikeSlots int `json:"bike_slots"`
}
