package generator

// import (
// 	"bytes"
// 	"io"
// 	"io/fs"
// 	"log"
// 	"os"
// 	"path"
// 	"regexp"
// 	"strings"
// )

// type (
// 	generationContext struct {
// 		targetPath   string
// 		params       PkgData
// 		pathReplacer *strings.Replacer
// 		snapshot     Snapshot
// 	}
// )

// const (
// 	rootDirectory = "."
// 	fileMode      = 0755
// )

// var (
// 	skipListPrefix = []string{}

// 	skipMatchList = []string{
// 		rootDirectory,
// 		"templates",
// 	}

// 	directCopyListPrefix = []string{
// 		"pkg/ansiblegenerator", // because internal golang templates in code
// 	}

// 	dbTypeRegExp = regexp.MustCompile(`model\/repository\/(\w+)\/service_name`)
// )

// func isDirectCopy(p string) bool {
// 	for _, e := range directCopyListPrefix {
// 		if strings.HasPrefix(p, e) {
// 			return true
// 		}
// 	}

// 	return false
// }

// func inSkipList(p string) bool {
// 	for _, e := range skipListPrefix {
// 		if strings.HasPrefix(p, e) {
// 			return true
// 		}
// 	}

// 	for _, e := range skipMatchList {
// 		if p == e {
// 			return true
// 		}
// 	}

// 	return false
// }

// func (g *Generator) pathNameReplacer() *strings.Replacer {
// 	return strings.NewReplacer(ServiceName, g.config.Main.Name)
// }

// func (g *Generator) prepareSkipListPrefix() {
// 	if !g.grpc {
// 		skipListPrefix = append(skipListPrefix,
// 			"internal/transport/grpc",
// 			"configs/grpc.gen.yaml",
// 		)
// 	}

// 	if !g.rest {
// 		skipListPrefix = append(skipListPrefix, "internal/transport/rest")
// 	}

// 	if !g.config.Scheduler.Enabled {
// 		skipListPrefix = append(skipListPrefix, "cmd/scheduler")
// 	}

// 	foundRepos := make(map[TypeDB]struct{})

// 	for _, repo := range g.repositories {
// 		foundRepos[repo.TypeDB] = struct{}{}
// 	}

// 	if _, ok := foundRepos[Psql]; !ok {
// 		skipListPrefix = append(skipListPrefix,
// 			"internal/model/repository/psql",
// 			"configs/sqlc.yaml",
// 		)
// 	}

// 	if _, ok := foundRepos[Redis]; !ok {
// 		skipListPrefix = append(skipListPrefix, "internal/model/repository/redis")
// 	}

// 	if _, ok := foundRepos[Mongo]; !ok {
// 		skipListPrefix = append(skipListPrefix, "internal/model/repository/mongodb")
// 	}

// 	if len(foundRepos) == 0 {
// 		skipListPrefix = append(skipListPrefix, "cmd/migrate", "internal/model/repository")
// 	}

// 	if !g.Kafka {
// 		skipListPrefix = append(skipListPrefix,
// 			"internal/transport/kafka",
// 			"cmd/consumer",
// 			"configs/kafka.gen.yaml",
// 		)
// 	}

// 	if !g.ws {
// 		skipListPrefix = append(skipListPrefix, "internal/transport/ws")
// 	}
// }

// func (c *generationContext) moduleTemplateApply(
// 	replacer *strings.Replacer,
// 	params any,
// 	walkPath string,
// 	relativePath string,
// 	isDirectory bool,
// ) error {

// 	if isDirectory {
// 		if err := os.MkdirAll(replacer.Replace(path.Join(c.targetPath, relativePath)), fileMode); err != nil {
// 			return err
// 		}
// 	} else {
// 		relativePath = strings.TrimSuffix(relativePath, ".tmpl")
// 		content, err := fs.ReadFile(templates, walkPath)
// 		if err != nil {
// 			return err
// 		}

// 		err = c.generateFileContent(replacer.Replace(path.Join(c.targetPath, relativePath)), relativePath, content, params)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func (c *generationContext) repositoryProcessing(walkPath string, relativePath string, isDirectory bool) (bool, error) {
// 	if !strings.Contains(walkPath, "internal/model/repository") {
// 		return false, nil
// 	}

// 	match := dbTypeRegExp.FindStringSubmatch(walkPath)
// 	if len(match) != 2 {
// 		return true, nil
// 	}

// 	for _, e := range c.params.Repos {
// 		if e.TypeDB != match[1] {
// 			continue
// 		}
// 		switch TypeDB(match[1]) {
// 		case Psql:
// 			replacer := strings.NewReplacer(ServiceName, e.Name)
// 			params := map[string]interface{}{
// 				"ProjectName":      e.Name,
// 				"GitlabModulePath": c.params.GitlabModulePath,
// 			}

// 			if err := c.moduleTemplateApply(replacer, &params, walkPath, relativePath, isDirectory); err != nil {
// 				return false, err
// 			}

// 		case Redis:
// 			replacer := strings.NewReplacer(ServiceName, e.Name)
// 			params := map[string]interface{}{
// 				"ProjectName":      e.Name,
// 				"GitlabModulePath": c.params.GitlabModulePath,
// 			}

// 			if err := c.moduleTemplateApply(replacer, &params, walkPath, relativePath, isDirectory); err != nil {
// 				return false, err
// 			}
// 		case Mongo:
// 		}
// 	}

// 	return true, nil
// }

