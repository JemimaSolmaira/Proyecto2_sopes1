package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	pb "blackfriday/proto"
)

type server struct {
	pb.UnimplementedProductSaleServiceServer
}

func (s *server) ProcesarVenta(ctx context.Context, req *pb.ProductSaleRequest) (*pb.ProductSaleResponse, error) {
	log.Printf("gRPC: venta recibida categoria=%s producto_id=%s precio=%.2f cantidad=%d",
		req.Categoria.String(), req.ProductoId, req.Precio, req.CantidadVendida)

	return &pb.ProductSaleResponse{Estado: "OK"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("No pude escuchar :50051: %v", err)
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterProductSaleServiceServer(grpcSrv, &server{})

	log.Println("gRPC Server escuchando en :50051")
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("gRPC Serve error: %v", err)
	}
}
