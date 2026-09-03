VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X briefrelay/internal/web.Version=$(VERSION)
GO      ?= go

.PHONY: build run test lint check release clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/briefrelay ./cmd/briefrelay

run: build
	BRIEFRELAY_ENV=development ./dist/briefrelay serve

test:
	$(GO) test -race -count=1 ./...

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)
	$(GO) vet ./...
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: lint test

# Customer package: static binaries for common hosts + checksums.
release: clean
	@for os_arch in linux/amd64 linux/arm64; do \
	  os=$${os_arch%/*}; arch=$${os_arch#*/}; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/briefrelay-$(VERSION)-$$os-$$arch/briefrelay ./cmd/briefrelay; \
	  cp .env.example README.md dist/briefrelay-$(VERSION)-$$os-$$arch/; \
	  (cd dist && tar -czf briefrelay-$(VERSION)-$$os-$$arch.tar.gz briefrelay-$(VERSION)-$$os-$$arch); \
	done
	cd dist && sha256sum *.tar.gz > SHA256SUMS
	$(GO) version -m dist/briefrelay-$(VERSION)-linux-amd64/briefrelay > dist/dependencies.txt
	@ls -1 dist

clean:
	rm -rf dist
