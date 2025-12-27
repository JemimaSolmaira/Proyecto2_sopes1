# ===== build =====
FROM golang:1.24 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/k8s_kafka .

# ===== runtime =====
FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/k8s_kafka /k8s_kafka
USER nonroot:nonroot
CMD ["/k8s_kafka"]
