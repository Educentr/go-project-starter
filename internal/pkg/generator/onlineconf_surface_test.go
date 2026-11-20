package generator

import (
	"fmt"
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

// The surface decides which generator-owned keys exist for a project, and that
// decides two things at once: what the registry claims the service reads, and
// what the service's key map is forbidden to declare. A dimension the generator
// emits no reader for must not appear in either — reserving a path nothing reads
// is the mirror image of the reserved-prefix list this feature replaced.
func TestOnlineConfSurfaceCoversOnlyGeneratedReaders(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			{Name: "api", Version: "v1", GeneratorType: "ogen"},
			{Name: "sys", Version: "v1", GeneratorType: "template", GeneratorTemplate: "sys"},
			{Name: "billing", Version: "v2", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		GrpcList: cfg.GrpcList{
			{Name: "ItemService", GeneratorType: "buf_client"},
			// Not a client: reads neither host nor port.
			{Name: "InternalAPI", GeneratorType: "buf_server"},
		},
		KafkaList: cfg.KafkaList{
			{Name: "events", Type: ds.KafkaTypeProducer, Client: "main", Events: []cfg.KafkaEvent{{Name: "created"}}},
			// A consumer and a custom driver both read none of the producer
			// config keys: the generator emits no config.go for either.
			{Name: "orders", Type: "consumer", Client: "orders"},
			{
				Name: "audit", Type: ds.KafkaTypeProducer, Driver: ds.KafkaDriverCustom,
				Client: "audit", Events: []cfg.KafkaEvent{{Name: "logged"}},
			},
		},
		WorkerList: cfg.WorkerList{
			{Name: "reaper", GeneratorType: "template", GeneratorTemplate: "daemon"},
			{Name: "bot", GeneratorType: "template", GeneratorTemplate: "telegram"},
			// A queue worker's generated code reads no OnlineConf key at all.
			{Name: "tasks", GeneratorType: "template", GeneratorTemplate: "queue"},
		},
		Applications: []cfg.Application{
			{
				Name:          "web",
				TransportList: []cfg.AppTransport{{Name: "api"}, {Name: "sys"}},
			},
			{
				Name:          "admin",
				TransportList: []cfg.AppTransport{{Name: "api"}},
			},
		},
	}

	surface := onlineConfSurface(config)

	if surface.Service != "svc" {
		t.Errorf("Service = %q, want svc", surface.Service)
	}

	wantServers := map[string]string{"api_v1": "web,admin", "sys_v1": "web"}

	if len(surface.RESTServers) != len(wantServers) {
		t.Fatalf("RESTServers = %+v, want %d entries", surface.RESTServers, len(wantServers))
	}

	for _, server := range surface.RESTServers {
		wantApps, known := wantServers[server.Transport]
		if !known {
			t.Errorf("RESTServers carries %q, which is not a REST server", server.Transport)

			continue
		}

		if got := strings.Join(server.Apps, ","); got != wantApps {
			t.Errorf("transport %q apps = %q, want %q", server.Transport, got, wantApps)
		}
	}

	// A client reads its configuration under the package name, which carries
	// the api version.
	if strings.Join(surface.RESTClients, ",") != "billing_v2" {
		t.Errorf("RESTClients = %v, want [billing_v2]", surface.RESTClients)
	}

	if strings.Join(surface.GRPCClients, ",") != "ItemService" {
		t.Errorf("GRPCClients = %v, want only the buf_client transport", surface.GRPCClients)
	}

	if len(surface.KafkaProducers) != 1 ||
		surface.KafkaProducers[0].Client != "main" ||
		strings.Join(surface.KafkaProducers[0].Events, ",") != "created" {
		t.Errorf("KafkaProducers = %+v, want only the generated segmentio producer", surface.KafkaProducers)
	}

	if strings.Join(surface.DaemonWorkers, ",") != "reaper" {
		t.Errorf("DaemonWorkers = %v, want [reaper]", surface.DaemonWorkers)
	}

	if strings.Join(surface.TelegramWorkers, ",") != "bot" {
		t.Errorf("TelegramWorkers = %v, want [bot]", surface.TelegramWorkers)
	}
}

// The surface is the input of the expansion, so the end-to-end property worth
// pinning is the one the rejected design got wrong: a path the generated code
// does not read must not turn up among the generator's keys.
func TestGeneratorKeysExcludeAbsentReaders(t *testing.T) {
	config := cfg.Config{
		Main:     cfg.Main{Name: "svc"},
		RestList: cfg.RestList{{Name: "api", Version: "v1", GeneratorType: "ogen"}},
		KafkaList: cfg.KafkaList{{
			Name: "audit", Type: ds.KafkaTypeProducer, Driver: ds.KafkaDriverCustom,
			Client: "audit", Events: []cfg.KafkaEvent{{Name: "logged"}},
		}},
		WorkerList:   cfg.WorkerList{{Name: "tasks", GeneratorType: "template", GeneratorTemplate: "queue"}},
		Applications: []cfg.Application{{Name: "web", TransportList: []cfg.AppTransport{{Name: "api"}}}},
	}

	decls, err := templater.LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	generated, err := ockeymap.ExpandGenDecls(decls, onlineConfSurface(config))
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	if len(generated.Keys) == 0 {
		t.Fatal("a project with a REST server reads OnlineConf keys")
	}

	for _, key := range generated.Keys {
		for _, unwanted := range []string{"/svc/kafka/", "/svc/worker/", "/svc/bot/"} {
			if strings.HasPrefix(key.Path, unwanted) {
				t.Errorf("%s is claimed by %v, but nothing generated reads it", key.Path, key.ReadBy)
			}
		}
	}

	// And the keys that ARE read are there, expanded against this project.
	want := map[string]bool{
		"/svc/transport/rest/api_v1/web/port": false,
		"/svc/security/csrf/enabled":          false,
		"/svc/devstand":                       false,
		"/svc/settings/enabled/<action>":      false,
	}

	for _, key := range generated.Keys {
		if _, listed := want[key.Path]; listed {
			want[key.Path] = true
		}
	}

	for path, found := range want {
		if !found {
			t.Errorf("%s should be among the generator's keys", path)
		}
	}
}

// The startup check is per application on purpose: a required key belongs to a
// component, and a process that does not run that component never reads it.
// Checking the project.s whole set in every process would stop a REST-only
// application over the telegram token of a worker it never starts.
func TestAppRequiredChecksFollowTheApplication(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			{Name: "api", Version: "v1", GeneratorType: "ogen"},
			{Name: "billing", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		GrpcList: cfg.GrpcList{{Name: "items", GeneratorType: "buf_client"}},
		KafkaList: cfg.KafkaList{{
			Name: "orders", Type: ds.KafkaTypeProducer, Client: "orders",
			Events: []cfg.KafkaEvent{{Name: "created"}},
		}},
		WorkerList: cfg.WorkerList{{Name: "bot", GeneratorType: "template", GeneratorTemplate: "telegram"}},
		Applications: []cfg.Application{
			{Name: "web", TransportList: []cfg.AppTransport{{Name: "api"}}},
			{Name: "bot", WorkerList: []string{"bot"}},
			{Name: "producers", KafkaList: []cfg.AppKafka{{Name: "orders"}}},
			{
				Name: "integrations",
				TransportList: []cfg.AppTransport{
					{Name: "billing"},
					// Optional: the generated client skips an absent host, so the
					// startup check must not demand one.
					{Name: "items", Config: cfg.AppTransportConfig{Optional: true}},
				},
			},
		},
	}

	config.RestMap = map[string]cfg.Rest{}
	for _, rest := range config.RestList {
		config.RestMap[rest.Name] = rest
	}

	config.GrpcMap = map[string]cfg.Grpc{}
	for _, grpc := range config.GrpcList {
		config.GrpcMap[grpc.Name] = grpc
	}

	config.WorkerMap = map[string]cfg.Worker{}
	for _, worker := range config.WorkerList {
		config.WorkerMap[worker.Name] = worker
	}

	config.KafkaMap = map[string]cfg.Kafka{}
	for _, kafka := range config.KafkaList {
		config.KafkaMap[kafka.Name] = kafka
	}

	decls, err := templater.LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	want := map[string][]string{
		"web": {},
		"bot": {"/svc/security/tg_token"},
		// A client host is not `required`: a client declared
		// `instantiation: dynamic` never reads it, and one declared
		// `optional: true` is skipped when it is unset — whether the key is
		// needed is a project setting, not a property of the key.
		"integrations": {},
		"producers":    {"/svc/kafka/orders/brokers"},
	}

	for _, app := range config.Applications {
		surface, err := ockeymap.ExpandGenDecls(decls, onlineConfAppSurface(config, app))
		if err != nil {
			t.Fatalf("ExpandGenDecls(%s) error: %v", app.Name, err)
		}

		got := make([]string, 0, len(surface.Required))
		for _, check := range surface.Required {
			got = append(got, check.Path)
		}

		if strings.Join(got, ",") != strings.Join(want[app.Name], ",") {
			t.Errorf("application %s requires %v, want %v", app.Name, got, want[app.Name])
		}
	}
}

// Every scope a declaration can name has to be reachable from a project
// configuration, or the declarations in it are dead and the paths they describe
// silently stay declarable by the service.
func TestEveryScopeIsReachableFromAProject(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			{Name: "api", Version: "v1", GeneratorType: "ogen"},
			{Name: "billing", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		GrpcList: cfg.GrpcList{{Name: "items", GeneratorType: "buf_client"}},
		KafkaList: cfg.KafkaList{{
			Name: "events", Type: ds.KafkaTypeProducer, Client: "main",
			Events: []cfg.KafkaEvent{{Name: "created"}},
		}},
		WorkerList: cfg.WorkerList{
			{Name: "reaper", GeneratorType: "template", GeneratorTemplate: "daemon"},
			{Name: "bot", GeneratorType: "template", GeneratorTemplate: "telegram"},
		},
		Applications: []cfg.Application{{Name: "web", TransportList: []cfg.AppTransport{{Name: "api"}}}},
	}

	surface := onlineConfSurface(config)

	for _, scope := range ockeymap.GenScopes() {
		if !ockeymap.ScopeAppliesTo(scope, surface) {
			t.Errorf("scope %q expands to nothing even for a project with one of everything", scope)
		}
	}
}

// The tolerant set is what keeps `required_at_init` from demanding
// configuration the project says is not needed. Two settings put a component
// there, and they are different reasons: `optional: true` means the generated
// reader skips the component when the key is unset, `instantiation: dynamic`
// means the generated client takes its address as an argument and reads no key
// at all.
func TestOnlineConfSurfaceMarksTolerantComponents(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			{Name: "api", Version: "v1", GeneratorType: "ogen"},
			// Optional server: an application willing to run without it.
			{Name: "extra", Version: "v1", GeneratorType: "ogen"},
			{Name: "billing", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
			{
				Name: "audit", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient,
				Instantiation: cfg.InstantiationDynamic,
			},
			{Name: "legacy", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		GrpcList: cfg.GrpcList{
			{Name: "ItemService", GeneratorType: "buf_client"},
			{Name: "Reporting", GeneratorType: "buf_client", Instantiation: cfg.InstantiationDynamic},
		},
		KafkaList: cfg.KafkaList{
			{Name: "events", Type: ds.KafkaTypeProducer, Client: "main", Events: []cfg.KafkaEvent{{Name: "created"}}},
			{Name: "audits", Type: ds.KafkaTypeProducer, Client: "audits", Events: []cfg.KafkaEvent{{Name: "logged"}}},
		},
		Applications: []cfg.Application{{
			Name: "web",
			TransportList: []cfg.AppTransport{
				{Name: "api"},
				{Name: "extra", Config: cfg.AppTransportConfig{Optional: true}},
				{Name: "billing"},
				{Name: "audit"},
				// A client the project declares static and this application
				// wires dynamically reads nothing either.
				{Name: "legacy", Config: cfg.AppTransportConfig{Instantiation: cfg.InstantiationDynamic}},
				{Name: "ItemService"},
				{Name: "Reporting"},
			},
			KafkaList: []cfg.AppKafka{{Name: "events"}, {Name: "audits", Optional: true}},
		}},
	}

	surface := onlineConfSurface(config)

	want := map[ockeymap.TolerantComponent]bool{
		{Dimension: ockeymap.DimensionRESTServer, Name: "api_v1"}:      false,
		{Dimension: ockeymap.DimensionRESTServer, Name: "extra_v1"}:    true,
		{Dimension: ockeymap.DimensionRESTClient, Name: "billing_v1"}:  false,
		{Dimension: ockeymap.DimensionRESTClient, Name: "audit_v1"}:    true,
		{Dimension: ockeymap.DimensionRESTClient, Name: "legacy_v1"}:   true,
		{Dimension: ockeymap.DimensionGRPCClient, Name: "ItemService"}: false,
		{Dimension: ockeymap.DimensionGRPCClient, Name: "Reporting"}:   true,
		{Dimension: ockeymap.DimensionKafkaClient, Name: "main"}:       false,
		{Dimension: ockeymap.DimensionKafkaClient, Name: "audits"}:     true,
	}

	for component, tolerant := range want {
		if surface.Tolerant[component] != tolerant {
			t.Errorf("Tolerant[%v] = %v, want %v", component, surface.Tolerant[component], tolerant)
		}
	}
}

// One static wiring is one Init that fails without the address, so the mode of
// a client is dynamic only when every application wires it that way.
func TestReadsItsAddressFollowsEveryWiring(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		wirings  []string
		want     bool
	}{
		{name: "static, unset", declared: "", wirings: []string{""}, want: true},
		{name: "static, declared", declared: cfg.InstantiationStatic, wirings: []string{""}, want: true},
		{name: "dynamic everywhere", declared: cfg.InstantiationDynamic, wirings: []string{""}, want: false},
		{
			name:     "dynamic, one application overrides to static",
			declared: cfg.InstantiationDynamic, wirings: []string{"", cfg.InstantiationStatic}, want: true,
		},
		{
			name:     "static, every application overrides to dynamic",
			declared: cfg.InstantiationStatic,
			wirings:  []string{cfg.InstantiationDynamic, cfg.InstantiationDynamic}, want: false,
		},
		{name: "wired nowhere, static", declared: "", wirings: nil, want: true},
		{name: "wired nowhere, dynamic", declared: cfg.InstantiationDynamic, wirings: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := cfg.Config{Main: cfg.Main{Name: "svc"}}

			for i, wiring := range tt.wirings {
				config.Applications = append(config.Applications, cfg.Application{
					Name: string(rune('a' + i)),
					TransportList: []cfg.AppTransport{
						{Name: "other"},
						{Name: "client", Config: cfg.AppTransportConfig{Instantiation: wiring}},
					},
				})
			}

			if got := readsItsAddress(config, "client", tt.declared); got != tt.want {
				t.Errorf("readsItsAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// One wiring without `optional` is one process that refuses to start without
// the key, so a component is tolerant only when every application that wires
// it declares it optional.
//
// This is the defect the whole flag exists to prevent, one level down: a client
// optional in the REST application and mandatory in the worker was published as
// needing no configuration, while the worker's ValidateFor<App> refused to
// start without it — a crash-loop no guard over the registry could have caught.
func TestOptionalEverywhereFollowsEveryWiring(t *testing.T) {
	// Each wiring is one application; true means it declares the component
	// optional.
	tests := []struct {
		name    string
		wirings []bool
		want    bool
	}{
		{name: "the only wiring is mandatory", wirings: []bool{false}, want: false},
		{name: "the only wiring is optional", wirings: []bool{true}, want: true},
		{name: "optional in one application, mandatory in another", wirings: []bool{true, false}, want: false},
		{name: "optional in every application", wirings: []bool{true, true}, want: true},
		// Nothing of the project answers for a component no application wires,
		// the same way readsItsAddress answers for an unwired static client.
		{name: "wired nowhere", wirings: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, kind := range []string{"transport", "kafka"} {
				config := cfg.Config{Main: cfg.Main{Name: "svc"}}

				for i, optional := range tt.wirings {
					app := cfg.Application{Name: string(rune('a' + i))}

					if kind == "transport" {
						app.TransportList = []cfg.AppTransport{
							{Name: "other"},
							{Name: "component", Config: cfg.AppTransportConfig{Optional: optional}},
						}
					} else {
						app.KafkaList = []cfg.AppKafka{
							{Name: "other"},
							{Name: "component", Optional: optional},
						}
					}

					config.Applications = append(config.Applications, app)
				}

				if got := optionalEverywhere(config, "component"); got != tt.want {
					t.Errorf("optionalEverywhere(%s) = %v, want %v", kind, got, tt.want)
				}
			}
		})
	}
}

// The surface is what carries that answer into the expansion: a client wired
// optional in one application and mandatory in another is not tolerant, so the
// registry marks its host as a key the project has to seed.
func TestOnlineConfSurfaceTolerantNeedsEveryWiringOptional(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			{Name: "api", Version: "v1", GeneratorType: "ogen"},
			{Name: "billing", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
			{Name: "audit", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		KafkaList: cfg.KafkaList{
			{Name: "events", Type: ds.KafkaTypeProducer, Client: "main", Events: []cfg.KafkaEvent{{Name: "created"}}},
		},
		Applications: []cfg.Application{
			{
				Name: "api",
				TransportList: []cfg.AppTransport{
					{Name: "api"},
					{Name: "billing", Config: cfg.AppTransportConfig{Optional: true}},
					{Name: "audit", Config: cfg.AppTransportConfig{Optional: true}},
				},
				KafkaList: []cfg.AppKafka{{Name: "events", Optional: true}},
			},
			{
				Name: "worker",
				// The same client, wired without `optional`: this process does
				// not come up without the host.
				TransportList: []cfg.AppTransport{{Name: "billing"}},
				KafkaList:     []cfg.AppKafka{{Name: "events", Optional: true}},
			},
		},
	}

	surface := onlineConfSurface(config)

	want := map[ockeymap.TolerantComponent]bool{
		{Dimension: ockeymap.DimensionRESTClient, Name: "billing_v1"}: false,
		{Dimension: ockeymap.DimensionRESTClient, Name: "audit_v1"}:   true,
		{Dimension: ockeymap.DimensionKafkaClient, Name: "main"}:      true,
	}

	for component, tolerant := range want {
		if surface.Tolerant[component] != tolerant {
			t.Errorf("Tolerant[%v] = %v, want %v", component, surface.Tolerant[component], tolerant)
		}
	}
}

// A Kafka client is one segment and one brokers key, and several producers may
// be configured under it. Tolerance is therefore folded over all of them with
// AND: one producer the project can do without must not answer for one it
// cannot.
//
// Taken from any single producer, as it used to be, this is the crash-loop the
// flag exists to prevent — the producer optional everywhere cleared the flags
// of a client whose other producer the worker could not start without, and the
// startup check of that worker stayed where it was.
func TestOnlineConfSurfaceKafkaToleranceFoldsEveryProducerOfTheClient(t *testing.T) {
	// Each entry is one producer under the shared client: true means every
	// application wiring it declares it optional.
	tests := []struct {
		name      string
		producers []bool
		want      bool
	}{
		{name: "the only producer is mandatory", producers: []bool{false}, want: false},
		{name: "the only producer is optional", producers: []bool{true}, want: true},
		{name: "one optional, one mandatory", producers: []bool{true, false}, want: false},
		{name: "mandatory first, optional second", producers: []bool{false, true}, want: false},
		{name: "every producer optional", producers: []bool{true, true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := cfg.Config{Main: cfg.Main{Name: "svc"}}
			app := cfg.Application{Name: "worker"}

			for i, optional := range tt.producers {
				name := fmt.Sprintf("producer%d", i)

				config.KafkaList = append(config.KafkaList, cfg.Kafka{
					Name: name, Type: ds.KafkaTypeProducer, Client: "shared",
					Events: []cfg.KafkaEvent{{Name: name}},
				})

				app.KafkaList = append(app.KafkaList, cfg.AppKafka{Name: name, Optional: optional})
			}

			config.Applications = []cfg.Application{app}

			client := ockeymap.TolerantComponent{Dimension: ockeymap.DimensionKafkaClient, Name: "shared"}
			if got := onlineConfSurface(config).Tolerant[client]; got != tt.want {
				t.Errorf("Tolerant[%v] = %v, want %v", client, got, tt.want)
			}
		})
	}
}

// Tolerance belongs to a component, and a component is a name plus the kind of
// thing it names. Nothing keeps the names of two kinds apart: a gRPC client
// `audit` and a Kafka client `audit` are different components reading different
// subtrees, and so are a gRPC client and the name_version segment of a REST
// one.
//
// Answered by the name alone, the component the project can do without answers
// for the one it cannot — and the answer that travels is "needs no
// configuration", so the key a process does not start without leaves the
// registry while its startup check stays where it is. Rare, silent, and the
// same crash-loop as the rest of this flag's history.
func TestOnlineConfSurfaceToleranceIsPerDimension(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		RestList: cfg.RestList{
			// Reads under audit_v1_v1 — the segment of the gRPC client below.
			{Name: "audit_v1", Version: "v1", GeneratorType: cfg.GeneratorTypeOgenClient},
		},
		GrpcList: cfg.GrpcList{
			{Name: "audit", GeneratorType: grpcGeneratorBufClient},
			{Name: "audit_v1_v1", GeneratorType: grpcGeneratorBufClient},
		},
		KafkaList: cfg.KafkaList{
			{
				Name: "audit_producer", Type: ds.KafkaTypeProducer, Client: "audit",
				Events: []cfg.KafkaEvent{{Name: "logged"}},
			},
		},
		Applications: []cfg.Application{{
			Name: "worker",
			TransportList: []cfg.AppTransport{
				{Name: "audit", Config: cfg.AppTransportConfig{Optional: true}},
				{Name: "audit_v1", Config: cfg.AppTransportConfig{Optional: true}},
				// Mandatory, and sharing its segment with the optional REST
				// client above.
				{Name: "audit_v1_v1"},
			},
			KafkaList: []cfg.AppKafka{{Name: "audit_producer"}},
		}},
	}

	decls, err := templater.LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	generated, err := ockeymap.ExpandGenDecls(decls, onlineConfSurface(config))
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	// The brokers of the Kafka client, and the address of the mandatory gRPC
	// client: both are keys this project's worker does not come up without.
	want := map[string]bool{
		"/svc/kafka/audit/brokers":             true,
		"/svc/transport/grpc/audit_v1_v1/host": true,
		"/svc/transport/grpc/audit_v1_v1/port": true,
		"/svc/transport/grpc/audit/host":       false,
		"/svc/transport/rest/audit_v1_v1/host": false,
	}

	for _, key := range generated.Keys {
		requiredAtInit, checked := want[key.Path]
		if !checked {
			continue
		}

		if key.RequiredAtInit != requiredAtInit {
			t.Errorf("%s: RequiredAtInit = %v, want %v", key.Path, key.RequiredAtInit, requiredAtInit)
		}

		delete(want, key.Path)
	}

	for path := range want {
		t.Errorf("registry should list %s", path)
	}
}

// TestAppRequiredChecksFollowTheWiring is the pin on the agreement between the
// registry and the generated startup checks.
//
// The registry answers "does any process of this project refuse to start
// without the key" and the check answers "does THIS application", so the two
// have to read `optional: true` the same way: as a setting of the wiring. Read
// project-wide by the check, as it used to be, a producer optional in one
// application and mandatory in another was published as a key to seed while no
// application checked it — a guard demanding configuration nothing verifies,
// and for Kafka nothing else stops the start: the producer is never nil, so
// ValidateFor<App> says nothing about it.
func TestAppRequiredChecksFollowTheWiring(t *testing.T) {
	config := cfg.Config{
		Main: cfg.Main{Name: "svc"},
		KafkaList: cfg.KafkaList{
			{
				Name: "audits", Type: ds.KafkaTypeProducer, Client: "audits",
				Events: []cfg.KafkaEvent{{Name: "logged"}},
			},
			{
				Name: "events", Type: ds.KafkaTypeProducer, Client: "main",
				Events: []cfg.KafkaEvent{{Name: "created"}},
			},
		},
		Applications: []cfg.Application{
			{
				Name: "api",
				KafkaList: []cfg.AppKafka{
					{Name: "audits", Optional: true},
					{Name: "events", Optional: true},
				},
			},
			{
				Name: "worker",
				// The same producer, wired without `optional`: this process
				// does not come up without the brokers.
				KafkaList: []cfg.AppKafka{{Name: "audits"}},
			},
		},
	}

	config.KafkaMap = map[string]cfg.Kafka{}
	for _, kafka := range config.KafkaList {
		config.KafkaMap[kafka.Name] = kafka
	}

	decls, err := templater.LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	want := map[string][]string{
		// Both producers are optional here, so neither key is checked.
		"api": {},
		// The mandatory wiring is the one that needs the brokers.
		"worker": {"/svc/kafka/audits/brokers"},
	}

	checked := map[string]bool{}

	for _, app := range config.Applications {
		surface, err := ockeymap.ExpandGenDecls(decls, onlineConfAppSurface(config, app))
		if err != nil {
			t.Fatalf("ExpandGenDecls(%s) error: %v", app.Name, err)
		}

		got := make([]string, 0, len(surface.Required))

		for _, check := range surface.Required {
			got = append(got, check.Path)
			checked[check.Path] = true
		}

		if strings.Join(got, ",") != strings.Join(want[app.Name], ",") {
			t.Errorf("application %s requires %v, want %v", app.Name, got, want[app.Name])
		}
	}

	// And the registry says the same about those keys as the checks do. This is
	// the invariant the published flag rests on: a guard may take every
	// `required_at_init` row of the registry as a key to seed, because each one
	// is a key some process of the project verifies at startup.
	generated, err := ockeymap.ExpandGenDecls(decls, onlineConfSurface(config))
	if err != nil {
		t.Fatalf("ExpandGenDecls(project) error: %v", err)
	}

	published := map[string]ockeymap.GeneratorKey{}

	for _, key := range generated.Keys {
		published[key.Path] = key

		if key.Required && !checked[key.Path] {
			t.Errorf("%s is published as required, but no application checks it", key.Path)
		}
	}

	for path := range checked {
		if key, ok := published[path]; !ok || !key.Required || !key.RequiredAtInit {
			t.Errorf("%s is checked at startup, but the registry does not publish it as required", path)
		}
	}
}
