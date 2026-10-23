package ds

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// OnlineConfConfig carries a project's whole OnlineConf key map into templates.
//
// The map has two halves. The generator's half is the paths the generated code
// reads — transport timeouts, log levels, security toggles, worker and Kafka
// settings — declared next to the templates that read them. The service's half
// is `onlineconf.path` in project.yaml, which is optional. The generated
// accessor package, the validator, the startup check and the registry are built
// from both, so they exist in every project: a project without a key map of its
// own still reads OnlineConf, and the guard has to hold there too.
type OnlineConfConfig struct {
	// ServiceTree is the tree the generated code reads under — the service name.
	ServiceTree string
	// Accessors are every function the generated package gets, sorted by name:
	// the service's declarations and the generator's, in one list, because the
	// generated code and the service code read through the same package.
	Accessors []OnlineConfAccessor
	// Surface is every path of the service's OnlineConf tree, sorted by path.
	// It is what the generated registry is built from.
	Surface []OnlineConfSurfaceEntry
	// ServiceRequired are the startup checks of the service's own declarations.
	// They hold for every application: the service map has no application
	// dimension.
	ServiceRequired []OnlineConfCheck
	// AppRequired are the startup checks of the generator's declarations, per
	// application. They are per application on purpose — a REST-only process
	// must not refuse to start over the telegram token of a worker it does not
	// run.
	AppRequired []OnlineConfAppChecks
}

// OnlineConfAccessor is one generated function: a typed read of one declared
// key, or a builder of a path the generated code reads below.
type OnlineConfAccessor struct {
	// Name is the Go identifier.
	Name string
	// Origin is OnlineConfOriginService or OnlineConfOriginGenerated.
	Origin string
	// Type is the declared value type, empty for a path builder.
	Type string
	// Kind is the accessor shape family (scalar, strings, struct).
	Kind string
	// Shape is what the accessor returns: required, default or optional.
	Shape string
	// Required reports a key the startup check verifies. For a generator key it
	// does not change Shape — the read still answers with presence, because a
	// component declared optional in project.yaml is skipped rather than
	// reported — it only says the startup check covers it.
	Required bool
	// GoType is the Go type of the value.
	GoType string
	// ZeroLiteral is the value returned alongside an error.
	ZeroLiteral string
	// Getter is the go-onlineconf reader the body calls.
	Getter string
	// PathConst is the exported path constant of a service key, empty for a
	// generator key — whose path depends on arguments and so cannot be one.
	PathConst string
	// PathBuilder is the name of the generated function returning the path,
	// empty for a service key.
	PathBuilder string
	// PathArgs are the arguments of the onlineconf.MakePath call in the path
	// builder, empty for a service key.
	PathArgs string
	// ConstPath is the whole path as a Go string literal, set only when the
	// accessor takes no arguments — then the path is known at generation time and
	// the builder returns a constant instead of composing it on every read.
	ConstPath string
	// PathExpr is the Go expression the accessor reads at: the path constant, or
	// a call to the path builder.
	PathExpr string
	// ParamDecl is the accessor's parameter list ("transport, app string"),
	// empty when it takes none.
	ParamDecl string
	// ParamNames is the same parameters as an argument list.
	ParamNames string
	// DocPath is the declared path as it reads in the doc comment, variable
	// segments intact.
	DocPath string
	// Description says what the key does.
	Description string
	// DefaultLiteral is the Go expression of a service key's declared default.
	DefaultLiteral string
	// DefaultText is the default as written, for the doc comment.
	DefaultText string
	// ReadBy lists the templates that read a generator key, sorted.
	ReadBy []string
	// ReadByText is ReadBy as one line, for the doc comment.
	ReadByText string
	// IsPathBuilder reports an accessor that returns a path instead of reading.
	IsPathBuilder bool
}

