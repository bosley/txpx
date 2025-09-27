package main

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
	SpecificEventIdentifier_SystemBeginInclusive = 0  // Start of system events reserved range
	SpecificEventIdentifier_SystemEndExclusive   = 32 // End of system events reserved range

	// TODO: Other category specific identifiers

	SpecificEventIdentifier_ExternalBeginInclusive = 128 // Start of external events reserved range
	SpecificEventIdentifier_ExternalEndExclusive   = 255 // End of external events reserved range

)

const (
	ConfigFileClusterDefault = "cluster.yaml"
	ConfigFileSiteDefault    = "site.yaml"
)
