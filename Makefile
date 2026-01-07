.PHONY: build run test clean lint docker-build docker-run

BINARY_NAME=dex-aggregator
VERSION?=0.1.0

build:
	go build -ldflags="-s -w" -o bin/api ./cmd/api

run: build
	./bin/api

test:
	go test -v -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-build:
	docker build -t $(BINARY_NAME):$(VERSION) .

docker-run:
	docker run -p 8080:8080 \
		-e ETH_RPC_URL=${ETH_RPC_URL} \
		$(BINARY_NAME):$(VERSION)

docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down

fmt:
	go fmt ./...

tidy:
	go mod tidy

dev:
	ETH_RPC_URL=https://eth.llamarpc.com go run ./cmd/api
