.PHONY: proto build run clean

# generate sqlc files
sqlc:
	@echo "Generating sqlc files..."
	sqlc generate

# Generate protobuf Go code
proto:
	@echo "Generating protobuf code..."
	@mkdir -p internal/proto
	protoc --go_out=. --go_opt=module=origin \
		api/proto/packets.proto

# Install protobuf tools
proto-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Build the server
build: proto sqlc
	go build -trimpath -o gameserver ./cmd/gameserver

# Run the server
run: proto sqlc
	go run ./cmd/gameserver

map-gen:
	go run ./cmd/mapgen

# Clean build artifacts
clean:
	rm -f gameserver
	rm -rf internal/network/pb

# Install dependencies
deps:
	go mod tidy
