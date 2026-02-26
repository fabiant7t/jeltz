package rules

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/fabiant7t/jeltz/internal/config"
)

// MapRemoteRule is a compiled map-remote rule.
type MapRemoteRule struct {
	Match   *Match
	BaseURL *url.URL
}

// MapRemoteTarget is the resolved upstream destination for a matched rule.
type MapRemoteTarget struct {
	Scheme   string
	Host     string
	Port     string
	Path     string
	RawQuery string
}

// CompileMapRemoteRule compiles a RawRule of type map_remote.
func CompileMapRemoteRule(raw config.RawRule) (*MapRemoteRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}

	// Enforce path regex anchor from start for deterministic prefix stripping.
	if !strings.HasPrefix(raw.Match.Path, "^") {
		return nil, fmt.Errorf("map_remote match.path regex must start with '^' for correct prefix stripping, got %q", raw.Match.Path)
	}

	if raw.URL == "" {
		return nil, fmt.Errorf("map_remote rule requires url")
	}

	u, err := url.Parse(raw.URL)
	if err != nil {
		return nil, fmt.Errorf("map_remote url %q: %w", raw.URL, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("map_remote url %q must include scheme", raw.URL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("map_remote url %q must include host", raw.URL)
	}

	return &MapRemoteRule{
		Match:   m,
		BaseURL: u,
	}, nil
}

// Resolve attempts to match fm against this rule and returns destination
// upstream target metadata. Returns (nil, nil) if the rule does not match.
func (r *MapRemoteRule) Resolve(fm FlowMeta) (*MapRemoteTarget, error) {
	if !r.Match.Matches(fm) {
		return nil, nil
	}

	// Prefix stripping: find match extent from index 0.
	loc := r.Match.Path.FindStringIndex(fm.Path)
	if loc == nil || loc[0] != 0 {
		return nil, nil
	}
	stripped := fm.Path[loc[1]:]
	if stripped == "" {
		stripped = "/"
	}

	basePath := r.BaseURL.Path
	if basePath == "" {
		basePath = "/"
	}
	mappedPath := joinMappedPath(basePath, stripped)

	rawQuery := fm.RawQuery
	if r.BaseURL.RawQuery != "" {
		if rawQuery == "" {
			rawQuery = r.BaseURL.RawQuery
		} else {
			rawQuery = r.BaseURL.RawQuery + "&" + rawQuery
		}
	}

	return &MapRemoteTarget{
		Scheme:   r.BaseURL.Scheme,
		Host:     r.BaseURL.Hostname(),
		Port:     r.BaseURL.Port(),
		Path:     mappedPath,
		RawQuery: rawQuery,
	}, nil
}

func joinMappedPath(basePath, stripped string) string {
	base := strings.TrimSuffix(basePath, "/")
	suffix := strings.TrimPrefix(stripped, "/")
	if suffix == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	if base == "" {
		return "/" + suffix
	}
	return base + "/" + suffix
}
