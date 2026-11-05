package config

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/pkg/errors"
)

type (
	// CustomGenerator declares a code generator the project brings itself:
	// how to install it, how to run it and, optionally, the linter that ships
	// with it. There is no package-manager model here on purpose — the version
	// and the way to install it belong to the generator's own ecosystem
	// (`go install …@v1`, `npm ci` against a lockfile, `pip install -r`), so
	// the pin lives inside the install command rather than in a separate key.
	//
	// The three commands land on the three targets the generator already
	// fills: install → install-tools, cmd → generate (and regenerate through
	// it), lint → lint.
	//
	// YAML example:
	//
	//	generator:
	//	  - name: runtime_limits_ts
	//	    install: npm ci --prefix web
	//	    cmd: node web/scripts/generate-runtime-limits.mjs {{ .Spec }} --out {{ .Target }}
	//	    lint: npx --prefix web spec-lint {{ .Spec }}
	//
	// See docs/configuration/custom-generators.md for full documentation.
	CustomGenerator struct {
		// Name identifies the generator and is referenced by
		// Contract.GeneratorName. It becomes the suffix of the generated make
		// targets, so it must match ^[a-z0-9_-]+$ and must not collide with a
		// built-in target. Required.
		Name string `mapstructure:"name"`
		// Install installs the generator. Optional — leave it empty when the
		// tool fetches itself (`go run tool@version`, `npx`).
		// No template variables.
		Install string `mapstructure:"install"`
		// Cmd runs the generator against one contract spec. Required.
		// Template variables: .Spec, .Target.
		Cmd string `mapstructure:"cmd"`
		// Lint is the linter shipped with the generator, run against the spec.
		// Optional. Template variables: .Spec, .Target.
		Lint string `mapstructure:"lint"`
	}

	// Contract declares a spec the generator fetches for a custom generator:
	// where to take it from, where to put it, and which generator consumes it.
	// This is not a "copy the file as-is" mode — an input nothing is generated
	// from is not a contract, which is why GeneratorName is required.
	//
	// YAML example:
	//
	//	contract:
	//	  - path: git+ssh://git@github.com/org/contracts.git#limits/runtime_limits.yaml
	//	    copy: api/contract/runtime_limits.yaml
	//	    target: web/src/generated/runtimeLimits.ts
	//	    generator_name: runtime_limits_ts
	//
	// See docs/configuration/custom-generators.md for full documentation.
	Contract struct {
		// Path is the spec source. Same grammar as rest[].path: a local path,
		// an https URL, or a git+ssh/git+https URI. Required.
		Path string `mapstructure:"path"`
		// Copy is the project-relative destination of the fetched spec, and
		// the value of .Spec in the generator commands. Required.
		Copy string `mapstructure:"copy"`
		// Target is the project-relative output path handed to the generator
		// as .Target. Optional — required only when a command references it.
		Target string `mapstructure:"target"`
		// GeneratorName references a CustomGenerator by name. Required.
		GeneratorName string `mapstructure:"generator_name"`
	}

	// generatorCommand pairs a templated command with the config field it
	// came from, so a validation failure can name it.
	generatorCommand struct {
		field string
		text  string
	}

	CustomGeneratorList []CustomGenerator
	ContractList        []Contract
)

// Variables a generator command may reference. There are exactly two: the
// generator is told where the contract landed and where its output belongs;
// everything else about the invocation is the project's own business and
// belongs in the command string.
const (
	// GeneratorVarSpec is the project-relative path of the copied contract
	// spec — the `copy` field of the contract entry.
	GeneratorVarSpec = "Spec"
	// GeneratorVarTarget is the `target` field of the contract entry.
	GeneratorVarTarget = "Target"

	// generatorCommandTmplName names the parsed command template; it only
	// shows up in text/template's own error messages.
	generatorCommandTmplName = "generator-command"

	errMsgEmptyGeneratorName = "Empty generator name"
	errMsgEmptyContractPath  = "Empty contract path"
)

