package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

func TestActorRateLimiter_RefillsAndBoundsActors(t *testing.T) {
	t.Parallel()

	clock := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	limiter, err := newActorRateLimiter(1, 2, 2, 10*time.Second, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("newActorRateLimiter() error = %v", err)
	}
	first, second, third := uuid.New(), uuid.New(), uuid.New()
	firstAllowed := limiter.Allow(first)
	secondAllowed := limiter.Allow(first)
	thirdAllowed := limiter.Allow(first)
	if !firstAllowed || !secondAllowed || thirdAllowed {
		t.Fatal("burst was not enforced")
	}
	clock = clock.Add(time.Second)
	if !limiter.Allow(first) || !limiter.Allow(second) {
		t.Fatal("refill or second actor was rejected")
	}
	if limiter.Allow(third) {
		t.Fatal("actor map exceeded its configured maximum")
	}
	clock = clock.Add(11 * time.Second)
	allowed := limiter.Allow(third)
	if !allowed || len(limiter.items) != 1 {
		t.Fatalf("idle eviction failed: allowed=%v items=%d", allowed, len(limiter.items))
	}
}

func TestNewActorRateLimiter_RejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewActorRateLimiter(0, 1, 1, time.Minute); err == nil {
		t.Fatal("zero rate was accepted")
	}
	if _, err := NewActorRateLimiter(1, 1, 1, 0); err == nil {
		t.Fatal("zero idle expiry was accepted")
	}
}

func TestRateLimitActor(t *testing.T) {
	t.Parallel()

	actor := domain.Actor{UserID: uuid.New(), Role: "STUDENT"}
	called := false
	handler := RateLimitActor(staticActorLimiter(false), func(w http.ResponseWriter, err error) {
		if err != domain.ErrRateLimited {
			t.Fatalf("error = %v", err)
		}
		w.WriteHeader(http.StatusTooManyRequests)
	})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), actorKey{}, actor))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || called {
		t.Fatalf("status=%d retry=%q called=%v", response.Code, response.Header().Get("Retry-After"), called)
	}
}

func TestRecoverPanics_PreservesAbortHandler(t *testing.T) {
	t.Parallel()

	preserved := false
	func() {
		defer func() { preserved = recover() == http.ErrAbortHandler }()
		handler := RecoverPanics(func(http.ResponseWriter, error) {
			t.Fatal("abort handler panic was converted to an HTTP response")
		})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	if !preserved {
		t.Fatal("http.ErrAbortHandler was not preserved")
	}
}

type staticActorLimiter bool

func (value staticActorLimiter) Allow(uuid.UUID) bool { return bool(value) }
