package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
)

const berlinUUID = "40d8f682-8646-11e6-9066-549f350fcb0c"

func autocompleteStub(t *testing.T, body string) *flixbus.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return flixbus.New(srv.URL, "", 5*time.Second)
}

func TestResolveCityUUIDPassthrough(t *testing.T) {
	// no server needed: UUID must not hit the network
	c := flixbus.New("http://127.0.0.1:1", "", time.Second)
	got, err := resolveCity(context.Background(), c, berlinUUID)
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if got.ID != berlinUUID {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestResolveCityExactMatchWins(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-suburb","name":"Berlin Wannsee","country":"de","score":90,"is_flixbus_city":true},
	  {"id":"`+berlinUUID+`","name":"Berlin","country":"de","score":60,"is_flixbus_city":true}
	]`)
	got, err := resolveCity(context.Background(), c, "berlin")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if got.ID != berlinUUID {
		t.Errorf("resolved %q (%s), want exact match Berlin", got.Name, got.ID)
	}
}

func TestResolveCitySingleResult(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-x","name":"Villach","country":"at","score":50,"is_flixbus_city":true}
	]`)
	got, err := resolveCity(context.Background(), c, "villa")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if got.ID != "uuid-x" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestResolveCityAmbiguous(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-a","name":"Springfield IL","country":"us","score":50,"is_flixbus_city":true},
	  {"id":"uuid-b","name":"Springfield MA","country":"us","score":49,"is_flixbus_city":true}
	]`)
	_, err := resolveCity(context.Background(), c, "springfield")
	if err == nil {
		t.Fatal("want ambiguity error")
	}
	if !strings.Contains(err.Error(), "uuid-a") || !strings.Contains(err.Error(), "uuid-b") {
		t.Errorf("error should list candidate UUIDs, got: %v", err)
	}
}

func TestResolveCityNoMatch(t *testing.T) {
	c := autocompleteStub(t, `[]`)
	_, err := resolveCity(context.Background(), c, "atlantis")
	if err == nil || !strings.Contains(err.Error(), "atlantis") {
		t.Fatalf("want no-match error naming the query, got: %v", err)
	}
}

func TestResolveCityFiltersNonFlixbus(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-no","name":"Nowhere","country":"de","score":90,"is_flixbus_city":false},
	  {"id":"uuid-yes","name":"Somewhere","country":"de","score":50,"is_flixbus_city":true}
	]`)
	got, err := resolveCity(context.Background(), c, "where")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if got.ID != "uuid-yes" {
		t.Errorf("ID = %q, want the is_flixbus_city one", got.ID)
	}
}
