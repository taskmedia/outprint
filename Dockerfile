FROM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
# Test binaries first: they do not carry the version, so a new VERSION only
# invalidates the layer below.
RUN CGO_ENABLED=0 go test -c -tags golden -o /out/golden.test ./internal/golden \
 && CGO_ENABLED=0 go test -c -tags golden -o /out/worker.test ./internal/worker \
 && CGO_ENABLED=0 go build -trimpath -o /out/sandbox-probe ./cmd/sandbox-probe
# The build identity is linked in (internal/version). COMMIT is optional: the
# build context has no VCS data, so the release workflow and `task` pass it in.
ARG VERSION=dev
ARG COMMIT=
RUN LDFLAGS="-X github.com/taskmedia/outprint/internal/version.Version=${VERSION} -X github.com/taskmedia/outprint/internal/version.Commit=${COMMIT}" \
 && CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /out/outprint ./cmd/outprint \
 && CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /out/outprint-injector ./cmd/outprint-injector

FROM debian:trixie-20261027-slim AS outprint
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      pandoc \
      librsvg2-bin \
      texlive-luatex \
      texlive-latex-base \
      texlive-latex-recommended \
      fonts-texgyre \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/outprint /usr/local/bin/outprint
COPY templates /opt/outprint/templates
COPY configurations /opt/outprint/configurations
COPY config/service.yaml /opt/outprint/service.yaml
ENV OUTPRINT_TEMPLATES_ROOT=/opt/outprint/templates \
    OUTPRINT_CONFIGURATIONS_ROOT=/opt/outprint/configurations \
    OUTPRINT_SERVICE_CONFIG=/opt/outprint/service.yaml
RUN useradd --system --create-home --uid 10001 render \
 && install -d -o render -g render -m 0755 /opt/outprint/texcache
USER render
# Build the luaotfload cache once, at image build time. A job copies it into
# its own writable cache directory, so the fonts are not processed again.
RUN outprint warm-cache --template neutral --config example --out /opt/outprint/texcache \
 && chmod -R a+rX /opt/outprint/texcache
# Labels last: they cost no layer, and a new VERSION must not redo the TeX
# layers above.
ARG VERSION=dev
LABEL org.opencontainers.image.title="outprint" \
      org.opencontainers.image.description="Outprint for Outline: turn Outline documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork." \
      org.opencontainers.image.source="https://github.com/taskmedia/outprint" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
ENTRYPOINT ["/usr/local/bin/outprint"]

# Tier-2 golden tests run against the same toolchain as the release image,
# plus the poppler tools used to inspect the PDFs.
FROM outprint AS golden
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/golden.test /out/worker.test /usr/local/bin/
COPY scripts/run-golden.sh /usr/local/bin/run-golden.sh
COPY testdata /work/testdata
COPY go.mod /work/go.mod
USER render
WORKDIR /work
ENV OUTPRINT_REPO_ROOT=/work \
    SOURCE_DATE_EPOCH=1700000000 \
    FORCE_SOURCE_DATE=1
ENTRYPOINT ["/usr/local/bin/run-golden.sh"]
CMD ["-test.v"]

# Security layers 2 and 3 run the Worker image with an intentionally unsafe
# pipeline: the probe stands in for lualatex ahead of the real one on PATH and
# runs hostile actions on request (cmd/sandbox-probe). It exists in this stage
# only; the release image must not contain it (checked by the sandbox tests).
FROM outprint AS sandbox
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/sandbox-probe /usr/local/bin/sandbox-probe
COPY --from=build /out/sandbox-probe /usr/local/bin/lualatex
USER render

# The injector image: a reverse proxy in front of Outline. It carries no TeX
# toolchain, no shell and no package manager, only the one static binary.
# Build it with --target injector (task image-injector).
FROM gcr.io/distroless/static-debian13:nonroot AS injector
COPY --from=build /out/outprint-injector /outprint-injector
ARG VERSION=dev
LABEL org.opencontainers.image.title="outprint-injector" \
      org.opencontainers.image.description="Outprint for Outline: turn Outline documents into typeset PDFs (letters, reports, custom LaTeX templates) with a Download PDF button. Self-hosted, no Outline fork." \
      org.opencontainers.image.source="https://github.com/taskmedia/outprint" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
USER 65532:65532
EXPOSE 8090
ENTRYPOINT ["/outprint-injector"]

# The release image is the default target.
FROM outprint
