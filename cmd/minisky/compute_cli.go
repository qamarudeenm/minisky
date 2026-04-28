package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"

	"github.com/spf13/cobra"
)

var computeCmd = &cobra.Command{
	Use:   "compute",
	Short: "Manage Compute Engine resources",
}

var instancesCmd = &cobra.Command{
	Use:   "instances",
	Short: "Manage GCE instances",
}

var listInstancesCmd = &cobra.Command{
	Use:   "list",
	Short: "List GCE instances",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/compute/instances", port))
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var data struct {
			Items []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"items"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		fmt.Println("COMPUTE INSTANCES:")
		if len(data.Items) == 0 {
			fmt.Println("  (None)")
			return
		}
		for _, i := range data.Items {
			fmt.Printf("  - %s [%s]\n", i.Name, i.Status)
		}
	},
}

func init() {
	instancesCmd.AddCommand(listInstancesCmd)
	computeCmd.AddCommand(instancesCmd)
	rootCmd.AddCommand(computeCmd)
}
