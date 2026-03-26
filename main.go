package main

import (
	"fmt"
	"os"

	"hospital-backend/config"
	"hospital-backend/routes"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Connect Database
	config.ConnectDB()

	// Create Router
	r := gin.Default()
	r.Use(cors.Default())

	// Setup Routes
	routes.SetupRoutes(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("🚀 Server running on http://localhost:" + port)

	r.Run(":" + port)
}
