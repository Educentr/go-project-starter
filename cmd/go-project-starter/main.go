package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/generator"
	projinit "github.com/Educentr/go-project-starter/internal/pkg/init"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/Educentr/go-project-starter/internal/pkg/migrate"
	"github.com/Educentr/go-project-starter/internal/pkg/setup"
)

const (
	msgConfig         = "used config path ="
	cmdSetup          = "setup"
	cmdInit           = "init"
	cmdMigrate        = "migrate"
	defaultConfigDir  = ".project-config"
	defaultConfigFile = "project.yaml"
	flagConfig        = "config"
	flagDryRun        = "dry-run"
	usageConfigFile   = "project configuration file"

	layoutFailedToBindFlags       = "failed to bind flags: %v"
	layoutFailedToLoadConfig      = "failed to load config: %v"
	layoutFailedToLoadMeta        = "failed to load meta: %v"
	layoutFailedToCreateGenerator = "failed to create generator: %v"
	layoutFailedToGenerate        = "failed to generate: %v"
	layoutFailedToSetup           = "failed to run setup: %v"
	layoutFailedToInit            = "failed to run init: %v"
	layoutFailedToMigrate         = "failed to migrate config: %v"
)

var AppInfo string = "go-sterter-v0.01"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case cmdSetup:
			runSetup()

			return
		case cmdInit:
			runInit()

			return
		case cmdMigrate:
			runMigrate()

			return
		}
	}

	runGenerator()
}

func runSetup() {
	// Setup command flags
	setupFlags := pflag.NewFlagSet(cmdSetup, pflag.ExitOnError)

	var (
		configDir string
		targetDir string
		dryRun    bool
	)

	setupFlags.StringVar(&configDir, "configDir", defaultConfigDir, "project configuration directory")
	setupFlags.StringVar(&targetDir, "target", ".", "target directory")
	setupFlags.BoolVar(&dryRun, flagDryRun, false, "Dry run mode")

	// Parse flags after "setup" command
	if err := setupFlags.Parse(os.Args[2:]); err != nil {
		log.Fatalf("failed to parse setup flags: %v", err)
	}

	// Determine subcommand (ci, server, deploy, or none for full wizard)
	args := setupFlags.Args()

	var cmd setup.Command

	if len(args) > 0 {
		switch args[0] {
		case "ci":
			cmd = setup.CommandCI
		case "server":
			cmd = setup.CommandServer
		case "deploy":
			cmd = setup.CommandDeploy
		default:
			log.Fatalf("unknown setup subcommand: %s\nUsage: go-project-starter setup [ci|server|deploy] [flags]", args[0])
		}
	}

	// Create setup instance
	s, err := setup.New(setup.Options{
		ConfigDir: configDir,
		TargetDir: targetDir,
		DryRun:    dryRun,
	})
	if err != nil {
		log.Fatalf(layoutFailedToSetup, err)
	}

	// Run setup
	if err := s.Run(cmd); err != nil {
		log.Fatalf(layoutFailedToSetup, err)
	}
}

func runInit() {
	// Init command flags
	initFlags := pflag.NewFlagSet(cmdInit, pflag.ExitOnError)

	var (
		configDir string
		targetDir string
	)

	initFlags.StringVar(&configDir, "configDir", defaultConfigDir, "project configuration directory")
	initFlags.StringVar(&targetDir, "target", ".", "target directory")

	// Parse flags after "init" command
	if err := initFlags.Parse(os.Args[2:]); err != nil {
		log.Fatalf("failed to parse init flags: %v", err)
	}

	// Create init instance
	i := projinit.New(projinit.Options{
		ConfigDir: configDir,
		TargetDir: targetDir,
	})

	// Run init
	if err := i.Run(); err != nil {
		log.Fatalf(layoutFailedToInit, err)
	}
}

func runMigrate() {
	// Migrate command flags
	migrateFlags := pflag.NewFlagSet(cmdMigrate, pflag.ExitOnError)

	var (
		configDir  string
		configFile string
		dryRun     bool
	)

	migrateFlags.StringVar(&configDir, "configDir", defaultConfigDir, "project configuration directory")
	migrateFlags.StringVar(&configFile, flagConfig, defaultConfigFile, usageConfigFile)
	migrateFlags.BoolVar(&dryRun, flagDryRun, false, "Dry run mode - show what would be changed")

	// Parse flags after "migrate" command
	if err := migrateFlags.Parse(os.Args[2:]); err != nil {
		log.Fatalf("failed to parse migrate flags: %v", err)
	}

	// Find config file
	configPath := migrate.FindConfigFile(configDir, configFile)

	fmt.Printf("Migrating config: %s\n", configPath)

	if dryRun {
		fmt.Println("(dry-run mode)")
	}

	// Create migrator and run
	m := migrate.New(configPath, dryRun)

	result, err := m.Migrate()
	if err != nil {
		log.Fatalf(layoutFailedToMigrate, err)
	}

	if !result.Modified {
		fmt.Println("✓ Config is already up to date, no migration needed")

		return
	}

	if dryRun {
		fmt.Println("\n⚠️  Deprecations found:")
		migrate.PrintWarnings(result.Warnings)
		fmt.Println("Run without --dry-run to apply migrations")

		return
	}

	fmt.Printf("✓ Config migrated successfully\n")
	fmt.Printf("  Backup saved to: %s\n", result.BackupPath)

	if len(result.Warnings) > 0 {
		fmt.Printf("  %d deprecation(s) fixed\n", len(result.Warnings))
	}
}

func runGenerator() {
	var (
		gen           *generator.Generator
		cfg           config.Config
		genMeta       meta.Meta
		cfgPath       string
		targetDir     string
		baseConfigDir string
		err           error
		dryRun        bool
	)

	pflag.StringVar(&baseConfigDir, "configDir", defaultConfigDir, "project configuration directory")
	pflag.StringVar(&cfgPath, flagConfig, defaultConfigFile, usageConfigFile)
	pflag.StringVar(&targetDir, "target", "", "target directory")
	pflag.BoolVar(&dryRun, flagDryRun, false, "Dry run")

	pflag.Parse()

	if err = viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf(layoutFailedToBindFlags, err)
	}

	log.Println(msgConfig, cfgPath)

	cfgDir := baseConfigDir
	if !filepath.IsAbs(baseConfigDir) {
		cfgDir = filepath.Join(targetDir, baseConfigDir)
	}

	if cfg, err = config.GetConfig(cfgDir, cfgPath); err != nil {
		log.Fatalf(layoutFailedToLoadConfig, err)
	}

	// Meta is always stored in target directory's .project-config
	metaDir := filepath.Join(targetDir, ".project-config")
	if genMeta, err = meta.GetMeta(metaDir, "meta.yaml"); err != nil {
		log.Fatalf(layoutFailedToLoadMeta, err)
	}

	if targetDir != "" {
		cfg.SetTargetDir(targetDir)
	}

	if gen, err = generator.New(AppInfo, cfg, genMeta, dryRun); err != nil {
		log.Fatalf(layoutFailedToCreateGenerator, err)
	}

	// ToDo debug log
	// Прикрутить логгер, сделать уровни логирования и добавить эту секцию как Debug
	// log.Printf("Generator: %+v", gen)

	if err = gen.Generate(); err != nil {
		log.Fatalf(layoutFailedToGenerate, err)
	}

	log.Println("done")
}
