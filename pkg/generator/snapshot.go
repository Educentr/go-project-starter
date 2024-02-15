package generator

/*
import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

type Snapshot struct {
	Version    string                            `json:"version"`
	Files      map[string]string                 `json:"files"`
	Patches    map[string][]diffmatchpatch.Patch `json:"-"`
	ManualCode map[string][]byte                 `json:"-"`
	Dmp        *diffmatchpatch.DiffMatchPatch    `json:"-"`
}

func NewSnapshot() Snapshot {
	return Snapshot{
		Version:    "1.0",
		Files:      make(map[string]string),
		Patches:    make(map[string][]diffmatchpatch.Patch),
		ManualCode: make(map[string][]byte),
		Dmp:        diffmatchpatch.New(),
	}
}

func (s *Snapshot) ApplyPatches(destPath string, relativePath string) error {
	patches, ok := s.Patches[relativePath]
	if !ok || len(patches) == 0 {
		return nil
	}

	manualCode := s.ManualCode[relativePath]

	if !fileExists(destPath) {
		return nil
	}

	fin, err := os.Open(destPath)
	if err != nil {
		return err
	}

	content, err := io.ReadAll(fin)
	if err != nil {
		return err
	}

	text, _ := s.Dmp.PatchApply(patches, string(content))

	fout, err := os.Create(destPath)
	if err != nil {
		return err
	}

	defer fout.Close()

	if _, err = fout.WriteString(text); err != nil {
		return err
	}

	if _, err = fout.Write(manualCode); err != nil {
		return err
	}

	return nil
}

func (s *Snapshot) CheckCustomCode(destPath string, relativePath string) error {
	var manualCode []byte

	prevVersion, ok := s.Files[relativePath]
	if !ok {
		return nil
	}

	cmt, _ := makeComment(relativePath, splitter) //nolint:errcheck
	cmt = strings.TrimSpace(cmt)

	if strings.Contains(prevVersion, cmt) && cmt != "" {
		splt := strings.SplitN(prevVersion, cmt, 2) //nolint:gomnd
		prevVersion = splt[0]
	}

	if !fileExists(destPath) {
		return nil
	}

	fin, err := os.Open(destPath)
	if err != nil {
		return err
	}

	content, err := io.ReadAll(fin)
	if err != nil {
		return err
	}

	if len(content) == 0 {
		return nil
	}

	if strings.Contains(string(content), cmt) && cmt != "" {
		splt := strings.SplitN(string(content), cmt, 2) //nolint:gomnd
		content = []byte(splt[0])
		manualCode = []byte(splt[1])
	}

	diffs := s.Dmp.DiffMain(prevVersion, string(content), false)
	if len(diffs) == 0 {
		return nil
	}

	patches := s.Dmp.PatchMake(diffs)
	if len(patches) == 0 {
		return nil
	}

	s.ManualCode[relativePath] = manualCode
	s.Patches[relativePath] = patches

	return nil
}

func (s *Snapshot) Load(path string) error {
	if !fileExists(path) {
		return nil
	}

	fin, err := os.Open(path)
	if err != nil {
		return err
	}

	read, err := gzip.NewReader(fin)
	if err != nil {
		return err
	}

	return json.NewDecoder(read).Decode(s)
}

func (s *Snapshot) CompressAndSave(destination string) error {
	fout, err := os.Create(destination)
	if err != nil {
		return err
	}

	defer fout.Close()

	gz := gzip.NewWriter(fout)
	defer gz.Close()

	return json.NewEncoder(gz).Encode(s)
}
*/
