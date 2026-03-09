package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleOpenAPI(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	srv.HandleOpenAPI(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.JSONEq(t, string(embeddedOpenAPISpec), rec.Body.String())
}

func TestHandleOpenAPISha(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/openapi.sha256", nil)
	rec := httptest.NewRecorder()

	srv.HandleOpenAPISha(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, embeddedOpenAPISha, payload["sha256"])
}
