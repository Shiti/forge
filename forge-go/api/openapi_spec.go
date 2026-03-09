package api

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"net/http"
)

var (
	//go:embed openapi/openapi.json
	embeddedOpenAPISpec []byte
	embeddedOpenAPISha  = sha256Hex(embeddedOpenAPISpec)
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Server) HandleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(embeddedOpenAPISpec)
}

func (s *Server) HandleOpenAPISha(w http.ResponseWriter, _ *http.Request) {
	ReplyJSON(w, http.StatusOK, map[string]string{"sha256": embeddedOpenAPISha})
}
