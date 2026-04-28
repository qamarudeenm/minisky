package main

import (
	"fmt"
	"net/http"
	"os"
	"encoding/json"

	"github.com/spf13/cobra"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage Cloud Storage resources",
}

var storageBucketsCmd = &cobra.Command{
	Use:   "buckets",
	Short: "Manage storage buckets",
}

var listBucketsCmd = &cobra.Command{
	Use:   "list",
	Short: "List storage buckets",
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("MINISKY_UI_PORT")
		if port == "" { port = "8081" }
		
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/api/manage/storage/b", port))
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			return
		}
		defer resp.Body.Close()

		var data struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		json.NewDecoder(resp.Body).Decode(&data)

		fmt.Println("STORAGE BUCKETS:")
		if len(data.Items) == 0 {
			fmt.Println("  (None)")
			return
		}
		for _, b := range data.Items {
			fmt.Printf("  - gs://%s\n", b.Name)
		}
	},
}

func init() {
	storageBucketsCmd.AddCommand(listBucketsCmd)
	storageCmd.AddCommand(storageBucketsCmd)
	rootCmd.AddCommand(storageCmd)
}
