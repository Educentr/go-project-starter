package main

import (
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/Educentr/go-project-starter/internal/pkg/specsource"
)

// bindSourceRefFlag re-creates the flag/viper wiring runGenerator sets up, on
// a fresh flag set so subtests don't leak state into each other.
func bindSourceRefFlag(t *testing.T, flagValue string) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	fs := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
	fs.String(flagSourceRef, "", usageSourceRef)

	if flagValue != "" {
		if err := fs.Set(flagSourceRef, flagValue); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}

	if err := viper.BindPFlags(fs); err != nil {
		t.Fatalf("bind flags: %v", err)
	}
}

func TestResolveSourceRef(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"neither set means no override", "", "", ""},
		{"flag only", "release/a", "", "release/a"},
		{"env only", "", "release/b", "release/b"},
		{"flag wins over env", "release/a", "release/b", "release/a"},
		{"whitespace is trimmed", "", "  release/b  ", "release/b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envSourceRef, tc.env)
			bindSourceRefFlag(t, tc.flag)

			got, err := resolveSourceRef()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSourceRef_RejectsUnusableRef(t *testing.T) {
	t.Setenv(envSourceRef, "")
	bindSourceRefFlag(t, "--upload-pack=evil")

	if _, err := resolveSourceRef(); !errors.Is(err, specsource.ErrInvalidRef) {
		t.Fatalf("got %v, want ErrInvalidRef", err)
	}
}
