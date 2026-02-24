package rules

import (
	"fmt"

	"github.com/fabiant7t/jeltz/internal/config"
)

// RuleType identifies the kind of rule.
type RuleType string

const (
	RuleTypeHeader   RuleType = "header"
	RuleTypeMapLocal RuleType = "map_local"
)

// HeaderRule is a compiled header rule.
type HeaderRule struct {
	Match    *Match
	Request  *Ops
	Response *Ops
}

// RuleSet holds all compiled rules in file order.
type RuleSet struct {
	Headers  []*HeaderRule
	MapLocal []*MapLocalRule
}

// Compile compiles all raw rules from config. basePath is used to resolve
// relative filesystem paths in map_local rules.
func Compile(rawRules []config.RawRule, basePath string) (*RuleSet, error) {
	rs := &RuleSet{}
	for i, raw := range rawRules {
		switch raw.Type {
		case string(RuleTypeHeader):
			hr, err := compileHeaderRule(raw)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (header): %w", i, err)
			}
			rs.Headers = append(rs.Headers, hr)
		case string(RuleTypeMapLocal):
			ml, err := CompileMapLocalRule(raw, basePath)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (map_local): %w", i, err)
			}
			rs.MapLocal = append(rs.MapLocal, ml)
		default:
			return nil, fmt.Errorf("rules[%d]: unknown type %q", i, raw.Type)
		}
	}
	return rs, nil
}

func compileHeaderRule(raw config.RawRule) (*HeaderRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}
	reqOps, err := CompileOps(raw.Request)
	if err != nil {
		return nil, fmt.Errorf("request ops: %w", err)
	}
	respOps, err := CompileOps(raw.Response)
	if err != nil {
		return nil, fmt.Errorf("response ops: %w", err)
	}
	return &HeaderRule{Match: m, Request: reqOps, Response: respOps}, nil
}
