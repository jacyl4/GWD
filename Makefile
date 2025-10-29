SERVER ?= server

build:
	go build -o $(SERVER) ./cmd/server/main.go

clean:
	rm -f $(SERVER)

test:
	go test -v ./...