// OnlineConfCheck is one entry of the startup check: the accessor to call and
// the arguments to call it with.
type OnlineConfCheck struct {
	// Accessor is the Go identifier of the accessor.
	Accessor string
	// Args are the arguments, already rendered as Go string literals, empty for
	// an accessor that takes none.
	Args string
	// Path is the path the call reads, for review.
	Path string
}

// OnlineConfAppChecks are the startup checks of one application.
type OnlineConfAppChecks struct {
	// App is the application name, as the generated main passes it.
	App string
	// Checks are the required keys that application's process reads.
	Checks []OnlineConfCheck
}

// OnlineConfSurfaceEntry is one row of the generated OnlineConf registry: one
// path of the service's tree, whoever reads it.
type OnlineConfSurfaceEntry struct {
	// Path is the full OnlineConf path. A segment the generated code computes at
	// runtime keeps its <name> spelling: that is the honest rendering of a path
	// that only exists once the process runs.
	Path string
	// Origin is OnlineConfOriginService or OnlineConfOriginGenerated.
	Origin string
	// Type is the value type, empty for a path the generated code reads below.
	Type string
	// Required marks a key the startup check verifies.
	Required bool
	// Accessor is the generated accessor the path is read through.
	Accessor string
	// ReadVia names the shared accessor a generated read goes through, empty
	// otherwise.
	ReadVia string
	// Owns is the path from which the subtree belongs to the generated code,
	// empty when the path claims no subtree.
	Owns string
	// HasDefault reports that a fallback is applied when the key is absent.
	// Separate from Default because a declared default can be the empty string
	// or the empty list, and "no row in the registry" would then read as "no
	// default", which is a different accessor shape.
	HasDefault bool
	// Default is the fallback applied when the key is absent, as prose.
	Default string
	// ReadBy lists the templates performing a generated read.
	ReadBy []string
	// Description says what the key does.
	Description string
}

const (
	// OnlineConfPackage is the Go package name of the generated accessor
	// package. Deliberately not "onlineconf": the generated package wraps the
	// library of that name, and the two must be distinguishable at a call
	// site.
	OnlineConfPackage = "occonf"

	// onlineConfTreeConst is the identifier of the generated constant holding
	// the service tree root. The path builders are rendered with it rather than
	// with the root itself, so the tree name appears once in the package.
	onlineConfTreeConst = "serviceTree"

	// onlineConfValidatorMsg is the common tail of every validator message.
	// Kept in one place so the two generated lint configs cannot drift.
	onlineConfValidatorMsg = "the generated accessor package is the only place OnlineConf is read — " +
		"call the accessor of the key, and declare the key in the OnlineConf key map when it has none"
)

// Origins of a row of the generated OnlineConf registry.
const (
	// OnlineConfOriginService marks a key declared by the service's key map.
	OnlineConfOriginService = "service"
	// OnlineConfOriginGenerated marks a key the generated code reads.
	OnlineConfOriginGenerated = "generated"
)

// namesPerAccessor is how many identifiers one accessor can claim — its own
// name and, for a path builder, the path constant. Used only to pre-size the
// collision map.
const namesPerAccessor = 2

// onlineConfDir is the project-relative directory of the generated accessor
// package. It sits under internal/pkg/ because it is generated core logic with
// no config dependency, following the repository's three-layer split.
var onlineConfDir = filepath.Join("internal", "pkg", OnlineConfPackage)

// ErrOnlineConfNameTaken reports two declarations that would generate one
// function in the accessor package.
var ErrOnlineConfNameTaken = errors.New("onlineconf accessor name is already taken")

// onlineConfReserved are the names the accessor package carries itself: the
// library's lifecycle, the subscription helper, the startup check and its
// sentinel error. A declaration cannot have one of them.
var onlineConfReserved = map[string]struct{}{
	"Initialize": {}, "StartWatcher": {}, "StopWatcher": {}, "Instance": {},
	"Subscribe": {}, "CheckRequired": {}, "Paths": {}, "ErrKeyNotSet": {},
}

