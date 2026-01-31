NPM ?= npm
FRONTEND_DIR := frontend

.PHONY: frontend-install frontend-build demo

frontend-install:
	cd $(FRONTEND_DIR) && $(NPM) install

frontend-build: frontend-install
	cd $(FRONTEND_DIR) && $(NPM) run build

# Run the demo server (depends on built admin UI so embeds are present)
demo: frontend-build
	go run ./cmd/demo
