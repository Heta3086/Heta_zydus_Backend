package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const loginPayloadKey = "heta-zydus-secret-key-32-bytes!!"

func decryptLoginPassword(passwordPayload string) (string, error) {
	if !strings.HasPrefix(passwordPayload, "enc:") {
		return passwordPayload, nil
	}

	encoded := strings.TrimPrefix(passwordPayload, "enc:")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	if len(raw) < 12 {
		return "", fmt.Errorf("invalid encrypted payload")
	}

	iv := raw[:12]
	ciphertext := raw[12:]

	block, err := aes.NewCipher([]byte(loginPayloadKey))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Input structure
type RegisterInput struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Gender    string `json:"gender"`
	BloodType string `json:"blood_type"`
	Address   string `json:"address"`
	Password  string `json:"password"`
	Role      string `json:"role"`
}

func splitRegisterName(fullName string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func normalizeRegisterRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	if value == "receptioniest" || value == "recertioniest" || value == "receiptionist" {
		return "receptionist"
	}
	if value == "pharmacist" {
		return "pharmacy"
	}
	return value
}

func isAllowedRegisterRole(role string) bool {
	allowed := map[string]bool{
		"admin":        true,
		"doctor":       true,
		"patient":      true,
		"receptionist": true,
		"pharmacy":     true,
	}
	return allowed[role]
}

func createUserWithRole(name, email, password, role string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}

	_, err = config.DB.Exec(context.Background(),
		"INSERT INTO users (name, email, password_hash, role) VALUES ($1,$2,$3,$4)",
		name,
		email,
		string(hashedPassword),
		role,
	)

	return err
}

