package create

import (
	"fmt"
	"net/mail"
	"syscall"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

		if _, err := mail.ParseAddress(email); err != nil {
			return fmt.Errorf("invalid email address: %v", err)
		}

		password, err := cmd.Flags().GetString("password")
		if err != nil {
			return fmt.Errorf("error getting 'password' flag: %v", err)
		}

		if password == "" {
			fmt.Print("Enter password: ")
			bytePwd, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("error reading password: %v", err)
			}
			password = string(bytePwd)
			fmt.Println()
		}

		if err := auth.CheckPasswordPolicy(password); err != nil {
			return fmt.Errorf("invalid password: %v", err)
		}

		existingUser, _ := db.Connection().GetUserByEmail(email)
		if existingUser != nil {
			return fmt.Errorf("user with email '%s' already exists", email)
		}

		passwordHash := auth.GeneratePassword(password)
		if passwordHash == "" {
			return fmt.Errorf("failed to generate password hash")
		}

		user := &db.User{
			Email:        email,
			PasswordHash: passwordHash,
			Active:       true,
		}

		user, err = db.Connection().CreateUser(user)
		if err != nil {
			return fmt.Errorf("error creating user: %v", err)
		}

		fmt.Printf("User created successfully! ID: %s\n", user.ID)
		return nil
	},
}

func init() {
	CreateCmd.AddCommand(createUserCmd)

	createUserCmd.Flags().StringP("email", "e", "", "Email for the new user (required, must be valid email format)")
	createUserCmd.Flags().StringP("password", "p", "", "Password for the new user (if omitted, will be prompted; must be at least 7 characters with letters and numbers)")

	cobra.CheckErr(createUserCmd.MarkFlagRequired("email"))
}
