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

// ======================================
// HANDLER 1: Receptionist marks bill as paid
// ======================================
type MarkBillPaidInput struct {
	PaymentMethod string `json:"payment_method"` // 'cash', 'card', 'upi', 'netbanking', 'wallet'
	Amount        int    `json:"amount"`         // Amount paid (in paise)
	TransactionID string `json:"transaction_id"` // Optional: for online payments
	Notes         string `json:"notes"`          // Optional: payment notes
}

func MarkBillAsPaid(c *gin.Context) {
	billID := c.Param("id")

	var input MarkBillPaidInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get current user ID (should be receptionist)
	userID, ok := getContextUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Verify payment method exists
	var paymentMethodID int
	err := config.DB.QueryRow(context.Background(),
		`SELECT payment_method_id FROM payment_methods 
		 WHERE method_name=$1 AND is_active=true`,
		input.PaymentMethod,
	).Scan(&paymentMethodID)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment method"})
		return
	}

	// Get bill details
	var patientID, billAmount, currentStatus string
	err = config.DB.QueryRow(context.Background(),
		`SELECT patient_id, amount, status FROM billing WHERE bill_id=$1`,
		billID,
	).Scan(&patientID, &billAmount, &currentStatus)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Check if amount matches
	parsedBillAmount := 0
	fmt.Sscanf(billAmount, "%d", &parsedBillAmount)
	if input.Amount < parsedBillAmount {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Amount mismatch. Expected ₹%d, got ₹%d", parsedBillAmount, input.Amount)})
		return
	}

	// Start transaction
	tx, err := config.DB.Begin(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction error"})
		return
	}
	defer tx.Rollback(context.Background())

	// Create payment record
	var paymentID int
	err = tx.QueryRow(context.Background(),
		`INSERT INTO payments 
		 (bill_id, patient_id, amount, payment_method_id, status, transaction_id, paid_at)
		 VALUES ($1, $2, $3, $4, 'completed', $5, CURRENT_TIMESTAMP)
		 RETURNING payment_id`,
		billID, patientID, input.Amount, paymentMethodID, input.TransactionID,
	).Scan(&paymentID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record payment"})
		return
	}

	// Update bill status to PAID
	_, err = tx.Exec(context.Background(),
		`UPDATE billing 
		 SET status='paid', updated_at=CURRENT_TIMESTAMP, paid_date=CURRENT_TIMESTAMP
		 WHERE bill_id=$1`,
		billID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
		return
	}

	// Create audit log
	_, _ = tx.Exec(context.Background(),
		`INSERT INTO billing_audit_log 
		 (bill_id, action, old_status, new_status, changed_by, change_reason)
		 VALUES ($1, 'paid', $2, 'paid', $3, $4)`,
		billID, currentStatus, userID, "Payment received by "+input.PaymentMethod,
	)

	// Commit transaction
	err = tx.Commit(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Bill marked as paid",
		"bill_id":     billID,
		"payment_id":  paymentID,
		"amount_paid": input.Amount,
	})
}

// ======================================
// HANDLER 2: Get all bills (Admin)
// ======================================
func GetAllBills(c *gin.Context) {
	// Filter parameters
	status := c.Query("status") // pending, paid, cancelled
	patientID := c.Query("patient_id")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")

	query := `SELECT b.bill_id, b.patient_id, b.appointment_id, b.amount, b.status,
	                 b.description, b.created_at,
	                 COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, ''))) as patient_name
	          FROM billing b
	          JOIN patients p ON b.patient_id = p.patient_id
	          LEFT JOIN users u ON u.user_id = p.user_id
	          WHERE 1=1`

	args := []interface{}{}
	argIndex := 1

	if status != "" {
		query += fmt.Sprintf(` AND b.status=$%d`, argIndex)
		args = append(args, status)
		argIndex++
	}

	if patientID != "" {
		query += fmt.Sprintf(` AND b.patient_id=$%d`, argIndex)
		args = append(args, patientID)
		argIndex++
	}

	if fromDate != "" {
		query += fmt.Sprintf(` AND b.created_at::date >= $%d`, argIndex)
		args = append(args, fromDate)
		argIndex++
	}

	if toDate != "" {
		query += fmt.Sprintf(` AND b.created_at::date <= $%d`, argIndex)
		args = append(args, toDate)
		argIndex++
	}

	query += ` ORDER BY b.bill_id DESC`

	rows, err := config.DB.Query(context.Background(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bills"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	totalAmount := 0
	paidAmount := 0

	for rows.Next() {
		var billID, patientID, appointmentID, amount int
		var status, description, patientName string
		var createdAt time.Time

		if err := rows.Scan(&billID, &patientID, &appointmentID, &amount, &status, &description, &createdAt, &patientName); err != nil {
			continue
		}

		totalAmount += amount
		if status == "paid" {
			paidAmount += amount
		}

		// Parse breakdown
		breakdown := gin.H{
			"consultation": 0,
			"medicines":    0,
			"lab_tests":    0,
		}

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
			"patient_id":     patientID,
			"patient_name":   patientName,
			"appointment_id": appointmentID,
			"amount":         amount,
			"status":         status,
			"breakdown":      breakdown,
			"created_at":     createdAt,
		})
	}

	pendingAmount := totalAmount - paidAmount

	c.JSON(http.StatusOK, gin.H{
		"bills": list,
		"summary": gin.H{
			"total_bills":    len(list),
			"total_amount":   totalAmount,
			"paid_amount":    paidAmount,
			"pending_amount": pendingAmount,
		},
	})
}

