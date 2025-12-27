package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

func main() {
	ctx := context.Background()

	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "kafka:9092"
	}
	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "ventas"
	}
	group := os.Getenv("KAFKA_GROUP")
	if group == "" {
		group = "ventas-consumer"
	}

	valkeyAddr := os.Getenv("VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "valkey-0.valkey:6379"
	}

	rdb := redis.NewClient(&redis.Options{Addr: valkeyAddr})
	defer rdb.Close()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		Topic:   topic,
		GroupID: group,
	})
	defer reader.Close()

	log.Printf("Consumer listo | brokers=%s | topic=%s group=%s | valkey=%s", brokers, topic, group, valkeyAddr)

	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Kafka read error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		key := "venta:" + topic + ":" + itoa(m.Partition) + ":" + itoa64(m.Offset)

		// para confirmar en logs
		log.Printf("Consumido topic=%s partition=%d offset=%d key=%s value=%s", topic, m.Partition, m.Offset, string(m.Key), string(m.Value))

		if err := rdb.Set(ctx, key, string(m.Value), 0).Err(); err != nil {
			log.Printf("Valkey set error: %v", err)
			continue
		}
	}
}

func itoa(v int) string     { return strconv.Itoa(v) }
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
