//go:build example

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8080),
		gosocket.WithPath("/ws"),
		gosocket.WithMaxConnections(100),
		gosocket.WithMiddleware(LoggingMiddleware),
		gosocket.WithMiddleware(AuthMiddleware),
		gosocket.OnConnect(func(c *gosocket.Client, ctx *gosocket.Context) error {
			fmt.Printf("Client connected: %s\n", c.ID)
			return nil
		}),
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(ws.Start())
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
