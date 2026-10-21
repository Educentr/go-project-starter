package generator

import (
	"strings"
	"testing"

	cfg "github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
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

	keys, err := generatorOnlineConfKeys(config)
	if err != nil {
		t.Fatalf("generatorOnlineConfKeys() error: %v", err)
	}

	if len(keys) == 0 {
		t.Fatal("a project with a REST server reads OnlineConf keys")
	}

	for _, key := range keys {
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
		"/svc/settings/enabled":               false,
	}

	for _, key := range keys {
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
