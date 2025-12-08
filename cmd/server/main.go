// Package main is the entry point for the Hermes chat server
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"Hermes/gen/chat/v1/chatv1connect"
	"Hermes/internal/chat"
	"Hermes/internal/config"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: %v (using defaults)", err)
		// Continue with defaults for development
	}

	// Create chat server with default in-memory implementations
	chatServer := chat.NewServerWithDefaults()

	// Set up HTTP handler
	mux := http.NewServeMux()
	path, handler := chatv1connect.NewChatServiceHandler(chatServer)
	mux.Handle(path, handler)

	// Create HTTP server with h2c (HTTP/2 cleartext) support
	server := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down server...")
		if err := server.Close(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	// Start server
	log.Printf("Starting Hermes chat server on %s", cfg.Server.Address())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}
