package main

import (
	"log/slog"

	"github.com/bosley/txpx/pkg/events"
)

/*
By design the app event system only permits one handler per topic.
The idea is that each thing is only concerned with what its doing.

The main app is the exception in that it is responsible for handling
its own, as the others do, but also system events.

System events are what will bridge the application as a whole to
external intances of the application via insi et all.

Whenever http or someone gets something that is determined to be
a system event FROM an external instance, they will publish it to
the system event topic.

The main app will then handle it and determine what to do with it.
*/
type AppTxPxSystemEventHandler struct {
	logger *slog.Logger
	app    *AppTxPx
}

var _ events.EventHandler = &AppTxPxSystemEventHandler{}

func (a *AppTxPxSystemEventHandler) OnEvent(event events.Event) {
	a.app.logger.Info("received event", "event", event)
}
