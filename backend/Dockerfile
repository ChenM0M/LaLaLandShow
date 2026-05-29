FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/lalalandshow ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/lalalandshow /app/lalalandshow
COPY backend/migrations /app/migrations

ENV HTTP_ADDR=:8080
ENV MIGRATIONS_DIR=/app/migrations

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/lalalandshow"]
