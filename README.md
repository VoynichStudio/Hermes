# Hermes
# gRPC Chat Server

This is a real-time chat server implemented using gRPC with Buf's Connect RPC protocol. It supports multiple users joining channels, sending messages, and persisting chat history using ScyllaDB for optimized real-time performance. Authentication is handled via AWS Cognito.

## Features
- Real-time messaging using gRPC streams
- Multiple users per channel
- Authentication via AWS Cognito with JWT validation
- Message persistence with ScyllaDB
- HTTP/2 support with h2c (cleartext HTTP/2)

## Installation

### Prerequisites
- Go 1.19+
- ScyllaDB
- AWS Cognito setup
- Buf CLI

### Setup
1. Clone the repository:
   ```sh
   git clone https://github.com/your-username/grpc-chat-server.git
   cd grpc-chat-server
   ```
2. Install dependencies:
   ```sh
   go mod tidy
   ```
3. Set up ScyllaDB:
   ```sh
   docker run --name scylla -d -p 9042:9042 scylladb/scylla
   ```
4. Generate protobuf files:
   ```sh
   buf generate
   ```

## Running the Server

Start the chat server:
```sh
 go run main.go
```

## Testing
Run unit tests:
```sh
 go test ./...
```

## API Endpoints
The chat service exposes the following endpoints:
- `JoinChannel` - Allows a user to join a channel and receive messages
- `SendMessage` - Sends a message to a channel

## Contributing
Feel free to submit issues and pull requests to improve the project.

## License
MIT License

