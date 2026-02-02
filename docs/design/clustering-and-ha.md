# Clustering and High Availability Design

## 1. Context & Motivation

GoSocket is an experimental WebSocket library written in Go, currently under active development.  
It does **not** have a stable release yet and is **not being used in production environments** at the moment.

Even so, from early design decisions and experimentation, it is already possible to identify some architectural directions and limitations that will matter if the project evolves beyond a single-process, single-node use case.

One of these topics is **Clustering and High Availability**.

This discussion is not meant to propose a final implementation or to suggest that GoSocket is ready for distributed production setups. Instead, the goal is to **reason early about architecture**, document trade-offs, and avoid design paths that could make future clustering support significantly harder or more invasive.

In particular:
- WebSocket-based systems tend to start simple (single node, in-memory state)
- Over time, requirements such as scalability, fault tolerance, and zero-downtime deploys often emerge
- Retrofitting clustering and HA later usually requires breaking changes or deep refactors

By discussing these topics **before** a stable API is finalized, GoSocket can:
- keep its core simple
- define clear boundaries between library responsibilities and infrastructure concerns
- avoid coupling the core to specific technologies (e.g. Redis, Kafka)
- make future scaling paths explicit, even if optional

This discussion is therefore exploratory by nature.  
It aims to clarify concepts, identify problems early, and propose a **minimal and optional foundation** for clustering and high availability, without compromising the simplicity of single-node usage.

### 1.1. Note on Scope

This document focuses exclusively on **clustering and cross-node message delivery**.

For a complete production-ready system, GoSocket also needs:
- **Message Reliability** (ACK system)
- **Message Persistence** (durability, offline delivery)

These concerns are intentionally separated to keep each design focused. I'm still learning/creating new discussion topics about it and, once done them should be posted here.

## 2. Basic Concepts

Before discussing APIs or design proposals, it is important to clarify a few basic concepts that are often mentioned together but represent different concerns.  
This section intentionally starts from first principles, assuming no prior knowledge of clustering or high availability.

### 2.1 What is a Cluster?

In this context, a **cluster** refers to multiple instances of the same server application running at the same time and cooperating as part of a single logical system.

For example:
- Multiple GoSocket server processes running on different machines or containers
- All instances are capable of accepting WebSocket connections
- From the client’s perspective, they appear as “one service”

