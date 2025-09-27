package main

import "time"

const (
	EventTopicTxPxSystem     = "txpx-system"
	EventTopicTxPxMainApp    = "txpx-main-app"
	EventTopicTxPxHttpServer = "txpx-http-server"
)

/*
Convention:

	L->R
	0) Importance (highest is 255)
	1) Thread/System Origin
	3) Specific System Event Identifier
	4) RESERVED
*/

const (
	ImportanceHighest = 255
	ImportanceHigh    = 128
	ImportanceMedium  = 64
	ImportanceLow     = 32
	ImportanceLowest  = 16

	ThreadOriginTxPxMainApp      = 0
	ThreadOriginTxPxHttpServer   = 1
	ThreadOriginTxPxSideCar      = 2
	ThreadOriginTxPxInsi         = 3
	ThreadOriginTxPxEvents       = 4
	ThreadOriginTxPxSystem       = 5
	ThreadOriginTxPxRemoteOrigin = 6
	ThreadOriginTxPxOther        = 255
)

const (
	ConfigFileClusterDefault = "cluster.yaml"
	ConfigFileSiteDefault    = "site.yaml"
)

func ConstructEventHeader(importance int, threadOrigin int, specificEventIdentifier int) [4]byte {
	return [4]byte{byte(importance), byte(threadOrigin), byte(specificEventIdentifier), 0}
}

type SystemEventIdentifier int

const (
	SystemEventIdentifierInsiOnline  SystemEventIdentifier = 0
	SystemEventIdentifierInsiOffline SystemEventIdentifier = 1
)

type EventInsiOnline struct {
	PingData map[string]string
}

type EventInsiOffline struct {
	At time.Time
}
