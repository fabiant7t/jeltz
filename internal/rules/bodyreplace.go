package rules

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/fabiant7t/jeltz/internal/config"
)

const (
	BodyReplaceSearchModeRegex   = "regex"
	BodyReplaceSearchModeLiteral = "literal"
)

// BodyReplaceRule is a compiled response body search/replace rule.
type BodyReplaceRule struct {
	Match           *Match
	ContentTypeExpr *regexp.Regexp // nil = any content type
	SearchMode      string
	SearchRegex     *regexp.Regexp // only set for regex mode
	SearchLiteral   string         // only set for literal mode
	Replace         string
}

// CompileBodyReplaceRule compiles a RawRule of type body_replace.
func CompileBodyReplaceRule(raw config.RawRule) (*BodyReplaceRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}

	if raw.Search == "" {
		return nil, fmt.Errorf("body_replace rule requires search")
	}

	mode := raw.SearchMode
	if mode == "" {
		mode = BodyReplaceSearchModeRegex
	}
	if mode != BodyReplaceSearchModeRegex && mode != BodyReplaceSearchModeLiteral {
		return nil, fmt.Errorf("search_mode must be regex or literal, got %q", mode)
	}

	var ctExpr *regexp.Regexp
	if raw.ContentType != "" {
		ctExpr, err = regexp.Compile(raw.ContentType)
		if err != nil {
			return nil, fmt.Errorf("content_type regex %q: %w", raw.ContentType, err)
		}
	}

	r := &BodyReplaceRule{
		Match:           m,
		ContentTypeExpr: ctExpr,
		SearchMode:      mode,
		Replace:         raw.Replace,
	}
	if mode == BodyReplaceSearchModeRegex {
		re, reErr := regexp.Compile(raw.Search)
		if reErr != nil {
			return nil, fmt.Errorf("search regex %q: %w", raw.Search, reErr)
		}
		r.SearchRegex = re
	} else {
		r.SearchLiteral = raw.Search
	}

	return r, nil
}

// Matches reports whether this rule applies to the current flow and content type.
func (r *BodyReplaceRule) Matches(fm FlowMeta, contentType string) bool {
	if !r.Match.Matches(fm) {
		return false
	}
	if r.ContentTypeExpr == nil {
		return true
	}
	return r.ContentTypeExpr.MatchString(contentType)
}

// Apply returns the body after replace-all search/replace.
func (r *BodyReplaceRule) Apply(body []byte) []byte {
	switch r.SearchMode {
	case BodyReplaceSearchModeLiteral:
		return bytes.ReplaceAll(body, []byte(r.SearchLiteral), []byte(r.Replace))
	default:
		return r.SearchRegex.ReplaceAll(body, []byte(r.Replace))
	}
}
