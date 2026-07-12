package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/flixbus-mcp/internal/config"
	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
	"github.com/adambenhassen/flixbus-mcp/internal/tools"
)

const munichUUID = "40d901a5-8646-11e6-9066-549f350fcb0c"

// twoEndpointStub serves the autocomplete fixture and the recorded search fixture.
func twoEndpointStub(t *testing.T) *flixbus.Client {
	t.Helper()
	search, err := os.ReadFile("../flixbus/testdata/search_berlin_munich.json")
	if err != nil {
		t.Fatalf("read search fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body []byte
		switch r.URL.Path {
		case "/search/autocomplete/cities":
			body = []byte(`[{"id":"` + berlinUUID + `","name":"Berlin","country":"de","score":60,"is_flixbus_city":true}]`)
		case "/search/service/v4/search":
			body = search
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

func testDeps(t *testing.T) *tools.Deps {
	t.Helper()
	return &tools.Deps{Client: twoEndpointStub(t), Cfg: config.Config{Currency: "EUR", Locale: "en_US"}}
}

func TestSearchTripsValidation(t *testing.T) {
	d := testDeps(t)
	cases := []struct {
		name string
		in   tools.SearchTripsIn
		want string // substring of the expected error
	}{
		{"missing from", tools.SearchTripsIn{To: "Munich", Date: "2026-07-20"}, "from"},
		{"missing date", tools.SearchTripsIn{From: "Berlin", To: "Munich"}, "date"},
		{"bad date format", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "20.07.2026"}, "yyyy-mm-dd"},
		{"bad currency", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Currency: "euros"}, "currency"},
		{"negative adults", tools.SearchTripsIn{From: "Berlin", To: "Munich", Date: "2026-07-20", Adults: -1}, "adults"},
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
		From: "Berlin", To: munichUUID, Date: "2026-07-20",
	})
	if err != nil {
		t.Fatalf("searchTrips: %v", err)
	}
	if len(out.Trips) == 0 {
		t.Fatal("no trips returned")
	}
	for i := 1; i < len(out.Trips); i++ {
		if out.Trips[i-1].Departure > out.Trips[i].Departure {
			t.Errorf("trips not sorted by departure: %q > %q", out.Trips[i-1].Departure, out.Trips[i].Departure)
		}
	}
	first := out.Trips[0]
	if first.Departure == "" || first.Arrival == "" || first.Duration == "" {
		t.Errorf("missing fields: %+v", first)
	}
	if first.Currency != "EUR" {
		t.Errorf("Currency = %q", first.Currency)
	}
	if !strings.Contains(first.BookingURL, "shop.flixbus.com/search") ||
		!strings.Contains(first.BookingURL, "rideDate=20.07.2026") {
		t.Errorf("BookingURL = %q", first.BookingURL)
	}
	// fixture declares platform_fee_in_price_required; first trip's checkout
	// total is 78.97 (total 77.98 + fee) — the fee-inclusive price must win
	if first.Price != 78.97 {
		t.Errorf("Price = %v, want fee-inclusive 78.97", first.Price)
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
	u := out.Trips[0].BookingURL
	for _, want := range []string{"adult=2", "children=1", "bike_slot=1"} {
		if !strings.Contains(u, want) {
			t.Errorf("BookingURL %q missing %q", u, want)
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
