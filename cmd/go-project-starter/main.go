package main

import (
	"fmt"
	"io"
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
	"github.com/Educentr/go-project-starter/internal/pkg/selfversion"
	"github.com/Educentr/go-project-starter/internal/pkg/setup"
	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
	"github.com/Educentr/go-project-starter/internal/pkg/treecheck"
)

const (
	msgConfig         = "used config path ="
	msgSourceRef      = "spec sources without an explicit @ref are taken from:"
	cmdSetup          = "setup"
	cmdInit           = "init"
	cmdMigrate        = "migrate"
	cmdVersion        = "version"
	argVersion        = "--version"
	argEndOfFlags     = "--"
	defaultConfigDir  = ".project-config"
	defaultConfigFile = "project.yaml"
	flagConfig        = "config"
	flagConfigDir     = "configDir"
	flagDryRun        = "dry-run"
	flagCheck         = "check"
	flagSourceRef     = "sourceRef"
	envSourceRef      = "GPS_SOURCE_REF"
	usageConfigFile   = "project configuration file"
	usageCheck        = "verify the generated area of the project tree against the manifest in " +
		".project-config/meta.yaml and exit; writes nothing and reads neither the config nor any spec source"
	usageSourceRef = "git ref for the spec sources in project.yaml that do not pin one explicitly: " +
		"either <ref> for every repository, or <repo>:<ref> for one of them. " +
		"Repeatable, and one value may carry several entries separated by commas or spaces " +
		"(env " + envSourceRef + ", used only when the flag is absent)"

	layoutFailedToBindFlags       = "failed to bind flags: %v"
	layoutFailedToLoadConfig      = "failed to load config: %v"
	layoutFailedToLoadMeta        = "failed to load meta: %v"
	layoutFailedToCreateGenerator = "failed to create generator: %v"
	layoutFailedToGenerate        = "failed to generate: %v"
	layoutFailedToSetup           = "failed to run setup: %v"
	layoutFailedToInit            = "failed to run init: %v"
	layoutFailedToMigrate         = "failed to migrate config: %v"
	layoutFailedToCheck           = "failed to check the project tree: %v"
	layoutInvalidSourceRef        = "invalid --%s: %v"
	layoutVersion                 = "go-project-starter %s\ncommit: %s\nbuilt: %s\n"
	layoutCheckIgnoredFlag        = "warning: --%s is ignored by --%s: the check reads the manifest and the tree, nothing else"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Answered before the subcommand dispatch: --version is documented as a
	// flag common to every command, and each subcommand hands pflag only its
	// own arguments, so no single flag set can see it.
	if versionRequested(os.Args[1:]) {
		printVersion(os.Stdout)

		return
	}

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

// versionRequested reports whether args — the process arguments without the
// program name — ask for the version of the generator. Both documented forms
// count: the `version` subcommand, and the --version flag in any position,
// including after a subcommand of its own. Past the end-of-flags terminator
// everything is an operand, so a literal `--version` there is not a request.
func versionRequested(args []string) bool {
	if len(args) > 0 && args[0] == cmdVersion {
		return true
	}

	for _, arg := range args {
		switch arg {
		case argEndOfFlags:
			return false
		case argVersion:
			return true
		}
	}

	return false
}

// printVersion writes the build identity of the binary: the version it was
// built from, the commit and the build time, all three stamped by the linker
// (LD_FLAGS in the Makefile). Both request forms print exactly this, so a
// script may parse the output of either one.
func printVersion(w io.Writer) {
	fmt.Fprintf(w, layoutVersion, version, commit, buildDate)
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
		sourceRefs    *specsource.SourceRefs
		err           error
		dryRun        bool
		checkMode     bool
	)

	pflag.StringVar(&baseConfigDir, flagConfigDir, defaultConfigDir, "project configuration directory")
	pflag.StringVar(&cfgPath, flagConfig, defaultConfigFile, usageConfigFile)
	pflag.StringVar(&targetDir, "target", "", "target directory")
	pflag.BoolVar(&dryRun, flagDryRun, false, "Dry run")
	pflag.BoolVar(&checkMode, flagCheck, false, usageCheck)
	// Repeatable: contracts come from several repositories, each of which may
	// need its own ref in one build (see resolveSourceRefs).
	pflag.StringArray(flagSourceRef, nil, usageSourceRef)

	pflag.Parse()

	// Before anything else is read: the check is defined as a verdict on the
	// tree and the manifest alone, so it must not depend on a loadable config
	// or a reachable contract repository (see internal/pkg/treecheck).
	if checkMode {
		runCheck(targetDir, pflag.CommandLine)

		return
	}

	if err = viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf(layoutFailedToBindFlags, err)
	}

	if sourceRefs, err = resolveSourceRefs(pflag.CommandLine, os.Getenv); err != nil {
		log.Fatalf(layoutInvalidSourceRef, flagSourceRef, err)
	}

	log.Println(msgConfig, cfgPath)

	if !sourceRefs.IsEmpty() {
		log.Println(msgSourceRef, sourceRefs)
	}

	cfgDir := baseConfigDir
	if !filepath.IsAbs(baseConfigDir) {
		cfgDir = filepath.Join(targetDir, baseConfigDir)
	}

	if cfg, err = config.GetConfig(cfgDir, cfgPath); err != nil {
		log.Fatalf(layoutFailedToLoadConfig, err)
	}

	// Meta is always stored in target directory's .project-config
	metaDir := filepath.Join(targetDir, meta.DirName)
	if genMeta, err = meta.GetMeta(metaDir, meta.FileName); err != nil {
		log.Fatalf(layoutFailedToLoadMeta, err)
	}

	if targetDir != "" {
		cfg.SetTargetDir(targetDir)
	}

	// The version of this binary is pinned into the generated project, so the
	// tree records which generator produced it (see selfversion).
	if gen, err = generator.New(selfversion.Resolve(version), cfg, genMeta, dryRun, sourceRefs); err != nil {
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

// runCheck compares the generated area of the tree against the manifest the
// last run wrote and exits non-zero when it no longer matches — the form a CI
// gate needs. It writes nothing, so it is safe to run on a tree someone else
// is about to commit.
//
// Flags that only make sense for generation are reported as ignored instead of
// being honored or silently dropped: --sourceRef in particular would suggest
// the check consults the contract repositories, which is exactly what it does
// not do.
func runCheck(targetDir string, flags *pflag.FlagSet) {
	for _, name := range []string{flagConfig, flagConfigDir, flagSourceRef} {
		if flags.Changed(name) {
			log.Printf(layoutCheckIgnoredFlag, name, flagCheck)
		}
	}

	res, err := treecheck.Run(targetDir)
	if err != nil {
		log.Fatalf(layoutFailedToCheck, err)
	}

	fmt.Println(res.Report())

	if !res.OK() {
		os.Exit(1)
	}
}

// resolveSourceRefs returns the git ref overrides for the `path:` sources of
// project.yaml: the --sourceRef flags when any were passed, otherwise the
// GPS_SOURCE_REF env var, otherwise an empty set — no override, every source
// keeps the ref written in the config.
//
// Precedence is all-or-nothing on purpose: if the command line names any ref,
// the env var is ignored entirely rather than merged into it. A per-repository
// set is only meaningful as a whole, and half of it coming from an exported
// variable nobody remembers is how a tree ends up built from two states of the
// contracts.
//
// Every ref is validated here, at the boundary where it enters the process, so
// a typo fails before any config is read or any repository is cloned.
func resolveSourceRefs(fs *pflag.FlagSet, getenv func(string) string) (*specsource.SourceRefs, error) {
	var entries []string

	if fs.Changed(flagSourceRef) {
		values, err := fs.GetStringArray(flagSourceRef)
		if err != nil {
			return nil, fmt.Errorf("read --%s: %w", flagSourceRef, err)
		}

		entries = values
	} else if env := getenv(envSourceRef); env != "" {
		entries = []string{env}
	}

	return specsource.ParseSourceRefs(entries)
}
