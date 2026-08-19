ARG GO_IMAGE=golang:1.26-bookworm
ARG MYSQL_IMAGE=mysql:8.4
ARG NODE_IMAGE=node:22-bookworm-slim

FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM ${GO_IMAGE} AS builder

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunnuo-server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunnuo-healthcheck ./cmd/healthcheck

FROM ${MYSQL_IMAGE}

COPY --from=builder /out/yunnuo-server /usr/local/bin/yunnuo-server
COPY --from=builder /out/yunnuo-healthcheck /usr/local/bin/yunnuo-healthcheck
COPY backend/migrations /opt/yunnuo/backend/migrations
COPY --from=frontend-builder /src/frontend/dist /opt/yunnuo/frontend
COPY docker/entrypoint.sh /usr/local/bin/yunnuo-entrypoint

RUN chmod 0755 /usr/local/bin/yunnuo-server /usr/local/bin/yunnuo-healthcheck /usr/local/bin/yunnuo-entrypoint

ENV YN_ADDR=:8080 \
    YN_DB_DRIVER=mysql \
    YN_PUBLIC_STATIC_DIR=/opt/yunnuo/frontend \
    YN_ADMIN_STATIC_DIR=/opt/yunnuo/frontend/admin-console \
    YN_AGENT_STATIC_DIR=/opt/yunnuo/frontend/agent-console \
    MYSQL_DATABASE=yunnuo_license \
    MYSQL_USER=yunnuo

EXPOSE 8080
VOLUME ["/var/lib/mysql"]
HEALTHCHECK --interval=10s --timeout=4s --start-period=45s --retries=6 CMD ["/usr/local/bin/yunnuo-healthcheck"]
ENTRYPOINT ["/usr/local/bin/yunnuo-entrypoint"]
