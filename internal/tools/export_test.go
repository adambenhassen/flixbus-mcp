package tools

// Hooks for the external tools_test package to reach unexported handlers.
var (
	ResolveCity         = resolveCity
	SearchCitiesHandler = (*Deps).searchCities
	SearchTripsHandler  = (*Deps).searchTrips
	LangFromLocale      = langFromLocale
)
