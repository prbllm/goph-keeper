SHELL=/bin/bash
GOTOOLCHAIN=go1.25.7

CLIENT = goph-keeper-client
BUILD_DATE = $(shell date +'%Y/%m/%d %H:%M:%S')
VERSION = v1.0.1
LDFLAGS = -X 'github.com/prbllm/goph-keeper/pkg/version.Version=$(VERSION)' -X 'github.com/prbllm/goph-keeper/pkg/version.BuildDate=$(BUILD_DATE)'

.PHONY: clear prep perm

all: prep clients perm

clear:
	rm -rf bin/*

prep:
	GOTOOLCHAIN=$(GOTOOLCHAIN) go mod tidy

clients:
	# Linux
	GOOS=linux GOARCH=amd64 GOTOOLCHAIN=$(GOTOOLCHAIN) go build -buildvcs=false -ldflags "$(LDFLAGS)" -o=bin/$(CLIENT)-linux-amd64 ./cmd/client/
	# Windows
	GOOS=windows GOARCH=amd64 GOTOOLCHAIN=$(GOTOOLCHAIN) go build -buildvcs=false -ldflags "$(LDFLAGS)" -o=bin/$(CLIENT)-windows-amd64.exe ./cmd/client/
	# Darwin (macOS)
	GOOS=darwin GOARCH=amd64 GOTOOLCHAIN=$(GOTOOLCHAIN) go build -buildvcs=false -ldflags "$(LDFLAGS)" -o=bin/$(CLIENT)-darwin-amd64 ./cmd/client/
	GOOS=darwin GOARCH=arm64 GOTOOLCHAIN=$(GOTOOLCHAIN) go build -buildvcs=false -ldflags "$(LDFLAGS)" -o=bin/$(CLIENT)-darwin-arm64 ./cmd/client/

perm:
	chmod -R +x bin
