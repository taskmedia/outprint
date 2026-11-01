FROM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -o /out/renderer ./cmd/renderer \
 && CGO_ENABLED=0 go build -trimpath -o /out/injector ./cmd/injector \
 && CGO_ENABLED=0 go test -c -tags golden -o /out/golden.test ./internal/golden \
 && CGO_ENABLED=0 go test -c -tags golden -o /out/worker.test ./internal/worker \
 && CGO_ENABLED=0 go build -trimpath -o /out/sandbox-probe ./cmd/sandbox-probe

FROM debian:trixie-20261027-slim AS renderer
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      pandoc \
      librsvg2-bin \
      texlive-luatex \
      texlive-latex-base \
      texlive-latex-recommended \
      fonts-texgyre \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/renderer /usr/local/bin/renderer
COPY templates /opt/renderer/templates
COPY configurations /opt/renderer/configurations
COPY config/service.yaml /opt/renderer/service.yaml
ENV RENDERER_TEMPLATES_ROOT=/opt/renderer/templates \
    RENDERER_CONFIGURATIONS_ROOT=/opt/renderer/configurations \
    RENDERER_SERVICE_CONFIG=/opt/renderer/service.yaml
RUN useradd --system --create-home --uid 10001 render \
 && install -d -o render -g render -m 0755 /opt/renderer/texcache
USER render
# Build the luaotfload cache once, at image build time. A job copies it into
# its own writable cache directory, so the fonts are not processed again.
ENV RENDERER_TEXCACHE_DIR=/opt/renderer/texcache
RUN renderer warm-cache --template neutral --config example --out /opt/renderer/texcache \
 && chmod -R a+rX /opt/renderer/texcache
ENTRYPOINT ["/usr/local/bin/renderer"]

# Tier-2 golden tests run against the same toolchain as the release image,
# plus the poppler tools used to inspect the PDFs.
FROM renderer AS golden
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
ENV RENDERER_REPO_ROOT=/work \
    SOURCE_DATE_EPOCH=1700000000 \
    FORCE_SOURCE_DATE=1
ENTRYPOINT ["/usr/local/bin/run-golden.sh"]
CMD ["-test.v"]

# Security layers 2 and 3 run the Worker image with an intentionally unsafe
# pipeline: the probe stands in for lualatex ahead of the real one on PATH and
# runs hostile actions on request (cmd/sandbox-probe). It exists in this stage
# only; the release image must not contain it (checked by the sandbox tests).
FROM renderer AS sandbox
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/sandbox-probe /usr/local/bin/sandbox-probe
COPY --from=build /out/sandbox-probe /usr/local/bin/lualatex
USER render

# The injector image: a reverse proxy in front of Outline. It carries no TeX
# toolchain, no shell and no package manager, only the one static binary.
# Build it with --target injector (make image-injector).
FROM gcr.io/distroless/static-debian13:nonroot AS injector
COPY --from=build /out/injector /injector
USER 65532:65532
EXPOSE 8090
ENTRYPOINT ["/injector"]

# The release image is the default target.
FROM renderer