// NewOnlineConfConfig assembles a project's whole OnlineConf key map: the
// service's declarations (nil when project.yaml configures none) and the
// generator's, expanded against this project and against each of its
// applications.
//
// A default that does not parse as its declared type fails here rather than
// becoming a value the service silently never gets.
func NewOnlineConfConfig(
	serviceTree string,
	serviceKeys []ockeymap.Key,
	generated *ockeymap.GenSurface,
	perApp map[string]*ockeymap.GenSurface,
) (OnlineConfConfig, error) {
	out := OnlineConfConfig{ServiceTree: serviceTree}

	for _, key := range serviceKeys {
		accessor, err := serviceAccessor(key)
		if err != nil {
			return OnlineConfConfig{}, err
		}

		out.Accessors = append(out.Accessors, accessor)

		if key.Required {
			out.ServiceRequired = append(out.ServiceRequired, OnlineConfCheck{
				Accessor: key.VarName(), Path: key.Path,
			})
		}
	}

	if generated != nil {
		for _, accessor := range generated.Accessors {
			out.Accessors = append(out.Accessors, generatedAccessor(accessor, serviceTree))
		}
	}

	sort.Slice(out.Accessors, func(i, j int) bool { return out.Accessors[i].Name < out.Accessors[j].Name })

	if err := checkOnlineConfNames(out.Accessors); err != nil {
		return OnlineConfConfig{}, err
	}

	out.Surface = onlineConfSurfaceRows(serviceKeys, generated)
	out.AppRequired = appChecks(perApp)

	return out, nil
}

// checkOnlineConfNames rejects two declarations whose generated functions would
// collide.
//
// Each half of the map validates its own names, and the paths of the two halves
// are checked against each other — but the identifiers are not, and the two
// halves land in ONE Go package. A service key whose derived name happens to be
// the generator's (`/svc/dev/stand` → DevStand, next to the generator's
// `/svc/devstand`) used to pass every check and produce a package that does not
// compile, in the user's project rather than here. The package's own exported
// names are reserved for the same reason: they only became collidable when the
// library's lifecycle moved into the generated package.
func checkOnlineConfNames(accessors []OnlineConfAccessor) error {
	taken := make(map[string]string, len(accessors)*namesPerAccessor)

	var problems []string

	for _, accessor := range accessors {
		// A path builder IS its accessor, so its two names are one name.
		names := []string{accessor.Name}

		if accessor.PathBuilder != "" && accessor.PathBuilder != accessor.Name {
			names = append(names, accessor.PathBuilder)
		}

		if accessor.PathConst != "" {
			names = append(names, accessor.PathConst)
		}

		for _, name := range names {
			if _, reserved := onlineConfReserved[name]; reserved {
				problems = append(problems, fmt.Sprintf(
					"%s (%s) is named %s, which the accessor package already carries",
					accessor.DocPath, accessor.Origin, name))

				continue
			}

			if owner, dup := taken[name]; dup {
				problems = append(problems, fmt.Sprintf(
					"%s (%s) and %s both generate %s", accessor.DocPath, accessor.Origin, owner, name))

				continue
			}

			taken[name] = accessor.DocPath
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)

		return fmt.Errorf("%w:\n  %s", ErrOnlineConfNameTaken, strings.Join(problems, "\n  "))
	}

	return nil
}

// serviceAccessor turns one service declaration into a generated function.
func serviceAccessor(key ockeymap.Key) (OnlineConfAccessor, error) {
	defaultLiteral, err := key.DefaultLiteral()
	if err != nil {
		return OnlineConfAccessor{}, fmt.Errorf("onlineconf key %s: %w", key.Path, err)
	}

	return OnlineConfAccessor{
		Name:           key.VarName(),
		Origin:         OnlineConfOriginService,
		Type:           key.Type,
		Kind:           key.Kind(),
		Shape:          key.Shape(),
		Required:       key.Required,
		GoType:         key.GoType(),
		ZeroLiteral:    key.ZeroLiteral(),
		Getter:         key.Getter(),
		PathConst:      key.PathConstName(),
		PathExpr:       key.PathConstName(),
		DocPath:        key.Path,
		Description:    key.Description,
		DefaultLiteral: defaultLiteral,
		DefaultText:    key.DefaultText(),
	}, nil
}

