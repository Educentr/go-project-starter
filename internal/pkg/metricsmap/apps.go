package metricsmap

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// AppScope is what the metrics map needs to know about one application of the
// project: its name, whether it exposes /metrics, and which generator
// components it runs.
//
// Components is keyed by the Owner* constants and answers the half of "who
// publishes this" that the map cannot state: a declaration owned by a
// generator component is constructed by the processes that run that component,
// not by every process of the service.
type AppScope struct {
	// Name is the application name as written in project.yaml.
	Name string
	// ExposesMetrics reports whether the application serves /metrics, i.e.
	// whether it has a sys transport. An application that does not has no
	// registry and registers nothing.
	ExposesMetrics bool
	// Components holds the generator components the application runs.
	Components map[string]bool
}

// ErrUnknownApp reports an `apps` entry that names no application of the
// project, or one that serves no /metrics endpoint to register into.
var ErrUnknownApp = errors.New("unknown application in apps")

// Publishes reports whether the application publishes the declaration — both
// halves of the question: the service's own `apps` list, and, for a
// declaration the generator owns, whether this application runs the component
// that constructs it.
func (s AppScope) Publishes(m Metric) bool {
	if !m.PublishedBy(s.Name) {
		return false
	}

	if m.Owner != "" && !s.Components[m.Owner] {
		return false
	}

	return true
}

// PublishedBy reports whether the named application publishes the metric. A
// declaration with no `apps` is published by every application — see the Apps
// field for why that is the default.
func (m Metric) PublishedBy(app string) bool {
	if len(m.Apps) == 0 {
		return true
	}

	return slices.Contains(m.Apps, app)
}

// ValidateApps checks every `apps` reference against the project's actual
// applications. A name that matches nothing is a typo, and a typo that is
// tolerated moves the metric nowhere while reading as if it had — so it is a
// generation error, not a warning.
//
// Problems name the metric, not its index: unlike the rest of the package's
// validation this runs on the MERGED set, where position counts the
// generator's own declarations first and so points at a line the service's map
// does not have.
//
// All problems are reported at once, for the same reason the rest of the map's
// validation does: one round trip to fix a bad map.
func ValidateApps(metrics []Metric, scopes []AppScope) error {
	exposes := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		exposes[scope.Name] = scope.ExposesMetrics
	}

	hint := knownAppsHint(scopes)

	var problems []string

	for _, metric := range metrics {
		for _, app := range metric.Apps {
			exposing, known := exposes[app]

			switch {
			case !known:
				problems = append(problems, fmt.Sprintf(
					"metric %q: apps names %q, which is not an application of this project%s",
					metric.Name, app, hint))
			case !exposing:
				problems = append(problems, fmt.Sprintf(
					"metric %q: application %q has no sys transport, so it serves no /metrics endpoint to register into",
					metric.Name, app))
			}
		}
	}

	if len(problems) > 0 {
		return problemList(ErrUnknownApp, problems)
	}

	return nil
}

// knownAppsHint renders the project's application names for an error message,
// so that the reader sees the spelling they meant without opening project.yaml.
func knownAppsHint(scopes []AppScope) string {
	if len(scopes) == 0 {
		return ""
	}

	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, scope.Name)
	}

	sort.Strings(names)

	return " (declared applications: " + strings.Join(names, ", ") + ")"
}
