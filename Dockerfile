# ---- build stage ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine3.21 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /app/fsb -ldflags="-w -s" ./cmd/fsb

# ---- runtime stage ----
FROM alpine:3.21
# ca-certificates: outbound HTTPS | tzdata: correct log timestamps
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 fsb \
    && mkdir -p /app/logs /app/sessions \
    && chown -R fsb:fsb /app
WORKDIR /app
COPY --from=builder /app/fsb /app/fsb
USER fsb
# Koyeb routes traffic to port 8000 by default
ENV PORT=8000
EXPOSE 8000
ENTRYPOINT ["/app/fsb", "run"]
