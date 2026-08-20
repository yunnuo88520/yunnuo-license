ARG GO_IMAGE=golang:1.26-bookworm
ARG MYSQL_IMAGE=mysql:8.4
ARG NODE_IMAGE=node:22-bookworm-slim
ARG APP_VERSION=0.2.0
ARG VCS_REF=unknown
ARG BUILD_TIME=unknown

FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM ${GO_IMAGE} AS builder
ARG APP_VERSION
ARG VCS_REF
ARG BUILD_TIME

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Version=${APP_VERSION} -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Commit=${VCS_REF} -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.BuildTime=${BUILD_TIME}" -o /out/yunnuo-server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunnuo-healthcheck ./cmd/healthcheck

FROM ${MYSQL_IMAGE}

ARG APP_VERSION
ARG VCS_REF
ARG BUILD_TIME
LABEL org.opencontainers.image.title="Yunnuo License" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_TIME}"

COPY --from=builder /out/yunnuo-server /usr/local/bin/yunnuo-server
COPY --from=builder /out/yunnuo-healthcheck /usr/local/bin/yunnuo-healthcheck
COPY backend/migrations /opt/yunnuo/backend/migrations
COPY --from=frontend-builder /src/frontend/dist /opt/yunnuo/frontend
COPY docker/entrypoint.sh /usr/local/bin/yunnuo-entrypoint

RUN chmod 0755 /usr/local/bin/yunnuo-server /usr/local/bin/yunnuo-healthcheck /usr/local/bin/yunnuo-entrypoint

ENV YN_ADDR=:8080 \
    YN_DB_CONFIG_FILE=/var/lib/mysql/.yunnuo/database.json \
    YN_SQLITE_SETUP_DSN="file:/var/lib/mysql/.yunnuo/yn-license.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)" \
    YN_PUBLIC_STATIC_DIR=/opt/yunnuo/frontend \
    YN_ADMIN_STATIC_DIR=/opt/yunnuo/frontend/admin-console \
    YN_AGENT_STATIC_DIR=/opt/yunnuo/frontend/agent-console \
    MYSQL_DATABASE=yunnuo_license \
    MYSQL_USER=yunnuo

EXPOSE 8080
VOLUME ["/var/lib/mysql"]
HEALTHCHECK --interval=10s --timeout=4s --start-period=45s --retries=6 CMD ["/usr/local/bin/yunnuo-healthcheck"]
ENTRYPOINT ["/usr/local/bin/yunnuo-entrypoint"]
