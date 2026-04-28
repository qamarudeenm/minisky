package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"
	"bytes"

	"github.com/spf13/cobra"
)

var spannerCmd = &cobra.Command{
	Use:   "spanner",
	Short: "Manage Spanner resources",
}

var spannerProject string

var spannerInstancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "Manage Spanner instances",
}

var listSpannerInstancesCmd = &cobra.Command{
	Use:   "list",
	Short: "List Spanner instances",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/spanner/projects/%s/instances", port, spannerProject))
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var data struct {
			Instances []struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
				State       string `json:"state"`
			} `json:"instances"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		fmt.Println("SPANNER INSTANCES (Project: " + spannerProject + "):")
		if len(data.Instances) == 0 {
			fmt.Println("  (None)")
			return
		}
		for _, i := range data.Instances {
			fmt.Printf("  - %s (%s) [%s]\n", i.DisplayName, i.Name, i.State)
		}
	},
}

var createSpannerInstanceCmd = &cobra.Command{
	Use:   "create [instance-id]",
	Short: "Create a Spanner instance",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		id := args[0]
		
		payload := map[string]interface{}{
			"instanceId": id,
			"instance": map[string]string{
				"displayName": id,
				"config":      "projects/" + spannerProject + "/instanceConfigs/emulator-config",
			},
		}
		data, _ := json.Marshal(payload)
		
		resp, err := http.Post(fmt.Sprintf("http://localhost:%s/api/manage/spanner/projects/%s/instances", port, spannerProject), "application/json", bytes.NewBuffer(data))
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			fmt.Println("❌ Failed to create instance")
			return
		}
		fmt.Printf("✅ Instance '%s' created in project '%s'\n", id, spannerProject)
	},
}

func init() {
	spannerCmd.PersistentFlags().StringVar(&spannerProject, "project", "local-dev-project", "GCP Project ID")
	spannerInstancesCmd.AddCommand(listSpannerInstancesCmd)
	spannerInstancesCmd.AddCommand(createSpannerInstanceCmd)
	spannerCmd.AddCommand(spannerInstancesCmd)
	rootCmd.AddCommand(spannerCmd)
}
