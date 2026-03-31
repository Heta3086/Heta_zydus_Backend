package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type LabTest struct {
	TestID         int       `json:"test_id"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	Price          float64   `json:"price"`
	Description    string    `json:"description"`
	TurnaroundTime string    `json:"turnaround_time"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateLabTestInput struct {
	Name           string  `json:"name" binding:"required"`
	Category       string  `json:"category"`
	Price          float64 `json:"price" binding:"required"`
	Description    string  `json:"description"`
	TurnaroundTime string  `json:"turnaround_time"`
}

// GetLabTestsCatalog fetches all lab tests with their prices
func GetLabTestsCatalog(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT test_id, name, category, price, description, turnaround_time, is_active, created_at
		 FROM lab_tests_catalog
		 WHERE is_active = true
		 ORDER BY category ASC, name ASC`,
	)
	if err != nil {
		fmt.Println("[ERROR] GetLabTestsCatalog query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch lab tests"})
		return
	}
	defer rows.Close()

	tests := make([]LabTest, 0)
	for rows.Next() {
		var test LabTest
		if err := rows.Scan(&test.TestID, &test.Name, &test.Category, &test.Price, &test.Description, &test.TurnaroundTime, &test.IsActive, &test.CreatedAt); err != nil {
			fmt.Println("[ERROR] Scan failed:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse lab tests"})
			return
		}
		tests = append(tests, test)
	}

	if err := rows.Err(); err != nil {
		fmt.Println("[ERROR] Row iteration failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate lab tests"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tests": tests})
}

// GetLabTestCatalogByID fetches a single lab test from the catalog by ID
func GetLabTestCatalogByID(c *gin.Context) {
	id := c.Param("id")

	var test LabTest
	err := config.DB.QueryRow(context.Background(),
		`SELECT test_id, name, category, price, description, turnaround_time, is_active, created_at
		 FROM lab_tests_catalog
		 WHERE test_id=$1`,
		id,
	).Scan(&test.TestID, &test.Name, &test.Category, &test.Price, &test.Description, &test.TurnaroundTime, &test.IsActive, &test.CreatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lab test not found"})
			return
		}
		fmt.Println("[ERROR] GetLabTestByID query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch lab test"})
		return
	}

	c.JSON(http.StatusOK, test)
}

// CreateLabTestCatalog adds a new lab test to the catalog
func CreateLabTestCatalog(c *gin.Context) {
	var input CreateLabTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be greater than 0"})
		return
	}

	var testID int
	err := config.DB.QueryRow(context.Background(),
		`INSERT INTO lab_tests_catalog (name, category, price, description, turnaround_time)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING test_id`,
		input.Name,
		input.Category,
		input.Price,
		input.Description,
		input.TurnaroundTime,
	).Scan(&testID)

	if err != nil {
		fmt.Println("[ERROR] CreateLabTestCatalog insert failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create lab test"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Lab test created",
		"test_id": testID,
	})
}

// UpdateLabTestCatalog updates an existing lab test
func UpdateLabTestCatalog(c *gin.Context) {
	id := c.Param("id")

	var input CreateLabTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be greater than 0"})
		return
	}

	cmd, err := config.DB.Exec(context.Background(),
		`UPDATE lab_tests_catalog
		 SET name=$1, category=$2, price=$3, description=$4, turnaround_time=$5, updated_at=CURRENT_TIMESTAMP
		 WHERE test_id=$6`,
		input.Name,
		input.Category,
		input.Price,
		input.Description,
		input.TurnaroundTime,
		id,
	)

	if err != nil {
		fmt.Println("[ERROR] UpdateLabTestCatalog update failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update lab test"})
		return
	}

	if cmd.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Lab test not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lab test updated"})
}

// DeleteLabTestCatalog deactivates a lab test (soft delete)
func DeleteLabTestCatalog(c *gin.Context) {
	id := c.Param("id")

	cmd, err := config.DB.Exec(context.Background(),
		`UPDATE lab_tests_catalog
		 SET is_active=false, updated_at=CURRENT_TIMESTAMP
		 WHERE test_id=$1`,
		id,
	)

	if err != nil {
		fmt.Println("[ERROR] DeleteLabTestCatalog failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete lab test"})
		return
	}

	if cmd.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Lab test not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lab test deleted"})
}

// GetLabTestsByCategory fetches tests by category
func GetLabTestsByCategory(c *gin.Context) {
	category := c.Param("category")

	rows, err := config.DB.Query(context.Background(),
		`SELECT test_id, name, category, price, description, turnaround_time, is_active, created_at
		 FROM lab_tests_catalog
		 WHERE category=$1 AND is_active=true
		 ORDER BY name ASC`,
		category,
	)
	if err != nil {
		fmt.Println("[ERROR] GetLabTestsByCategory query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch lab tests"})
		return
	}
	defer rows.Close()

	tests := make([]LabTest, 0)
	for rows.Next() {
		var test LabTest
		if err := rows.Scan(&test.TestID, &test.Name, &test.Category, &test.Price, &test.Description, &test.TurnaroundTime, &test.IsActive, &test.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse lab tests"})
			return
		}
		tests = append(tests, test)
	}

	c.JSON(http.StatusOK, gin.H{"tests": tests})
}
