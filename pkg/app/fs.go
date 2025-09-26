package app

import "github.com/bosley/txpx/pkg/xfs"

/*
The specific concerns for FS users
When FS is indicated the panel can be offered to inform the application
instance of the current install path.
*/
type AppFSPanel interface {
	GetInstallPath() string
	GetDataStore() xfs.DataStore
}

type runtimeFSConcern struct {
	fs          xfs.DataStore
	installPath string
}

var _ AppFSPanel = &runtimeFSConcern{}

func (r *runtimeFSConcern) GetInstallPath() string {
	return r.installPath
}

func (r *runtimeFSConcern) GetDataStore() xfs.DataStore {
	return r.fs
}

func (r *runtimeImpl) GetFSPanel() AppFSPanel {
	return &r.cFS
}
