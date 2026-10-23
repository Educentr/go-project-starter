package ockeymap_test

import (
	"errors"
	"slices"
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

func expand(t *testing.T, yaml string, surface ockeymap.ProjectSurface) *ockeymap.GenSurface {
	t.Helper()

	got, err := ockeymap.ExpandGenDecls([]ockeymap.GenDeclFile{parseDecl(t, yaml)}, surface)
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	return got
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
			name: "service",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n" +
				"        name: DevStand\n        type: bool\n        description: d\n",
			paths: []string{"/svc/devstand"},
		},
		{
			name: "rest server, per transport and per application",
			yaml: "groups:\n  - scope: rest_server\n    keys:\n" +
				"      - path: \"{service}/transport/rest/{transport}/port\"\n        name: Port\n" +
				"        type: string\n        description: d\n" +
				"      - path: \"{service}/transport/rest/{transport}/{app}/port\"\n        name: AppPort\n" +
				"        type: string\n        description: d\n",
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
				"        name: ClientHost\n        type: string\n        description: d\n",
			paths: []string{"/svc/transport/rest/billing_v1/host"},
		},
		{
			name: "grpc client",
			yaml: "groups:\n  - scope: grpc_client\n    keys:\n      - path: \"{service}/transport/grpc/{transport}/host\"\n" +
				"        name: GRPCHost\n        type: string\n        description: d\n",
			paths: []string{"/svc/transport/grpc/users/host"},
		},
		{
			name: "kafka producer",
			yaml: "groups:\n  - scope: kafka_producer\n    keys:\n      - path: \"{service}/kafka/{client}/brokers\"\n" +
				"        name: Brokers\n        type: string\n        description: d\n",
			paths: []string{"/svc/kafka/main/brokers"},
		},
		{
			name: "kafka producer event",
			yaml: "groups:\n  - scope: kafka_producer_event\n    keys:\n" +
				"      - path: \"{service}/kafka/{client}/events/{event}/topic\"\n        name: Topic\n" +
				"        type: string\n        description: d\n",
			paths: []string{"/svc/kafka/main/events/created/topic", "/svc/kafka/main/events/updated/topic"},
		},
		{
			name: "daemon worker",
			yaml: "groups:\n  - scope: worker_daemon\n    keys:\n      - path: \"{service}/worker/{worker}/batch_size\"\n" +
				"        name: BatchSize\n        type: int\n        description: d\n",
			paths: []string{"/svc/worker/reaper/batch_size"},
		},
		{
			name: "telegram worker",
			yaml: "groups:\n  - scope: worker_telegram\n    keys:\n      - path: \"{service}/worker/{worker}/job/timeout\"\n" +
				"        name: JobTimeout\n        type: duration\n        description: d\n",
			paths: []string{"/svc/worker/bot/job/timeout"},
		},
		{
			name: "telegram driver, once per project",
			yaml: "groups:\n  - scope: telegram_driver\n    keys:\n      - path: \"{service}/security/tg_token\"\n" +
				"        name: Token\n        type: string\n        description: d\n",
			paths: []string{"/svc/security/tg_token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths(expand(t, tt.yaml, fullSurface()).Keys)

			if strings.Join(got, " ") != strings.Join(tt.paths, " ") {
				t.Errorf("ExpandGenDecls() paths =\n  %v\nwant\n  %v", got, tt.paths)
			}
		})
	}
}

// A dimension placeholder becomes a parameter of the accessor, in path order,
// and a runtime segment becomes one too. The generated readers are generic over
// both — the REST configuration provider is handed a server and an application
// name by the runtime — so an accessor that baked them in could not be called.
func TestExpandAccessorParameters(t *testing.T) {
	got := expand(t,
		"groups:\n  - scope: rest_server\n    keys:\n"+
			"      - path: \"{service}/transport/rest/{transport}/{app}/handler/<urlPath>/timeout\"\n"+
			"        name: HandlerTimeout\n        type: duration\n        description: d\n",
		fullSurface())

	if len(got.Accessors) != 1 {
		t.Fatalf("want one accessor, got %d", len(got.Accessors))
	}

	accessor := got.Accessors[0]

	if accessor.ParamDecl() != "transport, app, urlPath string" {
		t.Errorf("ParamDecl() = %q", accessor.ParamDecl())
	}

	if want := `serviceTree, "transport", "rest", transport, app, "handler", urlPath, "timeout"`; accessor.PathArgs("serviceTree") != want {
		t.Errorf("PathArgs() = %q, want %q", accessor.PathArgs("serviceTree"), want)
	}

	if accessor.PathBuilderName() != "HandlerTimeoutPath" {
		t.Errorf("PathBuilderName() = %q", accessor.PathBuilderName())
	}

	// A runtime segment is the generated code's own, so the subtree from it on
	// is claimed: /svc/.../handler is not declarable by the service.
	for _, key := range got.Keys {
		if !strings.HasSuffix(key.OwnedPrefix, "/handler") {
			t.Errorf("key %q claims %q, want the subtree to start at /handler", key.Path, key.OwnedPrefix)
		}
	}
}

