package handlers

import (
	"context"
	"net/http"

	"github.com/bemulima/ms-go-comment/internal/domain"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"github.com/google/uuid"
)

type RealtimeService interface {
	Mint(context.Context, domain.Actor, realtimeuc.MintTicketInput) (realtimeuc.MintedTicket, error)
}

type RealtimeHandler struct {
	Service RealtimeService
}

type realtimeTicketRequest struct {
	ThreadID     uuid.UUID `json:"thread_id"`
	LastSequence *int64    `json:"last_sequence,omitempty"`
}

func (h RealtimeHandler) MintTicket(w http.ResponseWriter, r *http.Request) {
	var request realtimeTicketRequest
	if err := decodeJSON(w, r, &request); err != nil {
		WriteError(w, domain.ErrInvalidRealtimeTicket)
		return
	}
	result, err := h.Service.Mint(r.Context(), mustActor(r), realtimeuc.MintTicketInput{
		ThreadID: request.ThreadID, LastSequence: request.LastSequence,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket": result.Ticket, "expires_at": result.ExpiresAt, "protocol": result.Protocol,
	})
}
