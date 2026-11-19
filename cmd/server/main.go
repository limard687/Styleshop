package main

import (
	"clothing-store/internal/config"
	"clothing-store/internal/httpapi"
	"clothing-store/internal/store"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	c, err := config.Load()
	if err != nil {
		log.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	db, err := store.Open(c.DBPath)
	if err != nil {
		log.Error("database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if c.SeedDemo {
		if err = store.SeedStyleShop(db, c.DemoPassword); err != nil {
			log.Error("seed failed", "error", err)
			os.Exit(1)
		}
	}
	app, err := httpapi.New(db, c, log)
	if err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: net.JoinHostPort(c.Host, c.Port), Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Error("shutdown failed", "error", err)
		}
	}()
	log.Info("server listening", "address", srv.Addr, "demo", c.SeedDemo)
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server failed", "error", err)
		os.Exit(1)
	}
}
