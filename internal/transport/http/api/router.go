package api

import (
	"net/http"

	"github.com/bemulima/ms-go-comment/internal/transport/http/handlers"
	httpmw "github.com/bemulima/ms-go-comment/internal/transport/http/middleware"
	"github.com/go-chi/chi/v5"
)

type RouterDependencies struct {
	CommentService   handlers.CommentService
	RealtimeService  handlers.RealtimeService
	UserRateLimiter  httpmw.ActorLimiter
	WebSocketHandler http.Handler
}

func NewRouter(deps RouterDependencies) chi.Router {
	router := chi.NewRouter()
	if deps.CommentService != nil {
		handler := handlers.CommentHandler{Service: deps.CommentService}
		router.Group(func(api chi.Router) {
			api.Use(httpmw.RequireActor(handlers.WriteError))
			api.Use(httpmw.CaptureAccessGrant)
			if deps.UserRateLimiter != nil {
				api.Use(httpmw.RateLimitActor(deps.UserRateLimiter, handlers.WriteError))
			}
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
		middlewares := []func(http.Handler) http.Handler{
			httpmw.RequireActor(handlers.WriteError), httpmw.CaptureAccessGrant,
		}
		if deps.UserRateLimiter != nil {
			middlewares = append(middlewares, httpmw.RateLimitActor(deps.UserRateLimiter, handlers.WriteError))
		}
		router.With(middlewares...).Post("/realtime/ticket", handler.MintTicket)
	}
	if deps.WebSocketHandler != nil {
		router.Handle("/ws", deps.WebSocketHandler)
	}
	return router
}
