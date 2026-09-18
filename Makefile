BINARY := git-hotspots
DIST   := dist

# Falls back to "dev" when there's no tag yet (or building outside a git
# checkout, e.g. from a source tarball).
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Cross-compile targets for `make release`. Windows binaries need the .exe
# suffix or the file just won't run there.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test clean release $(PLATFORMS)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

clean:
	rm -rf $(BINARY) $(DIST)

release: $(PLATFORMS)

$(PLATFORMS):
	$(eval GOOS := $(word 1,$(subst /, ,$@)))
	$(eval GOARCH := $(word 2,$(subst /, ,$@)))
	$(eval OUT := $(DIST)/$(BINARY)-$(GOOS)-$(GOARCH)$(if $(findstring windows,$(GOOS)),.exe))
	mkdir -p $(DIST)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(OUT) .
