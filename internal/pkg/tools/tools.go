package tools

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"gitlab.educentr.info/golang/service-starter/internal/pkg/ds"
)

const (
	fileMode = 0755

	msgStopFailedToOpenGitRepo      = "failed to open git repository (%s): %s"
	msgStopFailedToGetGitWorktree   = "failed to get git worktree (%s): %s"
	msgStopFailedToGetGitStatus     = "failed to get git status (%s): %s"
	msgStopGitHasUncommittedChanges = "git (%v) has uncommitted changes. Please commit them first"
)

func CheckGitStatus(paths ...string) (err error) {
	var (
		rep *git.Repository
		wrt *git.Worktree
		sta git.Status
	)

	for _, path := range paths {
		if rep, err = git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true}); err != nil {
			if errors.Is(err, git.ErrRepositoryNotExists) {
				log.Printf(msgStopFailedToOpenGitRepo, path, err)
				continue
			}

			return fmt.Errorf(msgStopFailedToOpenGitRepo, path, err) //nolint:goerr113,stylecheck
		}

		if wrt, err = rep.Worktree(); err != nil {
			log.Printf(msgStopFailedToGetGitWorktree, path, err)
			return
		}

		if sta, err = wrt.Status(); err != nil {
			log.Printf(msgStopFailedToGetGitStatus, path, err)
			return
		}

		if !sta.IsClean() {
			return fmt.Errorf(msgStopGitHasUncommittedChanges, path) //nolint:goerr113
		}
	}

	return nil
}

func MakeDirs(dirs []ds.Files) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir.DestName, 0700); err != nil && err != os.ErrExist {
			return fmt.Errorf("failed to create directory %s: %w", dir.DestName, err)
		}
	}

	return nil
}

func CreatePathForFile(p string) (err error) {
	if p, err = filepath.Abs(p); err != nil {
		return err
	}

	return os.MkdirAll(filepath.Dir(p), fileMode)
}

func CopyFile(src, dst string) (err error) {
	if err = CreatePathForFile(dst); err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("error create file: %w", err)
	}

	defer destination.Close()

	if _, err = io.Copy(destination, source); err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	return nil
}

func CleanDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}

	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		if err = os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return err
		}
	}

	return nil
}

type retFileExist error

var ErrExist retFileExist = errors.New("file exist")
var ErrInvalid retFileExist = errors.New("invalid file")

func FileExists(filename string) retFileExist {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if info.Mode().IsRegular() {
		return ErrExist
	}

	return ErrInvalid
}
