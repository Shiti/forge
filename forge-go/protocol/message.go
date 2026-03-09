package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ForwardHeader tracks the broadcast history and deduplication metadata of a message.
type ForwardHeader struct {
	OriginMessageID uint64   `json:"origin_message_id"`
	OnBehalfOf      AgentTag `json:"on_behalf_of"`
}

// ProcessEntry records a single processing step in the message history.
type ProcessEntry struct {
	Agent     AgentTag `json:"agent"`
	Origin    uint64   `json:"origin"`
	Result    uint64   `json:"result"`
	Processor string   `json:"processor"`
	FromTopic *string  `json:"from_topic,omitempty"`
	ToTopics  []string `json:"to_topics,omitempty"`
	Reason    []string `json:"reason,omitempty"`
}

func NewProcessEntry() ProcessEntry {
	p := ProcessEntry{
		ToTopics: []string{},
		Reason:   []string{},
	}
	p.Normalize()
	return p
}

func (p *ProcessEntry) Normalize() {
	p.Agent.Normalize()
	if p.ToTopics == nil {
		p.ToTopics = []string{}
	}
	if p.Reason == nil {
		p.Reason = []string{}
	}
}

func (p *ProcessEntry) UnmarshalJSON(data []byte) error {
	type alias ProcessEntry
	raw := alias(NewProcessEntry())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = ProcessEntry(raw)
	p.Normalize()
	return nil
}

// ForwardHeader UnmarshalJSON prevents DisallowUnknownFields from propagating.
func (f *ForwardHeader) UnmarshalJSON(data []byte) error {
	type alias ForwardHeader
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = ForwardHeader(raw)
	return nil
}

// GuildStackEntry represents a guild/saga pair in the origin guild stack.
type GuildStackEntry struct {
	GuildID string  `json:"guild_id"`
	SagaID  *string `json:"saga_id,omitempty"`
}

// GuildStackEntry UnmarshalJSON prevents DisallowUnknownFields from propagating.
func (g *GuildStackEntry) UnmarshalJSON(data []byte) error {
	type alias GuildStackEntry
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*g = GuildStackEntry(raw)
	return nil
}

// Topics is a union type that handles `string | []string` in JSON.
type Topics struct {
	value interface{} // string or []string
}

func TopicsFromString(s string) Topics {
	return Topics{value: s}
}

func TopicsFromSlice(ss []string) Topics {
	if ss == nil {
		ss = []string{}
	}
	return Topics{value: ss}
}

// ToSlice returns the topics as a string slice. A single string topic is wrapped in a slice.
func (t Topics) ToSlice() []string {
	switch v := t.value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	default:
		return []string{}
	}
}

// String returns the topic as a string. If multiple topics, returns the first one.
func (t Topics) String() string {
	switch v := t.value.(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
		return ""
	default:
		return ""
	}
}

// IsZero returns true if no topics value is set.
func (t Topics) IsZero() bool {
	return t.value == nil
}

func (t Topics) MarshalJSON() ([]byte, error) {
	if t.value == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(t.value)
}

func (t *Topics) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.value = nil
		return nil
	}
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		t.value = s
		return nil
	}
	// Try []string
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		t.value = ss
		return nil
	}
	// unrecognized
	t.value = nil
	return nil
}

// Message represents the core communication unit between Agents and WebSockets.
type Message struct {
	ID                uint64            `json:"id"`
	Topics            Topics            `json:"topics"`
	Sender            AgentTag          `json:"sender"`
	RecipientList     []AgentTag        `json:"recipient_list"`
	Format            string            `json:"format"`
	Payload           json.RawMessage   `json:"payload"`
	Thread            []uint64          `json:"thread"`
	MessageHistory    []ProcessEntry    `json:"message_history"`
	RoutingSlip       *RoutingSlip      `json:"routing_slip"`
	ForwardHeader     *ForwardHeader    `json:"forward_header"`
	Traceparent       *string           `json:"traceparent"`
	InResponseTo      *uint64           `json:"in_response_to"`
	ConversationID    *uint64           `json:"conversation_id"`
	TTL               *int              `json:"ttl"`
	IsErrorMessage    bool              `json:"is_error_message"`
	SessionState      json.RawMessage   `json:"session_state"`
	TopicPublishedTo  *string           `json:"topic_published_to"`
	EnrichWithHistory int               `json:"enrich_with_history"`
	ProcessStatus     *ProcessStatus    `json:"process_status"`
	Priority          int               `json:"priority"`
	Timestamp         float64           `json:"timestamp"`
	OriginGuildStack  []GuildStackEntry `json:"origin_guild_stack"`
}

