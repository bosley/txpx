package main

import (
	"errors"
	"fmt"
)

var cmdMap = map[string]map[string]func(app *AppTxPx, remainingArgs []string){
	"app": {
		"ping":     subCmdAppPing,
		"status":   subCmdAppStatus,
		"shutdown": subCmdAppShutdown,
	},
}

func executeCliArguments(args []string, app *AppTxPx) error {

	if len(args) == 0 {
		fmt.Println("No arguments provided")
		return errors.New("no arguments provided")
	}

	if _, ok := cmdMap[args[0]][args[1]]; !ok {
		return errors.New("invalid command")
	}

	cmdMap[args[0]][args[1]](app, args[2:])
	return nil
}

func subCmdAppPing(app *AppTxPx, remainingArgs []string) {
	// TODO: make the client lib for the app backend, and make it so that an un-launched
	// instance of an app candidate can be used to query the active runtime (another instance
	// of the app) for its status.
	//
	// For now, we'll just print the status of the app backend.
	fmt.Println("App ping")
}

func subCmdAppStatus(app *AppTxPx, remainingArgs []string) {
	// TODO: make the client lib for the app backend, and make it so that an un-launched
	// instance of an app candidate can be used to query the active runtime (another instance
	// of the app) for its status.
	//
	// For now, we'll just print the status of the app backend.
	fmt.Println("App status")
}

func subCmdAppShutdown(app *AppTxPx, remainingArgs []string) {
	// TODO: make the client lib for the app backend, and make it so that an un-launched
	// instance of an app candidate can be used to query the active runtime (another instance
	// of the app) for its status.
	//
	// For now, we'll just print the status of the app backend.
	fmt.Println("App shutdown")
}
