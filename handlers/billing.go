package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type BillingInput struct {
	PatientID     int    `json:"patient_id"`
	AppointmentID int    `json:"appointment_id"`
	Amount        int    `json:"amount"`
	Status        string `json:"status"`
	Description   string `json:"description"`
}

func CreateBill(c *gin.Context) {
	var input BillingInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.PatientID <= 0 || input.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id and amount are required"})
		return
	}

	if input.Status == "" {
		input.Status = "pending"
	}

	var billID int
	err := config.DB.QueryRow(context.Background(),
		`INSERT INTO billing
		(patient_id, appointment_id, amount, status, description)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING bill_id`,
		input.PatientID,
		input.AppointmentID,
		input.Amount,
		input.Status,
		input.Description,
	).Scan(&billID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bill creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Bill created",
		"bill_id": billID,
	})
}

func GetBillByID(c *gin.Context) {
	id := c.Param("id")

	var billID, patientID, appointmentID, amount int
	var status, description string
	var createdAt time.Time

	err := config.DB.QueryRow(context.Background(),
		`SELECT bill_id,
		        patient_id,
		        COALESCE(appointment_id, 0),
		        COALESCE(amount, 0),
		        COALESCE(status, ''),
		        COALESCE(description, ''),
		        created_at
		 FROM billing
		 WHERE bill_id=$1`,
		id,
	).Scan(&billID, &patientID, &appointmentID, &amount, &status, &description, &createdAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bill"})
		return
	}

	// Parse description to show breakdown
	breakdown := gin.H{
		"consultation": 0,
		"medicines":    0,
		"lab_tests":    0,
	}

	// Parse description for breakdown (format: "Consultation: ₹X | Medicines: ₹Y | Lab Tests: ₹Z")
	parts := strings.Split(description, " | ")
	for _, part := range parts {
		if strings.Contains(part, "Consultation:") {
			var val int
			fmt.Sscanf(part, "Consultation: ₹%d", &val)
			breakdown["consultation"] = val
		} else if strings.Contains(part, "Medicines:") {
			var val int
			fmt.Sscanf(part, "Medicines: ₹%d", &val)
			breakdown["medicines"] = val
		} else if strings.Contains(part, "Lab Tests:") {
			var val int
			fmt.Sscanf(part, "Lab Tests: ₹%d", &val)
			breakdown["lab_tests"] = val
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"bill_id":        billID,
		"patient_id":     patientID,
		"appointment_id": appointmentID,
		"amount":         amount,
		"status":         status,
		"description":    description,
		"breakdown":      breakdown,
		"created_at":     createdAt,
	})
}

func GetMyBills(c *gin.Context) {
	userID, ok := getContextUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var patientID int
	err := config.DB.QueryRow(context.Background(),
		"SELECT patient_id FROM patients WHERE user_id=$1",
		userID,
	).Scan(&patientID)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve patient"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT bill_id,
		        COALESCE(appointment_id, 0),
		        COALESCE(amount, 0),
		        COALESCE(status, 'pending'),
		        COALESCE(description, 'Medical charges'),
		        created_at
		 FROM billing
		 WHERE patient_id=$1
		 ORDER BY bill_id DESC`,
		patientID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bills"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var billID int
		var appointmentID int
		var amount int
		var status string
		var description string
		var createdAt time.Time

		if err := rows.Scan(&billID, &appointmentID, &amount, &status, &description, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse bills"})
			return
		}

		// Parse description to show breakdown
		breakdown := gin.H{
			"consultation": 0,
			"medicines":    0,
			"lab_tests":    0,
		}

		// Parse description for breakdown (format: "Consultation: ₹X | Medicines: ₹Y | Lab Tests: ₹Z")
		parts := strings.Split(description, " | ")
		for _, part := range parts {
			if strings.Contains(part, "Consultation:") {
				var val int
				fmt.Sscanf(part, "Consultation: ₹%d", &val)
				breakdown["consultation"] = val
			} else if strings.Contains(part, "Medicines:") {
				var val int
				fmt.Sscanf(part, "Medicines: ₹%d", &val)
				breakdown["medicines"] = val
			} else if strings.Contains(part, "Lab Tests:") {
				var val int
				fmt.Sscanf(part, "Lab Tests: ₹%d", &val)
				breakdown["lab_tests"] = val
			}
		}

		list = append(list, gin.H{
			"bill_id":        billID,
			"appointment_id": appointmentID,
			"amount":         amount,
			"status":         status,
			"description":    description,
			"breakdown":      breakdown,
			"created_at":     createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate bills"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bills": list})
}
