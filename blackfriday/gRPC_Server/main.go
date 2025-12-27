package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"

	pb "blackfriday/proto"
)

type server struct {
	pb.UnimplementedProductSaleServiceServer
	kw *kafka.Writer
}

type SaleEvent struct {
	Categoria       string  `json:"categoria"`
	ProductoId      string  `json:"productoId"`
	Precio          float64 `json:"precio"`
	CantidadVendida int32   `json:"cantidadVendida"`
	TimestampUnixMs int64   `json:"timestampUnixMs"`
}

func (s *server) ProcesarVenta(ctx context.Context, req *pb.ProductSaleRequest) (*pb.ProductSaleResponse, error) {
	log.Printf("gRPC: venta recibida categoria=%s producto_id=%s precio=%.2f cantidad=%d",
		req.Categoria.String(), req.ProductoId, req.Precio, req.CantidadVendida)

	ev := SaleEvent{
		Categoria:       req.Categoria.String(),
		ProductoId:      req.ProductoId,
		Precio:          req.Precio,
		CantidadVendida: req.CantidadVendida,
		TimestampUnixMs: time.Now().UnixMilli(),
	}

	b, err := json.Marshal(ev)
	if err != nil {
		log.Printf("Error serializando evento: %v", err)
		return &pb.ProductSaleResponse{Estado: "ERROR_SERIALIZE"}, nil
	}

	// Produce a Kafka
	if err := s.kw.WriteMessages(ctx, kafka.Message{
		Key:   []byte(req.ProductoId), // clave de partición
		Value: b,
	}); err != nil {
		log.Printf("Kafka write error: %v", err)
		return &pb.ProductSaleResponse{Estado: "ERROR_KAFKA"}, nil
	}

	return &pb.ProductSaleResponse{Estado: "OK"}, nil
}

func main() {
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	brokers := os.Getenv("KAFKA_BROKERS") // ej: "kafka:9092" o "my-cluster-kafka-bootstrap:9092"
	if brokers == "" {
		brokers = "kafka:9092"
	}
	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "ventas"
	}

	kw := &kafka.Writer{
		Addr:     kafka.TCP(strings.Split(brokers, ",")...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer kw.Close()

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("No pude escuchar :%s: %v", grpcPort, err)
	}

	grpcSrv := grpc.NewServer()
	pb.RegisterProductSaleServiceServer(grpcSrv, &server{kw: kw})

	log.Printf("gRPC Server escuchando :%s | Kafka brokers=%s topic=%s", grpcPort, brokers, topic)
	if err := grpcSrv.Serve(lis); err != nil {
		log.Fatalf("gRPC Serve error: %v", err)
	}
}
