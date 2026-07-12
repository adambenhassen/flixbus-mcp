package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/adambenhassen/flixbus-mcp/internal/config"
	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
	"github.com/adambenhassen/flixbus-mcp/internal/tools"
)

const munichUUID = "40d901a5-8646-11e6-9066-549f350fcb0c"

const berlinAutocomplete = `[{"id":"` + berlinUUID + `","name":"Berlin","country":"de","is_flixbus_city":true}]`

// stubClient serves the Berlin autocomplete plus the given search body.
func stubClient(t *testing.T, searchBody []byte) *flixbus.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body []byte
		switch r.URL.Path {
		case "/search/autocomplete/cities":
			body = []byte(berlinAutocomplete)
		case "/search/service/v4/search":
			body = searchBody
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			return
		}
		if _, err := w.Write(body); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return flixbus.New(srv.URL, "", 5*time.Second)
}

// testDeps wires Deps against the recorded Berlin→Munich search fixture.
func testDeps(t *testing.T) *tools.Deps {
	t.Helper()
	search, err := os.ReadFile("../flixbus/testdata/search_berlin_munich.json")
	if err != nil {
		t.Fatalf("read search fixture: %v", err)
	}
	return &tools.Deps{Client: stubClient(t, search), Cfg: config.Config{Currency: "EUR", Locale: "en_US"}}
}

func TestRegister(t *testing.T) {
	// AddTool infers schemas from struct tags at registration time; a bad tag
	// or renamed tool would panic here before ever reaching production
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	tools.Register(server, testDeps(t))
}

func TestSearchTripsValidation(t *testing.T) {
	d := testDeps(t)
	cases := []struct {
		name string
		in   tools.SearchTripsIn
		want string // substring of the expected error
	}{
		{"missing from", tools.SearchTripsIn{To: "Munich", Date: "2026-07-20"}, "from"},
		{"missing to", tools.SearchTripsIn{From: "Berlin", Date: "2026-07-20"}, "to"},
		{"missing date", tools.SearchTripsIn{From: "Berlin", To: "Munich"}, "date"},
		{"bad date format", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "20.07.2026"}, "yyyy-mm-dd"},
		{"bad currency", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Currency: "euros"}, "currency"},
		{"negative adults", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Adults: -1}, "adults"},
		{"too many bikes", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Bikes: 10}, "bikes"},
		{"negative children", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Children: -1}, "children"},
		{"max_results out of range", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", MaxResults: 101}, "max_results"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tools.SearchTripsHandler(d, context.Background(), nil, tc.in)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestSearchTripsHappyPath(t *testing.T) {
	d := testDeps(t)
	_, out, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20", Currency: "eur",
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	if len(out.Trips) == 0 {
		t.Fatal("no trips returned")
	}
	for i := 1; i < len(out.Trips); i++ {
		prev, err := time.Parse(time.RFC3339, out.Trips[i-1].Departure)
		if err != nil {
			t.Fatalf("unparseable departure %q: %v", out.Trips[i-1].Departure, err)
		}
		cur, err := time.Parse(time.RFC3339, out.Trips[i].Departure)
		if err != nil {
			t.Fatalf("unparseable departure %q: %v", out.Trips[i].Departure, err)
		}
		if prev.After(cur) {
			t.Errorf("trips not in chronological order: %q > %q", out.Trips[i-1].Departure, out.Trips[i].Departure)
		}
	}
	first := out.Trips[0]
	if first.Departure == "" || first.Arrival == "" || first.Duration == "" {
		t.Errorf("missing fields: %+v", first)
	}
	if first.DepartureStation == "" || first.ArrivalStation == "" {
		t.Errorf("station names not resolved: %+v", first)
	}
	if out.Currency != "EUR" {
		t.Errorf("Currency = %q, want lowercase input normalized to EUR", out.Currency)
	}
	for _, want := range []string{
		"shop.flixbus.com/search", "rideDate=20.07.2026",
		"departureCity=" + berlinUUID, "arrivalCity=" + munichUUID,
	} {
		if !strings.Contains(out.BookingURL, want) {
			t.Errorf("BookingURL %q missing %q", out.BookingURL, want)
		}
	}
	// fixture declares platform_fee_in_price_required; first trip's checkout
	// total is 78.97 (total 77.98 + fee) — the fee-inclusive price must win
	if first.Price != 78.97 {
		t.Errorf("Price = %v, want fee-inclusive 78.97", first.Price)
	}
	// the fixture's nonstop rides carry uid prefix "direct:" but label "Fast"
	// or "Direct" — both must surface as direct
	directs := 0
	for _, trip := range out.Trips {
		if trip.Direct {
			directs++
		}
	}
	if directs == 0 {
		t.Error("no trip marked direct; fixture contains direct: uid rides")
	}
	if out.FromCity == "" || out.ToCity == "" {
		t.Errorf("from/to city names not resolved: %+v", out)
	}
}

