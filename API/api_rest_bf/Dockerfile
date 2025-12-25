# ===== build =====
FROM rust:1.92 AS builder
WORKDIR /app

# protoc para tonic-build
RUN apt-get update && apt-get install -y protobuf-compiler pkg-config && rm -rf /var/lib/apt/lists/*

# Copiar manifiestos
COPY Cargo.toml Cargo.lock ./
COPY build.rs ./
COPY proto ./proto
COPY src ./src

# Compilar
RUN cargo build --release

# ===== runtime =====
FROM debian:bookworm-slim
WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/api_rest_bf /app/api_rest_bf

EXPOSE 8080
ENV GRPC_SERVER_ADDR=http://grpc-server:50051

CMD ["./api_rest_bf"]
