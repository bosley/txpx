package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bosley/txpx/cmd/txpx/config"
	"github.com/bosley/txpx/pkg/app"
)

type cliSubcommand struct {
	description string
	action      func(app *AppTxPx, remainingArgs []string, application app.AppRuntime)
}

type cliCommand struct {
	description string
	subcommands map[string]cliSubcommand
}

var cmdMap = map[string]cliCommand{
	"app": {
		description: "App commands",
		subcommands: map[string]cliSubcommand{
			"ping": {
				description: "Ping the app",
				action:      subCmdAppPing,
			},
			"status": {
				description: "Get the status of the app",
				action:      subCmdAppStatus,
			},
			"shutdown": {
				description: "Shutdown the app",
				action:      subCmdAppShutdown,
			},
		},
	},
}

func showCliUsage() {
	fmt.Printf("Usage: txpx <command> <subcommand> [options]\n")
	for name, cmd := range cmdMap {

		fmt.Printf(`
%s 	- %s`, name, cmd.description)

		for subcmdName, subcmd := range cmd.subcommands {
			fmt.Printf(`
    %s 	- %s`, subcmdName, subcmd.description)
		}
	}
}

func executeCliArguments(args []string, app *AppTxPx, application app.AppRuntime) error {

	if len(args) == 0 {
		fmt.Printf("No arguments provided\n")
		return errors.New("no arguments provided")
	}

	if len(args) == 1 {
		showCliUsage()
		if args[0] == "help" {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if _, ok := cmdMap[args[0]].subcommands[args[1]]; !ok {
		fmt.Printf("Invalid command\n")
		showCliUsage()
		os.Exit(1)
	}

	cmdMap[args[0]].subcommands[args[1]].action(app, args[2:], application)
	return nil
}

func shouldSkipTLSVerify(config *config.Config) bool {
	return !config.Prod
}

func subCmdAppPing(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	if err := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).Ping(); err != nil {
		fmt.Println("App ping failed:", err)
	} else {
		fmt.Println("App ping successful")
	}
}

func subCmdAppStatus(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	status := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).Status()
	if status != "" {
		fmt.Println("Status:", status)
	} else {
		fmt.Println("Failed to get status")
	}
}

func subCmdAppShutdown(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	msg, err := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).Shutdown()
	if err != nil {
		fmt.Println("App shutdown failed:", err)
		return
	}
	fmt.Println("Message:", msg)
}
