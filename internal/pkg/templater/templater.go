package templater

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/Educentr/go-project-starter/internal/pkg/grafana"
	"github.com/Educentr/go-project-starter/internal/pkg/meta"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// MinRuntimeVersion is the minimum supported version of go-project-starter-runtime.
// This should be updated when new runtime version is released.
const MinRuntimeVersion = "v0.15.0"

// DefaultPrivateReposTokenSecret is the CI secret name used to fetch private
// Go modules when git.private_repos_token_secret is not set in project.yaml.
const DefaultPrivateReposTokenSecret = "GH_PAT"

type GeneratorParams struct {
	AppInfo         string
	Logger          ds.Logger
	Author          string
	Year            string
	ProjectName     string
	ProjectPath     string
	RegistryType    string
	UseActiveRecord bool
	DevStand        bool
	GenerateLlmsMd  bool
	LlmsFileName    string // "CLAUDE.md" or "LLMS.md" — determined at generation time
	CI              []string
	CISet           bool
	Repo            string
	PrivateRepos    string
	// Read via PrivateReposTokenSecretName, which applies the default.
	PrivateReposTokenSecret string
	DockerImagePrefix       string
	SkipServiceInit         bool
	GoLangVersion           string
	OgenVersion             string
	ArgenVersion            string
	GolangciVersion         string
	LintExcludePaths        []string
	RuntimeVersion          string
	GoJSONSchemaVersion     string
	GoatVersion             string
	GoatServicesVersion     string
	Drivers                 ds.Drivers
	Workers                 ds.Workers
	JSONSchemas             ds.JSONSchemas
	Kafka                   ds.KafkaConfigs
	// GRPCVersion   string
	// Transports ds.Transtorts
	// Models ???
	Applications  ds.Apps
	Grafana       grafana.Config
	Artifacts     ds.ArtifactsConfig
	Documentation ds.DocsConfig
	// GithubCI carries the runner, job timeout and doctl version applied to
	// every job of the generated GitHub Actions workflows.
	GithubCI ds.GithubCIConfig
}

// PrivateReposTokenSecretName returns the name of the CI secret that holds the
// token used to fetch private Go modules, falling back to
// DefaultPrivateReposTokenSecret when the project does not configure one.
// Templates call this instead of reading PrivateReposTokenSecret directly, so
// an unset value never renders as an empty secret reference.
func (g GeneratorParams) PrivateReposTokenSecretName() string {
	if g.PrivateReposTokenSecret == "" {
		return DefaultPrivateReposTokenSecret
	}

	return g.PrivateReposTokenSecret
}

// type GeneratorParamDriver struct {
// 	PackageName string
// 	ImportPath  string
// }

type GeneratorRepositoryParams struct {
	Name       string //unused
	TypeDB     string //unused
	DriverName string //unused
}

type GeneratorAppParams struct {
	GeneratorParams
	Deploy      ds.DeployType
	Application ds.App
}

type GeneratorHandlerParams struct {
	GeneratorParams
	Transport       ds.Transport
	TransportParams map[string]string
}

type GeneratorRunnerParams struct {
	GeneratorParams
	Worker       ds.Worker
	WorkerParams map[string]string
}

// GeneratorKafkaParams holds parameters for Kafka driver template generation
//
//nolint:decorder // follows existing pattern - types after consts
type GeneratorKafkaParams struct {
	GeneratorParams
	Kafka ds.KafkaConfig
}

// GeneratorStaticDirParams holds parameters for generating the static-files
// drop-zone (the <dir>/.keep placeholder) of a static template transport.
//
//nolint:decorder // follows existing pattern - types after consts
type GeneratorStaticDirParams struct {
	GeneratorAppParams
	Dir string
}

type Template struct {
	Name string
	Tmpl string
}

type TemplateCache struct {
	sync.Mutex
	templates map[string]Template
}

// ToDo кажется эта константа бесполезная
const (
	TemplateName string = "ProjectStarterGeneratorTemplate"
)

