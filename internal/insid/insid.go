/*
This controller plays multiple roles regarding the insi cluster.
It primarily is used to start/stop the cluster utilizing passed-through arguments
to the insi runtime.

I've lifted the primary "insid" runtime code from insi so we could leverage the
thread as-if it itself was an application rather than an object within our
process. This helps homoginize the useage of insi, as well as providing a solid
base for us to build upon.
*/

package insid

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/InsulaLabs/insi/client"
	"github.com/InsulaLabs/insi/config"
	"github.com/InsulaLabs/insi/runtime"
	"github.com/bosley/txpx/pkg/app"
)

var (
	ErrAlreadyRunning    = errors.New("insid is already running")
	ErrInsiFailedToStart = errors.New("insid failed to start")
)

type StateChangeHandler interface {
	InsidOnline(insiPingData map[string]string)
	InsidOffline()
}

// Notice that the controller is-a AppInsiBinder so we can
// hand this whole controller to the app initilization process
// to setup/ configure the client for standard access across
// the txpx application.
type Controller interface {
	app.AppInsiBinder
	Start(
		ctx context.Context,
		args []string,
		startupTimeout time.Duration,
		appRt app.AppRuntime,
	) error
	Stop()
}

var _ app.AppInsiBinder = &insidController{}

type insidController struct {
	logger *slog.Logger

	rt      *runtime.Runtime
	running atomic.Bool

	errChannel         chan error
	stateChangeHandler StateChangeHandler
	clusterConfig      *config.Cluster

	insiEndpoints []client.Endpoint
	apiKey        string
	skipVerify    bool

	appRt app.AppRuntime
}

func NewController(
	logger *slog.Logger,
	config *config.Cluster,
	stateChangeHandler StateChangeHandler,
) *insidController {

	/*
		Here, we use the instance secret from the cluster configuration
		to construct the root api key as is done in the insi runtime.
		This allows us to have root-level access to the cluster and arbitrarily
		assign api keys/ entities as we see fit.
	*/
	secretHash := sha256.New()
	secretHash.Write([]byte(config.InstanceSecret))
	rootInsiApiKey := hex.EncodeToString(secretHash.Sum(nil))
	rootInsiApiKey = base64.StdEncoding.EncodeToString([]byte(rootInsiApiKey))

	// If we are only a single node, or operating as a single node, we still eat up all
	// of the endpoints so that the client can load-balance calls to the node
	// cluster and locate the leader node.
	endpoints := []client.Endpoint{}
	for _, node := range config.Nodes {
		endpoints = append(endpoints, client.Endpoint{
			PublicBinding:  node.PublicBinding,
			PrivateBinding: node.PrivateBinding,
			ClientDomain:   node.ClientDomain,
		})
	}

	return &insidController{
		logger:             logger,
		errChannel:         make(chan error),
		stateChangeHandler: stateChangeHandler,
		clusterConfig:      config,
		insiEndpoints:      endpoints,
		apiKey:             rootInsiApiKey,
		skipVerify:         config.ClientSkipVerify,
	}
}

func (c *insidController) GetInsiApiKey() string {
	return c.apiKey
}

func (c *insidController) GetInsiEndpoints() []client.Endpoint {
	return c.insiEndpoints
}

func (c *insidController) GetInsiSkipVerify() bool {
	return c.skipVerify
}

func (c *insidController) Start(
	ctx context.Context,
	args []string,
	startupTimeout time.Duration,
	appRt app.AppRuntime,
) error {

	if c.running.Load() {
		return ErrAlreadyRunning
	}

	c.appRt = appRt

	go c.insidMain(ctx, c.errChannel, args)

	for !c.running.Load() {
		select {
		case err := <-c.errChannel:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			continue
		case <-time.After(startupTimeout):
			return ErrInsiFailedToStart
		}
	}

	go c.startMonitorForStatusChange(ctx)

	c.logger.Info("insid started")
	return nil
}

func (c *insidController) Stop() {
	if !c.running.Load() {
		c.logger.Info("insid is not running")
		return
	}
	c.running.Store(false)
	if c.rt != nil {
		c.logger.Info("stopping insid")
		c.rt.Stop()
		c.logger.Info("waiting for insid to stop")
		c.rt.Wait()
		c.logger.Info("insid stopped")
	}
}

// This is the entire main for insid. We need to start our app with http and an insi client,
// but before we even define the app we should start insid in a different thread.
// Then, we can tie our insi client to the insi cluster running in the background.
// This will provide a stable distributed cache/kv store for the application to use
// We can ALSO (later) hookup the insi event system to the application event system
// and then leverage the insi raft eventing system to coordinate with other instances of the application
// That will be once we get this up and running.
func (c *insidController) insidMain(ctx context.Context, errCh chan error, args []string) {
	// misconfigured logger in raft bbolt backend
	// we use slog so this only silences the bbolt backend
	// to raft snapshot store that isn't directly used by us
	log.SetOutput(io.Discard)

	c.running.Store(true)
	defer c.running.Store(false)

	// The default config file path can be set here
	// It's passed to the runtime, which handles flag parsing for --config override.
	rt, err := runtime.New(ctx, args, "cluster.yaml")
	if err != nil {
		errCh <- err
		return
	}

	c.running.Store(true)

	c.rt = rt
	if err := rt.Run(); err != nil {
		errCh <- err
		return
	}

	rt.Wait()
}

/*
Monitors the insi cluster by intermittently pinging it to see if its online.
When the status changes, the controller will issue the appropriate callbacks
to the application to handle as required.
*/
func (c *insidController) startMonitorForStatusChange(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	insiClient := c.appRt.GetInsiPanel().GetInsiClient()
	if insiClient == nil {
		c.logger.Error("Failed to get Insi client")
		return
	}

	prevIsOnline := false
	isOnline := false

	checkForStatusChange := func() {
		pingData, err := insiClient.Ping()
		if err != nil {
			c.logger.Error("Failed to ping Insi cluster", "error", err)
			isOnline = false
		} else {
			isOnline = true
		}
		if isOnline != prevIsOnline {
			prevIsOnline = isOnline
			if isOnline {
				c.stateChangeHandler.InsidOnline(pingData)
			} else {
				c.stateChangeHandler.InsidOffline()
			}
		}
	}

	defer func() {
		c.logger.Info("insidmonitor for status change stopped")
	}()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("stopping monitor for status change; context done")
			return
		case <-ticker.C:
			checkForStatusChange()
		case <-time.After(100 * time.Millisecond):
			continue
		}
	}
}
