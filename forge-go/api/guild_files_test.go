package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rustic-ai/forge/forge-go/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleFileUploadAndDownload(t *testing.T) {
	_, _, _, mux, cleanup := setupTestServer(t)
	defer cleanup()

	createReq := CreateGuildRequest{
		Spec: &protocol.GuildSpec{
			ID:          "test-guild-abc",
			Name:        "File API Test Guild",
			Description: "guild for file tests",
			Agents:      []protocol.AgentSpec{},
			Properties:  map[string]any{},
		},
		OrganizationID: "org-1",
	}
	createBody, err := json.Marshal(createReq)
	require.NoError(t, err)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/guilds", bytes.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	rrCreate := httptest.NewRecorder()
	mux.ServeHTTP(rrCreate, reqCreate)
	assert.Equal(t, http.StatusCreated, rrCreate.Code)

	// 1. Upload Test
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", "hello.txt")
	require.NoError(t, err)
	fw.Write([]byte("Hello, FileSystem!"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/guilds/test-guild-abc/files/", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// 2. List Test
	reqList := httptest.NewRequest(http.MethodGet, "/api/guilds/test-guild-abc/files/", nil)
	rrList := httptest.NewRecorder()
	mux.ServeHTTP(rrList, reqList)

	assert.Equal(t, http.StatusOK, rrList.Code)
	var files []MediaLink
	err = json.Unmarshal(rrList.Body.Bytes(), &files)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "hello.txt", files[0].Name)
	assert.Equal(t, "application/octet-stream", files[0].MimeType)

	// 3. Download Test
	reqDown := httptest.NewRequest(http.MethodGet, "/api/guilds/test-guild-abc/files/hello.txt", nil)
	rrDown := httptest.NewRecorder()
	mux.ServeHTTP(rrDown, reqDown)

	assert.Equal(t, http.StatusOK, rrDown.Code)
	assert.Contains(t, rrDown.Header().Get("Content-Type"), "text/plain")
	assert.Equal(t, "Hello, FileSystem!", rrDown.Body.String())

	// 4. Delete Test
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/guilds/test-guild-abc/files/hello.txt", nil)
	rrDel := httptest.NewRecorder()
	mux.ServeHTTP(rrDel, reqDel)

	assert.Equal(t, http.StatusNoContent, rrDel.Code)

	// Verify Deletion
	reqList2 := httptest.NewRequest(http.MethodGet, "/api/guilds/test-guild-abc/files/", nil)
	rrList2 := httptest.NewRecorder()
	mux.ServeHTTP(rrList2, reqList2)
	var filesAfter []MediaLink
	json.Unmarshal(rrList2.Body.Bytes(), &filesAfter)
	assert.Len(t, filesAfter, 0)
}
