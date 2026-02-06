package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	outbound_processor "video-processor-worker/internal/adapters/outbound/processor"
	outbound_repository "video-processor-worker/internal/adapters/outbound/repository"
	outbound_storage "video-processor-worker/internal/adapters/outbound/storage"
	core_services "video-processor-worker/internal/core/services"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	fmt.Println("🚀 Video Processor Worker v1 starting...")

	// Verify if ffmpeg is installed
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		log.Fatal("❌ Erro: ffmpeg não encontrado no sistema. Por favor, instale o ffmpeg para continuar.")
	}

	// Database initialization
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	if dbHost == "" {
		dbHost = "db"
	}
	if dbPort == "" {
		dbPort = "5432"
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	var dbPool *pgxpool.Pool
	for i := 0; i < 10; i++ {
		dbPool, err = pgxpool.New(context.Background(), connStr)
		if err == nil {
			err = dbPool.Ping(context.Background())
			if err == nil {
				break
			}
		}
		log.Printf("⏳ Waiting for database... (%d/10)", i+1)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatal("❌ Erro ao conectar ao banco de dados: ", err)
	}
	defer dbPool.Close()

	// Initialize Outbound Adapters
	storage := outbound_storage.NewFSStorage()
	processor := outbound_processor.NewFFmpegProcessor()
	videoRepo := outbound_repository.NewPostgresVideoRepository(dbPool)

	// Initialize Worker Service
	worker := core_services.NewWorkerService(processor, storage, videoRepo)

	// Start Polling
	worker.Start()
}
