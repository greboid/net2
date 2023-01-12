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
	"time"
)

type Server struct {
	Server   *http.Server
	shutdown chan os.Signal
	Sites    *net2.SiteManager
}

func (s *Server) listenAndServe() error {
	if s.shutdown == nil {
		s.shutdown = make(chan os.Signal)
	}
	if err := s.Server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Debug().Err(err).Msg("Server error")
		return err
	}
	<-s.shutdown
	return nil
}

func (s *Server) waitForShutdown() error {
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, os.Kill)
	<-signals
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
}

func (s *Server) Run() error {
	if s.Server == nil {
		return fmt.Errorf("server must be initialised")
	}
	g := new(errgroup.Group)
	g.Go(func() error {
		return s.listenAndServe()
	})
	g.Go(func() error {
		return s.waitForShutdown()
	})
	return g.Wait()
}
