package admin

import (
	"github.com/bemulima/ms-go-comment/internal/transport/http/handlers"
	httpmw "github.com/bemulima/ms-go-comment/internal/transport/http/middleware"
	"github.com/go-chi/chi/v5"
)

func NewRouter(service handlers.AdminService) chi.Router {
	admin := chi.NewRouter()
	if service == nil {
		return admin
	}
	handler := handlers.AdminHandler{Service: service}
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
	return admin
}
