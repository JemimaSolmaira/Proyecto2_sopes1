use axum::{
    extract::State,
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::{net::SocketAddr, sync::Arc};
use std::sync::atomic::{AtomicU64, Ordering};
use tracing::{info, warn};
use uuid::Uuid;

#[derive(Clone)]
struct AppState {
    total_ventas: Arc<AtomicU64>,
    go_client_url: String,
    http: Client,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
struct ProductSaleRequest {
    categoria: CategoriaProducto,
    producto_id: String,
    precio: f64,
    cantidad_vendida: i32,
}

#[derive(Debug, Deserialize, Serialize)]
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

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt().with_env_filter("info").init();

    // URL del Go client (REST)
    let go_client_url =
        std::env::var("GO_CLIENT_URL").unwrap_or_else(|_| "http://localhost:8081".to_string());

    let state = AppState {
        total_ventas: Arc::new(AtomicU64::new(0)),
        go_client_url,
        http: Client::new(),
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

    // Llamar al Go Client (HTTP)
    let url = format!("{}/ventas", state.go_client_url.trim_end_matches('/'));

    let resp = state
        .http
        .post(url)
        .json(&req)
        .timeout(std::time::Duration::from_secs(2))
        .send()
        .await;

    let estado = match resp {
        Ok(r) if r.status().is_success() => {
            let v: serde_json::Value =
                r.json().await.unwrap_or_else(|_| serde_json::json!({"estado":"OK"}));
            v.get("estado")
                .and_then(|x| x.as_str())
                .unwrap_or("OK")
                .to_string()
        }
        Ok(r) => {
            warn!("Go client respondió HTTP {}", r.status());
            return (StatusCode::BAD_GATEWAY, "Error llamando Go Client").into_response();
        }
        Err(e) => {
            warn!("Error llamando Go Client: {}", e);
            return (StatusCode::BAD_GATEWAY, "Error llamando Go Client").into_response();
        }
    };

    info!(
        "Venta recibida id={} categoria={:?} producto_id={} precio={} qty={} total={} -> GoClient estado={}",
        request_id, req.categoria, req.producto_id, req.precio, req.cantidad_vendida, total, estado
    );

    let resp = ProductSaleResponse {
        estado,
        request_id,
        total_ventas_recibidas: total,
    };

    (StatusCode::OK, Json(resp)).into_response()
}
