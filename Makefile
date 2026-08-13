# WARNING: dasm-burner generates dense OpenShift topologies (namespaces,
# routes, services, pods). NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT.

BINARY_NAME          := dasm-burner
WEB_NAME             := dasm-burner-web
BIN_DIR              := bin
IMAGE_TOOL           ?= podman
IMAGE_NAME           ?= ghcr.io/dasmlab/dasm-burner
WEB_IMAGE_NAME       ?= ghcr.io/dasmlab/dasm-burner-web
IMAGE_TAG            ?= latest
WEB_IMAGE_TAG        ?= dev
KUBE_BURNER_VERSION  := v2.8.1
KUBE_BURNER_ASSET    := kube-burner-V2.8.1-linux-x86_64.tar.gz
VERSION              ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS              := -X main.version=$(VERSION)
GOTOOLCHAIN          ?= local
export GOTOOLCHAIN

.PHONY: help
help: ## Show this help
	@echo "dasm-burner -- NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the CLI into ./bin
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/dasm-burner

.PHONY: build-web
build-web: ## Build the slim pod webserver
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "-s -w" -o $(BIN_DIR)/$(WEB_NAME) ./cmd/webserver

.PHONY: test
test: ## go vet + go test
	go vet ./...
	go test ./...

.PHONY: kube-burner
kube-burner: ## Download kube-burner v2.8.1 into ./bin
	mkdir -p $(BIN_DIR)
	curl -fsSL -o /tmp/$(KUBE_BURNER_ASSET) \
		https://github.com/kube-burner/kube-burner/releases/download/$(KUBE_BURNER_VERSION)/$(KUBE_BURNER_ASSET)
	tar -xzf /tmp/$(KUBE_BURNER_ASSET) -C $(BIN_DIR) kube-burner
	chmod +x $(BIN_DIR)/kube-burner
	$(BIN_DIR)/kube-burner version

.PHONY: image-web
image-web: ## Build the workload image (ghcr.io/dasmlab/dasm-burner-web:dev)
	$(IMAGE_TOOL) build -t $(WEB_IMAGE_NAME):$(WEB_IMAGE_TAG) \
		-f deployments/containers/Containerfile.webserver .

.PHONY: ui
ui: ## Build the Quasar UI into cmd/dasm-burner/static
	cd web && npm install && npm run build
	rm -rf cmd/dasm-burner/static/assets
	cp -a web/dist/. cmd/dasm-burner/static/

.PHONY: serve
serve: ui build ## Build UI+CLI and serve on :8080
	./$(BIN_DIR)/$(BINARY_NAME) serve --addr :8080 --config config/smoke.yaml --run-dir ./run

.PHONY: report-smoke
report-smoke: build ## Snapshot OVN/node health + last collected metrics
	./$(BIN_DIR)/$(BINARY_NAME) report --config config/smoke.yaml --out ./run

.PHONY: image
image: ## Build the product image (CLI + UI)
	$(IMAGE_TOOL) build -t $(IMAGE_NAME):$(IMAGE_TAG) \
		-f deployments/containers/Containerfile .

.PHONY: fmt
fmt: ## gofmt all Go source
	gofmt -l -w $$(find . -name '*.go' -not -path './vendor/*')

.PHONY: plan
plan: build ## Print default topology counts
	./$(BIN_DIR)/$(BINARY_NAME) plan --config config/route-service-density.yaml

.PHONY: plan-smoke
plan-smoke: build ## Print 2-namespace smoke counts
	./$(BIN_DIR)/$(BINARY_NAME) plan --config config/smoke.yaml

.PHONY: generate-smoke
generate-smoke: build ## Write smoke YAML under ./run (no apply)
	./$(BIN_DIR)/$(BINARY_NAME) generate --config config/smoke.yaml --out ./run

.PHONY: apply-dry-run
apply-dry-run: build ## Dry-run smoke apply (2 namespaces, 2 batches)
	./$(BIN_DIR)/$(BINARY_NAME) apply --config config/smoke.yaml --dry-run --out ./run

.PHONY: apply-smoke
apply-smoke: build ## Apply 2-namespace smoke with kube-burner measure+index (requires confirmation flags)
	./$(BIN_DIR)/$(BINARY_NAME) apply --config config/smoke.yaml \
		--i-understand-this-loads-the-control-plane --skip-baseline --measure --out ./run

.PHONY: status-smoke
status-smoke: build ## Convergence for the smoke seed/run-id
	./$(BIN_DIR)/$(BINARY_NAME) status --config config/smoke.yaml

.PHONY: cleanup-smoke
cleanup-smoke: build ## Delete smoke namespaces (seed 1837291)
	./$(BIN_DIR)/$(BINARY_NAME) cleanup --config config/smoke.yaml --yes --wait

