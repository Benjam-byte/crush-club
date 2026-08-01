FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY apps/api/go.mod apps/api/go.sum* ./
RUN go mod download
COPY apps/api ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.22 AS production
RUN apk add --no-cache ca-certificates wget && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/api /app/api
RUN mkdir -p /data/photos && chown -R app:app /data
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM production AS coolify
COPY --from=build /out/migrate /app/migrate
COPY --from=build /src/migrations /app/migrations
ENTRYPOINT ["/bin/sh", "-c", "/app/migrate up && exec /app/api"]

FROM alpine:3.22 AS migration
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/migrate /app/migrate
COPY --from=build /src/migrations /app/migrations
ENTRYPOINT ["/app/migrate", "up"]
