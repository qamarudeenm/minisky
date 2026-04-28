package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"

	"github.com/spf13/cobra"
)

var pubsubCmd = &cobra.Command{
	Use:   "pubsub",
	Short: "Manage Pub/Sub resources",
}

var topicsCmd = &cobra.Command{
	Use:   "topics",
	Short: "Manage Pub/Sub topics",
}

var listTopicsCmd = &cobra.Command{
	Use:   "list",
	Short: "List Pub/Sub topics",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/pubsub/topics", port))
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var data struct {
			Topics []struct {
				Name string `json:"name"`
			} `json:"topics"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		fmt.Println("PUB/SUB TOPICS:")
		if len(data.Topics) == 0 {
			fmt.Println("  (None)")
			return
		}
		for _, t := range data.Topics {
			fmt.Printf("  - %s\n", t.Name)
		}
	},
}

func init() {
	topicsCmd.AddCommand(listTopicsCmd)
	pubsubCmd.AddCommand(topicsCmd)
	rootCmd.AddCommand(pubsubCmd)
}
