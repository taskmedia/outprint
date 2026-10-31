FROM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/renderer ./cmd/renderer \
 && CGO_ENABLED=0 go test -c -tags golden -o /out/golden.test ./internal/golden

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
RUN useradd --system --create-home --uid 10001 render
USER render
ENTRYPOINT ["/usr/local/bin/renderer"]

# Tier-2 golden tests run against the same toolchain as the release image,
# plus the poppler tools used to inspect the PDFs.
FROM renderer AS golden
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/golden.test /usr/local/bin/golden.test
COPY testdata /work/testdata
COPY go.mod /work/go.mod
USER render
WORKDIR /work
ENV RENDERER_REPO_ROOT=/work \
    SOURCE_DATE_EPOCH=1700000000 \
    FORCE_SOURCE_DATE=1
ENTRYPOINT ["/usr/local/bin/golden.test"]
CMD ["-test.v"]

# The release image is the default target.
FROM renderer
