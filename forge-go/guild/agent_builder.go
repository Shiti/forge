package guild

import (
	"fmt"

	"github.com/rustic-ai/forge/forge-go/helper/idgen"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

// AgentBuilder provides a stateful, fluent API for constructing an AgentSpec.
type AgentBuilder struct {
	spec    protocol.AgentSpec
	nameSet bool
	descSet bool
	err     error
}

// NewAgentBuilder creates an empty builder with an auto-generated ID.
func NewAgentBuilder() *AgentBuilder {
	return &AgentBuilder{
		spec: protocol.AgentSpec{
			ID:                     idgen.NewShortUUID(),
			AdditionalTopics:       []string{},
			Properties:             map[string]interface{}{},
			Predicates:             map[string]protocol.RuntimePredicate{},
			DependencyMap:          map[string]protocol.DependencySpec{},
			AdditionalDependencies: []string{},
			Resources:              protocol.NewResourceSpec(),
		},
	}
}

// --- Fluent setters ---

// SetID overrides the auto-generated agent ID.
func (b *AgentBuilder) SetID(id string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	if id == "" {
		b.err = fmt.Errorf("agent ID must not be empty")
		return b
	}
	b.spec.ID = id
	return b
}

// SetName sets the agent name. Must be 1-64 characters.
func (b *AgentBuilder) SetName(name string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	if name == "" || len(name) > 64 {
		b.err = fmt.Errorf("agent name must be 1-64 characters")
		return b
	}
	b.spec.Name = name
	b.nameSet = true
	return b
}

// SetDescription sets the agent description. Must be non-empty.
func (b *AgentBuilder) SetDescription(desc string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	if desc == "" {
		b.err = fmt.Errorf("agent description must not be empty")
		return b
	}
	b.spec.Description = desc
	b.descSet = true
	return b
}

// SetClassName sets the agent's implementation class name.
func (b *AgentBuilder) SetClassName(className string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.ClassName = className
	return b
}

// AddAdditionalTopic appends a topic to the agent's additional topics.
func (b *AgentBuilder) AddAdditionalTopic(topic string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.AdditionalTopics = append(b.spec.AdditionalTopics, topic)
	return b
}

// SetProperties replaces the agent's properties map.
func (b *AgentBuilder) SetProperties(props map[string]interface{}) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.Properties = props
	return b
}

// ListenToDefaultTopic sets whether the agent listens to the default topic.
func (b *AgentBuilder) ListenToDefaultTopic(listen bool) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.ListenToDefaultTopic = &listen
	return b
}

// ActOnlyWhenTagged sets whether the agent acts only when tagged.
func (b *AgentBuilder) ActOnlyWhenTagged(act bool) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.ActOnlyWhenTagged = &act
	return b
}

// SetDependencyMap replaces the agent's dependency map.
func (b *AgentBuilder) SetDependencyMap(deps map[string]protocol.DependencySpec) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.DependencyMap = deps
	return b
}

// AddDependencyResolver adds a single dependency resolver entry.
func (b *AgentBuilder) AddDependencyResolver(key string, dep protocol.DependencySpec) *AgentBuilder {
	if b.err != nil {
		return b
	}
	if b.spec.DependencyMap == nil {
		b.spec.DependencyMap = make(map[string]protocol.DependencySpec)
	}
	b.spec.DependencyMap[key] = dep
	return b
}

// AddPredicate adds a runtime predicate for a method.
func (b *AgentBuilder) AddPredicate(methodName string, pred protocol.RuntimePredicate) *AgentBuilder {
	if b.err != nil {
		return b
	}
	if b.spec.Predicates == nil {
		b.spec.Predicates = make(map[string]protocol.RuntimePredicate)
	}
	b.spec.Predicates[methodName] = pred
	return b
}

// SetAdditionalDependencies replaces the agent's additional dependencies list.
func (b *AgentBuilder) SetAdditionalDependencies(deps []string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.AdditionalDependencies = deps
	return b
}

// AddAdditionalDependency appends a single additional dependency.
func (b *AgentBuilder) AddAdditionalDependency(dep string) *AgentBuilder {
	if b.err != nil {
		return b
	}
	b.spec.AdditionalDependencies = append(b.spec.AdditionalDependencies, dep)
	return b
}

// --- Build methods ---

// Validate checks that required fields (name, description, className) are set.
func (b *AgentBuilder) Validate() error {
	if b.err != nil {
		return b.err
	}
	var missing []string
	if !b.nameSet {
		missing = append(missing, "name")
	}
	if !b.descSet {
		missing = append(missing, "description")
	}
	if b.spec.ClassName == "" {
		missing = append(missing, "class_name")
	}
	if len(missing) > 0 {
		return fmt.Errorf("agent builder missing required fields: %v", missing)
	}
	return nil
}

// ValidateResources performs basic sanity checks on the resource spec.
func (b *AgentBuilder) ValidateResources() error {
	if b.spec.Resources.NumCPUs != nil && *b.spec.Resources.NumCPUs < 0 {
		return fmt.Errorf("agent %s: num_cpus cannot be negative", b.spec.Name)
	}
	if b.spec.Resources.NumGPUs != nil && *b.spec.Resources.NumGPUs < 0 {
		return fmt.Errorf("agent %s: num_gpus cannot be negative", b.spec.Name)
	}
	return nil
}

// BuildSpec validates, applies defaults, and returns a copy of the agent spec.
func (b *AgentBuilder) BuildSpec() (protocol.AgentSpec, error) {
	if b.err != nil {
		return protocol.AgentSpec{}, b.err
	}
	if err := b.Validate(); err != nil {
		return protocol.AgentSpec{}, err
	}
	if err := b.ValidateResources(); err != nil {
		return protocol.AgentSpec{}, err
	}

	b.spec.Normalize()
	out := b.spec
	return out, nil
}
