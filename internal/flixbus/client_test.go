package flixbus_test

import (
	"context"

	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureServer(t *testing.T, path, fixture string, wantQuery map[string]string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		for k, v := range wantQuery {
			if got := r.URL.Query().Get(k); got != v {
				t.Errorf("query %s = %q, want %q", k, got, v)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAutocompleteCities(t *testing.T) {
	srv := fixtureServer(t, "/search/autocomplete/cities",
		"testdata/autocomplete_berlin.json", map[string]string{"q": "berlin", "lang": "en"})
	c := flixbus.New(srv.URL, "", 5*time.Second)
	cities, err := c.AutocompleteCities(context.Background(), "berlin", "en")
	if err != nil {
		t.Fatalf("AutocompleteCities: %v", err)
	}
	if len(cities) == 0 {
		t.Fatal("no cities decoded")
	}
	top := cities[0]
	if top.Name == "" || top.ID == "" || top.Country == "" {
		t.Errorf("first city missing fields: %+v", top)
	}
	if top.Score <= 0 {
		t.Errorf("Score = %v, want > 0", top.Score)
	}
}

func TestSearchTrips(t *testing.T) {
	srv := fixtureServer(t, "/search/service/v4/search",
		"testdata/search_berlin_munich.json", map[string]string{
			"from_city_id":   "40d8f682-8646-11e6-9066-549f350fcb0c",
			"to_city_id":     "40d901a5-8646-11e6-9066-549f350fcb0c",
			"departure_date": "20.07.2026",
			"products":       `{"adult":1}`,
			"currency":       "EUR",
			"locale":         "en_US",
			"search_by":      "cities",
		})
	c := flixbus.New(srv.URL, "", 5*time.Second)
	resp, err := c.SearchTrips(context.Background(), flixbus.SearchParams{
		FromCityID:    "40d8f682-8646-11e6-9066-549f350fcb0c",
		ToCityID:      "40d901a5-8646-11e6-9066-549f350fcb0c",
		DepartureDate: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		Adults:        1,
		Currency:      "EUR",
		Locale:        "en_US",
	})
	if err != nil {
		t.Fatalf("SearchTrips: %v", err)
	}
	if len(resp.Trips) == 0 {
		t.Fatal("no trips decoded")
	}
	if len(resp.Trips[0].Results) == 0 {
		t.Fatal("trips[0].results decoded empty — map decode broken")
	}
	for uid, r := range resp.Trips[0].Results {
		if r.Departure.Date == "" || r.Arrival.Date == "" {
			t.Errorf("result %s: empty departure/arrival date", uid)
		}
		if r.Price.Total <= 0 && r.Status == "available" {
			t.Errorf("result %s: available but price %v", uid, r.Price.Total)
		}
	}
	if _, ok := resp.Cities["40d8f682-8646-11e6-9066-549f350fcb0c"]; !ok {
		t.Error("cities map missing Berlin UUID")
	}
}

func TestAPIError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring the returned error must carry
	}{
		{"json message", `{"code":400,"message":"To city id X was not found!"}`, "To city id X was not found!"},
		{"plain text fallback", `service unavailable, try later`, "service unavailable, try later"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Errorf("write: %v", err)
				}
			}))
			t.Cleanup(srv.Close)
			c := flixbus.New(srv.URL, "", 5*time.Second)
			_, err := c.SearchTrips(context.Background(), flixbus.SearchParams{Adults: 1})
			if err == nil {
				t.Fatal("want error on 400")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}
