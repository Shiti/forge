package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWrapHTTPWithPathValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/ws/guilds/:id/usercomms/:user_id/:user_name", wrapHTTPWithPathValues(func(w http.ResponseWriter, r *http.Request) {
		ReplyJSON(w, http.StatusOK, map[string]string{
			"id":        r.PathValue("id"),
			"user_id":   r.PathValue("user_id"),
			"user_name": r.PathValue("user_name"),
		})
	}, "id", "user_id", "user_name"))

	req := httptest.NewRequest(http.MethodGet, "/ws/guilds/guild-1/usercomms/user-1/Alice", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "guild-1", got["id"])
	require.Equal(t, "user-1", got["user_id"])
	require.Equal(t, "Alice", got["user_name"])
}
