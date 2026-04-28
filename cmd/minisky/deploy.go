package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	deployName       string
	deployRuntime    string
	deployEntryPoint string
	deploySource     string
	deployType       string
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a serverless resource to MiniSky",
	Example: `./minisky deploy --name my-func --runtime python312 --entry-point handler --source main.py`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Read source code
		code, err := os.ReadFile(deploySource)
		if err != nil {
			fmt.Printf("❌ Error reading source file: %v\n", err)
			return
		}

		// 2. Prepare payload
		payload := map[string]interface{}{
			"type":       deployType,
			"name":       deployName,
			"runtime":    deployRuntime,
			"entryPoint": deployEntryPoint,
			"code":       string(code),
		}
		data, _ := json.Marshal(payload)

		// 3. Send to MiniSky Gateway
		port := os.Getenv("MINISKY_PORT")
		if port == "" {
			port = "8080"
		}
		url := fmt.Sprintf("http://localhost:%s/v2/deploy", port)
		
		fmt.Printf("🚀 Deploying %s '%s' to MiniSky...\n", deployType, deployName)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
		if err != nil {
			fmt.Printf("❌ Connection failed: %v (Is MiniSky running?)\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("❌ Deployment failed (%s): %s\n", resp.Status, string(body))
			return
		}

		fmt.Printf("✅ Successfully deployed '%s'!\n", deployName)
		fmt.Printf("🔗 Local URL: http://localhost:5500x (Check Dashboard for exact port)\n")
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources in MiniSky",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_PORT")
		if port == "" {
			port = "8080"
		}

		// Fetch Functions
		fmt.Println("--- Cloud Functions v2 ---")
		printResources(fmt.Sprintf("http://localhost:%s/v2/functions", port))

		// Fetch Services
		fmt.Println("\n--- Cloud Run Services ---")
		printResources(fmt.Sprintf("http://localhost:%s/v2/services", port))
	},
}

func printResources(url string) {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data struct {
		Functions []interface{} `json:"functions"`
		Services  []interface{} `json:"services"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	list := data.Functions
	if list == nil {
		list = data.Services
	}

	if len(list) == 0 {
		fmt.Println("  (None)")
		return
	}

	for _, item := range list {
		m := item.(map[string]interface{})
		name := m["name"].(string)
		state := m["state"].(string)
		fmt.Printf("  - %s [%s]\n", last(strings.Split(name, "/")), state)
	}
}

// Simple helper because strings.Split returns a slice
func last(parts []string) string {
	return parts[len(parts)-1]
}

func init() {
	deployCmd.Flags().StringVar(&deployName, "name", "", "Name of the resource")
	deployCmd.Flags().StringVar(&deployRuntime, "runtime", "python312", "Runtime (python312, nodejs22, etc.)")
	deployCmd.Flags().StringVar(&deployEntryPoint, "entry-point", "handler", "Function entry point")
	deployCmd.Flags().StringVar(&deploySource, "source", "", "Path to source code file")
	deployCmd.Flags().StringVar(&deployType, "type", "function", "Resource type (function or service)")
	
	deployCmd.MarkFlagRequired("name")
	deployCmd.MarkFlagRequired("source")

	rootCmd.AddCommand(deployCmd)
	rootCmd.AddCommand(listCmd)
}