var (
	tmplErrRx = regexp.MustCompile(`template: .*?:(\d+):(\d+:)? `)
	// importErrRx = regexp.MustCompile(`(\d+):(\d+): `)
)

var cache = TemplateCache{
	templates: make(map[string]Template),
}

func GetTemplate(filename string) (Template, error) {
	cache.Lock()
	defer cache.Unlock()

	if tmpl, ok := cache.templates[filename]; ok {
		return tmpl, nil
	}

	file, err := templates.ReadFile(filename)
	if err != nil {
		return Template{}, fmt.Errorf("failed to read %s: %w", filename, err)
	}

	tmpl := Template{
		Name: filename,
		Tmpl: string(file),
	}

	cache.templates[filename] = tmpl

	return tmpl, nil
}

func GenerateFilenameByTmpl(file *ds.Files, targetPath string, lastVer int) error {
	buf := new(strings.Builder)

	var funcs = template.FuncMap{
		"ToLower":     strings.ToLower,
		"ToUpper":     strings.ToUpper,
		"ReplaceDash": func(s string) string { return strings.ReplaceAll(s, "-", "_") },
		"Capitalize":  cases.Title(language.Und).String,
	}

	templatePackage, err := template.New(TemplateName).Funcs(funcs).Parse(file.DestName)
	if err != nil {
		return fmt.Errorf("error parse template name `%s`: %w", file.DestName, err)
	}

	if err = templatePackage.Execute(buf, file.ParamsTmpl); err != nil {
		return fmt.Errorf("error execute template name `%s`: %w", file.DestName, err)
	}

	destFileName := buf.String()

	if pos := strings.Index(destFileName, ".go"); pos > 0 && pos == len(destFileName)-3 {
		dir, fName := filepath.Split(destFileName)

		// Special handling for _test.go files to preserve Go test file naming convention
		// Go requires test files to end with _test.go, not _gen.go
		if strings.HasSuffix(fName, "_test.go") {
			// Convert main_test.go -> psg_main_test.go (not psg_main_test_gen.go)
			destFileName = filepath.Join(dir, "psg_"+fName)
		} else {
			destFileName = filepath.Join(dir, "psg_"+fName[:len(fName)-3]+"_gen.go")
		}
	}

	oldFileName := destFileName

	// ToDo сделать миграции
	switch {
	case lastVer < 2:
		oldFileName = buf.String()
	case lastVer < 3:
		if strings.Contains(destFileName, "pkg/ds") {
			oldFileName = strings.ReplaceAll(destFileName, "pkg/ds", "pkg/app/ds")
		}
	case lastVer < 4:
		// Migration v3→v4: tests/{app_name}/{file} -> old location was tests/{file}
		if strings.HasPrefix(destFileName, "tests/") {
			parts := strings.SplitN(destFileName, "/", 3)
			if len(parts) == 3 {
				// New: tests/apibalancer/psg_config_gen.go → Old: tests/psg_config_gen.go
				oldFileName = filepath.Join("tests", parts[2])
			}
		}
	}

	file.DestName = filepath.Join(targetPath, destFileName)
	file.OldDestName = filepath.Join(targetPath, oldFileName)

	return nil
}

// ToDo сделать проверку, что эта строка есть в файле дисклеймера
// для случаев, когда дисклеймер меняется, надо поддержать массив строк для поиска
// CI должен проверять, что массив строк "никогда" не уменьшается, что бы не нарушать обратную совместимость
const (
	disclaimer = "If you need you can add your code after this message"
)

var (
	ignoreExistingPath = []string{
		".git/",
		"docs/",
	}
	ignoreIfExistsFiles = map[string]struct{}{
		".gitignore":           {},
		"go.mod":               {},
		"go.sum":               {},
		"LICENSE.txt":          {},
		"README.md":            {},
		"LLMS.md":              {},
		"CLAUDE.md":            {},
		"etc/onlineconf/.keep": {},
		"public/.keep":         {}, // ToDo ignore all keep files
	}
)

