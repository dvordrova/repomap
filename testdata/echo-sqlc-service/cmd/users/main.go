package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"example.com/echo-sqlc-service/internal/app"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "users",
		Short: "Work with users",
	}
	root.AddCommand(newGetCommand())
	return root
}

func newGetCommand() *cobra.Command {
	var id int64
	command := &cobra.Command{
		Use:   "get",
		Short: "Get a user by ID",
		RunE: func(command *cobra.Command, _ []string) error {
			users, err := app.NewUsers()
			if err != nil {
				return err
			}
			defer users.Close()

			user, err := users.Service.GetUser(context.Background(), id)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(user)
		},
	}
	command.Flags().Int64Var(&id, "id", 0, "user ID")
	return command
}
