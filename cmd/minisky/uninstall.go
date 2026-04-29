package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstalls MiniSky (removes containers, networks, and data)",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("🛑 Uninstalling MiniSky...")

		// Stop daemon if running
		pidFile := filepath.Join(config.GetMiniskyDir(), "minisky.pid")
		if _, err := os.Stat(pidFile); err == nil {
			log.Println("Found running daemon, stopping it first...")
			stopCmd.Run(cmd, args)
		}

		svcMgr, err := orchestrator.NewServiceManager()
		if err == nil {
			log.Println("🧹 Cleaning up Docker containers and networks...")
			svcMgr.Teardown(context.Background())
		} else {
			log.Printf("⚠️ Could not connect to Docker to clean up containers: %v", err)
		}

		// Delete ~/.minisky
		miniskyDir := config.GetMiniskyDir()
		log.Printf("🗑️ Deleting data directory: %s", miniskyDir)
		if err := os.RemoveAll(miniskyDir); err != nil {
			log.Printf("⚠️ Failed to delete %s: %v", miniskyDir, err)
		} else {
			log.Println("✅ Data directory removed.")
		}

		exe, err := os.Executable()
		if err == nil {
			log.Printf("✨ Uninstall complete! You can now safely delete the executable at: %s", exe)
		} else {
			log.Println("✨ Uninstall complete! You can now safely delete the minisky binary.")
		}
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