func isFileIgnore(path string) bool {
	if _, ok := ignoreIfExistsFiles[path]; ok {
		return true
	}

	for _, ignorePath := range ignoreExistingPath {
		if strings.HasPrefix(path, ignorePath) {
			return true
		}
	}

	return false
}

// IsUserOwned reports whether a project-relative path is written once and
// owned by the user afterwards (go.mod, .gitignore, README.md, static assets
// and the like). Such files are never rewritten or diffed, so they are not
// tracked in the manifest either — which keeps the manifest a function of the
// config rather than of what happened to be on disk.
func IsUserOwned(relPath string, extraIgnore ...string) bool {
	return isFileIgnore(relPath) || hasAnyPrefix(relPath, extraIgnore)
}

// hasAnyPrefix reports whether path begins with any of the given prefixes.
func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}

// GetUserCodeFromFiles diffs the generated file set against the target dir,
// preserving user code below disclaimer markers. prev is the manifest of the
// previous run (see meta.Manifest); when it is non-empty orphan detection is
// exact — only paths the generator itself wrote are read, diffed or deleted.
// An empty manifest (project created before meta.yaml v5) falls back to
// scanning the whole target tree and guessing from file contents.
// extraIgnore holds additional project-relative path prefixes (e.g. "static/")
// whose existing contents must never be read, diffed, or deleted — used for
// user-provided static assets.
func GetUserCodeFromFiles(targetDir string, files []ds.Files, prev meta.Manifest, extraIgnore ...string) (ds.FilesDiff, error) {
	filesDiff := ds.FilesDiff{
		NewFiles:       make(map[string]struct{}),
		IgnoreFiles:    make(map[string]struct{}),
		NewDirectory:   make(map[string]struct{}),
		OtherFiles:     make(map[string]struct{}),
		ObsoleteFiles:  make(map[string]struct{}),
		OtherDirectory: make(map[string]struct{}),
		UserContent:    make(map[string][]byte),
		RenameFiles:    make(map[string]string),
		DriftFiles:     make(map[string]string),
	}

	for _, file := range files {
		filesDiff.NewFiles[file.DestName] = struct{}{}
		dir := file.DestName

		for {
			dir, _ = filepath.Split(dir)
			if _, ex := filesDiff.NewDirectory[dir]; ex || len(dir) < len(targetDir) {
				break
			}

			filesDiff.NewDirectory[dir] = struct{}{}
			dir = dir[:len(dir)-1]
		}

		if file.OldDestName != file.DestName {
			filesDiff.RenameFiles[file.OldDestName] = file.DestName
		}
	}

	if len(prev) > 0 {
		if err := diffAgainstManifest(&filesDiff, targetDir, files, prev, extraIgnore); err != nil {
			return ds.FilesDiff{}, err
		}

		return filesDiff, nil
	}

	if err := scanTargetTree(&filesDiff, targetDir, extraIgnore); err != nil {
		return ds.FilesDiff{}, err
	}

	return filesDiff, nil
}

// GeneratedRegionHash returns the hash of the generated region of content:
// everything up to and including the disclaimer line. Files that carry no
// disclaimer (JSON, go.mod, .keep …) are hashed whole — the generator owns
// them end to end. The returned value is what gets stored in
// meta.FileRecord.Hash.
func GeneratedRegionHash(destName string, content []byte) (string, error) {
	_, fname := filepath.Split(destName)

	if isFileIgnored(fname) {
		return meta.HashContent(content), nil
	}

	genPart, _, err := splitDisclaimer(string(content))
	if err != nil {
		return "", err
	}

	return meta.HashContent([]byte(genPart)), nil
}

