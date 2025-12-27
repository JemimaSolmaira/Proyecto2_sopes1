# ===== build =====
FROM rust:1.92 AS builder
WORKDIR /app

# Copiar manifiestos primero (mejor cache)
COPY Cargo.toml Cargo.lock ./

# Copiar código
COPY src ./src

# Compilar
RUN cargo build --release

# ===== runtime =====
FROM debian:bookworm-slim
WORKDIR /app

RUN apt-get update \
    && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/api_rest_bf /app/api_rest_bf

EXPOSE 8080

ENV GO_CLIENT_URL=http://grpc-client-svc:8081

CMD ["/app/api_rest_bf"]