// func (c *generationContext) restModuleProcessing(walkPath string, relativePath string, isDirectory bool) (bool, error) {
// 	if !strings.Contains(walkPath, "internal/transport/rest/service_name/v1") {
// 		return false, nil
// 	}

// 	for _, e := range c.params.RestData {
// 		replacer := strings.NewReplacer(ServiceName, e.Name)
// 		params := map[string]interface{}{
// 			"RestPackageName":  e.Name,
// 			"GitlabModulePath": c.params.GitlabModulePath,
// 		}

// 		if err := c.moduleTemplateApply(replacer, params, walkPath, relativePath, isDirectory); err != nil {
// 			return false, err
// 		}
// 	}

// 	return true, nil
// }

// func (c *generationContext) gRPCModuleProcessing(walkPath string, relativePath string, isDirectory bool) (bool, error) {
// 	if !strings.Contains(walkPath, "internal/transport/grpc/service_name/v1") {
// 		return false, nil
// 	}

// 	for _, e := range c.params.GrpcData {
// 		replacer := strings.NewReplacer(ServiceName, e.PackageName)
// 		params := map[string]interface{}{
// 			"GrpcServerName":   e.Name,
// 			"GrpcPackageName":  e.PackageName,
// 			"GitlabModulePath": c.params.GitlabModulePath,
// 		}

// 		if err := c.moduleTemplateApply(replacer, params, walkPath, relativePath, isDirectory); err != nil {
// 			return false, err
// 		}
// 	}

// 	return true, nil
// }

// // generateFileContent generates the content of a file and writes it to the specified destination path.
// // It also applies custom code patches and saves a snapshot of the generated content.
// func (c *generationContext) generateFileContent(destPath string, relativePath string, content []byte, params any) error {
// 	var (
// 		buf                  = &bytes.Buffer{}
// 		snapShotRelativePath = strings.TrimPrefix(destPath, c.targetPath)
// 	)

// 	if err := c.snapshot.CheckCustomCode(destPath, snapShotRelativePath); err != nil {
// 		log.Printf("custom code check for file %s failed: %s\n", destPath, err)
// 	}

// 	fout, err := os.Create(destPath)
// 	if err != nil {
// 		return err
// 	}

// 	writer := io.MultiWriter(fout, buf)

// 	defer func() { c.snapshot.Files[snapShotRelativePath] = buf.String() }()
// 	defer fout.Close()

// 	if isDirectCopy(relativePath) {
// 		if _, err = writer.Write(content); err != nil {
// 			return err
// 		}
// 		return fout.Sync()
// 	}

// 	if err = GenerateByTmpl(writer, params, relativePath, string(content)); err != nil {
// 		return err
// 	}

// 	// TODO: apply patches before writing generated code to file
// 	if err = c.snapshot.ApplyPatches(destPath, snapShotRelativePath); err != nil {
// 		log.Printf("failed to apply patch for file %s: error: %s\n", snapShotRelativePath, err)
// 	}

// 	return nil
// }

// func (c *generationContext) usualFile(walkPath string, relativePath string, isDirectory bool) error {
// 	if isDirectory {
// 		if err := os.MkdirAll(c.pathReplacer.Replace(path.Join(c.targetPath, relativePath)), fileMode); err != nil {
// 			return err
// 		}
// 	} else {
// 		relativePath = strings.TrimSuffix(relativePath, ".tmpl")
// 		content, err := fs.ReadFile(templates, walkPath)
// 		if err != nil {
// 			return err
// 		}

// 		return c.generateFileContent(c.pathReplacer.Replace(path.Join(c.targetPath, relativePath)), relativePath, content, c.params)
// 	}

// 	return nil
// }

// func (g *Generator) walkAndGenerateCodeUsingTemplates(targetPath string) error {
// 	log.Printf("create directory structure\n")
// 	snapShotFile := path.Join(targetPath, ".scale/snapshot.gz")

// 	g.prepareSkipListPrefix()

// 	gctx := generationContext{
// 		targetPath:   targetPath,
// 		pathReplacer: g.pathNameReplacer(),
// 		params:       g.GetParams(),
// 		snapshot:     NewSnapshot(),
// 	}

// 	if err := gctx.snapshot.Load(snapShotFile); err != nil {
// 		log.Println("wasn't able to load snapshot:", err)
// 	}

// 	err := fs.WalkDir(templates, rootDirectory, func(walkPath string, d fs.DirEntry, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		isDirectory := d.IsDir()
// 		relativePath := strings.TrimPrefix(walkPath, "templates/")

// 		if inSkipList(relativePath) {
// 			return nil
// 		}

// 		ok, err := gctx.gRPCModuleProcessing(walkPath, relativePath, isDirectory)
// 		if err != nil {
// 			return err
// 		}

// 		if ok {
// 			return nil
// 		}

// 		ok, err = gctx.restModuleProcessing(walkPath, relativePath, isDirectory)
// 		if err != nil {
// 			return err
// 		}

// 		if ok {
// 			return nil
// 		}

// 		ok, err = gctx.repositoryProcessing(walkPath, relativePath, isDirectory)
// 		if err != nil {
// 			return err
// 		}

// 		if ok {
// 			return nil
// 		}

// 		if err = gctx.usualFile(walkPath, relativePath, isDirectory); err != nil {
// 			return err
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		return err
// 	}

// 	if err = os.MkdirAll(path.Join(targetPath, ".scale"), fileMode); err != nil {
// 		return err
// 	}

// 	if err = gctx.snapshot.CompressAndSave(snapShotFile); err != nil {
// 		return err
// 	}

// 	return nil
// }
