package update

import (
	"fmt"

	"github.com/pyneda/sukyan/db"
	"github.com/spf13/cobra"
)

var updateUserCmd = &cobra.Command{
	Use:     "user",
	Aliases: []string{"users"},
	Short:   "Modify an existing user",
	RunE: func(cmd *cobra.Command, args []string) error {
		email, err := cmd.Flags().GetString("email")
		if err != nil {
			return fmt.Errorf("error getting 'email' flag: %v", err)
		}

		// An unset flag must leave the account alone rather than write false.
		if !cmd.Flags().Changed("superuser") {
			return fmt.Errorf("nothing to update: pass --superuser=true or --superuser=false")
		}

		superuser, err := cmd.Flags().GetBool("superuser")
		if err != nil {
			return fmt.Errorf("error getting 'superuser' flag: %v", err)
		}

		user, err := db.Connection().SetUserSuperuser(email, superuser)
		if err != nil {
			return err
		}

		role := "member"
		if user.Superuser {
			role = "superuser"
		}
		fmt.Printf("%s is now a %s\n", user.Email, role)
		return nil
	},
}

func init() {
	UpdateCmd.AddCommand(updateUserCmd)

	updateUserCmd.Flags().StringP("email", "e", "", "Email of the user to update (required)")
	updateUserCmd.Flags().Bool("superuser", false, "Grant or revoke deployment-wide administration rights")

	cobra.CheckErr(updateUserCmd.MarkFlagRequired("email"))
}
