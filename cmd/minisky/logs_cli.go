package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Manage Cloud Logging",
}

var tailLogsCmd = &cobra.Command{
	Use:   "tail",
	Short: "Tail logs in real-time from MiniSky",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		fmt.Println("🛰️  Streaming MiniSky logs (Ctrl+C to stop)...")
		lastSeenId := ""

		for {
			url := fmt.Sprintf("http://localhost:%s/api/manage/logging/entries", port)
			resp, err := http.Get(url)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}

			var entries []map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&entries)
			resp.Body.Close()

			// Print new entries
			foundLast := lastSeenId == ""
			newLastSeen := lastSeenId
			
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				id := e["insertId"].(string)
				
				if id == lastSeenId {
					foundLast = true
					continue
				}

				if foundLast {
					ts := e["timestamp"].(string)
					severity := e["severity"].(string)
					text := e["textPayload"].(string)
					res := e["resource"].(map[string]interface{})
					labels := res["labels"].(map[string]interface{})
					name := labels["name"].(string)

					fmt.Printf("[%s] %s | %s: %s\n", ts, severity, name, text)
					newLastSeen = id
				}
			}
			lastSeenId = newLastSeen
			time.Sleep(1 * time.Second)
		}
	},
}

func init() {
	logsCmd.AddCommand(tailLogsCmd)
	rootCmd.AddCommand(logsCmd)
}
