package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rustic-ai/forge/forge-go/api/contract"
	"github.com/rustic-ai/forge/forge-go/guild/store"
	"github.com/rustic-ai/forge/forge-go/scheduler"
	"github.com/stretchr/testify/require"
)

func buildContractTestRouter(t *testing.T, st store.Store) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	srv := NewServer(st, nil, nil, nil, ":0")
	router := gin.New()
	router.RedirectTrailingSlash = false

	contract.RegisterHandlersWithOptions(router, srv, contract.GinServerOptions{
		ErrorHandler: func(c *gin.Context, err error, statusCode int) {
			ReplyError(c.Writer, statusCode, err.Error())
		},
	})

	return router
}

func TestContractRouterBlueprintListRoute(t *testing.T) {
	db, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)

	_, err = db.CreateBlueprint(&store.Blueprint{
		Name:        "BP One",
		Description: "desc one",
		Exposure:    store.ExposurePublic,
		AuthorID:    "author_1",
	})
	require.NoError(t, err)

	router := buildContractTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/catalog/blueprints/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp []BlueprintInfoResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp, 1)
	require.Equal(t, "BP One", resp[0].Name)
}

func TestContractRouterNoSlashPathReturnsNotFound(t *testing.T) {
	db, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)

	router := buildContractTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/catalog/categories", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestContractRouterBlueprintListNoSlashReturnsNotFound(t *testing.T) {
	db, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)

	router := buildContractTestRouter(t, db)

	req := httptest.NewRequest(http.MethodGet, "/catalog/blueprints", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestContractRouterNodeHeartbeatRouteUsesPathParam(t *testing.T) {
	orig := scheduler.GlobalNodeRegistry
	t.Cleanup(func() { scheduler.GlobalNodeRegistry = orig })
	scheduler.GlobalNodeRegistry = scheduler.NewNodeRegistry()

	db, err := store.NewGormStore("sqlite", "file::memory:")
	require.NoError(t, err)
	router := buildContractTestRouter(t, db)

	registerBody := []byte(`{"node_id":"node-1","capacity":{"cpus":2,"memory":1024,"gpus":0}}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/nodes/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusCreated, registerRec.Code)

	hbReq := httptest.NewRequest(http.MethodPost, "/nodes/node-1/heartbeat", nil)
	hbRec := httptest.NewRecorder()
	router.ServeHTTP(hbRec, hbReq)
	require.Equal(t, http.StatusOK, hbRec.Code)

	nodesReq := httptest.NewRequest(http.MethodGet, "/nodes", nil)
	nodesRec := httptest.NewRecorder()
	router.ServeHTTP(nodesRec, nodesReq)
	require.Equal(t, http.StatusOK, nodesRec.Code)

	var nodes []scheduler.NodeState
	require.NoError(t, json.NewDecoder(nodesRec.Body).Decode(&nodes))
	require.Len(t, nodes, 1)
	require.Equal(t, "node-1", nodes[0].NodeID)
}
