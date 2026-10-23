package net2

import (
	"context"
	"crypto/tls"
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
			httpClient:       getHttpClient(conf.ClientID, conf.Sites[index].Username, conf.Sites[index].Password, baseURL, conf.Sites[index].VerifyTLS),
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

// outboundTimeout bounds each request to a Net2 server (including token
// fetches) so a hung connection fails instead of blocking the update loop
// indefinitely.
const outboundTimeout = 30 * time.Second

func getHttpClient(clientID string, username string, password string, baseURL string, verifyTLS bool) *http.Client {
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
	// Net2 serves self-signed certificates by default, so certificate
	// verification is opt-in per site via verifyTLS.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !verifyTLS},
	}
	sslcli := &http.Client{Transport: tr, Timeout: outboundTimeout}
	ctx := context.Background()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)
	httpClient := oauth2.NewClient(ctx, oauthConfig.TokenSource(ctx))
	httpClient.Timeout = outboundTimeout
	return httpClient
}

type SiteManager struct {
	mu      sync.RWMutex
	started bool
	sites   map[int]*Site
	Logger  *zerolog.Logger
}

func (m *SiteManager) Start(sites []*Site) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}
	sitesByID := make(map[int]*Site, len(sites))
	for _, site := range sites {
		if _, exists := sitesByID[site.SiteID]; exists {
			return fmt.Errorf("duplicate site id: %d", site.SiteID)
		}
		sitesByID[site.SiteID] = site
	}
	m.sites = sitesByID
	log.Info().Msg("Starting sites")
	var firstErr error
	started := make([]*Site, 0, len(m.sites))
	for _, site := range lo.Values(m.sites) {
		log.Debug().Str("Site", site.Name).Msg("Starting site")
		err := site.Start()
		if err != nil {
			firstErr = fmt.Errorf("unable to start site %s: %w", site.Name, err)
			break
		}
		started = append(started, site)
	}
	if firstErr != nil {
		// Roll back: stop the sites that did start so nothing is left
		// running while the manager is marked as not started.
		for _, site := range started {
			site.Stop()
		}
		m.sites = nil
		return firstErr
	}
	m.started = true
	return nil
}

func (m *SiteManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return
	}
	lo.ForEach(lo.Values(m.sites), func(item *Site, _ int) {
		item.Stop()
	})
	m.started = false
}

func (m *SiteManager) GetSite(id int) *Site {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.started {
		return nil
	}
	return m.sites[id]
}

func (m *SiteManager) GetSites() map[int]*Site {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.started {
		return nil
	}
	return m.sites
}

func (m *SiteManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.started {
		return 0
	}
	return len(m.sites)
}

func (m *SiteManager) UpdateAll() {
	start := time.Now()
	log.Debug().Msg("Update all sites started")
	m.mu.RLock()
	sites := lo.Values(m.sites)
	m.mu.RUnlock()
	wg := new(sync.WaitGroup)
	for _, site := range sites {
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
