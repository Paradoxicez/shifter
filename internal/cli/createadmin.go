package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// createAdminCmd is the recovery escape hatch — D-14. An operator with shell
// access can always (re-)provision an admin even if the install wizard never
// completed or every admin password has been lost. Plan 09 fills the body
// once auth.Hash and the user store land.
var (
	createAdminEmail    string
	createAdminPassword string
	createAdminReset    bool

	createAdminCmd = &cobra.Command{
		Use:   "create-admin",
		Short: "Create or reset an admin user (recovery escape hatch — D-14)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if createAdminEmail == "" || createAdminPassword == "" {
				return errors.New("--email and --password are required")
			}
			// TODO(plan-09): wire auth.Hash + db.InsertAdminUser /
			// UpdateUserPassword. See Plan 09 §"create-admin recovery flow"
			// — once that plan implements auth.Hash and the user store,
			// replace this body with the full create-or-reset path.
			return errors.New("create-admin: pending Plan 09 implementation")
		},
	}
)

func init() {
	createAdminCmd.Flags().StringVar(&createAdminEmail, "email", "", "Admin email")
	createAdminCmd.Flags().StringVar(&createAdminPassword, "password", "", "New password")
	createAdminCmd.Flags().BoolVar(&createAdminReset, "reset", false, "Reset existing admin's password instead of creating a new one")
}
