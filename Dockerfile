FROM golang:1.27.3 AS backend_builder

ARG TARGETPLATFORM

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY ./cmd /app/cmd
COPY ./dev-scripts /app/dev-scripts
COPY ./handlers /app/handlers
COPY ./limit /app/limit
COPY ./random /app/random
COPY ./store /app/store

RUN TARGETPLATFORM="${TARGETPLATFORM}" ./dev-scripts/build-backend "prod"

FROM scratch AS artifact
COPY --from=backend_builder /app/bin/logpaste ./

FROM litestream/litestream:0.3.9 AS litestream

FROM alpine:3.15 AS upstream

RUN apk add --no-cache bash

COPY --from=backend_builder /app/bin/logpaste /app/logpaste
COPY --from=litestream /usr/local/bin/litestream /app/litestream
COPY ./docker-entrypoint /app/docker-entrypoint
COPY ./litestream.yml /etc/litestream.yml
COPY ./views /app/views

WORKDIR /app

# Frequency that database snapshots are replicated.
ENV DB_SYNC_INTERVAL="10s"

ENTRYPOINT ["/app/docker-entrypoint"]

FROM upstream AS vibenest
EXPOSE 3001
HEALTHCHECK --interval=30s --timeout=3s CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3001/healthz"]
ENTRYPOINT ["/app/logpaste", "vibenest-pilot"]
