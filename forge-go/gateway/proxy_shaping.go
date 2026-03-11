package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

var proxyFormatAliases = map[string]string{
	"healthcheck":           "rustic_ai.core.guild.agent_ext.mixins.health.HealthCheckRequest",
	"questionResponse":      "rustic_ai.core.ui_protocol.types.QuestionResponse",
	"formResponse":          "rustic_ai.core.ui_protocol.types.FormResponse",
	"participantsRequest":   "rustic_ai.core.agents.utils.user_proxy_agent.ParticipantListRequest",
	"chatCompletionRequest": "rustic_ai.core.guild.agent_ext.depends.llm.models.ChatCompletionRequest",
	"stopGuildRequest":      "rustic_ai.core.agents.system.models.StopGuildRequest",
}

var proxyExtractAPIPathRe = regexp.MustCompile(`^.*?(/api.*)$`)

func proxyNormalizeIncomingFormat(format string) string {
	if mapped, ok := proxyFormatAliases[format]; ok {
		return mapped
	}
	return format
}

func proxyNormalizeIncomingEnvelope(msg map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range msg {
		out[k] = v
	}

	if v := coalesceMapValue(out, "topic", "topics"); v != nil {
		out["topics"] = v
	}
	if v := coalesceMapValue(out, "data", "payload"); v != nil {
		out["payload"] = v
	}
	if v := coalesceMapValue(out, "threads", "thread"); v != nil {
		out["thread"] = v
	}
	if v := coalesceMapValue(out, "messageHistory", "message_history"); v != nil {
		out["message_history"] = v
	}
	if v := coalesceMapValue(out, "conversationId", "conversation_id"); v != nil {
		out["conversation_id"] = v
	}
	if v := coalesceMapValue(out, "inReplyTo", "in_response_to"); v != nil {
		out["in_response_to"] = v
	}
	if v := coalesceMapValue(out, "recipientList", "recipient_list"); v != nil {
		out["recipient_list"] = v
	}
	if v := coalesceMapValue(out, "processStatus", "process_status"); v != nil {
		out["process_status"] = v
	}
	if format, ok := out["format"].(string); ok {
		out["format"] = proxyNormalizeIncomingFormat(format)
	}

	if format, _ := out["format"].(string); strings.HasSuffix(format, "ChatCompletionRequest") {
		if payload, ok := out["payload"].(map[string]interface{}); ok {
			if messages, ok := payload["messages"].([]interface{}); ok {
				payload["messages"] = transformChatCompletionForBackend(messages)
				out["payload"] = payload
			}
		}
	}
	return out
}

func transformChatCompletionForBackend(messages []interface{}) []interface{} {
	out := make([]interface{}, 0, len(messages))
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			out = append(out, m)
			continue
		}
		contentArr, ok := msg["content"].([]interface{})
		if !ok {
			out = append(out, msg)
			continue
		}

		nextContent := make([]interface{}, 0, len(contentArr))
		for _, item := range contentArr {
			part, ok := item.(map[string]interface{})
			if !ok {
				nextContent = append(nextContent, item)
				continue
			}
			partType, _ := part["type"].(string)
			if !strings.HasSuffix(partType, "_url") {
				nextContent = append(nextContent, part)
				continue
			}
			raw, ok := part[partType].(map[string]interface{})
			if !ok {
				nextContent = append(nextContent, part)
				continue
			}
			// Proxy behavior: set url to name field (may become nil if name missing in JS).
			raw["url"] = raw["name"]
			part[partType] = raw
			nextContent = append(nextContent, part)
		}
		msg["content"] = nextContent
		out = append(out, msg)
	}
	return out
}

// Proxy parity: only http/https are treated as already-absolute URLs.
func isAbsoluteURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func proxyGuildFileURL(baseOrigin, guildID, filename string) string {
	if isAbsoluteURL(filename) {
		return filename
	}
	filePath := "/rustic/api/guilds/" + url.PathEscape(guildID) + "/files/" + url.PathEscape(filename)
	return strings.TrimRight(baseOrigin, "/") + filePath
}

