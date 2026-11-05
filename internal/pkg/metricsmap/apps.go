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
	// whether it has a sys transport. It decides who gets the DEFAULT surface
	// (a declaration with no `apps`) and whether the generated main registers
	// anything at all — not whether the application may be named in `apps`.
	ExposesMetrics bool
	// Components holds the generator components the application runs.
	Components map[string]bool
}

// ErrUnknownApp reports an `apps` entry that names no application of the
// project.
var ErrUnknownApp = errors.New("unknown application in apps")

// Publishes reports whether the application publishes the declaration — all
// three halves of the question: whether the declaration names the application,
// whether the application has a /metrics endpoint to put the default surface
// on, and, for a declaration the generator owns, whether this application runs
// the component that constructs it.
//
// A declaration that NAMES applications is published by them whether or not
// they expose /metrics. An application without a sys transport has no endpoint
// for the generated main to register into, but it can still hold its own
// registry and register the collectors from its own code — which is the case
// the field exists for: a CLI whose instruments live in package-private
// registries deliberately.
//
// A declaration that names nobody keeps its pre-field meaning, "every
// application that exposes /metrics". Widening that to processes without an
// endpoint would push the service's server-side collectors into such a private
// registry, which is the inverse of the defect the field removes.
func (s AppScope) Publishes(m Metric) bool {
	if m.Owner != "" && !s.Components[m.Owner] {
		return false
	}

	if m.NamesApp(s.Name) {
		return true
	}

	return len(m.Apps) == 0 && s.ExposesMetrics
}

// NamesApp reports whether the declaration's `apps` list names the
// application. A declaration with no `apps` names nobody: it is published by
// default instead — see AppScope.Publishes.
func (m Metric) NamesApp(app string) bool {
	return slices.Contains(m.Apps, app)
}

// ValidateApps checks every `apps` reference against the project's actual
// applications. A name that matches nothing is a typo, and a typo that is
// tolerated moves the metric nowhere while reading as if it had — so it is a
// generation error, not a warning.
//
// A name that matches an application WITHOUT a sys transport is not an error:
// see AppScope.Publishes for what such an entry means. The only thing a
// reference can get wrong is the spelling.
//
// Problems name the metric, not its index: unlike the rest of the package's
// validation this runs on the MERGED set, where position counts the
// generator's own declarations first and so points at a line the service's map
// does not have.
//
// All problems are reported at once, for the same reason the rest of the map's
// validation does: one round trip to fix a bad map.
func ValidateApps(metrics []Metric, scopes []AppScope) error {
	known := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		known[scope.Name] = struct{}{}
	}

	hint := knownAppsHint(scopes)

	var problems []string

	for _, metric := range metrics {
		for _, app := range metric.Apps {
			if _, ok := known[app]; !ok {
				problems = append(problems, fmt.Sprintf(
					"metric %q: apps names %q, which is not an application of this project%s",
					metric.Name, app, hint))
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
