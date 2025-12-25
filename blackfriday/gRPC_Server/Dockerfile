# ===== build =====
FROM golang:1.24 AS builder
WORKDIR /app

# go.mod y go.sum están en blackfriday/
COPY go.mod go.sum ./
RUN go mod download

# Copiar proto + server
COPY proto ./proto
COPY gRPC_Server ./gRPC_Server

# Compilar binario del gRPC server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o grpc-server ./gRPC_Server

# ===== runtime =====
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=builder /app/grpc-server /grpc-server

EXPOSE 50051
CMD ["/grpc-server"]
