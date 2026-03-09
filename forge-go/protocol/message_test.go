package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicsMarshalString(t *testing.T) {
	topics := TopicsFromString("my_topic")
	b, err := json.Marshal(topics)
	require.NoError(t, err)
	assert.JSONEq(t, `"my_topic"`, string(b))
}

func TestTopicsMarshalSlice(t *testing.T) {
	topics := TopicsFromSlice([]string{"a", "b"})
	b, err := json.Marshal(topics)
	require.NoError(t, err)
	assert.JSONEq(t, `["a","b"]`, string(b))
}

func TestTopicsUnmarshalString(t *testing.T) {
	var topics Topics
	err := json.Unmarshal([]byte(`"single"`), &topics)
	require.NoError(t, err)
	assert.Equal(t, "single", topics.String())
	assert.Equal(t, []string{"single"}, topics.ToSlice())
}

func TestTopicsUnmarshalSlice(t *testing.T) {
	var topics Topics
	err := json.Unmarshal([]byte(`["x","y"]`), &topics)
	require.NoError(t, err)
	assert.Equal(t, []string{"x", "y"}, topics.ToSlice())
	assert.Equal(t, "x", topics.String())
}

func TestTopicsUnmarshalNull(t *testing.T) {
	var topics Topics
	err := json.Unmarshal([]byte(`null`), &topics)
	require.NoError(t, err)
	assert.True(t, topics.IsZero())
	assert.Equal(t, []string{}, topics.ToSlice())
}

func TestTopicsRoundTrip(t *testing.T) {
	original := TopicsFromSlice([]string{"topic_a", "topic_b"})
	b, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Topics
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, original.ToSlice(), decoded.ToSlice())
}

func TestProcessEntryRoundTrip(t *testing.T) {
	agentID := "agent-1"
	agentName := "TestAgent"
	fromTopic := "input_topic"
	pe := ProcessEntry{
		Agent:     AgentTag{ID: &agentID, Name: &agentName},
		Origin:    100,
		Result:    200,
		Processor: "forward_message",
		FromTopic: &fromTopic,
		ToTopics:  []string{"out_a", "out_b"},
		Reason:    []string{"matched"},
	}

	b, err := json.Marshal(pe)
	require.NoError(t, err)

	var decoded ProcessEntry
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pe.Agent.ID, decoded.Agent.ID)
	assert.Equal(t, pe.Agent.Name, decoded.Agent.Name)
	assert.Equal(t, pe.Origin, decoded.Origin)
	assert.Equal(t, pe.Result, decoded.Result)
	assert.Equal(t, pe.Processor, decoded.Processor)
	assert.Equal(t, pe.FromTopic, decoded.FromTopic)
	assert.Equal(t, pe.ToTopics, decoded.ToTopics)
	assert.Equal(t, pe.Reason, decoded.Reason)
}

func TestProcessEntryNormalizeNilSlices(t *testing.T) {
	var pe ProcessEntry
	err := json.Unmarshal([]byte(`{"agent":{},"origin":1,"result":2,"processor":"p"}`), &pe)
	require.NoError(t, err)
	assert.NotNil(t, pe.ToTopics)
	assert.NotNil(t, pe.Reason)
	assert.Equal(t, []string{}, pe.ToTopics)
	assert.Equal(t, []string{}, pe.Reason)
}

