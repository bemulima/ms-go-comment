package internalhttp

import (
	"github.com/bemulima/ms-go-comment/internal/adapters/http/handlers"
	httpmw "github.com/bemulima/ms-go-comment/internal/adapters/http/middleware"
	"github.com/go-chi/chi/v5"
)

func NewRouter(service handlers.InternalService, token string) chi.Router {
	router := chi.NewRouter()
	router.Use(httpmw.RequireInternalToken(token, handlers.WriteError))
	handler := handlers.InternalHandler{Service: service}
	router.Post("/access-grant/create", handler.CreateAccessGrant)
	router.Post("/thread/ensure", handler.EnsureThread)
	router.Get("/thread/get-by-resource", handler.GetThreadByResource)
	return router
}