// `required` does not change what the accessor returns. A reader of a required
// key still has to be able to see an absent one — a transport declared
// `optional: true` in project.yaml is skipped rather than reported — so the
// requirement is a startup check, and the read keeps its presence flag.
func TestExpandRequiredKeepsPresenceShape(t *testing.T) {
	got := expand(t,
		"groups:\n  - scope: grpc_client\n    keys:\n      - path: \"{service}/transport/grpc/{transport}/host\"\n"+
			"        name: Host\n        type: string\n        required: true\n        description: d\n",
		fullSurface())

	if got.Accessors[0].Shape() != ockeymap.ShapeOptional {
		t.Errorf("Shape() = %q, want %q", got.Accessors[0].Shape(), ockeymap.ShapeOptional)
	}

	if len(got.Required) != 1 {
		t.Fatalf("want one startup check, got %d", len(got.Required))
	}

	check := got.Required[0]

	if check.Accessor != "Host" || check.QuotedArgs() != `"users"` {
		t.Errorf("startup check = %+v, want Host(\"users\")", check)
	}

	if check.Path != "/svc/transport/grpc/users/host" {
		t.Errorf("check path = %q", check.Path)
	}
}

// A declaration with no type is a path builder: generated code is handed the
// path and reads below it, so the subtree belongs to it as a whole.
func TestExpandPathBuilder(t *testing.T) {
	got := expand(t,
		"groups:\n  - scope: worker_daemon\n    keys:\n      - path: \"{service}/worker/{worker}\"\n"+
			"        name: WorkerPath\n        description: d\n",
		fullSurface())

	if !got.Accessors[0].IsPathBuilder() {
		t.Error("a declaration without a type should be a path builder")
	}

	if got.Keys[0].OwnedPrefix != "/svc/worker/reaper" {
		t.Errorf("OwnedPrefix = %q, want the path itself", got.Keys[0].OwnedPrefix)
	}
}

// A shared accessor composes its whole path from segments its caller supplies —
// starting with an absolute prefix, so no service tree is prepended to it — and
// therefore claims nothing and contributes no row to the registry: the concrete
// paths it reaches are the ones declared with read_via.
func TestExpandSharedAccessorClaimsNothing(t *testing.T) {
	got := expand(t,
		"groups:\n  - scope: service\n    keys:\n      - path: \"<prefix>/<levelKey>/log_level\"\n"+
			"        name: LogLevel\n        type: string\n        shared: true\n        description: d\n"+
			"      - path: \"{service}/worker/x/default/log_level\"\n        type: string\n"+
			"        read_via: LogLevel\n        description: d\n",
		ockeymap.ProjectSurface{Service: "svc"})

	if len(got.Accessors) != 1 || got.Accessors[0].Name != "LogLevel" {
		t.Fatalf("want only the shared accessor, got %+v", got.Accessors)
	}

	if len(got.Keys) != 1 || got.Keys[0].Path != "/svc/worker/x/default/log_level" {
		t.Fatalf("want the concrete path as the only registry row, got %v", paths(got.Keys))
	}

	key := got.Keys[0]

	if key.ReadVia != "LogLevel" || key.Accessor != "" || key.OwnedPrefix != "" {
		t.Errorf("read_via row = %+v, want it to name the shared accessor and claim nothing", key)
	}
}

