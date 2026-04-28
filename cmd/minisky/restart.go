package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restarts the MiniSky Daemon",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🔄 Restarting MiniSky...")

		// 1. Stop the process
		stopCmd.Run(cmd, args)

		// 2. Wait a moment for ports to clear
		fmt.Print("Waiting for cleanup...")
		for i := 0; i < 5; i++ {
			time.Sleep(500 * time.Millisecond)
			fmt.Print(".")
		}
		fmt.Println()

		// 3. Start the process
		// Note: We use the same binary that is running.
		// If running via 'go run', this might be tricky, but for the compiled binary it works.
		executable, _ := os.Executable()
		
		newCmd := exec.Command(executable, "start")
		newCmd.Stdout = os.Stdout
		newCmd.Stderr = os.Stderr
		
		fmt.Println("🚀 Starting new instance...")
		if err := newCmd.Start(); err != nil {
			log.Fatalf("Failed to restart: %v", err)
		}
		
		fmt.Printf("✅ MiniSky restarted (PID: %d)\n", newCmd.Process.Pid)
		fmt.Println("Note: This process is now running in the background.")
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
