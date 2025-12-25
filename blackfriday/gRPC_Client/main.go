package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "blackfriday/proto"
)

type saleJSON struct {
	Categoria       string  `json:"categoria"`
	ProductoID      string  `json:"productoId"`
	Precio          float64 `json:"precio"`
	CantidadVendida int32   `json:"cantidadVendida"`
}

func mapCategoria(cat string) pb.CategoriaProducto {
	switch cat {
	case "Electronica":
		return pb.CategoriaProducto_Electronica
	case "Ropa":
		return pb.CategoriaProducto_Ropa
	case "Hogar":
		return pb.CategoriaProducto_Hogar
	case "Belleza":
		return pb.CategoriaProducto_Belleza
	default:
		return pb.CategoriaProducto_CATEGORIA_PRODUCTO_UNSPECIFIED
	}
}

func main() {
	grpcAddr := os.Getenv("GRPC_SERVER_ADDR")
	if grpcAddr == "" {
		grpcAddr = "localhost:50051"
	}

	// Conexión gRPC (cliente)
	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("No pude conectar a gRPC %s: %v", grpcAddr, err)
	}
	defer conn.Close()

	client := pb.NewProductSaleServiceClient(conn)

	// REST endpoint
	http.HandleFunc("/ventas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var s saleJSON
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("JSON inválido"))
			return
		}

		// Llamada gRPC
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := client.ProcesarVenta(ctx, &pb.ProductSaleRequest{
			Categoria:       mapCategoria(s.Categoria),
			ProductoId:      s.ProductoID,
			Precio:          s.Precio,
			CantidadVendida: s.CantidadVendida,
		})
		if err != nil {
			log.Printf("Error llamando gRPC: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("Error llamando gRPC"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"estado": resp.Estado,
		})
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("Go REST (cliente gRPC) escuchando en :8081, apuntando a gRPC:", grpcAddr)
	log.Fatal(http.ListenAndServe(":8081", nil))
}
