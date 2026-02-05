package generator

import (
	"testing"
	"time"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/loggers"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/templater"
)

func TestFilenameToTypeName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "simple json schema",
			path: "user.schema.json",
			want: "UserSchemaJson",
		},
		{
			name: "multiple dots",
			path: "abonent.user.schema.json",
			want: "AbonentUserSchemaJson",
		},
		{
			name: "with path",
			path: "/path/to/schemas/user.schema.json",
			want: "UserSchemaJson",
		},
		{
			name: "with dashes",
			path: "user-profile.schema.json",
			want: "UserProfileSchemaJson",
		},
		{
			name: "with underscores",
			path: "user_profile.schema.json",
			want: "UserProfileSchemaJson",
		},
		{
			name: "mixed separators",
			path: "api-v2_user.schema.json",
			want: "ApiV2UserSchemaJson",
		},
		{
			name: "uppercase input",
			path: "USER.SCHEMA.JSON",
			want: "UserSchemaJson",
		},
		{
			name: "single word",
			path: "user.json",
			want: "UserJson",
		},
		{
			name: "empty string",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filenameToTypeName(tt.path)

			if got != tt.want {
				t.Errorf("filenameToTypeName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerator_GetTmplParams(t *testing.T) {
	logger := loggers.LoggerMapping["zerolog"]

	g := &Generator{
		Logger:              logger,
		ProjectName:         "test-project",
		RegistryType:        "onlineconf",
		Author:              "Test Author",
		ProjectPath:         "github.com/test/project",
		UseActiveRecord:     true,
		DevStand:            true,
		Repo:                "https://github.com/test/project",
		PrivateRepos:        "github.com/private/*",
		DockerImagePrefix:   "ghcr.io/test",
		SkipInitService:     false,
		GoLangVersion:       "1.23",
		OgenVersion:         "v1.0.0",
		ArgenVersion:        "v2.0.0",
		GolangciVersion:     "v1.55.0",
		RuntimeVersion:      "v0.12.0",
		GoJSONSchemaVersion: "v0.15.0",
		GoatVersion:         "v0.8.0",
		GoatServicesVersion: "v0.2.0",
		Applications:        ds.Apps{},
		Drivers:             ds.Drivers{},
		Workers:             ds.Workers{},
		JSONSchemas:         ds.JSONSchemas{},
		Kafka:               ds.KafkaConfigs{},
	}

	params := g.GetTmplParams()

	if params.ProjectName != "test-project" {
		t.Errorf("GetTmplParams().ProjectName = %q, want %q", params.ProjectName, "test-project")
	}

	if params.Author != "Test Author" {
		t.Errorf("GetTmplParams().Author = %q, want %q", params.Author, "Test Author")
	}

	if params.RegistryType != "onlineconf" {
		t.Errorf("GetTmplParams().RegistryType = %q, want %q", params.RegistryType, "onlineconf")
	}

	if params.ProjectPath != "github.com/test/project" {
		t.Errorf("GetTmplParams().ProjectPath = %q, want %q", params.ProjectPath, "github.com/test/project")
	}

	if !params.UseActiveRecord {
		t.Error("GetTmplParams().UseActiveRecord = false, want true")
	}

	if !params.DevStand {
		t.Error("GetTmplParams().DevStand = false, want true")
	}

	if params.GoLangVersion != "1.23" {
		t.Errorf("GetTmplParams().GoLangVersion = %q, want %q", params.GoLangVersion, "1.23")
	}

	if params.RuntimeVersion != "v0.12.0" {
		t.Errorf("GetTmplParams().RuntimeVersion = %q, want %q", params.RuntimeVersion, "v0.12.0")
	}

	// Year should be current year
	currentYear := time.Now().Format("2006")
	if params.Year != currentYear {
		t.Errorf("GetTmplParams().Year = %q, want %q", params.Year, currentYear)
	}

	if params.Logger != logger {
		t.Error("GetTmplParams().Logger is incorrect")
	}
}

func TestGenerator_GetTmplAppParams(t *testing.T) {
	logger := loggers.LoggerMapping["zerolog"]

	g := &Generator{
		Logger:      logger,
		ProjectName: "test-project",
		ProjectPath: "github.com/test/project",
		Deploy: ds.DeployType{
			LogCollector: ds.LogCollectorType{
				Type:    "loki",
				Enabled: true,
			},
		},
	}

	app := ds.App{
		Name:            "web-app",
		Transports:      ds.Transports{},
		Workers:         ds.Workers{},
		Drivers:         ds.Drivers{},
		UseActiveRecord: true,
	}

	params := g.GetTmplAppParams(app)

	if params.Application.Name != "web-app" {
		t.Errorf("GetTmplAppParams().Application.Name = %q, want %q", params.Application.Name, "web-app")
	}

	if params.Deploy.LogCollector.Type != "loki" {
		t.Errorf("GetTmplAppParams().Deploy.LogCollector.Type = %q, want %q", params.Deploy.LogCollector.Type, "loki")
	}

	if !params.Deploy.LogCollector.Enabled {
		t.Error("GetTmplAppParams().Deploy.LogCollector.Enabled = false, want true")
	}

	// Check that GeneratorParams is embedded
	if params.ProjectName != "test-project" {
		t.Errorf("GetTmplAppParams().ProjectName = %q, want %q", params.ProjectName, "test-project")
	}
}

func TestGenerator_GetTmplHandlerParams(t *testing.T) {
	logger := loggers.LoggerMapping["zerolog"]

	g := &Generator{
		Logger:      logger,
		ProjectName: "test-project",
		ProjectPath: "github.com/test/project",
	}

	transport := ds.Transport{
		Name:          "api",
		Type:          ds.RestTransportType,
		GeneratorType: "ogen",
		Port:          "8080",
		GeneratorParams: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	params := g.GetTmplHandlerParams(transport)

	if params.Transport.Name != "api" {
		t.Errorf("GetTmplHandlerParams().Transport.Name = %q, want %q", params.Transport.Name, "api")
	}

	if params.Transport.Type != ds.RestTransportType {
		t.Errorf("GetTmplHandlerParams().Transport.Type = %q, want %q", params.Transport.Type, ds.RestTransportType)
	}

	if params.Transport.Port != "8080" {
		t.Errorf("GetTmplHandlerParams().Transport.Port = %q, want %q", params.Transport.Port, "8080")
	}

	if len(params.TransportParams) != 2 {
		t.Errorf("GetTmplHandlerParams().TransportParams len = %d, want 2", len(params.TransportParams))
	}

	if params.TransportParams["key1"] != "value1" {
		t.Errorf("GetTmplHandlerParams().TransportParams[key1] = %q, want %q", params.TransportParams["key1"], "value1")
	}

	// Check that GeneratorParams is embedded
	if params.ProjectName != "test-project" {
		t.Errorf("GetTmplHandlerParams().ProjectName = %q, want %q", params.ProjectName, "test-project")
	}
}

func TestGenerator_GetTmplRunnerParams(t *testing.T) {
	logger := loggers.LoggerMapping["zerolog"]

	g := &Generator{
		Logger:      logger,
		ProjectName: "test-project",
		ProjectPath: "github.com/test/project",
	}

	worker := ds.Worker{
		Name:              "telegram",
		GeneratorType:     "telegram",
		GeneratorTemplate: "default",
		GeneratorParams: map[string]string{
			"token": "bot_token",
		},
	}

	params := g.GetTmplRunnerParams(worker)

	if params.Worker.Name != "telegram" {
		t.Errorf("GetTmplRunnerParams().Worker.Name = %q, want %q", params.Worker.Name, "telegram")
	}

	if params.Worker.GeneratorType != "telegram" {
		t.Errorf("GetTmplRunnerParams().Worker.GeneratorType = %q, want %q", params.Worker.GeneratorType, "telegram")
	}

	if params.WorkerParams["token"] != "bot_token" {
		t.Errorf("GetTmplRunnerParams().WorkerParams[token] = %q, want %q", params.WorkerParams["token"], "bot_token")
	}

	// Check that GeneratorParams is embedded
	if params.ProjectName != "test-project" {
		t.Errorf("GetTmplRunnerParams().ProjectName = %q, want %q", params.ProjectName, "test-project")
	}
}

func TestExecCmd(t *testing.T) {
	cmd := ExecCmd{
		Cmd: "make",
		Arg: []string{"build"},
		Msg: "building project",
	}

	if cmd.Cmd != "make" {
		t.Errorf("ExecCmd.Cmd = %q, want %q", cmd.Cmd, "make")
	}

	if len(cmd.Arg) != 1 || cmd.Arg[0] != "build" {
		t.Errorf("ExecCmd.Arg = %v, want [build]", cmd.Arg)
	}

	if cmd.Msg != "building project" {
		t.Errorf("ExecCmd.Msg = %q, want %q", cmd.Msg, "building project")
	}
}

func TestGeneratorStruct_Fields(t *testing.T) {
	g := Generator{
		AppInfo:             "test-app-info",
		DryRun:              true,
		Meta:                meta.Meta{Version: 3},
		ProjectName:         "my-project",
		RegistryType:        "onlineconf",
		Author:              "Author Name",
		ProjectPath:         "github.com/my/project",
		UseActiveRecord:     true,
		DevStand:            true,
		Repo:                "https://github.com/my/project",
		PrivateRepos:        "github.com/private/*",
		GoLangVersion:       "1.23",
		OgenVersion:         "v1.0.0",
		ArgenVersion:        "v2.0.0",
		GolangciVersion:     "v1.55.0",
		RuntimeVersion:      "v0.12.0",
		GoJSONSchemaVersion: "v0.15.0",
		GoatVersion:         "v0.8.0",
		GoatServicesVersion: "v0.2.0",
		TargetDir:           "/target",
		ConfigPath:          "/config/project.yaml",
		DockerImagePrefix:   "ghcr.io/my",
		SkipInitService:     true,
	}

	if g.AppInfo != "test-app-info" {
		t.Errorf("Generator.AppInfo = %q, want %q", g.AppInfo, "test-app-info")
	}

	if !g.DryRun {
		t.Error("Generator.DryRun = false, want true")
	}

	if g.Meta.Version != 3 {
		t.Errorf("Generator.Meta.Version = %d, want 3", g.Meta.Version)
	}

	if g.ProjectName != "my-project" {
		t.Errorf("Generator.ProjectName = %q, want %q", g.ProjectName, "my-project")
	}

	if !g.UseActiveRecord {
		t.Error("Generator.UseActiveRecord = false, want true")
	}

	if !g.DevStand {
		t.Error("Generator.DevStand = false, want true")
	}

	if !g.SkipInitService {
		t.Error("Generator.SkipInitService = false, want true")
	}
}

func TestMinRuntimeVersion(t *testing.T) {
	if templater.MinRuntimeVersion == "" {
		t.Error("MinRuntimeVersion is empty")
	}

	// Should start with 'v'
	if templater.MinRuntimeVersion[0] != 'v' {
		t.Errorf("MinRuntimeVersion = %q, should start with 'v'", templater.MinRuntimeVersion)
	}
}

func TestGeneratorParamsTypes(t *testing.T) {
	t.Run("GeneratorParams fields", func(t *testing.T) {
		params := templater.GeneratorParams{
			AppInfo:           "info",
			ProjectName:       "project",
			RegistryType:      "onlineconf",
			Author:            "author",
			Year:              "2024",
			ProjectPath:       "path",
			UseActiveRecord:   true,
			DevStand:          true,
			Repo:              "repo",
			PrivateRepos:      "private",
			DockerImagePrefix: "prefix",
			SkipServiceInit:   true,
			GoLangVersion:     "1.23",
		}

		if params.AppInfo != "info" {
			t.Errorf("GeneratorParams.AppInfo = %q, want %q", params.AppInfo, "info")
		}

		if params.Year != "2024" {
			t.Errorf("GeneratorParams.Year = %q, want %q", params.Year, "2024")
		}
	})

	t.Run("GeneratorAppParams embeds GeneratorParams", func(t *testing.T) {
		params := templater.GeneratorAppParams{
			GeneratorParams: templater.GeneratorParams{
				ProjectName: "project",
			},
			Application: ds.App{Name: "app"},
		}

		if params.ProjectName != "project" {
			t.Errorf("GeneratorAppParams.ProjectName = %q, want %q", params.ProjectName, "project")
		}

		if params.Application.Name != "app" {
			t.Errorf("GeneratorAppParams.Application.Name = %q, want %q", params.Application.Name, "app")
		}
	})

	t.Run("GeneratorHandlerParams embeds GeneratorParams", func(t *testing.T) {
		params := templater.GeneratorHandlerParams{
			GeneratorParams: templater.GeneratorParams{
				ProjectName: "project",
			},
			Transport:       ds.Transport{Name: "api"},
			TransportParams: map[string]string{"key": "value"},
		}

		if params.ProjectName != "project" {
			t.Errorf("GeneratorHandlerParams.ProjectName = %q, want %q", params.ProjectName, "project")
		}

		if params.Transport.Name != "api" {
			t.Errorf("GeneratorHandlerParams.Transport.Name = %q, want %q", params.Transport.Name, "api")
		}

		if params.TransportParams["key"] != "value" {
			t.Errorf("GeneratorHandlerParams.TransportParams[key] = %q, want %q", params.TransportParams["key"], "value")
		}
	})

	t.Run("GeneratorRunnerParams embeds GeneratorParams", func(t *testing.T) {
		params := templater.GeneratorRunnerParams{
			GeneratorParams: templater.GeneratorParams{
				ProjectName: "project",
			},
			Worker:       ds.Worker{Name: "worker"},
			WorkerParams: map[string]string{"key": "value"},
		}

		if params.ProjectName != "project" {
			t.Errorf("GeneratorRunnerParams.ProjectName = %q, want %q", params.ProjectName, "project")
		}

		if params.Worker.Name != "worker" {
			t.Errorf("GeneratorRunnerParams.Worker.Name = %q, want %q", params.Worker.Name, "worker")
		}

		if params.WorkerParams["key"] != "value" {
			t.Errorf("GeneratorRunnerParams.WorkerParams[key] = %q, want %q", params.WorkerParams["key"], "value")
		}
	})
}

func TestGeneratorKafkaParams(t *testing.T) {
	params := templater.GeneratorKafkaParams{
		GeneratorParams: templater.GeneratorParams{
			ProjectName: "project",
		},
		Kafka: ds.KafkaConfig{
			Name:   "events",
			Type:   "consumer",
			Driver: "segmentio",
		},
	}

	if params.ProjectName != "project" {
		t.Errorf("GeneratorKafkaParams.ProjectName = %q, want %q", params.ProjectName, "project")
	}

	if params.Kafka.Name != "events" {
		t.Errorf("GeneratorKafkaParams.Kafka.Name = %q, want %q", params.Kafka.Name, "events")
	}

	if params.Kafka.Type != "consumer" {
		t.Errorf("GeneratorKafkaParams.Kafka.Type = %q, want %q", params.Kafka.Type, "consumer")
	}

	if params.Kafka.Driver != "segmentio" {
		t.Errorf("GeneratorKafkaParams.Kafka.Driver = %q, want %q", params.Kafka.Driver, "segmentio")
	}
}
