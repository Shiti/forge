package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/require"
)

func TestProxyMarshalOutgoingMessage_UsesDataEnvelope(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 12345
	msg.Format = "rustic_ai.core.guild.agent_ext.mixins.health.AgentsHealthReport"
	msg.Topics = protocol.TopicsFromString("user_system_notification:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{"guild_health":"ok","agents":{"a1":{"checkstatus":"ok"}}}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "healthReport", out["format"])
	require.NotNil(t, out["data"])
	require.Nil(t, out["payload"])

	data, ok := out["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ok", data["guild_health"])
}

func TestProxyMarshalOutgoingMessage_ParticipantListUsesLegacyArrayShape(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 12346
	msg.Format = "rustic_ai.core.agents.utils.user_proxy_agent.ParticipantList"
	msg.Topics = protocol.TopicsFromString("user_system_notification:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{
		"participants": [
			{"id":"a1","name":"Echo Agent","type":"bot"},
			{"id":"upa-dummyuserid","name":"dummyuserid","type":"human"}
		]
	}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "participants", out["format"])

	data, ok := out["data"].([]interface{})
	require.True(t, ok)
	require.Len(t, data, 2)

	first, ok := data[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "a1", first["id"])
}

func TestProxyMarshalOutgoingMessage_ChatCompletionResponseToMarkdown(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 67890
	msg.Format = "rustic_ai.core.guild.agent_ext.depends.llm.models.ChatCompletionResponse"
	msg.Topics = protocol.TopicsFromString("user_notifications:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{"choices":[{"message":{"content":"hello world"}}]}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "MarkdownFormat", out["format"])

	data, ok := out["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "hello world", data["text"])
}

func TestProxyNormalizeIncomingEnvelope_ChatCompletionRequest(t *testing.T) {
	in := map[string]interface{}{
		"format": "chatCompletionRequest",
		"topic":  "echo_topic",
		"data": map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"name": "hello.png",
								"url":  "http://example.com/api/guilds/g1/files/hello.png",
							},
						},
					},
				},
			},
		},
	}

	out := proxyNormalizeIncomingEnvelope(in)
	require.Equal(t, "rustic_ai.core.guild.agent_ext.depends.llm.models.ChatCompletionRequest", out["format"])
	require.Equal(t, "echo_topic", out["topics"])
	require.NotNil(t, out["payload"])

	payload, ok := out["payload"].(map[string]interface{})
	require.True(t, ok)
	messages, ok := payload["messages"].([]interface{})
	require.True(t, ok)
	first, ok := messages[0].(map[string]interface{})
	require.True(t, ok)
	content, ok := first["content"].([]interface{})
	require.True(t, ok)
	part, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	imageURL, ok := part["image_url"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "hello.png", imageURL["url"])
}

func TestProxyMarshalOutgoingMessage_ChatCompletionRequest_DataMessagesParity(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 24680
	msg.Format = "rustic_ai.core.guild.agent_ext.depends.llm.models.ChatCompletionRequest"
	msg.Topics = protocol.TopicsFromString("user_notifications:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{
		"data": {
			"messages": [
				{
					"role": "user",
					"content": [
						{
							"type": "image_url",
							"image_url": {
								"url": "http://backend/api/guilds/g1/files/hello.png"
							}
						}
					]
				}
			]
		}
	}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "ChatCompletionRequest", out["format"])

	data, ok := out["data"].(map[string]interface{})
	require.True(t, ok)
	msgs, ok := data["messages"].([]interface{})
	require.True(t, ok)
	first, ok := msgs[0].(map[string]interface{})
	require.True(t, ok)
	content, ok := first["content"].([]interface{})
	require.True(t, ok)
	part, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	imageURL, ok := part["image_url"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(
		t,
		"http://localhost:3001/rustic/api/guilds/g1/files//api/guilds/g1/files/hello.png",
		imageURL["url"],
	)
}

func TestProxyMarshalOutgoingMessage_CanvasMediaLinkRemovesLegacyURLFields(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 30001
	msg.Format = "rustic_ai.core.ui_protocol.types.CanvasFormat"
	msg.Topics = protocol.TopicsFromString("user_notifications:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{
		"component": "rustic_ai.core.ui_protocol.types.ImageFormat",
		"mediaLink": {"url": "asset.png"},
		"url": "stale-url",
		"src": "stale-src",
		"title": "Card"
	}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "CanvasFormat", out["format"])

	data, ok := out["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ImageFormat", data["component"])
	require.Equal(t, "Card", data["title"])
	require.Equal(t, "http://localhost:3001/rustic/api/guilds/g1/files/asset.png", data["src"])
	_, hasURL := data["url"]
	require.False(t, hasURL)
	_, hasMediaLink := data["mediaLink"]
	require.False(t, hasMediaLink)
}

func TestProxyMarshalOutgoingMessage_VegaLitePreservesQualifiedFormat(t *testing.T) {
	msg := protocol.NewMessage()
	msg.ID = 30002
	msg.Format = "rustic_ai.core.ui_protocol.types.VegaLiteFormat"
	msg.Topics = protocol.TopicsFromString("user_notifications:dummyuserid")
	msg.Priority = int(protocol.PriorityNormal)
	msg.Payload = json.RawMessage(`{"spec":{"data":{"url":"chart.csv"}}}`)

	raw, err := proxyMarshalOutgoingMessage(msg, "g1", "http://localhost:3001")
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Equal(t, "rustic_ai.core.ui_protocol.types.VegaLiteFormat", out["format"])
	data, ok := out["data"].(map[string]interface{})
	require.True(t, ok)
	spec, ok := data["spec"].(map[string]interface{})
	require.True(t, ok)
	d, ok := spec["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "http://localhost:3001/rustic/api/guilds/g1/files/chart.csv", d["url"])
}

func TestProxyBaseOrigin_EnvOverrideThenFallbackHost(t *testing.T) {
	t.Setenv("FORGE_RUSTIC_THIS_SERVER", "https://proxy.example.net/rustic")
	req := httptest.NewRequest("GET", "http://localhost/rustic/ws/abc/usercomms", nil)
	req.Host = "localhost:3001"
	require.Equal(t, "https://proxy.example.net", ProxyBaseOrigin(req))

	t.Setenv("FORGE_RUSTIC_THIS_SERVER", " ")
	require.Equal(t, "http://localhost:3001", ProxyBaseOrigin(req))
}