func proxyFormatType(format string) string {
	if strings.Contains(format, ".") {
		parts := strings.Split(format, ".")
		return parts[len(parts)-1]
	}
	return format
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func omitKeys(src map[string]interface{}, keys ...string) map[string]interface{} {
	out := cloneMap(src)
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

func proxyTransformFilesWithText(data map[string]interface{}, guildID, baseOrigin string) map[string]interface{} {
	out := map[string]interface{}{"text": data["text"]}
	rawFiles, _ := data["files"].([]interface{})
	files := make([]interface{}, 0, len(rawFiles))
	for _, f := range rawFiles {
		fileData, ok := f.(map[string]interface{})
		if !ok {
			files = append(files, f)
			continue
		}
		mediaLink, ok := fileData["mediaLink"].(map[string]interface{})
		if !ok {
			files = append(files, fileData)
			continue
		}
		fileName, _ := mediaLink["name"].(string)
		fileURL, _ := mediaLink["url"].(string)
		if fileURL == "" {
			fileURL = fileName
		}
		files = append(files, map[string]interface{}{
			"name": fileName,
			"url":  proxyGuildFileURL(baseOrigin, guildID, fileURL),
		})
	}
	out["files"] = files
	return out
}

func proxyTransformChatCompletionResponse(data map[string]interface{}) map[string]interface{} {
	choices, _ := data["choices"].([]interface{})
	if len(choices) == 0 {
		return map[string]interface{}{"text": ""}
	}
	c0, _ := choices[0].(map[string]interface{})
	msg, _ := c0["message"].(map[string]interface{})
	return map[string]interface{}{"text": msg["content"]}
}

// Proxy parity for backend->UI ChatCompletionRequest path.
// Mirrors proxy quirks: expects response.data.messages and rewrites nested *_url values
// by concatenating baseOrigin + /rustic/api/guilds/{guild}/files/ + extractedPath.
func proxyTransformChatCompletionRequest(response map[string]interface{}, guildID, baseOrigin string) ([]interface{}, bool) {
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	msgsRaw, ok := data["messages"].([]interface{})
	if !ok {
		return nil, false
	}

	outMsgs := make([]interface{}, 0, len(msgsRaw))
	for _, m := range msgsRaw {
		msg, ok := m.(map[string]interface{})
		if !ok {
			outMsgs = append(outMsgs, m)
			continue
		}
		if content, ok := msg["content"].(string); ok {
			msg["content"] = content
			outMsgs = append(outMsgs, msg)
			continue
		}
		contentArr, ok := msg["content"].([]interface{})
		if !ok {
			outMsgs = append(outMsgs, msg)
			continue
		}
		next := make([]interface{}, 0, len(contentArr))
		for _, item := range contentArr {
			part, ok := item.(map[string]interface{})
			if !ok {
				next = append(next, item)
				continue
			}
			partType, _ := part["type"].(string)
			partData, ok := part[partType].(map[string]interface{})
			if !ok || !strings.HasSuffix(partType, "_url") {
				next = append(next, part)
				continue
			}
			linkURL, _ := partData["url"].(string)
			extractedPath := proxyExtractAPIPathRe.ReplaceAllString(linkURL, "$1")
			partData["url"] = strings.TrimRight(baseOrigin, "/") +
				"/rustic/api/guilds/" + guildID + "/files/" + extractedPath
			part[partType] = partData
			next = append(next, part)
		}
		msg["content"] = next
		outMsgs = append(outMsgs, msg)
	}
	return outMsgs, true
}

func proxyTransformVegaLite(data map[string]interface{}, guildID, baseOrigin string) map[string]interface{} {
	result := cloneMap(data)
	spec, ok := result["spec"].(map[string]interface{})
	if !ok {
		return result
	}
	spec = cloneMap(spec)
	d, ok := spec["data"].(map[string]interface{})
	if !ok {
		result["spec"] = spec
		return result
	}
	d = cloneMap(d)
	rawURL, _ := d["url"].(string)
	if rawURL != "" {
		d["url"] = proxyGuildFileURL(baseOrigin, guildID, rawURL)
		spec["data"] = d
		result["spec"] = spec
	}
	return result
}

func proxyTransformAnomaly(data map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"data": data["relevant_entries"],
		"config": map[string]interface{}{
			"dateColumns": []string{"Date"},
		},
		"description": fmt.Sprintf(
			`<div style="text-align:center">%v/%v</div><div>%v</div>`,
			data["index"], data["total"], data["reason"],
		),
	}
	reason, _ := data["reason"].(string)
	if strings.HasPrefix(reason, "Wages") {
		result["data"] = data["relevant_entries"]
		cfg := result["config"].(map[string]interface{})
		cfg["groupBy"] = []string{"period", "#"}
		cfg["expansionDepth"] = 0
		result["config"] = cfg
	} else if strings.HasPrefix(reason, "Amount varies") {
		result["data"] = data["journal_entries"]
		if incorrect, ok := data["incorrect_entry"].(map[string]interface{}); ok {
			result["description"] = result["description"].(string) + " for journal entry " + fmt.Sprint(incorrect["#"])
		}
	}
	return result
}

func proxyApplyAdditionalTransforms(guildID, baseOrigin, format string, raw interface{}) (string, interface{}) {
	msgData, _ := raw.(map[string]interface{})
	if msgData == nil {
		msgData = map[string]interface{}{}
	}

	transformedFormat := format
	transformedData := interface{}(msgData)

	formatType := proxyFormatType(format)
	switch formatType {
	case "FilesWithTextFormat":
		transformedFormat = "FilesWithTextFormat"
		transformedData = proxyTransformFilesWithText(msgData, guildID, baseOrigin)
	case "ChatCompletionResponse":
		transformedFormat = "MarkdownFormat"
		transformedData = proxyTransformChatCompletionResponse(msgData)
	case "VegaLiteFormat":
		// Proxy preserves original format string here.
		transformedData = proxyTransformVegaLite(msgData, guildID, baseOrigin)
	case "ChatCompletionRequest":
		transformedFormat = "ChatCompletionRequest"
		next := cloneMap(msgData)
		if msgs, ok := proxyTransformChatCompletionRequest(msgData, guildID, baseOrigin); ok {
			next["messages"] = msgs
		}
		transformedData = next
	case "ImageFormat", "AudioFormat", "VideoFormat":
		if mediaLink, ok := msgData["mediaLink"].(map[string]interface{}); ok {
			dataWithoutMediaLink := omitKeys(msgData, "mediaLink")
			urlRaw, _ := mediaLink["url"].(string)
			dataWithoutMediaLink["src"] = proxyGuildFileURL(baseOrigin, guildID, urlRaw)
			transformedData = dataWithoutMediaLink
		} else {
			dataWithoutSrc := omitKeys(msgData, "src")
			if src, ok := msgData["src"].(string); ok && src != "" {
				dataWithoutSrc["src"] = proxyGuildFileURL(baseOrigin, guildID, src)
			}
			transformedData = dataWithoutSrc
		}
		transformedFormat = formatType
	case "CanvasFormat", "GoalFormat":
		transformedFormat = formatType
		dataWithoutMediaLink := omitKeys(msgData, "mediaLink", "url", "src")
		componentType := proxyFormatType(fmt.Sprint(msgData["component"]))

		if mediaLink, ok := msgData["mediaLink"].(map[string]interface{}); ok {
			urlRaw, _ := mediaLink["url"].(string)
			if componentType == "ImageFormat" || componentType == "AudioFormat" || componentType == "VideoFormat" {
				dataWithoutMediaLink["src"] = proxyGuildFileURL(baseOrigin, guildID, urlRaw)
			} else {
				dataWithoutMediaLink["url"] = proxyGuildFileURL(baseOrigin, guildID, urlRaw)
			}
			dataWithoutMediaLink["component"] = componentType
			transformedData = dataWithoutMediaLink
		} else if componentType == "VegaLiteFormat" {
			vega := proxyTransformVegaLite(dataWithoutMediaLink, guildID, baseOrigin)
			vega["component"] = componentType
			transformedData = vega
		} else {
			if rawURL, ok := msgData["url"].(string); ok && rawURL != "" {
				dataWithoutMediaLink["url"] = proxyGuildFileURL(baseOrigin, guildID, rawURL)
			}
			if src, ok := msgData["src"].(string); ok && src != "" {
				dataWithoutMediaLink["src"] = proxyGuildFileURL(baseOrigin, guildID, src)
			}
			dataWithoutMediaLink["component"] = componentType
			transformedData = dataWithoutMediaLink
		}
	case "Anomaly":
		transformedFormat = "PerspectiveFormat"
		transformedData = proxyTransformAnomaly(msgData)
	case "AgentsHealthReport":
		transformedFormat = "healthReport"
	case "StopGuildResponse":
		transformedFormat = "stoppingChat"
	case "ParticipantList":
		transformedFormat = "participants"
		transformedData = msgData["participants"]
	default:
		transformedFormat = formatType
	}

	return transformedFormat, transformedData
}

func proxyPriorityName(priority int) string {
	switch priority {
	case int(protocol.PriorityUrgent):
		return "URGENT"
	case int(protocol.PriorityImportant):
		return "IMPORTANT"
	case int(protocol.PriorityHigh):
		return "HIGH"
	case int(protocol.PriorityAboveNormal):
		return "ABOVE_NORMAL"
	case int(protocol.PriorityNormal):
		return "NORMAL"
	case int(protocol.PriorityLow):
		return "LOW"
	case int(protocol.PriorityVeryLow):
		return "VERY_LOW"
	case int(protocol.PriorityLowest):
		return "LOWEST"
	default:
		return "NORMAL"
	}
}

func proxyFormatTimestamp(ms float64) string {
	if ms <= 0 {
		return time.Now().Local().Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
	}
	return time.UnixMilli(int64(ms)).Local().Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
}

func proxyServerOriginFromEnv() string {
	raw := strings.TrimSpace(os.Getenv("FORGE_RUSTIC_THIS_SERVER"))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func proxyBaseOrigin(req *http.Request) string {
	if configured := proxyServerOriginFromEnv(); configured != "" {
		return configured
	}
	scheme := "http"
	host := ""
	if req != nil {
		if req.TLS != nil {
			scheme = "https"
		}
		host = req.Host
	}
	if strings.TrimSpace(host) == "" {
		return scheme + "://localhost"
	}
	return scheme + "://" + host
}

func proxyMarshalOutgoingMessage(msg protocol.Message, guildID, baseOrigin string) ([]byte, error) {
	var payload interface{}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			payload = map[string]interface{}{}
		}
	} else {
		payload = map[string]interface{}{}
	}

	format, data := proxyApplyAdditionalTransforms(guildID, baseOrigin, msg.Format, payload)
	threads := make([]string, 0, len(msg.Thread))
	for _, t := range msg.Thread {
		threads = append(threads, strconv.FormatUint(t, 10))
	}

	out := map[string]interface{}{
		"id":             strconv.FormatUint(msg.ID, 10),
		"timestamp":      proxyFormatTimestamp(msg.Timestamp),
		"sender":         msg.Sender,
		"conversationId": "default",
		"format":         format,
		"data":           data,
		"priority":       proxyPriorityName(msg.Priority),
		"topic":          msg.Topics.String(),
		"threads":        threads,
		"messageHistory": msg.MessageHistory,
	}
	if msg.InResponseTo != nil {
		out["inReplyTo"] = strconv.FormatUint(*msg.InResponseTo, 10)
	}
	if msg.ProcessStatus != nil {
		out["processStatus"] = string(*msg.ProcessStatus)
	}
	if msg.ForwardHeader != nil {
		out["sender"] = msg.ForwardHeader.OnBehalfOf
	}
	return json.Marshal(out)
}

// ProxyBaseOrigin returns the rustic proxy-style base origin:
// configured FORGE_RUSTIC_THIS_SERVER origin, otherwise request-derived origin.
func ProxyBaseOrigin(req *http.Request) string {
	return proxyBaseOrigin(req)
}

// ProxyRewriteGuildFileURL mirrors proxy getGuildFileUrl behavior.
func ProxyRewriteGuildFileURL(req *http.Request, guildID, filename string) string {
	return proxyGuildFileURL(proxyBaseOrigin(req), guildID, filename)
}

// ProxyMarshalOutgoingMessage marshals protocol messages into proxy-compatible websocket envelope.
func ProxyMarshalOutgoingMessage(msg protocol.Message, guildID string, req *http.Request) ([]byte, error) {
	return proxyMarshalOutgoingMessage(msg, guildID, proxyBaseOrigin(req))
}