func TestMessageRoundTrip(t *testing.T) {
	senderID := "agent-x"
	senderName := "AgentX"
	recipientID := "agent-y"
	traceparent := "00-trace-span-01"
	topicPublished := "my_topic"

	msg := Message{
		ID:     42,
		Topics: TopicsFromString("my_topic"),
		Sender: AgentTag{ID: &senderID, Name: &senderName},
		RecipientList: []AgentTag{
			{ID: &recipientID},
		},
		Format:  "test.Format",
		Payload: json.RawMessage(`{"key":"value"}`),
		Thread:  []uint64{1, 2, 42},
		MessageHistory: []ProcessEntry{
			{
				Agent:     AgentTag{ID: &senderID},
				Origin:    1,
				Result:    2,
				Processor: "route",
				ToTopics:  []string{},
				Reason:    []string{},
			},
		},
		Traceparent:      &traceparent,
		TopicPublishedTo: &topicPublished,
		OriginGuildStack: []GuildStackEntry{
			{GuildID: "guild-1"},
		},
	}

	b, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.ID, decoded.ID)
	assert.Equal(t, msg.Topics.String(), decoded.Topics.String())
	assert.Equal(t, *msg.Sender.ID, *decoded.Sender.ID)
	assert.Equal(t, *msg.Sender.Name, *decoded.Sender.Name)
	assert.Len(t, decoded.RecipientList, 1)
	assert.Equal(t, *msg.RecipientList[0].ID, *decoded.RecipientList[0].ID)
	assert.Equal(t, msg.Format, decoded.Format)
	assert.Equal(t, msg.Thread, decoded.Thread)
	assert.Len(t, decoded.MessageHistory, 1)
	assert.Equal(t, msg.MessageHistory[0].Processor, decoded.MessageHistory[0].Processor)
	assert.Len(t, decoded.OriginGuildStack, 1)
	assert.Equal(t, "guild-1", decoded.OriginGuildStack[0].GuildID)
}

func TestMessageNormalizeNilSlices(t *testing.T) {
	var msg Message
	err := json.Unmarshal([]byte(`{"id":1}`), &msg)
	require.NoError(t, err)
	assert.NotNil(t, msg.RecipientList)
	assert.NotNil(t, msg.Thread)
	assert.NotNil(t, msg.MessageHistory)
	assert.NotNil(t, msg.OriginGuildStack)
	assert.Equal(t, []AgentTag{}, msg.RecipientList)
	assert.Equal(t, []uint64{}, msg.Thread)
	assert.Equal(t, []ProcessEntry{}, msg.MessageHistory)
	assert.Equal(t, []GuildStackEntry{}, msg.OriginGuildStack)
}

func TestMessageCurrentThreadID(t *testing.T) {
	msg := &Message{ID: 99, Thread: []uint64{10, 20, 30}}
	assert.Equal(t, uint64(30), msg.CurrentThreadID())

	msg2 := &Message{ID: 99}
	msg2.Normalize()
	assert.Equal(t, uint64(99), msg2.CurrentThreadID())
}

func TestMessageRootThreadID(t *testing.T) {
	msg := &Message{ID: 99, Thread: []uint64{10, 20, 30}}
	assert.Equal(t, uint64(10), msg.RootThreadID())

	msg2 := &Message{ID: 99}
	msg2.Normalize()
	assert.Equal(t, uint64(99), msg2.RootThreadID())
}

func TestMessageSessionStatePreserved(t *testing.T) {
	raw := `{"id":1,"session_state":{"cursor":42,"mode":"edit"}}`
	var msg Message
	err := json.Unmarshal([]byte(raw), &msg)
	require.NoError(t, err)
	assert.NotNil(t, msg.SessionState)

	var state map[string]interface{}
	err = json.Unmarshal(msg.SessionState, &state)
	require.NoError(t, err)
	assert.EqualValues(t, 42, state["cursor"])
	assert.Equal(t, "edit", state["mode"])
}

func TestMessageProcessStatusTyped(t *testing.T) {
	raw := `{"id":1,"process_status":"running"}`
	var msg Message
	err := json.Unmarshal([]byte(raw), &msg)
	require.NoError(t, err)
	require.NotNil(t, msg.ProcessStatus)
	assert.Equal(t, ProcessStatusRunning, *msg.ProcessStatus)
}

func TestGuildStackEntryRoundTrip(t *testing.T) {
	sagaID := "saga-1"
	entry := GuildStackEntry{GuildID: "guild-1", SagaID: &sagaID}
	b, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded GuildStackEntry
	err = json.Unmarshal(b, &decoded)
	require.NoError(t, err)
	assert.Equal(t, entry.GuildID, decoded.GuildID)
	assert.Equal(t, *entry.SagaID, *decoded.SagaID)
}
