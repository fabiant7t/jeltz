package config

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type yamlRuleSourceFile struct {
	Rules []RawRule `yaml:"rules"`
}

func loadRulesFromSources(sources []string, configDir string) ([]RawRule, error) {
	seen := make(map[string]struct{})
	var loaded []RawRule

	for _, source := range sources {
		files, err := resolveRuleSourceFiles(source, configDir)
		if err != nil {
			return nil, fmt.Errorf("loading rule source %q: %w", source, err)
		}
		for _, f := range files {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			rulesInFile, err := parseRuleSourceFile(f)
			if err != nil {
				return nil, fmt.Errorf("loading rule file %q: %w", f, err)
			}
			loaded = append(loaded, rulesInFile...)
		}
	}

	return loaded, nil
}

func resolveRuleSourceFiles(source, configDir string) ([]string, error) {
	resolved := source
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(configDir, resolved)
	}

	if hasGlobMeta(source) {
		matches, err := filepath.Glob(resolved)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("glob %q resolved to %q matched no files", source, resolved)
		}
		var files []string
		for _, m := range matches {
			collected, err := collectRuleFiles(m)
			if err != nil {
				return nil, err
			}
			files = append(files, collected...)
		}
		return files, nil
	}

	return collectRuleFiles(resolved)
}

func collectRuleFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		if !isRuleYAMLFile(path) {
			return nil, fmt.Errorf("file is not .yaml/.yml: %q", path)
		}
		return []string{path}, nil
	}

	var files []string
	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isRuleYAMLFile(p) {
			files = append(files, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(files)
	return files, nil
}

func parseRuleSourceFile(path string) ([]RawRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	top, err := decodeTopLevelNode(data)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if top == nil {
		return nil, nil
	}

	switch top.Kind {
	case yaml.MappingNode:
		if err := validateRuleSourceMapping(top); err != nil {
			return nil, err
		}
		var wrapped yamlRuleSourceFile
		if err := decodeYAMLKnownFields(data, &wrapped); err != nil {
			return nil, fmt.Errorf("invalid rule mapping: %w", err)
		}
		return wrapped.Rules, nil
	case yaml.SequenceNode:
		var seq []RawRule
		if err := decodeYAMLKnownFields(data, &seq); err != nil {
			return nil, fmt.Errorf("invalid rule sequence: %w", err)
		}
		return seq, nil
	default:
		return nil, fmt.Errorf("top-level YAML must be either a mapping with key \"rules\" or a sequence of rules")
	}
}

func decodeTopLevelNode(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	var extra yaml.Node
	err := dec.Decode(&extra)
	if err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("multiple YAML documents are not supported")
	}
	if len(root.Content) == 0 {
		return nil, nil
	}
	return root.Content[0], nil
}

func validateRuleSourceMapping(node *yaml.Node) error {
	hasRules := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Value == "rules" {
			hasRules = true
			continue
		}
		return fmt.Errorf("unknown top-level key %q at line %d (expected only \"rules\")", k.Value, k.Line)
	}
	if !hasRules {
		return fmt.Errorf("top-level mapping must contain key \"rules\"")
	}
	return nil
}

func decodeYAMLKnownFields(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}

	// Reject multi-document YAML in rule sources for deterministic behavior.
	var extra any
	err := dec.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("multiple YAML documents are not supported")
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func isRuleYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
