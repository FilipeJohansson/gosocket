GO_VERSION := 1.24
COVERAGE_FILE := coverage.out
PKG := ./...

.PHONY: fmt lint test test-verbose test-race test-race-verbose test-integration docker-test docker-down install-tools clean

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
	@echo Running tests with Redis container...
	@$(MAKE) docker-test
	@go test -v -coverprofile=$(COVERAGE_FILE) $(PKG) \
		&& $(MAKE) docker-down || ($(MAKE) docker-down && exit 1)

test-verbose:
	@echo Running verbose tests with Redis container...
	@$(MAKE) docker-test
	@go test -v -coverprofile=$(COVERAGE_FILE) $(PKG) \
		&& $(MAKE) docker-down || ($(MAKE) docker-down && exit 1)

test-race:
	@echo Running race tests with Redis container...
	@$(MAKE) docker-test
	@go test -race -coverprofile=$(COVERAGE_FILE) $(PKG) \
		&& $(MAKE) docker-down || ($(MAKE) docker-down && exit 1)

test-race-verbose:
	@echo Running verbose race tests with Redis container...
	@$(MAKE) docker-test
	@go test -v -race -coverprofile=$(COVERAGE_FILE) $(PKG) \
		&& $(MAKE) docker-down || ($(MAKE) docker-down && exit 1)

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


archive:
	@echo Creating source archive...
	git archive --format=zip --output=source-archive.zip HEAD

ifeq ($(OS),Windows_NT)
docker-test:
	@echo Starting Redis for integration tests...
	@docker compose -f docker-compose.yml up -d
	@echo Waiting for Redis to be ready...
	@powershell -Command "for ($$i=0; $$i -lt 15; $$i++) { $$out = docker compose -f docker-compose.yml exec -T redis redis-cli ping 2>$$null; if ($$out -match 'PONG') { Write-Host 'redis ready'; exit 0 } Start-Sleep -Seconds 1 }; Write-Host 'redis did not become ready'; exit 1"
else
docker-test:
	@echo Starting Redis for integration tests...
	@docker compose -f docker-compose.yml up -d
	@echo Waiting for Redis to be ready...
	@sh -c 'i=0; until [ $$i -ge 15 ]; do if docker compose -f docker-compose.yml exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; then echo "redis ready"; break; fi; i=$$((i+1)); sleep 1; done; if [ $$i -ge 15 ]; then echo "redis did not become ready"; exit 1; fi'
endif

docker-down:
	@echo Stopping test containers...
	@docker compose -f docker-compose.yml down --remove-orphans

test-integration:
	@echo Running integration tests with Redis container...
	@$(MAKE) docker-test
	@go test -v ./... -run Integration \
		&& $(MAKE) docker-down || ($(MAKE) docker-down && exit 1)