// A read_via naming an accessor this project does not have would leave a
// registry row pointing at nothing.
func TestExpandRejectsUnknownReadVia(t *testing.T) {
	_, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{parseDecl(t,
			"groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a/log_level\"\n"+
				"        type: string\n        read_via: Nowhere\n        description: d\n")},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}

	if !strings.Contains(err.Error(), "Nowhere") {
		t.Errorf("the error should name the missing accessor, got: %v", err)
	}
}

// A project without the dimension a declaration names reads none of its keys —
// so it claims none of them. Without this the surface of, say, a REST-only
// project would claim the whole Kafka and telegram subtrees against its own key
// map for no reason.
func TestExpandSkipsAbsentDimensions(t *testing.T) {
	yaml := "groups:\n" +
		"  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        name: DevStand\n" +
		"        type: bool\n        description: d\n" +
		"  - scope: kafka_producer\n    keys:\n      - path: \"{service}/kafka/{client}/brokers\"\n" +
		"        name: Brokers\n        type: string\n        description: d\n" +
		"  - scope: telegram_driver\n    keys:\n      - path: \"{service}/bot/admins/<tgID>\"\n" +
		"        name: AdminACL\n        type: string\n        description: d\n"

	got := expand(t, yaml, ockeymap.ProjectSurface{Service: "svc"})

	if len(got.Keys) != 1 || got.Keys[0].Path != "/svc/devstand" {
		t.Errorf("a project with no Kafka and no telegram worker should read only /svc/devstand, got %v",
			paths(got.Keys))
	}

	// The accessors, by contrast, are all there. An accessor takes its
	// dimensions as arguments, so it is valid whatever the project has — and a
	// template that is generated calls its accessors unconditionally, so gating
	// them on a dimension is a compile error waiting to happen: the REST
	// configuration provider is generated for a project whose only REST
	// transport is a client, and it still calls every server accessor.
	names := make([]string, 0, len(got.Accessors))
	for _, accessor := range got.Accessors {
		names = append(names, accessor.Name)
	}

	for _, want := range []string{"DevStand", "Brokers", "AdminACL"} {
		if !slices.Contains(names, want) {
			t.Errorf("accessor %s should be generated whatever the project has, got %v", want, names)
		}
	}
}

// A REST transport wired into no application still has its transport-level
// paths: the generated provider reads them whether or not an application name
// is available.
func TestExpandRESTServerWithoutApplications(t *testing.T) {
	yaml := "groups:\n  - scope: rest_server\n    keys:\n" +
		"      - path: \"{service}/transport/rest/{transport}/port\"\n        name: Port\n" +
		"        type: string\n        description: d\n" +
		"      - path: \"{service}/transport/rest/{transport}/{app}/port\"\n        name: AppPort\n" +
		"        type: string\n        description: d\n"

	got := paths(expand(t, yaml, ockeymap.ProjectSurface{
		Service:     "svc",
		RESTServers: []ockeymap.RESTServer{{Transport: "api"}},
	}).Keys)

	if len(got) != 1 || got[0] != "/svc/transport/rest/api/port" {
		t.Errorf("want only the transport-level path, got %v", got)
	}
}

// Several templates read the same key — the dev-stand flag is read by the
// logger and by the ogen error responses — so the same path must merge into one
// entry naming both readers, not into two entries.
func TestExpandMergesSharedPaths(t *testing.T) {
	decl := "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        name: DevStand\n" +
		"        type: bool\n        description: console logging.\n"

	first := parseDecl(t, decl)
	second := parseDecl(t,
		"groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        name: DevStand\n"+
			"        type: bool\n        description: error detail in responses.\n")
	second.Owner = "other.go.tmpl"

	got, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	if len(got.Keys) != 1 {
		t.Fatalf("want one merged entry, got %d: %v", len(got.Keys), paths(got.Keys))
	}

	if len(got.Accessors) != 1 {
		t.Fatalf("want one merged accessor, got %d", len(got.Accessors))
	}

	if strings.Join(got.Keys[0].ReadBy, ",") != "other.go.tmpl,test.go.tmpl" {
		t.Errorf("merged entry should name both readers, sorted, got %v", got.Keys[0].ReadBy)
	}

	// Several readers of one key mean several reasons it is read. Keeping only
	// the first would leave the registry naming every reader but explaining one
	// of them, and which one would depend on the sort order of template paths.
	description := got.Keys[0].Description()

	for _, want := range []string{"console logging.", "error detail in responses."} {
		if !strings.Contains(description, want) {
			t.Errorf("merged description = %q, want it to carry %q", description, want)
		}
	}
}

