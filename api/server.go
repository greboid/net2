package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Server struct {
	Server   *http.Server
	shutdown chan struct{}
	Sites    *net2.SiteManager
}

func (s *Server) listenAndServe() error {
	if err := s.Server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Debug().Err(err).Msg("Server error")
		return err
	}
	<-s.shutdown
	return nil
}

func (s *Server) waitForShutdown(ctx context.Context) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case <-ctx.Done():
		// The server failed to start (or otherwise stopped) on its own;
		// there is nothing left to shut down gracefully.
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := s.Server.Shutdown(ctx)
	log.Debug().Err(err).Msg("Server shutdown")
	defer close(s.shutdown)
	return err
}

func (s *Server) Init(port int, handler http.Handler) {
	s.Server = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: handler,
	}
	s.shutdown = make(chan struct{})
}

func (s *Server) Run() error {
	if s.Server == nil {
		return fmt.Errorf("server must be initialised")
	}
	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		return s.listenAndServe()
	})
	g.Go(func() error {
		return s.waitForShutdown(ctx)
	})
	return g.Wait()
}
