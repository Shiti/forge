package probe

import (
	"encoding/json"
)

type AgentTag struct {
	ID   *string `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

type Message struct {
	ID               int64                  `json:"id"`
	Timestamp        float64                `json:"timestamp"`
	Sender           AgentTag               `json:"sender"`
	Topics           interface{}            `json:"topics"`
	TopicPublishedTo string                 `json:"topic_published_to"`
	RecipientList    []AgentTag             `json:"recipient_list"`
	Payload          map[string]interface{} `json:"payload"`
	Format           string                 `json:"format"`
	InResponseTo     *int64                 `json:"in_response_to,omitempty"`
	Thread           []int64                `json:"thread"`
	ConversationID   *int64                 `json:"conversation_id,omitempty"`
	RoutingSlip      map[string]interface{} `json:"routing_slip,omitempty"`
}

// DefaultMessage creates a message with sensible defaults
func DefaultMessage(id int64, senderName string, payload map[string]interface{}) *Message {
	return &Message{
		ID:            id,
		Sender:        AgentTag{ID: &senderName, Name: &senderName},
		Topics:        []string{"default_topic"},
		Payload:       payload,
		Format:        "generic_json",
		RecipientList: []AgentTag{},
		Thread:        []int64{},
	}
}

// ToJSON serializes the message to a JSON string
func (m *Message) ToJSON() (string, error) {
	b, err := json.Marshal(m)
	return string(b), err
}

// MessageFromJSON deserializes a JSON string to a Message
func MessageFromJSON(data string) (*Message, error) {
	var m Message
	err := json.Unmarshal([]byte(data), &m)
	return &m, err
}
