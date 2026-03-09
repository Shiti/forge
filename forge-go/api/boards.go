package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rustic-ai/forge/forge-go/guild/store"
)

//nolint:unused // kept for stdlib mux wiring parity with non-gin API surfaces.
func (s *Server) registerBoardRoutes(mux *http.ServeMux) {
	mux.Handle("POST /addons/boards", WithTelemetry("boards.create", http.HandlerFunc(s.HandleCreateBoard)))
	mux.Handle("GET /addons/boards", WithTelemetry("boards.list", http.HandlerFunc(s.HandleGetBoards)))
	mux.Handle("POST /addons/boards/{board_id}/messages", WithTelemetry("boards.messages.add", http.HandlerFunc(s.HandleAddMessageToBoard)))
	mux.Handle("GET /addons/boards/{board_id}/messages", WithTelemetry("boards.messages.list", http.HandlerFunc(s.HandleGetBoardMessageIDs)))
	mux.Handle("DELETE /addons/boards/{board_id}/messages/{message_id}", WithTelemetry("boards.messages.delete", http.HandlerFunc(s.HandleRemoveMessageFromBoard)))
}

type createBoardRequest struct {
	GuildID   string `json:"guild_id"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
	IsDefault bool   `json:"is_default"`
	IsPrivate bool   `json:"is_private"`
}

func (s *Server) HandleCreateBoard(w http.ResponseWriter, r *http.Request) {
	var req createBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ReplyError(w, http.StatusUnprocessableEntity, "invalid request body")
		return
	}
	if req.GuildID == "" || req.Name == "" || req.CreatedBy == "" {
		ReplyError(w, http.StatusUnprocessableEntity, "guild_id, name, and created_by are required")
		return
	}
	if _, err := s.store.GetGuild(req.GuildID); err != nil {
		ReplyError(w, http.StatusNotFound, "Guild not found")
		return
	}

	board := &store.Board{
		GuildID:   req.GuildID,
		Name:      req.Name,
		CreatedBy: req.CreatedBy,
		IsDefault: req.IsDefault,
		IsPrivate: req.IsPrivate,
	}
	if err := s.store.CreateBoard(board); err != nil {
		ReplyError(w, http.StatusInternalServerError, "failed to create board")
		return
	}

	ReplyJSON(w, http.StatusCreated, map[string]string{"id": board.ID})
}

func (s *Server) HandleGetBoards(w http.ResponseWriter, r *http.Request) {
	guildID := r.URL.Query().Get("guild_id")
	if guildID == "" {
		ReplyError(w, http.StatusUnprocessableEntity, "guild_id is required")
		return
	}
	if _, err := s.store.GetGuild(guildID); err != nil {
		ReplyError(w, http.StatusNotFound, "Guild not found")
		return
	}

	boards, err := s.store.GetBoardsByGuild(guildID)
	if err != nil {
		ReplyError(w, http.StatusInternalServerError, "failed to list boards")
		return
	}
	ReplyJSON(w, http.StatusOK, map[string]interface{}{"boards": boards})
}

type addMessageRequest struct {
	MessageID string `json:"message_id"`
}

func (s *Server) HandleAddMessageToBoard(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("board_id")
	var req addMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ReplyError(w, http.StatusUnprocessableEntity, "invalid request body")
		return
	}
	if req.MessageID == "" {
		ReplyError(w, http.StatusUnprocessableEntity, "message_id is required")
		return
	}

	err := s.store.AddMessageToBoard(boardID, req.MessageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ReplyError(w, http.StatusNotFound, "Board not found")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			ReplyError(w, http.StatusConflict, "Message already added to board")
			return
		}
		ReplyError(w, http.StatusInternalServerError, "failed to add message to board")
		return
	}
	ReplyJSON(w, http.StatusOK, map[string]string{"message": "Message added to the board successfully"})
}

func (s *Server) HandleGetBoardMessageIDs(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("board_id")
	ids, err := s.store.GetBoardMessageIDs(boardID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ReplyError(w, http.StatusNotFound, "Board not found")
			return
		}
		ReplyError(w, http.StatusInternalServerError, "failed to get board messages")
		return
	}
	ReplyJSON(w, http.StatusOK, map[string]interface{}{"ids": ids})
}

func (s *Server) HandleRemoveMessageFromBoard(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("board_id")
	messageID := r.PathValue("message_id")
	err := s.store.RemoveMessageFromBoard(boardID, messageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ReplyError(w, http.StatusNotFound, "Board not found")
			return
		}
		ReplyError(w, http.StatusInternalServerError, "failed to remove message from board")
		return
	}
	ReplyJSON(w, http.StatusOK, map[string]string{"message": "Message removed from the board successfully"})
}
