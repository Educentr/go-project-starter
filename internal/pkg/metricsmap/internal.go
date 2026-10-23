package metricsmap

import (
	"embed"
	"fmt"
	"sync"
)

const (
	internalMapPath = "embedded/internal-metrics.yaml"

	// OriginInternal names the generator's own declarations in merge errors.
	OriginInternal = "go-project-starter internal metrics"
	// OriginService names the service's declarations in merge errors.
	OriginService = "service metrics map"

	// Kafka producer metric names. Referenced by the Kafka driver templates so
	// that the name and help text live in the internal map only.
	KafkaPublishTotal    = "kafka_producer_publish_total"
	KafkaPublishErrors   = "kafka_producer_publish_errors_total"
	KafkaPublishDuration = "kafka_producer_publish_duration_seconds"
)

//go:embed embedded/internal-metrics.yaml
var embedded embed.FS

// Memo of the embedded internal metrics map. The input is an embed.FS fixed at
// compile time, so the parsed result is the same for the whole process — this is
// a read-through cache over an immutable input, not shared mutable state.
var (
	internalOnce   sync.Once //nolint:gochecknoglobals // memo over the embedded FS, see above
	internalMap    *Map      //nolint:gochecknoglobals // memo over the embedded FS, see above
	errInternalMap error
)

// Internal returns the generator's own metric declarations. The embedded map is
// parsed and validated once; a failure here is a defect in this repository, not
// in the user's project, which is why the error carries that framing.
func Internal() (*Map, error) {
	internalOnce.Do(func() {
		data, err := embedded.ReadFile(internalMapPath)
		if err != nil {
			errInternalMap = fmt.Errorf("%w: read embedded %s: %w", ErrInternalMapDefect, internalMapPath, err)

			return
		}

		internalMap, err = Parse(data)
		if err != nil {
			errInternalMap = fmt.Errorf("%w: embedded %s is invalid: %w", ErrInternalMapDefect, internalMapPath, err)
		}
	})

	return internalMap, errInternalMap
}

// Find returns the declaration with the given name, or false when it is not
// declared.
func (m *Map) Find(name string) (Metric, bool) {
	if m == nil {
		return Metric{}, false
	}

	for _, metric := range m.Metrics {
		if metric.Name == name {
			return metric, true
		}
	}

	return Metric{}, false
}

// MustFindInternal returns an internal declaration by name. It is meant for
// generator code wiring a known internal metric into a template; a missing name
// means this repository's embedded map and its Go callers drifted apart.
func MustFindInternal(name string) (Metric, error) {
	internal, err := Internal()
	if err != nil {
		return Metric{}, err
	}

	metric, ok := internal.Find(name)
	if !ok {
		return Metric{}, fmt.Errorf("%w: metric %q is not declared in %s", ErrInternalMapDefect, name, internalMapPath)
	}

	return metric, nil
}

// MergeWithInternal folds the service map into the generator's own namespace,
// keeping only the internal declarations relevant to a project holding the
// given capabilities. A nil service map yields the applicable internal
// declarations alone.
//
// Filtering the internal set matters in both directions: a metric nothing in
// the project can emit must not reserve its name against the service, and must
// not get a dashboard panel that can only ever read empty.
func MergeWithInternal(service *Map, capabilities map[string]bool) ([]Metric, error) {
	internal, err := Internal()
	if err != nil {
		return nil, err
	}

	applicable := make([]Metric, 0, len(internal.Metrics))

	for _, metric := range internal.Metrics {
		if metric.Applies(capabilities) {
			applicable = append(applicable, metric)
		}
	}

	sets := []Set{{Origin: OriginInternal, Metrics: applicable}}

	if service != nil {
		sets = append(sets, Set{Origin: OriginService, Metrics: service.Metrics})
	}

	return Merge(sets...)
}
