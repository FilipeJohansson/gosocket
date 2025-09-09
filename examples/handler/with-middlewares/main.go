//go:build example

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
)

func main() {
	handler := handler.New(
		handler.WithMiddleware(LoggingMiddleware),
		handler.WithMiddleware(AuthMiddleware),
		handler.OnConnect(func(client *gosocket.Client, ctx *handler.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", handler) // middlewares applied automatically

	server := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	log.Fatal(server.ListenAndServe())
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fmt.Printf("[%s] %s %s - START\n", start.Format("15:04:05"), r.Method, r.URL.Path)

		next.ServeHTTP(w, r)

		duration := time.Since(start)
		fmt.Printf("[%s] %s %s - END (took %v)\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, duration)
	})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			fmt.Println("[AUTH] No token provided")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		fmt.Printf("[AUTH] Token validated: %s\n", token)
		// here you could add more complex token validation logic

		next.ServeHTTP(w, r)
	})
}
