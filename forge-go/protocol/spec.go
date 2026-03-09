package protocol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rustic-ai/forge/forge-go/helper/idgen"
)

// AgentTag represents a tag that can be assigned to an agent.
type AgentTag struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

func NewAgentTag() AgentTag {
	return AgentTag{}
}

func (a *AgentTag) Normalize() {}

func (a *AgentTag) UnmarshalJSON(data []byte) error {
	type alias AgentTag
	raw := alias(NewAgentTag())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*a = AgentTag(raw)
	a.Normalize()
	return nil
}

// DependencySpec maps a dependency to a resolver class.
type DependencySpec struct {
	ClassName  string                 `json:"class_name" yaml:"class_name"`
	Properties map[string]interface{} `json:"properties,omitempty" yaml:"properties,omitempty"`
}

func NewDependencySpec(className string) DependencySpec {
	d := DependencySpec{
		ClassName:  className,
		Properties: map[string]interface{}{},
	}
	d.Normalize()
	return d
}

func (d *DependencySpec) Normalize() {
	if d.Properties == nil {
		d.Properties = map[string]interface{}{}
	}
}

func (d *DependencySpec) UnmarshalJSON(data []byte) error {
	type alias DependencySpec
	raw := alias(NewDependencySpec(""))
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*d = DependencySpec(raw)
	d.Normalize()
	return nil
}

// ResourceSpec specifies the resources required by an agent.
type ResourceSpec struct {
	NumCPUs         *float64               `json:"num_cpus,omitempty"`
	NumGPUs         *float64               `json:"num_gpus,omitempty"`
	Secrets         []string               `json:"secrets,omitempty"`
	CustomResources map[string]interface{} `json:"custom_resources,omitempty"`
}

func NewResourceSpec() ResourceSpec {
	r := ResourceSpec{
		Secrets:         []string{},
		CustomResources: map[string]interface{}{},
	}
	r.Normalize()
	return r
}

func (r *ResourceSpec) Normalize() {
	if r.Secrets == nil {
		r.Secrets = []string{}
	}
	if r.CustomResources == nil {
		r.CustomResources = map[string]interface{}{}
	}
}

func (r *ResourceSpec) UnmarshalJSON(data []byte) error {
	type alias ResourceSpec
	raw := alias(NewResourceSpec())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = ResourceSpec(raw)
	r.Normalize()
	return nil
}

// ValidateCustomResources checks that all custom resource values are numeric.
func (r *ResourceSpec) ValidateCustomResources() error {
	for k, v := range r.CustomResources {
		switch v.(type) {
		case float64, float32, int, int64, int32, int16, int8,
			uint, uint64, uint32, uint16, uint8, json.Number:
			// valid numeric type
		default:
			return fmt.Errorf("custom_resources[%q]: value must be numeric, got %T", k, v)
		}
	}
	return nil
}

// Validate checks the ResourceSpec for correctness.
func (r *ResourceSpec) Validate() error {
	return r.ValidateCustomResources()
}

// QOSSpec specifies Quality of Service settings for an agent.
type QOSSpec struct {
	Timeout    *int `json:"timeout,omitempty"`
	RetryCount *int `json:"retry_count,omitempty"`
	Latency    *int `json:"latency,omitempty"`
}

func NewQOSSpec() QOSSpec {
	q := QOSSpec{}
	q.Normalize()
	return q
}

func (q *QOSSpec) Normalize() {}

func (q *QOSSpec) UnmarshalJSON(data []byte) error {
	type alias QOSSpec
	raw := alias(NewQOSSpec())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*q = QOSSpec(raw)
	q.Normalize()
	return nil
}

type ProcessStatus string

const (
	ProcessStatusRunning   ProcessStatus = "running"
	ProcessStatusError     ProcessStatus = "error"
	ProcessStatusCompleted ProcessStatus = "completed"
)

type RoutingOrigin struct {
	OriginSender        *AgentTag `json:"origin_sender,omitempty"`
	OriginTopic         *string   `json:"origin_topic,omitempty"`
	OriginMessageFormat *string   `json:"origin_message_format,omitempty"`
}

func NewRoutingOrigin() RoutingOrigin {
	r := RoutingOrigin{}
	r.Normalize()
	return r
}

