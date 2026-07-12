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

// langFromLocale reduces a locale (en_US or en-US) to the short ISO 639-1
// code the autocomplete endpoint expects (en).
func langFromLocale(locale string) string {
	lang, _, _ := strings.Cut(strings.ReplaceAll(locale, "-", "_"), "_")
	if lang == "" {
		return "en"
	}
	return lang
}

// resolveCity turns a user-supplied city name or UUID into a city id plus
// display name. UUIDs pass through without a network call (name comes back
// empty). Names resolve via autocomplete in the given language: a UNIQUE
// exact case-insensitive match wins; else a single bookable candidate wins;
// else the caller gets an error listing candidates — including when several
// distinct cities share the exact name (Birmingham GB vs Birmingham US).
func resolveCity(ctx context.Context, c *flixbus.Client, input, lang string) (id, name string, err error) {
	input = strings.TrimSpace(input)
	if uuidRe.MatchString(input) {
		return input, "", nil
	}
	all, err := c.AutocompleteCities(ctx, input, lang)
	if err != nil {
		return "", "", err
	}
	cities := make([]flixbus.City, 0, len(all))
	for _, city := range all {
		if city.IsFlixbusCity {
			cities = append(cities, city)
		}
	}
	if len(cities) == 0 {
		return "", "", fmt.Errorf("no FlixBus city found for %q", input)
	}
	var exact []flixbus.City
	for _, city := range cities {
		if strings.EqualFold(city.Name, input) {
			exact = append(exact, city)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0].ID, exact[0].Name, nil
	case len(exact) > 1:
		cities = exact // ambiguous among the exact matches
	case len(cities) == 1:
		return cities[0].ID, cities[0].Name, nil
	}
	names := make([]string, 0, maxCandidates)
	for _, city := range cities[:min(len(cities), maxCandidates)] {
		names = append(names, fmt.Sprintf("%s, %s (%s)", city.Name, strings.ToUpper(city.Country), city.ID))
	}
	return "", "", fmt.Errorf("ambiguous city %q; pass one of these ids: %s", input, strings.Join(names, "; "))
}
