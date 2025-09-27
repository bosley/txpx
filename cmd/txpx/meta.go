package main

import (
	"time"

	"github.com/bosley/txpx/pkg/app"
)

type AppMetaTxPx struct {
	identifier string
	startTime  time.Time
}

var _ app.AppMetaStat = &AppMetaTxPx{}

func (a *AppMetaTxPx) GetIdentifier() string {
	return a.identifier
}

func (a *AppMetaTxPx) GetUptime() time.Duration {
	return time.Since(a.startTime)
}
