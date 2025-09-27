package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/InsulaLabs/insi/client"
	"github.com/InsulaLabs/insi/config"
	"github.com/bosley/txpx/pkg/app"
	"github.com/bosley/txpx/pkg/events"
)

type AppTxPxInsiProvider struct {
	apiKey     string
	endpoints  []client.Endpoint
	skipVerify bool
	app        *AppTxPx
}

var _ app.AppInsiBinder = &AppTxPxInsiProvider{}

func (t *AppTxPxInsiProvider) GetInsiApiKey() string {
	return t.apiKey
}

func (t *AppTxPxInsiProvider) GetInsiEndpoints() []client.Endpoint {
	return t.endpoints
}

func (t *AppTxPxInsiProvider) GetInsiSkipVerify() bool {
	return t.skipVerify
}

func (t *AppTxPxInsiProvider) SetApp(app *AppTxPx) {
	t.app = app
}

func NewAppTxPxInsiProvider(
	config *config.Cluster) *AppTxPxInsiProvider {

	// Construct the insi root password
	secretHash := sha256.New()
	secretHash.Write([]byte(config.InstanceSecret))
	rootInsiApiKey := hex.EncodeToString(secretHash.Sum(nil))
	rootInsiApiKey = base64.StdEncoding.EncodeToString([]byte(rootInsiApiKey))

	endpoints := []client.Endpoint{}
	for _, node := range config.Nodes {
		endpoints = append(endpoints, client.Endpoint{
			PublicBinding:  node.PublicBinding,
			PrivateBinding: node.PrivateBinding,
			ClientDomain:   node.ClientDomain,
		})
	}

	return &AppTxPxInsiProvider{
		apiKey:     rootInsiApiKey,
		endpoints:  endpoints,
		skipVerify: config.ClientSkipVerify,
	}
}

func (t *AppTxPxInsiProvider) MonitorForInsiStartup() {

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	insiClient := t.app.rt.GetInsiPanel().GetInsiClient()
	if insiClient == nil {
		t.app.logger.Error("Failed to get Insi client")
		return
	}

	systemPublisher, err := t.app.rt.GetEventsPanel().GetTopicPublisher(EventTopicTxPxSystem)
	if err != nil {
		t.app.logger.Error("Failed to get System publisher", "error", err)
		return
	}

	prevIsOnline := false
	isOnline := false

	checkForStatusChange := func() {
		pingData, err := insiClient.Ping()
		if err != nil {
			t.app.logger.Error("Failed to ping Insi cluster", "error", err)
			isOnline = false
		} else {
			isOnline = true
		}
		if isOnline != prevIsOnline {
			prevIsOnline = isOnline
			systemEventIdentifier := SystemEventIdentifierInsiOnline
			var eventBody interface{}
			if isOnline {
				eventBody = EventInsiOnline{
					PingData: pingData,
				}
			} else {
				eventBody = EventInsiOffline{
					At: time.Now(),
				}
				systemEventIdentifier = SystemEventIdentifierInsiOffline
			}
			systemPublisher.Publish(
				events.Event{
					Header: ConstructEventHeader(
						ImportanceHighest,
						ThreadOriginTxPxInsi,
						int(systemEventIdentifier),
					),
					Body: eventBody,
				},
			)
		}
	}

	for {
		select {
		case <-t.app.appCtx.Done():
			return
		case <-ticker.C:
			checkForStatusChange()
		}
	}
}
