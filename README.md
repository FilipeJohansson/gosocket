# GoSocket

A simple WebSocket abstraction for Go that gets you up and running in minutes.

## Why GoSocket?

Setting up WebSocket servers in Go usually means writing a lot of boilerplate code. You need to handle connections, manage clients, implement broadcasting, deal with rooms, and handle different message formats. GoSocket abstracts all of this away.

## What GoSocket will be

- **Simple setup**: Install package, write 5-10 lines of code, have a working WebSocket server
- **Multiple encoding support**: JSON, Protobuf, MessagePack, or raw binary data
- **Built-in rooms**: Join/leave rooms without manual management  
- **Broadcasting**: Send to all clients, specific rooms, or individual clients
- **Middleware support**: Add auth, logging, CORS, whatever you need
- **Graceful shutdown**: Clean connection handling when stopping
- **Multiple servers**: Run different WebSocket servers on different ports easily

## Current Status

🚧 **Work in Progress** 🚧

This project is in early development, some API structure is defined but methods are not implemented yet.
If you're interested in contributing, or want to follow progress, watch this repository.

## Planned API

```go
// Basic server
ws := gosocket.New().
    WithPort(8080).
    WithPath("/ws").
    OnConnect(func(client *gosocket.Client) {
        fmt.Printf("Client connected: %s\n", client.ID)
    }).
    OnMessage(func(client *gosocket.Client, message *gosocket.Message) {
        // Echo message to all clients
        ws.Broadcast(message.RawData)
    })

// Start server (blocking)
ws.Start()
```

```go
// Advanced usage with rooms and JSON
ws := gosocket.New().
    WithPort(8080).
    WithJSONSerializer().
    OnConnect(func(client *gosocket.Client) {
        client.JoinRoom("general")
    }).
    OnJSONMessage(func(client *gosocket.Client, data interface{}) {
        // Broadcast JSON to room
        ws.BroadcastToRoomJSON("general", data)
    })

go ws.Start()
```

## Contributing

This project is just getting started. If you want to contribute:

1. Check out the current structure in the code
2. Open an issue to discuss what you'd like to work on
3. The main focus right now is implementing the core functionality
