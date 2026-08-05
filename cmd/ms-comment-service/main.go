package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	filestorageadapter "github.com/bemulima/ms-go-comment/internal/adapters/filestorage"
	httpadapter "github.com/bemulima/ms-go-comment/internal/adapters/http"
	natsadapter "github.com/bemulima/ms-go-comment/internal/adapters/nats"
	pgadapter "github.com/bemulima/ms-go-comment/internal/adapters/postgres"
	websocketadapter "github.com/bemulima/ms-go-comment/internal/adapters/websocket"
	"github.com/bemulima/ms-go-comment/internal/config"
	adminuc "github.com/bemulima/ms-go-comment/internal/usecase/admin"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
	realtimeuc "github.com/bemulima/ms-go-comment/internal/usecase/realtime"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("logger error: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	rootContext := context.Background()
	pool, err := pgadapter.Connect(rootContext, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	defer pool.Close()

	spaces := pgadapter.SpaceRepository{Pool: pool}
	threads := pgadapter.ThreadRepository{Pool: pool}
	comments := pgadapter.CommentRepository{Pool: pool}
	attachments := pgadapter.AttachmentRepository{Pool: pool}
	outbox := pgadapter.OutboxRepository{Pool: pool}
	tickets := pgadapter.RealtimeTicketRepository{Pool: pool}
	fileStorage := filestorageadapter.Client{BaseURL: cfg.FileStorageServiceBaseURL}
	commentService := &commentuc.Service{
		Spaces: spaces, Threads: threads, Comments: comments, Attachments: attachments,
		Outbox: outbox, Tx: pgadapter.TransactionManager{Pool: pool}, Files: fileStorage,
		AttachmentTTLMinutes:  cfg.AttachmentTTLMinutes,
		SignedURLMinutes:      cfg.AttachmentSignedURLMinutes,
		ActivationMaxAttempts: cfg.AttachmentActivationAttempts,
	}
	adminService := &adminuc.Service{
		Spaces: spaces, Threads: threads, Comments: comments, Attachments: attachments,
		Outbox: outbox, Tx: pgadapter.TransactionManager{Pool: pool},
	}
	realtimeService := &realtimeuc.TicketService{
		Spaces: spaces, Threads: threads, Tickets: tickets,
		TTL: time.Duration(cfg.RealtimeTicketTTLSeconds) * time.Second,
	}
	dispatcher := &realtimeuc.Dispatcher{
		Outbox: outbox, Lease: time.Duration(cfg.OutboxLeaseSeconds) * time.Second,
	}

	var natsConnection interface{ Drain() error }
	var natsClient *natsadapter.Client
	if modeHasRealtime(cfg.ServiceMode) || modeHasWorkers(cfg.ServiceMode) {
		connection, err := natsadapter.Connect(cfg.NATSURL)
		if err != nil {
			log.Fatalf("NATS error: %v", err)
		}
		natsConnection = connection
		natsClient = &natsadapter.Client{Conn: connection}
		dispatcher.Publisher = natsClient
		if modeHasWorkers(cfg.ServiceMode) {
			if err := natsClient.EnsureLifecycleStream(rootContext); err != nil {
				log.Fatalf("NATS stream error: %v", err)
			}
		}
	}

	var hub *websocketadapter.Hub
	var realtimeSubscription *natsadapter.Subscription
	var websocketHandler http.Handler
	if modeHasRealtime(cfg.ServiceMode) {
		hub = websocketadapter.NewHub(cfg.WSMaxConnectionsPerUser, cfg.WSQueueSize)
		websocketHandler = websocketadapter.Handler{
			Tickets: realtimeService, Hub: hub, Typing: natsClient, MaxFrameBytes: cfg.WSMaxFrameBytes,
		}
		realtimeSubscription, err = natsClient.SubscribeRealtime(hub)
		if err != nil {
			log.Fatalf("NATS realtime subscription error: %v", err)
		}
	}

	routerDependencies := httpadapter.RouterDependencies{}
	if modeHasAPI(cfg.ServiceMode) {
		routerDependencies.CommentService = commentService
		routerDependencies.AdminService = adminService
		routerDependencies.RealtimeService = realtimeService
	}
	if modeHasRealtime(cfg.ServiceMode) {
		routerDependencies.WebSocketHandler = websocketHandler
	}

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           httpadapter.NewRouter(routerDependencies),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var workers sync.WaitGroup
	if modeHasWorkers(cfg.ServiceMode) {
		workers.Add(3)
		go func() {
			defer workers.Done()
			runAttachmentWorker(ctx, logger, commentService, time.Duration(cfg.AttachmentWorkerInterval)*time.Second, cfg.AttachmentWorkerBatch)
		}()
		go func() {
			defer workers.Done()
			runOutboxWorker(ctx, logger, dispatcher, time.Duration(cfg.OutboxWorkerIntervalMS)*time.Millisecond, cfg.OutboxWorkerBatch)
		}()
		go func() {
			defer workers.Done()
			runTicketCleanupWorker(ctx, logger, realtimeService, time.Duration(cfg.RealtimeTicketCleanupSeconds)*time.Second, cfg.OutboxWorkerBatch)
		}()
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("starting comment service", zap.String("port", cfg.HTTPPort), zap.String("mode", cfg.ServiceMode))
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down comment service")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", zap.Error(err))
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeout)*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
	if realtimeSubscription != nil {
		_ = realtimeSubscription.Close()
	}
	if hub != nil {
		hub.Close()
	}
	workers.Wait()
	if natsConnection != nil {
		_ = natsConnection.Drain()
	}
}

func runAttachmentWorker(ctx context.Context, logger *zap.Logger, service *commentuc.Service, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if batch <= 0 {
		batch = 50
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := service.ProcessAttachmentWork(ctx, batch)
			if err != nil {
				logger.Error("attachment worker cycle failed", zap.Error(err))
				continue
			}
			if result.Activated+result.Failed+result.Deleted > 0 {
				logger.Info("attachment worker cycle completed",
					zap.Int("activated", result.Activated), zap.Int("failed", result.Failed), zap.Int("deleted", result.Deleted))
			}
		}
	}
}

func runOutboxWorker(ctx context.Context, logger *zap.Logger, dispatcher *realtimeuc.Dispatcher, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := dispatcher.Process(ctx, batch)
			if err != nil {
				logger.Error("outbox worker cycle failed", zap.Error(err))
				continue
			}
			if result.Published+result.Failed > 0 {
				logger.Info("outbox worker cycle completed", zap.Int("published", result.Published), zap.Int("failed", result.Failed))
			}
		}
	}
}

func runTicketCleanupWorker(ctx context.Context, logger *zap.Logger, service *realtimeuc.TicketService, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := service.DeleteExpired(ctx, batch)
			if err != nil {
				logger.Error("realtime ticket cleanup failed", zap.Error(err))
			} else if deleted > 0 {
				logger.Info("realtime ticket cleanup completed", zap.Int("deleted", deleted))
			}
		}
	}
}

func modeHasAPI(mode string) bool      { return mode == "all" || mode == "api" }
func modeHasRealtime(mode string) bool { return mode == "all" || mode == "realtime" }
func modeHasWorkers(mode string) bool  { return mode == "all" || mode == "worker" }
