package cmd

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/spf13/cobra"
)

// createUserCmd represents the createUser command
var createUserCmd = &cobra.Command{
	Use:     "user",
	Aliases: []string{"users", "u"},
	Short:   "Creates a new user",
	RunE: func(cmd *cobra.Command, args []string) error {
		email, err := cmd.Flags().GetString("email")
		if err != nil {
			return fmt.Errorf("error getting 'email' flag: %v", err)
		}

		password, err := cmd.Flags().GetString("password")
		if err != nil {
			return fmt.Errorf("error getting 'password' flag: %v", err)
		}

		validPassword := auth.CheckPasswordPolicy(password)
		if validPassword != nil {
			return fmt.Errorf("invalid password: %v", validPassword)
		}

		user := &db.User{
			Email:        email,
			PasswordHash: auth.GeneratePassword(password),
			Active:       true,
		}

		user, err = db.Connection.CreateUser(user)
		if err != nil {
			return fmt.Errorf("error creating user: %v", err)
		}

		fmt.Println("User created successfully!")
		return nil
	},
}

func init() {
	createCmd.AddCommand(createUserCmd)

	// Here you will define your flags and configuration settings.
	createUserCmd.Flags().StringP("email", "e", "", "Email for the new user (required)")
	createUserCmd.Flags().StringP("password", "p", "", "Password for the new user (required)")

	cobra.CheckErr(createUserCmd.MarkFlagRequired("email"))
	cobra.CheckErr(createUserCmd.MarkFlagRequired("password"))
}
