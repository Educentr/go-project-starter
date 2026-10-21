package ockeymap_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// fullSurface is a project with one of everything, so that an expansion test
// covers every scope at once.
func fullSurface() ockeymap.ProjectSurface {
	return ockeymap.ProjectSurface{
		Service: "svc",
		RESTServers: []ockeymap.RESTServer{
			{Transport: "api", Apps: []string{"web", "admin"}},
			{Transport: "sys", Apps: []string{"web"}},
		},
		RESTClients:     []string{"billing_v1"},
		GRPCClients:     []string{"users"},
		KafkaProducers:  []ockeymap.KafkaProducer{{Client: "main", Events: []string{"created", "updated"}}},
		DaemonWorkers:   []string{"reaper"},
		TelegramWorkers: []string{"bot"},
	}
}

func parseDecl(t *testing.T, yaml string) ockeymap.GenDeclFile {
	t.Helper()

	decl, err := ockeymap.ParseGenDecl([]byte(yaml), "test.go.tmpl"+ockeymap.GenDeclSuffix)
	if err != nil {
		t.Fatalf("ParseGenDecl() error: %v", err)
	}

	return ockeymap.GenDeclFile{Owner: "test.go.tmpl", Decl: decl}
}

func expand(t *testing.T, yaml string, surface ockeymap.ProjectSurface) []ockeymap.GeneratorKey {
	t.Helper()

	keys, err := ockeymap.ExpandGenDecls([]ockeymap.GenDeclFile{parseDecl(t, yaml)}, surface)
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	return keys
}

func paths(keys []ockeymap.GeneratorKey) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key.Path)
	}

	return out
}

// A declaration expands over the dimension its scope names — that is the whole
// point of declaring a pattern rather than a path: the generated readers build
// their paths out of transport, application, worker and client names, and so
// must the declaration.
func TestExpandCoversEveryScope(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		paths []string
	}{
		{
			name:  "service",
			yaml:  "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        type: bool\n        description: d\n",
			paths: []string{"/svc/devstand"},
		},
		{
			name: "rest server, per transport and per application",
			yaml: "groups:\n  - scope: rest_server\n    keys:\n" +
				"      - path: \"{service}/transport/rest/{transport}/port\"\n        type: string\n        description: d\n" +
				"      - path: \"{service}/transport/rest/{transport}/{app}/port\"\n        type: string\n        description: d\n",
			paths: []string{
				"/svc/transport/rest/api/admin/port",
				"/svc/transport/rest/api/port",
				"/svc/transport/rest/api/web/port",
				"/svc/transport/rest/sys/port",
				"/svc/transport/rest/sys/web/port",
			},
		},
		{
			name: "rest client",
			yaml: "groups:\n  - scope: rest_client\n    keys:\n      - path: \"{service}/transport/rest/{transport}/host\"\n" +
				"        type: string\n        description: d\n",
			paths: []string{"/svc/transport/rest/billing_v1/host"},
		},
		{
			name: "grpc client",
			yaml: "groups:\n  - scope: grpc_client\n    keys:\n      - path: \"{service}/transport/grpc/{transport}/host\"\n" +
				"        type: string\n        description: d\n",
			paths: []string{"/svc/transport/grpc/users/host"},
		},
		{
			name: "kafka producer",
			yaml: "groups:\n  - scope: kafka_producer\n    keys:\n      - path: \"{service}/kafka/{client}/brokers\"\n" +
				"        type: string\n        description: d\n",
			paths: []string{"/svc/kafka/main/brokers"},
		},
		{
			name: "kafka producer event",
			yaml: "groups:\n  - scope: kafka_producer_event\n    keys:\n" +
				"      - path: \"{service}/kafka/{client}/events/{event}/topic\"\n        type: string\n        description: d\n",
			paths: []string{"/svc/kafka/main/events/created/topic", "/svc/kafka/main/events/updated/topic"},
		},
		{
			name: "daemon worker",
			yaml: "groups:\n  - scope: worker_daemon\n    keys:\n      - path: \"{service}/worker/{worker}/batch_size\"\n" +
				"        type: int\n        description: d\n",
			paths: []string{"/svc/worker/reaper/batch_size"},
		},
		{
			name: "telegram worker",
			yaml: "groups:\n  - scope: worker_telegram\n    keys:\n      - path: \"{service}/worker/{worker}/job/timeout\"\n" +
				"        type: duration\n        description: d\n",
			paths: []string{"/svc/worker/bot/job/timeout"},
		},
		{
			name: "telegram driver, once per project",
			yaml: "groups:\n  - scope: telegram_driver\n    keys:\n      - path: \"{service}/security/tg_token\"\n" +
				"        type: string\n        description: d\n",
			paths: []string{"/svc/security/tg_token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(expand(t, tt.yaml, fullSurface()))

			if strings.Join(got, " ") != strings.Join(tt.paths, " ") {
				t.Errorf("ExpandGenDecls() paths =\n  %v\nwant\n  %v", got, tt.paths)
			}
		})
	}
}

