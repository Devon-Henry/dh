/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

// getenvCmd represents the getenv command
var getenvCmd = &cobra.Command{
	Use:   "getenv",
	Short: "returns the current workload environment",
	Long: `
	Call this command to get the current pipeline runs workload environment.
	
	Returns one of:
	- Production
	- Test
	- Development

	This is derived from the Build Reason and the Build Source Branch, alongside the target of a pull request using conditionals.

	can be tested locally with variation of

	BUILD_SOURCEBRANCH=refs/heads/main BUILD_REASON=Manual go run main.go getenv
	`,
	Run: func(cmd *cobra.Command, args []string) {

		environment, err := GetWorkloadEnvironment()
		if err != nil {
			log.Fatal(err)
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), environment)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(getenvCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// getenvCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// getenvCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func GetWorkloadEnvironment() (string, error) {
	sourceBranch := os.Getenv("BUILD_SOURCEBRANCH")
	buildReason := os.Getenv("BUILD_REASON")
	targetBranch := os.Getenv("SYSTEM_PULLREQUEST_TARGETBRANCH")

	if buildReason == "" || sourceBranch == "" {
		return "", fmt.Errorf("Required Env variables are not set. This command can only be run in an Azure DevOps Pipeline")
	}

	var env string 
	
	if sourceBranch == "refs/heads/main" {
		env = "Production"
	} else if buildReason == "PullRequest" && targetBranch == "refs/heads/main" {
		env = "Test"
	} else {
		env = "Development"
	}

	return env, nil
}