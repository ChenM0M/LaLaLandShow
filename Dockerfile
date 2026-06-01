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

FROM nginx:1.27-alpine

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=backend-build /out/lalalandshow /app/lalalandshow
COPY backend/migrations /app/migrations
COPY --from=frontend-build /src/dist /usr/share/nginx/html
COPY deploy/nginx.app.conf /etc/nginx/conf.d/default.conf
COPY deploy/app-entrypoint.sh /app/app-entrypoint.sh

ENV APP_ENV=production
ENV HTTP_ADDR=127.0.0.1:8080
ENV MIGRATIONS_DIR=/app/migrations

EXPOSE 80
ENTRYPOINT ["/app/app-entrypoint.sh"]
