# GoSocket

Setting up WebSocket servers in Go usually means writing a lot of boilerplate code. You need to handle connections, manage clients, implement broadcasting, deal with rooms, and handle different message formats. **GoSocket** abstracts all of this away.

## Overview

A simple WebSocket abstraction for Go that gets you up and running in minutes.

**GoSocket** provides:
- **Simple setup**: Install package, write 5-10 lines of code, have a working WebSocket server
- **Multiple encoding support**: JSON (ready), Protobuf and MessagePack (planned), or raw binary data
- **Built-in rooms**: Join/leave rooms without manual management  
- **Broadcasting**: Send to all clients, specific rooms, or individual clients
- **Middleware support**: Add auth, logging, CORS, whatever you need
- **Graceful shutdown**: Clean connection handling when stopping
- **Multiple servers**: Run chat, notifications, and admin panels on different ports simultaneously

## Installing
First, use `go get` to install the latest version of the library.

```
go get -u github.com/FilipeJohansson/gosocket@latest
```

Next, include **GoSocket** in your application:
```go
import "github.com/FilipeJohansson/gosocket"
```

## Quick Start

### Use as a server

```go
ws := gosocket.NewServer()

ws.WithPort(8080).
    WithPath("/ws").
    OnConnect(func(client *gosocket.Client) error {
        fmt.Printf("Client connected: %s\n", client.ID)
        return nil
    }).
    OnMessage(func(client *gosocket.Client, message *gosocket.Message) error {
        fmt.Printf("Received: %s\n", string(message.RawData))
        // Echo back
        client.Send(message.RawData)
        return nil
    }).
    OnDisconnect(func(client *gosocket.Client) error {
        fmt.Printf("Client disconnected: %s\n", client.ID)
        return nil
    })

log.Fatal(ws.Start())
```

### Use as a handler

```go
handler := gosocket.NewHandler()

handler.
    OnConnect(func(client *gosocket.Client) error {
        fmt.Printf("Client connected: %s\n", client.ID)
        return nil
    }).
    OnMessage(func(client *gosocket.Client, message *gosocket.Message) error {
        fmt.Printf("Received: %s\n", string(message.RawData))
        // Echo back
        client.Send(message.RawData)
        return nil
    }).
    OnDisconnect(func(client *gosocket.Client) error {
        fmt.Printf("Client disconnected: %s\n", client.ID)
        return nil
    })

http.Handle("/ws", handler)
http.ListenAndServe(":8080", nil)
```

For more examples, see [examples](examples).

## Current Status

Planning to launch v1.0.0 soon. Until then, you can start testing our pre-prod versions.

## Contributing

This project is actively being developed. We're looking for contributions in:

- Documentation: Help improve examples and API documentation
- Testing: Write tests for edge cases and performance scenarios
- Serializers: Implement Protobuf and MessagePack support

1. Check out the current structure in the code
2. Open an issue to discuss what you'd like to work on
3. Focus areas: core functionality, serializers, and comprehensive testing


## License

**GoSocket** is released under the MIT license. See [LICENSE](LICENSE).
