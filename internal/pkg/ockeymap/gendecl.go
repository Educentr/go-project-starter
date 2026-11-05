package ockeymap

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Declarations of the OnlineConf keys the GENERATOR's own code reads.
//
// Every generated project reads OnlineConf — transport timeouts, security
// toggles, log levels, worker and Kafka settings, the dev-stand flag — whether
// or not it declares a key map of its own. So the project's key map has two
// halves: the service's declarations, and these. Both are generated into one
// accessor package, both go into one registry, and a path in one half may not be
// declared in the other.
//
// A declaration lives next to the read it describes: every template whose
// generated code reads a key carries a `<template>.ockeys.yaml` beside it. That
// placement is the point — the file a developer has open while adding a read is
// the file that must grow an entry, and the templater tests fail when it does
// not.
//
// A declared path carries two kinds of variable segment:
//
//	{service}, {transport}, {app}, {worker}, {client}, {event}
//	    project dimensions, known at generation time. They become parameters of
//	    the generated accessor, because the generated readers are generic over
//	    them: the REST configuration provider is handed a server and an
//	    application name by the runtime, not by the generator.
//
//	<urlPath>, <tgID>, <action>, …
//	    segments the generated code computes at runtime. They become parameters
//	    too, and they make the generator the owner of the subtree they open: there
//	    is no single path to compare against, so the service map may not declare
//	    inside it.
//
// Reserving prefixes (`/svc/transport/**` and friends) was the first answer and
// the wrong one: the list was maintained by hand, nothing tied it to the
// templates, and subtrees the generated code genuinely reads — `worker/<name>`,
// `settings`, `bot` — were simply missing from it.

// GenKey is one declared read of the generated code.
type GenKey struct {
	// Path is the key path with dimension placeholders and runtime segments,
	// e.g. {service}/transport/rest/{transport}/{app}/handler/<urlPath>/timeout.
	// Required.
	Path string `yaml:"path"`
	// Name is the Go identifier of the generated accessor. Required, except on
	// an entry carrying ReadVia — that entry generates nothing.
	Name string `yaml:"name"`
	// Type is the value type, one of the Type* constants. An entry without one
	// is a path builder: generated code is handed the path and reads below it,
	// so the subtree belongs to the generator and the builder is what the
	// template calls.
	Type string `yaml:"type"`
	// Required marks a key the generated reader cannot work without. The
	// accessor then reports ErrKeyNotSet instead of a zero value, and the
	// startup check of every application reading it fails when it is absent.
	// Only a path with no runtime segment can be required: a path the generator
	// cannot enumerate is a path the startup check cannot verify.
	Required bool `yaml:"required"`
	// RequiredAtInit marks a key the generated code reads while the component it
	// belongs to is being brought up, and whose absence keeps that component —
	// and so the process — from starting. It generates nothing: no startup
	// check, no change of accessor shape. It is a fact published in the
	// registry, so that the guard of whoever seeds the tree can assert every
	// such path is set before a deploy discovers it at runtime, which is how the
	// host of a newly connected client used to be found missing.
	//
	// A `required` key is one of these by construction — its startup check IS
	// that failure — so the registry marks both and the two are not declared
	// together. What differs is who reports the absence: a `required` key is
	// verified by the generated startup check, a `required_at_init` one by the
	// component's own Init — which is where a key whose necessity depends on a
	// project.yaml setting has to be answered, because the check cannot tell
	// whether this installation needs the component at all.
	RequiredAtInit bool `yaml:"required_at_init"`
	// Shared marks an accessor whose path is composed entirely from segments its
	// caller supplies at runtime. It claims no subtree: the concrete paths it
	// reaches are declared by the readers that compose them, each carrying
	// ReadVia pointing back here. The log level is the one of these — the runtime
	// hands the logger a prefix, and the prefixes are the REST middleware's and
	// the workers', declared in their own scopes.
	Shared bool `yaml:"shared"`
	// ReadVia names the shared accessor this path is read through. The entry then
	// declares a concrete path for the registry and for the collision check, and
	// generates no accessor of its own — the reader already has one.
	ReadVia string `yaml:"read_via"`
	// Default is the fallback the generated reader applies, as prose for the
	// registry. It documents the key; nothing is generated from it.
	Default string `yaml:"default"`
	// Description says what the key does. Required: this is the text the owner
	// of the contract reads in the generated registry, and the only place the
	// meaning of a generator-owned key is written down.
	Description string `yaml:"description"`
}

// GenGroup is a set of declarations sharing one scope.
type GenGroup struct {
	// Scope names the project dimension the keys are read for. Required.
	Scope string `yaml:"scope"`
	// Keys are the declarations. Required, non-empty.
	Keys []GenKey `yaml:"keys"`
}

// GenDecl is a parsed declaration file: the OnlineConf keys read by the
// generated code of one template.
type GenDecl struct {
	Groups []GenGroup `yaml:"groups"`
}

// GenDeclFile pairs a declaration with the template whose generated code
// performs the reads.
type GenDeclFile struct {
	// Owner is the template path relative to the embedded templates root, e.g.
	// transport/rest/files/pkg/app/restconfig/config_oc.go.tmpl.
	Owner string
	// Decl holds the parsed declaration.
	Decl *GenDecl
}

// RESTServer is one REST server transport and the applications serving it.
type RESTServer struct {
	Transport string
	Apps      []string
}

// KafkaProducer is one Kafka producer client and the events published through
// it.
type KafkaProducer struct {
	Client string
	Events []string
}

// ProjectSurface carries the project dimensions the declarations expand over.
// It is deliberately a plain description of the project rather than the
// generator's config type: the expansion is a pure function of these names, and
// that is what makes it testable without building a whole project.
//
// The same type describes one application, which is what the startup check needs
// — a process must not refuse to start over a key only some other application of
// the project reads.
type ProjectSurface struct {
	// Service is the service tree root — the project name.
	Service string
	// RESTServers are the REST transports the project serves (everything but
	// ogen_client), each with the applications it is wired into.
	RESTServers []RESTServer
	// RESTClients are the names generated REST clients read their configuration
	// under (the ogen_client package name, name_version).
	RESTClients []string
	// GRPCClients are the gRPC client transport names.
	GRPCClients []string
	// KafkaProducers are the Kafka producer clients and their events.
	KafkaProducers []KafkaProducer
	// DaemonWorkers are the names of workers generated from the daemon
	// template.
	DaemonWorkers []string
	// TelegramWorkers are the names of workers generated from the telegram
	// template.
	TelegramWorkers []string
	// Tolerant are the components whose start a missing key does not stop,
	// named as they appear in the expanded paths — a transport's, client's or
	// worker's own segment. Two project.yaml settings put a component here:
	// `optional: true`, whose generated reader treats an absent key as "not
	// configured, skip", and `instantiation: dynamic`, whose generated client
	// takes its address as an argument and does not read the key at all. Either
	// way a key declared `required_at_init` is not one for this component, and
	// a guard demanding it would demand configuration the project says is not
	// needed.
	Tolerant map[string]bool
}

