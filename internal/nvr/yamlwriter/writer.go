// Package yamlwriter provides safe YAML configuration modification
// using AST manipulation to preserve comments and formatting.
package yamlwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Writer provides thread-safe YAML configuration modification.
type Writer struct {
	path string
	mu   sync.Mutex
}

// New returns a Writer that operates on the YAML file at path.
func New(path string) *Writer {
	return &Writer{path: path}
}

// AddPath adds or replaces a path entry under the top-level "paths" mapping.
// It uses AST manipulation to preserve existing comments and formatting.
func (w *Writer) AddPath(name string, config map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	pathsMapping, err := findPathsMapping(file)
	if err != nil {
		return fmt.Errorf("find paths mapping: %w", err)
	}

	// Build the new entry by marshalling the config to YAML, then parsing it
	// as a snippet so we get proper AST nodes.
	newEntry, err := buildMappingValueNode(name, config)
	if err != nil {
		return fmt.Errorf("build entry: %w", err)
	}

	// Check if the path already exists; if so, replace it.
	replaced := false
	for i, v := range pathsMapping.Values {
		if v.Key.String() == name {
			pathsMapping.Values[i] = newEntry
			replaced = true
			break
		}
	}
	if !replaced {
		pathsMapping.Values = append(pathsMapping.Values, newEntry)
	}

	return w.atomicWrite(file)
}

// RemovePath removes a path entry from the top-level "paths" mapping.
func (w *Writer) RemovePath(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	pathsMapping, err := findPathsMapping(file)
	if err != nil {
		return fmt.Errorf("find paths mapping: %w", err)
	}

	filtered := make([]*ast.MappingValueNode, 0, len(pathsMapping.Values))
	for _, v := range pathsMapping.Values {
		if v.Key.String() != name {
			filtered = append(filtered, v)
		}
	}
	pathsMapping.Values = filtered

	return w.atomicWrite(file)
}

// GetNVRPaths returns all path names under the "paths" mapping that are
// prefixed with "nvr/".
func (w *Writer) GetNVRPaths() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	pathsMapping, err := findPathsMapping(file)
	if err != nil {
		return nil, fmt.Errorf("find paths mapping: %w", err)
	}

	var result []string
	for _, v := range pathsMapping.Values {
		key := v.Key.String()
		if strings.HasPrefix(key, "nvr/") {
			result = append(result, key)
		}
	}

	return result, nil
}

// findPathsMapping locates the "paths" MappingNode in the parsed YAML file.
func findPathsMapping(file *ast.File) (*ast.MappingNode, error) {
	if len(file.Docs) == 0 {
		return nil, fmt.Errorf("empty YAML document")
	}

	body := file.Docs[0].Body
	mapping, ok := body.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("document body is not a mapping")
	}

	for _, v := range mapping.Values {
		if v.Key.String() == "paths" {
			switch val := v.Value.(type) {
			case *ast.MappingNode:
				return val, nil
			case *ast.NullNode:
				// paths key exists but is empty; create a new mapping.
				newMapping := &ast.MappingNode{
					BaseNode: &ast.BaseNode{},
				}
				v.Value = newMapping
				return newMapping, nil
			default:
				return nil, fmt.Errorf("paths value is not a mapping (type: %T)", v.Value)
			}
		}
	}

	return nil, fmt.Errorf("paths key not found")
}

// buildMappingValueNode creates an AST MappingValueNode for a path entry
// by marshalling the config and parsing the resulting snippet.
func buildMappingValueNode(name string, config map[string]interface{}) (*ast.MappingValueNode, error) {
	// Marshal the config values.
	configBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	// Build a small YAML snippet: "name:\n  key: value\n  ..."
	// Indent the config under the name key.
	indented := indentLines(string(configBytes), "  ")
	snippet := name + ":\n" + indented

	snippetFile, err := parser.ParseBytes([]byte(snippet), parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse snippet: %w", err)
	}

	if len(snippetFile.Docs) == 0 {
		return nil, fmt.Errorf("empty snippet document")
	}

	snippetBody, ok := snippetFile.Docs[0].Body.(*ast.MappingNode)
	if !ok {
		// It might be a single MappingValueNode.
		if mv, ok2 := snippetFile.Docs[0].Body.(*ast.MappingValueNode); ok2 {
			return mv, nil
		}
		return nil, fmt.Errorf("snippet body is not a mapping (type: %T)", snippetFile.Docs[0].Body)
	}

	if len(snippetBody.Values) == 0 {
		return nil, fmt.Errorf("snippet has no values")
	}

	return snippetBody.Values[0], nil
}

// indentLines adds a prefix to each non-empty line.
func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// atomicWrite writes the AST file to disk using a temp file and rename.
func (w *Writer) atomicWrite(file *ast.File) error {
	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".mediamtx-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	content := file.String() + "\n"
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, w.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
