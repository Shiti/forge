package guild

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"gopkg.in/yaml.v3"
)

// ParseFile reads a YAML or JSON file, unmarshals it to a GuildSpec,
// and returns both the GuildSpec and the raw JSON bytes representing it.
func ParseFile(path string) (*protocol.GuildSpec, []byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))

	var spec protocol.GuildSpec
	var rawJSON []byte

	switch ext {
	case ".yaml", ".yml":
		var rootNode yaml.Node
		if err := yaml.Unmarshal(content, &rootNode); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal YAML %s: %w", path, err)
		}

		baseDir := filepath.Dir(path)
		if err := resolveYamlTags(&rootNode, baseDir, make(map[string]bool)); err != nil {
			return nil, nil, fmt.Errorf("failed to resolve modular guild tags: %w", err)
		}

		var body interface{}
		if err := rootNode.Decode(&body); err != nil {
			return nil, nil, fmt.Errorf("failed to decode revolved YAML node: %w", err)
		}

		rawJSON, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal YAML body to JSON: %w", err)
		}
	case ".json":
		rawJSON = content
	default:
		return nil, nil, fmt.Errorf("unsupported file extension: %s", ext)
	}

	if err := json.Unmarshal(rawJSON, &spec); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal JSON into GuildSpec: %w", err)
	}

	return &spec, rawJSON, nil
}

// resolveYamlTags recursively walks the parsed YAML AST to resolve !include and !code
func resolveYamlTags(node *yaml.Node, baseDir string, visited map[string]bool) error {
	switch node.ShortTag() {
	case "!include":
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("!include tag must be on a scalar value (file path)")
		}

		targetPath := node.Value
		if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(baseDir, targetPath)
		}

		if visited[targetPath] {
			return fmt.Errorf("circular include detected: %s", targetPath)
		}

		ext := strings.ToLower(filepath.Ext(targetPath))
		if ext != ".yaml" && ext != ".yml" {
			return fmt.Errorf("!include only supports .yaml or .yml files, found %s (use !code for texts)", ext)
		}

		content, err := os.ReadFile(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read included file %s: %w", targetPath, err)
		}

		var subNode yaml.Node
		if err := yaml.Unmarshal(content, &subNode); err != nil {
			return fmt.Errorf("failed to parse included YAML %s: %w", targetPath, err)
		}

		newVisited := maps.Clone(visited)
		newVisited[targetPath] = true

		if err := resolveYamlTags(&subNode, filepath.Dir(targetPath), newVisited); err != nil {
			return err
		}

		if subNode.Kind == yaml.DocumentNode && len(subNode.Content) > 0 {
			*node = *subNode.Content[0]
		}
		return nil

	case "!code":
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("!code tag must be on a scalar value (file path)")
		}

		targetPath := node.Value
		if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(baseDir, targetPath)
		}

		content, err := os.ReadFile(targetPath)
		if err != nil {
			return fmt.Errorf("failed to read code file %s: %w", targetPath, err)
		}

		node.Tag = "!!str"
		node.Value = string(content)
		return nil
	}

	for _, child := range node.Content {
		if err := resolveYamlTags(child, baseDir, visited); err != nil {
			return err
		}
	}

	return nil
}
