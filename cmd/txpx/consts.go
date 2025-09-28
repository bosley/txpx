package main

const (
	EventTopicTxPxSystem     = "txpx-system"
	EventTopicTxPxMainApp    = "txpx-main-app"
	EventTopicTxPxHttpServer = "txpx-http-server"

	EventTopicTX = "tx"
	EventTopicPX = "px"
)

/*
Convention:

	L->R
	0) Importance (highest is 255)
	1) Thread/System Origin
	3) Specific System Event Identifier
	4) Filter - 0 = all, 1 = none (might cahnge later to be more specific)
*/

const (
	HeaderValImportanceHighest = 255
	HeaderValImportanceHigh    = 128
	HeaderValImportanceMedium  = 64
	HeaderValImportanceLow     = 32
	HeaderValImportanceLowest  = 16

	HeaderValThreadOriginTxPxApi          = 0
	ThreadOriginTxPxMainApp               = 1
	HeaderValThreadOriginTxPxHttpServer   = 2
	HeaderValThreadOriginTxPxSideCar      = 3
	HeaderValThreadOriginTxPxInsi         = 4
	HeaderValThreadOriginTxPxEvents       = 5
	HeaderValThreadOriginTxPxSystem       = 6
	HeaderValThreadOriginTxPxRemoteOrigin = 6
	HeaderValThreadOriginTxPxOther        = 255

	HeaderValSpecificEventTxPxApiToMainApp = 0

	HeaderValFilterAcceptAll  = 0
	HeaderValFilterAcceptNone = 1
)

const (
	HeaderValSpecificEventIdentifier_AppApiBeginInclusive = 0
	HeaderValSpecificEventIdentifier_AppApiEndExclusive   = 32

	// 0-32 is reserved for backend app api usage
	HeaderValSpecificEventIdentifier_SystemBeginInclusive = 32 // Start of system events reserved range
	HeaderValSpecificEventIdentifier_SystemEndExclusive   = 64 // End of system events reserved range

	// TODO: Other category specific identifiers

	HeaderValSpecificEventIdentifier_ExternalBeginInclusive = 128 // Start of external events reserved range
	HeaderValSpecificEventIdentifier_ExternalEndExclusive   = 255 // End of external events reserved range

)

const (
	ConfigFileClusterDefault = "cluster.yaml"
	ConfigFileSiteDefault    = "site.yaml"
)
