package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Input structure
type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// Register API
func Register(c *gin.Context) {
	var input RegisterInput

	// Bind JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), 14)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password hashing failed"})
		return
	}

	// Insert into DB
	_, err = config.DB.Exec(context.Background(),
		"INSERT INTO users (name, email, password_hash, role) VALUES ($1,$2,$3,$4)",
		input.Name,
		input.Email,
		string(hashedPassword),
		input.Role,
	)

	if err != nil {
		fmt.Println("DB ERROR:", err) // 👈 for debugging
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User registered successfully",
	})
}

func Login(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// Get input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from DB
	var storedPassword string
	var userID int
	var role string

	err := config.DB.QueryRow(context.Background(),
		"SELECT user_id, password_hash, role FROM users WHERE email=$1",
		input.Email,
	).Scan(&userID, &storedPassword, &role)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email"})
		return
	}

	// Compare password
	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(input.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	// Generate JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString([]byte("secret_key"))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	// ✅ FINAL RESPONSE
	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   tokenString,
	})
}

func Profile(c *gin.Context) {
	userID, exists1 := c.Get("user_id")
	role, exists2 := c.Get("role")

	if !exists1 || !exists2 {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	c.JSON(200, gin.H{
		"user_id": userID,
		"role":    role,
	})
}
