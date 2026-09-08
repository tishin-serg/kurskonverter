FROM golang:1.27.1-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}" -o /out/bot ./cmd/bot

FROM alpine:3.24
RUN apk add --no-cache ca-certificates && addgroup -g 10001 bot && adduser -D -u 10001 -G bot bot && mkdir /data && chown bot:bot /data
COPY --from=builder /out/bot /usr/local/bin/bot
USER 10001:10001
WORKDIR /data
ENV SQLITE_PATH=/data/bot.db GOMEMLIMIT=384MiB GOMAXPROCS=1
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 CMD ["bot", "healthcheck"]
ENTRYPOINT ["bot"]
