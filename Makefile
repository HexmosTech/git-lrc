.PHONY: all build install clean reindex query report ui doc test test-short test-cover test-integration

BINARY=dbctx
INSTALL_DIR=$(HOME)/go/bin
DBCTX_DTX=dbctx.dtx

all: build test

build:
	go build -o $(BINARY) ./cmd/dbctx

install: build
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -f $(BINARY) $(DBCTX_DTX)

reindex: build
	rm -f $(DBCTX_DTX)
	@source .env.prod && ./$(BINARY) build "$$DATABASE_URL" --output $(DBCTX_DTX)

$(DBCTX_DTX):
	@source .env.prod && ./$(BINARY) build "$$DATABASE_URL" --output $(DBCTX_DTX)

query: $(DBCTX_DTX)
	@test -n "$(Q)" || (echo "Usage: make query Q='your question here'" && exit 1)
	./$(BINARY) query $(DBCTX_DTX) "$(Q)"

report: $(DBCTX_DTX)
	./$(BINARY) report $(DBCTX_DTX)

ui: $(DBCTX_DTX)
	./$(BINARY) ui $(DBCTX_DTX)

doc:
	@echo "Serving docs at http://localhost:6060/pkg/github.com/shrsv/dbctx/"
	@echo "Press Ctrl+C to stop."
	godoc -http=:6060

test:
	go test ./... -count=1

test-short:
	go test ./... -short -count=1

test-cover:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out

test-integration:
	go test -tags integration . -v -count=1 -timeout 120s
