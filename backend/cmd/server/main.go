package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"siracrm/internal/auth"
	"siracrm/internal/config"
	"siracrm/internal/database"
	"siracrm/internal/httpapi"
	"siracrm/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	pool, err := database.Open(startupContext, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = database.Migrate(startupContext, pool); err != nil {
		return err
	}
	repository := store.New(pool)
	authentication := auth.New(repository)
	if err = authentication.Bootstrap(startupContext, settings.AdminEmail, settings.AdminPassword); err != nil {
		return err
	}
	application := httpapi.New(repository, authentication, settings)
	server := &http.Server{Addr: settings.Address, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	listenerErrors := make(chan error, 1)
	go func() {
		log.Printf("Sira API listening on %s", settings.Address)
		listenerErrors <- server.ListenAndServe()
	}()
	signalContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stopSignals()
	select {
	case err = <-listenerErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-signalContext.Done():
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	return server.Shutdown(shutdownContext)
}
