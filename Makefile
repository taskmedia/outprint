SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -ec

IMAGE         ?= outline-latex-pdf-renderer
GOLDEN_IMAGE  ?= $(IMAGE)-golden
SANDBOX_IMAGE ?= $(IMAGE)-sandbox
# Where the security targets keep their test output for the skip guard.
SECURITY_LOG_DIR ?= .security-logs
# -v for people, -json for CI.
GOTEST_FLAGS  ?= -v
SANDBOX_PARALLEL ?= 4
# Fixed so that PDFs are reproducible across runs.
SOURCE_DATE_EPOCH ?= 1700000000

.PHONY: build test image test-golden test-security test-security-layer1 test-sandbox test-guard update-goldens sanitize test-sanitize

build:
	go build -o bin/renderer ./cmd/renderer

# Tier 1: unit tests, no external tools.
test:
	go vet ./...
	go test ./...

image:
	docker build -t $(IMAGE) .

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
