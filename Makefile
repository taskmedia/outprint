IMAGE        ?= outline-latex-pdf-renderer
GOLDEN_IMAGE ?= $(IMAGE)-golden
# Fixed so that PDFs are reproducible across runs.
SOURCE_DATE_EPOCH ?= 1700000000

.PHONY: build test image test-golden update-goldens sanitize test-sanitize

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
