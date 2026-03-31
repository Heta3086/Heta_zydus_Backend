package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type PharmacyItemInput struct {
	MedicineName string `json:"medicine_name"`
	Quantity     int    `json:"quantity"`
	UnitPrice    int    `json:"unit_price"`
}

func CreatePharmacyItem(c *gin.Context) {
	var input PharmacyItemInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.MedicineName = strings.TrimSpace(input.MedicineName)

	if input.MedicineName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "medicine_name is required"})
		return
	}

	if input.Quantity <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be greater than 0"})
		return
	}

	if input.UnitPrice <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unit_price must be greater than 0"})
		return
	}

	var itemID int
	err := config.DB.QueryRow(context.Background(),
		`INSERT INTO pharmacy_items (medicine_name, quantity, unit_price)
		 VALUES ($1,$2,$3)
		 RETURNING item_id`,
		input.MedicineName,
		input.Quantity,
		input.UnitPrice,
	).Scan(&itemID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Medicine creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Pharmacy item created",
		"item_id": itemID,
	})
}

func GetPharmacyItems(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT item_id,
		        COALESCE(medicine_name, ''),
		        COALESCE(quantity, 0),
		        COALESCE(unit_price, 0),
		        created_at
		 FROM pharmacy_items
		 ORDER BY item_id ASC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pharmacy items"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var itemID, quantity, unitPrice int
		var medicineName string
		var createdAt time.Time

		if err := rows.Scan(&itemID, &medicineName, &quantity, &unitPrice, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse pharmacy items"})
			return
		}

		list = append(list, gin.H{
			"item_id":       itemID,
			"medicine_name": medicineName,
			"quantity":      quantity,
			"unit_price":    unitPrice,
			"created_at":    createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate pharmacy items"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": list})
}

func GetPharmacyItemByID(c *gin.Context) {
	id := c.Param("id")

	var itemID, quantity, unitPrice int
	var medicineName string
	var createdAt time.Time

	err := config.DB.QueryRow(context.Background(),
		`SELECT item_id,
		        COALESCE(medicine_name, ''),
		        COALESCE(quantity, 0),
		        COALESCE(unit_price, 0),
		        created_at
		 FROM pharmacy_items
		 WHERE item_id=$1`,
		id,
	).Scan(&itemID, &medicineName, &quantity, &unitPrice, &createdAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Pharmacy item not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pharmacy item"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"item_id":       itemID,
		"medicine_name": medicineName,
		"quantity":      quantity,
		"unit_price":    unitPrice,
		"created_at":    createdAt,
	})
}

func UpdatePharmacyItem(c *gin.Context) {
	id := c.Param("id")

	var input PharmacyItemInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.MedicineName = strings.TrimSpace(input.MedicineName)
	if input.MedicineName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "medicine_name is required"})
		return
	}

	if input.Quantity <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity must be greater than 0"})
		return
	}

	if input.UnitPrice <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unit_price must be greater than 0"})
		return
	}

	cmd, err := config.DB.Exec(context.Background(),
		`UPDATE pharmacy_items
		 SET medicine_name=$1, quantity=$2, unit_price=$3
		 WHERE item_id=$4`,
		input.MedicineName,
		input.Quantity,
		input.UnitPrice,
		id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Pharmacy item update failed"})
		return
	}

	if cmd.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pharmacy item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pharmacy item updated"})
}

func DeletePharmacyItem(c *gin.Context) {
	id := c.Param("id")

	cmd, err := config.DB.Exec(context.Background(),
		"DELETE FROM pharmacy_items WHERE item_id=$1",
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Pharmacy item delete failed"})
		return
	}

	if cmd.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pharmacy item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pharmacy item deleted"})
}
