package test

import (
	"regexp"
	"strings"
	"testing"
)

// makefileRuleLine matches a rule line of the generated Makefile: a list of
// plain target names at column zero, followed by ':' and not by '=' (which
// would make it a variable assignment such as `export PATH := …`).
//
// Plain names only, deliberately. A rule whose target is built from a variable
// or a path is not something the generated Makefile has, and admitting one
// would mean guessing what it expands to.
var makefileRuleLine = regexp.MustCompile(`^([a-zA-Z0-9_.%-]+(?: [a-zA-Z0-9_.%-]+)*):([^=]|$)`)

// makefileRules returns every target the Makefile declares and, per rule line,
// the prerequisites it lists.
//
// Prerequisites are cut at the first '#': `dep-ci: ## Install dependencies` is
// the help-text convention of this Makefile, and the words of a help string are
// not prerequisites in any sense a reader would recognize.
func makefileRules(makefile string) (map[string]bool, map[string][]string) {
	targets := make(map[string]bool)
	prereqs := make(map[string][]string)

	for _, line := range strings.Split(makefile, "\n") {
		match := makefileRuleLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		names := strings.Fields(match[1])
		for _, name := range names {
			targets[name] = true
		}

		rest := line[strings.Index(line, ":")+1:]
		if hash := strings.IndexByte(rest, '#'); hash >= 0 {
			rest = rest[:hash]
		}

		if deps := strings.Fields(rest); len(deps) > 0 {
			prereqs[names[0]] = append(prereqs[names[0]], deps...)
		}
	}

	return targets, prereqs
}

// TestMakefilePrerequisitesResolve checks that every prerequisite in the
// generated Makefile names a target the same Makefile declares (TOOLS-96,
// issue #124 item 2).
//
// A prerequisite that resolves to nothing is invisible until someone runs the
// target: `test-integration-with-deps` depended on `test-integration`, which
// the generator never emitted, and `test-int-up-deps` brought up a compose
// service `wiremock` that no compose template declares. Both shipped for as
// long as nobody tried them. This is the check that would have caught it, so
// it guards the whole target graph rather than those two names.
func TestMakefilePrerequisitesResolve(t *testing.T) {
	// Fixtures chosen for target-graph breadth: a multi-app project with
	// workers and a gRPC client, a CLI-only project, and a project with GOAT
	// tests and use_envs (the goat-* and test-env-* families).
	for _, fixture := range []string{"combined", "cli-only", "rest-envs-goat"} {
		t.Run(fixture, func(t *testing.T) {
			makefile := readGenerated(t,
				generateFromConfig(t, fixture, "Generate "+fixture+" for Makefile target graph test"), "Makefile")

			targets, prereqs := makefileRules(makefile)

			if len(targets) == 0 {
				t.Fatal("no rules found in the generated Makefile — the rule parser stopped matching")
			}

			for target, deps := range prereqs {
				for _, dep := range deps {
					if !targets[dep] {
						t.Errorf("target %q depends on %q, which no rule in the generated Makefile declares", target, dep)
					}
				}
			}
		})
	}
}

// TestMakefileHasNoWiremockDependency pins the other half of the dead
// test-integration targets: the compose service they tried to start and the
// module they were the only reason to require.
//
// Separate from the graph check above because nothing about it is a dangling
// prerequisite — `docker compose up wiremock` is a recipe line, and a recipe
// naming a service no compose file declares fails only when run.
func TestMakefileHasNoWiremockDependency(t *testing.T) {
	tmpDir := generateFromConfig(t, "combined", "Generate project for wiremock leftovers test")

	for _, file := range []string{"Makefile", "go.mod"} {
		if content := readGenerated(t, tmpDir, file); strings.Contains(content, "wiremock") {
			t.Errorf("%s should not mention wiremock: no generated compose file declares the service, "+
				"and no generated code imports the module", file)
		}
	}
}
