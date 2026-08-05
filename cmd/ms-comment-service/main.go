package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	filestorageadapter "github.com/bemulima/ms-go-comment/internal/adapters/filestorage"
	httpadapter "github.com/bemulima/ms-go-comment/internal/adapters/http"
	pgadapter "github.com/bemulima/ms-go-comment/internal/adapters/postgres"
	"github.com/bemulima/ms-go-comment/internal/config"
	commentuc "github.com/bemulima/ms-go-comment/internal/usecase/comment"
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
	fileStorage := filestorageadapter.Client{BaseURL: cfg.FileStorageServiceBaseURL}
	commentService := &commentuc.Service{
		Spaces: spaces, Threads: threads, Comments: comments, Attachments: attachments,
		Outbox: outbox, Tx: pgadapter.TransactionManager{Pool: pool}, Files: fileStorage,
		AttachmentTTLMinutes:  cfg.AttachmentTTLMinutes,
		SignedURLMinutes:      cfg.AttachmentSignedURLMinutes,
		ActivationMaxAttempts: cfg.AttachmentActivationAttempts,
	}

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           httpadapter.NewRouter(httpadapter.RouterDependencies{CommentService: commentService}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go runAttachmentWorker(ctx, logger, commentService, time.Duration(cfg.AttachmentWorkerInterval)*time.Second, cfg.AttachmentWorkerBatch)

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
			logger.Fatal("http server error", zap.Error(err))
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeout)*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
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
