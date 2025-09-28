package main

import "time"

func ConstructEventHeader(importance int, threadOrigin int, specificEventIdentifier int) [4]byte {
	return [4]byte{
		byte(importance),
		byte(threadOrigin),
		byte(specificEventIdentifier),
		HeaderValFilterAcceptAll,
	}
}

/*
	System Events
*/

type SystemEventIdentifier int

const (
	SystemEventIdentifierInsiOnline  SystemEventIdentifier = HeaderValSpecificEventIdentifier_SystemBeginInclusive
	SystemEventIdentifierInsiOffline SystemEventIdentifier = HeaderValSpecificEventIdentifier_SystemBeginInclusive + 1
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