// The same reason declared by two readers is still one reason: a shared key
// whose readers copy the prose must not say it twice.
func TestExpandDeduplicatesIdenticalDescriptions(t *testing.T) {
	decl := "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/devstand\"\n        name: DevStand\n" +
		"        type: bool\n        description: same prose.\n"

	first := parseDecl(t, decl)
	second := parseDecl(t, decl)
	second.Owner = "other.go.tmpl"

	got, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if err != nil {
		t.Fatalf("ExpandGenDecls() error: %v", err)
	}

	if description := got.Keys[0].Description(); description != "same prose." {
		t.Errorf("Description() = %q, want it said once", description)
	}
}

// Two templates disagreeing about what a path is means they do not mean the
// same key, which no merge can paper over.
func TestExpandRejectsContradiction(t *testing.T) {
	first := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n"+
		"        type: bool\n        description: d\n")

	second := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n"+
		"        type: int\n        description: d\n")
	second.Owner = "other.go.tmpl"

	_, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}

	if !strings.Contains(err.Error(), "bool") || !strings.Contains(err.Error(), "int") {
		t.Errorf("the error should name both declarations, got: %v", err)
	}
}

// Two paths asking for one accessor name would generate one function reading
// one of them, and which one would depend on the walk order of the templates.
func TestExpandRejectsAccessorNameCollision(t *testing.T) {
	first := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: Same\n"+
		"        type: bool\n        description: d\n")

	second := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/b\"\n        name: Same\n"+
		"        type: bool\n        description: d\n")
	second.Owner = "other.go.tmpl"

	_, err := ockeymap.ExpandGenDecls(
		[]ockeymap.GenDeclFile{first, second},
		ockeymap.ProjectSurface{Service: "svc"},
	)
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}

	if !strings.Contains(err.Error(), "one name, two paths") {
		t.Errorf("error = %q, want it to say the name is taken", err)
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
			name: "unknown scope",
			yaml: "groups:\n  - scope: nowhere\n    keys:\n      - path: \"{service}/a\"\n        name: A\n" +
				"        type: bool\n        description: d\n",
			wantMsg: `scope "nowhere" is unknown`,
		},
		{
			name: "unknown field",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n" +
				"        type: bool\n        description: d\n        owner: me\n",
			wantMsg: "unknown field",
		},
		{
			name: "placeholder the scope cannot fill",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/worker/{worker}/x\"\n" +
				"        name: X\n        type: bool\n        description: d\n",
			wantMsg: "which this scope cannot fill",
		},
		{
			name: "not rooted in the service tree",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"other/a\"\n        name: A\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "must start with {service}",
		},
		{
			name: "unknown type",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n" +
				"        type: float\n        description: d\n",
			wantMsg: `declares type "float"`,
		},
		{
			name: "no description",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n" +
				"        type: bool\n",
			wantMsg: "has no description",
		},
		{
			name:    "no keys",
			yaml:    "groups:\n  - scope: service\n    keys: []\n",
			wantMsg: "declares no keys",
		},
		{
			name: "no name",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "has no name",
		},
		{
			name: "a path builder whose name does not say so",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n" +
				"        description: d\n",
			wantMsg: `must end in "Path"`,
		},
		{
			name: "a value accessor named like a path builder",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: APath\n" +
				"        type: bool\n        description: d\n",
			wantMsg: `must not end in "Path"`,
		},
		{
			name: "a runtime segment right under the service root",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/<anything>/x\"\n        name: X\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "would claim the whole service tree",
		},
		{
			name: "an invalid runtime segment",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a/<Bad-Name>\"\n        name: X\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "invalid runtime segment",
		},
		{
			name: "required with a runtime segment",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a/<id>\"\n        name: X\n" +
				"        type: bool\n        required: true\n        description: d\n",
			wantMsg: "the startup check verifies paths the generator can enumerate",
		},
		{
			name: "required alongside a default",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: X\n" +
				"        type: bool\n        required: true\n        default: \"false\"\n        description: d\n",
			wantMsg: "declares both `required` and `default`",
		},
		{
			name: "shared without a runtime segment",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: X\n" +
				"        type: bool\n        shared: true\n        description: d\n",
			wantMsg: "has no runtime segment",
		},
		{
			// The caller of a shared accessor hands in an absolute prefix, so a
			// declaration that also names the tree would have the accessor
			// prepend it a second time — the TOOLS-65 defect, stated as grammar.
			name: "shared rooted in the service tree",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/<prefix>/log_level\"\n" +
				"        name: X\n        type: string\n        shared: true\n        description: d\n",
			wantMsg: "no service tree is prepended to it",
		},
		{
			name: "a non-shared key outside the service tree",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"other/a\"\n        name: X\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "must start with {service}",
		},
		{
			name: "read_via together with a name of its own",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: X\n" +
				"        type: bool\n        read_via: Y\n        description: d\n",
			wantMsg: "names both read_via",
		},
		{
			name: "two parameters of one name",
			yaml: "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a/<id>/b/<id>\"\n        name: X\n" +
				"        type: bool\n        description: d\n",
			wantMsg: "twice",
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
// root of the per-user ones, so there are two declarations and the service map
// is kept out of either.
func TestExpandAllowsValueAndSubtreeOnOnePath(t *testing.T) {
	got := expand(t,
		"groups:\n  - scope: service\n    keys:\n      - path: \"{service}/bot/users\"\n        name: UsersACL\n"+
			"        type: string\n        description: blanket.\n"+
			"      - path: \"{service}/bot/users/<tgID>\"\n        name: UserACL\n"+
			"        type: string\n        description: per user.\n",
		ockeymap.ProjectSurface{Service: "svc"})

	if len(got.Keys) != 2 {
		t.Fatalf("want two entries, got %v", paths(got.Keys))
	}

	if got.Keys[0].OwnedPrefix != "" {
		t.Errorf("the value read claims nothing, got %q", got.Keys[0].OwnedPrefix)
	}

	if got.Keys[1].OwnedPrefix != "/svc/bot/users" {
		t.Errorf("the per-user read should claim /svc/bot/users, got %q", got.Keys[1].OwnedPrefix)
	}
}

func TestCheckGeneratorKeys(t *testing.T) {
	generated := []ockeymap.GeneratorKey{
		{
			Path: "/svc/security/csrf/enabled", Type: "bool", Accessor: "CSRF",
			ReadBy: []string{"restconfig.go.tmpl"}, Descriptions: []string{"csrf"},
		},
		{
			Path: "/svc/settings/enabled/<action>", Type: "strings", Accessor: "SettingsEnabled",
			OwnedPrefix: "/svc/settings/enabled",
			ReadBy:      []string{"service.go.tmpl"}, Descriptions: []string{"per-action allow lists"},
		},
		{
			Path: "/svc/bot/users", Type: "string", Accessor: "UsersACL",
			ReadBy: []string{"telegram.go.tmpl"}, Descriptions: []string{"blanket user acl"},
		},
		{
			Path: "/svc/bot/users/<tgID>", Type: "string", Accessor: "UserACL",
			OwnedPrefix: "/svc/bot/users",
			ReadBy:      []string{"telegram.go.tmpl"}, Descriptions: []string{"user acl"},
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
			name:     "inside a subtree opened by a runtime segment",
			yaml:     "keys:\n  - path: /svc/settings/enabled/login\n    type: strings\n",
			wantErr:  true,
			wantMsgs: []string{"is inside \"/svc/settings/enabled\"", "computes", "service.go.tmpl"},
		},
		{
			// The root counts too: otherwise the service declares the node the
			// generated reader owns, and the registry grows two rows for one
			// path, one of them saying the path is not declarable.
			name:     "the root of such a subtree",
			yaml:     "keys:\n  - path: /svc/settings/enabled\n    type: strings\n",
			wantErr:  true,
			wantMsgs: []string{"is the root of a subtree", "service.go.tmpl"},
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
	decl := parseDecl(t, "groups:\n  - scope: service\n    keys:\n      - path: \"{service}/a\"\n        name: A\n"+
		"        type: bool\n        description: d\n")

	_, err := ockeymap.ExpandGenDecls([]ockeymap.GenDeclFile{decl}, ockeymap.ProjectSurface{})
	if !errors.Is(err, ockeymap.ErrGenDeclDefect) {
		t.Fatalf("ExpandGenDecls() error = %v, want ErrGenDeclDefect", err)
	}
}