func (r *RoutingOrigin) Normalize() {
	if r.OriginSender != nil {
		r.OriginSender.Normalize()
	}
}

func (r *RoutingOrigin) UnmarshalJSON(data []byte) error {
	type alias RoutingOrigin
	raw := alias(NewRoutingOrigin())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = RoutingOrigin(raw)
	r.Normalize()
	return nil
}

type RoutingDestination struct {
	Topics        Topics     `json:"topics,omitempty"`
	RecipientList []AgentTag `json:"recipient_list,omitempty"`
	Priority      *int       `json:"priority,omitempty"`
}

func NewRoutingDestination() RoutingDestination {
	d := RoutingDestination{
		RecipientList: []AgentTag{},
	}
	d.Normalize()
	return d
}

func (d *RoutingDestination) Normalize() {
	if d.RecipientList == nil {
		d.RecipientList = []AgentTag{}
	}
	for i := range d.RecipientList {
		d.RecipientList[i].Normalize()
	}
}

func (d *RoutingDestination) UnmarshalJSON(data []byte) error {
	type alias RoutingDestination
	raw := alias(NewRoutingDestination())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*d = RoutingDestination(raw)
	d.Normalize()
	return nil
}

// PredicateType defines the kind of runtime predicate.
type PredicateType string

const (
	PredicateJSONata    PredicateType = "jsonata_fn"
	PredicateCel        PredicateType = "cel_fn"
	PredicateTypeEquals PredicateType = "type_equals"
)

// RuntimePredicate describes a runtime condition for routing or agent dispatch.
type RuntimePredicate struct {
	PredicateType PredicateType `json:"predicate_type"`
	Expression    *string       `json:"expression,omitempty"`
	ExpectedType  *string       `json:"expected_type,omitempty"`
}

func NewRuntimePredicate() RuntimePredicate {
	r := RuntimePredicate{}
	r.Normalize()
	return r
}

func (r *RuntimePredicate) Normalize() {}

func (r *RuntimePredicate) UnmarshalJSON(data []byte) error {
	type alias RuntimePredicate
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = RuntimePredicate(raw)
	r.Normalize()
	return nil
}

type RoutingRule struct {
	Agent            *AgentTag           `json:"agent,omitempty"`
	AgentType        *string             `json:"agent_type,omitempty"`
	MethodName       *string             `json:"method_name,omitempty"`
	OriginFilter     *RoutingOrigin      `json:"origin_filter,omitempty"`
	MessageFormat    *string             `json:"message_format,omitempty"`
	Destination      *RoutingDestination `json:"destination,omitempty"`
	MarkForwarded    bool                `json:"mark_forwarded"`
	RouteTimes       *int                `json:"route_times,omitempty"`
	Transformer      RawJSON             `json:"transformer,omitempty"`
	AgentStateUpdate RawJSON             `json:"agent_state_update,omitempty"`
	GuildStateUpdate RawJSON             `json:"guild_state_update,omitempty"`
	ProcessStatus    *ProcessStatus      `json:"process_status,omitempty"`
	Reason           *string             `json:"reason,omitempty"`
}

func NewRoutingRule() RoutingRule {
	r := RoutingRule{
		MarkForwarded: false,
		RouteTimes:    intPtr(1),
	}
	r.Normalize()
	return r
}

func (r *RoutingRule) Normalize() {
	if r.Agent != nil {
		r.Agent.Normalize()
	}
	if r.OriginFilter != nil {
		r.OriginFilter.Normalize()
	}
	if r.Destination != nil {
		r.Destination.Normalize()
	}
	if r.RouteTimes == nil {
		r.RouteTimes = intPtr(1)
	}
}

func (r *RoutingRule) UnmarshalJSON(data []byte) error {
	type alias RoutingRule
	raw := alias(NewRoutingRule())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = RoutingRule(raw)
	r.Normalize()
	return nil
}

type RoutingSlip struct {
	Steps []RoutingRule `json:"steps"`
}

func NewRoutingSlip() RoutingSlip {
	s := RoutingSlip{
		Steps: []RoutingRule{},
	}
	s.Normalize()
	return s
}

func (s *RoutingSlip) Normalize() {
	if s.Steps == nil {
		s.Steps = []RoutingRule{}
	}
	for i := range s.Steps {
		s.Steps[i].Normalize()
	}
}

