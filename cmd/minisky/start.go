package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
	"minisky/pkg/router"
	"minisky/pkg/dashboard"
	_ "minisky/pkg/shims" // Triggers all shim registrations
	"minisky/pkg/shims/appengine"
	"minisky/pkg/shims/bigquery"
	"minisky/pkg/shims/gke"
	"minisky/pkg/shims/logging"
	"minisky/pkg/shims/memorystore"
	"minisky/pkg/shims/monitoring"
	"minisky/pkg/shims/scheduler"
	"minisky/pkg/shims/serverless"
	"minisky/ui"

	"github.com/spf13/cobra"
)

var (
	apiPort string
	uiPort  string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the MiniSky Daemon and API Router",
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Starting MiniSky Daemon (API :%s, UI :%s)...", apiPort, uiPort)
		if os.Getenv("DOCKER_API_VERSION") == "" { 
			os.Setenv("DOCKER_API_VERSION", "1.44"); 
		}
		
		// Write PID file
		pidFile := ".minisky/minisky.pid"
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
			log.Printf("[WARN] Failed to write PID file: %v", err)
		}

		// ── Orchestrator boot ───────────────────────────────────────────────
		// 1. Shared LRO state machine — passed into every shim that needs async ops.
		opMgr := orchestrator.NewOperationManager()

		// 2. Docker service manager — creates the isolated minisky-net bridge network
		//    and handles cold-starting long-lived emulator containers (GCS, Pub/Sub, etc.)
		svcMgr, err := orchestrator.NewServiceManager()
		if err != nil {
			log.Fatalf("[FATAL] Cannot connect to Docker: %v", err)
		}
		ctx := context.Background()
		if err := svcMgr.EnsureNetwork(ctx); err != nil {
			log.Fatalf("[FATAL] Cannot create isolated minisky-net network: %v", err)
		}

		// ── Router ──────────────────────────────────────────────────────────
		// ── Router ──────────────────────────────────────────────────────────
		proxyRouter := router.NewProxyRouterWithManager(svcMgr)

		// ── Dynamic Registry Boot ──────────────────────────────────────────
		// This replaces the long list of manual RegisterShim calls.
		// All shims that are imported (using _ below) will self-register.
		shims, lazyDomains := registry.BootAll(opMgr, svcMgr)
		
		for domain, handler := range shims {
			proxyRouter.RegisterShim(domain, handler)
		}
		for _, domain := range lazyDomains {
			proxyRouter.RegisterLazyDocker(domain)
		}

		// Resolve shims needed for Dashboard
		logShim := shims["logging.googleapis.com"].(*logging.API)
		monShim := shims["monitoring.googleapis.com"].(*monitoring.API)
		serverlessShim := shims["cloudfunctions.googleapis.com"].(*serverless.API)
		bqAPI := shims["bigquery.googleapis.com"].(*bigquery.API)
		gkeAPI := shims["container.googleapis.com"].(*gke.API)
		appEngineAPI := shims["appengine.googleapis.com"].(*appengine.API)
		memoAPI := shims["redis.googleapis.com"].(*memorystore.API)
		schedulerAPI := shims["cloudscheduler.googleapis.com"].(*scheduler.API)

		// ── Graceful Shutdown ────────────────────────────────────────────────
		go func() {
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
			<-quit
			log.Println("⏹️  MiniSky shutting down — tearing down isolated network...")
			svcMgr.Teardown(context.Background())
			os.Remove(".minisky/minisky.pid")
			os.Exit(0)
		}()

		// ── Dashboard UI ─────────────────────────────────────────────────────
		go func() {
			addr := ":" + uiPort
			log.Printf("✨ MiniSky Dashboard available at http://localhost:%s", uiPort)
			
			uiMux := http.NewServeMux()
			
			// REST API for dynamic dashboard control
			apiHandler := dashboard.NewAPIHandler(
				svcMgr,
				bqAPI.GetBackend(),
				gkeAPI.GetBackend(),
				serverlessShim.GetBackend(),
				logShim,
				monShim,
				appEngineAPI,
				memoAPI,
				schedulerAPI,
			)
			uiMux.Handle("/api/", apiHandler)
			// Fallback to static dist
			uiMux.Handle("/", ui.Handler())
			
			if err := http.ListenAndServe(addr, uiMux); err != nil {
				log.Fatalf("UI Server crashed: %v", err)
			}
		}()

		// ── API Proxy Gateway ────────────────────────────────────────────────
		addr := ":" + apiPort
		log.Printf("🚀 MiniSky API Gateway listening on http://localhost:%s", apiPort)
		if err := http.ListenAndServe(addr, proxyRouter); err != nil {
			log.Fatalf("Failed to start router: %v", err)
		}
	},
}

func init() {
	startCmd.Flags().StringVar(&apiPort, "port", "8080", "Port for the MiniSky API Gateway (env: MINISKY_PORT)")
	startCmd.Flags().StringVar(&uiPort, "ui-port", "8081", "Port for the MiniSky Dashboard UI (env: MINISKY_UI_PORT)")

	// Allow environment variable overrides
	if p := os.Getenv("MINISKY_PORT"); p != "" {
		apiPort = p
	}
	if p := os.Getenv("MINISKY_UI_PORT"); p != "" {
		uiPort = p
	}

	rootCmd.AddCommand(startCmd)
}
