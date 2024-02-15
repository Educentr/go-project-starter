package main

import (
	"fmt"
	"log"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"gitlab.educentr.info/golang/service-starter/pkg/generator"
)

const (
	keyConfig       = "config"
	defaultConfig   = "./config.yaml"
	msgConfig       = "config path ="
	usageFlagConfig = "configuration file with information regarding the generation new project"

	layoutFailedToBindFlags       = "failed to bind flags: %v"
	layoutFailedToLoadConfig      = "failed to load config: %v"
	layoutFailedToCreateGenerator = "failed to create generator: %v"
	layoutFailedToGenerate        = "failed to generate: %v"
)

func main() {
	var (
		gen       *generator.Generator
		cfg       generator.Config
		cfgPath   string
		targetDir string
		err       error
	)

	pflag.StringVar(&cfgPath, "config", "./project-config.yaml", "project configuration file")
	pflag.StringVar(&targetDir, "target", "./", "target directory")
	pflag.Parse()

	if err = viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf(layoutFailedToBindFlags, err)
	}

	log.Println(msgConfig, cfgPath)

	if cfg, err = generator.GetConfig(cfgPath); err != nil {
		log.Fatalf(layoutFailedToLoadConfig, err)
	}

	if targetDir != "" {
		cfg.SetTargetDir(targetDir)
	}

	if gen, err = generator.New(cfg); err != nil {
		log.Fatalf(layoutFailedToCreateGenerator, err)
	}
	fmt.Printf("%+v", gen)

	// if err = gen.Generate(); err != nil {
	// 	log.Fatalf(layoutFailedToGenerate, err)
	// }

	log.Println("done")
}