// GeneratorKey is one concrete OnlineConf path the generated code of a concrete
// project reads. Runtime segments stay in their `<name>` spelling: that is the
// honest rendering of a path whose last segments only exist at runtime.
type GeneratorKey struct {
	// Path is the full path, with every dimension placeholder expanded.
	Path string
	// Type is the value type, empty for a path handed to generated code as a
	// prefix.
	Type string
	// Accessor is the generated accessor of this path, empty when the path is
	// read through a shared one.
	Accessor string
	// ReadVia names the shared accessor the path is read through, empty
	// otherwise.
	ReadVia string
	// OwnedPrefix is the path from which the subtree belongs to the generator,
	// empty when the path claims no subtree. It is the path itself for a prefix
	// builder, and the part before the first runtime segment otherwise.
	OwnedPrefix string
	// Required reports a key the startup check verifies.
	Required bool
	// RequiredAtInit reports a key whose absence keeps the process from
	// starting without any startup check seeing it: the component reports it
	// from its own Init. False where the project's own settings answer for the
	// component — a client taking its address as an argument, or one declared
	// `optional: true` — because then the key is not needed here.
	RequiredAtInit bool
	// Default is the fallback the generated reader applies, as prose.
	Default string
	// Descriptions holds what the key does, one entry per distinct reason it is
	// read. More than one is normal where more than one reader shares the key.
	Descriptions []string
	// ReadBy lists the templates declaring this path, sorted. More than one is
	// normal: the dev-stand flag, for instance, is read by the logger and by the
	// ogen error responses.
	ReadBy []string
}

// substitution is one expansion of a declaration over the project: the values
// its dimension placeholders take, and what the project says about the
// component being expanded.
type substitution struct {
	// values maps every dimension placeholder of the expansion to its value.
	values map[string]string
	// tolerant reports a component whose start a missing key does not stop:
	// one the project declares optional, or one that does not read the key at
	// all.
	tolerant bool
}

// GenParam is one parameter of a generated accessor, in path order.
type GenParam struct {
	// Name is the Go parameter name.
	Name string
	// Runtime reports a segment the generated code computes at runtime, as
	// opposed to a project dimension the generator knows.
	Runtime bool
}

// GenAccessor is one function the generator contributes to the accessor
// package: a typed read, or a builder of a path generated code reads below.
//
// One accessor per declared path template, merged across the declaration files:
// the dev-stand flag is declared by four templates and read through one
// function.
type GenAccessor struct {
	// Name is the Go identifier.
	Name string
	// Template is the declared path, normalized, with its placeholders and
	// runtime segments intact. It is the identity of the accessor.
	Template string
	// Type is the value type, empty for a path builder.
	Type string
	// Required reports an accessor that returns ErrKeyNotSet for an absent key.
	Required bool
	// RequiredAtInit reports a declaration whose key the component cannot start
	// without. It changes nothing about the accessor — it is carried here so
	// that two declarations of one path disagreeing about it are caught.
	RequiredAtInit bool
	// Shared reports an accessor that claims no subtree because its whole path
	// comes from its caller.
	Shared bool
	// Params are the accessor's parameters, in path order.
	Params []GenParam
	// Default is the fallback the generated reader applies, as prose.
	Default string
	// Descriptions holds what the key does, one entry per distinct reason.
	Descriptions []string
	// ReadBy lists the declaring templates, sorted.
	ReadBy []string
}

// GenRequiredCheck is one entry of an application's startup check: a required
// accessor and the concrete arguments this application reads it with.
type GenRequiredCheck struct {
	// Accessor is the Go identifier of the accessor to call.
	Accessor string
	// Args are the dimension values, in parameter order, already quoted as Go
	// string literals.
	Args []string
	// Path is the concrete path the call reads, for the error message of a
	// declaration defect and for review.
	Path string
}

// GenSurface is the generator's half of a project's OnlineConf key map.
type GenSurface struct {
	// Keys are the concrete paths the generated code of this project reads.
	Keys []GeneratorKey
	// Accessors are the functions the accessor package gets for them.
	Accessors []GenAccessor
	// Required are the startup checks, one per required accessor and expansion.
	Required []GenRequiredCheck
}

// expansion accumulates the expansion of every declaration over one project.
type expansion struct {
	keys      map[string]*GeneratorKey
	accessors map[string]*GenAccessor
	// byName maps an accessor name to the template that claimed it, so that two
	// different paths cannot generate one function.
	byName   map[string]string
	required []GenRequiredCheck
	problems []string
}

// GenDeclSuffix is the file name suffix of a declaration file. The owning
// template is the same path with the suffix removed: a declaration for
// `service.go.tmpl` is `service.go.tmpl.ockeys.yaml` in the same directory, so
// the mapping between a read and its declaration is mechanical and checkable.
const GenDeclSuffix = ".ockeys.yaml"

// Dimension placeholders a declared path may carry. Which ones are available is
// decided by the group's scope — that is what makes a declaration expandable
// into the concrete paths of a concrete project instead of a pattern nobody can
// match.
const (
	PlaceholderService   = "{service}"
	PlaceholderTransport = "{transport}"
	PlaceholderApp       = "{app}"
	PlaceholderWorker    = "{worker}"
	PlaceholderClient    = "{client}"
	PlaceholderEvent     = "{event}"
)

// requiredAtInitSuffix is how `required_at_init` reads in the description of a
// declaration, which is what the message of a disagreement between two
// declarations of one path is built from.
const requiredAtInitSuffix = ", required at init"