// diffAgainstManifest fills filesDiff using the previous run's manifest as the
// source of truth for "was this file generated by us". Files outside the
// manifest and outside the current output set are never opened, so a foreign
// file that happens to contain the disclaimer string (an editor backup, a
// *_gen.go.orig left by a merge, a code-intelligence index) neither aborts the
// run nor gets deleted.
func diffAgainstManifest(filesDiff *ds.FilesDiff, targetDir string, files []ds.Files, prev meta.Manifest, extraIgnore []string) error {
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return errors.Wrap(err, "error resolve target dir")
	}

	prevIdx := prev.Index()
	curSet := make(map[string]struct{}, len(files))

	for _, file := range files {
		rel, err := meta.RelPath(absTarget, file.DestName)
		if err != nil {
			return errors.Wrap(err, "error resolve relative path of "+file.DestName)
		}

		curSet[rel] = struct{}{}

		if err := diffExpectedFile(filesDiff, file, rel, prevIdx, extraIgnore); err != nil {
			return err
		}
	}

	// Existing directories are not "new" — keeps the dry-run report honest.
	for dir := range filesDiff.NewDirectory {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			delete(filesDiff.NewDirectory, dir)
		}
	}

	return diffOrphans(filesDiff, absTarget, prev, curSet)
}

// diffExpectedFile classifies one file of the current output set against its
// on-disk state: user code below the marker is preserved, hand edits above the
// marker are reported as drift, and files that must not be touched once they
// exist are moved to IgnoreFiles.
func diffExpectedFile(filesDiff *ds.FilesDiff, file ds.Files, rel string, prevIdx map[string]meta.FileRecord, extraIgnore []string) error {
	// A pending rename means the content still lives under the old name.
	srcPath := file.DestName
	if file.OldDestName != "" && file.OldDestName != file.DestName && !regularFileExists(file.DestName) && regularFileExists(file.OldDestName) {
		srcPath = file.OldDestName
	}

	if isFileIgnore(rel) || hasAnyPrefix(rel, extraIgnore) {
		// Generated once, owned by the user afterwards.
		if regularFileExists(srcPath) {
			delete(filesDiff.NewFiles, file.DestName)
			filesDiff.IgnoreFiles[file.DestName] = struct{}{}
		}

		return nil
	}

	st, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Brand new file — stays in NewFiles.
			return nil
		}

		return errors.Wrap(err, "error stat file "+srcPath)
	}

	if !st.Mode().IsRegular() {
		return errors.New("target path is not a regular file: " + srcPath)
	}

	delete(filesDiff.NewFiles, file.DestName)

	fileContent, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	_, fname := filepath.Split(srcPath)
	if !isFileIgnored(fname) {
		_, userData, err := splitDisclaimer(string(fileContent))
		if err != nil {
			// ToDo сделать force режим
			return errors.Wrap(err, "error split disclaimer in file "+srcPath)
		}

		if len(userData) > 0 {
			filesDiff.UserContent[file.DestName] = []byte(userData)
		}
	}

	rec, ok := prevIdx[rel]
	if !ok || rec.Hash == "" {
		return nil
	}

	hash, err := GeneratedRegionHash(srcPath, fileContent)
	if err != nil {
		return errors.Wrap(err, "error hash file "+srcPath)
	}

	if hash != rec.Hash {
		filesDiff.DriftFiles[srcPath] = "generated part was changed after the last generation, the changes will be overwritten"
	}

	return nil
}