// Errors reported for a generator command that parses but cannot be used.
// Static so that callers can match them; the offending command is wrapped in.
var (
	// ErrGeneratorCommandEmpty is returned when a command renders to nothing,
	// which would leave an empty make recipe line.
	ErrGeneratorCommandEmpty = errors.New("generator command renders empty")
	// ErrGeneratorCommandMultiline is returned when a command renders to more
	// than one line: a recipe line cannot carry an embedded newline, the rest
	// would become a separate unindented make line.
	ErrGeneratorCommandMultiline = errors.New("generator command renders to more than one line")
	// ErrGeneratorVarUnknown is returned for a variable the generator commands
	// do not support.
	ErrGeneratorVarUnknown = errors.New("unknown generator command variable")
)

// generatorNameRe is the accepted shape of a custom generator name. The name
// becomes a make target suffix (install-<name>, generate-<name>, lint-<name>),
// so it is restricted to what a target name may contain.
var generatorNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// reservedGeneratorNames are names whose derived make targets would collide
// with a target the generated Makefile already defines (see
// internal/pkg/templater/embedded/templates/main/Makefile.tmpl). A collision
// would silently override a built-in step, so it is rejected at config load.
var reservedGeneratorNames = map[string]struct{}{ //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards
	// install-*
	"argen":              {},
	"buf":                {},
	"go-jsonschema":      {},
	"goat":               {},
	"goimports":          {},
	"hooks":              {},
	"lint":               {},
	"nfpm":               {},
	"ogen":               {},
	"onlineconf-updater": {},
	"protoc-gen-go-grpc": {},
	"sqlc":               {},
	"tools":              {},
	// generate-*
	"ansible":    {},
	"jsonschema": {},
	"mocks":      {},
	// lint-*
	"fix":  {},
	"full": {},
}

// reservedGeneratorPrefixes are name prefixes reserved for the same reason as
// reservedGeneratorNames, where the built-in target name is parameterized
// (generate-test-env-<app>).
var reservedGeneratorPrefixes = []string{"test-env-"} //nolint:gochecknoglobals // fixed lookup table, written as a literal and read-only afterwards

// IsValid validates a single generator declaration in isolation. Cross-checks
// against the contract list (duplicate names, dangling references, a command
// that reads .Target from a contract that has none) are in ValidateGenerators.
func (g CustomGenerator) IsValid() (bool, string) {
	if g.Name == "" {
		return false, errMsgEmptyGeneratorName
	}

	if !generatorNameRe.MatchString(g.Name) {
		return false, "generator name must match ^[a-z0-9_-]+$, got: " + g.Name
	}

	if _, reserved := reservedGeneratorNames[g.Name]; reserved {
		return false, "generator name '" + g.Name + "' collides with a built-in make target"
	}

	for _, prefix := range reservedGeneratorPrefixes {
		if strings.HasPrefix(g.Name, prefix) {
			return false, "generator name '" + g.Name + "' uses the reserved prefix '" + prefix + "'"
		}
	}

	if strings.TrimSpace(g.Cmd) == "" {
		return false, "generator '" + g.Name + "': empty cmd"
	}

	// install runs no per-contract loop, so it gets no variables — a reference
	// there would have no value to resolve to.
	if strings.Contains(g.Install, "{{") {
		return false, "generator '" + g.Name + "': install takes no template variables"
	}

	for _, c := range g.commands() {
		if _, err := generatorCommandVars(c.text); err != nil {
			return false, "generator '" + g.Name + "': " + c.field + ": " + err.Error()
		}
	}

	return true, ""
}

// commands returns the non-empty templated commands in a fixed order. Fixed,
// not map iteration: a config error message that names a different field on
// every run is a message nobody can act on.
func (g CustomGenerator) commands() []generatorCommand {
	var out []generatorCommand

	for _, c := range []generatorCommand{{field: "cmd", text: g.Cmd}, {field: "lint", text: g.Lint}} {
		if c.text != "" {
			out = append(out, c)
		}
	}

	return out
}

