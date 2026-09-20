package http

import (
	"encoding/json"
	"net/http"

	adminhttp "github.com/bemulima/ms-go-comment/internal/transport/http/admin"
	apihttp "github.com/bemulima/ms-go-comment/internal/transport/http/api"
	"github.com/bemulima/ms-go-comment/internal/transport/http/handlers"
	httpmw "github.com/bemulima/ms-go-comment/internal/transport/http/middleware"
	internalhttp "github.com/bemulima/ms-go-comment/internal/transport/http/private"
	"github.com/go-chi/chi/v5"
)

type RouterDependencies struct {
	CommentService   handlers.CommentService
	AdminService     handlers.AdminService
	RealtimeService  handlers.RealtimeService
	InternalService  handlers.InternalService
	InternalToken    string
	UserRateLimiter  httpmw.ActorLimiter
	WebSocketHandler http.Handler
}

func NewRouter(deps RouterDependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(httpmw.SecurityHeaders)
	router.Use(httpmw.RequestID)
	router.Use(httpmw.RecoverPanics(handlers.WriteError))
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "ms-go-comment"})
	})
	router.Mount("/api/v1", apihttp.NewRouter(apihttp.RouterDependencies{
		CommentService: deps.CommentService, RealtimeService: deps.RealtimeService,
		UserRateLimiter: deps.UserRateLimiter, WebSocketHandler: deps.WebSocketHandler,
	}))
	if deps.InternalService != nil {
		router.Mount("/internal/v1", internalhttp.NewRouter(deps.InternalService, deps.InternalToken))
	}
	router.Mount("/admin/v1", adminhttp.NewRouter(deps.AdminService))
	router.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "route not found"})
	})
	return router
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
