.PHONY: build test race vet lint run docker-up docker-down clean

build:
	go build -o bin/ap-cloud-server$(shell go env GOEXE) ./cmd/server

test:
	go test -count=1 ./...

race:
	go test -race -count=1 ./...

# 完整集成测试：需要本机 PostgreSQL，并设置 TEST_DATABASE_URL
test-integration:
	TEST_DATABASE_URL="$${TEST_DATABASE_URL:-postgres://postgres:postgres@localhost:5432/apcloud_test?sslmode=disable}" go test -race -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run

run:
	go run ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

clean:
	rm -rf bin dist
