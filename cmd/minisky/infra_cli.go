package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"

	"github.com/spf13/cobra"
)

var gkeCmd = &cobra.Command{
	Use:   "gke",
	Short: "Manage Kubernetes clusters",
}

var sqlCmd = &cobra.Command{
	Use:   "sql",
	Short: "Manage Cloud SQL instances",
}

func init() {
	// GKE
	gkeCmd.AddCommand(&cobra.Command{
		Use:   "clusters list",
		Run: func(cmd *cobra.Command, args []string) {
			port := os.Getenv("MINISKY_UI_PORT")
			if port == "" { port = "8081" }
			resp, _ := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/gke/clusters", port))
			defer resp.Body.Close()
			var data struct {
				Clusters []struct {
					Name string `json:"name"`
					Status string `json:"status"`
				} `json:"clusters"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			fmt.Println("GKE CLUSTERS:")
			for _, c := range data.Clusters {
				fmt.Printf("  - %s [%s]\n", c.Name, c.Status)
			}
		},
	})

	// SQL
	sqlCmd.AddCommand(&cobra.Command{
		Use:   "instances list",
		Run: func(cmd *cobra.Command, args []string) {
			port := os.Getenv("MINISKY_UI_PORT")
			if port == "" { port = "8081" }
			resp, _ := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/cloudsql/instances", port))
			defer resp.Body.Close()
			var data struct {
				Items []struct {
					Name string `json:"name"`
					State string `json:"state"`
				} `json:"items"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			fmt.Println("CLOUDSQL INSTANCES:")
			for _, i := range data.Items {
				fmt.Printf("  - %s [%s]\n", i.Name, i.State)
			}
		},
	})

	rootCmd.AddCommand(gkeCmd)
	rootCmd.AddCommand(sqlCmd)
}
