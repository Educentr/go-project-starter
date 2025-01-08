package reqctx

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ContextCumulativeMetric struct {
	collector prometheus.Collector
	metrics   map[string]*float64
	sync.Mutex
}

var (
	defaultMetricsCount = 10
)

func NewContextCumulativeMetric(collector prometheus.Collector) *ContextCumulativeMetric {
	return &ContextCumulativeMetric{
		collector: collector,
		metrics:   make(map[string]*float64, defaultMetricsCount),
	}
}

func (ccm *ContextCumulativeMetric) IncMetric(name string, diff int32) {
	ccm.Mutex.Lock()
	if ccm.metrics[name] == nil {
		newMet := float64(0)
		ccm.metrics[name] = &newMet
	}

	*ccm.metrics[name] += float64(diff)

	ccm.Mutex.Unlock()
}

func (ccm *ContextCumulativeMetric) TimeMetric(name string, dur time.Duration) {
	ccm.Mutex.Lock()
	if ccm.metrics[name] == nil {
		newMet := float64(0)
		ccm.metrics[name] = &newMet
	}

	*ccm.metrics[name] += dur.Seconds()

	ccm.Mutex.Unlock()
}

func (ccm *ContextCumulativeMetric) FlushMetric(requestName string, labels ...string) {
	allLabels := []string{requestName}
	allLabels = append(allLabels, labels...)

	for name, cumulativeVal := range ccm.metrics {
		allLabels := append(allLabels, name)

		switch coll := ccm.collector.(type) {
		case *prometheus.HistogramVec:
			coll.WithLabelValues(allLabels...).Observe(*cumulativeVal)
		case *prometheus.CounterVec:
			coll.WithLabelValues(allLabels...).Add(*cumulativeVal)
		}
	}
}