// A project without the dimension a declaration names reads none of its keys.
// Without this the surface of, say, a REST-only project would reserve the whole
// Kafka and telegram subtrees against its own key map for no reason.
func TestExpandSkipsAbsentDimensions(t *testing.T) {
	yaml := "groups:\n" +
		"  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        type: bool\n        description: d\n" +
		"  - scope: kafka_producer\n    keys:\n      - path: \"{service}/kafka/{client}/brokers\"\n        type: string\n        description: d\n" +
		"  - scope: telegram_driver\n    keys:\n      - path: \"{service}/bot/admins\"\n        dynamic: true\n        description: d\n"

	got := paths(expand(t, yaml, ockeymap.ProjectSurface{Service: "svc"}))

	if len(got) != 1 || got[0] != "/svc/devstand" {
		t.Errorf("a project with no Kafka and no telegram worker should read only /svc/devstand, got %v", got)
	}
}

// A REST transport wired into no application still has its transport-level
// paths: the generated provider reads them whether or not an application name
// is available.
func TestExpandRESTServerWithoutApplications(t *testing.T) {
	yaml := "groups:\n  - scope: rest_server\n    keys:\n" +
		"      - path: \"{service}/transport/rest/{transport}/port\"\n        type: string\n        description: d\n" +
		"      - path: \"{service}/transport/rest/{transport}/{app}/port\"\n        type: string\n        description: d\n"

	got := paths(expand(t, yaml, ockeymap.ProjectSurface{
		Service:     "svc",
		RESTServers: []ockeymap.RESTServer{{Transport: "api"}},
	}))

	if len(got) != 1 || got[0] != "/svc/transport/rest/api/port" {
		t.Errorf("want only the transport-level path, got %v", got)
	}
}

// Several templates read the same key — the dev-stand flag is read by the
// logger and by the ogen error responses — so the same path must merge into one
// entry naming both readers, not into two entries.
func TestExpandMergesSharedPaths(t *testing.T) {
	decl := "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        type: bool\n        description: d\n"

	first := parseDecl(t, decl)
	second := parseDecl(t, decl)
	second.Owner = "other.go.tmpl"

	keys, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	if len(keys) != 1 {
		t.Fatalf("want one merged entry, got %d: %v", len(keys), paths(keys))
	}

	if strings.Join(keys[0].ReadBy, ",") != "other.go.tmpl,test.go.tmpl" {
		t.Errorf("merged entry should name both readers, sorted, got %v", keys[0].ReadBy)
	}
}

// Two templates disagreeing about what a path is means they do not mean the
// same key, which no merge can paper over.
func TestExpandRejectsContradiction(t *testing.T) {
	first := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: bool\n        description: d\n")

	second := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: int\n        description: d\n")
	second.Owner = "other.go.tmpl"

	_, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}

	if !strings.Contains(err.Error(), "type bool") || !strings.Contains(err.Error(), "type int") {
		t.Errorf("the error should name both declarations, got: %v", err)
	}
}

// A broken declaration is a defect in this repository, and the error says so
// rather than blaming the user's project.
func TestParseGenDeclRejectsDefects(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantMsg string
	}{
		{
			name:    "empty",
			yaml:    "   \n",
			wantMsg: "is empty",
		},
		{
			name:    "no groups",
			yaml:    "groups: []\n",
			wantMsg: "no groups declared",
		},
		{
			name:    "unknown scope",
			yaml:    "groups:\n  - scope: nowhere\n    keys:\n      - path: \"{service}/a\"\n        type: bool\n        description: d\n",
			wantMsg: `scope "nowhere" is unknown`,
		},
		{
			name:    "unknown field",
			yaml:    "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: bool\n        description: d\n        owner: me\n",
			wantMsg: "unknown field",
		},
		{
			name: "placeholder the scope cannot fill",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/worker/{worker}/x\"\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "which this scope cannot fill",
		},
		{
			name:    "not rooted in the service tree",
			yaml:    "groups:\n  - scope: service\n    keys:\n      - path: \"other/a\"\n        type: bool\n        description: d\n",
			wantMsg: "must start with {service}",
		},
		{
			name:    "neither type nor dynamic",
			yaml:    "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        description: d\n",
			wantMsg: "declares neither `type` nor `dynamic: true`",
		},
		{
			name:    "unknown type",
			yaml:    "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: float\n        description: d\n",
			wantMsg: `declares type "float"`,
		},
		{
			name:    "no description",
			yaml:    "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: bool\n",
			wantMsg: "has no description",
		},
		{
			name:    "no keys",
			yaml:    "groups:\n  - scope: service\n    keys: []\n",
			wantMsg: "declares no keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ockeymap.ParseGenDecl([]byte(tt.yaml), "test"+ockeymap.GenDeclSuffix)
			if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
				t.Fatalf("ParseGenDecl() error = %v, want ErrGenDeclDefect", err)
			}

			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
		})
	}
}

