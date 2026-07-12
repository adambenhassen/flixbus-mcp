package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
	"github.com/adambenhassen/flixbus-mcp/internal/tools"
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
	id, name, err := tools.ResolveCity(context.Background(), c, berlinUUID, "en")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if id != berlinUUID {
		t.Errorf("id = %q", id)
	}
	if name != "" {
		t.Errorf("name = %q, want empty on UUID path", name)
	}
}

func TestResolveCityExactMatchWins(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-suburb","name":"Berlin Wannsee","country":"de","score":90,"is_flixbus_city":true},
	  {"id":"`+berlinUUID+`","name":"Berlin","country":"de","score":60,"is_flixbus_city":true}
	]`)
	id, name, err := tools.ResolveCity(context.Background(), c, "berlin", "en")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if id != berlinUUID || name != "Berlin" {
		t.Errorf("resolved %q (%s), want exact match Berlin", name, id)
	}
}

func TestResolveCitySingleResult(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-x","name":"Villach","country":"at","score":50,"is_flixbus_city":true}
	]`)
	id, name, err := tools.ResolveCity(context.Background(), c, "villa", "en")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if id != "uuid-x" || name != "Villach" {
		t.Errorf("id = %q, name = %q", id, name)
	}
}

func TestResolveCityAmbiguous(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-a","name":"Springfield IL","country":"us","score":50,"is_flixbus_city":true},
	  {"id":"uuid-b","name":"Springfield MA","country":"us","score":49,"is_flixbus_city":true}
	]`)
	_, _, err := tools.ResolveCity(context.Background(), c, "springfield", "en")
	if err == nil {
		t.Fatal("want ambiguity error")
	}
	if !strings.Contains(err.Error(), "uuid-a") || !strings.Contains(err.Error(), "uuid-b") {
		t.Errorf("error should list candidate UUIDs, got: %v", err)
	}
}

func TestResolveCityNoMatch(t *testing.T) {
	c := autocompleteStub(t, `[]`)
	_, _, err := tools.ResolveCity(context.Background(), c, "atlantis", "en")
	if err == nil || !strings.Contains(err.Error(), "atlantis") {
		t.Fatalf("want no-match error naming the query, got: %v", err)
	}
}

func TestResolveCityFiltersNonFlixbus(t *testing.T) {
	c := autocompleteStub(t, `[
	  {"id":"uuid-no","name":"Nowhere","country":"de","score":90,"is_flixbus_city":false},
	  {"id":"uuid-yes","name":"Somewhere","country":"de","score":50,"is_flixbus_city":true}
	]`)
	id, _, err := tools.ResolveCity(context.Background(), c, "where", "en")
	if err != nil {
		t.Fatalf("resolveCity: %v", err)
	}
	if id != "uuid-yes" {
		t.Errorf("id = %q, want the is_flixbus_city one", id)
	}
}
