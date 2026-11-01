SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -ec

IMAGE         ?= outline-latex-pdf-renderer
INJECTOR_IMAGE ?= outline-latex-pdf-injector
GOLDEN_IMAGE  ?= $(IMAGE)-golden
SANDBOX_IMAGE ?= $(IMAGE)-sandbox
# Where the security targets keep their test output for the skip guard.
SECURITY_LOG_DIR ?= .security-logs
# -v for people, -json for CI.
GOTEST_FLAGS  ?= -v
SANDBOX_PARALLEL ?= 4
# Fixed so that PDFs are reproducible across runs.
SOURCE_DATE_EPOCH ?= 1700000000

.PHONY: dev dev-up dev-secrets seed dev-down dev-down-volumes dev-logs dev-clean e2e e2e-install build test test-web image image-injector test-golden test-security test-security-layer1 test-sandbox test-guard update-goldens sanitize test-sanitize audit test-audit

build:
	go build -o bin/renderer ./cmd/renderer
	go build -o bin/injector ./cmd/injector

# Tier 1: unit tests, no external tools besides Go. The button script tests
# need Node and are skipped, with a note, where it is not installed; CI
# always has it.
test: test-web
	go vet ./...
	go test ./...

test-web:
	@if command -v node >/dev/null 2>&1; then \
		node --test web/*.test.js; \
	else \
		echo "note: node is not installed; skipping the button script tests"; \
	fi

image:
	docker build -t $(IMAGE) .

# The slim injector image: no TeX toolchain.
image-injector:
	docker build --target injector -t $(INJECTOR_IMAGE) .

# Tier 2: goldens, run inside the pinned renderer image.
test-golden:
	docker build --target golden -t $(GOLDEN_IMAGE) .
	docker run --rm $(GOLDEN_IMAGE)

# Security layers 1 to 3. A skipped test counts as a failure.
test-security: test-security-layer1 test-sandbox

# Layer 1: the hostile-payload corpus and the engine lockdown tests, in the
# golden image. Part of test-golden as well; this is the quick way to run it.
test-security-layer1:
	mkdir -p $(SECURITY_LOG_DIR)
	docker build --target golden -t $(GOLDEN_IMAGE) .
	docker run --rm -e GOLDEN_RUN='Security|TestEngine|TestFilters|TestRender|TestSVG' $(GOLDEN_IMAGE) -test.v 2>&1 | tee $(SECURITY_LOG_DIR)/layer1.log
	./scripts/security-guard.sh $(SECURITY_LOG_DIR)/layer1.log

# Layers 2 and 3: the Worker image run with the flags of the shipped Compose
# file against a hostile pipeline (docs/security.md). Needs a Docker daemon.
# The "sandbox" image stage holds the test-only probe; the release image must
# not, and the suite checks that.
test-sandbox:
	mkdir -p $(SECURITY_LOG_DIR)
	docker build -t $(IMAGE) .
	docker build --target sandbox -t $(SANDBOX_IMAGE) .
	rc=0; RELEASE_IMAGE=$(IMAGE) SANDBOX_IMAGE=$(SANDBOX_IMAGE) \
		go test -tags sandbox $(GOTEST_FLAGS) -parallel $(SANDBOX_PARALLEL) -count=1 -timeout 15m ./internal/sandbox/... \
		> $(SECURITY_LOG_DIR)/sandbox.log 2>&1 || rc=$$?; \
		cat $(SECURITY_LOG_DIR)/sandbox.log | tail -n 400; \
		./scripts/security-guard.sh $(SECURITY_LOG_DIR)/sandbox.log || rc=1; exit $$rc

test-guard:
	./scripts/security_guard_test.sh

# Regenerates goldens inside the pinned image only, never on the host.
update-goldens:
	docker build --target golden -t $(GOLDEN_IMAGE) .
	docker run --rm --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR)/testdata:/work/testdata" \
		-e UPDATE_GOLDENS=1 \
		-e SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) -e FORCE_SOURCE_DATE=1 \
		$(GOLDEN_IMAGE)

sanitize:
	./scripts/sanitize.sh

test-sanitize:
	./scripts/sanitize_test.sh

# Pre-publish audit (docs/sanitize.md, docs/publishing.md): make sanitize plus
# a scan of issues and pull requests through gh. Read-only.
audit:
	./scripts/prepublish-audit.sh

test-audit:
	./scripts/prepublish_audit_test.sh

# Development stack (docs/development.md): Outline with login, the injector,
# Gateway and Worker, seeded with a fictional user, a bound and an unbound
# Collection. DEV ONLY.
DEV_COMPOSE ?= docker compose --env-file deploy/dev/versions.env -f deploy/compose/compose.yaml -f deploy/dev/compose.yaml
DEV_URL     ?= http://localhost:8090

dev-secrets:
	./deploy/dev/gen-secrets.sh

dev-up: dev-secrets
	$(DEV_COMPOSE) up -d --build --wait

# The one seeding implementation, shared with the browser tests. Safe to rerun.
seed:
	go run ./cmd/seed -url $(DEV_URL)

dev: dev-up seed

dev-down:
	$(DEV_COMPOSE) down --remove-orphans

# Also removes the database and the files: the next `make dev` starts empty.
dev-down-volumes:
	$(DEV_COMPOSE) down --remove-orphans --volumes

dev-logs:
	$(DEV_COMPOSE) logs -f --tail=100

# Forget the generated dev secrets and the seed output (stop the stack with
# volumes first, or Outline will not accept the new keys).
dev-clean:
	rm -rf deploy/dev/.generated

# Tier 3: browser end-to-end tests against the dev stack. Needs Docker, Go and
# Node. Starts the stack when it is not running.
e2e-install:
	cd e2e && npm ci && npx playwright install chromium

e2e: dev-up seed
	cd e2e && npx playwright test
