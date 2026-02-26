package rules

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/fabiant7t/jeltz/internal/config"
)

// MapRule is a compiled inline map rule.
type MapRule struct {
	Match       *Match
	Body        []byte
	StatusCode  int
	ContentType string
	Response    *Ops
}

// MapResult is returned when a MapRule is matched.
type MapResult struct {
	Body        []byte
	StatusCode  int
	ContentType string
	Response    *Ops
}

// CompileMapRule compiles a RawRule of type map.
func CompileMapRule(raw config.RawRule) (*MapRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}

	hasBody := raw.Body != ""
	hasBodyBase64 := raw.BodyBase64 != ""
	if hasBody == hasBodyBase64 {
		return nil, fmt.Errorf("map rule requires exactly one of body or body_base64")
	}

	var body []byte
	if hasBody {
		body = []byte(raw.Body)
	} else {
		data, err := base64.StdEncoding.DecodeString(raw.BodyBase64)
		if err != nil {
			return nil, fmt.Errorf("invalid body_base64: %w", err)
		}
		body = data
	}

	statusCode := raw.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	respOps, err := CompileOps(raw.Response)
	if err != nil {
		return nil, fmt.Errorf("response ops: %w", err)
	}

	return &MapRule{
		Match:       m,
		Body:        body,
		StatusCode:  statusCode,
		ContentType: raw.ContentType,
		Response:    respOps,
	}, nil
}

// Resolve attempts to match fm against this rule.
// Returns (nil, nil) if the rule does not match.
func (r *MapRule) Resolve(fm FlowMeta) (*MapResult, error) {
	if !r.Match.Matches(fm) {
		return nil, nil
	}
	return &MapResult{
		Body:        append([]byte(nil), r.Body...),
		StatusCode:  r.StatusCode,
		ContentType: r.ContentType,
		Response:    r.Response,
	}, nil
}
