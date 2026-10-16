package specsource

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// SourceRefs is the set of git ref overrides that entered the process from
// outside project.yaml (the --sourceRef flag / GPS_SOURCE_REF env). Contracts
// of one project come from more than one repository, so the knob is per
// repository, with an optional blanket entry:
//
//	<ref>                   blanket — every non-pinned source, any repository
//	<repo-selector>:<ref>   only the sources whose repository matches
//
// A ref written in the URI ("…/specs.git@v1.2.0#api.yaml") is a pin and is
// never overridden by either form.
//
// The zero value is not usable; build one with ParseSourceRefs. A nil
// *SourceRefs means "no override at all" and every method tolerates it, so a
// caller without the knob passes nil.
type SourceRefs struct {
	// blanket is the ref of the entry written without a selector ("" = none).
	blanket string
	// scoped is ordered by descending selector length, so the most specific
	// selector matching a repository wins.
	scoped []scopedRef

	mu sync.Mutex
	// matched records the selectors that named a repository this run actually
	// resolved; seenRepos records the repositories themselves. A selector that
	// matched nothing is a typo — see CheckUsed.
	matched   map[string]bool
	seenRepos map[string]bool
}

// scopedRef is one "<repo-selector>:<ref>" entry.
type scopedRef struct {
	// selector is the repository as the user wrote it (error messages).
	selector string
	// canon is selector in canonicalRepo form (matching).
	canon string
	ref   string
}

// ParseSourceRefs builds the override set from raw --sourceRef / GPS_SOURCE_REF
// values. Every entry may itself carry several overrides separated by commas or
// whitespace, so a single `SOURCE_REF="a:x b:y"` and a repeated flag are the
// same thing. Entries are:
//
//	release/2026.10                          blanket ref (at most one)
//	github.com/org/specs:release/2026.10     ref for one repository
//
// The selector is split off at the LAST colon: a git ref cannot contain ":"
// (ValidateRef rejects it), so a selector may be written in any form —
// "specs", "org/specs", "github.com/org/specs",
// "git+ssh://git@github.com/org/specs.git" — all of them identify the same
// repository. With no entries the returned set is empty (IsEmpty), and every
// source keeps the ref written in the config.
func ParseSourceRefs(entries []string) (*SourceRefs, error) {
	out := &SourceRefs{
		matched:   make(map[string]bool),
		seenRepos: make(map[string]bool),
	}

	blanketFrom := ""
	byCanon := make(map[string]string)

	for _, raw := range entries {
		for _, entry := range splitRefEntries(raw) {
			selector, ref, scoped := splitSelectorRef(entry)

			if ref == "" {
				return nil, fmt.Errorf("%w: %q has no ref after the colon", ErrInvalidSourceRef, entry)
			}

			if err := ValidateRef(ref); err != nil {
				return nil, fmt.Errorf("%w (entry %q)", err, entry)
			}

			if !scoped {
				if blanketFrom != "" {
					return nil, fmt.Errorf("%w: %q and %q both set the ref for every repository; "+
						"scope one of them to a repository (<repo>:<ref>)",
						ErrInvalidSourceRef, blanketFrom, entry)
				}

				blanketFrom = entry
				out.blanket = ref

				continue
			}

			canon := canonicalRepo(selector)
			if canon == "" {
				return nil, fmt.Errorf("%w: %q has an empty repository selector", ErrInvalidSourceRef, entry)
			}

			if prev, dup := byCanon[canon]; dup {
				return nil, fmt.Errorf("%w: %q and %q name the same repository",
					ErrInvalidSourceRef, prev, entry)
			}

			byCanon[canon] = entry

			out.scoped = append(out.scoped, scopedRef{selector: selector, canon: canon, ref: ref})
		}
	}

	// Longest selector first: "github.com/org/specs" must win over "specs"
	// when both are given. Stable, so entries of equal length keep input order.
	sort.SliceStable(out.scoped, func(i, j int) bool {
		return len(out.scoped[i].canon) > len(out.scoped[j].canon)
	})

	return out, nil
}

// IsEmpty reports whether there is no override at all.
func (o *SourceRefs) IsEmpty() bool {
	return o == nil || (o.blanket == "" && len(o.scoped) == 0)
}

// String renders the set for the "which refs is this build using" log line.
func (o *SourceRefs) String() string {
	if o.IsEmpty() {
		return ""
	}

	parts := make([]string, 0, len(o.scoped)+1)
	for _, s := range o.scoped {
		parts = append(parts, s.selector+" -> "+s.ref)
	}

	if o.blanket != "" {
		parts = append(parts, "every other repository -> "+o.blanket)
	}

	return strings.Join(parts, ", ")
}

