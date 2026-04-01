package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rustic-ai/forge/forge-go/forgepath"
	"github.com/rustic-ai/forge/forge-go/modelfit"
)

type modelFitService struct {
	catalogPath          string
	dependencyConfigPath string
	profiler             modelfit.Profiler
}

func newModelFitService(catalogPath, dependencyConfigPath string, profiler modelfit.Profiler) *modelFitService {
	return &modelFitService{
		catalogPath:          catalogPath,
		dependencyConfigPath: dependencyConfigPath,
		profiler:             profiler,
	}
}

func (s *Server) handleListLocalModelFits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svc := s.modelFit
		if svc == nil {
			svc = newModelFitService(forgepath.LocalModelCatalogPath(), forgepath.DependencyConfigPath(), modelfit.DefaultProfiler{})
		}

		limit := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				ReplyError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			limit = parsed
		}

		profiles, err := modelfit.LoadProfiles(svc.catalogPath, svc.dependencyConfigPath)
		if err != nil {
			ReplyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		system, err := svc.profiler.Profile(r.Context())
		if err != nil {
			ReplyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		results := modelfit.Recommend(profiles, system, modelfit.QueryOptions{
			UseCase:      r.URL.Query().Get("use_case"),
			Limit:        limit,
			RunnableOnly: parseBoolQuery(r.URL.Query().Get("runnable_only")),
		})
		ReplyJSON(w, http.StatusOK, results)
	}
}

func (s *Server) handleGetModelFitCapabilities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svc := s.modelFit
		if svc == nil {
			svc = newModelFitService(forgepath.LocalModelCatalogPath(), forgepath.DependencyConfigPath(), modelfit.DefaultProfiler{})
		}

		system, err := svc.profiler.Profile(r.Context())
		if err != nil {
			ReplyError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ReplyJSON(w, http.StatusOK, system)
	}
}

func parseBoolQuery(raw string) bool {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
