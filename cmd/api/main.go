package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gmixoulis/company-service/internal/auth"
	"github.com/gmixoulis/company-service/internal/company"
	"github.com/gmixoulis/company-service/internal/config"
	"github.com/gmixoulis/company-service/internal/events"
	"github.com/gmixoulis/company-service/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	log.Println("database connection established")

	producer := events.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("close kafka producer: %v", err)
		}
	}()

	companyRepo := company.NewRepository(pool)
	companySvc := company.NewService(companyRepo, producer)
	companyHandler := company.NewHandler(companySvc)

	authHandler := auth.NewHandler(cfg.JWT.Secret, cfg.JWT.AdminUsername, cfg.JWT.AdminPassword)
	authMiddleware := auth.NewMiddleware(cfg.JWT.Secret)

	handler := server.New(companyHandler, authHandler, authMiddleware)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
