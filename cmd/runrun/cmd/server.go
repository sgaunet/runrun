package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgaunet/runrun/internal/config"
	"github.com/sgaunet/runrun/internal/server"
	"github.com/spf13/cobra"
)

var (
	configFile      string
	serverPort      int
	logLevel        string
	shutdownTimeout int
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the RunRun HTTP server",
	Long: `Start the RunRun HTTP server to provide a web interface for
task execution and monitoring.

The server will:
  - Load configuration from the specified file
  - Initialize task executor workers
  - Start the HTTP server
  - Handle graceful shutdown on SIGINT/SIGTERM`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)

	// Server-specific flags
	serverCmd.Flags().StringVarP(&configFile, "config", "c", "configs/example.yaml", "Path to configuration file")
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 0, "Server port (overrides config file)")
	serverCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	serverCmd.Flags().IntVar(&shutdownTimeout, "shutdown-timeout", 30, "Graceful shutdown timeout in seconds")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override port if specified via flag
	if serverPort > 0 {
		cfg.Server.Port = serverPort
	}

	log.Printf("Configuration loaded successfully from %s", configFile)
	log.Printf("Server will run on port %d", cfg.Server.Port)
	log.Printf("Loaded %d tasks, %d users", len(cfg.Tasks), len(cfg.Auth.Users))

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create server
	srv := server.New(cfg)
	srv.SetupRoutes()

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Channel to listen for errors from the HTTP server
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on %s", addr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-shutdown:
		log.Printf("Received signal: %v. Starting graceful shutdown...", sig)

		// Create context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(shutdownTimeout)*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
			// Force close if graceful shutdown fails
			if closeErr := httpServer.Close(); closeErr != nil {
				return fmt.Errorf("failed to force close server: %w", closeErr)
			}
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}

		log.Println("Server shutdown completed successfully")
	}

	return nil
}

// validateConfig performs startup validation on the configuration
func validateConfig(cfg *config.Config) error {
	// Validate port range
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid port number: %d (must be 1-65535)", cfg.Server.Port)
	}

	// Validate that at least one task is configured
	if len(cfg.Tasks) == 0 {
		log.Println("WARNING: No tasks configured")
	}

	// Validate that at least one user is configured
	if len(cfg.Auth.Users) == 0 {
		return fmt.Errorf("no users configured - at least one user is required")
	}

	// Validate task configurations
	for i, task := range cfg.Tasks {
		if task.Name == "" {
			return fmt.Errorf("task %d has no name", i)
		}
		if len(task.Steps) == 0 {
			return fmt.Errorf("task '%s' has no steps", task.Name)
		}
		// Validate each step
		for j, step := range task.Steps {
			if step.Name == "" {
				return fmt.Errorf("task '%s' step %d has no name", task.Name, j)
			}
			if step.Command == "" {
				return fmt.Errorf("task '%s' step '%s' has no command", task.Name, step.Name)
			}
		}
	}

	// Validate user configurations
	for i, user := range cfg.Auth.Users {
		if user.Username == "" {
			return fmt.Errorf("user %d has empty username", i)
		}
		if user.Password == "" {
			return fmt.Errorf("user '%s' has no password hash", user.Username)
		}
	}

	// Validate JWT secret
	if len(cfg.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters")
	}

	log.Println("Configuration validation passed")
	return nil
}
