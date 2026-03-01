.PHONY: proto clean setup

# Install the required protobuf compiler plugins for Go
setup:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Compiles the protobuf file into Go structs and gRPC service interfaces
proto:
	@echo "Generating Go files from Kademlia Protobuf definitions..."
	protoc --go_out=. --go_opt=paths=source_relative \
	--go-grpc_out=. --go-grpc_opt=paths=source_relative \
	proto/dht.proto

# Cleans up the generated files if you need to start fresh
clean:
	rm -f proto/*.pb.go