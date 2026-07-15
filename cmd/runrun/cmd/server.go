package cmd

import (
	"context"
	"errors"
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

// Default HTTP server tuning and validation limits.
const (
	defaultShutdownTimeoutSeconds = 30
	defaultReadTimeout            = 15 * time.Second
	defaultWriteTimeout           = 15 * time.Second
	defaultIdleTimeout            = 60 * time.Second
	minJWTSecretLength            = 32
	minPort                       = 1
	maxPort                       = 65535
)

// Sentinel errors returned by configuration validation.
var (
	errInvalidPort        = errors.New("invalid port number (must be 1-65535)")
	errNoUsersConfigured  = errors.New("no users configured - at least one user is required")
	errTaskNoName         = errors.New("task has no name")
	errTaskNoSteps        = errors.New("task has no steps")
	errStepNoName         = errors.New("task step has no name")
	errStepNoCommand      = errors.New("task step has no command")
	errUserNoUsername     = errors.New("user has empty username")
	errUserNoPasswordHash = errors.New("user has no password hash")
	errJWTSecretTooShort  = errors.New("JWT secret must be at least 32 characters")
)

var (
	configFile      string
	serverPort      int
	logLevel        string
	shutdownTimeout int
)

// serverCmd represents the server command.
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
	serverCmd.Flags().IntVar(&shutdownTimeout, "shutdown-timeout", defaultShutdownTimeoutSeconds,
		"Graceful shutdown timeout in seconds")
}

func runServer(_ *cobra.Command, _ []string) error {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	// Create server
	srv := server.New(cfg)
	srv.SetupRoutes()

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
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

	return waitForShutdown(httpServer, serverErrors, shutdown)
}

// loadAndValidateConfig loads the configuration file, applies the port
// override flag if set, and validates the result.
func loadAndValidateConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override port if specified via flag
	if serverPort > 0 {
		cfg.Server.Port = serverPort
	}

	log.Printf("Configuration loaded successfully from %s", configFile)
	log.Printf("Server will run on port %d", cfg.Server.Port)
	log.Printf("Loaded %d tasks, %d users", len(cfg.Tasks), len(cfg.Auth.Users))

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// waitForShutdown blocks until the HTTP server fails or a shutdown signal is
// received, then performs a graceful (falling back to forced) shutdown.
func waitForShutdown(httpServer *http.Server, serverErrors <-chan error, shutdown <-chan os.Signal) error {
	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// validateConfig performs startup validation on the configuration.
func validateConfig(cfg *config.Config) error {
	if cfg.Server.Port < minPort || cfg.Server.Port > maxPort {
		return fmt.Errorf("port %d: %w", cfg.Server.Port, errInvalidPort)
	}

	// Validate that at least one task is configured
	if len(cfg.Tasks) == 0 {
		log.Println("WARNING: No tasks configured")
	}

	if len(cfg.Auth.Users) == 0 {
		return errNoUsersConfigured
	}

	if err := validateTasks(cfg.Tasks); err != nil {
		return err
	}

	if err := validateUsers(cfg.Auth.Users); err != nil {
		return err
	}

	if len(cfg.Auth.JWTSecret) < minJWTSecretLength {
		return errJWTSecretTooShort
	}

	log.Println("Configuration validation passed")
	return nil
}

// validateTasks validates each configured task and its steps.
func validateTasks(tasks []config.Task) error {
	for i, task := range tasks {
		if task.Name == "" {
			return fmt.Errorf("task %d: %w", i, errTaskNoName)
		}
		if len(task.Steps) == 0 {
			return fmt.Errorf("task '%s': %w", task.Name, errTaskNoSteps)
		}
		if err := validateSteps(task.Name, task.Steps); err != nil {
			return err
		}
	}
	return nil
}

// validateSteps validates each step of a task.
func validateSteps(taskName string, steps []config.Step) error {
	for j, step := range steps {
		if step.Name == "" {
			return fmt.Errorf("task '%s' step %d: %w", taskName, j, errStepNoName)
		}
		if step.Command == "" {
			return fmt.Errorf("task '%s' step '%s': %w", taskName, step.Name, errStepNoCommand)
		}
	}
	return nil
}

// validateUsers validates each configured user.
func validateUsers(users []config.User) error {
	for i, user := range users {
		if user.Username == "" {
			return fmt.Errorf("user %d: %w", i, errUserNoUsername)
		}
		if user.Password == "" {
			return fmt.Errorf("user '%s': %w", user.Username, errUserNoPasswordHash)
		}
	}
	return nil
}