// Register API
func Register(c *gin.Context) {
	var input RegisterInput

	// Bind JSON
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = normalizePatientPhone(input.Phone)
	input.Address = strings.TrimSpace(input.Address)
	input.Password = strings.TrimSpace(input.Password)
	input.Role = normalizeRegisterRole(input.Role)
	gender, genderValid := normalizePatientGender(input.Gender)
	bloodType, bloodTypeValid := normalizePatientBloodType(input.BloodType)

	if input.Name == "" || input.Email == "" || input.Phone == "" || input.Address == "" || input.Password == "" || input.Role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, email, phone, gender, blood_type, address, password and role are required"})
		return
	}

	if !isValidPatientPhone(input.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone must be exactly 10 digits"})
		return
	}

	if !genderValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gender must be Male, Female, or Other"})
		return
	}

	if !bloodTypeValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blood_type must be one of A+, A-, B+, B-, AB+, AB-, O+, O-"})
		return
	}

	if !isAllowedRegisterRole(input.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if input.Role != "patient" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "register supports only patient role"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), 14)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password hashing failed"})
		return
	}

	ctx := context.Background()
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not start registration transaction"})
		return
	}
	defer tx.Rollback(ctx)

	var userID int
	err = tx.QueryRow(ctx,
		"INSERT INTO users (name, email, password_hash, role) VALUES ($1,$2,$3,$4) RETURNING user_id",
		input.Name,
		input.Email,
		string(hashedPassword),
		input.Role,
	).Scan(&userID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email already exists"})
				return
			}
			if pgErr.Code == "23514" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role value"})
				return
			}
		}
		fmt.Println("DB ERROR:", err) // 👈 for debugging
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	firstName, lastName := splitRegisterName(input.Name)
	_, err = tx.Exec(ctx,
		`INSERT INTO patients (user_id, first_name, last_name, date_of_birth, phone, gender, blood_type, address)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		userID,
		firstName,
		lastName,
		"2000-01-01", // Default date of birth
		input.Phone,
		gender,
		bloodType,
		input.Address,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Phone already exists"})
				return
			}
		}
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Patient profile creation failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration commit failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User registered successfully",
		"role":    "patient",
	})
}

func CreateReceptionistByAdmin(c *gin.Context) {
	var input RegisterInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Password = strings.TrimSpace(input.Password)

	if input.Name == "" || input.Email == "" || input.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, email and password are required"})
		return
	}

	err := createUserWithRole(input.Name, input.Email, input.Password, "receptionist")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email already exists"})
				return
			}
		}
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Receptionist creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Receptionist created successfully"})
}

func CreatePharmacistByAdmin(c *gin.Context) {
	var input RegisterInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Password = strings.TrimSpace(input.Password)

	if input.Name == "" || input.Email == "" || input.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, email and password are required"})
		return
	}

	err := createUserWithRole(input.Name, input.Email, input.Password, "pharmacy")
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Email already exists"})
				return
			}
			if pgErr.Code == "23514" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role value"})
				return
			}
		}
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Pharmacist creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pharmacist created successfully"})
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

	password, err := decryptLoginPassword(input.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid encrypted password payload"})
		return
	}

	// Get user from DB
	var storedPassword string
	var userID int
	var role string

	err = config.DB.QueryRow(context.Background(),
		"SELECT user_id, password_hash, role FROM users WHERE email=$1",
		input.Email,
	).Scan(&userID, &storedPassword, &role)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email"})
		return
	}

	// Compare password
	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(password))
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
		"role":    role,
	})
}

func Profile(c *gin.Context) {
	userID, exists1 := c.Get("user_id")
	role, exists2 := c.Get("role")

	if !exists1 || !exists2 {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	var name string
	err := config.DB.QueryRow(context.Background(),
		"SELECT name FROM users WHERE user_id = $1",
		userID,
	).Scan(&name)

	if err != nil {
		name = ""
	}

	c.JSON(200, gin.H{
		"user_id": userID,
		"role":    role,
		"name":    name,
	})
}

func AdminDashboard(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to Admin Dashboard",
	})
}

func DoctorDashboard(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to Doctor Dashboard",
	})
}

func PatientDashboard(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to Patient Dashboard",
	})
}

func Logout(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "Logout successful",
	})
}

func GetReceptionists(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		"SELECT user_id, name, email FROM users WHERE role = 'receptionist' ORDER BY name")
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch receptionists"})
		return
	}
	defer rows.Close()

	var receptionists []gin.H
	for rows.Next() {
		var id int
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			fmt.Println("SCAN ERROR:", err)
			continue
		}
		receptionists = append(receptionists, gin.H{
			"id":    id,
			"name":  name,
			"email": email,
		})
	}

	c.JSON(http.StatusOK, receptionists)
}

func GetPharmacists(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		"SELECT user_id, name, email FROM users WHERE role = 'pharmacy' ORDER BY name")
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pharmacists"})
		return
	}
	defer rows.Close()

	var pharmacists []gin.H
	for rows.Next() {
		var id int
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			fmt.Println("SCAN ERROR:", err)
			continue
		}
		pharmacists = append(pharmacists, gin.H{
			"id":    id,
			"name":  name,
			"email": email,
		})
	}

	c.JSON(http.StatusOK, pharmacists)
}

func DeleteReceptionist(c *gin.Context) {
	id := c.Param("id")

	// Delete the user
	result, err := config.DB.Exec(context.Background(),
		"DELETE FROM users WHERE user_id = $1 AND role = 'receptionist'",
		id)
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete receptionist"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Receptionist not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Receptionist deleted successfully"})
}

func DeletePharmacist(c *gin.Context) {
	id := c.Param("id")

	// Delete the user
	result, err := config.DB.Exec(context.Background(),
		"DELETE FROM users WHERE user_id = $1 AND role = 'pharmacy'",
		id)
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete pharmacist"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pharmacist not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pharmacist deleted successfully"})
}

func UpdateReceptionist(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Name == "" || input.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name and email are required"})
		return
	}

	result, err := config.DB.Exec(context.Background(),
		"UPDATE users SET name = $1, email = $2 WHERE user_id = $3 AND role = 'receptionist'",
		input.Name, input.Email, id)
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update receptionist"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Receptionist not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Receptionist updated successfully"})
}

func UpdatePharmacist(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Name == "" || input.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name and email are required"})
		return
	}

	result, err := config.DB.Exec(context.Background(),
		"UPDATE users SET name = $1, email = $2 WHERE user_id = $3 AND role = 'pharmacy'",
		input.Name, input.Email, id)
	if err != nil {
		fmt.Println("DB ERROR:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update pharmacist"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pharmacist not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pharmacist updated successfully"})
}
