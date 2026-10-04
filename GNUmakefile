BIN_DIR = $(CURDIR)/bin

all: build

build:
	CGO_ENABLED=0 GOBIN=$(BIN_DIR) go install $(CURDIR)/...

check: vet

vet:
	go vet $(CURDIR)/...

test:
	go test -race -count 1 -timeout 10s $(CURDIR)/...

clean:
	$(RM) $(wildcard bin/*)

.PHONY: all build check vet test clean