// pathBuilderSuffix is the mandatory tail of the name of a path-builder entry —
// a declaration with no `type`, which generates a function returning the path
// rather than reading it. The suffix keeps a builder from colliding with the
// `<Name>Path` builder generated alongside a value accessor.
const pathBuilderSuffix = "Path"

// Declaration scopes — the project dimension a group of keys is read for, and
// therefore what the generator expands it over.
const (
	// ScopeService covers code every project gets, read once per service.
	ScopeService = "service"
	// ScopeRESTServer covers the REST server configuration, read per REST
	// server and per application serving it. A path that names no {app}
	// expands once per server. {transport} is the server name, which carries
	// the api version ("api_v1"): the name the generated main hands to
	// rest.NewServer.
	ScopeRESTServer = "rest_server"
	// ScopeRESTClient covers the generated REST clients (ogen_client), read per
	// client transport.
	ScopeRESTClient = "rest_client"
	// ScopeGRPCClient covers the generated gRPC clients (buf_client), read per
	// client transport.
	ScopeGRPCClient = "grpc_client"
	// ScopeKafkaProducer covers the Kafka producer configuration, read per
	// client name.
	ScopeKafkaProducer = "kafka_producer"
	// ScopeKafkaProducerEvent covers per-event Kafka settings, read per client
	// and event.
	ScopeKafkaProducerEvent = "kafka_producer_event"
	// ScopeWorkerDaemon covers the daemon worker template, read per worker.
	ScopeWorkerDaemon = "worker_daemon"
	// ScopeWorkerTelegram covers the telegram worker template, read per worker.
	ScopeWorkerTelegram = "worker_telegram"
	// ScopeTelegramDriver covers the shared telegram driver, generated once when
	// the project has a telegram worker.
	ScopeTelegramDriver = "telegram_driver"
)

// ErrGenDeclDefect reports a broken declaration shipped with the generator: a
// declaration file that does not parse, names an unknown scope, uses a
// placeholder its scope cannot fill, or contradicts another declaration of the
// same path. It is a defect in this repository, not in the user's project, and
// the message says so.
var ErrGenDeclDefect = errors.New("generator onlineconf key declaration defect")

// scopePlaceholders is the closed set of dimension placeholders each scope can
// fill. A path using anything else is a defect: it could never be expanded into
// a path that exists in a tree.
var scopePlaceholders = map[string][]string{
	ScopeService:            {PlaceholderService},
	ScopeRESTServer:         {PlaceholderService, PlaceholderTransport, PlaceholderApp},
	ScopeRESTClient:         {PlaceholderService, PlaceholderTransport},
	ScopeGRPCClient:         {PlaceholderService, PlaceholderTransport},
	ScopeKafkaProducer:      {PlaceholderService, PlaceholderClient},
	ScopeKafkaProducerEvent: {PlaceholderService, PlaceholderClient, PlaceholderEvent},
	ScopeWorkerDaemon:       {PlaceholderService, PlaceholderWorker},
	ScopeWorkerTelegram:     {PlaceholderService, PlaceholderWorker},
	ScopeTelegramDriver:     {PlaceholderService},
}

// runtimeParamRx is the accepted grammar of a runtime segment, `<name>`: the
// name becomes a parameter of the generated accessor, so it has to be an
// unexported Go identifier.
var runtimeParamRx = regexp.MustCompile(`^<[a-z][A-Za-z0-9]*>$`)

// componentPlaceholders are the placeholders naming the component a key is read
// for — the thing a project.yaml setting can declare `optional: true`.
var componentPlaceholders = []string{PlaceholderTransport, PlaceholderClient, PlaceholderWorker}

// genKeyFields is the closed set of fields a declaration entry may carry.
var genKeyFields = map[string]struct{}{
	"path": {}, "name": {}, "type": {}, "required": {}, "required_at_init": {},
	"shared": {}, "read_via": {}, "default": {}, "description": {},
}

// genGroupFields is the closed set of fields a declaration group may carry.
var genGroupFields = map[string]struct{}{
	"scope": {}, "keys": {},
}

// Description returns the prose of the entry: every distinct reason the key is
// read, in declaration order, as one sentence-joined string.
func (k GeneratorKey) Description() string {
	return strings.Join(k.Descriptions, " ")
}

// Description returns every distinct reason the key is read, joined.
func (a GenAccessor) Description() string {
	return strings.Join(a.Descriptions, " ")
}

// IsPathBuilder reports whether the accessor returns a path instead of reading
// a value.
func (a GenAccessor) IsPathBuilder() bool {
	return a.Type == ""
}

// UnmarshalYAML decodes a declaration entry, rejecting unknown fields.
func (k *GenKey) UnmarshalYAML(value *yaml.Node) error {
	type plain struct {
		Path           string `yaml:"path"`
		Name           string `yaml:"name"`
		Type           string `yaml:"type"`
		Required       bool   `yaml:"required"`
		RequiredAtInit bool   `yaml:"required_at_init"`
		Shared         bool   `yaml:"shared"`
		ReadVia        string `yaml:"read_via"`
		Default        string `yaml:"default"`
		Description    string `yaml:"description"`
	}

	if err := rejectUnknownFields(value, genKeyFields); err != nil {
		return err
	}

	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}

	k.Path = normalizePath(p.Path)
	k.Name = p.Name
	k.Type = p.Type
	k.Required = p.Required
	k.RequiredAtInit = p.RequiredAtInit
	k.Shared = p.Shared
	k.ReadVia = p.ReadVia
	k.Default = p.Default
	k.Description = p.Description

	return nil
}

// UnmarshalYAML decodes a declaration group, rejecting unknown fields.
func (g *GenGroup) UnmarshalYAML(value *yaml.Node) error {
	type plain struct {
		Scope string   `yaml:"scope"`
		Keys  []GenKey `yaml:"keys"`
	}

	if err := rejectUnknownFields(value, genGroupFields); err != nil {
		return err
	}

	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}

	g.Scope = p.Scope
	g.Keys = p.Keys

	return nil
}

// ParseGenDecl decodes and validates one declaration file. origin names the file
// in error messages.
func ParseGenDecl(data []byte, origin string) (*GenDecl, error) {
	var d GenDecl

	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: %s is empty", ErrGenDeclDefect, origin)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)

	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrGenDeclDefect, origin, err)
	}

	if problems := d.problems(); len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s:%s%s", ErrGenDeclDefect, origin,
			problemIndent, strings.Join(problems, problemIndent))
	}

	return &d, nil
}

