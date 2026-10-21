package ockeymap

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Declarations of the OnlineConf keys the GENERATOR's own code reads.
//
// The service map alone is not enough to make "no key is read from two places
// with two spellings" true: generated code reads OnlineConf too — transport
// timeouts, security toggles, worker and Kafka settings, the dev-stand flag.
// Until those reads are declared, two things are wrong at once. The service can
// declare a path generated code already reads, getting a second reader with a
// second spelling; and the owner of the contract cannot see the whole surface of
// the tree, because half of it exists only inside templates.
//
// So every template whose generated code reads a key carries a declaration file
// next to it (`<template>.ockeys.yaml`), and the paths in it are expanded at
// generation time against the actual project: transports, applications, workers,
// Kafka clients. Declaration next to the read is the point — the file a
// developer has open while adding a read is the file that must grow an entry,
// and TestOnlineConfKeyDeclarations fails when it does not.
//
// Reserving prefixes (`/svc/transport/**` and friends) was the earlier answer
// and the wrong one: the list was maintained by hand, nothing tied it to the
// templates, and subtrees the generated code genuinely reads — `worker/<name>`,
// `settings`, `bot` — were simply missing from it.

// ErrGenDeclDefect reports a broken declaration shipped with the generator: a
// declaration file that does not parse, names an unknown scope, uses a
// placeholder its scope cannot fill, or contradicts another declaration of the
// same path. It is a defect in this repository, not in the user's project, and
// the message says so.
var ErrGenDeclDefect = errors.New("generator onlineconf key declaration defect")

// GenDeclSuffix is the file name suffix of a declaration file. The owning
// template is the same path with the suffix removed: a declaration for
// `service.go.tmpl` is `service.go.tmpl.ockeys.yaml` in the same directory, so
// the mapping between a read and its declaration is mechanical and checkable.
const GenDeclSuffix = ".ockeys.yaml"

// Placeholders a declared path may carry. Which ones are available is decided by
// the group's scope — that is what makes a declaration expandable into the
// concrete paths of a concrete project instead of a pattern nobody can match.
const (
	PlaceholderService   = "{service}"
	PlaceholderTransport = "{transport}"
	PlaceholderApp       = "{app}"
	PlaceholderWorker    = "{worker}"
	PlaceholderClient    = "{client}"
	PlaceholderEvent     = "{event}"
)