// A path read as a value AND below it: /svc/bot/users is the blanket ACL and the
// root of the per-user ones, so the declaration carries both and the service map
// is kept out of either.
func TestExpandAllowsValueAndSubtreeOnOnePath(t *testing.T) {
	keys := expand(t,
		"groups:\n  - scope: service\n    keys:\n      - path: \"{service}/bot/users\"\n"+
			"        type: string\n        dynamic: true\n        description: d\n",
		ockeymap.ProjectSurface{Service: "svc"})

	if len(keys) != 1 || keys[0].Type != "string" || !keys[0].Dynamic {
		t.Fatalf("want one entry that is both a value and a subtree, got %+v", keys)
	}
}

func TestCheckGeneratorKeys(t *testing.T) {
	generated := []ockeymap.GeneratorKey{
		{
			Path: "/svc/security/csrf/enabled", Type: "bool",
			ReadBy: []string{"restconfig.go.tmpl"}, Description: "csrf",
		},
		{
			Path: "/svc/settings/enabled", Dynamic: true,
			ReadBy: []string{"service.go.tmpl"}, Description: "per-action allow lists",
		},
		{
			Path: "/svc/bot/users", Type: "string", Dynamic: true,
			ReadBy: []string{"telegram.go.tmpl"}, Description: "user acl",
		},
	}

	tests := []struct {
		name     string
		yaml     string
		wantErr  bool
		wantMsgs []string
	}{
		{
			name: "no collision",
			yaml: "keys:\n  - path: /svc/supervisor/min_lifetime\n    type: duration\n",
		},
		{
			name:     "exact path the generator reads",
			yaml:     "keys:\n  - path: /svc/security/csrf/enabled\n    type: bool\n",
			wantErr:  true,
			wantMsgs: []string{"reads itself", "restconfig.go.tmpl", "csrf"},
		},
		{
			name:     "inside a dynamic subtree",
			yaml:     "keys:\n  - path: /svc/settings/enabled/login\n    type: strings\n",
			wantErr:  true,
			wantMsgs: []string{"/svc/settings/enabled", "runtime key", "service.go.tmpl"},
		},
		{
			name:     "under a path that is both a value and a subtree",
			yaml:     "keys:\n  - path: /svc/bot/users/42\n    type: string\n",
			wantErr:  true,
			wantMsgs: []string{"/svc/bot/users", "telegram.go.tmpl"},
		},
		{
			name:    "a sibling whose name merely starts the same",
			yaml:    "keys:\n  - path: /svc/settings/enabledness\n    type: bool\n",
			wantErr: false,
		},
		{
			name:    "a parent of a generated path is not a collision",
			yaml:    "keys:\n  - path: /svc/security/csrf\n    type: struct\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyMap, err := ockeymap.Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			err = keyMap.CheckGeneratorKeys(generated)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CheckGeneratorKeys() error = %v, want nil", err)
				}

				return
			}

			if !errors.Is(err, ockeymap.ErrInvalidMap) || !errors.Is(err, ockeymap.ErrReservedPath) {
				t.Fatalf("CheckGeneratorKeys() error = %v, want ErrInvalidMap wrapping ErrReservedPath", err)
			}

			for _, want := range tt.wantMsgs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestCheckGeneratorKeysWithoutGeneratedKeys(t *testing.T) {
	keyMap, err := ockeymap.Parse([]byte("keys:\n  - path: /svc/a\n    type: bool\n"))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if err := keyMap.CheckGeneratorKeys(nil); err != nil {
		t.Errorf("CheckGeneratorKeys(nil) error = %v, want nil", err)
	}
}

func TestExpandRejectsSurfaceWithoutService(t *testing.T) {
	decl := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        type: bool\n        description: d\n")

	_, err := ockeymap.ExpandGenDecls([]ockeymap.GenDeclFile{decl}, ockeymap.ProjectSurface{})
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}
}
