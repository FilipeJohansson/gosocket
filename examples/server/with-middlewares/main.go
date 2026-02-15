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
		gosocket.WithMaxConnections(gosocket.ConnectionPoolConfig{MaxTotal: 100, MaxPerIP: 10}),
		gosocket.WithAuth(AuthMiddleware),
		gosocket.WithMiddleware(LoggingMiddleware),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Client connected: %s\n", client.GetID())
			fmt.Printf("Client data: %v\n", client.GetUserData())
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

func AuthMiddleware(r *http.Request) (map[string]interface{}, error) {
	token := r.Header.Get("Authorization")
	if token == "" {
		fmt.Println("[AUTH] No token provided")
		return nil, fmt.Errorf("No token provided")
	}

	fmt.Printf("[AUTH] Token validated: %s\n", token)
	// here you could add more complex token validation logic

	return map[string]interface{}{"user_id": "123", "token": token}, nil
}
