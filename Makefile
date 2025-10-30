SERVER ?= server

build:
	go build -o $(SERVER) ./cmd/server/main.go
	bash ./archive_sha256.sh

clean:
	rm -f $(SERVER)

test:
	go test -v ./...
