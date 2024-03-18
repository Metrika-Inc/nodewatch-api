package httpTransport

import (
	"context"
	"errors"
	"eth2-crawler/utils/config"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Metrika-Inc/golibs/health"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type HTTPTransport struct {
	*config.Server
	r               *chi.Mux
	livenessHandler *health.LivenessHTTPHandler
}

func NewHttpTransport(cfg *config.Server) *HTTPTransport {
	handler := &HTTPTransport{
		Server:          cfg,
		r:               chi.NewRouter(),
		livenessHandler: &health.LivenessHTTPHandler{},
	}

	handler.r.Use(middleware.Logger)

	return handler
}

func (h *HTTPTransport) RegisterRoutes() {
	// Register routes

	// Prometheus
	h.r.Get("/metrics", promhttp.Handler().ServeHTTP)

	// Healthchecks
	h.r.Get("/alive", h.livenessHandler.ServeHTTP)
}

func (h *HTTPTransport) Start(ctx context.Context, wg *sync.WaitGroup) {
	// Start the server
	srv := &http.Server{
		Handler:           h.r,
		ReadTimeout:       time.Duration(h.ReadTimeout) * time.Second,
		ReadHeaderTimeout: time.Duration(h.ReadHeaderTimeout) * time.Second,
		WriteTimeout:      time.Duration(h.WriteTimeout) * time.Second,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr := fmt.Sprintf("%s:%d", "0.0.0.0", h.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			zap.S().Fatalw("failed to start listening", "address", addr, zap.Error(err))
		}

		if err := srv.Serve(listener); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			zap.S().Errorw("http serving failed", zap.Error(err))
		}
	}()

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
}