// Declaration scopes — the project dimension a group of keys is read for, and
// therefore what the generator expands it over.
const (
	// ScopeService covers code every project gets, read once per service.
	ScopeService = "service"
	// ScopeRESTServer covers the REST server configuration, read per REST
	// server and per application serving it. A path that names no {app}
	// expands once per server. {transport} is the server name, which carries
	// the api version ("api_v1") — the name the generated main hands to
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

// scopePlaceholders is the closed set of placeholders each scope can fill. A
// path using anything else is a defect: it could never be expanded into a path
// that exists in a tree.
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

// GenKey is one declared read of the generated code.
type GenKey struct {
	// Path is the key path with placeholders, e.g.
	// {service}/transport/rest/{transport}/{app}/port. Required.
	Path string `yaml:"path"`
	// Type is the value type, one of the Type* constants. Set when the path
	// itself is read as a value; a declaration may carry only Dynamic, when
	// generated code reads below the path but not the path itself.
	Type string `yaml:"type"`
	// Dynamic marks a subtree the generated code reads under a key it computes
	// at runtime: the url path of a request, a telegram user id, an action name.
	// There is no single path to declare, so the subtree as a whole belongs to
	// the generator and the service map may not declare inside it.
	//
	// It combines with Type: /<svc>/bot/users is read both as a value (the
	// blanket ACL) and per telegram id below it.
	Dynamic bool `yaml:"dynamic"`
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
}

// GeneratorKey is one concrete OnlineConf path the generated code of a concrete
// project reads.
type GeneratorKey struct {
	// Path is the full path, expanded.
	Path string
	// Type is the value type, empty for a dynamic subtree.
	Type string
	// Dynamic reports a subtree read under a key computed at runtime.
	Dynamic bool
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

// Description returns the prose of the entry: every distinct reason the key is
// read, in declaration order, as one sentence-joined string.
func (k GeneratorKey) Description() string {
	return strings.Join(k.Descriptions, " ")
}

// genKeyFields is the closed set of fields a declaration entry may carry.
var genKeyFields = map[string]struct{}{
	"path": {}, "type": {}, "dynamic": {}, "default": {}, "description": {},
}

// genGroupFields is the closed set of fields a declaration group may carry.
var genGroupFields = map[string]struct{}{
	"scope": {}, "keys": {},
}

// UnmarshalYAML decodes a declaration entry, rejecting unknown fields.
func (k *GenKey) UnmarshalYAML(value *yaml.Node) error {
	type plain struct {
		Path        string `yaml:"path"`
		Type        string `yaml:"type"`
		Dynamic     bool   `yaml:"dynamic"`
		Default     string `yaml:"default"`
		Description string `yaml:"description"`
	}

	if err := rejectUnknownFields(value, genKeyFields); err != nil {
		return err
	}

	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}

	k.Path = strings.TrimSpace(p.Path)
	k.Type = p.Type
	k.Dynamic = p.Dynamic
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
		return nil, fmt.Errorf("%w: %s:\n  %s", ErrGenDeclDefect, origin, strings.Join(problems, "\n  "))
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
	var problems []string

	if k.Path == "" {
		return []string{"path is required"}
	}

	segments := pathSegments(normalizePath(k.Path))

	if len(segments) < minPathSegments {
		problems = append(problems, fmt.Sprintf(
			"path %q has no key segment: a path must be {service}/<key> at the least", k.Path))
	}

	if len(segments) > 0 && segments[0] != PlaceholderService {
		problems = append(problems, fmt.Sprintf(
			"path %q must start with %s: every key the generator reads lives in the service tree",
			k.Path, PlaceholderService))
	}

	for _, segment := range segments {
		switch {
		case isPlaceholder(segment):
			if !slices.Contains(allowed, segment) {
				problems = append(problems, fmt.Sprintf(
					"path %q uses %s, which this scope cannot fill (available: %s)",
					k.Path, segment, strings.Join(allowed, ", ")))
			}
		case !segmentRx.MatchString(segment):
			problems = append(problems, fmt.Sprintf("path %q has an invalid segment %q, expected %s or a placeholder",
				k.Path, segment, segmentRx.String()))
		}
	}

	problems = append(problems, k.typeProblems()...)

	if strings.TrimSpace(k.Description) == "" {
		problems = append(problems, fmt.Sprintf(
			"path %q has no description: the generated registry is where the owner of the contract "+
				"reads what a generator-owned key does", k.Path))
	}

	return problems
}

// typeProblems checks what the declaration says is read. `type` means the path
// itself is read as a value, `dynamic` means generated code reads below it under
// a runtime key; a declaration carrying neither says nothing is read and is a
// defect.
func (k GenKey) typeProblems() []string {
	if k.Type == "" {
		if !k.Dynamic {
			return []string{fmt.Sprintf(
				"path %q declares neither `type` nor `dynamic: true`, so it says nothing is read: "+
					"name the value type, or mark the subtree read under a runtime key", k.Path)}
		}

		return nil
	}

	if _, ok := knownTypes[k.Type]; !ok {
		return []string{fmt.Sprintf("path %q declares type %q, expected one of %s (or `dynamic: true`)",
			k.Path, k.Type, strings.Join(KnownTypes(), ", "))}
	}

	return nil
}

// ExpandGenDecls turns the generator's declarations into the concrete paths the
// generated code of this project reads.
//
// A path whose placeholders the project cannot fill is skipped rather than
// reported: a REST transport wired into no application has no {app} to expand,
// and a project with no Kafka producer reads no Kafka keys. Two declarations of
// the same path are merged — the dev-stand flag is read by several templates —
// but a disagreement about its type or its dynamic flag is a defect, because
// then the two readers do not mean the same key.
func ExpandGenDecls(decls []GenDeclFile, surface ProjectSurface) ([]GeneratorKey, error) {
	if surface.Service == "" {
		return nil, fmt.Errorf("%w: project surface has no service name", ErrGenDeclDefect)
	}

	merged := make(map[string]*GeneratorKey)

	var problems []string

	for _, decl := range decls {
		if decl.Decl == nil {
			continue
		}

		for _, group := range decl.Decl.Groups {
			for _, substitution := range surface.substitutions(group.Scope) {
				for _, key := range group.Keys {
					path, ok := expandPath(key.Path, substitution)
					if !ok {
						continue
					}

					if msg := mergeGeneratorKey(merged, path, key, decl.Owner); msg != "" {
						problems = append(problems, msg)
					}
				}
			}
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)

		return nil, problemList(ErrGenDeclDefect, problems)
	}

	out := make([]GeneratorKey, 0, len(merged))

	for _, key := range merged {
		sort.Strings(key.ReadBy)
		out = append(out, *key)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out, nil
}

// mergeGeneratorKey folds one expanded declaration into the result, returning a
// problem description when it contradicts an entry already there.
func mergeGeneratorKey(merged map[string]*GeneratorKey, path string, key GenKey, owner string) string {
	existing, seen := merged[path]
	if !seen {
		merged[path] = &GeneratorKey{
			Path:         path,
			Type:         key.Type,
			Dynamic:      key.Dynamic,
			Default:      key.Default,
			Descriptions: []string{key.Description},
			ReadBy:       []string{owner},
		}

		return ""
	}

	if existing.Type != key.Type || existing.Dynamic != key.Dynamic {
		return fmt.Sprintf("path %q is declared as %s by %s and as %s by %s",
			path, describeGenType(existing.Type, existing.Dynamic), strings.Join(existing.ReadBy, ", "),
			describeGenType(key.Type, key.Dynamic), owner)
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

	return ""
}

// substitutions returns one placeholder mapping per expansion of the scope over
// this project.
func (s ProjectSurface) substitutions(scope string) []map[string]string {
	base := map[string]string{PlaceholderService: s.Service}

	switch scope {
	case ScopeService:
		return []map[string]string{base}
	case ScopeRESTServer:
		return s.restServerSubstitutions()
	case ScopeRESTClient:
		return oneDimension(base, PlaceholderTransport, s.RESTClients)
	case ScopeGRPCClient:
		return oneDimension(base, PlaceholderTransport, s.GRPCClients)
	case ScopeKafkaProducer:
		clients := make([]string, 0, len(s.KafkaProducers))
		for _, producer := range s.KafkaProducers {
			clients = append(clients, producer.Client)
		}

		return oneDimension(base, PlaceholderClient, clients)
	case ScopeKafkaProducerEvent:
		return s.kafkaEventSubstitutions()
	case ScopeWorkerDaemon:
		return oneDimension(base, PlaceholderWorker, s.DaemonWorkers)
	case ScopeWorkerTelegram:
		return oneDimension(base, PlaceholderWorker, s.TelegramWorkers)
	case ScopeTelegramDriver:
		if len(s.TelegramWorkers) == 0 {
			return nil
		}

		return []map[string]string{base}
	default:
		// Unreachable: ParseGenDecl rejects an unknown scope.
		return nil
	}
}

// restServerSubstitutions yields one mapping per (transport, application) pair,
// plus a transport-only mapping so that the transport-level fallback paths —
// which name no {app} — expand for a transport serving no application too.
func (s ProjectSurface) restServerSubstitutions() []map[string]string {
	out := make([]map[string]string, 0, len(s.RESTServers))

	for _, server := range s.RESTServers {
		out = append(out, map[string]string{
			PlaceholderService:   s.Service,
			PlaceholderTransport: server.Transport,
		})

		for _, app := range server.Apps {
			out = append(out, map[string]string{
				PlaceholderService:   s.Service,
				PlaceholderTransport: server.Transport,
				PlaceholderApp:       app,
			})
		}
	}

	return out
}

// kafkaEventSubstitutions yields one mapping per (client, event) pair.
func (s ProjectSurface) kafkaEventSubstitutions() []map[string]string {
	out := make([]map[string]string, 0, len(s.KafkaProducers))

	for _, producer := range s.KafkaProducers {
		for _, event := range producer.Events {
			out = append(out, map[string]string{
				PlaceholderService: s.Service,
				PlaceholderClient:  producer.Client,
				PlaceholderEvent:   event,
			})
		}
	}

	return out
}

// oneDimension yields one mapping per value of a single placeholder.
func oneDimension(base map[string]string, placeholder string, values []string) []map[string]string {
	out := make([]map[string]string, 0, len(values))

	for _, value := range values {
		substitution := make(map[string]string, len(base)+1)
		for k, v := range base {
			substitution[k] = v
		}

		substitution[placeholder] = value
		out = append(out, substitution)
	}

	return out
}

// expandPath substitutes the placeholders of a declared path. It reports false
// when the path needs a placeholder the mapping does not carry — the expansion
// does not apply to this dimension of the project.
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

// CheckGeneratorKeys reports declarations of the service map that the generated
// code already reads.
//
// Two ways that happens. An exact path match is a key with two readers and two
// spellings — the thing the map exists to prevent. And a declaration at or
// inside a subtree the generated code reads under a runtime key (the url path
// of a request, a telegram user id, an action name) is the same collision, only
// invisible: there is no single generated path to compare against, so the
// subtree belongs to the generator as a whole.
func (m *Map) CheckGeneratorKeys(generated []GeneratorKey) error {
	if m == nil || len(generated) == 0 {
		return nil
	}

	exact := make(map[string]GeneratorKey, len(generated))

	dynamic := make([]GeneratorKey, 0, len(generated))

	for _, key := range generated {
		// Both, when a path is read as a value and read below: /<svc>/bot/users
		// is the blanket ACL and the root of the per-id ones.
		if key.Type != "" {
			exact[key.Path] = key
		}

		if key.Dynamic {
			dynamic = append(dynamic, key)
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

		for _, subtree := range dynamic {
			if !isAtOrUnder(key.Path, subtree.Path) {
				continue
			}

			where := fmt.Sprintf("is inside %q, a subtree", subtree.Path)
			if key.Path == subtree.Path {
				where = "is the root of a subtree"
			}

			problems = append(problems, fmt.Sprintf(
				"keys[%d]: path %q %s the generator reads itself under a "+
					"runtime key (generated by %s): %s",
				i, key.Path, where, strings.Join(subtree.ReadBy, ", "), subtree.Description()))

			break
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidMap, problemList(ErrReservedPath, problems))
	}

	return nil
}

// isAtOrUnder reports whether path is prefix itself or sits below it, matching
// on segment boundaries so that /svc/bots is not "under" /svc/bot.
//
// The root counts: a declaration of the root of a generated dynamic subtree is
// the service claiming a node the generated code already owns, and it would
// otherwise produce two rows for one path in the registry, one of them saying
// the path is not declarable.
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

// describeGenType names what a declaration says a path is, for the message of a
// contradiction between two declarations.
func describeGenType(valueType string, dynamic bool) string {
	switch {
	case valueType == "":
		return "a dynamic subtree"
	case dynamic:
		return "type " + valueType + " plus a dynamic subtree"
	default:
		return "type " + valueType
	}
}

// isPlaceholder reports whether a path segment is a placeholder rather than a
// literal segment.
func isPlaceholder(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}
