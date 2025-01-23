package templater

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"gitlab.educentr.info/golang/service-starter/pkg/ds"
)

//go:embed all:embedded
var templates embed.FS

//go:embed embedded/disclaimer.txt
var disclaimerTop string

//go:embed embedded/finish_disclaimer.txt
var disclaimerBottom string

func GetTemplates(templateFS embed.FS, prefix string, params any) (dirs []ds.Files, files []ds.Files, err error) {
	dirs = []ds.Files{}
	files = []ds.Files{}

	trimCnt := len(strings.Split(prefix, string(os.PathSeparator)))

	err = fs.WalkDir(templateFS, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			dirs = append(dirs, ds.Files{
				SourceName: path,
				DestName:   filepath.Join(strings.Split(path, string(os.PathSeparator))[trimCnt:]...),
				ParamsTmpl: params,
			})

			return nil
		}

		files = append(files, ds.Files{
			SourceName: path,
			DestName:   strings.TrimSuffix(filepath.Join(strings.Split(path, string(os.PathSeparator))[trimCnt:]...), ".tmpl"),
			ParamsTmpl: params,
			Code:       &bytes.Buffer{},
		})

		return nil
	})

	if err != nil {
		err = errors.Wrapf(err, "error while walk dir `%s`", prefix)
	}

	return
}

func GetMainTemplates(params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/main", params)
	if err != nil {
		err = errors.Wrap(err, "error while get main templates")
	}

	return
}

func GetLoggerTemplates(path string, dst string, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/logger", path), params)
	if err != nil {
		err = errors.Wrap(err, "error while get main templates")
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(dst, dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(dst, files[i].DestName)
	}

	return
}

func GetTransportTemplates(transportType ds.TransportType, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/transport", string(transportType), "files"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get transport templates")

		return
	}

	return
}

func GetTransportGeneratorTemplates(transportType ds.TransportType, generatorType string, params GeneratorParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, filepath.Join("embedded/templates/transport", string(transportType), generatorType, "config"), params)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil

			return
		}

		err = errors.Wrap(err, "error while get transport templates")

		return
	}

	return
}

var (
	prefixDirs = map[string]string{
		"transport": "internal/app/transport/{{ .Transport.Type }}/{{ .Transport.Handler.Name }}/{{ .Transport.Handler.ApiVersion }}",
		"app":       "cmd/{{ .Application.Name }}",
	}
)

type MapTemplateFileName struct {
	dirs  []ds.Files
	files []ds.Files
}

type TemplateFileNameCache struct {
	sync.Mutex
	cache map[string]MapTemplateFileName
}

var (
	templateFileNameCache = TemplateFileNameCache{
		cache: make(map[string]MapTemplateFileName),
	}
)

func GetTransportHandlerTemplates(transport ds.TransportType, template string, params GeneratorHandlerParams) (dirs, files []ds.Files, err error) {
	cacheKey := filepath.Join("embedded/templates/transport", string(transport), template, "files")

	templateFileNameCache.Lock()
	defer templateFileNameCache.Unlock()

	if v, ok := templateFileNameCache.cache[cacheKey]; ok {
		return v.dirs, v.files, nil
	}

	dirs, files, err = GetTemplates(templates, cacheKey, params)
	if err != nil {
		err = errors.Wrapf(err, "error while get transport handler templates `%s`", cacheKey)

		return
	}

	for i := range dirs {
		dirs[i].DestName = filepath.Join(prefixDirs["transport"], dirs[i].DestName)
	}

	for i := range files {
		files[i].DestName = filepath.Join(prefixDirs["transport"], files[i].DestName)
	}

	templateFileNameCache.cache[cacheKey] = MapTemplateFileName{dirs, files}

	return dirs, files, nil

}

func GetAppTemplates(params GeneratorAppParams) (dirs []ds.Files, files []ds.Files, err error) {
	dirs, files, err = GetTemplates(templates, "embedded/templates/app/files", params)
	if err != nil {
		err = errors.Wrap(err, "error while get app templates")

		return
	}

	for i := range files {
		ext := filepath.Ext(files[i].DestName)
		fname := strings.TrimSuffix(files[i].DestName, ext)

		files[i].DestName = fname + "-" + params.Application.Name + ext
	}

	dirsC, filesC, err := GetTemplates(templates, "embedded/templates/app/cmd", params)
	if err != nil {
		err = errors.Wrap(err, "error while get app templates")

		return
	}

	for i := range dirsC {
		dirsC[i].DestName = filepath.Join(prefixDirs["app"], dirsC[i].DestName)
	}

	for i := range filesC {
		filesC[i].DestName = filepath.Join(prefixDirs["app"], filesC[i].DestName)
	}

	dirs = append(dirs, dirsC...)
	files = append(files, filesC...)

	return
}
