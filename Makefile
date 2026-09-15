BINARY := git-hotspots
DIST   := dist

# Cross-compile targets for `make release`. Windows binaries need the .exe
# suffix or the file just won't run there.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test clean release $(PLATFORMS)

build:
	go build -o $(BINARY) .

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
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(OUT) .
