# syntax=docker/dockerfile:1

# mc: the MeshCore terminal client. Built CGO-free — Bluetooth on Linux uses
# D-Bus (pure Go), so no cgo is needed and the result is a static binary.

FROM golang:1.25-bookworm AS build
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w \
          -X github.com/meshcore-cz/meshcore-go/cmd/mc/internal/cli.Version=${VERSION} \
          -X github.com/meshcore-cz/meshcore-go/cmd/mc/internal/cli.Commit=${COMMIT} \
          -X github.com/meshcore-cz/meshcore-go/backend.Version=${VERSION}" \
        -o /out/mc ./cmd/mc

FROM gcr.io/distroless/static-debian12 AS runtime
COPY --from=build /out/mc /usr/local/bin/mc
ENTRYPOINT ["mc"]
