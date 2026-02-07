.PHONY: all build build-backend build-frontend test clean install

VERSION ?= 0.1.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

all: build

build: build-backend build-frontend

build-backend:
	go build $(LDFLAGS) -o tapebackarr ./cmd/tapebackarr

build-frontend:
	cd web/frontend && npm install && npm run build

test:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f tapebackarr coverage.out coverage.html
	rm -rf web/frontend/node_modules web/frontend/build web/frontend/.svelte-kit

install: build
	mkdir -p /opt/tapebackarr /etc/tapebackarr /var/lib/tapebackarr /var/log/tapebackarr
	cp tapebackarr /opt/tapebackarr/
	cp -r web/frontend/build /opt/tapebackarr/static
	test -f /etc/tapebackarr/config.json || cp deploy/config.example.json /etc/tapebackarr/config.json
	cp deploy/tapebackarr.service /etc/systemd/system/
	systemctl daemon-reload

uninstall:
	systemctl stop tapebackarr || true
	systemctl disable tapebackarr || true
	rm -rf /opt/tapebackarr
	rm -f /etc/systemd/system/tapebackarr.service
	systemctl daemon-reload

dev-backend:
	go run ./cmd/tapebackarr -config deploy/config.example.json

dev-frontend:
	cd web/frontend && npm run dev

lint:
	go vet ./...
	go fmt ./...

help:
	@echo "TapeBackarr Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build backend and frontend"
	@echo "  make build-backend  - Build Go backend only"
	@echo "  make build-frontend - Build SvelteKit frontend only"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make install        - Install to /opt/tapebackarr (requires root)"
	@echo "  make uninstall      - Remove installation (requires root)"
	@echo "  make dev-backend    - Run backend in development mode"
	@echo "  make dev-frontend   - Run frontend in development mode"
	@echo "  make lint           - Run linters"