// problems returns every rule the declaration breaks.
func (d *GenDecl) problems() []string {
	if len(d.Groups) == 0 {
		return []string{"no groups declared"}
	}

	var problems []string

	for i, group := range d.Groups {
		allowed, known := scopePlaceholders[group.Scope]
		if !known {
			problems = append(problems, fmt.Sprintf("groups[%d]: scope %q is unknown, expected one of %s",
				i, group.Scope, strings.Join(GenScopes(), ", ")))

			continue
		}

		if len(group.Keys) == 0 {
			problems = append(problems, fmt.Sprintf("groups[%d]: scope %q declares no keys", i, group.Scope))

			continue
		}

		for j, key := range group.Keys {
			for _, msg := range key.problems(allowed) {
				problems = append(problems, fmt.Sprintf("groups[%d].keys[%d]: %s", i, j, msg))
			}
		}
	}

	return problems
}

// problems checks one declaration entry against the placeholders its scope can
// fill.
func (k GenKey) problems(allowed []string) []string {
	if k.Path == "" {
		return []string{"path is required"}
	}

	problems := k.segmentProblems(allowed)
	problems = append(problems, k.shapeProblems()...)
	problems = append(problems, k.namingProblems()...)

	if strings.TrimSpace(k.Description) == "" {
		problems = append(problems, fmt.Sprintf(
			"path %q has no description: the generated registry is where the owner of the contract "+
				"reads what a generator-owned key does", k.Path))
	}

	return problems
}

// segmentProblems checks the grammar of the path and the placeholders against
// the scope.
func (k GenKey) segmentProblems(allowed []string) []string {
	segments := k.Segments()

	var problems []string

	if len(segments) < minPathSegments {
		problems = append(problems, fmt.Sprintf(
			"path %q has no key segment: a path must be {service}/<key> at the least", k.Path))
	}

	problems = append(problems, k.rootProblems(segments)...)

	for _, segment := range segments {
		switch {
		case isPlaceholder(segment):
			if !slices.Contains(allowed, segment) {
				problems = append(problems, fmt.Sprintf(
					"path %q uses %s, which this scope cannot fill (available: %s)",
					k.Path, segment, strings.Join(allowed, ", ")))
			}
		case strings.HasPrefix(segment, "<"), strings.HasSuffix(segment, ">"):
			if !runtimeParamRx.MatchString(segment) {
				problems = append(problems, fmt.Sprintf(
					"path %q has an invalid runtime segment %q, expected %s",
					k.Path, segment, runtimeParamRx.String()))
			}
		case !segmentRx.MatchString(segment):
			problems = append(problems, fmt.Sprintf("path %q has an invalid segment %q, expected %s, "+
				"a {dimension} or a <runtimeSegment>", k.Path, segment, segmentRx.String()))
		}
	}

	names := make(map[string]struct{}, len(segments))

	for _, param := range k.Params() {
		if _, taken := names[param.Name]; taken {
			problems = append(problems, fmt.Sprintf(
				"path %q uses the parameter name %q twice, so the accessor could not be written",
				k.Path, param.Name))
		}

		names[param.Name] = struct{}{}
	}

	if len(segments) > 1 && isRuntimeParam(segments[1]) && !k.Shared {
		problems = append(problems, fmt.Sprintf(
			"path %q opens a runtime subtree directly under {service}, which would claim the whole "+
				"service tree. Only a `shared: true` accessor may do that, and it claims nothing: "+
				"its concrete paths are declared by the readers that compose them", k.Path))
	}

	return problems
}

// rootProblems checks what a path starts at, which depends on who composes it.
//
// Every key the generator reads itself lives in the service tree, so its
// declaration names the tree: the first segment is {service}, and the generated
// accessor prepends it.
//
// A `shared: true` accessor is the exception, and the reason is that it does not
// compose its own path: its caller hands in a prefix that is already the absolute
// path of the subtree owning the reader, service name included. Prepending the
// tree to such a prefix duplicates it and moves the read one level deeper than
// anything declares — which is exactly how the log level became dead config
// (TOOLS-65). So a shared path starts at the caller's own segment, and the
// concrete paths it reaches are declared by those callers with `read_via`, each
// of them rooted in the service tree as usual.
func (k GenKey) rootProblems(segments []string) []string {
	if len(segments) == 0 {
		return nil
	}

	if k.Shared {
		if !isRuntimeParam(segments[0]) {
			return []string{fmt.Sprintf(
				"path %q is `shared` but starts with %q: a shared accessor is handed an absolute "+
					"prefix by its caller, so its path starts at that <runtimeSegment> and no service "+
					"tree is prepended to it", k.Path, segments[0])}
		}

		return nil
	}

	if segments[0] != PlaceholderService {
		return []string{fmt.Sprintf(
			"path %q must start with %s: every key the generator reads lives in the service tree",
			k.Path, PlaceholderService)}
	}

	return nil
}

// shapeProblems checks the combination of type, required, shared and read_via —
// what the entry says is read and how.
func (k GenKey) shapeProblems() []string {
	var problems []string

	if k.Type != "" {
		if _, ok := knownTypes[k.Type]; !ok {
			problems = append(problems, fmt.Sprintf(
				"path %q declares type %q, expected one of %s (or no type at all, for a path "+
					"generated code reads below)", k.Path, k.Type, strings.Join(KnownTypes(), ", ")))
		}
	}

	if k.ReadVia != "" {
		if k.Type == "" {
			problems = append(problems, fmt.Sprintf(
				"path %q names read_via %s but no type: a path read through a shared accessor is "+
					"read as a value", k.Path, k.ReadVia))
		}

		if k.Name != "" {
			problems = append(problems, fmt.Sprintf(
				"path %q names both read_via %s and name %s: the path is read through the shared "+
					"accessor, so no accessor of its own is generated", k.Path, k.ReadVia, k.Name))
		}

		if k.Shared {
			problems = append(problems, fmt.Sprintf(
				"path %q is both `shared` and read_via %s: a shared accessor is the one others are "+
					"read through", k.Path, k.ReadVia))
		}
	} else if k.Name == "" {
		problems = append(problems, fmt.Sprintf(
			"path %q has no name: it names the generated accessor the template calls", k.Path))
	}

	if k.Shared {
		if k.Type == "" {
			problems = append(problems, fmt.Sprintf(
				"path %q is `shared` but declares no type: a shared accessor reads a value", k.Path))
		}

		if !slices.ContainsFunc(k.Params(), func(p GenParam) bool { return p.Runtime }) {
			problems = append(problems, fmt.Sprintf(
				"path %q is `shared` but has no runtime segment: a shared accessor is one whose "+
					"path its caller composes", k.Path))
		}
	}

	problems = append(problems, k.requiredProblems()...)
	problems = append(problems, k.requiredAtInitProblems()...)

	return problems
}

