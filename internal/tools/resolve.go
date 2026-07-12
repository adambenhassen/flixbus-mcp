package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const maxCandidates = 5

// langFromLocale reduces an underscore locale (en_US) to the short ISO 639-1
// code the autocomplete endpoint expects (en).
func langFromLocale(locale string) string {
	lang, _, _ := strings.Cut(locale, "_")
	if lang == "" {
		return "en"
	}
	return lang
}

// resolveCity turns a user-supplied city name or UUID into a city id plus
// display name. UUIDs pass through without a network call (name comes back
// empty). Names resolve via autocomplete in the given language: an exact
// case-insensitive match wins; else a single bookable candidate wins; else
// the caller gets an error listing candidates.
func resolveCity(ctx context.Context, c *flixbus.Client, input, lang string) (id, name string, err error) {
	input = strings.TrimSpace(input)
	if uuidRe.MatchString(input) {
		return input, "", nil
	}
	all, err := c.AutocompleteCities(ctx, input, lang)
	if err != nil {
		return "", "", err
	}
	var cities []flixbus.City
	for _, city := range all {
		if city.IsFlixbusCity {
			cities = append(cities, city)
		}
	}
	if len(cities) == 0 {
		return "", "", fmt.Errorf("no FlixBus city found for %q", input)
	}
	for _, city := range cities {
		if strings.EqualFold(city.Name, input) {
			return city.ID, city.Name, nil
		}
	}
	if len(cities) == 1 {
		return cities[0].ID, cities[0].Name, nil
	}
	names := make([]string, 0, maxCandidates)
	for _, city := range cities[:min(len(cities), maxCandidates)] {
		names = append(names, fmt.Sprintf("%s, %s (%s)", city.Name, strings.ToUpper(city.Country), city.ID))
	}
	return "", "", fmt.Errorf("ambiguous city %q; pass one of these ids: %s", input, strings.Join(names, "; "))
}
