FROM golang:1.26-alpine AS build
WORKDIR /src

RUN apk add --no-cache git

# Set at build time, e.g. --build-arg VERSION=$(git describe --tags --always).
# Defaults to "dev" (see internal/version) when not passed.
ARG VERSION=dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X deuce/backend/internal/version.Version=${VERSION}" \
    -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/server ./server
COPY migrations ./migrations
USER appuser

EXPOSE 8080
ENTRYPOINT ["./server"]
