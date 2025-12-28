package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type Venta struct {
	Categoria       string  `json:"categoria"`
	Precio          float64 `json:"precio"`
	CantidadVendida int     `json:"cantidadVendida"`
	ProductoID      string  `json:"productoId"`
}

func main() {
	ctx := context.Background()

	brokers := getenv("KAFKA_BROKERS", "kafka:9092")
	topic := getenv("KAFKA_TOPIC", "ventas")
	group := getenv("KAFKA_GROUP", "ventas-consumer")
	valkeyAddr := getenv("VALKEY_ADDR", "valkey-primary:6379")

	rdb := redis.NewClient(&redis.Options{Addr: valkeyAddr})
	defer rdb.Close()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		Topic:   topic,
		GroupID: group,
	})
	defer reader.Close()

	log.Printf("Consumer listo | brokers=%s | topic=%s group=%s | valkey=%s", brokers, topic, group, valkeyAddr)

	// === Keys requeridas para el dashboard ===
	sumKey := "venta:stats:sumPrecio"              // HASH: categoria -> suma(precio)
	cntKey := "venta:stats:count"                  // HASH: categoria -> conteo
	avgKey := "venta:stats:categorias"             // HASH: categoria -> avg(precio)
	repKey := "venta:stats:reportes_por_categoria" // HASH: categoria -> total reportes

	maxKey := "venta:stats:precio_max" // STRING
	minKey := "venta:stats:precio_min" // STRING

	prodZKey := "venta:stats:productos_vendidos"      // ZSET: score=cantidad, member=productoId
	bestProdKey := "venta:stats:producto_mas_vendido" // STRING: "ID (score)"
	worstProdKey := "venta:stats:producto_menos_vendido"

	for {
		m, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Kafka read error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		keyEvento := "venta:" + topic + ":" + itoa(m.Partition) + ":" + itoa64(m.Offset)
		val := string(m.Value)

		// 1) Guardar evento crudo
		if err := rdb.Set(ctx, keyEvento, val, 0).Err(); err != nil {
			log.Printf("Valkey set error: %v", err)
			continue
		}

		// 2) Parsear JSON
		var v Venta
		if err := json.Unmarshal(m.Value, &v); err != nil {
			log.Printf("JSON inválido, no agrego stats. offset=%d err=%v value=%s", m.Offset, err, val)
			continue
		}
		if v.Categoria == "" {
			v.Categoria = "Desconocida"
		}
		if v.ProductoID == "" {
			v.ProductoID = "UNKNOWN"
		}

		// 3) Total de reportes por categoría (para gráfica "Total de Reportes por Categoría")
		if err := rdb.HIncrBy(ctx, repKey, v.Categoria, 1).Err(); err != nil {
			log.Printf("Valkey HINCRBY reportes error: %v", err)
			continue
		}

		// 4) Promedio de precio por categoría (para gráfica "Precio/Producto Promedio por Categoría")
		newSum, err := rdb.HIncrByFloat(ctx, sumKey, v.Categoria, v.Precio).Result()
		if err != nil {
			log.Printf("Valkey HINCRBYFLOAT sum error: %v", err)
			continue
		}
		newCnt, err := rdb.HIncrBy(ctx, cntKey, v.Categoria, 1).Result()
		if err != nil {
			log.Printf("Valkey HINCRBY cnt error: %v", err)
			continue
		}
		avg := newSum / float64(newCnt)
		if err := rdb.HSet(ctx, avgKey, v.Categoria, avg).Err(); err != nil {
			log.Printf("Valkey HSET avg error: %v", err)
			continue
		}

		// 5) Precio máximo y mínimo global (KPIs)
		if err := updateMaxMin(ctx, rdb, maxKey, minKey, v.Precio); err != nil {
			log.Printf("Valkey update max/min error: %v", err)
			continue
		}

		// 6) Producto más/menos vendido global (KPIs)
		if v.CantidadVendida > 0 {
			if _, err := rdb.ZIncrBy(ctx, prodZKey, float64(v.CantidadVendida), v.ProductoID).Result(); err != nil {
				log.Printf("Valkey ZINCRBY error: %v", err)
				continue
			}
			if err := updateBestWorstProduct(ctx, rdb, prodZKey, bestProdKey, worstProdKey); err != nil {
				log.Printf("Valkey update best/worst error: %v", err)
				continue
			}
		}

		log.Printf("OK | cat=%s precio=%.2f cnt=%d avg=%.2f prod=%s cant=%d",
			v.Categoria, v.Precio, newCnt, avg, v.ProductoID, v.CantidadVendida)
	}
}

func updateMaxMin(ctx context.Context, rdb *redis.Client, maxKey, minKey string, precio float64) error {
	// MAX
	curMaxStr, err := rdb.Get(ctx, maxKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	curMax := -math.MaxFloat64
	if err != redis.Nil {
		if f, e := strconv.ParseFloat(curMaxStr, 64); e == nil {
			curMax = f
		}
	}
	if precio > curMax {
		if err := rdb.Set(ctx, maxKey, fmt.Sprintf("%.2f", precio), 0).Err(); err != nil {
			return err
		}
	}

	// MIN
	curMinStr, err := rdb.Get(ctx, minKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	curMin := math.MaxFloat64
	if err != redis.Nil {
		if f, e := strconv.ParseFloat(curMinStr, 64); e == nil {
			curMin = f
		}
	}
	if precio < curMin {
		if err := rdb.Set(ctx, minKey, fmt.Sprintf("%.2f", precio), 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

func updateBestWorstProduct(ctx context.Context, rdb *redis.Client, zkey, bestKey, worstKey string) error {
	// más vendido
	top, err := rdb.ZRevRangeWithScores(ctx, zkey, 0, 0).Result()
	if err != nil {
		return err
	}
	if len(top) == 1 {
		best := fmt.Sprintf("%v (%.0f)", top[0].Member, top[0].Score)
		if err := rdb.Set(ctx, bestKey, best, 0).Err(); err != nil {
			return err
		}
	}

	// menos vendido
	low, err := rdb.ZRangeWithScores(ctx, zkey, 0, 0).Result()
	if err != nil {
		return err
	}
	if len(low) == 1 {
		worst := fmt.Sprintf("%v (%.0f)", low[0].Member, low[0].Score)
		if err := rdb.Set(ctx, worstKey, worst, 0).Err(); err != nil {
			return err
		}
	}

	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func itoa(v int) string     { return strconv.Itoa(v) }
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
