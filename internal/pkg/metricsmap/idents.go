package metricsmap

import (
	"errors"
	"fmt"
	"slices"
)

// identClaim is one identifier the generated collector package holds, and what
// put it there: a declaration of the map (metric plus role), or the package
// itself (metric empty).
type identClaim struct {
	ident  string
	metric string
	role   string
}

// Roles of a generated identifier, as they read in a collision message.
const (
	roleVariable   = "variable"
	roleSampleType = "sample type"
	roleSourceType = "source type"
)

// ErrIdentCollision reports two declarations whose generated identifiers
// collide, or one that lands on an identifier the generated collector package
// declares for itself.
var ErrIdentCollision = errors.New("metric identifier collision")

// metricsPackageIdents is the exported identifier surface the generated
// collector package declares for itself
// (templater/embedded/templates/metrics/metrics.go.tmpl).
//
// Every identifier a declaration generates is exported — folding the
// separators of a Prometheus name away upper-cases the first character — so a
// declaration can land on one of these, and what it produces then is a package
// that redeclares Register or GaugeSet: the generator prints `done` and the
// project does not compile. The unexported identifiers of the package
// (collectorsByApp, gaugeSetSample) and the local names of its imports are out
// of reach for the same reason, which is why they are not listed.
//
// assertDeclarationsCollideWithNothing (test/metrics_map_state_family_test.go)
// reads the identifiers back out of a generated package and fails when one of
// them is missing here.
//
//nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
var metricsPackageIdents = []string{
	"Collectors",
	"CollectorsFor",
	"ErrUnknownApp",
	"Register",
	"RegisterFor",
	"Unregister",
}

// gaugeSetPackageIdents is the part of that surface emitted only for a map
// holding at least one state family. A map without one may well declare a
// metric named `gauge_set`: nothing is generated for it to collide with, and
// reserving the name regardless would reject a map the generator handles
// correctly today.
//
//nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
var gaugeSetPackageIdents = []string{gaugeSetType}

// ReservedIdents returns the identifiers the generated collector package
// declares for itself, sorted. No declaration of the map may generate one of
// them. hasGaugeSet selects whether the state-family machinery is part of the
// package, because it is emitted only for a map that declares a state family.
//
// Exported for the test that keeps the list and the collector template in step.
func ReservedIdents(hasGaugeSet bool) []string {
	out := slices.Clone(metricsPackageIdents)

	if hasGaugeSet {
		out = append(out, gaugeSetPackageIdents...)
	}

	slices.Sort(out)

	return out
}

// collidesWith renders the message for a claim landing on an identifier
// already taken by prev.
func (c identClaim) collidesWith(prev identClaim) string {
	switch {
	case prev.metric == "":
		return fmt.Sprintf(
			"%q generates the %s %s, which the generated collector package declares itself; rename the metric",
			c.metric, c.role, c.ident)
	case prev.role == c.role:
		return fmt.Sprintf("%q and %q both generate the %s %s", prev.metric, c.metric, c.role, c.ident)
	default:
		return fmt.Sprintf("%q generates the %s %s, which is already the %s of %q",
			c.metric, c.role, c.ident, prev.role, prev.metric)
	}
}

// identClaims returns every identifier the declaration puts into the generated
// package. A plain declaration generates one variable; a state family also
// generates the struct of its samples and the type of its source, so the name
// of one declaration can reach the types of another ("fleet_state_sample"
// against the sample type of "fleet_state").
func (m Metric) identClaims() []identClaim {
	claims := []identClaim{{ident: m.VarName(), metric: m.Name, role: roleVariable}}

	if m.IsGaugeSet() {
		claims = append(claims,
			identClaim{ident: m.SampleTypeName(), metric: m.Name, role: roleSampleType},
			identClaim{ident: m.SourceTypeName(), metric: m.Name, role: roleSourceType},
		)
	}

	return claims
}

// ValidateIdents checks that the declarations fill the identifier namespace of
// the generated collector package without colliding — with each other or with
// what the package declares for itself.
//
// Called on the merged set, because a collision can be introduced by a service
// name meeting an internal one. The whole namespace is checked rather than the
// collector variables alone: a state family generates two types besides its
// variable, and an identifier declared twice is a package the service did not
// write and cannot compile, reported by the generator as `done`.
func ValidateIdents(metrics []Metric) error {
	hasGaugeSet := false

	for _, metric := range metrics {
		if metric.Generated() && metric.IsGaugeSet() {
			hasGaugeSet = true

			break
		}
	}

	owner := make(map[string]identClaim, len(metrics)+len(metricsPackageIdents)+1)

	for _, ident := range ReservedIdents(hasGaugeSet) {
		owner[ident] = identClaim{ident: ident}
	}

	var problems []string

	for _, metric := range metrics {
		// An external or generator-owned declaration produces nothing in this
		// package, so it claims no identifier in it.
		if !metric.Generated() {
			continue
		}

		for _, claim := range metric.identClaims() {
			prev, taken := owner[claim.ident]
			if taken {
				// The first claimant keeps the identifier: reporting the
				// second one against it is what names a declaration the reader
				// can go and rename.
				problems = append(problems, claim.collidesWith(prev))

				continue
			}

			owner[claim.ident] = claim
		}
	}

	if len(problems) > 0 {
		return problemList(ErrIdentCollision, problems)
	}

	return nil
}
