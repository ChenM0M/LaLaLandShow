FROM node:22-alpine AS frontend-build

WORKDIR /src
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-bookworm AS backend-build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/lalalandshow ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata postgresql16 postgresql16-client redis

WORKDIR /app
COPY --from=backend-build /out/lalalandshow /app/lalalandshow
COPY backend/migrations /app/migrations
COPY --from=frontend-build /src/dist /app/public
COPY deploy/app-entrypoint.sh /app/app-entrypoint.sh

ENV APP_ENV=standalone
ENV HTTP_ADDR=:8080
ENV MIGRATIONS_DIR=/app/migrations
ENV STATIC_DIR=/app/public
ENV RUN_MIGRATIONS=true
ENV SEED_SEATS=true

VOLUME ["/data"]

EXPOSE 8080
ENTRYPOINT ["/app/app-entrypoint.sh"]
