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
	"strconv"
	"strings"
	"time"
)

const userAgent = "flixbus-mcp/0.1"

// maxResponseBytes bounds how much of an API response the decoder will read,
// so a hostile or broken upstream cannot stream unbounded data into memory.
const maxResponseBytes = 8 << 20

// Product keys shared by the search API's products param and the shop deeplink.
const (
	productAdult    = "adult"
	productChildren = "children"
	productBikeSlot = "bike_slot"
)

// Client calls the FlixBus API at the configured base URL (default
// global.api.flixbus.com). Safe for concurrent use.
type Client struct {
	hc      *http.Client
	baseURL string
	apiKey  string
}

// New returns a Client. Trailing slashes on baseURL are stripped; apiKey may
// be empty (the API needs none today — sent as X-Api-Key if provided).
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

// SearchTrips runs a v4 city-to-city search. Adults must be at least 1.
func (c *Client) SearchTrips(ctx context.Context, p SearchParams) (*SearchResponse, error) {
	if p.Adults < 1 {
		return nil, fmt.Errorf("adults must be at least 1, got %d", p.Adults)
	}
	products := map[string]int{productAdult: p.Adults}
	if p.Children > 0 {
		products[productChildren] = p.Children
	}
	if p.Bikes > 0 {
		products[productBikeSlot] = p.Bikes
	}
	pj, err := json.Marshal(products)
	if err != nil {
		return nil, fmt.Errorf("marshal products: %w", err)
	}
	q := url.Values{}
	q.Set("from_city_id", p.FromCityID)
	q.Set("to_city_id", p.ToCityID)
	q.Set("departure_date", p.DepartureDate.Format(dateFormat))
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
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		return errorFromResponse(resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// closeBody drains up to 64KiB of leftover body so the connection can be
// reused (keep-alive), then closes it, warning on stderr since callers have
// nothing useful to do with either error.
func closeBody(resp *http.Response) {
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)); err != nil {
		fmt.Fprintf(os.Stderr, "warn: drain response body: %v\n", err)
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: close response body: %v\n", err)
	}
}

// BookingURL builds a shop.flixbus.com deeplink from the same params as a
// search, so the linked shop search matches the results.
func BookingURL(p SearchParams) string {
	q := url.Values{}
	q.Set("departureCity", p.FromCityID)
	q.Set("arrivalCity", p.ToCityID)
	q.Set("rideDate", p.DepartureDate.Format(dateFormat))
	q.Set(productAdult, strconv.Itoa(p.Adults))
	if p.Children > 0 {
		q.Set(productChildren, strconv.Itoa(p.Children))
	}
	if p.Bikes > 0 {
		q.Set(productBikeSlot, strconv.Itoa(p.Bikes))
	}
	if p.Currency != "" {
		q.Set("currency", p.Currency)
	}
	return "https://shop.flixbus.com/search?" + q.Encode()
}

// errorFromResponse extracts the API's own error message body when present.
func errorFromResponse(resp *http.Response) error {
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
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("api http %d: %s", resp.StatusCode, msg)
}
