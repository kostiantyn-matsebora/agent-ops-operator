# Local development for the console.
#
# `go build` needs ui/dist to exist, because the SPA is embedded. `make ui`
# produces it; `make dev-build` skips it entirely via the `dev` build tag, which
# serves the SPA from disk instead.

UI := ui
DIST := $(UI)/dist

.PHONY: ui
ui: ## build the SPA into ui/dist (what go:embed needs)
	cd $(UI) && npm install && npm run build

.PHONY: build
build: $(DIST) ## build the console binary with the SPA embedded
	go build -o bin/console .

$(DIST):
	$(MAKE) ui

.PHONY: dev-build
dev-build: ## build without embedding; serves ui/dist (or $UI_DIR) from disk
	go build -tags dev -o bin/console-dev .

.PHONY: test
test: ## Go tests + the frontend suite
	go test ./...
	cd $(UI) && npm test

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: clean
clean:
	rm -rf bin $(DIST)