// requiredProblems checks that `required` can be enforced for this entry.
func (k GenKey) requiredProblems() []string {
	if !k.Required {
		return nil
	}

	var problems []string

	if k.Type == "" {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required` but declares no type: there is no value to require", k.Path))
	}

	if k.Default != "" {
		problems = append(problems, fmt.Sprintf(
			"path %q declares both `required` and `default`: a key with a fallback is never "+
				"missing, and requiring it would stop installations that rely on the fallback", k.Path))
	}

	if k.Shared {
		problems = append(problems, fmt.Sprintf(
			"path %q is `shared` and `required`: the startup check cannot enumerate the paths a "+
				"caller composes", k.Path))
	}

	if slices.ContainsFunc(k.Params(), func(p GenParam) bool { return p.Runtime }) {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required` but has a runtime segment: the startup check verifies paths the "+
				"generator can enumerate, and this one only exists once the process is running", k.Path))
	}

	if k.Type == TypeStruct {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required` and type %s: verifying it would mean unmarshaling the value, and "+
				"the destination type belongs to the caller. The startup check also calls the accessor "+
				"without a destination, which a struct accessor has no signature for", k.Path, TypeStruct))
	}

	return problems
}

// requiredAtInitProblems checks that `required_at_init` says something a guard
// outside the project could act on.
//
// The flag is published, not enforced, so the rules are about whether the fact
// is checkable at all: a path a guard cannot enumerate, or one that is never
// missing because a fallback answers for it, would make the registry promise a
// check nobody can run.
func (k GenKey) requiredAtInitProblems() []string {
	if !k.RequiredAtInit {
		return nil
	}

	var problems []string

	if k.Type == "" {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required_at_init` but declares no type: a path handed to generated code "+
				"as a prefix has no value to be missing", k.Path))
	}

	if k.Default != "" {
		problems = append(problems, fmt.Sprintf(
			"path %q declares both `required_at_init` and `default`: a key with a fallback is never "+
				"missing, and a guard demanding it would be demanding the fallback never be used", k.Path))
	}

	if k.Required {
		problems = append(problems, fmt.Sprintf(
			"path %q declares both `required` and `required_at_init`: the startup check of a "+
				"`required` key IS the failed start, and the registry marks it as one — drop "+
				"`required_at_init`", k.Path))
	}

	if k.Shared {
		problems = append(problems, fmt.Sprintf(
			"path %q is `shared` and `required_at_init`: a shared accessor's paths are composed by "+
				"its callers, so there is no path for a guard to demand. Mark them where they are "+
				"declared with `read_via`", k.Path))
	}

	if slices.ContainsFunc(k.Params(), func(p GenParam) bool { return p.Runtime }) {
		problems = append(problems, fmt.Sprintf(
			"path %q is `required_at_init` but has a runtime segment: the path only exists once the "+
				"process runs, so nothing can be seeded against it beforehand", k.Path))
	}

	return problems
}

// namingProblems checks that the entry names a usable Go identifier.
func (k GenKey) namingProblems() []string {
	var problems []string

	if k.Name != "" && !identRx.MatchString(k.Name) {
		problems = append(problems, fmt.Sprintf(errNotExportedNameFmt, k.Name))

		return problems
	}

	if k.ReadVia != "" && !identRx.MatchString(k.ReadVia) {
		problems = append(problems, fmt.Sprintf(
			"read_via %q is not an exported Go identifier; it names the shared accessor the path is "+
				"read through", k.ReadVia))
	}

	switch {
	case k.Name == "":
	case k.Type == "" && !strings.HasSuffix(k.Name, pathBuilderSuffix):
		problems = append(problems, fmt.Sprintf(
			"path %q declares no type, so it generates a path builder and its name %q must end in %q",
			k.Path, k.Name, pathBuilderSuffix))
	case k.Type != "" && strings.HasSuffix(k.Name, pathBuilderSuffix):
		problems = append(problems, fmt.Sprintf(
			"path %q reads a value, so its name %q must not end in %q — that spelling is taken by "+
				"the path builder generated alongside the accessor", k.Path, k.Name, pathBuilderSuffix))
	}

	return problems
}

// Segments returns the path segments of the declaration, root first.
func (k GenKey) Segments() []string {
	return pathSegments(k.Path)
}

// Params returns the parameters the generated accessor takes: every dimension
// placeholder except {service}, and every runtime segment, in path order.
func (k GenKey) Params() []GenParam {
	segments := k.Segments()
	out := make([]GenParam, 0, len(segments))

	for i, segment := range segments {
		switch {
		case i == 0 && segment == PlaceholderService:
		case isPlaceholder(segment):
			out = append(out, GenParam{Name: strings.Trim(segment, "{}")})
		case isRuntimeParam(segment):
			out = append(out, GenParam{Name: strings.Trim(segment, "<>"), Runtime: true})
		}
	}

	return out
}

// ownedPrefix returns the path from which the subtree belongs to the generator,
// as a template, or the empty string when the entry claims nothing.
//
// A path builder owns everything below the path it hands out: the generated
// reader appends segments the generator never sees. A path with a runtime
// segment owns everything from that segment on, for the same reason. A shared
// accessor owns nothing — its caller supplies the whole path, and the concrete
// paths are declared where the caller is.
func (k GenKey) ownedPrefix() string {
	if k.Shared || k.ReadVia != "" {
		return ""
	}

	segments := k.Segments()

	for i, segment := range segments {
		if isRuntimeParam(segment) {
			return "/" + strings.Join(segments[:i], "/")
		}
	}

	if k.Type == "" {
		return k.Path
	}

	return ""
}

