package http

import (
	"encoding/json"
	"net/http"

	"github.com/bemulima/ms-go-comment/internal/adapters/http/handlers"
	httpmw "github.com/bemulima/ms-go-comment/internal/adapters/http/middleware"
	"github.com/go-chi/chi/v5"
)

type RouterDependencies struct {
	CommentService   handlers.CommentService
	AdminService     handlers.AdminService
	RealtimeService  handlers.RealtimeService
	WebSocketHandler http.Handler
}

func NewRouter(deps RouterDependencies) http.Handler {
	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "ms-go-comment"})
	})
	if deps.CommentService != nil {
		handler := handlers.CommentHandler{Service: deps.CommentService}
		router.Route("/api/v1", func(api chi.Router) {
			api.Use(httpmw.RequireActor(handlers.WriteError))
			api.Put("/thread/ensure", handler.EnsureThread)
			api.Get("/thread/get/{threadID}", handler.GetThread)
			api.Get("/comment/list", handler.ListComments)
			api.Get("/comment/get/{commentID}", handler.GetComment)
			api.Get("/comment/changes", handler.ListChanges)
			api.Post("/comment/create", handler.CreateComment)
			api.Put("/comment/update/{commentID}", handler.UpdateComment)
			api.Delete("/comment/delete/{commentID}", handler.DeleteComment)
			api.Post("/comment-attachment/upload", handler.UploadAttachment)
			api.Get("/comment-attachment/signed-url/{attachmentID}", handler.GetAttachmentSignedURL)
			api.Delete("/comment-attachment/delete/{attachmentID}", handler.DeleteAttachment)
		})
	}
	if deps.RealtimeService != nil {
		handler := handlers.RealtimeHandler{Service: deps.RealtimeService}
		router.With(httpmw.RequireActor(handlers.WriteError)).Post("/api/v1/realtime/ticket", handler.MintTicket)
	}
	if deps.AdminService != nil {
		handler := handlers.AdminHandler{Service: deps.AdminService}
		router.Route("/admin/v1", func(admin chi.Router) {
			admin.Use(httpmw.RequireActor(handlers.WriteError))
			admin.Post("/space/create", handler.CreateSpace)
			admin.Get("/space/get/{spaceID}", handler.GetSpace)
			admin.Get("/space/list", handler.ListSpaces)
			admin.Put("/space/update/{spaceID}", handler.UpdateSpace)
			admin.Delete("/space/delete/{spaceID}", handler.DisableSpace)
			admin.Get("/thread/list", handler.ListThreads)
			admin.Put("/thread/update/{threadID}", handler.UpdateThread)
			admin.Put("/comment/hide/{commentID}", handler.HideComment)
			admin.Put("/comment/restore/{commentID}", handler.RestoreComment)
		})
	}
	if deps.WebSocketHandler != nil {
		router.Handle("/api/v1/ws", deps.WebSocketHandler)
	}
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
