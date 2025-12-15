package main

import (
	"log"
	"path/filepath"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Educentr/go-project-starter/internal/pkg/config"
	"github.com/Educentr/go-project-starter/internal/pkg/generator"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
)

const (
	keyConfig       = "config"
	defaultConfig   = "./config.yaml"
	msgConfig       = "used config path ="
	usageFlagConfig = "configuration file with information regarding the generation new project"

	layoutFailedToBindFlags       = "failed to bind flags: %v"
	layoutFailedToLoadConfig      = "failed to load config: %v"
	layoutFailedToLoadMeta        = "failed to load meta: %v"
	layoutFailedToCreateGenerator = "failed to create generator: %v"
	layoutFailedToGenerate        = "failed to generate: %v"
)

var AppInfo string = "go-sterter-v0.01"

func main() {
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

	pflag.StringVar(&baseConfigDir, "configDir", ".project-config", "project configuration directory")
	pflag.StringVar(&cfgPath, "config", "project.yaml", "project configuration file")
	pflag.StringVar(&targetDir, "target", "", "target directory")
	pflag.BoolVar(&dryRun, "dry-run", false, "Dry run")

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
