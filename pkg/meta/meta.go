package meta

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const curVer = 2

type Meta struct {
	Path    string `yaml:"-"`
	Version int    `yaml:"version"`
}

func GetDefaultMeta(path string) Meta {
	return Meta{
		Path:    path,
		Version: 1,
	}
}

func GetMeta(baseDir, metaPath string) (Meta, error) {
	var meta Meta

	realMetaPath := metaPath

	if !strings.Contains(metaPath, "/") {
		realMetaPath = filepath.Join(baseDir, metaPath)
	}

	meta = GetDefaultMeta(realMetaPath)

	source, err := os.ReadFile(realMetaPath)
	if err != nil {
		errNotFound := &fs.PathError{}
		if errors.As(err, &errNotFound) {
			return meta, nil
		}
	}

	if err := yaml.Unmarshal(source, &meta); err != nil {
		return meta, err
	}

	meta.Path = realMetaPath

	return meta, nil
}

func (m *Meta) Save() error {
	m.Version = curVer

	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(m.Path, data, 0644)
}
