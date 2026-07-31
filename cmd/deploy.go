/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentstacks/v2"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var deploymentName string
var armTemplatePath string
var excludedActions []string
var parameters []string
var ctx = context.Background()

// deployCmd represents the deploy command
var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploys the bicep to azure",
	Long: `
		Deploy an ARM JSON template as an Azure deployment stack.

		Build Bicep before running this command, then pass the generated ARM JSON file:

  		az bicep build \
    		--file ../infra/main.bicep \
      		--outfile ../infra/main.json

        dh deploy \
        	--deployment-name workload-main \
         	--arm-template ../infra/main.json \
          	--parameter workloadName=example-api \
           	--parameter imageTag=2026.07.30.1 \
            --excluded-actions Microsoft.Authorization/locks/delete \
            --excluded-actions Microsoft.Resources/deployments/delete

        In Azure DevOps pipelines, the workload environment is derived from
        BUILD_SOURCEBRANCH, BUILD_REASON, and SYSTEM_PULLREQUEST_TARGETBRANCH.

        This command also expects the pipeline to provide the subscription, location,
        and object ID variables for the detected environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		environment, err := GetWorkloadEnvironment()
		if err != nil {
			return err
		}

		armTemplate, err := readJSON(armTemplatePath)
		if err != nil {
			return err
		}

		parsedParameters, err := parseParameters(parameters)
		if err != nil {
			return err
		}

		config, err := getConfig(environment, armTemplate, parsedParameters, excludedActions)
		if err != nil {
			return err
		}

		cred, err := azidentity.NewAzureCLICredential(nil)
		if err != nil {
			return err
		}

		clientFactory, err := armdeploymentstacks.NewClientFactory(config.SubscriptionId, cred, nil)
		if err != nil {
			return err
		}

		client := clientFactory.NewClient()

		deploy, err := client.BeginCreateOrUpdateAtSubscription(
			ctx,
			config.DeploymentName,
			armdeploymentstacks.DeploymentStack{
				Location: to.Ptr(config.Location),
				Properties: &armdeploymentstacks.DeploymentStackProperties{
					Template:         config.ARMTemplate,
					ActionOnUnmanage: &config.ActionOnUnmanage,
					DenySettings:     &config.DenySettings,
					Parameters:       config.Parameters,
				},
			},
			nil,
		)

		if err != nil {
			return err
		}

		_, err = deploy.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// deployCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// deployCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	deployCmd.Flags().StringVar(&deploymentName, "deployment-name", "", "The Deployment name for this Azure deployment stack")
	if err := deployCmd.MarkFlagRequired("deployment-name"); err != nil {
		panic(err)
	}
	deployCmd.Flags().StringVar(&armTemplatePath, "arm-template", "../infra/main.json", "Location of the ARM JSON template.")
	deployCmd.Flags().StringSliceVar(&excludedActions, "excluded-actions", nil, "Actions excluded from deny settings. Can be comma-separated or repeated.")
	deployCmd.Flags().StringArrayVar(&parameters, "parameter", nil, "ARM template parameter in key=value format. Can be repeated.")
}

type Config struct {
	SubscriptionId   string
	Location         string
	Parameters       map[string]*armdeploymentstacks.DeploymentParameter
	DeploymentName   string
	ARMTemplate      map[string]any
	ActionOnUnmanage armdeploymentstacks.ActionOnUnmanage
	DenySettings     armdeploymentstacks.DenySettings
}

func getConfig(env string, armTemplate map[string]any, parameters map[string]*armdeploymentstacks.DeploymentParameter, excludedActions []string) (Config, error) {

	var subscriptionId string
	var location string
	denySettings, err := getDenySettings(env, excludedActions)
	if err != nil {
		return Config{}, err
	}
	actionsOnUnmanage := getActionOnUnmanage()

	switch env {
	case "Production":
		subscriptionId = os.Getenv("PROD_SUB_ID")
		location = os.Getenv("PROD_DEPLOYMENT_LOCATION")
	case "Test":
		subscriptionId = os.Getenv("TEST_SUB_ID")
		location = os.Getenv("TEST_DEPLOYMENT_LOCATION")
	case "Development":
		subscriptionId = os.Getenv("DEV_SUB_ID")
		location = os.Getenv("DEV_DEPLOYMENT_LOCATION")
	default:
		return Config{}, fmt.Errorf("Unknown env: %s", env)
	}

	return Config{
		SubscriptionId:   subscriptionId,
		Location:         location,
		Parameters:       parameters,
		DeploymentName:   deploymentName,
		ARMTemplate:      armTemplate,
		DenySettings:     denySettings,
		ActionOnUnmanage: actionsOnUnmanage,
	}, nil

}

func getDenySettings(env string, excludedActions []string) (armdeploymentstacks.DenySettings, error) {
	var principalObjectID string

	switch env {
	case "Production":
		principalObjectID = os.Getenv("PROD_OBJ_ID")
	case "Test":
		principalObjectID = os.Getenv("TEST_OBJ_ID")
	case "Development":
		return armdeploymentstacks.DenySettings{
			Mode: to.Ptr(armdeploymentstacks.DenySettingsModeNone),
		}, nil
	default:
		return armdeploymentstacks.DenySettings{}, fmt.Errorf("Unknown env: %s", env)
	}

	var excludedPrincipals []*string
	if principalObjectID != "" {
		excludedPrincipals = []*string{to.Ptr(principalObjectID)}
	}

	return armdeploymentstacks.DenySettings{
		Mode:               to.Ptr(armdeploymentstacks.DenySettingsModeDenyWriteAndDelete),
		ExcludedPrincipals: excludedPrincipals,
		ExcludedActions:    toStringPointers(excludedActions),
	}, nil

}

func getActionOnUnmanage() armdeploymentstacks.ActionOnUnmanage {
	return armdeploymentstacks.ActionOnUnmanage{
		Resources:                     to.Ptr(armdeploymentstacks.UnmanageActionResourceModeDelete),
		ResourceGroups:                to.Ptr(armdeploymentstacks.UnmanageActionResourceGroupModeDelete),
		ResourcesWithoutDeleteSupport: to.Ptr(armdeploymentstacks.ResourcesWithoutDeleteSupportActionDetach),
	}
}

func parseParameters(values []string) (map[string]*armdeploymentstacks.DeploymentParameter, error) {
	parsed := make(map[string]*armdeploymentstacks.DeploymentParameter)

	for _, value := range values {
		key, parameterValue, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("invalid parameter %q, expected key=value", value)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid parameter %q, key cannot be empty", value)
		}

		parsed[key] = &armdeploymentstacks.DeploymentParameter{
			Value: parameterValue,
		}
	}

	return parsed, nil
}

func toStringPointers(values []string) []*string {
	if len(values) == 0 {
		return nil
	}

	pointers := make([]*string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		pointers = append(pointers, to.Ptr(value))
	}

	return pointers
}

func readJSON(path string) (map[string]any, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file read error: %w", err)
	}

	var content map[string]any

	err = json.Unmarshal(file, &content)
	if err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return content, nil
}
