# 🚀 GoSocket

*The simplest way to add WebSockets to your Go application*

[![Go Version](https://img.shields.io/github/go-mod/go-version/FilipeJohansson/gosocket)](https://github.com/FilipeJohansson/gosocket) [![GoDoc](https://godoc.org/github.com/FilipeJohansson/gosocket?status.svg)](https://godoc.org/github.com/FilipeJohansson/gosocket) [![Go Report Card](https://goreportcard.com/badge/github.com/FilipeJohansson/gosocket)](https://goreportcard.com/report/github.com/FilipeJohansson/gosocket) [![License](https://img.shields.io/github/license/FilipeJohansson/gosocket)](LICENSE)

Stop writing WebSocket boilerplate. Start building features.

```go
// That's it. You have a working WebSocket server.
ws, _ := gosocket.NewServer(
    gosocket.WithPort(8080),
    gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
        client.Send(message.RawData) // Echo back
        return nil
    }),
)
log.Fatal(ws.Start())
```

---

## Why GoSocket?

GoSocket focuses on **developer experience** and **getting started quickly**:

- **Minimal boilerplate** - Get a WebSocket server running in a few lines
- **Built-in features** - Rooms, broadcasting, and client management included
- **Middleware support**: Add auth, logging, CORS, whatever you need
- **Simple API** - Intuitive methods that do what you expect
- **Flexible** - Use standalone or integrate with existing HTTP servers
- **Multiple servers**: Run chat, notifications, and admin panels on different ports simultaneously

## Quick Start

### Installation
```bash
go get -u github.com/FilipeJohansson/gosocket@latest
```

### Standalone Server
Perfect for dedicated WebSocket services:

```go
package main

import (
    "fmt"
    "log"
    "github.com/FilipeJohansson/gosocket"
)

func main() {
    ws, err := gosocket.NewServer(
        gosocket.WithPort(8080),
        gosocket.WithPath("/ws"),
        gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
            fmt.Printf("Client %s connected\n", client.ID)
            return nil
        }),
        gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
            // Broadcast to all clients
            ctx.BroadcastToAll(gosocket.NewRawMessage(gosocket.TextMessage, message.RawData))
            return nil
        }),
        gosocket.OnDisconnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
            fmt.Printf("Client %s disconnected\n", client.ID)
            return nil
        }),
    )

    if err != nil {
        // something wrong with the server configuration
    }
    
    log.Fatal(ws.Start())
}
```

### Integrate with Existing HTTP Server
Perfect for adding real-time features to REST APIs:

```go
package main

import (
    "net/http"
    "github.com/FilipeJohansson/gosocket"
)

func main() {
    // Your existing routes
    http.HandleFunc("/api/users", getUsersHandler)
    
    // Add WebSocket endpoint
    ws, err := gosocket.NewHandler(
        gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
            client.Send(message.RawData)
            return nil
        }),
    )

    if err != nil {
        // something wrong with the handler configuration
    }
    
    http.Handle("/ws", ws)
    http.ListenAndServe(":8080", nil)
}
```

## Middleware Support

Add authentication, logging, CORS, and more:

```go
ws, _ := gosocket.NewServer(
    gosocket.WithPort(8080),
    gosocket.WithMiddleware(AuthMiddleware),
    gosocket.WithMiddleware(LoggingMiddleware),
    gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
        fmt.Printf("Authenticated client connected: %s\n", client.ID)
        return nil
    }),
)
```

## Rooms & Broadcasting

```go
gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
    client.JoinRoom("general")
    return nil
})

gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
    // Send to specific room
    ctx.BroadcastToRoom("general", message.RawData)
    
    // Send to specific client
    client.Send([]byte("ACK"))
    
    // Send to everyone
    ctx.BroadcastToAll([]byte("Global announcement"))
    
    return nil
})
```

## Real-World Examples

- **[Chat Application](examples/chat)** - Multi-room chat with rooms
- **[Live Notifications](examples/notifications)** - Push notifications to web clients
- **[Game Lobby](examples/game-lobby)** - Real-time multiplayer coordination
- **[Stock Ticker](examples/stock-ticker)** - Live data streaming
- **[Middleware Example](examples/server/with-middlewares)** - Authentication and logging middleware
- **[Gin Integration](examples/gin-integration)** - Add WebSockets to Gin apps

## Performance

GoSocket is built for production use:

- **Memory efficient**: Minimal allocation per connection
- **Fast**: Built on proven WebSocket libraries
- **Scalable**: Handle thousands of concurrent connections
- **Reliable**: Comprehensive error handling

## Features

- **Quick Setup** - Minimal code to get started
- **Built-in Rooms** - Join/leave rooms without manual management  
- **Broadcasting** - Send to all clients, rooms, or individuals
- **Flexible Integration** - Standalone server or HTTP handler
- **Multiple Encodings** - JSON ready, Protobuf & MessagePack coming

## Roadmap

- [x] **v0.1**: Basic WebSocket server + middleware support
- [x] **v0.2**: Rooms and broadcasting  
- [x] **v0.3**: Functional options pattern
- [ ] **v0.4**: Protobuf & MessagePack support
- [ ] **v1.0**: Production ready

## Contributing

We're actively looking for contributors! Areas where you can help:

- **Documentation**: Improve examples and guides
- **Testing**: Write tests and benchmarks  
- **Features**: Implement Protobuf/MessagePack serializers
- **Bug fixes**: Report and fix issues

Check out [CONTRIBUTING.md](CONTRIBUTING.md) to get started.

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">

**Love GoSocket?** Give us a ⭐ and help spread the word!

[🐦 Tweet](https://twitter.com/intent/tweet?text=Check%20out%20GoSocket%20-%20the%20simplest%20way%20to%20add%20WebSockets%20to%20Go%20apps!%20https://github.com/FilipeJohansson/gosocket) • [📺 Demo](examples/) • [💬 Discussions](https://github.com/FilipeJohansson/gosocket/discussions)

</div>