func NewMessage() Message {
	m := Message{
		RecipientList:    []AgentTag{},
		Thread:           []uint64{},
		MessageHistory:   []ProcessEntry{},
		OriginGuildStack: []GuildStackEntry{},
	}
	m.Normalize()
	return m
}

// NewMessageFromGemstoneID creates a Message with ID, Priority, and Timestamp
// derived from the given GemstoneID.
func NewMessageFromGemstoneID(gid GemstoneID) Message {
	m := NewMessage()
	m.ID = gid.ToInt()
	m.Priority = int(gid.Priority)
	m.Timestamp = float64(gid.Timestamp)
	return m
}

func (m *Message) Normalize() {
	m.Sender.Normalize()
	if m.RecipientList == nil {
		m.RecipientList = []AgentTag{}
	}
	for i := range m.RecipientList {
		m.RecipientList[i].Normalize()
	}
	if m.Thread == nil {
		m.Thread = []uint64{}
	}
	if m.MessageHistory == nil {
		m.MessageHistory = []ProcessEntry{}
	}
	for i := range m.MessageHistory {
		m.MessageHistory[i].Normalize()
	}
	if m.OriginGuildStack == nil {
		m.OriginGuildStack = []GuildStackEntry{}
	}
	if m.RoutingSlip != nil {
		m.RoutingSlip.Normalize()
	}
	if m.ForwardHeader != nil {
		m.ForwardHeader.OnBehalfOf.Normalize()
	}
	// Derive Priority and Timestamp from ID (matches Python's @computed_field)
	if m.ID != 0 {
		if parsed, err := ParseGemstoneID(m.ID); err == nil {
			m.Priority = int(parsed.Priority)
			m.Timestamp = float64(parsed.Timestamp)
		}
	}
	// Whitespace stripping (matches Python's str_strip_whitespace)
	m.Format = strings.TrimSpace(m.Format)
}

func (m *Message) UnmarshalJSON(data []byte) error {
	type alias Message
	raw := alias(NewMessage())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	*m = Message(raw)
	m.Normalize()
	return nil
}

// CurrentThreadID returns the last thread ID, or the message ID if thread is empty.
func (m *Message) CurrentThreadID() uint64 {
	if len(m.Thread) > 0 {
		return m.Thread[len(m.Thread)-1]
	}
	return m.ID
}

// RootThreadID returns the first thread ID, or the message ID if thread is empty.
func (m *Message) RootThreadID() uint64 {
	if len(m.Thread) > 0 {
		return m.Thread[0]
	}
	return m.ID
}

// MessageRoutable contains the routable subset of a Message.
type MessageRoutable struct {
	Topics            Topics          `json:"topics"`
	Priority          int             `json:"priority"`
	RecipientList     []AgentTag      `json:"recipient_list"`
	Payload           json.RawMessage `json:"payload"`
	Format            string          `json:"format"`
	ForwardHeader     *ForwardHeader  `json:"forward_header,omitempty"`
	Context           json.RawMessage `json:"context,omitempty"`
	EnrichWithHistory int             `json:"enrich_with_history"`
	ProcessStatus     *ProcessStatus  `json:"process_status,omitempty"`
}

func NewMessageRoutable() MessageRoutable {
	mr := MessageRoutable{
		RecipientList: []AgentTag{},
	}
	mr.Normalize()
	return mr
}

func (mr *MessageRoutable) Normalize() {
	if mr.RecipientList == nil {
		mr.RecipientList = []AgentTag{}
	}
	for i := range mr.RecipientList {
		mr.RecipientList[i].Normalize()
	}
	mr.Format = strings.TrimSpace(mr.Format)
}

func (mr *MessageRoutable) UnmarshalJSON(data []byte) error {
	type alias MessageRoutable
	raw := alias(NewMessageRoutable())
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*mr = MessageRoutable(raw)
	mr.Normalize()
	return nil
}
