package app

import (
	"log/slog"

	"github.com/InsulaLabs/insi/client"
)

/*
This is used to indicate that the application needs to be able to talk to the insi api
and get information about the insi cluster.
*/
type AppInsiBinder interface {
	GetInsiApiKey() string
	GetInsiEndpoints() []client.Endpoint
	GetInsiSkipVerify() bool
}

type AppInsiPanel interface {
	GetInsiClient() *client.Client
}

type runtimeInsiConcern struct {
	insiApiKey    string
	insiEndpoints []client.Endpoint
	insiClient    *client.Client
	logger        *slog.Logger
	skipVerify    bool
}

var _ AppInsiPanel = &runtimeInsiConcern{}

func (r *runtimeInsiConcern) GetInsiClient() *client.Client {
	if r.insiClient != nil {
		return r.insiClient
	}
	var err error
	r.insiClient, err = client.NewClient(&client.Config{
		ConnectionType: client.ConnectionTypeDirect,
		Endpoints:      r.insiEndpoints,
		ApiKey:         r.insiApiKey,
		SkipVerify:     r.skipVerify,
		Logger:         r.logger,
	})
	if err != nil {
		r.logger.Error("Failed to create Insi client", "error", err)
		return nil
	}
	return r.insiClient
}

func (r *runtimeImpl) internalSetupInsi() {
	r.logger.Info("Setting up Insi client", "endpoints", r.cInsi.insiEndpoints)
}

func (r *runtimeImpl) GetInsiPanel() AppInsiPanel {
	return &r.cInsi
}
