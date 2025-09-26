package xfs

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/bosley/txpx/pkg/beau"
	"github.com/bosley/txpx/pkg/planar"
	"github.com/bosley/txpx/pkg/planar/goba"
	"github.com/bosley/txpx/pkg/pool"
)

var (
	XFS_WORKERS_PER_KV = 5
)

type DataStore interface {
	SystemSettings() planar.LockedVirtualKV
	ApplicationSettings() planar.LockedVirtualKV

	AppData() planar.KV
}

type xfsImpl struct {
	location string

	appData    planar.KV
	systemData planar.KV

	systemSettings      planar.VirtualKV
	applicationSettings planar.VirtualKV
}

var _ DataStore = &xfsImpl{}

func NewDataStore(logger *slog.Logger, ctx context.Context, location string) DataStore {

	dataProvider := goba.OpenBadgerBackend(filepath.Join(location, "xfs"))

	appData := beau.MFn[planar.KV](dataProvider.LoadOrCreate,
		logger.With("component", "xfs"),
		"app-data",
	)

	/*
		Load or create will either load or create a whole backend database
		In this case its "system-data"

		Under system data we have virtual databases that scope entries to a key prefix
		For system data we have two top-level scopes that can only be read/written with
		keys within their scope; i.e settings and system can work with key "a" and
		have it really mark "settings:system:a" and "settings:application:a" respectively.

		We dont need to atm but we also pool observer workers (that the interface requires)
		for us to monitor changes in an event-driven fashion
	*/
	systemData := beau.MFn[planar.KV](dataProvider.LoadOrCreate,
		logger.With("component", "xfs"),
		"system-data",
	)

	sharedPool := pool.NewBuilder().WithLogger(logger).WithWorkers(XFS_WORKERS_PER_KV).Build(ctx)

	settingsVDB := planar.NewVirtualKV(
		sharedPool,
		systemData,
		[]byte("settings:"),
	)

	systemSettings := planar.NewVirtualKV(
		sharedPool,
		settingsVDB,
		[]byte("system:"),
	)

	applicationSettings := planar.NewVirtualKV(
		sharedPool,
		settingsVDB,
		[]byte("application:"),
	)

	return &xfsImpl{
		location:            location,
		appData:             appData,
		systemData:          systemData,
		systemSettings:      systemSettings,
		applicationSettings: applicationSettings,
	}
}

func (x *xfsImpl) AppData() planar.KV {
	return x.appData
}

func (x *xfsImpl) SystemData() planar.KV {
	return x.systemData
}

func (x *xfsImpl) SystemSettings() planar.LockedVirtualKV {
	return x.systemSettings.GetLockedVariant()
}

func (x *xfsImpl) ApplicationSettings() planar.LockedVirtualKV {
	return x.applicationSettings.GetLockedVariant()
}
