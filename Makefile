.PHONY: build install clean reindex query report ui

BINARY=dbctx
INSTALL_DIR=$(HOME)/go/bin
DBCTX_DTX=dbctx.dtx

build:
	go build -o $(BINARY) .

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
