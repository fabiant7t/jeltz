package rules

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"

	"github.com/fabiant7t/jeltz/internal/config"
)

// DeleteOp is a compiled header delete operation.
type DeleteOp struct {
	// Name-based: delete header by name (optionally filtered by ValueRegex).
	Name string
	// AnyName: wildcard delete across all header names by value regex.
	AnyName bool
	// ValueRegex is nil if no filtering on value.
	ValueRegex *regexp.Regexp
}

// SetOp is a compiled header set operation.
type SetOp struct {
	Name   string
	Mode   string // "replace" or "append"
	Value  string
}

// Ops is an ordered, compiled set of header operations.
type Ops struct {
	Delete []DeleteOp
	Set    []SetOp
}

// Apply executes all delete ops then all set ops on h.
func (ops *Ops) Apply(h http.Header) {
	for _, d := range ops.Delete {
		applyDelete(h, d)
	}
	for _, s := range ops.Set {
		applySet(h, s)
	}
}

func applyDelete(h http.Header, d DeleteOp) {
	if d.AnyName {
		// Wildcard: remove values matching regex across all header names.
		// Collect keys first to avoid mutating while ranging.
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic traversal
		for _, k := range keys {
			filterHeaderValues(h, k, d.ValueRegex)
		}
		return
	}
	// Name-based delete.
	canonical := http.CanonicalHeaderKey(d.Name)
	if d.ValueRegex == nil {
		h.Del(canonical)
		return
	}
	filterHeaderValues(h, canonical, d.ValueRegex)
}

// filterHeaderValues removes values matching re from header name k.
// If all values are removed, the header key is deleted.
func filterHeaderValues(h http.Header, k string, re *regexp.Regexp) {
	vals := h.Values(k)
	if len(vals) == 0 {
		return
	}
	var keep []string
	for _, v := range vals {
		if !re.MatchString(v) {
			keep = append(keep, v)
		}
	}
	h.Del(k)
	for _, v := range keep {
		h.Add(k, v)
	}
}

func applySet(h http.Header, s SetOp) {
	canonical := http.CanonicalHeaderKey(s.Name)
	switch s.Mode {
	case "replace":
		h.Set(canonical, s.Value)
	case "append":
		h.Add(canonical, s.Value)
	}
}

// CompileOps compiles a RawOps into Ops, validating all fields.
func CompileOps(raw *config.RawOps) (*Ops, error) {
	if raw == nil {
		return &Ops{}, nil
	}
	ops := &Ops{}
	for i, rd := range raw.Delete {
		d, err := compileDeleteOp(rd)
		if err != nil {
			return nil, fmt.Errorf("delete[%d]: %w", i, err)
		}
		ops.Delete = append(ops.Delete, d)
	}
	for i, rs := range raw.Set {
		s, err := compileSetOp(rs)
		if err != nil {
			return nil, fmt.Errorf("set[%d]: %w", i, err)
		}
		ops.Set = append(ops.Set, s)
	}
	return ops, nil
}

func compileDeleteOp(rd config.RawDeleteOp) (DeleteOp, error) {
	if rd.AnyName {
		if rd.ValueRegex == "" {
			return DeleteOp{}, fmt.Errorf("any_name delete requires value")
		}
		re, err := regexp.Compile(rd.ValueRegex)
		if err != nil {
			return DeleteOp{}, fmt.Errorf("value %q: %w", rd.ValueRegex, err)
		}
		return DeleteOp{AnyName: true, ValueRegex: re}, nil
	}
	if rd.Name == "" {
		return DeleteOp{}, fmt.Errorf("delete op requires name or any_name")
	}
	var re *regexp.Regexp
	if rd.ValueRegex != "" {
		var err error
		re, err = regexp.Compile(rd.ValueRegex)
		if err != nil {
			return DeleteOp{}, fmt.Errorf("value %q: %w", rd.ValueRegex, err)
		}
	}
	return DeleteOp{Name: rd.Name, ValueRegex: re}, nil
}

func compileSetOp(rs config.RawSetOp) (SetOp, error) {
	if rs.Name == "" {
		return SetOp{}, fmt.Errorf("set op requires name")
	}
	if rs.Mode != "replace" && rs.Mode != "append" {
		return SetOp{}, fmt.Errorf("set op mode must be replace or append, got %q", rs.Mode)
	}
	return SetOp{Name: rs.Name, Mode: rs.Mode, Value: rs.Value}, nil
}