// ExpandGenDecls turns the generator's declarations into the concrete paths the
// generated code of this project reads, the accessors generated for them, and
// the startup checks of the required ones.
//
// A path whose placeholders the project cannot fill is skipped rather than
// reported: a REST transport wired into no application has no {app} to expand,
// and a project with no Kafka producer reads no Kafka keys. That is also what
// makes the function usable per application — pass the dimensions of one
// application and the required checks are the ones that application's process
// needs.
//
// Two declarations of the same path are merged — the dev-stand flag is read by
// several templates — but a disagreement about what the path is counts as a defect,
// because then the two readers do not mean the same key.
func ExpandGenDecls(decls []GenDeclFile, surface ProjectSurface) (*GenSurface, error) {
	if surface.Service == "" {
		return nil, fmt.Errorf("%w: project surface has no service name", ErrGenDeclDefect)
	}

	e := expansion{
		keys:      map[string]*GeneratorKey{},
		accessors: map[string]*GenAccessor{},
		byName:    map[string]string{},
	}

	for _, decl := range decls {
		if decl.Decl == nil {
			continue
		}

		for _, group := range decl.Decl.Groups {
			e.addGroup(group, decl.Owner, surface)
		}
	}

	e.checkReadVia()

	if len(e.problems) > 0 {
		sort.Strings(e.problems)

		return nil, problemList(ErrGenDeclDefect, e.problems)
	}

	return e.result(), nil
}

// addGroup expands one declaration group over the project.
//
// The accessor set does NOT depend on the project's dimensions, the registry
// does. An accessor takes its dimensions as arguments, so it is valid whatever
// the project has — and a template that is generated calls its accessors
// unconditionally, so gating them on a dimension would be a compile error
// waiting to happen: the REST configuration provider is generated for a project
// whose only REST transport is a client, and it still calls every server
// accessor.
//
// What the dimensions decide is which concrete paths this project reads: the
// rows of the registry, and the startup checks of the required ones.
func (e *expansion) addGroup(group GenGroup, owner string, surface ProjectSurface) {
	substitutions := surface.substitutions(group.Scope)

	for _, key := range group.Keys {
		e.addAccessor(key, owner)

		// A shared accessor contributes no row to the registry: the concrete
		// paths it reaches are the ones declared with read_via.
		if key.Shared {
			continue
		}

		for _, sub := range substitutions {
			path, ok := expandPath(key.Path, sub.values)
			if !ok {
				continue
			}

			// A component the project lets do without its configuration is not
			// one a guard can demand configuration for, so the published fact
			// is per expansion rather than per declaration: the same host key
			// is a start-stopper for one client and not for the next.
			e.addKey(key, owner, path, key.RequiredAtInit && !sub.tolerant)

			if key.Required {
				e.addRequired(key, sub.values, path)
			}
		}
	}
}

// addKey folds one expanded path into the registry, reporting a contradiction
// with an entry already there.
func (e *expansion) addKey(key GenKey, owner, path string, requiredAtInit bool) {
	accessor := key.Name
	if key.ReadVia != "" {
		accessor = ""
	}

	existing, seen := e.keys[path]
	if !seen {
		e.keys[path] = &GeneratorKey{
			Path:           path,
			Type:           key.Type,
			Accessor:       accessor,
			ReadVia:        key.ReadVia,
			OwnedPrefix:    expandOwnedPrefix(key, path),
			Required:       key.Required,
			RequiredAtInit: requiredAtInit,
			Default:        key.Default,
			Descriptions:   []string{key.Description},
			ReadBy:         []string{owner},
		}

		return
	}

	// The flag is compared here as well as on the accessor, because a path read
	// through a shared accessor has no accessor entry of its own — and it is
	// exactly such a path that a second declaration could mark differently.
	if existing.Type != key.Type || existing.Accessor != accessor || existing.ReadVia != key.ReadVia ||
		existing.RequiredAtInit != requiredAtInit {
		e.problems = append(e.problems, fmt.Sprintf("path %q is declared as %s by %s and as %s by %s",
			path, describeGenKey(existing.Type, existing.Accessor, existing.ReadVia, existing.RequiredAtInit),
			strings.Join(existing.ReadBy, ", "),
			describeGenKey(key.Type, accessor, key.ReadVia, requiredAtInit), owner))

		return
	}

	if !slices.Contains(existing.ReadBy, owner) {
		existing.ReadBy = append(existing.ReadBy, owner)
	}

	// Several readers of one key mean several reasons it is read: the dev-stand
	// flag switches console logging, error-response detail and bot diagnostics.
	// Keeping only the first would leave the registry naming every reader but
	// explaining one of them, and which one would depend on the sort order of
	// template paths.
	if key.Description != "" && !slices.Contains(existing.Descriptions, key.Description) {
		existing.Descriptions = append(existing.Descriptions, key.Description)
	}
}

// addAccessor folds one declaration into the set of generated functions, keyed
// by its path template.
func (e *expansion) addAccessor(key GenKey, owner string) {
	if key.ReadVia != "" {
		return
	}

	existing, seen := e.accessors[key.Path]
	if !seen {
		// Both names a declaration occupies: the accessor and, for a value key,
		// the builder of its path. Reserving only the declared one would let a
		// value key `Worker` and a path builder `WorkerPath` through, and the
		// generated package would declare WorkerPath twice.
		for _, name := range key.generatedNames() {
			if claimant, taken := e.byName[name]; taken {
				e.problems = append(e.problems, fmt.Sprintf(
					"%s is generated for %q and for %q: one name, two paths",
					name, claimant, key.Path))

				return
			}
		}

		for _, name := range key.generatedNames() {
			e.byName[name] = key.Path
		}

		e.accessors[key.Path] = &GenAccessor{
			Name:           key.Name,
			Template:       key.Path,
			Type:           key.Type,
			Required:       key.Required,
			RequiredAtInit: key.RequiredAtInit,
			Shared:         key.Shared,
			Params:         key.Params(),
			Default:        key.Default,
			Descriptions:   []string{key.Description},
			ReadBy:         []string{owner},
		}

		return
	}

	if existing.Name != key.Name || existing.Type != key.Type ||
		existing.Required != key.Required || existing.RequiredAtInit != key.RequiredAtInit ||
		existing.Shared != key.Shared {
		e.problems = append(e.problems, fmt.Sprintf(
			"template %q is declared as %s by %s and as %s by %s",
			key.Path, describeGenAccessor(existing.Name, existing.Type, existing.Required, existing.RequiredAtInit),
			strings.Join(existing.ReadBy, ", "),
			describeGenAccessor(key.Name, key.Type, key.Required, key.RequiredAtInit), owner))

		return
	}

	if !slices.Contains(existing.ReadBy, owner) {
		existing.ReadBy = append(existing.ReadBy, owner)
	}

	if key.Description != "" && !slices.Contains(existing.Descriptions, key.Description) {
		existing.Descriptions = append(existing.Descriptions, key.Description)
	}
}

