package cmd

import (
	db "github.com/Cloud-Pie/SPDT/storage"
	"github.com/spf13/cobra"
	"fmt"
	"github.com/Cloud-Pie/SPDT/util"
)

// policiesCmd represents the delete policies command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove policy",
	Long: "Remove a stored policy",
	Run: delete,
}

var (
	force bool
	id	string
)

func init() {
	deleteCmd.Flags().StringVar(&id, "pId", "", "Policy ID")
	deleteCmd.Flags().BoolVar(&force,"f", false, "Force the action")
	deleteCmd.Flags().String("config-file", "config.yml", "Configuration file path")
}

func delete(cmd *cobra.Command, args []string) {
	if force {
		configFile := cmd.Flag("config-file").Value.String()
		systemConfiguration,_ := util.ReadConfigFile(configFile)
		policyDAO := db.GetPolicyDAO(systemConfiguration.MainServiceName)
		err := policyDAO.DeleteById(id)
		if err != nil {
			fmt.Println("Error, policy could not be deleted")
			fmt.Println(err.Error())
		} else {
			fmt.Println("Policy deleted")
		}
	} else {
		fmt.Println("Are you sure you want to delete this policy?, use the -f flag to force it")
	}
}
