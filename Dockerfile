# syntax=docker/dockerfile:1.7

FROM golang:1.23 AS build
WORKDIR /src

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath \
  -ldflags="-s -w \
    -X 'main.version=${VERSION}' \
    -X 'main.commit=${COMMIT}' \
    -X 'main.date=${DATE}'" \
  -o /out/runner ./cmd/runner

# Runtime
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/runner /runner
USER nonroot:nonroot
ENTRYPOINT ["/runner"]
