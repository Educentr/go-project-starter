package ds

// MetricsExport is the export channel of an application — the way the metrics
// it collects leave the process.
//
// It says nothing about whether metrics are collected: the registry is
// initialized in every application, whatever its transports, because the code
// that writes metrics (the REST middleware, the generated collectors,
// ActiveRecord) is handed it on initialization and has to get a working one.
// The channel decides only whether anything reads them.
type MetricsExport string

const (
	// MetricsExportPull serves the metrics over /metrics on the application's
	// sys transport, for a scraper to pull. The sys transport is what makes
	// this channel possible, so it is also what the generator derives the
	// channel from when the project declares nothing.
	MetricsExportPull MetricsExport = "pull"

	// MetricsExportPush sends the metrics to an external receiver. The value is
	// part of the grammar and not implemented yet: reserving it means the
	// implementation arrives without changing the shape of project.yaml.
	MetricsExportPush MetricsExport = "push"

	// MetricsExportNone collects metrics and never hands them out. This is a
	// legitimate configuration, not a degraded one: a process running on a
	// user's machine has no scraper that could reach it, and a listening port
	// nobody reads is surface without a reader.
	MetricsExportNone MetricsExport = "none"
)

// IsPull reports whether the application exposes /metrics on its sys transport.
func (e MetricsExport) IsPull() bool {
	return e == MetricsExportPull
}

// IsPush reports whether the application pushes metrics to an external
// receiver.
func (e MetricsExport) IsPush() bool {
	return e == MetricsExportPush
}

// IsNone reports whether the metrics of the application stay inside the
// process.
func (e MetricsExport) IsNone() bool {
	return e == MetricsExportNone
}

// String returns the declared name of the channel.
func (e MetricsExport) String() string {
	return string(e)
}

// Comment returns the one-line description the generated main carries next to
// the metrics initialization, so that the resolved channel is visible in the
// generated project and not only in project.yaml.
func (e MetricsExport) Comment() string {
	switch e {
	case MetricsExportPull:
		return "pull — served on /metrics by the sys transport of this application"
	case MetricsExportPush:
		return "push — sent to an external receiver"
	case MetricsExportNone:
		return "none — collected and never handed out; nothing reads this registry"
	default:
		return string(e)
	}
}
