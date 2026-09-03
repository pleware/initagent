# initagent build glue.
#
#   make          build everything into ./initagent (UI embedded)
#   make ui       build the web UI and stage it for embedding
#   make go       build the Go binary only (uses last staged UI)
#   make test     run Go tests
#   make cross    cross-compile for all supported platforms into dist/

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: all ui go test cross clean

all: ui go

ui:
	cd ui && npm install --no-audit --no-fund && npm run build
	rm -rf cmd/initagent/uidist
	cp -r ui/dist cmd/initagent/uidist
	touch cmd/initagent/uidist/.gitkeep   # keep the go:embed target non-empty for `go test` without a UI build

go:
	go build -ldflags '$(LDFLAGS)' -o initagent ./cmd/initagent

test:
	go vet ./...
	go test ./...

cross: ui
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building dist/initagent_$${os}_$${arch}"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' \
			-o dist/initagent_$${os}_$${arch} ./cmd/initagent || exit 1; \
	done

clean:
	rm -rf initagent dist ui/dist cmd/initagent/uidist
	mkdir -p cmd/initagent/uidist && touch cmd/initagent/uidist/.gitkeep
