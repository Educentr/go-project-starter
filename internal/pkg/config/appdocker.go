package config

import (
	"fmt"
	"regexp"

	"github.com/Educentr/go-project-starter/internal/pkg/ds"
	"github.com/pkg/errors"
)

// AppDocker contains the image settings of one application: the platforms its
// image is built for.
//
// The key is per-application and not per-project because the audience of the
// images differs. An application that is run on developer machines (a CLI tool
// shipped as an image) has to work on amd64 and on Apple Silicon, while the
// server applications of the same project usually need one platform and should
// not pay for a second build.
//
// YAML example:
//
//	applications:
//	  - name: cli
//	    docker:
//	      platforms: [linux/amd64, linux/arm64]
//
// See docs/configuration/docker.md for full documentation.
type AppDocker struct {
	// Platforms declares the target platforms of the application image. Each
	// entry is "linux/<arch>". Omitted (or empty) keeps the historical
	// behavior: the image is built for the platform of the builder. Read via
	// Parse.
	Platforms []string `mapstructure:"platforms"`
}

// dockerPlatformRx matches an accepted platform: linux and an architecture.
//
// Only linux, because the runtime stage of the generated Dockerfile is alpine —
// an image for another OS would not run whatever the declaration says. Without
// a variant ("linux/arm/v7"), because the variant does not reach the Go build:
// the builder stage takes the architecture from TARGETARCH and a variant needs
// its own GOARM/GOAMD64 mapping, so accepting one would silently produce a
// binary for the wrong sub-architecture.
var dockerPlatformRx = regexp.MustCompile(`^linux/[a-z0-9]+$`)

// Sentinel errors of the per-application docker section. Use errors.Is to
// distinguish.
var (
	ErrAppDockerPlatform = errors.New(
		`applications[].docker.platforms: expected "linux/<arch>", for example "linux/amd64" or "linux/arm64"`,
	)
	ErrAppDockerPlatformDuplicate = errors.New("applications[].docker.platforms: duplicate platform")
	ErrAppDockerNoImage           = errors.New(
		"applications[].docker.platforms: declared for an application that builds no docker artifact",
	)
)

// Parse normalizes the per-application docker section into ds.AppDockerConfig,
// validating every declared platform. An unparsable platform has to fail here:
// it would otherwise reach `docker buildx build --platform` and fail the build
// of the generated project instead of the generation.
func (d AppDocker) Parse() (ds.AppDockerConfig, error) {
	if len(d.Platforms) == 0 {
		return ds.AppDockerConfig{}, nil
	}

	platforms := make([]string, 0, len(d.Platforms))
	seen := make(map[string]struct{}, len(d.Platforms))

	for _, platform := range d.Platforms {
		if !dockerPlatformRx.MatchString(platform) {
			return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerPlatform, platform)
		}

		if _, dup := seen[platform]; dup {
			return ds.AppDockerConfig{}, fmt.Errorf(errFmtGotValue, ErrAppDockerPlatformDuplicate, platform)
		}

		seen[platform] = struct{}{}

		platforms = append(platforms, platform)
	}

	return ds.AppDockerConfig{Platforms: platforms}, nil
}
