package main

import (
	"flag"
	"fmt"
	"github.com/csmith/envflag"
	"github.com/greboid/net2/api"
	"github.com/greboid/net2/config"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"os"
	"os/signal"
	"syscall"
)

var (
	configFile = flag.String("config", "./config.yml", "Path to the config file")
	Debug      = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	envflag.Parse()
	logger := createLogger(*Debug)
	loadedConfig, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Unable to load config")
	}
	sites := net2.GetSites(loadedConfig, logger)
	run(sites, loadedConfig.APIPort, logger)
}

func run(sites []*net2.Site, apiPort int, logger *zerolog.Logger) {
	log.Info().Msg("Starting net2 proxy")
	log.Info().Strs("Sites", lo.Map(sites, func(item *net2.Site, index int) string {
		return fmt.Sprintf("%s (%d)", item.Name, item.SiteID)
	})).Msg("Loaded sites")
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	siteManager := &net2.SiteManager{Logger: logger}
	err := siteManager.Start(sites)
	if err != nil {
		log.Fatal().Err(err).Msg("Unable to start sites")
	}
	ws := api.Server{
		Sites: siteManager,
	}
	ws.Init(apiPort, ws.GetRoutes())
	if err = ws.Run(); err != nil {
		log.Error().Err(err).Msg("error running web server")
	}
	log.Info().Msg("Exiting.")
}

func createLogger(debug bool) *zerolog.Logger {
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	log.Logger = logger
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	return &logger
}
