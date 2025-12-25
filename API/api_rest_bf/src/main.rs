use axum::{
    extract::State,
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use std::{net::SocketAddr, sync::Arc};
use std::sync::atomic::{AtomicU64, Ordering};
use tokio::sync::Mutex;
use tracing::{info, warn};
use uuid::Uuid;

use tonic::transport::Channel;

pub mod blackfriday {
    tonic::include_proto!("blackfriday"); 
}

use blackfriday::{
    product_sale_service_client::ProductSaleServiceClient,
    CategoriaProducto as GrpcCategoria,
    ProductSaleRequest as GrpcSaleRequest,
};

#[derive(Clone)]
struct AppState {
    total_ventas: Arc<AtomicU64>,
    grpc_client: Arc<Mutex<ProductSaleServiceClient<Channel>>>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProductSaleRequest {
    categoria: CategoriaProducto,
    producto_id: String,
    precio: f64,
    cantidad_vendida: i32,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "PascalCase")]
enum CategoriaProducto {
    Electronica,
    Ropa,
    Hogar,
    Belleza,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ProductSaleResponse {
    estado: String,
    request_id: String,
    total_ventas_recibidas: u64,
}

fn map_categoria_to_grpc(cat: &CategoriaProducto) -> i32 {
    match cat {
        CategoriaProducto::Electronica => GrpcCategoria::Electronica as i32,
        CategoriaProducto::Ropa => GrpcCategoria::Ropa as i32,
        CategoriaProducto::Hogar => GrpcCategoria::Hogar as i32,
        CategoriaProducto::Belleza => GrpcCategoria::Belleza as i32,
    }
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt().with_env_filter("info").init();

    // Dirección del gRPC server (Go)
    let grpc_addr = std::env::var("GRPC_SERVER_ADDR").unwrap_or_else(|_| "http://localhost:50051".to_string());

    let channel = Channel::from_shared(grpc_addr)
        .expect("GRPC_SERVER_ADDR inválida (debe incluir http://)")
        .connect()
        .await
        .expect("No pude conectar al gRPC server");

    let grpc_client = ProductSaleServiceClient::new(channel);

    let state = AppState {
        total_ventas: Arc::new(AtomicU64::new(0)),
        grpc_client: Arc::new(Mutex::new(grpc_client)),
    };

    let app = Router::new()
        .route("/health", get(health))
        .route("/ventas", post(recibir_venta))
        .with_state(state);

    let addr: SocketAddr = "0.0.0.0:8080".parse().unwrap();
    info!("API REST escuchando en http://{addr}");
    axum::serve(tokio::net::TcpListener::bind(addr).await.unwrap(), app)
        .await
        .unwrap();
}

async fn health() -> &'static str {
    "ok"
}

async fn recibir_venta(
    State(state): State<AppState>,
    Json(req): Json<ProductSaleRequest>,
) -> impl IntoResponse {
    // Validaciones
    if req.producto_id.trim().is_empty() {
        warn!("producto_id vacío");
        return (StatusCode::BAD_REQUEST, "producto_id no puede ir vacío").into_response();
    }
    if req.precio.is_nan() || req.precio < 0.0 {
        warn!("precio inválido: {}", req.precio);
        return (StatusCode::BAD_REQUEST, "precio inválido").into_response();
    }
    if req.cantidad_vendida <= 0 {
        warn!("cantidad_vendida inválida: {}", req.cantidad_vendida);
        return (StatusCode::BAD_REQUEST, "cantidad_vendida debe ser > 0").into_response();
    }

    let total = state.total_ventas.fetch_add(1, Ordering::Relaxed) + 1;
    let request_id = Uuid::new_v4().to_string();

    // Construir request gRPC
    let grpc_req = GrpcSaleRequest {
        categoria: map_categoria_to_grpc(&req.categoria),
        producto_id: req.producto_id.clone(),
        precio: req.precio,
        cantidad_vendida: req.cantidad_vendida,
    };

    // Llamar gRPC
    let mut client = state.grpc_client.lock().await;
    let grpc_result = client.procesar_venta(tonic::Request::new(grpc_req)).await;

    let estado = match grpc_result {
        Ok(resp) => resp.into_inner().estado,
        Err(e) => {
            warn!("Error llamando gRPC: {}", e);
            // puedes decidir si devuelves 502 o 200 con estado error
            return (StatusCode::BAD_GATEWAY, "Error llamando gRPC").into_response();
        }
    };

    info!(
        "Venta recibida id={} categoria={:?} producto_id={} precio={} qty={} total={} -> gRPC estado={}",
        request_id, req.categoria, req.producto_id, req.precio, req.cantidad_vendida, total, estado
    );

    let resp = ProductSaleResponse {
        estado,
        request_id,
        total_ventas_recibidas: total,
    };

    (StatusCode::OK, Json(resp)).into_response()
}
