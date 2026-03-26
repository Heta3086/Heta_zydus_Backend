package routes

import (
	"hospital-backend/handlers"
	"hospital-backend/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Backend running 🚀",
		})
	})

	// Register API
	r.POST("/register", handlers.Register)
	// Login API
	r.POST("/login", handlers.Login)

	// Protected routes// Protected routes
	auth := r.Group("/")
	auth.Use(middleware.AuthMiddleware())
	{
		auth.GET("/profile", handlers.Profile)
	}
}