Typically, a cluster is placed behind a **[load balancer](https://en.wikipedia.org/wiki/Load_balancing_(computing))**, which distributes incoming connections across the available instances.

Important clarifications:
- A cluster does **not** imply shared memory
- Each node runs in its own process
- Communication between nodes must happen explicitly (over the network)

### 2.2.What is High Availability?

**High Availability (HA)** focuses on **continuity of service**, not on scaling or performance.

A system is considered highly available when:
- The service remains accessible even if one or more nodes fail
- Individual process crashes do not bring the entire system down
- Recovery is automatic or requires minimal manual intervention

In WebSocket-based systems, HA usually means:
- If a node crashes, other nodes continue running
- Existing WebSocket connections on the failed node are lost
- Clients reconnect and are handled by another node

High availability does **not** imply:
- Zero reconnects
- Perfect state recovery
- No user-visible effects at all

Instead, it aims to **minimize downtime** and limit the blast radius of failures.

### 2.3. Clustering vs High Availability

Although often mentioned together, clustering and high availability solve different problems:
- **Clustering** is primarily about scale and distribution
- **High availability** is primarily about fault tolerance

A system can be:
- Clustered but not highly available (e.g. all nodes depend on a single point of failure)
- Highly available without clustering (e.g. active-passive setup)

In practice, many systems aim to achieve both, but they should be considered separately at the design level.

### 2.4. Why WebSockets Make This Harder

WebSockets introduce additional complexity compared to stateless request/response systems.

Key reasons:
- WebSocket connections are **stateful**
- Each connection lives inside a specific process
- In-memory data structures (e.g. rooms, clients, subscriptions) are local to a node

This means:
- A client connected to node A cannot be directly accessed from node B
- Broadcasting a message in-memory only reaches clients connected to the same process
- Adding more nodes does not automatically distribute messages

As a result, simply running multiple instances of a WebSocket server does not produce a functional cluster without additional coordination mechanisms.

#### Single-node WebSocket server (simplified)

```
+----------------------+
|   GoSocket Server    |
|                      |
|  +---------------+   |
|  |   Room "A"    |   |
|  |               |   |
|  |  Client 1     |<---- WebSocket
|  |  Client 2     |<---- WebSocket
|  |  Client 3     |<---- WebSocket
|  +---------------+   |
|                      |
+----------------------+
```

- All clients are connected to the same process
- In-memory broadcast works as expected
- No cross-process communication is needed

#### Multiple nodes without clustering

```
               Load Balancer
              /             \
             v               v
+------------------+   +------------------+
| GoSocket Node A  |   | GoSocket Node B  |
|                  |   |                  |
|  Room "A"        |   |  Room "A"        |
|  Client 1        |   |  Client 3        |
|  Client 2        |   |  Client 4        |
+------------------+   +------------------+
```

- Both nodes have a room named `"A"`
- Each room exists only in local memory
- Broadcasting on Node A does **not** reach clients on Node B
- From the server’s perspective, these are completely independent systems

#### Clustered WebSocket servers (conceptual)

```
               Load Balancer
              /             \
             v               v
+------------------+   +------------------+
| GoSocket Node A  |<->| GoSocket Node B  |
|                  |   |                  |
|  Room "A"        |   |  Room "A"        |
|  Client 1        |   |  Client 3        |
|  Client 2        |   |  Client 4        |
+------------------+   +------------------+
          ^                     ^
          |                     |
          +--- Cluster Layer ---+
```

- Nodes communicate through a cluster layer
- Messages are forwarded between processes
- Each node still manages only its local connections
- No shared memory is assumed
### 2.5. Scope of This Discussion

Given these constraints, this discussion focuses on:
- Message delivery across nodes
- Distributed broadcast semantics
- Failure behavior at the server level

It explicitly does **not** attempt to solve:
- Global state replication
- Strong consistency guarantees
- Application-level data synchronization

Those concerns are intentionally left to the application layer.

## 3. GoSocket Today

At its current stage, GoSocket is designed and implemented as a **single-node WebSocket server**.

This is a deliberate and reasonable choice for an early-stage library, as it keeps the core simple, predictable, and easy to reason about. All runtime state lives in memory, and no assumptions are made about multiple processes cooperating.

The following points describe the current behavior at a high level.

### 3.1. Single Process, In-Memory State

Each GoSocket server instance runs as an independent process.

Within a single instance:
- WebSocket connections are handled locally
- Clients are represented by in-memory structures
- Rooms are maintained as in-memory collections of clients

There is no concept of a shared or global state across multiple instances.

### 3.2. Rooms and Message Delivery

Rooms exist only inside the process where they were created.

When a message is broadcast to a room:
- The server iterates over the local clients in that room
- The message is delivered directly to each connected client
- No external coordination or messaging is involved

This model is efficient and straightforward in a single-node environment.

### 3.3. No Cross-Node Awareness

Because GoSocket currently has no clustering mechanism:
- One server instance is not aware of other running instances
- Rooms with the same name on different nodes are unrelated
- Messages do not propagate beyond the local process

From the library’s perspective, multiple running instances are completely independent systems.

### 3.4. Failure Model

In the current design:
- If the GoSocket process crashes, all active connections are lost
- Clients must reconnect once the process is restarted
- No state is preserved across restarts

This behavior is consistent with many early-stage WebSocket servers and is acceptable for development and experimentation.

### 3.5. Summary of Current Scope

To summarize, GoSocket today:
- Assumes a single-node runtime model
- Uses in-memory data structures for connections and rooms
- Delivers messages only within the local process
- Does not attempt to address clustering or high availability

These characteristics form the baseline against which future design discussions, such as clustering and HA, can be evaluated.

### 3.6. Terminology Clarifications

This discussion uses a few terms that can have different meanings depending on context.  
To avoid ambiguity, the following definitions apply **within the scope of this discussion and the current GoSocket design**.

#### Room

A **Room** refers to an **in-memory grouping of WebSocket connections within a single GoSocket process**.

Important clarifications:
- A room exists only on the node where it was created
- Rooms with the same name on different nodes are unrelated
- Clustering does **not** imply that rooms become globally shared entities

Any application-level concept of a “global room” must be built on top of message routing, not shared memory.

#### Client

A **Client** represents a **single WebSocket connection** handled by a specific GoSocket process.

Important clarifications:
- A client is local to one node
- Clients are not users or sessions
- Client objects never move between nodes

User identity, authentication, and session management are outside the scope of the GoSocket core.

#### Broadcast

**Broadcast** refers to **delivering a message to multiple clients**.

In this discussion:
- _Local broadcast_ means delivering a message to clients connected to the same process
- _Cluster broadcast_ means delivering a message to all relevant nodes, which then perform local broadcast

The distinction is conceptual, even if the API surface remains minimal.

#### Message

A **Message** refers to the application-level payload exchanged between clients.

When clustering is introduced:
- Application messages remain unchanged
- Additional internal messages (cluster messages) may be used as envelopes for cross-node delivery

These internal messages are an implementation detail and are not exposed to application code.

#### Node

A **Node** refers to a single running instance of a GoSocket server.

Important clarifications:
- Each node runs in its own process
- Nodes do not share memory
- Coordination between nodes requires explicit communication

#### Scope Reminder

These definitions are intentionally narrow.  
They exist to clarify the design discussion and **do not imply future guarantees or features**.

## 4. What Breaks When We Scale

Given the current single-node design described above, it is useful to explore what happens when multiple GoSocket instances are introduced, for example to improve scalability or fault tolerance.

This section does not describe bugs or incorrect behavior. Instead, it highlights **natural limitations** that arise when a system designed for a single process is run in a multi-node environment.

### 4.1. Rooms Become Fragmented

When multiple GoSocket nodes are running:
- Each node maintains its own in-memory rooms
- Rooms with the same name exist independently on each node

As a result:
- Clients connected to different nodes but using the same room name do not see each other
- Broadcasting a message in a room only reaches clients connected to the same node

From the system’s perspective, there is no concept of a “shared room”.

### 4.2. Broadcast Is No Longer Global

In a single-node setup:
- Broadcasting is a simple in-memory operation

In a multi-node setup:
- Broadcast operations are limited to the local process
- There is no mechanism to forward messages to other nodes

This makes it impossible to implement features such as:
- Global chat rooms
- Multi-node presence
- Distributed real-time collaboration

Without additional coordination, each node behaves as an isolated island.

### 4.3. Load Balancers Do Not Solve Message Delivery

A load balancer can:
- Distribute incoming WebSocket connections
- Help with horizontal scaling

However, a load balancer:
- Does not synchronize in-memory state
- Does not forward messages between nodes
- Has no awareness of rooms or broadcasts

As a result, simply adding a load balancer does not solve the fragmentation problem described above.

### 4.4. Process Failure Disconnects All Local Clients

When a node crashes or is restarted:
- All WebSocket connections handled by that node are lost
- Clients must reconnect
- In-memory state is discarded

While this behavior is expected, in a multi-node environment it becomes more visible, especially if:
- A large number of clients are connected to a single node
- Rolling deploys are performed frequently

### 4.5. No Coordinated Failover Behavior

Because nodes are unaware of each other:
- There is no coordination during failures
- No node can take responsibility for another node’s connections
- Recovery relies entirely on client-side reconnection logic

This limits the level of high availability that can be achieved without additional mechanisms.

### 4.6. Summary

When scaling GoSocket by simply running multiple instances:
- Rooms become fragmented
- Broadcasts are limited to a single node
- Failures affect all local connections
- Load balancers alone are insufficient

These limitations are expected outcomes of the current design and provide the motivation for discussing clustering and high availability mechanisms.

## 5. Why Clustering & High Availability Matter for GoSocket

The limitations described in the previous section are not unique to GoSocket.  
They are common to most WebSocket servers that start with a simple, single-node design.

The question is not whether these limitations exist, but **whether GoSocket should acknowledge them early and provide a path forward**.

### 5.1. Supporting Horizontal Growth Without Redesign

As projects evolve, it is common for real-time systems to outgrow a single process due to:
- Increased number of concurrent connections
- CPU or memory constraints
- Operational requirements (deploys, restarts, fault isolation)

Without a clustering strategy, scaling often requires:
- Redesigning core abstractions
- Introducing breaking changes
- Rewriting message delivery logic

By defining clustering boundaries early, GoSocket can:
- Preserve a simple single-node experience
- Avoid invasive refactors later
- Make scaling a conscious, optional decision

### 5.2. Improving Fault Tolerance

In a single-node setup:
- A process crash disconnects all clients
- The entire real-time layer becomes unavailable

With multiple nodes and basic coordination:
- A single node failure affects only a subset of connections
- Other nodes continue operating
- Clients can reconnect to a healthy node

This does not eliminate failures, but it **reduces their impact**.

### 5.3. Enabling Distributed Real-Time Features

Many real-time features implicitly assume a clustered environment, such as:
- Global chat rooms
- Presence indicators
- Multiplayer game lobbies
- Collaborative tools

Without clustering support:
- These features are constrained to a single node
- Application developers must re-implement cross-node routing themselves

Providing a minimal clustering foundation allows GoSocket to:
- Support these use cases naturally
- Keep application-level logic simpler
- Avoid pushing infrastructure concerns into user code

### 5.4. Aligning With Modern Deployment Environments

Modern deployment environments (containers, orchestration platforms, cloud infrastructure) encourage:
- Running multiple instances by default
- Ephemeral processes
- Automated restarts and rescheduling

Even if GoSocket is not production-ready yet, aligning its design with these realities:
- Makes future adoption easier
- Avoids assumptions tied to long-lived single processes
- Encourages stateless and composable patterns

### 5.5. Clustering as an Optional Capability

Importantly, clustering and HA are not proposed as mandatory features.

Design goals include:
- Zero impact on single-node usage
- Explicit opt-in for clustering
- No required external dependencies

Users who do not need clustering should not pay for it in complexity or performance.

### 5.6. Summary

Clustering and high availability matter for GoSocket because they:
- Provide a clear scaling path
- Reduce the impact of failures
- Enable distributed real-time features
- Align the library with modern infrastructure expectations

The next sections explore how these goals could be achieved **without compromising simplicity**.

## 6. Design Goals

Before introducing any concrete APIs or implementation details, it is important to establish a clear set of design goals. These goals define the boundaries of the discussion and help evaluate whether a proposal aligns with the intended direction of GoSocket.

### 6.1. Clustering Must Be Optional

Clustering and high availability should be **opt-in features**.
- Single-node usage must remain simple
- No additional complexity should be required unless clustering is explicitly enabled
- Existing usage patterns should continue to work unchanged

### 6.2. No Mandatory External Dependencies

The GoSocket core should not depend directly on specific infrastructure technologies such as Redis, Kafka, or similar systems.
- The core defines contracts, not infrastructure
- Integration with external systems should happen via adapters
- Users should be free to choose the tools that fit their environment

### 6.3. Clear Separation of Responsibilities

The design should clearly separate:
- Library responsibilities (connections, rooms, message delivery)
- Infrastructure responsibilities (process coordination, networking, deployment)

This separation helps keep the core maintainable and avoids coupling design decisions to specific environments.

### 6.4. Predictable Failure Behavior

Failures should be explicit and easy to reason about.
- Node crashes result in local connection loss
- Recovery relies on client reconnection
- No hidden or implicit failover mechanisms

The goal is to reduce surprises, not to hide failures.

### 6.5. Minimal and Explicit API Surface

Any clustering-related API should:
- Be small and focused
- Expose only what is strictly necessary
- Avoid leaking implementation details into application code

A minimal API is easier to evolve and reason about.

### 6.6. No Assumptions About Application State

GoSocket should **not** attempt to:
- Replicate application state
- Synchronize user sessions
- Provide strong consistency guarantees

These concerns are intentionally left to the application layer.

### 6.7. Backward Compatibility as a First-Class Concern

Even before a stable release, the design should:
- Minimize future breaking changes
- Avoid architectural dead ends
- Favor extensibility over shortcuts

This is especially important when introducing foundational concepts such as clustering.

### 6.8. Summary

These design goals serve as constraints for any proposed clustering or high availability mechanism.  
The next section introduces a possible approach that aims to satisfy these goals while keeping GoSocket simple and flexible.

## 7. Proposed Approach

Based on the limitations identified so far and the design goals outlined above, this section proposes a **conceptual approach** for introducing clustering and high availability into GoSocket.

At this stage, the focus is on **architecture**, not implementation details.

### 7.1. Separate Message Delivery From Business Logic

A key observation is that two distinct concerns are often conflated in WebSocket servers:
- _What should happen when a message is received_ (business logic)
- _How a message is delivered to connected clients_ (infrastructure)

The proposed approach explicitly separates these concerns.
- Business logic remains local and synchronous
- Message delivery becomes cluster-aware when enabled

This separation allows clustering to be introduced without affecting application-level behavior.

### 7.2. Keep Connections and Rooms Local

Even in a clustered setup:
- WebSocket connections remain bound to a single process
- Rooms remain in-memory structures local to each node

No attempt is made to:
- Share memory across nodes
- Migrate live connections
- Create globally synchronized room objects

Instead, each node is responsible only for the clients it directly manages.

### 7.3. Introduce an Explicit Inter-Node Communication Layer

To enable cross-node message delivery, nodes must communicate explicitly.

The proposed approach introduces a **cluster communication layer** that:
- Allows nodes to publish messages intended for other nodes
- Allows nodes to receive messages published elsewhere
- Operates independently of application logic

This layer acts as a transport mechanism, not a coordinator or state manager.

### 7.4. Forward Messages, Not State

Rather than synchronizing state, the cluster layer focuses on **forwarding messages**.

This means:
- Messages are delivered to all relevant nodes
- Each node performs local delivery to its connected clients
- No shared global state is required

This approach favors simplicity and aligns with the stateless nature of message passing.

### 7.5. Opt-In Behavior With Clear Boundaries

Clustering is explicitly enabled by configuration.

When clustering is disabled:
- The system behaves exactly as it does today
- No additional overhead is introduced

When clustering is enabled:
- Local broadcast is extended to include cross-node forwarding
- Failure behavior remains predictable and explicit

### 7.6. Conceptual Flow

```
Client → Node A
   ↓
Application Logic (local)
   ↓
Local Broadcast
   ↓
Cluster Publish (optional)
   ↓
Other Nodes
   ↓
Local Broadcast on each node
```

This flow emphasizes that:
- Application logic runs once
- Delivery is replicated, not logic
- Nodes remain loosely coupled

### 7.7. Summary

The proposed approach introduces clustering as a **message routing concern**, not a state management problem.

By keeping nodes independent and communicating through a narrow, explicit interface, GoSocket can support clustering and basic high availability while remaining simple and flexible.

The next section explores how this approach could be expressed through a minimal API.

## 8. Minimal Cluster API

This section introduces a **minimal API proposal** that expresses the architectural ideas described above.  
The goal is not to define a complete clustering solution, but to outline the **smallest possible set of abstractions** required to enable cross-node message delivery.

### 8.1. Node Identity

Each GoSocket instance participating in a cluster must have a stable identifier.

```go
type NodeID string
```

This identifier:
- Is provided externally (e.g. environment variable, config, container ID)
- Is not generated by the library
- Uniquely identifies a running GoSocket process within a cluster

### 8.2. Cluster Transport Interface

The core of the proposal is a transport abstraction responsible for inter-node communication.

```go
type ClusterTransport interface {
    Publish(ctx context.Context, msg ClusterMessage) error
    Subscribe(ctx context.Context, handler func(ClusterMessage)) error
    Close() error
}
```

This interface:
- Defines _how_ messages move between nodes
- Does not define _where_ or _using which technology_
- Can be implemented using Redis, NATS, Kafka, gRPC, or any other mechanism

The GoSocket core depends only on this interface.

### 8.3. Cluster Message Envelope

Messages exchanged between nodes use a small, explicit envelope.

```go
type ClusterMessage struct {
    FromNode NodeID
    Target   ClusterTarget
    Type     ClusterMessageType
    Payload  []byte
}
```

```go
type ClusterTarget struct {
    Room         string // optional
    ConnectionID string // optional
    Broadcast    bool
}
```

```go
type ClusterMessageType string

const (
    ClusterMessageSend ClusterMessageType = "send"
)
```

This envelope:
- Contains only routing information
- Does not expose application-level concepts
- Avoids leaking local structures across nodes

### 8.4. Enabling Clustering

Clustering is explicitly enabled during server initialization.

```go
type ClusterConfig struct {
    NodeID    NodeID
    Transport ClusterTransport
}
```

```go
func (s *Server) EnableClustering(cfg ClusterConfig) error
```

If clustering is not enabled:
- The server behaves exactly as it does today
- No cluster-related code paths are executed

### 8.5. Interaction With Rooms

Rooms remain local, but broadcasts become cluster-aware.

Conceptually:
```go
func (r *Room) Broadcast(msg []byte) {
    // 1. Deliver locally
    r.broadcastLocal(msg)

    // 2. If clustering enabled, forward to other nodes
    if r.clusterEnabled {
        r.publishToCluster(msg)
    }
}
```

On receiving a cluster message:
- The node delivers the message only to its local clients
- Application-level handlers are not re-executed

This ensures that side effects happen exactly once.

### 8.6. Example Flow

```
Client → Node A
   ↓
Application Handler (runs once)
   ↓
Room.Broadcast()
   ↓
Local clients on Node A
   ↓
ClusterTransport.Publish()
   ↓
Node B / Node C
   ↓
Local broadcast on each node
```

### 8.7. What This API Does Not Do

This API intentionally does not:
- Replicate room state
- Synchronize clients
- Guarantee message ordering across nodes
- Provide persistence or replay
- Handle user sessions or identity

These concerns are outside the scope of the GoSocket core.

### 8.8. Why This Is Minimal

The proposed API:
- Introduces exactly one new abstraction (`ClusterTransport`)
- Keeps all existing concepts local
- Avoids coupling to infrastructure choices
- Can be ignored entirely by users who do not need clustering

This minimal surface makes it easier to evolve the design without breaking core assumptions.

### 8.9. Discussion Points

Open questions for discussion include:
- Naming and structure of `ClusterMessage`
- Whether join/leave events should be explicit or implicit
- Error handling and backpressure semantics
- Testing strategies for clustered behavior

## 9. How This Integrates With Existing APIs

This section explains how the proposed clustering approach fits into the current GoSocket model, without requiring major changes to existing abstractions or usage patterns.

The intent is to extend behavior where needed, not to replace it.

### 9.1. No Changes to Client or Connection APIs

From the perspective of application code:
- Clients remain local WebSocket connections
- Sending messages to a client works exactly as it does today
- No cluster-specific logic is required at the client level

Connections never move between nodes and are not exposed outside the local process.

### 9.2. Rooms Remain the Primary Abstraction

Rooms continue to be:
- The unit of message grouping
- Managed entirely in memory
- Responsible for local message delivery

The only behavioral change is that, when clustering is enabled, a room broadcast may result in **additional message forwarding to other nodes**.

This forwarding is transparent to application code.

### 9.3. Application-Level Handlers Remain Local

Application-level message handlers:
- Are executed only on the node that receives the client message
- Are not triggered again when messages are forwarded across the cluster

This avoids:
- Duplicate side effects
- Repeated persistence
- Inconsistent state updates

The cluster layer deals strictly with message delivery, not business logic.

### 9.4. Opt-In Configuration, Not API Pollution

Clustering is enabled through server configuration rather than through changes in application code.

This means:
- No new methods are required in typical usage
- Existing applications continue to compile unchanged
- Clustering concerns are isolated from business logic

Users who do not enable clustering do not interact with the cluster API at all.

### 9.5. Compatibility With Existing Extensions

Existing extension points (such as message handlers, middleware, or room hooks) continue to function as before.

Clustering does not:
- Introduce new lifecycle hooks
- Change handler execution order
- Require awareness of other nodes

This helps preserve the mental model of the current API.

### 9.6. Summary

The proposed clustering mechanism integrates with GoSocket by:
- Preserving all existing abstractions
- Extending broadcast behavior in a transparent way
- Keeping application code unaware of clustering details

This minimizes disruption while enabling new capabilities.

## 10. Example End-to-End Flow

This section walks through a concrete example to illustrate how the proposed clustering approach behaves in practice, from the moment a client sends a message to its delivery across multiple nodes.

### 10.1. Scenario Setup

Assume the following setup:
- Two GoSocket nodes running: **Node A** and **Node B**
- Both nodes have clustering enabled
- Both nodes have a room named `"chat"`
- Clients are connected through a load balancer

```
               Load Balancer
              /             \
             v               v
+------------------+   +------------------+
| GoSocket Node A  |<->| GoSocket Node B  |
|                  |   |                  |
|  Room "chat"     |   |  Room "chat"     |
|  Client 1        |   |  Client 3        |
|  Client 2        |   |  Client 4        |
+------------------+   +------------------+
          ^                     ^
          |                     |
          +--- Cluster Layer ---+
```

#### Step 1: Client Sends a Message

- Client 1 sends a WebSocket message to Node A
- Node A receives the message and identifies the target room (`"chat"`)

At this point, no cluster-related logic is involved.

#### Step 2: Application Logic Executes (Once)

- Node A invokes the application-level message handler
- Any validation, filtering, persistence, or side effects occur here
- The handler decides to broadcast the message to the room

Important:

- This logic runs **exactly once**
- No other node executes application logic for this message

#### Step 3: Local Broadcast on Node A

- Node A delivers the message to:
    - Client 1
    - Client 2

This behavior is identical to the current single-node model.

#### Step 4: Message Is Published to the Cluster

- Because clustering is enabled, Node A forwards the message to the cluster transport
- The message is wrapped in a cluster envelope
- No application-level data structures are shared

At this stage, Node A’s responsibility for the message is complete.

#### Step 5: Other Nodes Receive the Message

- Node B receives the cluster message via the transport
- Node B inspects the target room (`"chat"`)
- No application handlers are executed on Node B

Node B treats the message as a delivery instruction, not as a new event.

#### Step 6: Local Broadcast on Node B

- Node B delivers the message to:
    - Client 3
    - Client 4

From the clients’ perspective, the message appears to have been broadcast globally.

#### Step 7: Failure Behavior (Optional)

If Node A crashes after Step 4:
- Clients connected to Node A are disconnected
- Node B continues operating normally
- Clients reconnect and may be routed to Node B

No special failover logic is required inside GoSocket.

### 10.2. Summary of the Flow

Key properties of this flow:
- Application logic runs once
- Message delivery is replicated across nodes
- Rooms and clients remain local
- Failures are explicit and predictable

This example demonstrates how clustering can extend GoSocket’s current behavior without changing its fundamental execution model.

## 11. What This Does Not Try to Solve

This proposal intentionally limits its scope.  
The goal is to provide a minimal foundation for clustering and basic high availability, **not** to solve all problems related to distributed systems or real-time applications.

The following concerns are explicitly **out of scope**.

### 11.1. Global State Replication

This proposal does not attempt to:
- Share in-memory state across nodes
- Synchronize room contents
- Maintain globally consistent data structures

All state remains local to each node.

### 11.2. User Sessions and Identity

GoSocket does not manage:
- User authentication
- Session replication
- User identity across nodes

These concerns belong to the application layer and its backing services.

### 11.3. Strong Consistency Guarantees

This design does not provide:
- Exactly-once delivery
- Global message ordering
- Transactional guarantees across nodes

Message delivery is best-effort and favors simplicity over strict consistency.

### 11.4. Message Persistence or Replay

The cluster layer is not responsible for:
- Persisting messages
- Replaying missed messages after reconnect
- Providing message history

Persistence can be layered on top by the application if needed.

### 11.5. Transparent Failover or Connection Migration

This proposal does not attempt to:
- Migrate live WebSocket connections between nodes
- Hide disconnects caused by node failures
- Provide seamless client-side failover

Clients are expected to handle reconnection explicitly.

### 11.6. Distributed Coordination or Leader Election

No attempt is made to:
- Elect leaders
- Coordinate ownership of rooms
- Manage distributed locks

The cluster layer acts purely as a message transport mechanism.

### 11.7. Summary

By clearly defining what is **not** included, this proposal:
- Keeps the design focused
- Avoids hidden complexity
- Prevents over-promising

The intent is to offer a small, composable building block rather than a full distributed system.

## 12. Open Questions / Discussion

This proposal is intentionally incomplete and exploratory.  
The purpose of this section is to surface open questions and invite feedback on the overall direction, rather than to finalize a specific implementation.

The following topics are open for discussion.

### 12.1. API Shape and Naming

- Are the proposed abstractions (`ClusterTransport`, `ClusterMessage`, etc.) clear and intuitive?
- Is the naming aligned with GoSocket’s existing terminology?
- Are there unnecessary concepts that could be simplified further?

### 12.2. Message Envelope Design

- Should cluster messages be strictly minimal, or is there value in adding more explicit metadata?
- Is it better to model join/leave events explicitly or infer them implicitly?
- Should the envelope be opaque to the core and interpreted only by adapters?

### 12.3. Failure Semantics

- How should errors in the cluster transport be handled?
- Should message delivery failures be surfaced to application code?
- Is best-effort delivery sufficient for the intended use cases?

### 12.4. Adapter Strategy

- Should the repository provide official adapters (e.g. Redis, NATS) or leave all implementations to users?
- If adapters are included, where should they live in the project structure?
- What level of maintenance should be expected for these adapters?

### 12.5. Testing and Development Experience

- How should clustered behavior be tested locally?
- Is an in-memory or single-process cluster adapter useful for testing?
- What tooling or examples would make clustered setups easier to understand?

### 12.6. Backward Compatibility and Evolution

- Are there foreseeable changes that should be anticipated now?
- Does this proposal introduce any architectural dead ends?

### 12.7. Scope Validation
- Does the scope feel appropriate for GoSocket?
- Is anything essential missing from the proposal?
- Is anything included that should be explicitly removed?

### 12.8. Summary

Feedback is welcome on both:
- The **overall approach**
- The **constraints and goals** that shape it

The intent is to converge on a design that is simple, explicit, and sustainable over time.

## 13. Conclusion

This discussion explores clustering and high availability in the context of GoSocket **before** a stable API or production usage exists.

Rather than proposing a complete solution, the goal was to:
- Clarify core concepts
- Identify natural limitations of the current design
- Define clear boundaries and design goals
- Explore a minimal and optional path toward clustering and HA

The proposed approach focuses on **message delivery**, not state synchronization, and intentionally avoids coupling the core library to specific infrastructure technologies.

This discussion was also written intentionally as a **learning document**.  
GoSocket is being used as a personal learning project, and having a written, structured explanation helps guide design decisions, clarify trade-offs, and avoid confusion as the project evolves.

By reasoning about these concerns early, GoSocket can:
- Preserve its simplicity
- Avoid architectural dead ends
- Make future scaling paths explicit and intentional

All feedback is welcome, especially around:
- Scope and design goals
- API shape and naming
- Failure semantics and expectations

This discussion is meant to be a starting point for collaborative design, not a final decision.
