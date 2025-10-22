package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgaunet/runrun/internal/auth"
)

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Define CLI commands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			hashPasswordCommand()
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "version", "--version", "-v":
			fmt.Println("runrun v1.0.0")
			return
		}
	}

	// Default: Run server
	runServer()
}

func hashPasswordCommand() {
	hashCmd := flag.NewFlagSet("hash-password", flag.ExitOnError)
	cost := hashCmd.Int("cost", auth.DefaultCost, "BCrypt cost factor (4-31)")

	hashCmd.Parse(os.Args[2:])

	args := hashCmd.Args()
	if len(args) == 0 {
		fmt.Println("Usage: runrun hash-password [flags] <password>")
		fmt.Println("\nFlags:")
		hashCmd.PrintDefaults()
		fmt.Println("\nExample:")
		fmt.Println("  runrun hash-password mypassword")
		fmt.Println("  runrun hash-password --cost=12 mypassword")
		os.Exit(1)
	}

	password := args[0]

	// Validate password
	if err := auth.ValidatePasswordStrength(password); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Hash password
	hash, err := auth.HashPasswordWithCost(password, *cost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(hash)
}

func printHelp() {
	fmt.Println("RunRun - Task Execution Platform")
	fmt.Println("\nUsage:")
	fmt.Println("  runrun [command] [flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  (none)          Start the web server (default)")
	fmt.Println("  hash-password   Generate BCrypt hash for a password")
	fmt.Println("  help            Show this help message")
	fmt.Println("  version         Show version information")
	fmt.Println("\nFlags for server:")
	fmt.Println("  --config        Path to configuration file (default: config.yaml)")
	fmt.Println("\nExamples:")
	fmt.Println("  runrun")
	fmt.Println("  runrun --config=/path/to/config.yaml")
	fmt.Println("  runrun hash-password mypassword")
}

func runServer() {
	// Create HTTP server mux
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Create server
	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on port 8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down...")
}
