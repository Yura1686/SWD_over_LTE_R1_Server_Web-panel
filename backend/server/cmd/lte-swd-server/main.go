package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lte_swd/backend/server/internal/auth"
	"lte_swd/backend/server/internal/config"
	"lte_swd/backend/server/internal/httpapi"
	"lte_swd/backend/server/internal/service"
	"lte_swd/backend/server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	st, err := store.NewStateStore(cfg.DataFile, cfg.FleetLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store error: %v\n", err)
		os.Exit(1)
	}

	opAuth := auth.NewOperatorAuth(cfg.OperatorPassword, cfg.OperatorTokenTTL)
	svc := service.New(cfg, st, opAuth)
	api := httpapi.NewHandler(svc, cfg.StaticDir, httpapi.Options{
		MaxJSONBytes:      cfg.MaxJSONBytes,
		MaxArtifactBytes:  cfg.MaxArtifactBytes,
		APIRatePerMinute:  cfg.APIRatePerMinute,
		LoginRatePerMin:   cfg.LoginRatePerMinute,
		LoginBurst:        cfg.LoginBurst,
		TrustProxyHeaders: cfg.TrustProxyHeaders,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.BuildMux(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	useTLS := cfg.HTTPSAddr != ""
	if useTLS {
		server.Addr = cfg.HTTPSAddr
	}

	go func() {
		if useTLS {
			fmt.Printf("LTE_SWD HTTPS server listening on %s\n", cfg.HTTPSAddr)
			if err := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "https server error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		fmt.Printf("LTE_SWD HTTP server listening on %s\n", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "http server error: %v\n", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