// IsValid validates a single contract entry. baseConfigDir resolves local
// paths for the existence check, exactly as for rest[].path.
func (c Contract) IsValid(baseConfigDir string) (bool, string) {
	if c.Path == "" {
		return false, errMsgEmptyContractPath
	}

	if ok, msg := validateSpecPath(baseConfigDir, c.Path); !ok {
		return false, msg
	}

	if ok, msg := validateProjectRelPath("copy", c.Copy); !ok {
		return false, "contract '" + c.Path + "': " + msg
	}

	if c.Target != "" {
		if ok, msg := validateProjectRelPath("target", c.Target); !ok {
			return false, "contract '" + c.Path + "': " + msg
		}
	}

	if c.GeneratorName == "" {
		return false, "contract '" + c.Path + "': generator_name is required " +
			"(an input nothing is generated from is not a contract)"
	}

	return true, ""
}

// ValidateGenerators validates the generator and contract sections together:
// each entry on its own, plus the cross-references between them.
func ValidateGenerators(baseConfigDir string, generators CustomGeneratorList, contracts ContractList) (bool, string) {
	byName := make(map[string]CustomGenerator, len(generators))

	for _, g := range generators {
		if ok, msg := g.IsValid(); !ok {
			return false, msg
		}

		if _, dup := byName[g.Name]; dup {
			return false, "duplicate generator name: " + g.Name
		}

		byName[g.Name] = g
	}

	seenCopy := make(map[string]string, len(contracts))

	for _, c := range contracts {
		if ok, msg := c.IsValid(baseConfigDir); !ok {
			return false, msg
		}

		gen, exists := byName[c.GeneratorName]
		if !exists {
			return false, "contract '" + c.Path + "': unknown generator_name: " + c.GeneratorName
		}

		// Two contracts writing the same file would make the surviving
		// content depend on the order of the section.
		if other, dup := seenCopy[c.Copy]; dup {
			return false, "duplicate contract copy path '" + c.Copy + "': used by " + other + " and " + c.Path
		}

		seenCopy[c.Copy] = c.Path

		if ok, msg := validateContractVars(gen, c); !ok {
			return false, msg
		}
	}

	return true, ""
}

// validateContractVars checks that every variable the generator's commands
// read has a value in this contract. A command referencing .Target against an
// entry without `target` is an error rather than an empty string: silently
// rendering `--out ` would hand the generator a broken argument list.
func validateContractVars(gen CustomGenerator, c Contract) (bool, string) {
	if c.Target != "" {
		return true, ""
	}

	for _, cmd := range gen.commands() {
		vars, err := generatorCommandVars(cmd.text)
		if err != nil {
			// Already reported by CustomGenerator.IsValid.
			return false, "generator '" + gen.Name + "': " + cmd.field + ": " + err.Error()
		}

		if vars[GeneratorVarTarget] {
			return false, "contract '" + c.Path + "': generator '" + gen.Name + "' references ." +
				GeneratorVarTarget + " in " + cmd.field + ", so the contract must set 'target'"
		}
	}

	return true, ""
}

// RenderGeneratorCommand renders one generator command for one contract.
// Callers validate first (ValidateGenerators), so a failure here means the
// template was never checked.
func RenderGeneratorCommand(cmd, spec, target string) (string, error) {
	tmpl, err := template.New(generatorCommandTmplName).Parse(cmd)
	if err != nil {
		return "", fmt.Errorf("parse generator command %q: %w", cmd, err)
	}

	var out strings.Builder

	data := struct {
		Spec   string
		Target string
	}{Spec: spec, Target: target}

	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render generator command %q: %w", cmd, err)
	}

	rendered := strings.TrimSpace(out.String())
	if rendered == "" {
		return "", fmt.Errorf(errFmtGotValue, ErrGeneratorCommandEmpty, cmd)
	}

	// A command is one recipe line; an embedded newline would turn the rest
	// into a separate, unindented make line.
	if strings.ContainsAny(rendered, "\n\r") {
		return "", fmt.Errorf(errFmtGotValue, ErrGeneratorCommandMultiline, cmd)
	}

	return rendered, nil
}

