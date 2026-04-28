package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var arCmd = &cobra.Command{
	Use:   "artifact-registry",
	Short: "Manage Artifact Registry repositories",
}

var cbCmd = &cobra.Command{
	Use:   "cloud-build",
	Short: "Manage Cloud Build workflows",
}

var vertexCmd = &cobra.Command{
	Use:   "vertex-ai",
	Short: "Manage Vertex AI models and providers",
}

func init() {
	// Artifact Registry
	arCmd.AddCommand(&cobra.Command{
		Use:   "repositories list",
		Run: func(cmd *cobra.Command, args []string) {
			port := os.Getenv("MINISKY_UI_PORT")
			if port == "" { port = "8081" }
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/artifactregistry/repositories", port))
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()
			var data struct {
				Repositories []struct {
					Name string `json:"name"`
				} `json:"repositories"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			fmt.Println("ARTIFACT REGISTRY REPOSITORIES:")
			for _, r := range data.Repositories {
				fmt.Printf("  - %s\n", r.Name)
			}
		},
	})

	// Cloud Build
	cbCmd.AddCommand(&cobra.Command{
		Use:   "builds list",
		Run: func(cmd *cobra.Command, args []string) {
			port := os.Getenv("MINISKY_UI_PORT")
			if port == "" { port = "8081" }
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/cloudbuild/builds", port))
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()
			var data struct {
				Builds []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"builds"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			fmt.Println("CLOUD BUILD HISTORY:")
			for _, b := range data.Builds {
				fmt.Printf("  - [%s] %s\n", b.Status, b.ID)
			}
		},
	})

	// Vertex AI
	vertexCmd.AddCommand(&cobra.Command{
		Use:   "models list",
		Run: func(cmd *cobra.Command, args []string) {
			port := os.Getenv("MINISKY_UI_PORT")
			if port == "" { port = "8081" }
			resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/vertexai/internal/models", port))
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()
			var data struct {
				Models []string `json:"models"`
			}
			json.NewDecoder(resp.Body).Decode(&data)
			fmt.Println("AVAILABLE AI MODELS:")
			for _, m := range data.Models {
				fmt.Printf("  - %s\n", m)
			}
		},
	})

	rootCmd.AddCommand(arCmd)
	rootCmd.AddCommand(cbCmd)
	rootCmd.AddCommand(vertexCmd)
}
