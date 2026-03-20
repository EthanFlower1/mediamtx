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
// It works at the text level to ensure correct indentation.
func (w *Writer) AddPath(name string, config map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	content := string(data)

	// First remove any existing entry for this path.
	content = removePathText(content, name)

	// Build the new entry with proper 2-space indentation under paths:.
	configBytes, marshalErr := yaml.Marshal(config)
	if marshalErr != nil {
		return fmt.Errorf("marshal config: %w", marshalErr)
	}
	entry := "  " + name + ":\n" + indentLines(string(configBytes), "    ")

	// Find the end of the paths: section and append the entry.
	content = appendToPathsSection(content, entry)

	return w.atomicWriteText(content)
}

// RemovePath removes a path entry from the top-level "paths" mapping.
func (w *Writer) RemovePath(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	content := removePathText(string(data), name)
	return w.atomicWriteText(content)
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

// SetTopLevelValue sets a top-level scalar value in the YAML config file,
// preserving comments and formatting.
func (w *Writer) SetTopLevelValue(key, value string) error {
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

	if len(file.Docs) == 0 {
		return fmt.Errorf("empty YAML document")
	}

	body := file.Docs[0].Body
	mapping, ok := body.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("document body is not a mapping")
	}

	// Parse a snippet to get a properly initialized AST node for the value.
	snippet := fmt.Sprintf("v: %q", value)
	snippetFile, err := parser.ParseBytes([]byte(snippet), 0)
	if err != nil {
		return fmt.Errorf("parse value snippet: %w", err)
	}
	var valueNode ast.Node
	switch n := snippetFile.Docs[0].Body.(type) {
	case *ast.MappingNode:
		if len(n.Values) > 0 {
			valueNode = n.Values[0].Value
		}
	case *ast.MappingValueNode:
		valueNode = n.Value
	}
	if valueNode == nil {
		return fmt.Errorf("failed to extract value from snippet")
	}

	for _, v := range mapping.Values {
		if v.Key.String() == key {
			v.Value = valueNode
			return w.atomicWrite(file)
		}
	}

	return fmt.Errorf("key %q not found", key)
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

// removePathText removes a path entry (key + indented values) from the YAML text.
func removePathText(content, name string) string {
	lines := strings.Split(content, "\n")
	var result []string
	skip := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if skip {
			// Continue skipping indented lines belonging to this path entry.
			if trimmed == "" || (len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && !isTopLevelOrPathKey(line)) {
				continue
			}
			skip = false
		}

		// Check if this line is the path key we want to remove.
		if trimmed == name+":" || strings.HasPrefix(trimmed, name+":") {
			skip = true
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// isTopLevelOrPathKey checks if a line is a new path-level key (2-space indent).
func isTopLevelOrPathKey(line string) bool {
	if len(line) < 2 {
		return false
	}
	// A path key under paths: starts with exactly 2 spaces then a non-space char.
	return line[0] == ' ' && line[1] == ' ' && len(line) > 2 && line[2] != ' '
}

// appendToPathsSection finds the end of the paths: section and appends the entry.
func appendToPathsSection(content, entry string) string {
	lines := strings.Split(content, "\n")

	// Find the "paths:" line.
	pathsIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "paths:" {
			pathsIdx = i
			break
		}
	}
	if pathsIdx == -1 {
		// No paths section found; append at end.
		return content + "\npaths:\n" + entry
	}

	// Find the end of the paths section: last line that is empty or indented.
	endIdx := len(lines)
	for i := pathsIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		// A non-empty line at column 0 that isn't a comment marks end of paths section.
		if trimmed != "" && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			endIdx = i
			break
		}
	}

	// Insert the entry before endIdx.
	before := strings.Join(lines[:endIdx], "\n")
	after := strings.Join(lines[endIdx:], "\n")

	// Ensure there's a newline before the entry.
	if !strings.HasSuffix(before, "\n") {
		before += "\n"
	}

	return before + entry + after
}

// atomicWrite writes the AST file to disk using a temp file and rename.
func (w *Writer) atomicWrite(file *ast.File) error {
	return w.atomicWriteText(file.String() + "\n")
}

// atomicWriteText writes text content to disk using a temp file and rename.
func (w *Writer) atomicWriteText(content string) error {
	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".mediamtx-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

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
