package main

import (
	"fmt"
	"log"
	"minisky/pkg/config"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stops the MiniSky Daemon",
	Run: func(cmd *cobra.Command, args []string) {
		pidFile := filepath.Join(config.GetMiniskyDir(), "minisky.pid")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			log.Fatalf("MiniSky is not running (PID file missing: %s)", pidFile)
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			log.Fatalf("Invalid PID in %s: %v", pidFile, err)
		}

		log.Printf("Stopping MiniSky (PID %d)...", pid)
		
		process, err := os.FindProcess(pid)
		if err != nil {
			log.Fatalf("Failed to find process %d: %v", pid, err)
		}

		// Send SIGTERM for graceful shutdown
		if err := process.Signal(syscall.SIGTERM); err != nil {
			log.Fatalf("Failed to signal process %d: %v", pid, err)
		}

		fmt.Println("✅ Stop signal sent.")
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