// generatedAccessor turns one of the generator's declarations into a generated
// function.
func generatedAccessor(accessor ockeymap.GenAccessor, serviceTree string) OnlineConfAccessor {
	out := OnlineConfAccessor{
		Name:          accessor.Name,
		Origin:        OnlineConfOriginGenerated,
		Type:          accessor.Type,
		Kind:          accessor.Kind(),
		Shape:         accessor.Shape(),
		Required:      accessor.Required,
		GoType:        accessor.GoType(),
		ZeroLiteral:   accessor.ZeroLiteral(),
		Getter:        accessor.Getter(),
		PathBuilder:   accessor.PathBuilderName(),
		PathArgs:      accessor.PathArgs(onlineConfTreeConst),
		ConstPath:     accessor.ConstPath(serviceTree),
		ParamDecl:     accessor.ParamDecl(),
		ParamNames:    accessor.ParamNames(),
		DocPath:       accessor.DocPath(serviceTree),
		Description:   accessor.Description(),
		DefaultText:   accessor.Default,
		ReadBy:        accessor.ReadBy,
		ReadByText:    strings.Join(accessor.ReadBy, ", "),
		IsPathBuilder: accessor.IsPathBuilder(),
	}

	out.PathExpr = out.PathBuilder + "(" + out.ParamNames + ")"

	return out
}

