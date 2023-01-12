package net2

import (
	"context"
	"errors"
	"fmt"
	"github.com/greboid/net2/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"net/http"
	"net/url"
	"time"
)

func GetSites(conf *config.Config, logger *zerolog.Logger) []*Site {
	sites := make([]*Site, 0, len(conf.Sites))
	for _, site := range conf.Sites {
		var baseURL string
		if site.Https {
			baseURL = fmt.Sprintf("https://%s:%d", site.IP, site.Port)
		} else {
			baseURL = fmt.Sprintf("http://%s:%d", site.IP, site.Port)
		}
		sites = append(sites, &Site{
			httpClient:       getHttpClient(conf, site, baseURL),
			QuitChan:         make(chan bool),
			BaseURL:          baseURL,
			SiteID:           site.ID,
			Name:             site.Name,
			LastPolled:       time.Time{},
			localIDFieldName: site.LocalIDField,
			logger:           logger,
		})
	}
	return sites
}

func getHttpClient(conf *config.Config, site config.SiteConfig, baseURL string) *http.Client {
	oauthConfig := clientcredentials.Config{
		ClientID: conf.ClientID,
		TokenURL: fmt.Sprintf("%s/api/v1/authorization/tokens", baseURL),
		EndpointParams: url.Values{
			"username":   {site.Username},
			"password":   {site.Password},
			"grant_type": {"password"},
			"scope":      {"offline_access"},
		},
	}
	httpClient := oauth2.NewClient(context.Background(), oauthConfig.TokenSource(context.Background()))
	return httpClient
}

type SiteManager struct {
	started bool
	sites   map[int]*Site
	Logger  *zerolog.Logger
}

func (m *SiteManager) Start(sites []*Site) error {
	if m.started {
		return nil
	}
	m.sites = lo.SliceToMap(sites, func(item *Site) (int, *Site) {
		return item.SiteID, item
	})
	started := 0
	lo.ForEach(lo.Values(m.sites), func(item *Site, index int) {
		log.Debug().Int("Site", index).Msg("Starting site")
		err := item.Start()
		if err == nil {
			started++
		}
	})
	if started != len(m.sites) {
		return errors.New("unable start all sites")
	}
	m.started = true
	return nil
}

func (m *SiteManager) Stop() {
	if !m.started {
		return
	}
	lo.ForEach(lo.Values(m.sites), func(item *Site, _ int) {
		item.Stop()
	})
	m.started = false
}

func (m *SiteManager) GetSite(id int) *Site {
	if !m.started {
		return nil
	}
	return m.sites[id]
}

func (m *SiteManager) GetSites() map[int]*Site {
	if !m.started {
		return nil
	}
	return m.sites
}

func (m *SiteManager) Count() int {
	if !m.started {
		return 0
	}
	return len(m.sites)
}

func (m *SiteManager) UpdateAll() {
	for _, site := range m.sites {
		site.UpdateAll()
	}
}