// addRequired records one startup check: the required accessor and the dimension
// values this expansion reads it with.
func (e *expansion) addRequired(key GenKey, values map[string]string, path string) {
	args := make([]string, 0, len(key.Params()))

	for _, param := range key.Params() {
		args = append(args, values["{"+param.Name+"}"])
	}

	check := GenRequiredCheck{Accessor: key.Name, Args: args, Path: path}

	if slices.ContainsFunc(e.required, func(c GenRequiredCheck) bool { return c.Path == check.Path }) {
		return
	}

	e.required = append(e.required, check)
}

// checkReadVia reports a path declared as read through a shared accessor that
// does not exist, or that is not shared. Without the check a typo would leave a
// row in the registry pointing at nothing.
func (e *expansion) checkReadVia() {
	shared := make(map[string]struct{}, len(e.accessors))

	for _, accessor := range e.accessors {
		if accessor.Shared {
			shared[accessor.Name] = struct{}{}
		}
	}

	for path, key := range e.keys {
		if key.ReadVia == "" {
			continue
		}

		if _, ok := shared[key.ReadVia]; !ok {
			e.problems = append(e.problems, fmt.Sprintf(
				"path %q is declared read_via %s, which is no shared accessor of this project "+
					"(declared by %s)", path, key.ReadVia, strings.Join(key.ReadBy, ", ")))
		}
	}
}

// result assembles the sorted expansion.
func (e *expansion) result() *GenSurface {
	out := GenSurface{
		Keys:      make([]GeneratorKey, 0, len(e.keys)),
		Accessors: make([]GenAccessor, 0, len(e.accessors)),
		Required:  e.required,
	}

	for _, key := range e.keys {
		sort.Strings(key.ReadBy)
		out.Keys = append(out.Keys, *key)
	}

	for _, accessor := range e.accessors {
		sort.Strings(accessor.ReadBy)
		out.Accessors = append(out.Accessors, *accessor)
	}

	sort.Slice(out.Keys, func(i, j int) bool { return out.Keys[i].Path < out.Keys[j].Path })
	sort.Slice(out.Accessors, func(i, j int) bool { return out.Accessors[i].Name < out.Accessors[j].Name })
	sort.Slice(out.Required, func(i, j int) bool { return out.Required[i].Path < out.Required[j].Path })

	return &out
}

// generatedNames returns every Go identifier the declaration makes the generator
// emit: the accessor, plus the builder of its path when it reads a value. A
// path-builder entry emits only itself — its name already ends in the suffix.
func (k GenKey) generatedNames() []string {
	if k.Name == "" {
		return nil
	}

	if k.Type == "" {
		return []string{k.Name}
	}

	return []string{k.Name, k.Name + pathBuilderSuffix}
}

// substitutions returns one expansion of the scope per dimension value of this
// project.
func (s ProjectSurface) substitutions(scope string) []substitution {
	base := map[string]string{PlaceholderService: s.Service}

	switch scope {
	case ScopeService:
		return []substitution{s.substitute(base)}
	case ScopeRESTServer:
		return s.restServerSubstitutions()
	case ScopeRESTClient:
		return s.oneDimension(base, PlaceholderTransport, s.RESTClients)
	case ScopeGRPCClient:
		return s.oneDimension(base, PlaceholderTransport, s.GRPCClients)
	case ScopeKafkaProducer:
		clients := make([]string, 0, len(s.KafkaProducers))
		for _, producer := range s.KafkaProducers {
			clients = append(clients, producer.Client)
		}

		return s.oneDimension(base, PlaceholderClient, clients)
	case ScopeKafkaProducerEvent:
		return s.kafkaEventSubstitutions()
	case ScopeWorkerDaemon:
		return s.oneDimension(base, PlaceholderWorker, s.DaemonWorkers)
	case ScopeWorkerTelegram:
		return s.oneDimension(base, PlaceholderWorker, s.TelegramWorkers)
	case ScopeTelegramDriver:
		if len(s.TelegramWorkers) == 0 {
			return nil
		}

		return []substitution{s.substitute(base)}
	default:
		// Unreachable: ParseGenDecl rejects an unknown scope.
		return nil
	}
}

// substitute pairs a placeholder mapping with what the project says about the
// component it names.
func (s ProjectSurface) substitute(values map[string]string) substitution {
	return substitution{values: values, tolerant: s.tolerates(values)}
}

// tolerates reports whether the component this expansion is for is one whose
// start a missing key does not stop.
//
// Only the placeholders naming a component are consulted. {app} is who does the
// reading rather than what is read for — an application is not something a
// project declares optional — and {event} is a setting of the producer that
// publishes it, so the producer's own segment already answers for it.
func (s ProjectSurface) tolerates(values map[string]string) bool {
	if len(s.Tolerant) == 0 {
		return false
	}

	for _, placeholder := range componentPlaceholders {
		if value := values[placeholder]; value != "" && s.Tolerant[value] {
			return true
		}
	}

	return false
}

// restServerSubstitutions yields one mapping per (transport, application) pair,
// plus a transport-only mapping so that the transport-level fallback paths —
// which name no {app} — expand for a transport serving no application too.
func (s ProjectSurface) restServerSubstitutions() []substitution {
	out := make([]substitution, 0, len(s.RESTServers))

	for _, server := range s.RESTServers {
		out = append(out, s.substitute(map[string]string{
			PlaceholderService:   s.Service,
			PlaceholderTransport: server.Transport,
		}))

		for _, app := range server.Apps {
			out = append(out, s.substitute(map[string]string{
				PlaceholderService:   s.Service,
				PlaceholderTransport: server.Transport,
				PlaceholderApp:       app,
			}))
		}
	}

	return out
}

