package rules

import (
	"fmt"

	"github.com/fabiant7t/jeltz/internal/config"
)

// RuleType identifies the kind of rule.
type RuleType string

const (
	RuleTypeHeader      RuleType = "header"
	RuleTypeMap         RuleType = "map"
	RuleTypeMapLocal    RuleType = "map_local"
	RuleTypeMapRemote   RuleType = "map_remote"
	RuleTypeBodyReplace RuleType = "body_replace"
)

// HeaderRule is a compiled header rule.
type HeaderRule struct {
	Match    *Match
	Request  *Ops
	Response *Ops
}

// MappedRule is a map-stage rule variant in original config file order.
type MappedRule struct {
	Map      *MapRule
	MapLocal *MapLocalRule
}

// RuleSet holds all compiled rules in file order.
type RuleSet struct {
	Headers     []*HeaderRule
	Mapped      []*MappedRule
	Map         []*MapRule
	MapLocal    []*MapLocalRule
	MapRemote   []*MapRemoteRule
	BodyReplace []*BodyReplaceRule
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
		case string(RuleTypeMap):
			mr, err := CompileMapRule(raw)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (map): %w", i, err)
			}
			rs.Map = append(rs.Map, mr)
			rs.Mapped = append(rs.Mapped, &MappedRule{Map: mr})
		case string(RuleTypeMapLocal):
			ml, err := CompileMapLocalRule(raw, basePath)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (map_local path %q): %w", i, raw.Path, err)
			}
			rs.MapLocal = append(rs.MapLocal, ml)
			rs.Mapped = append(rs.Mapped, &MappedRule{MapLocal: ml})
		case string(RuleTypeMapRemote):
			mr, err := CompileMapRemoteRule(raw)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (map_remote): %w", i, err)
			}
			rs.MapRemote = append(rs.MapRemote, mr)
		case string(RuleTypeBodyReplace):
			br, err := CompileBodyReplaceRule(raw)
			if err != nil {
				return nil, fmt.Errorf("rules[%d] (body_replace): %w", i, err)
			}
			rs.BodyReplace = append(rs.BodyReplace, br)
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
