# ===== build =====
FROM golang:1.24 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY proto ./proto
COPY gRPC_Client ./gRPC_Client

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o grpc-client ./gRPC_Client

# ===== runtime =====
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=builder /app/grpc-client /grpc-client

EXPOSE 8081
ENV GRPC_SERVER_ADDR=grpc-server:50051
CMD ["/grpc-client"]