// diffOrphans walks the previous manifest and decides what to do with the
// files that dropped out of the current output set.
func diffOrphans(filesDiff *ds.FilesDiff, absTarget string, prev meta.Manifest, curSet map[string]struct{}) error {
	for _, rec := range prev {
		if _, ex := curSet[rec.Path]; ex {
			continue
		}

		path := filepath.Join(absTarget, filepath.FromSlash(rec.Path))

		// Renames are applied separately, the file is not an orphan.
		if _, ex := filesDiff.RenameFiles[path]; ex {
			continue
		}

		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return errors.Wrap(err, "error stat file "+path)
		}

		if !st.Mode().IsRegular() {
			filesDiff.OtherFiles[path] = struct{}{}

			continue
		}

		// A copied file (spec, schema) dropped from the config is left alone:
		// its content is not ours to judge, and it may be referenced by hand.
		if rec.Kind != meta.KindGenerated {
			filesDiff.OtherFiles[path] = struct{}{}

			continue
		}

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if _, userData, errSplit := splitDisclaimer(string(fileContent)); errSplit == nil && len(userData) > 0 {
			// Has user code below disclaimer — cannot auto-delete
			return errors.New("found user code in stale gen file " + absTarget + " / " + path)
		}

		hash, errHash := GeneratedRegionHash(path, fileContent)
		if errHash != nil || (rec.Hash != "" && hash != rec.Hash) {
			// Edited since we wrote it — report instead of deleting silently.
			filesDiff.OtherFiles[path] = struct{}{}
			filesDiff.DriftFiles[path] = "no longer generated but changed after the last generation, left on disk"

			continue
		}

		filesDiff.ObsoleteFiles[path] = struct{}{}
	}

	return nil
}

func regularFileExists(path string) bool {
	st, err := os.Stat(path)

	return err == nil && st.Mode().IsRegular()
}

