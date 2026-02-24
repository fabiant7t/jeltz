package rules

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fabiant7t/jeltz/internal/config"
)

// MapLocalRule is a compiled map-local rule.
type MapLocalRule struct {
	Match       *Match
	FSPath      string // absolute filesystem path (file or directory)
	IndexFile   string
	StatusCode  int
	ContentType string // empty = auto-detect
	Response    *Ops
}

// MapLocalResult is returned when a MapLocalRule is matched.
type MapLocalResult struct {
	FSTarget    string // resolved file path to serve
	StatusCode  int
	ContentType string // may be empty; caller uses mime/sniff
	Response    *Ops
}

// CompileMapLocalRule compiles a RawRule of type map_local.
// basePath is used to resolve relative rule.Path values.
func CompileMapLocalRule(raw config.RawRule, basePath string) (*MapLocalRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}

	// Enforce that path regex anchors from start (required for prefix stripping).
	if !strings.HasPrefix(raw.Match.Path, "^") {
		return nil, fmt.Errorf("map_local match.path regex must start with '^' for correct prefix stripping, got %q", raw.Match.Path)
	}

	if raw.Path == "" {
		return nil, fmt.Errorf("map_local rule requires path")
	}

	fsPath := raw.Path
	if !filepath.IsAbs(fsPath) {
		fsPath = filepath.Join(basePath, fsPath)
	}

	indexFile := raw.IndexFile
	if indexFile == "" {
		indexFile = "index.html"
	}

	statusCode := raw.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	respOps, err := CompileOps(raw.Response)
	if err != nil {
		return nil, fmt.Errorf("response ops: %w", err)
	}

	return &MapLocalRule{
		Match:       m,
		FSPath:      fsPath,
		IndexFile:   indexFile,
		StatusCode:  statusCode,
		ContentType: raw.ContentType,
		Response:    respOps,
	}, nil
}

// Resolve attempts to match fm against this rule and returns the target file
// path and metadata. Returns (nil, nil) if the rule does not match.
// Returns errTraversal if the resolved path escapes the rule directory.
func (r *MapLocalRule) Resolve(fm FlowMeta) (*MapLocalResult, error) {
	if !r.Match.Matches(fm) {
		return nil, nil
	}

	// Prefix stripping: find match extent from index 0.
	loc := r.Match.Path.FindStringIndex(fm.Path)
	if loc == nil || loc[0] != 0 {
		// Must match from start; treat as non-match.
		return nil, nil
	}
	stripped := fm.Path[loc[1]:]
	if stripped == "" {
		stripped = "/"
	}
	if strings.HasSuffix(stripped, "/") {
		stripped += r.IndexFile
	}

	// Determine filesystem target.
	info, err := os.Stat(r.FSPath)
	if err != nil {
		return nil, fmt.Errorf("rule path %q: %w", r.FSPath, err)
	}

	var target string
	if !info.IsDir() {
		// Rule path is a file — always serve it directly.
		target = r.FSPath
	} else {
		// Rule path is a directory — join with stripped URL path.
		urlPart := path.Clean("/" + stripped)
		fsRel := filepath.FromSlash(strings.TrimPrefix(urlPart, "/"))
		candidate := filepath.Join(r.FSPath, fsRel)

		// Traversal protection.
		absBase, err := filepath.Abs(r.FSPath)
		if err != nil {
			return nil, fmt.Errorf("abs rule path: %w", err)
		}
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return nil, fmt.Errorf("abs candidate path: %w", err)
		}
		rel, err := filepath.Rel(absBase, absCandidate)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, errTraversal
		}
		target = absCandidate
	}

	return &MapLocalResult{
		FSTarget:    target,
		StatusCode:  r.StatusCode,
		ContentType: r.ContentType,
		Response:    r.Response,
	}, nil
}

// errTraversal is a sentinel error for path traversal attempts.
var errTraversal = fmt.Errorf("path traversal detected")

// IsTraversal reports whether err is a traversal protection error.
func IsTraversal(err error) bool { return err == errTraversal }

// DetectContentType returns the MIME type for a file path.
// Uses ContentType override → mime.TypeByExtension → http.DetectContentType
// (reads first 512 bytes). Falls back to "application/octet-stream".
func DetectContentType(fsPath, override string, readSniff func(string) ([]byte, error)) string {
	if override != "" {
		return override
	}
	if ct := mime.TypeByExtension(filepath.Ext(fsPath)); ct != "" {
		return ct
	}
	if data, err := readSniff(fsPath); err == nil {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}
