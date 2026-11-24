# WRT AI — build frontend, backend, cross-compile, package
VERSION ?= 0.1.0
GO_LDFLAGS = -s -w -X github.com/sumce/openwrt-ai-assistant/internal/server.Version=$(VERSION)

LINUX_TARGETS := amd64 arm64 armv7 armv6 mips mipsle

.PHONY: frontend backend build dev test cross ipk clean

## frontend: build the React UI into web/dist (embedded by go:embed)
frontend:
	cd web && npm install --no-audit --no-fund && npm run build

## backend: build the host binary (uses whatever is in web/dist)
backend:
	go build -trimpath -ldflags "$(GO_LDFLAGS)" -o wrtai ./cmd/wrtai

## build: full host build (frontend + backend)
build: frontend backend

## dev: run with mocked router tools + scripted fake LLM (no API key needed)
dev:
	go run ./cmd/wrtai -dev -port 8765

## test: run Go tests
test:
	go test ./...

## cross: cross-compile static binaries for all supported architectures
cross: frontend
	@mkdir -p dist
	set -e; for t in $(LINUX_TARGETS); do \
		unset GOARM GOMIPS GOARCH; \
		case $$t in \
			amd64)  export GOARCH=amd64 ;; \
			arm64)  export GOARCH=arm64 ;; \
			armv7)  export GOARCH=arm GOARM=7 ;; \
			armv6)  export GOARCH=arm GOARM=6 ;; \
			mips)   export GOARCH=mips GOMIPS=softfloat ;; \
			mipsle) export GOARCH=mipsle GOMIPS=softfloat ;; \
		esac; \
		GOOS=linux CGO_ENABLED=0 \
		go build -trimpath -ldflags "$(GO_LDFLAGS)" -o dist/linux-$$t/wrtai ./cmd/wrtai; \
		echo "built dist/linux-$$t/wrtai"; \
	done

## ipk: print instructions for buildroot packaging
ipk:
	@echo "Copy openwrt/wrtai into your OpenWrt buildroot/package/, then:"
	@echo "  make package/wrtai/compile V=s"
	@echo "(requires dist/ binaries from 'make cross' for the target arch)"

## clean: remove build outputs
clean:
	rm -rf dist wrtai wrtai.exe
