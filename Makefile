VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X briefrelay/internal/web.Version=$(VERSION)
GO      ?= go

.PHONY: build run test lint check perf release package-test clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/briefrelay ./cmd/briefrelay

run: build
	BRIEFRELAY_ENV=development ./dist/briefrelay serve

test:
	$(GO) test -race -count=1 ./...

# Seeds 500 clients / 5,000 projects / 25,000 versions / 100,000 events and checks p95 budgets (plan §6.1).
perf:
	BRIEFRELAY_PERF=1 $(GO) test -count=1 -run TestPerformanceBudgets -v ./internal/web/

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)
	$(GO) vet ./...
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: lint test

# Customer package (plan §11): static binaries for common hosts, docs, example config, deploy files,
# changelog, third-party licenses, dependency inventory + machine-readable SBOM, checksums.
release: clean
	mkdir -p dist
	sh scripts/licenses.sh > dist/THIRD_PARTY_LICENSES.txt
	$(GO) list -m -json all > dist/sbom.json
	@for os_arch in linux/amd64 linux/arm64; do \
	  os=$${os_arch%/*}; arch=$${os_arch#*/}; dir=dist/briefrelay-$(VERSION)-$$os-$$arch; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$dir/briefrelay ./cmd/briefrelay; \
	  cp .env.example README.md CHANGELOG.md LICENSE.md docker-compose.yml Dockerfile $$dir/; \
	  cp -r docs deploy $$dir/; cp dist/THIRD_PARTY_LICENSES.txt dist/sbom.json $$dir/; \
	  echo "$(VERSION)" > $$dir/VERSION; \
	  (cd dist && tar -czf briefrelay-$(VERSION)-$$os-$$arch.tar.gz briefrelay-$(VERSION)-$$os-$$arch); \
	done
	$(GO) version -m dist/briefrelay-$(VERSION)-linux-amd64/briefrelay > dist/dependencies.txt
	cd dist && sha256sum *.tar.gz > SHA256SUMS
	@ls -1 dist

# Installs from the exact archive a customer downloads and runs the sample workspace (plan §10 "package test").
package-test: release
	sh scripts/package-test.sh dist/briefrelay-$(VERSION)-linux-amd64.tar.gz

clean:
	rm -rf dist
