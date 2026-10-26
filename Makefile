WAILS_VERSION := v3.0.0-beta.34
WAILS := $(shell go env GOPATH)/bin/wails3
export PATH := $(dir $(WAILS)):$(PATH)
VERSION ?= 0.5.0-beta.2
export SADPTOOLS_VERSION := $(VERSION)
VERSION_FLAGS := -X github.com/TerminalAddict/SADPTools/internal/version.Value=$(VERSION)

.PHONY: tools cli frontend bindings windows windows-installer installer linux-gui linux-packages check dev branding release-build release-publish

tools:
	CGO_ENABLED=0 go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

branding:
	python3 build/brand/generate.py

cli:
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='$(VERSION_FLAGS)' -o bin/sadptools ./cmd/sadptools

bindings:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(WAILS) generate bindings -f '-tags desktop' -clean=true

frontend: bindings
	npm --prefix frontend ci
	npm --prefix frontend run build

windows:
	$(WAILS) build GOOS=windows GOARCH=amd64
	mkdir -p bin/LICENSES
	cp LICENSE THIRD_PARTY_NOTICES.md bin/
	cp LICENSES/* bin/LICENSES/

linux-gui:
	$(WAILS) build GOOS=linux GOARCH=amd64

linux-packages:
	$(WAILS) package GOOS=linux GOARCH=amd64

installer: windows-installer

windows-installer:
	$(WAILS) package GOOS=windows GOARCH=amd64

check:
	go test ./...
	go vet ./...
	go test -race ./...
	npm --prefix frontend test
	python3 -m unittest discover -s build/windows -p 'test_*.py'
	python3 -m unittest discover -s build -p 'test_*.py'
	python3 -m unittest discover -s build/linux -p 'test_*.py'

release-build:
	python3 build/release.py build '$(VERSION)'

release-publish:
	python3 build/release.py publish '$(VERSION)'

dev:
	$(WAILS) dev -config build/config.yml