// scanTargetTree is the pre-manifest fallback: it walks the whole target tree
// and infers what used to be generated from the disclaimer string in the file
// content. Used for projects whose meta.yaml carries no manifest yet; the run
// writes one, so the next run takes the exact path.
func scanTargetTree(filesDiff *ds.FilesDiff, targetDir string, extraIgnore []string) error {
	foundDirs := make(map[string]struct{})

	err := fs.WalkDir(os.DirFS(targetDir), ".", func(relPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		path := filepath.Join(targetDir, relPath)

		if isFileIgnore(relPath) || hasAnyPrefix(relPath, extraIgnore) {
			delete(filesDiff.NewFiles, path)
			filesDiff.IgnoreFiles[path] = struct{}{}

			return nil
		}

		newFile := path
		if renameFile, ex := filesDiff.RenameFiles[path]; ex {
			newFile = renameFile
		}

		if d.IsDir() {
			newFile = newFile + "/"
			if _, ok := filesDiff.NewDirectory[newFile]; ok {
				foundDirs[newFile] = struct{}{}
			} else {
				filesDiff.OtherDirectory[newFile] = struct{}{}
			}

			return nil
		}

		if _, ex := filesDiff.NewFiles[newFile]; ex {
			delete(filesDiff.NewFiles, newFile)

			// Files without disclaimer support (e.g., JSON) are fully overwritten
			// No user code to preserve, just continue
			_, fname := filepath.Split(path)
			if isFileIgnored(fname) {
				return nil
			}

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			_, userData, err := splitDisclaimer(string(fileContent))
			if err != nil {
				// ToDo сделать force режим
				return errors.Wrap(err, "error split disclaimer in file "+path)
			}

			if len(userData) > 0 {
				filesDiff.UserContent[newFile] = []byte(userData)
			}
		} else {
			// Files without disclaimer support (e.g., JSON) - just mark as other files
			_, fname := filepath.Split(path)
			if isFileIgnored(fname) {
				filesDiff.OtherFiles[path] = struct{}{}

				return nil
			}

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			_, userData, err := splitDisclaimer(string(fileContent))
			if err == nil {
				// File has disclaimer — it was generated by us but is no longer in template set
				if len(userData) > 0 {
					// Has user code below disclaimer — cannot auto-delete
					return errors.New("found user code in stale gen file " + targetDir + " / " + path)
				}

				// No user code — safe to delete
				filesDiff.ObsoleteFiles[path] = struct{}{}

				return nil
			}

			filesDiff.OtherFiles[path] = struct{}{}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for delDir := range foundDirs {
		delete(filesDiff.NewDirectory, delDir)
	}

	return nil
}

func splitDisclaimer(fileContent string) (string, string, error) {
	disclamerFind := strings.Index(fileContent, disclaimer)
	if disclamerFind == -1 {
		return fileContent, "", errors.New("end disclaimer not found in file")
	}

	newLine := strings.Index(fileContent[disclamerFind+len(disclaimer):], "\n")
	if newLine == -1 {
		return fileContent[:disclamerFind], "", nil
	}

	userData := fileContent[disclamerFind+len(disclaimer)+newLine+1:]

	return fileContent[:disclamerFind+len(disclaimer)+newLine+1], userData, nil
}

const bufferSizeStep = 1024

func GenerateByTmpl(tmpl Template, params any, userCode []byte, destPath string) (*bytes.Buffer, error) {
	startDisclaimer, err := makeStartDisclaimer(destPath)
	if err != nil {
		return nil, err
	}

	finishDisclaimer, err := makeFinishDisclaimer(destPath)
	if err != nil {
		return nil, err
	}

	var funcs = template.FuncMap{
		"ToLower":     strings.ToLower,
		"ToUpper":     strings.ToUpper,
		"ReplaceDash": func(s string) string { return strings.ReplaceAll(s, "-", "_") },
		"Capitalize":  cases.Title(language.Und).String,
		"CapitalizeFirst": func(s string) string {
			if s == "" {
				return s
			}
			// Split by both "-" and "_" to produce PascalCase
			parts := strings.FieldsFunc(s, func(r rune) bool {
				return r == '-' || r == '_'
			})
			for i, p := range parts {
				if p == "" {
					continue
				}
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
			return strings.Join(parts, "")
		},
		"errorf": func(format string, args ...any) error { return fmt.Errorf(format, args...) },
		"add":    func(a, b int) int { return a + b },
		"escapeJSON": func(s string) string {
			// Escape special characters for JSON string
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return s
		},
		"ImageNameToPullerService": func(image string) string {
			// Extract image name from path like ghcr.io/org/name:tag -> name
			lastSlash := strings.LastIndex(image, "/")
			name := image
			if lastSlash >= 0 {
				name = image[lastSlash+1:]
			}
			// Remove tag
			if colonIndex := strings.Index(name, ":"); colonIndex >= 0 {
				name = name[:colonIndex]
			}
			return name + "-image-puller"
		},
		"ToGitHubSecret": func(s string) string {
			// Replace ${VAR_NAME} with ${{ secrets.VAR_NAME }} for GitHub Actions
			re := regexp.MustCompile(`\$\{([^}]+)\}`)
			return re.ReplaceAllString(s, `${{ secrets.$1 }}`)
		},
		// CLI template helpers for flag handling
		"flagDefault": func(goType, defaultVal string) string {
			if defaultVal != "" {
				switch goType {
				case "string":
					return `"` + defaultVal + `"`
				case "bool":
					return defaultVal
				default:
					return defaultVal
				}
			}
			switch goType {
			case "string":
				return `""`
			case "int":
				return "0"
			case "bool":
				return "false"
			case "float64":
				return "0"
			case "time.Duration":
				return "0"
			default:
				return `""`
			}
		},
		"flagZeroCheck": func(goType, varName string) string {
			switch goType {
			case "string":
				return "*" + varName + ` == ""`
			case "int":
				return "*" + varName + " == 0"
			case "float64":
				return "*" + varName + " == 0"
			case "bool":
				return "!*" + varName
			case "time.Duration":
				return "*" + varName + " == 0"
			default:
				return "*" + varName + ` == ""`
			}
		},
		"flagDeref": func(goType, varName string) string {
			return "*" + varName
		},
		"printf": fmt.Sprintf,
		// Queue serialization template helpers
		"serializeField": func(field ds.QueueField) string {
			switch field.Type {
			case "int":
				return "\t_ = binary.Write(&buf, binary.LittleEndian, int64(task." + field.GoName + "))\n"
			case "int64":
				return "\t_ = binary.Write(&buf, binary.LittleEndian, task." + field.GoName + ")\n"
			case "string":
				return "\twriteString(&buf, task." + field.GoName + ")\n"
			case "bool":
				return "\t_ = binary.Write(&buf, binary.LittleEndian, task." + field.GoName + ")\n"
			case "[]byte":
				return "\twriteBytes(&buf, task." + field.GoName + ")\n"
			case "[]int":
				return "\twriteIntSlice(&buf, task." + field.GoName + ")\n"
			case "[]int64":
				return "\twriteInt64Slice(&buf, task." + field.GoName + ")\n"
			default:
				return ""
			}
		},
		"deserializeField": func(field ds.QueueField) string {
			switch field.Type {
			case "int":
				return "\t{\n\t\tvar v int64\n\t\tif err := binary.Read(r, binary.LittleEndian, &v); err != nil {\n\t\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t\t}\n\t\ttask." + field.GoName + " = int(v)\n\t}\n"
			case "int64":
				return "\tif err := binary.Read(r, binary.LittleEndian, &task." + field.GoName + "); err != nil {\n\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t}\n"
			case "string":
				return "\t{\n\t\tv, err := readString(r)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t\t}\n\t\ttask." + field.GoName + " = v\n\t}\n"
			case "bool":
				return "\tif err := binary.Read(r, binary.LittleEndian, &task." + field.GoName + "); err != nil {\n\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t}\n"
			case "[]byte":
				return "\t{\n\t\tv, err := readBytes(r)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t\t}\n\t\ttask." + field.GoName + " = v\n\t}\n"
			case "[]int":
				return "\t{\n\t\tv, err := readIntSlice(r)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t\t}\n\t\ttask." + field.GoName + " = v\n\t}\n"
			case "[]int64":
				return "\t{\n\t\tv, err := readInt64Slice(r)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf(\"read " + field.Name + ": %w\", err)\n\t\t}\n\t\ttask." + field.GoName + " = v\n\t}\n"
			default:
				return ""
			}
		},
	}

	buf := &bytes.Buffer{}

	buf.Grow(len(userCode) + bufferSizeStep*((len(startDisclaimer)+len(tmpl.Tmpl)+len(finishDisclaimer))/bufferSizeStep) + 1)

	templatePackage, err := template.New(destPath).Funcs(funcs).Parse(startDisclaimer + tmpl.Tmpl + "\n" + finishDisclaimer)
	if err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return nil, errors.New("error parse template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	if err = templatePackage.Execute(buf, params); err != nil {
		tmplLines, errGetLine := getTmplErrorLine(strings.SplitAfter(startDisclaimer+tmpl.Tmpl+"\n"+finishDisclaimer, "\n"), err.Error())
		if errGetLine != nil {
			tmplLines = errGetLine.Error()
		}

		return nil, errors.New("error execute template `" + tmpl.Name + "` at line " + tmplLines + ": " + err.Error())
	}

	if len(userCode) > 0 {
		buf.Write(userCode)
	}

	return buf, nil
}

func getTmplErrorLine(lines []string, tmplErr string) (string, error) {
	lineTmpl := tmplErrRx.FindStringSubmatch(tmplErr)
	if len(lineTmpl) > 1 {
		lineNum, errParse := strconv.ParseInt(lineTmpl[1], 10, 64)
		if errParse != nil {
			return "", errors.New("error get line from error")
		} else if len(lines) == 0 {
			return "", errors.New("empty lines in template")
		} else {
			cntLine := 3
			startLine := int(lineNum) - cntLine - 1
			if startLine < 0 {
				startLine = 0
			}

			stopLine := int(lineNum) + cntLine
			if stopLine > int(lineNum) {
				stopLine = int(lineNum)
			}

			errorLines := lines[startLine:stopLine]
			for num := range errorLines {
				if num == cntLine {
					errorLines[num] = "-->> " + errorLines[num]
				} else {
					errorLines[num] = "     " + errorLines[num]
				}
			}

			return "\n" + strings.Join(errorLines, ""), nil
		}
	} else {
		return "", errors.New("can't get line from error")
	}
}
