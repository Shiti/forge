package guild

import (
	"fmt"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

// Validate checks a GuildSpec for structural correctness.
func Validate(spec *protocol.GuildSpec) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}

	if spec.Name == "" {
		return fmt.Errorf("guild must have a name")
	}

	if spec.Description == "" {
		return fmt.Errorf("guild must have a description")
	}

	if len(spec.Agents) == 0 {
		return fmt.Errorf("guild must have at least one agent")
	}

	names := make(map[string]bool)

	for i := range spec.Agents {
		agent := &spec.Agents[i]

		if agent.Name == "" {
			return fmt.Errorf("agent at index %d must have a name", i)
		}

		if agent.ClassName == "" {
			return fmt.Errorf("agent '%s' must have a class_name", agent.Name)
		}

		if names[agent.Name] {
			return fmt.Errorf("duplicate agent name found: %s", agent.Name)
		}
		names[agent.Name] = true

		// Apply per-agent defaults inline
		if agent.Properties == nil {
			agent.Properties = make(map[string]interface{})
		}
		if agent.Predicates == nil {
			agent.Predicates = make(map[string]protocol.RuntimePredicate)
		}
		if agent.DependencyMap == nil {
			agent.DependencyMap = make(map[string]protocol.DependencySpec)
		}
		if agent.Resources.CustomResources == nil {
			agent.Resources.CustomResources = make(map[string]interface{})
		}

		// Validate resources
		if agent.Resources.NumCPUs != nil && *agent.Resources.NumCPUs < 0 {
			return fmt.Errorf("agent %s: num_cpus cannot be negative", agent.Name)
		}
		if agent.Resources.NumGPUs != nil && *agent.Resources.NumGPUs < 0 {
			return fmt.Errorf("agent %s: num_gpus cannot be negative", agent.Name)
		}
	}

	if msg, ok := spec.Properties["messaging"]; ok {
		msgMap, isMap := msg.(map[string]interface{})
		if !isMap {
			return fmt.Errorf("properties.messaging must be an object")
		}
		if _, hasModule := msgMap["backend_module"]; !hasModule {
			return fmt.Errorf("messaging config missing 'backend_module'")
		}
		if _, hasClass := msgMap["backend_class"]; !hasClass {
			return fmt.Errorf("messaging config missing 'backend_class'")
		}
	}

	return nil
}