// kafkaEventSubstitutions yields one mapping per (client, event) pair.
func (s ProjectSurface) kafkaEventSubstitutions() []substitution {
	out := make([]substitution, 0, len(s.KafkaProducers))

	for _, producer := range s.KafkaProducers {
		for _, event := range producer.Events {
			out = append(out, s.substitute(map[string]string{
				PlaceholderService: s.Service,
				PlaceholderClient:  producer.Client,
				PlaceholderEvent:   event,
			}))
		}
	}

	return out
}

// oneDimension yields one expansion per value of a single placeholder.
func (s ProjectSurface) oneDimension(
	base map[string]string, placeholder string, values []string,
) []substitution {
	out := make([]substitution, 0, len(values))

	for _, value := range values {
		next := make(map[string]string, len(base)+1)
		for k, v := range base {
			next[k] = v
		}

		next[placeholder] = value
		out = append(out, s.substitute(next))
	}

	return out
}

// expandPath substitutes the dimension placeholders of a declared path, leaving
// runtime segments in their `<name>` spelling. It reports false when the path
// needs a placeholder the mapping does not carry — the expansion does not apply
// to this dimension of the project.
func expandPath(raw string, substitution map[string]string) (string, bool) {
	segments := pathSegments(normalizePath(raw))
	out := make([]string, 0, len(segments))

	for _, segment := range segments {
		if !isPlaceholder(segment) {
			out = append(out, segment)

			continue
		}

		value, ok := substitution[segment]
		if !ok || value == "" {
			return "", false
		}

		out = append(out, value)
	}

	return "/" + strings.Join(out, "/"), true
}

// expandOwnedPrefix returns the owned subtree of an expanded key, as a concrete
// path. The expanded path is passed in rather than re-expanded so that the
// prefix cannot drift from the key it belongs to.
func expandOwnedPrefix(key GenKey, path string) string {
	prefix := key.ownedPrefix()
	if prefix == "" {
		return ""
	}

	depth := len(pathSegments(prefix))

	return "/" + strings.Join(pathSegments(path)[:depth], "/")
}

// CheckGeneratorKeys reports declarations of the service map that the generated
// code already reads.
//
// Two ways that happens. An exact path match is a key with two readers and two
// spellings — the thing the map exists to prevent. And a declaration at or
// inside a subtree the generated code owns is the same collision, only
// invisible: the subtree is either handed to generated code as a prefix, or
// opened by a segment the generated code computes at runtime (the url path of a
// request, a telegram user id, an action name). Either way there is no single
// generated path to compare against, so the subtree belongs to the generator as
// a whole.
func (m *Map) CheckGeneratorKeys(generated []GeneratorKey) error {
	if m == nil || len(generated) == 0 {
		return nil
	}

	exact := make(map[string]GeneratorKey, len(generated))

	owned := make([]GeneratorKey, 0, len(generated))

	for _, key := range generated {
		// Both, when a path is read as a value and read below: /<svc>/bot/users
		// is the blanket ACL and the root of the per-id ones.
		if key.Type != "" {
			exact[key.Path] = key
		}

		if key.OwnedPrefix != "" {
			owned = append(owned, key)
		}
	}

	var problems []string

	for i, key := range m.Keys {
		if generatedKey, clash := exact[key.Path]; clash {
			problems = append(problems, fmt.Sprintf(
				"keys[%d]: path %q is a key the generator reads itself (generated by %s): %s",
				i, key.Path, strings.Join(generatedKey.ReadBy, ", "), generatedKey.Description()))

			continue
		}

		for _, subtree := range owned {
			if !isAtOrUnder(key.Path, subtree.OwnedPrefix) {
				continue
			}

			where := fmt.Sprintf("is inside %q, a subtree", subtree.OwnedPrefix)
			if key.Path == subtree.OwnedPrefix {
				where = "is the root of a subtree"
			}

			problems = append(problems, fmt.Sprintf(
				"keys[%d]: path %q %s the generated code reads below under a segment it computes "+
					"itself (generated by %s): %s",
				i, key.Path, where, strings.Join(subtree.ReadBy, ", "), subtree.Description()))

			break
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf(errWrapFmt, ErrInvalidMap, problemList(ErrReservedPath, problems))
	}

	return nil
}

// isAtOrUnder reports whether path is prefix itself or sits below it, matching
// on segment boundaries so that /svc/bots is not "under" /svc/bot.
//
// The root counts: a declaration of the root of a subtree the generated code
// owns is the service claiming a node the generated code already reads below,
// and it would otherwise produce two rows for one path in the registry, one of
// them saying the path is not declarable.
func isAtOrUnder(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// ScopeAppliesTo reports whether a scope expands to anything for the given
// project — that is, whether the project has the dimension the scope names.
// Exported for the generator's own test that every scope is reachable: a scope
// no project can reach declares nothing, and the paths in it stay declarable by
// the service with nothing to say so.
func ScopeAppliesTo(scope string, surface ProjectSurface) bool {
	return len(surface.substitutions(scope)) > 0
}

// GenScopes returns the known declaration scopes, sorted, for error messages.
func GenScopes() []string {
	out := make([]string, 0, len(scopePlaceholders))

	for scope := range scopePlaceholders {
		out = append(out, scope)
	}

	sort.Strings(out)

	return out
}

// describeGenKey names what a declaration says a path is, for the message of a
// contradiction between two declarations of it.
func describeGenKey(valueType, accessor, readVia string, requiredAtInit bool) string {
	var out string

	switch {
	case valueType == "":
		return "a path read below"
	case readVia != "":
		out = "type " + valueType + " read through " + readVia
	default:
		out = "type " + valueType + " read through " + accessor
	}

	if requiredAtInit {
		out += requiredAtInitSuffix
	}

	return out
}

// describeGenAccessor names what a declaration says an accessor is.
func describeGenAccessor(name, valueType string, required, requiredAtInit bool) string {
	if valueType == "" {
		return "the path builder " + name
	}

	out := name + " of type " + valueType

	if required {
		out += ", required"
	}

	if requiredAtInit {
		out += requiredAtInitSuffix
	}

	return out
}

// isPlaceholder reports whether a path segment is a dimension placeholder.
func isPlaceholder(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

// isRuntimeParam reports whether a path segment is a runtime segment.
func isRuntimeParam(segment string) bool {
	return strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">")
}
