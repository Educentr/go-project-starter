package generator

type (
	BufType  string
	TypeDB   string
	DriverDB string

	consumer struct {
		Name    string
		Path    string
		Backend BufType
		Group   string
		Topic   string
	}

	repository struct {
		Name string
		// Alias    string
		TypeDB   TypeDB
		DriverDB DriverDB
	}

	Contract struct {
		Path       string
		ServerName string
		Short      string
		APIPrefix  string
	}

	Ports struct {
		Grpc string
		Rest string
		Sys  string
	}

	PkgDataRepo struct {
		Name             string
		TypeDB           string
		DriverDB         string
		MigrationFileExt string
		Alias            string
	}

	PkgDataSchedulerWorker struct {
		PkgName string
	}

	PkgDataConsumer struct {
		Name        string
		PackageName string
		Topic       string
		Group       string
	}

	PkgDataGrpc struct {
		Name        string
		PackageName string
	}

	PkgDataRest struct {
		Name        string
		PackageName string
		APIPrefix   string
	}

	PkgData struct {
		ProjectName     string
		GoLangVersion   string
		OgenVersion     string
		ProtobufVersion string
		GolangciVersion string
		AppInfo         string

		GitlabModulePath string
		GitlabProjectID  uint

		Ports Ports

		Kafka    bool
		Repo     bool
		PG       bool
		Redis    bool
		Repos    []*PkgDataRepo
		Daemons  []*PkgDataSchedulerWorker
		GrpcData []*PkgDataGrpc
		RestData []*PkgDataRest
		GRPC     bool
		REST     bool
		Clean    bool

		Scheduler bool
		// Scheduler *PkgDataSchedulerWorker
		RepoData *PkgDataRepo
		Grpc     *PkgDataGrpc
		Consumer *PkgDataConsumer
	}

	process struct {
		pr   string
		argv []string
		msg  string
	}
)

const (
	TemplateName string = "ScalableSolutionsGeneratorTemplate"

	Kafka BufType = "kafka"

	Psql  TypeDB = "psql"
	Mongo TypeDB = "mongodb"
	Redis TypeDB = "redis"

	SQLC        DriverDB = "sqlc"
	DriverRedis DriverDB = "redis"
)