// generatorCommandVars parses a command template and returns the set of
// allowed variables it references. An unknown field is rejected here, at
// config load, instead of surfacing as a template execution error in the
// middle of a generation run.
func generatorCommandVars(cmd string) (map[string]bool, error) {
	tmpl, err := template.New(generatorCommandTmplName).Parse(cmd)
	if err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}

	used := make(map[string]bool)

	if tmpl.Tree == nil || tmpl.Tree.Root == nil {
		return used, nil
	}

	if err := collectTemplateFields(tmpl.Tree.Root, used); err != nil {
		return nil, err
	}

	return used, nil
}

// collectTemplateFields walks a parsed template and records every ".Field"
// reference, rejecting the ones that are not supported variables.
func collectTemplateFields(node parse.Node, used map[string]bool) error {
	switch n := node.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		if n == nil {
			return nil
		}

		for _, child := range n.Nodes {
			if err := collectTemplateFields(child, used); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return collectTemplateFields(n.Pipe, used)
	case *parse.IfNode:
		return collectBranchFields(n.Pipe, n.List, n.ElseList, used)
	case *parse.RangeNode:
		return collectBranchFields(n.Pipe, n.List, n.ElseList, used)
	case *parse.WithNode:
		return collectBranchFields(n.Pipe, n.List, n.ElseList, used)
	case *parse.PipeNode:
		if n == nil {
			return nil
		}

		for _, cmd := range n.Cmds {
			if err := collectTemplateFields(cmd, used); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			if err := collectTemplateFields(arg, used); err != nil {
				return err
			}
		}
	case *parse.FieldNode:
		return recordTemplateField(n.Ident, used)
	case *parse.ChainNode:
		if err := collectTemplateFields(n.Node, used); err != nil {
			return err
		}

		return recordTemplateField(n.Field, used)
	}

	return nil
}

func collectBranchFields(pipe *parse.PipeNode, list, elseList *parse.ListNode, used map[string]bool) error {
	for _, child := range []parse.Node{pipe, list, elseList} {
		if err := collectTemplateFields(child, used); err != nil {
			return err
		}
	}

	return nil
}

func recordTemplateField(ident []string, used map[string]bool) error {
	if len(ident) == 0 {
		return nil
	}

	name := ident[0]

	switch name {
	case GeneratorVarSpec, GeneratorVarTarget:
	default:
		return fmt.Errorf("%w: .%s (supported: .%s, .%s)",
			ErrGeneratorVarUnknown, name, GeneratorVarSpec, GeneratorVarTarget)
	}

	// Both variables are plain strings; ".Spec.Anything" cannot resolve.
	if len(ident) > 1 {
		return fmt.Errorf("%w: .%s is a string and has no field .%s", ErrGeneratorVarUnknown, name, ident[1])
	}

	used[name] = true

	return nil
}

// validateProjectRelPath checks that p is usable as a path inside the
// generated project: non-empty, relative, and not escaping the project root.
func validateProjectRelPath(field, p string) (bool, string) {
	if p == "" {
		return false, field + " is required"
	}

	if strings.ContainsAny(p, "\\") {
		return false, field + " must use '/' as separator, got: " + p
	}

	if path.IsAbs(p) {
		return false, field + " must be relative to the project root, got: " + p
	}

	cleaned := path.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false, field + " must not escape the project root, got: " + p
	}

	if cleaned != p {
		return false, field + " must be a clean path (" + cleaned + "), got: " + p
	}

	return true, ""
}
