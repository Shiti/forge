package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rustic-ai/forge/forge-go/gateway"
	"github.com/rustic-ai/forge/forge-go/protocol"
)

func isRusticPath(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/rustic/")
}

func writeCapturedResponse(c *gin.Context, rr *httptest.ResponseRecorder) {
	for k, vals := range rr.Header() {
		for _, v := range vals {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Status(rr.Code)
	_, _ = c.Writer.Write(rr.Body.Bytes())
}

func (s *Server) dispatchRusticHistoricalMessages(c *gin.Context, guildID, userID string) {
	req := c.Request.Clone(c.Request.Context())
	req.SetPathValue("id", guildID)
	req.SetPathValue("user_id", userID)

	rr := httptest.NewRecorder()
	s.HandleGetHistoricalMessages(rr, req)
	if rr.Code != http.StatusOK {
		writeCapturedResponse(c, rr)
		return
	}

	var msgs []protocol.Message
	if err := json.Unmarshal(rr.Body.Bytes(), &msgs); err != nil {
		ReplyError(c.Writer, http.StatusInternalServerError, "failed to decode message list")
		return
	}

	out := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, transformRusticMessage(m, guildID, c.Request))
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) dispatchRusticGuildFiles(c *gin.Context, guildID string) {
	req := c.Request.Clone(c.Request.Context())
	req.SetPathValue("id", guildID)

	rr := httptest.NewRecorder()
	s.HandleFileList(rr, req)
	if rr.Code != http.StatusOK {
		writeCapturedResponse(c, rr)
		return
	}

	var links []MediaLink
	if err := json.Unmarshal(rr.Body.Bytes(), &links); err != nil {
		ReplyError(c.Writer, http.StatusInternalServerError, "failed to decode file list")
		return
	}

	resp := make([]map[string]interface{}, 0, len(links))
	for _, l := range links {
		resp = append(resp, map[string]interface{}{
			"name":     l.Name,
			"metadata": l.Metadata,
			"url":      gateway.ProxyRewriteGuildFileURL(c.Request, guildID, l.Name),
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) dispatchRusticGuildFileUpload(c *gin.Context, guildID string) {
	req := c.Request.Clone(c.Request.Context())
	req.SetPathValue("id", guildID)

	rr := httptest.NewRecorder()
	s.HandleFileUpload(rr, req)
	if rr.Code != http.StatusOK {
		writeCapturedResponse(c, rr)
		return
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		writeCapturedResponse(c, rr)
		return
	}
	if raw, ok := body["url"].(string); ok {
		body["url"] = strings.TrimRight(gateway.ProxyBaseOrigin(c.Request), "/") + raw
	}
	c.JSON(http.StatusOK, body)
}

func transformRusticMessage(msg protocol.Message, guildID string, req *http.Request) map[string]interface{} {
	raw, err := gateway.ProxyMarshalOutgoingMessage(msg, guildID, req)
	if err != nil {
		fallback := map[string]interface{}{
			"id":             strconv.FormatUint(msg.ID, 10),
			"conversationId": "default",
			"format":         msg.Format,
			"data":           map[string]interface{}{},
		}
		return fallback
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{
			"id":             strconv.FormatUint(msg.ID, 10),
			"conversationId": "default",
			"format":         msg.Format,
			"data":           map[string]interface{}{},
		}
	}
	return out
}
