// Package flixbus is a minimal client for FlixBus's unofficial public JSON API.
package flixbus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const userAgent = "flixbus-mcp/0.1"

// Client calls global.api.flixbus.com. Safe for concurrent use.
type Client struct {
	hc      *http.Client
	baseURL string
	apiKey  string
}

// New returns a Client. baseURL has no trailing slash; apiKey may be empty
// (the API needs none today — sent as X-API-Key if provided).
func New(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		hc:      &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

// AutocompleteCities searches cities by (partial) name. lang is a short ISO
// 639-1 code like "en" — unlike search, which takes an underscore locale.
func (c *Client) AutocompleteCities(ctx context.Context, query, lang string) ([]City, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("lang", lang)
	var cities []City
	if err := c.getJSON(ctx, "/search/autocomplete/cities", q, &cities); err != nil {
		return nil, fmt.Errorf("autocomplete %q: %w", query, err)
	}
	return cities, nil
}

// SearchTrips runs a v4 city-to-city search.
func (c *Client) SearchTrips(ctx context.Context, p SearchParams) (*SearchResponse, error) {
	products := map[string]int{"adult": p.Adults}
	if p.Children > 0 {
		products["children"] = p.Children
	}
	if p.Bikes > 0 {
		products["bike_slot"] = p.Bikes
	}
	pj, err := json.Marshal(products)
	if err != nil {
		return nil, fmt.Errorf("marshal products: %w", err)
	}
	q := url.Values{}
	q.Set("from_city_id", p.FromCityID)
	q.Set("to_city_id", p.ToCityID)
	q.Set("departure_date", p.DepartureDate)
	q.Set("products", string(pj))
	q.Set("currency", p.Currency)
	q.Set("locale", p.Locale)
	q.Set("search_by", "cities")
	q.Set("include_after_midnight_rides", "1")
	var out SearchResponse
	if err := c.getJSON(ctx, "/search/service/v4/search", q, &out); err != nil {
		return nil, fmt.Errorf("search trips: %w", err)
	}
	return &out, nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "warn: close response body: %v\n", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// apiError extracts the API's own {code,message} body when present.
func apiError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("api http %d (unreadable body: %w)", resp.StatusCode, err)
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return fmt.Errorf("api http %d: %s", resp.StatusCode, e.Message)
	}
	return fmt.Errorf("api http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
