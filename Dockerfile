ARG GO_IMAGE=golang:1.26-bookworm
ARG MYSQL_IMAGE=mysql:8.4
ARG NODE_IMAGE=node:22-bookworm-slim
# Defaults to the repo-root VERSION file; pass APP_VERSION to override.
ARG APP_VERSION=
ARG VCS_REF=unknown
ARG BUILD_TIME=unknown

FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /src/frontend
COPY VERSION /src/VERSION
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
COPY sdk/node/src/request.js sdk/node/src/web.js sdk/node/src/web.d.ts sdk/node/src/index.d.ts /src/sdk/node/src/
RUN npm run build

FROM ${GO_IMAGE} AS builder
ARG APP_VERSION
ARG VCS_REF
ARG BUILD_TIME
COPY VERSION /src/VERSION

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOBIN=/out go install github.com/tianon/gosu@v0.0.0-20250923190938-6456aaa0f3c8
RUN APP_VERSION=${APP_VERSION:-$(tr -d "[:space:]" < /src/VERSION)} \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Version=${APP_VERSION} -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.Commit=${VCS_REF} -X github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo.BuildTime=${BUILD_TIME}" -o /out/yunnuo-server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunnuo-healthcheck ./cmd/healthcheck \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunnuo-ops ./cmd/maintenance

FROM ${MYSQL_IMAGE}

ARG VCS_REF
ARG BUILD_TIME
LABEL org.opencontainers.image.title="Yunnuo License" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_TIME}"

COPY --from=builder /out/gosu /usr/local/bin/gosu
COPY --from=builder /out/yunnuo-server /usr/local/bin/yunnuo-server
COPY --from=builder /out/yunnuo-ops /usr/local/bin/yunnuo-ops
COPY --from=builder /out/yunnuo-healthcheck /usr/local/bin/yunnuo-healthcheck
COPY backend/migrations /opt/yunnuo/backend/migrations
COPY LICENSE /opt/yunnuo/LICENSE
COPY sdk /opt/yunnuo/sdk
COPY --from=frontend-builder /src/frontend/dist /opt/yunnuo/frontend
COPY docker/entrypoint.sh /usr/local/bin/yunnuo-entrypoint

RUN rpm -e mysql-shell \
    && microdnf update -y --disablerepo='mysql*' libbrotli \
    && microdnf clean all \
    && chmod 0755 /usr/local/bin/yunnuo-server /usr/local/bin/yunnuo-healthcheck /usr/local/bin/yunnuo-entrypoint

ENV YN_ADDR=:8080 \
    YN_SDK_DIR=/opt/yunnuo/sdk \
    YN_RELEASE_DIR=/var/lib/mysql/.yunnuo/releases \
    YN_DB_CONFIG_FILE=/var/lib/mysql/.yunnuo/database.json \
    YN_SQLITE_SETUP_DSN="file:/var/lib/mysql/.yunnuo/yn-license.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)" \
    YN_PUBLIC_STATIC_DIR=/opt/yunnuo/frontend \
    YN_ADMIN_STATIC_DIR=/opt/yunnuo/frontend/admin-console \
    YN_AGENT_STATIC_DIR=/opt/yunnuo/frontend/agent-console \
    YN_DEVELOPER_STATIC_DIR=/opt/yunnuo/frontend/developer-console \
    YN_CUSTOMER_STATIC_DIR=/opt/yunnuo/frontend/user-console \
    MYSQL_DATABASE=yunnuo_license \
    MYSQL_USER=yunnuo

EXPOSE 8080
VOLUME ["/var/lib/mysql"]
HEALTHCHECK --interval=10s --timeout=4s --start-period=45s --retries=6 CMD ["/usr/local/bin/yunnuo-healthcheck"]
ENTRYPOINT ["/usr/local/bin/yunnuo-entrypoint"]