func (s *RoutingSlip) UnmarshalJSON(data []byte) error {
	type alias RoutingSlip
	raw := alias(NewRoutingSlip())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = RoutingSlip(raw)
	s.Normalize()
	return nil
}

// AgentSpec defines an agent's configuration.
type AgentSpec struct {
	ID                     string                      `json:"id"`
	Name                   string                      `json:"name"`
	Description            string                      `json:"description"`
	ClassName              string                      `json:"class_name"`
	AdditionalTopics       []string                    `json:"additional_topics"`
	Properties             map[string]interface{}      `json:"properties"`
	ListenToDefaultTopic   *bool                       `json:"listen_to_default_topic,omitempty"`
	ActOnlyWhenTagged      *bool                       `json:"act_only_when_tagged,omitempty"`
	Predicates             map[string]RuntimePredicate `json:"predicates,omitempty"`
	DependencyMap          map[string]DependencySpec   `json:"dependency_map,omitempty"`
	AdditionalDependencies []string                    `json:"additional_dependencies,omitempty"`
	Resources              ResourceSpec                `json:"resources,omitempty"`
	QOS                    QOSSpec                     `json:"qos,omitempty"`
}

func NewAgentSpec() AgentSpec {
	a := AgentSpec{
		ID:                     idgen.NewShortUUID(),
		AdditionalTopics:       []string{},
		Properties:             map[string]interface{}{},
		ListenToDefaultTopic:   boolPtr(true),
		ActOnlyWhenTagged:      boolPtr(false),
		Predicates:             map[string]RuntimePredicate{},
		DependencyMap:          map[string]DependencySpec{},
		AdditionalDependencies: []string{},
		Resources:              NewResourceSpec(),
		QOS:                    NewQOSSpec(),
	}
	a.Normalize()
	return a
}

func (a *AgentSpec) Normalize() {
	if a.ID == "" {
		a.ID = idgen.NewShortUUID()
	}
	// Whitespace stripping (matches Python's str_strip_whitespace)
	a.Name = strings.TrimSpace(a.Name)
	a.Description = strings.TrimSpace(a.Description)
	a.ClassName = strings.TrimSpace(a.ClassName)
	if a.AdditionalTopics == nil {
		a.AdditionalTopics = []string{}
	}
	if a.Properties == nil {
		a.Properties = map[string]interface{}{}
	}
	if a.ListenToDefaultTopic == nil {
		a.ListenToDefaultTopic = boolPtr(true)
	}
	if a.ActOnlyWhenTagged == nil {
		a.ActOnlyWhenTagged = boolPtr(false)
	}
	if a.Predicates == nil {
		a.Predicates = map[string]RuntimePredicate{}
	}
	if a.DependencyMap == nil {
		a.DependencyMap = map[string]DependencySpec{}
	}
	for key, dep := range a.DependencyMap {
		dep.Normalize()
		a.DependencyMap[key] = dep
	}
	if a.AdditionalDependencies == nil {
		a.AdditionalDependencies = []string{}
	}
	a.Resources.Normalize()
	a.QOS.Normalize()
}

// Validate checks the AgentSpec for correctness.
func (a *AgentSpec) Validate() error {
	if len(a.Name) < 1 || len(a.Name) > 64 {
		return fmt.Errorf("agent name must be 1-64 characters, got %d", len(a.Name))
	}
	if len(a.Description) < 1 {
		return fmt.Errorf("agent description must not be empty")
	}
	return nil
}

func (a *AgentSpec) UnmarshalJSON(data []byte) error {
	type alias AgentSpec
	raw := alias(NewAgentSpec())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*a = AgentSpec(raw)
	a.Normalize()
	return nil
}

// GatewayConfig specifies the automatic GatewayAgent configuration.
type GatewayConfig struct {
	Enabled         bool     `json:"enabled"`
	InputFormats    []string `json:"input_formats"`
	OutputFormats   []string `json:"output_formats"`
	ReturnedFormats []string `json:"returned_formats,omitempty"`
}

func NewGatewayConfig() GatewayConfig {
	g := GatewayConfig{
		Enabled:         true,
		InputFormats:    []string{},
		OutputFormats:   []string{},
		ReturnedFormats: []string{},
	}
	g.Normalize()
	return g
}

