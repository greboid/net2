package net2

import (
	"context"
	"crypto/tls"
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
	"sync"
	"time"
)

func GetSites(conf *config.Config, logger *zerolog.Logger) []*Site {
	sites := make([]*Site, 0, len(conf.Sites))
	for index := range conf.Sites {
		var baseURL string
		if conf.Sites[index].Https {
			baseURL = fmt.Sprintf("https://%s:%d", conf.Sites[index].IP, conf.Sites[index].Port)
		} else {
			baseURL = fmt.Sprintf("http://%s:%d", conf.Sites[index].IP, conf.Sites[index].Port)
		}
		sites = append(sites, &Site{
			config:           &conf.Sites[index],
			httpClient:       getHttpClient(conf.ClientID, conf.Sites[index].Username, conf.Sites[index].Password, baseURL),
			clientID:         conf.ClientID,
			QuitChan:         make(chan bool),
			BaseURL:          baseURL,
			SiteID:           conf.Sites[index].ID,
			Name:             conf.Sites[index].Name,
			LastPolled:       time.Time{},
			localIDFieldName: conf.Sites[index].LocalIDField,
			logger:           logger,
		})
	}
	return sites
}

func getHttpClient(clientID string, username string, password string, baseURL string) *http.Client {
	oauthConfig := clientcredentials.Config{
		ClientID: clientID,
		TokenURL: fmt.Sprintf("%s/api/v1/authorization/tokens", baseURL),
		EndpointParams: url.Values{
			"username":   {username},
			"password":   {password},
			"grant_type": {"password"},
			"scope":      {"offline_access"},
		},
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	sslcli := &http.Client{Transport: tr}
	ctx := context.Background()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)
	httpClient := oauth2.NewClient(ctx, oauthConfig.TokenSource(ctx))
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
	log.Info().Msg("Starting sites")
	started := 0
	lo.ForEach(lo.Values(m.sites), func(item *Site, index int) {
		log.Debug().Str("Site", item.Name).Msg("Starting site")
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
	start := time.Now()
	log.Debug().Msg("Update all sites started")
	wg := new(sync.WaitGroup)
	for _, site := range m.sites {
		wg.Add(1)
		go func(s *Site, wg *sync.WaitGroup) {
			s.UpdateAll()
			wg.Done()
		}(site, wg)
	}
	wg.Wait()
	total := time.Now().Sub(start).Milliseconds()
	log.Debug().Int64("Total (ms)", total).Msg("Update all sites finished")
}
