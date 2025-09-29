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
	"users": {
		description: "User management commands",
		subcommands: map[string]cliSubcommand{
			"list": {
				description: "List all users",
				action:      subCmdUsersList,
			},
			"create": {
				description: "Create a new user (usage: users create <email> <password>)",
				action:      subCmdUsersCreate,
			},
			"delete": {
				description: "Delete a user (usage: users delete <user_uuid>)",
				action:      subCmdUsersDelete,
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

func subCmdUsersList(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	result, err := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).ListUsers()
	if err != nil {
		fmt.Println("Failed to list users:", err)
		return
	}
	fmt.Println(result)
}

func subCmdUsersCreate(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	if len(remainingArgs) < 2 {
		fmt.Println("Error: email and password are required")
		fmt.Println("Usage: users create <email> <password>")
		os.Exit(1)
	}

	email := remainingArgs[0]
	password := remainingArgs[1]

	result, err := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).CreateUser(email, password)
	if err != nil {
		fmt.Println("Failed to create user:", err)
		return
	}
	fmt.Println(result)
}

func subCmdUsersDelete(app *AppTxPx, remainingArgs []string, application app.AppRuntime) {
	if len(remainingArgs) < 1 {
		fmt.Println("Error: user_uuid is required")
		fmt.Println("Usage: users delete <user_uuid>")
		os.Exit(1)
	}

	userUUID := remainingArgs[0]

	result, err := application.GetHttpPanel().GetApiClient(shouldSkipTLSVerify(app.config)).DeleteUser(userUUID)
	if err != nil {
		fmt.Println("Failed to delete user:", err)
		return
	}
	fmt.Println(result)
}