// CheckUsed reports repository-scoped selectors that never matched a resolved
// source. Such a selector is almost always a typo, and staying silent about it
// is exactly the failure this knob exists to prevent: the build would quietly
// take that repository from its default branch instead of the named ref. The
// blanket entry is not checked — it is meant to be set for whole CI pipelines,
// projects without git sources included. Call after every source has been
// resolved.
func (o *SourceRefs) CheckUsed() error {
	if o == nil {
		return nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	unused := make([]string, 0, len(o.scoped))

	for _, s := range o.scoped {
		if !o.matched[s.canon] {
			unused = append(unused, fmt.Sprintf("%q", s.selector))
		}
	}

	if len(unused) == 0 {
		return nil
	}

	repos := make([]string, 0, len(o.seenRepos))
	for repo := range o.seenRepos {
		repos = append(repos, repo)
	}

	sort.Strings(repos)

	known := "none — this project has no git spec sources"
	if len(repos) > 0 {
		known = strings.Join(repos, ", ")
	}

	return fmt.Errorf("%w: %s (repositories used by this project: %s)",
		ErrUnusedSourceRef, strings.Join(unused, ", "), known)
}

// apply returns src with its ref replaced by the override that applies to it.
// A pinned source is returned unchanged, yet still counts as a match for the
// selector that named its repository: the selector is not a typo, the pin
// simply wins over it.
func (o *SourceRefs) apply(src GitSource) GitSource {
	return src.applySourceRef(o.refFor(src.Repo))
}

// refFor returns the ref that overrides repo, or "" when none does.
func (o *SourceRefs) refFor(repo string) string {
	if o == nil {
		return ""
	}

	canon := canonicalRepo(repo)

	o.mu.Lock()
	o.seenRepos[canon] = true
	o.mu.Unlock()

	for _, s := range o.scoped {
		if !repoMatches(canon, s.canon) {
			continue
		}

		o.mu.Lock()
		o.matched[s.canon] = true
		o.mu.Unlock()

		return s.ref
	}

	return o.blanket
}

// repoMatches reports whether the canonical repo identity matches the
// canonical selector. A selector may name the repository in full
// ("github.com/org/specs") or by any trailing part of it ("org/specs",
// "specs"), which is what makes the flag usable by hand; the match is always
// on a path-segment boundary, so "specs" does not match "my-specs".
func repoMatches(repo, selector string) bool {
	return repo == selector || strings.HasSuffix(repo, "/"+selector)
}

// canonicalRepo reduces a repo URL (or a user-written selector) to
// "host/path/name" so that the many ways of spelling one repository compare
// equal: the scheme, any userinfo, a trailing ".git", a port and surrounding
// slashes are dropped and the rest is lowercased. SCP-style
// "git@host:org/repo.git" is understood here — it is not valid as a source URI,
// but it is the form people copy out of `git remote -v` into the flag.
// Filesystem paths (used by tests and local clones) fall through as-is.
func canonicalRepo(s string) string {
	s = strings.TrimSpace(s)

	// Scheme: "https://", "ssh://", "git+ssh://", "file://".
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+len("://"):]
	}

	host, repoPath, _ := strings.Cut(s, "/")

	// Userinfo ("git@", "oauth2:token@") is not part of the identity.
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}

	// "host:443" is a port and is dropped; "host:org" is the SCP-style
	// separator and belongs to the path.
	if i := strings.Index(host, ":"); i >= 0 {
		tail := host[i+1:]
		host = host[:i]

		if tail != "" && !isAllDigits(tail) {
			repoPath = strings.Trim(tail+"/"+repoPath, "/")
		}
	}

	repoPath = strings.TrimSuffix(strings.Trim(repoPath, "/"), ".git")

	return strings.ToLower(strings.Trim(host+"/"+repoPath, "/"))
}

// splitRefEntries splits one raw flag value / env var into entries. Commas and
// whitespace both separate: `SOURCE_REF="a:x b:y"` reads naturally in a
// Makefile, `GPS_SOURCE_REF=a:x,b:y` in a CI variable. Neither character can
// appear in a git ref (ValidateRef rejects both).
func splitRefEntries(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

// splitSelectorRef splits "<repo>:<ref>" at the last colon. A ref never
// contains a colon, so everything before the last one is the selector; an
// entry without a colon is a blanket ref and is reported as scoped == false
// (as opposed to ":<ref>", which is a scoped entry with an empty selector —
// a mistake worth reporting rather than silently reading as blanket).
func splitSelectorRef(entry string) (selector, ref string, scoped bool) {
	i := strings.LastIndex(entry, ":")
	if i < 0 {
		return "", entry, false
	}

	return entry[:i], entry[i+1:], true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
