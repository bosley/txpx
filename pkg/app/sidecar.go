package app

import (
	"sync"

	"github.com/bosley/txpx/pkg/procman"
)

type AppExternalBinder interface {
	BindProcman() *procman.Host // they provide us with a procman host
}

/*
The specific concerns for SideCar users
When SideCar is indicated the panel can be offered to inform the application
instance of the procman host for managing sidecar processes.
*/
type AppSideCarPanel interface {
	GetHost() *procman.Host
}

type runtimeSideCarConcern struct {
	hostedApps map[string]*procman.HostedApp
	mu         sync.RWMutex
	host       *procman.Host
}

var _ AppSideCarPanel = &runtimeSideCarConcern{}

func (r *runtimeSideCarConcern) GetHost() *procman.Host {
	return r.host
}

func (r *runtimeImpl) internalSetupSideCar() {
	r.cSideCar.host = r.sideCarBinder.BindProcman()
}

func (r *runtimeImpl) GetSideCarPanel() AppSideCarPanel {
	return &r.cSideCar
}