// onlineConfSurfaceRows returns every path of the service's OnlineConf tree —
// the service's own declarations and the paths the generated code reads —
// sorted by path.
//
// This is what the generated registry is built from. Half of the tree used to
// exist only inside the generator's templates, so the owner of the contract
// could not see it at all; one list of both is the answer to that.
func onlineConfSurfaceRows(
	serviceKeys []ockeymap.Key, generated *ockeymap.GenSurface,
) []OnlineConfSurfaceEntry {
	generatedKeys := 0
	if generated != nil {
		generatedKeys = len(generated.Keys)
	}

	out := make([]OnlineConfSurfaceEntry, 0, len(serviceKeys)+generatedKeys)

	for _, key := range serviceKeys {
		out = append(out, OnlineConfSurfaceEntry{
			Path:        key.Path,
			Origin:      OnlineConfOriginService,
			Type:        key.Type,
			Required:    key.Required,
			Accessor:    key.VarName(),
			HasDefault:  key.HasDefault(),
			Default:     key.DefaultText(),
			Description: key.Description,
		})
	}

	if generated != nil {
		for _, key := range generated.Keys {
			out = append(out, OnlineConfSurfaceEntry{
				Path:        key.Path,
				Origin:      OnlineConfOriginGenerated,
				Type:        key.Type,
				Required:    key.Required,
				Accessor:    key.Accessor,
				ReadVia:     key.ReadVia,
				Owns:        key.OwnedPrefix,
				HasDefault:  key.Default != "",
				Default:     key.Default,
				ReadBy:      key.ReadBy,
				Description: key.Description(),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out
}

// appChecks turns the per-application expansions into the startup checks of
// each application, sorted by application name.
func appChecks(perApp map[string]*ockeymap.GenSurface) []OnlineConfAppChecks {
	out := make([]OnlineConfAppChecks, 0, len(perApp))

	for app, surface := range perApp {
		if surface == nil || len(surface.Required) == 0 {
			continue
		}

		checks := make([]OnlineConfCheck, 0, len(surface.Required))
		for _, check := range surface.Required {
			checks = append(checks, OnlineConfCheck{
				Accessor: check.Accessor,
				Args:     check.QuotedArgs(),
				Path:     check.Path,
			})
		}

		out = append(out, OnlineConfAppChecks{App: app, Checks: checks})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].App < out[j].App })

	return out
}

// HasServiceKeys reports whether the service declared a key map with anything in
// it. The generated package exists either way.
func (o OnlineConfConfig) HasServiceKeys() bool {
	for _, accessor := range o.Accessors {
		if accessor.Origin == OnlineConfOriginService {
			return true
		}
	}

	return false
}

// ServicePathConsts returns the service keys' exported path constants, so that a
// caller can hand a declared path to Subscribe without reaching into the
// onlineconf package — which the validator forbids outside the generated one.
func (o OnlineConfConfig) ServicePathConsts() []OnlineConfAccessor {
	out := make([]OnlineConfAccessor, 0, len(o.Accessors))

	for _, accessor := range o.Accessors {
		if accessor.PathConst != "" {
			out = append(out, accessor)
		}
	}

	return out
}

// HasRequired reports whether anything is checked at startup.
func (o OnlineConfConfig) HasRequired() bool {
	return len(o.ServiceRequired) > 0 || len(o.AppRequired) > 0
}

// NeedsTimeImport reports whether the generated package needs "time" — that is,
// whether any accessor yields a duration.
func (o OnlineConfConfig) NeedsTimeImport() bool {
	for _, accessor := range o.Accessors {
		if accessor.Type == ockeymap.TypeDuration {
			return true
		}
	}

	return false
}

// NeedsStringsImport reports whether the generated package needs "strings". Only
// the startup check does, to join the problems it collects.
func (o OnlineConfConfig) NeedsStringsImport() bool {
	return o.HasRequired()
}

// NeedsErrorsImport reports whether the generated package needs "errors": only
// ErrKeyNotSet does, and only a required key can return it.
func (o OnlineConfConfig) NeedsErrorsImport() bool {
	return o.HasRequired()
}

// TreeRoot returns the service tree the generated code reads under.
func (o OnlineConfConfig) TreeRoot() string {
	return o.ServiceTree
}

// ForbiddenCalls returns the validator's rules: every call into the
// go-onlineconf package is forbidden outside the generated accessor package.
//
// A blanket rule rather than a list of readers, deliberately. The map exists
// because a path assembled at the call site is invisible to every external
// tool; banning only onlineconf.Get* would leave onlineconf.MakePath free to
// build a path for some other reader, and banning only MakePath would leave a
// bare string literal path free. One rule over the package closes both.
func (o OnlineConfConfig) ForbiddenCalls() []ForbidRule {
	return []ForbidRule{
		{
			Pattern: `^onlineconf\.`,
			Msg:     "reading OnlineConf directly is not allowed here: " + onlineConfValidatorMsg,
		},
	}
}

// ExemptPathPattern returns the regexp of the paths the validator does not
// check, as a forbidigo `path:` value.
//
// The generated accessor package is the only exemption, because it is where
// OnlineConf IS read. Generated files used to be exempt as a class, which left a
// hole: the manual section below the disclaimer marker of a psg_*_gen.go file
// could read any key and nothing would notice. The generated code reads through
// the accessors now, so the exemption is not needed and the hole is closed.
func (o OnlineConfConfig) ExemptPathPattern() string {
	return o.GetPackagePath() + "/"
}

// GetTargetGeneratePath returns the directory of the generated accessor
// package.
func (o OnlineConfConfig) GetTargetGeneratePath(targetDir string) string {
	return filepath.Join(targetDir, onlineConfDir)
}

// GetPackagePath returns the project-relative directory of the generated
// accessor package, with forward slashes — suitable for import paths and for
// lint-config path patterns.
func (o OnlineConfConfig) GetPackagePath() string {
	return filepath.ToSlash(onlineConfDir)
}

// GetPackageName returns the Go package name of the generated accessor package.
func (o OnlineConfConfig) GetPackageName() string {
	return OnlineConfPackage
}
