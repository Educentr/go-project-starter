package templater

import (
	"sort"
	"strings"
	"testing"

	"github.com/Educentr/go-project-starter/internal/pkg/ockeymap"
)

// TestOnlineConfKeyDeclarationsCoverEveryRead is the check that keeps the
// generator's own key declarations honest.
//
// Declaring the keys generated code reads is only worth anything if the
// declaration cannot fall behind the code. The previous answer — a hand-kept
// list of reserved path prefixes — looked complete and was not: subtrees the
// generated code genuinely reads (`worker/<name>`, `settings`, `bot`) were
// simply missing, and nothing would ever have said so. This test is what makes
// that impossible: a template that reads a key must carry a declaration file
// next to it, or this fails.
func TestOnlineConfKeyDeclarationsCoverEveryRead(t *testing.T) {
	reading, err := OnlineConfReadingTemplates()
	if err != nil {
		t.Fatalf("OnlineConfReadingTemplates() error: %v", err)
	}

	if len(reading) == 0 {
		t.Fatal("no template reads OnlineConf: the scan is broken, not the templates")
	}

	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	declared := make(map[string]*ockeymap.GenDecl, len(decls))
	for _, decl := range decls {
		declared[decl.Owner] = decl.Decl
	}

	for _, template := range reading {
		if _, ok := declared[template]; !ok {
			t.Errorf("%s reads an OnlineConf key but declares none: add %s%s next to it",
				template, template, ockeymap.GenDeclSuffix)
		}
	}

	// The other direction: a declaration whose template is gone declares a read
	// nobody performs, and would keep a path reserved against the service map
	// for no reason.
	for _, decl := range decls {
		if !contains(reading, decl.Owner) {
			t.Errorf("%s%s declares keys, but %s performs no OnlineConf read",
				decl.Owner, ockeymap.GenDeclSuffix, decl.Owner)
		}
	}
}

// TestOnlineConfDeclarationsCoverPathLiterals catches the subtler drift: a
// template that already has a declaration file grows a new read.
//
// It reads the path literals out of the template — the strings handed to
// MakePath and the path helpers around it — and requires each segment to appear
// in a path that template declares. A new key brings a new literal with it, so a
// read added without a declaration fails here even though the file exists.
//
// Segments assembled out of identifiers (a const, a parameter, a slice built
// above the call) are invisible to this and are covered by the declaration
// review instead; the check is deliberately one-sided, so that it can only fail
// on a literal nobody declared.
func TestOnlineConfDeclarationsCoverPathLiterals(t *testing.T) {
	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	for _, decl := range decls {
		source, rerr := ReadTemplateSource(decl.Owner)
		if rerr != nil {
			t.Fatalf("reading %s: %v", decl.Owner, rerr)
		}

		declaredSegments := declaredPathSegments(decl.Decl)

		for _, segment := range pathLiteralSegments(source) {
			if _, ok := declaredSegments[segment]; !ok {
				t.Errorf("%s builds an OnlineConf path out of %q, which no declared path in %s%s carries",
					decl.Owner, segment, decl.Owner, ockeymap.GenDeclSuffix)
			}
		}
	}
}

// TestOnlineConfDeclarationsParse pins that every shipped declaration is valid:
// a defect here would surface as a failed generation in a user's project.
func TestOnlineConfDeclarationsParse(t *testing.T) {
	decls, err := LoadOnlineConfKeyDecls()
	if err != nil {
		t.Fatalf("LoadOnlineConfKeyDecls() error: %v", err)
	}

	if len(decls) == 0 {
		t.Fatal("no declarations loaded: generated code does read OnlineConf")
	}

	for i := 1; i < len(decls); i++ {
		if decls[i-1].Owner >= decls[i].Owner {
			t.Fatalf("LoadOnlineConfKeyDecls() is not sorted by owner: %q before %q",
				decls[i-1].Owner, decls[i].Owner)
		}
	}
}

// TestOnlineConfDeclarationsAreNotCopiedIntoProjects pins that a declaration
// file stays generator input. It lives inside the templates tree, and the tree
// is walked wholesale — so without the skip in GetTemplates it would land in
// every generated project as a stray yaml file.
func TestOnlineConfDeclarationsAreNotCopiedIntoProjects(t *testing.T) {
	_, files, err := GetMainTemplates(GeneratorParams{})
	if err != nil {
		t.Fatalf("GetMainTemplates() error: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.SourceName, ockeymap.GenDeclSuffix) {
			t.Errorf("%s would be copied into the generated project as %s", file.SourceName, file.DestName)
		}
	}
}

// declaredPathSegments collects every literal segment of every path a
// declaration carries.
func declaredPathSegments(decl *ockeymap.GenDecl) map[string]struct{} {
	segments := make(map[string]struct{})

	for _, group := range decl.Groups {
		for _, key := range group.Keys {
			for _, segment := range strings.Split(strings.Trim(key.Path, "/"), "/") {
				segments[segment] = struct{}{}
			}
		}
	}

	return segments
}

// pathLiteralSegments returns the path segments a template spells out as string
// literals inside a path-building call.
//
// It scans the path-building calls (pathBuilders) and takes their quoted
// arguments. A literal carrying a template action ({{ .ProjectName }}) is
// skipped: its value is decided at generation time and is a placeholder in the
// declaration too.
func pathLiteralSegments(source string) []string {
	found := make(map[string]struct{})

	for _, offset := range pathCallOffsets(source) {
		for _, literal := range quotedArgs(source[offset:]) {
			if strings.Contains(literal, "{{") {
				continue
			}

			for _, segment := range strings.Split(strings.Trim(literal, "/"), "/") {
				if segment != "" {
					found[segment] = struct{}{}
				}
			}
		}
	}

	out := make([]string, 0, len(found))
	for segment := range found {
		out = append(out, segment)
	}

	sort.Strings(out)

	return out
}

// pathBuilders are the calls a template assembles an OnlineConf path with:
// go-onlineconf's own joiner plus the helpers the templates wrap it in. A
// template introducing another helper is still caught by the owner-level check
// above; this list only decides how much the literal check can see.
var pathBuilders = []string{
	"onlineconf.MakePath(",
	"GetConfigPath(",
	"GetTgTokenPath(",
}

// pathCallOffsets returns the offset of the opening parenthesis of every
// path-building call in source.
func pathCallOffsets(source string) []int {
	var offsets []int

	for _, builder := range pathBuilders {
		for searched := 0; ; {
			idx := strings.Index(source[searched:], builder)
			if idx < 0 {
				break
			}

			offsets = append(offsets, searched+idx+len(builder)-1)
			searched += idx + len(builder)
		}
	}

	sort.Ints(offsets)

	return offsets
}

// quotedArgs returns the double-quoted literals of the argument list starting at
// the opening parenthesis of source, stopping at its matching close so that a
// literal belonging to an enclosing call is not picked up.
func quotedArgs(source string) []string {
	var (
		args  []string
		depth int
	)

	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '(':
			depth++
		case ')':
			depth--

			if depth == 0 {
				return args
			}
		case '"':
			end := strings.IndexByte(source[i+1:], '"')
			if end < 0 {
				return args
			}

			args = append(args, source[i+1:i+1+end])
			i += end + 1
		}
	}

	return args
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}

	return false
}
