# initagent build glue.
#
#   make          build everything into ./initagent (UI embedded)
#   make ui       build the web UI and stage it for embedding
#   make go       build the Go binary only (uses last staged UI)
#   make generate  write site/ui TypeScript from catalog.yaml
#   make cross    cross-compile for all supported platforms into dist/

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
CMD_DIR := cmd/initagent

.PHONY: all ui go test cross clean generate

all: ui go

ui:
	cd ui && npm install --no-audit --no-fund && npm run build
	rm -rf $(CMD_DIR)/uidist
	cp -r ui/dist $(CMD_DIR)/uidist
	touch $(CMD_DIR)/uidist/.gitkeep   # keep the go:embed target non-empty for `go test` without a UI build

go:
	go build -ldflags '$(LDFLAGS)' -o initagent ./$(CMD_DIR)

test:
	go vet ./...
	go run ./internal/orgplan/gencatalog -check
	go test ./...

generate:
	go run ./internal/orgplan/gencatalog

cross: ui
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building dist/initagent_$${os}_$${arch}"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' \
			-o dist/initagent_$${os}_$${arch} ./$(CMD_DIR) || exit 1; \
	done

clean:
	rm -rf initagent dist ui/dist $(CMD_DIR)/uidist
	mkdir -p $(CMD_DIR)/uidist && touch $(CMD_DIR)/uidist/.gitkeep