func (g *GatewayConfig) Normalize() {
	if g.InputFormats == nil {
		g.InputFormats = []string{}
	}
	if g.OutputFormats == nil {
		g.OutputFormats = []string{}
	}
	if g.ReturnedFormats == nil {
		g.ReturnedFormats = []string{}
	}
}

func (g *GatewayConfig) UnmarshalJSON(data []byte) error {
	var raw struct {
		Enabled         *bool    `json:"enabled"`
		InputFormats    []string `json:"input_formats"`
		OutputFormats   []string `json:"output_formats"`
		ReturnedFormats []string `json:"returned_formats"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	cfg := NewGatewayConfig()
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	cfg.InputFormats = raw.InputFormats
	cfg.OutputFormats = raw.OutputFormats
	cfg.ReturnedFormats = raw.ReturnedFormats
	cfg.Normalize()
	*g = cfg
	return nil
}

// GuildSpec defines the overall guild configuration.
type GuildSpec struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	Properties    map[string]interface{}    `json:"properties"`
	Configuration map[string]interface{}    `json:"configuration,omitempty"`
	Agents        []AgentSpec               `json:"agents"`
	DependencyMap map[string]DependencySpec `json:"dependency_map,omitempty"`
	Routes        *RoutingSlip              `json:"routes,omitempty"`
	Gateway       *GatewayConfig            `json:"gateway,omitempty"`
}

func NewGuildSpec() GuildSpec {
	g := GuildSpec{
		ID:            idgen.NewShortUUID(),
		Properties:    map[string]interface{}{},
		Agents:        []AgentSpec{},
		DependencyMap: map[string]DependencySpec{},
		Routes:        routingSlipPtr(NewRoutingSlip()),
	}
	g.Normalize()
	return g
}

func (g *GuildSpec) Normalize() {
	if g.ID == "" {
		g.ID = idgen.NewShortUUID()
	}
	// Whitespace stripping (matches Python's str_strip_whitespace)
	g.Name = strings.TrimSpace(g.Name)
	g.Description = strings.TrimSpace(g.Description)
	if g.Properties == nil {
		g.Properties = map[string]interface{}{}
	}
	if g.Agents == nil {
		g.Agents = []AgentSpec{}
	}
	for i := range g.Agents {
		g.Agents[i].Normalize()
	}
	if g.DependencyMap == nil {
		g.DependencyMap = map[string]DependencySpec{}
	}
	for key, dep := range g.DependencyMap {
		dep.Normalize()
		g.DependencyMap[key] = dep
	}
	if g.Routes == nil {
		g.Routes = routingSlipPtr(NewRoutingSlip())
	}
	g.Routes.Normalize()
	if g.Gateway != nil {
		g.Gateway.Normalize()
	}
}

// Validate checks the GuildSpec for correctness.
func (g *GuildSpec) Validate() error {
	if len(g.Name) < 1 || len(g.Name) > 64 {
		return fmt.Errorf("guild name must be 1-64 characters, got %d", len(g.Name))
	}
	if len(g.Description) < 1 {
		return fmt.Errorf("guild description must not be empty")
	}
	return nil
}

func (g *GuildSpec) UnmarshalJSON(data []byte) error {
	type alias GuildSpec
	raw := alias(NewGuildSpec())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = GuildSpec(raw)
	g.Normalize()
	return nil
}

// MessagingConfig represents the configuration of a message bus.
type MessagingConfig struct {
	BackendModule string                 `json:"backend_module"`
	BackendClass  string                 `json:"backend_class"`
	BackendConfig map[string]interface{} `json:"backend_config"`
}

func NewMessagingConfig() MessagingConfig {
	m := MessagingConfig{
		BackendConfig: map[string]interface{}{},
	}
	m.Normalize()
	return m
}

func (m *MessagingConfig) Normalize() {
	if m.BackendConfig == nil {
		m.BackendConfig = map[string]interface{}{}
	}
}

func (m *MessagingConfig) UnmarshalJSON(data []byte) error {
	type alias MessagingConfig
	raw := alias(NewMessagingConfig())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = MessagingConfig(raw)
	m.Normalize()
	return nil
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func routingSlipPtr(s RoutingSlip) *RoutingSlip { return &s }
