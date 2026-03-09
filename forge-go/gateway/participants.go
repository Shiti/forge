package gateway

import "encoding/json"

func ensureHumanParticipant(payload json.RawMessage, userID string) json.RawMessage {
	if len(payload) == 0 {
		return payload
	}
	var body map[string]interface{}
	if err := json.Unmarshal(payload, &body); err != nil {
		return payload
	}
	rawParticipants, ok := body["participants"]
	if !ok {
		return payload
	}
	participants, ok := rawParticipants.([]interface{})
	if !ok {
		return payload
	}
	hasHuman := false
	for _, p := range participants {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := pm["type"].(string); ok && t == "human" {
			hasHuman = true
			break
		}
	}
	if hasHuman {
		return payload
	}
	participants = append(participants, map[string]interface{}{
		"id":   "upa-" + userID,
		"name": userID,
		"type": "human",
	})
	body["participants"] = participants
	normalized, err := json.Marshal(body)
	if err != nil {
		return payload
	}
	return normalized
}