// ======================================
// HANDLER 3: Get billing with itemized details
// ======================================
func GetBillWithItems(c *gin.Context) {
	billID := c.Param("id")

	// Get bill header
	var billIDInt, patientID, amount int
	var status string
	var createdAt, paidDate *time.Time
	err := config.DB.QueryRow(context.Background(),
		`SELECT bill_id, patient_id, amount, status, created_at, paid_date
		 FROM billing WHERE bill_id=$1`,
		billID,
	).Scan(&billIDInt, &patientID, &amount, &status, &createdAt, &paidDate)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Get itemized line items
	itemRows, err := config.DB.Query(context.Background(),
		`SELECT item_id, item_type, description, quantity, unit_price, total_amount
		 FROM billing_items WHERE bill_id=$1 ORDER BY item_type`,
		billID,
	)

	items := make([]gin.H, 0)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var itemID, quantity, unitPrice, totalAmount int
			var itemType, description string

			if err := itemRows.Scan(&itemID, &itemType, &description, &quantity, &unitPrice, &totalAmount); err != nil {
				continue
			}

			items = append(items, gin.H{
				"item_id":      itemID,
				"type":         itemType,
				"description":  description,
				"quantity":     quantity,
				"unit_price":   unitPrice,
				"total_amount": totalAmount,
			})
		}
	}

	// Get payments if any
	paymentRows, err := config.DB.Query(context.Background(),
		`SELECT payment_id, amount, pm.method_name, p.paid_at, p.status
		 FROM payments p
		 LEFT JOIN payment_methods pm ON p.payment_method_id = pm.payment_method_id
		 WHERE p.bill_id=$1 ORDER BY p.paid_at DESC`,
		billID,
	)

	payments := make([]gin.H, 0)
	if err == nil {
		defer paymentRows.Close()
		for paymentRows.Next() {
			var paymentID, paymentAmount int
			var paymentMethod, paymentStatus string
			var paidTime *time.Time

			if err := paymentRows.Scan(&paymentID, &paymentAmount, &paymentMethod, &paidTime, &paymentStatus); err != nil {
				continue
			}

			payments = append(payments, gin.H{
				"payment_id": paymentID,
				"amount":     paymentAmount,
				"method":     paymentMethod,
				"status":     paymentStatus,
				"paid_at":    paidTime,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"bill_id":    billIDInt,
		"patient_id": patientID,
		"amount":     amount,
		"status":     status,
		"created_at": createdAt,
		"paid_date":  paidDate,
		"items":      items,
		"payments":   payments,
	})
}

// ======================================
// SIMPLIFIED HANDLERS FOR MAIN WORKFLOW
// ======================================

// CreateBillByDoctor - Doctor generates bill for a patient
func CreateBillByDoctor(c *gin.Context) {
	_, ok := getContextUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

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
		`INSERT INTO billing (patient_id, appointment_id, amount, status, description)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING bill_id`,
		input.PatientID,
		input.AppointmentID,
		input.Amount,
		input.Status,
		input.Description,
	).Scan(&billID)

	if err != nil {
		fmt.Println("Bill creation error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Bill created successfully",
		"bill_id": billID,
		"amount":  input.Amount,
		"status":  input.Status,
	})
}

// GetAllBillsSimplified - For billing dashboard to see all bills
func GetAllBillsSimplified(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT b.bill_id, b.patient_id, b.appointment_id, b.amount, b.status, 
		        b.description, b.created_at, 
		        u.name as patient_name
		 FROM billing b
		 JOIN patients p ON b.patient_id = p.patient_id
		 JOIN users u ON u.user_id = p.user_id
		 ORDER BY b.bill_id DESC`,
	)
	if err != nil {
		fmt.Println("Query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bills"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	totalAmount := 0
	paidAmount := 0

	for rows.Next() {
		var billID, patientID, appointmentID, amount int
		var status, description, patientName string
		var createdAt time.Time

		if err := rows.Scan(&billID, &patientID, &appointmentID, &amount, &status, &description, &createdAt, &patientName); err != nil {
			continue
		}

		totalAmount += amount
		if status == "paid" {
			paidAmount += amount
		}

		list = append(list, gin.H{
			"bill_id":        billID,
			"patient_id":     patientID,
			"patient_name":   patientName,
			"appointment_id": appointmentID,
			"amount":         amount,
			"status":         status,
			"description":    description,
			"created_at":     createdAt,
		})
	}

	pendingAmount := totalAmount - paidAmount

	c.JSON(http.StatusOK, gin.H{
		"bills": list,
		"summary": gin.H{
			"total_bills":    len(list),
			"total_amount":   totalAmount,
			"paid_amount":    paidAmount,
			"pending_amount": pendingAmount,
		},
	})
}

// UpdateBillStatusToPaid - Simplified version to mark bill as paid
func UpdateBillStatusToPaid(c *gin.Context) {
	billID := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		`UPDATE billing SET status='paid' WHERE bill_id=$1`,
		billID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Bill marked as paid",
		"bill_id": billID,
	})
}
