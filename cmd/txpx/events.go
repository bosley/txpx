package main

import "time"

func ConstructEventHeader(importance int, threadOrigin int, specificEventIdentifier int) [4]byte {
	return [4]byte{byte(importance), byte(threadOrigin), byte(specificEventIdentifier), 0}
}

/*
	System Events
*/

type SystemEventIdentifier int

const (
	SystemEventIdentifierInsiOnline  SystemEventIdentifier = SpecificEventIdentifier_SystemBeginInclusive
	SystemEventIdentifierInsiOffline SystemEventIdentifier = SystemEventIdentifierInsiOnline + 1
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
