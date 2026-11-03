# ---------- build ----------
FROM golang:1.22-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
RUN go build -trimpath -ldflags="-s -w" -o /out/rma-server ./cmd/server

# ---------- runtime ----------
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata curl && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/rma-server /app/rma-server
USER app
EXPOSE 8080
ENV APP_PORT=8080 TZ=Asia/Shanghai
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/rma-server"]
