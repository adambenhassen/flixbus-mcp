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

// resolveCity turns a user-supplied city name or UUID into a flixbus.City.
// UUIDs pass through without a network call (Name stays empty). Names resolve
// via autocomplete: an exact case-insensitive match wins; else a single
// bookable candidate wins; else the caller gets an error listing candidates.
func resolveCity(ctx context.Context, c *flixbus.Client, input string) (flixbus.City, error) {
	input = strings.TrimSpace(input)
	if uuidRe.MatchString(input) {
		return flixbus.City{ID: input}, nil
	}
	all, err := c.AutocompleteCities(ctx, input, "en")
	if err != nil {
		return flixbus.City{}, err
	}
	var cities []flixbus.City
	for _, city := range all {
		if city.IsFlixbusCity {
			cities = append(cities, city)
		}
	}
	if len(cities) == 0 {
		return flixbus.City{}, fmt.Errorf("no FlixBus city found for %q", input)
	}
	for _, city := range cities {
		if strings.EqualFold(city.Name, input) {
			return city, nil
		}
	}
	if len(cities) == 1 {
		return cities[0], nil
	}
	names := make([]string, 0, maxCandidates)
	for _, city := range cities[:min(len(cities), maxCandidates)] {
		names = append(names, fmt.Sprintf("%s, %s (%s)", city.Name, strings.ToUpper(city.Country), city.ID))
	}
	return flixbus.City{}, fmt.Errorf("ambiguous city %q; pass one of these ids: %s", input, strings.Join(names, "; "))
}