func TestSearchTripsBookingURLKeepsPassengers(t *testing.T) {
	d := testDeps(t)
	_, out, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20",
		Adults: 2, Children: 1, Bikes: 1, MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	for _, want := range []string{"adult=2", "children=1", "bike_slot=1", "currency=EUR"} {
		if !strings.Contains(out.BookingURL, want) {
			t.Errorf("BookingURL %q missing %q", out.BookingURL, want)
		}
	}
}

func TestSearchTripsMaxResults(t *testing.T) {
	d := testDeps(t)
	_, out, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20", MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	if len(out.Trips) != 1 {
		t.Errorf("len(trips) = %d, want 1", len(out.Trips))
	}
}

func TestSearchTripsFeeNotRequired(t *testing.T) {
	// when the fee is not required in the price, the base total wins even
	// though a fee-inclusive total is present
	d := &tools.Deps{
		Client: stubClient(t, []byte(`{
		  "trips": [{"results": {"direct:x": {
		    "uid": "direct:x", "status": "available", "transfer_type": "Direct",
		    "departure": {"date": "2026-07-20T07:15:00+02:00", "city_id": "`+berlinUUID+`", "station_id": "s1"},
		    "arrival": {"date": "2026-07-20T13:00:00+02:00", "city_id": "`+munichUUID+`", "station_id": "s2"},
		    "duration": {"hours": 5, "minutes": 45},
		    "price": {"total": 39.99, "total_with_platform_fee": 40.98},
		    "available": {"seats": 5}
		  }}}],
		  "cities": {}, "stations": {},
		  "platform_fee_in_price_required": false
		}`)),
		Cfg: config.Config{Currency: "EUR", Locale: "en_US"},
	}
	_, out, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20",
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	if out.Trips[0].Price != 39.99 {
		t.Errorf("Price = %v, want base total 39.99 when fee not required", out.Trips[0].Price)
	}
	if !out.Trips[0].Direct {
		t.Error("uid prefix direct: must mark the trip direct")
	}
}

func TestSearchTripsShapeDriftFailsLoud(t *testing.T) {
	// an available trip with no price means the API renamed fields; the tool
	// must error instead of quoting 0.00
	d := &tools.Deps{
		Client: stubClient(t, []byte(`{
		  "trips": [{"results": {"ic:y": {
		    "uid": "ic:y", "status": "available",
		    "departure": {"date": "2026-07-20T07:15:00+02:00"},
		    "arrival": {"date": "2026-07-20T13:00:00+02:00"},
		    "price": {}
		  }}}],
		  "cities": {}, "platform_fee_in_price_required": true
		}`)),
		Cfg: config.Config{Currency: "EUR", Locale: "en_US"},
	}
	_, _, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20",
	})
	if err == nil || !strings.Contains(err.Error(), "shape") {
		t.Fatalf("want response-shape error, got: %v", err)
	}
}

func TestSearchCities(t *testing.T) {
	d := testDeps(t)
	_, out, err := tools.SearchCitiesHandler(d, context.Background(), nil, tools.SearchCitiesIn{Query: "berlin"})
	if err != nil {
		t.Fatalf("searchCities: %v", err)
	}
	if len(out.Cities) == 0 || out.Cities[0].ID != berlinUUID {
		t.Errorf("cities = %+v", out.Cities)
	}
	if _, _, err := tools.SearchCitiesHandler(d, context.Background(), nil, tools.SearchCitiesIn{}); err == nil {
		t.Error("want error for empty query")
	}
}

func TestSearchTripsEmptyResultsMarshalEmptySlice(t *testing.T) {
	d := &tools.Deps{
		Client: stubClient(t, []byte(`{"trips":[],"cities":{}}`)),
		Cfg:    config.Config{Currency: "EUR", Locale: "en_US"},
	}
	_, out, err := tools.SearchTripsHandler(d, context.Background(), nil, tools.SearchTripsIn{
		From: "Berlin", To: munichUUID, Date: "2026-07-20",
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	if out.Trips == nil {
		t.Error("Trips is nil; zero results must marshal as [] like search_cities")
	}
	if len(out.Trips) != 0 {
		t.Errorf("len(trips) = %d, want 0", len(out.Trips))
	}
}

func TestLangFromLocale(t *testing.T) {
	cases := map[string]string{
		"en_US": "en",
		"de_DE": "de",
		"en-US": "en", // hyphen form normalized
		"en":    "en",
		"":      "en",
	}
	for locale, want := range cases {
		if got := tools.LangFromLocale(locale); got != want {
			t.Errorf("langFromLocale(%q) = %q, want %q", locale, got, want)
		}
	}
}
