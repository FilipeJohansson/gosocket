GO_VERSION := 1.24
COVERAGE_FILE := coverage.out
PKG := ./...

.PHONY: fmt lint test install-tools clean

install-tools:
	@echo Installing development tools...
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securecodewarrior/github-action-gosec/gosec@latest

fmt:
	@echo Formatting code...
	go fmt $(PKG)
	goimports -w .

lint:
	@echo Running linters...
	golangci-lint run

test:
	@echo Running tests...
# 	go test -race -coverprofile=$(COVERAGE_FILE) $(PKG)
	go test -v -coverprofile=$(COVERAGE_FILE) $(PKG)

test-verbose:
	@echo Running tests...
	go test -v -race -coverprofile=$(COVERAGE_FILE) $(PKG)

test-coverage: test
	@echo Test coverage:
	go tool cover -func=$(COVERAGE_FILE)

test-coverage-html: test
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo Coverage report generated: coverage.html

build:
	@echo Building library...
	go build -v $(PKG)

clean:
	@echo Cleaning...
	go clean $(PKG)
	rm -f $(COVERAGE_FILE) coverage.html

mod-tidy:
	go mod tidy
	go mod verify

security:
	@echo Running security scan...
	gosec $(PKG)
