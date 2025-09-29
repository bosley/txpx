package main

import (
	"time"

	"github.com/bosley/txpx/cmd/txpx/config"
)

func ConstructEventHeader(importance int, threadOrigin int, specificEventIdentifier int) [4]byte {
	return [4]byte{
		byte(importance),
		byte(threadOrigin),
		byte(specificEventIdentifier),
		config.HeaderValFilterAcceptAll,
	}
}

/*
	System Events
*/

type SystemEventIdentifier int

const (
	SystemEventIdentifierInsiOnline  SystemEventIdentifier = config.HeaderValSpecificEventIdentifier_SystemBeginInclusive
	SystemEventIdentifierInsiOffline SystemEventIdentifier = config.HeaderValSpecificEventIdentifier_SystemBeginInclusive + 1
)

type EventInsiOnline struct {
	PingData map[string]string
}

type EventInsiOffline struct {
	At time.Time
}

/*
	External Events
*/
