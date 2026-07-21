
# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.5
FROM golang:${GO_VERSION}-alpine AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X coder-policy/internal/buildinfo.Version=${VERSION} -X coder-policy/internal/buildinfo.Commit=${COMMIT} -X coder-policy/internal/buildinfo.Date=${BUILD_DATE}" \
    -o /out/coder-policy ./cmd/coder-policy

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/coder-policy /usr/local/bin/coder-policy
USER nonroot:nonroot
EXPOSE 8080 8081 9090
ENTRYPOINT ["/usr/local/bin/coder-policy"]
CMD ["serve"]
