package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bemulima/ms-go-comment/internal/domain"
	"github.com/google/uuid"
)

type requestIDKey struct{}

type ActorLimiter interface {
	Allow(uuid.UUID) bool
}

type ActorRateLimiter struct {
	mu         sync.Mutex
	items      map[uuid.UUID]actorBucket
	rate       float64
	burst      float64
	maximum    int
	idleExpiry time.Duration
	now        func() time.Time
}

type actorBucket struct {
	tokens float64
	last   time.Time
	seen   time.Time
}

func NewActorRateLimiter(rate, burst, maximum int, idleExpiry time.Duration) (*ActorRateLimiter, error) {
	return newActorRateLimiter(rate, burst, maximum, idleExpiry, time.Now)
}

func newActorRateLimiter(rate, burst, maximum int, idleExpiry time.Duration, now func() time.Time) (*ActorRateLimiter, error) {
	if rate < 1 || burst < 1 || maximum < 1 || idleExpiry < time.Second || now == nil {
		return nil, fmt.Errorf("actor rate limiter values must be positive")
	}
	return &ActorRateLimiter{
		items: make(map[uuid.UUID]actorBucket), rate: float64(rate), burst: float64(burst),
		maximum: maximum, idleExpiry: idleExpiry, now: now,
	}, nil
}

func (l *ActorRateLimiter) Allow(actorID uuid.UUID) bool {
	if l == nil || actorID == uuid.Nil {
		return false
	}
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.items[actorID]
	if !exists {
		if len(l.items) >= l.maximum {
			l.evictIdle(now)
		}
		if len(l.items) >= l.maximum {
			return false
		}
		bucket = actorBucket{tokens: l.burst, last: now, seen: now}
	}
	if now.Before(bucket.last) {
		now = bucket.last
	}
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens = min(l.burst, bucket.tokens+elapsed*l.rate)
	}
	bucket.last = now
	bucket.seen = now
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}
	l.items[actorID] = bucket
	return allowed
}

func (l *ActorRateLimiter) evictIdle(now time.Time) {
	threshold := now.Add(-l.idleExpiry)
	for actorID, bucket := range l.items {
		if !bucket.seen.After(threshold) {
			delete(l.items, actorID)
		}
	}
}

func RateLimitActor(limiter ActorLimiter, writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := ActorFromContext(r.Context())
			if !ok {
				writeError(w, domain.ErrAuthenticationRequired)
				return
			}
			if limiter == nil || !limiter.Allow(actor.UserID) {
				w.Header().Set("Retry-After", "1")
				writeError(w, domain.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
	})
}

func RequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDKey{}).(string)
	return requestID, ok && requestID != ""
}

func RecoverPanics(writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					requestID, _ := RequestIDFromContext(r.Context())
					log.Printf("panic recovered request_id=%s: %v\n%s", requestID, recovered, debug.Stack())
					writeError(w, fmt.Errorf("panic recovered"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
