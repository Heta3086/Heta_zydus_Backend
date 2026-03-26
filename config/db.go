package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var DB *pgx.Conn

func ConnectDB() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("❌ Error loading .env file")
	}

	connStr := os.Getenv("DB_URL")

	DB, err = pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Database connection failed:", err)
	}

	fmt.Println("✅ Database connected")
}
