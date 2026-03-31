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

type LabTestInput struct {
	PatientID     int    `json:"patient_id"`
	AppointmentID int    `json:"appointment_id"`
	TestName      string `json:"test_name"`
	Result        string `json:"result"`
	Status        string `json:"status"`
}

func GetLabReports(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT lr.lab_report_id,
		        COALESCE(lr.patient_id, 0),
		        COALESCE(u.name, ''),
		        COALESCE(du.name, du2.name, ''),
		        COALESCE(lr.appointment_id, 0),
		        COALESCE(lr.test_name, ''),
		        COALESCE(lr.result, ''),
		        COALESCE(lr.status, ''),
		        COALESCE(lr.created_at::text, NOW()::text)
		 FROM lab_reports lr
		 LEFT JOIN patients p ON lr.patient_id = p.patient_id
		 LEFT JOIN users u ON p.user_id = u.user_id
		 LEFT JOIN appointments a ON a.appointment_id = lr.appointment_id
		 LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
		 LEFT JOIN users du ON du.user_id = d.user_id
		 LEFT JOIN LATERAL (
		 	SELECT a2.doctor_id
		 	FROM appointments a2
		 	WHERE a2.patient_id = lr.patient_id
		 	ORDER BY
		 		CASE
		 			WHEN lower(COALESCE(a2.status, '')) IN ('completed', 'confirmed', 'accepted', 'approved', 'in_progress', 'in progress', 'scheduled') THEN 0
		 			ELSE 1
		 		END,
		 		a2.appointment_date DESC,
		 		a2.appointment_time DESC,
		 		a2.appointment_id DESC
		 	LIMIT 1
		 ) latest_doctor ON TRUE
		 LEFT JOIN doctors d2 ON d2.doctor_id = latest_doctor.doctor_id
		 LEFT JOIN users du2 ON du2.user_id = d2.user_id
		 ORDER BY lr.lab_report_id DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch lab reports: %v", err)})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var labID, patientID, appointmentID int
		var patientName, doctorName, testName, result, status string
		var createdAt string

		if err := rows.Scan(&labID, &patientID, &patientName, &doctorName, &appointmentID, &testName, &result, &status, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse lab reports: %v", err)})
			return
		}

		list = append(list, gin.H{
			"lab_report_id":  labID,
			"patient_id":     patientID,
			"patient_name":   patientName,
			"doctor_name":    doctorName,
			"appointment_id": appointmentID,
			"test_name":      testName,
			"result":         result,
			"status":         status,
			"created_at":     createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate lab reports"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"lab_reports": list})
}

func CreateLabTest(c *gin.Context) {
	var input LabTestInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.PatientID <= 0 || input.TestName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id and test_name are required"})
		return
	}

	if input.Status == "" {
		input.Status = "ordered"
	}

	var labID int
	err := config.DB.QueryRow(context.Background(),
		`INSERT INTO lab_reports
		(patient_id, appointment_id, test_name, result, status)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING lab_report_id`,
		input.PatientID,
		input.AppointmentID,
		input.TestName,
		input.Result,
		input.Status,
	).Scan(&labID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lab test creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Lab test created",
		"lab_report_id": labID,
	})
}

func GetLabTestByID(c *gin.Context) {
	id := c.Param("id")

	var labID, patientID, appointmentID int
	var testName, result, status, doctorName string
	var createdAt time.Time

	err := config.DB.QueryRow(context.Background(),
		`SELECT lab_report_id,
		        COALESCE(patient_id, 0),
		        COALESCE(appointment_id, 0),
		        COALESCE(test_name, ''),
		        COALESCE(result, ''),
		        COALESCE(status, ''),
		        COALESCE(du.name, du2.name, ''),
		        created_at
		 FROM lab_reports lr
		 LEFT JOIN appointments a ON a.appointment_id = lr.appointment_id
		 LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
		 LEFT JOIN users du ON du.user_id = d.user_id
		 LEFT JOIN LATERAL (
		 	SELECT a2.doctor_id
		 	FROM appointments a2
		 	WHERE a2.patient_id = lr.patient_id
		 	ORDER BY
		 		CASE
		 			WHEN lower(COALESCE(a2.status, '')) IN ('completed', 'confirmed', 'accepted', 'approved', 'in_progress', 'in progress', 'scheduled') THEN 0
		 			ELSE 1
		 		END,
		 		a2.appointment_date DESC,
		 		a2.appointment_time DESC,
		 		a2.appointment_id DESC
		 	LIMIT 1
		 ) latest_doctor ON TRUE
		 LEFT JOIN doctors d2 ON d2.doctor_id = latest_doctor.doctor_id
		 LEFT JOIN users du2 ON du2.user_id = d2.user_id
		 WHERE lr.lab_report_id=$1`,
		id,
	).Scan(&labID, &patientID, &appointmentID, &testName, &result, &status, &doctorName, &createdAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lab report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch lab report: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"lab_report_id":  labID,
		"patient_id":     patientID,
		"appointment_id": appointmentID,
		"test_name":      testName,
		"result":         result,
		"status":         status,
		"doctor_name":    doctorName,
		"created_at":     createdAt,
	})
}

func UpdateLabTest(c *gin.Context) {
	id := c.Param("id")

	var input LabTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var currentStatus, currentResult string
	err := config.DB.QueryRow(context.Background(),
		`SELECT COALESCE(status, ''), COALESCE(result, '')
		 FROM lab_reports
		 WHERE lab_report_id = $1`,
		id,
	).Scan(&currentStatus, &currentResult)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Lab report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load lab report"})
		return
	}

	if strings.EqualFold(strings.TrimSpace(currentStatus), "completed") && strings.TrimSpace(currentResult) != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Completed lab report cannot be updated again"})
		return
	}

	if strings.EqualFold(strings.TrimSpace(input.Status), "completed") {
		incomingResult := strings.TrimSpace(input.Result)
		if incomingResult == "" {
			incomingResult = strings.TrimSpace(currentResult)
		}
		if incomingResult == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Result is required before marking report as completed"})
			return
		}
	}

	cmd, err := config.DB.Exec(context.Background(),
		`UPDATE lab_reports
		 SET test_name = COALESCE(NULLIF($1, ''), test_name),
		     result = COALESCE(NULLIF($2, ''), result),
		     status = COALESCE(NULLIF($3, ''), status)
		 WHERE lab_report_id = $4`,
		input.TestName,
		input.Result,
		input.Status,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lab report update failed"})
		return
	}

	if cmd.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Lab report not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lab report updated"})
}

func GetMyLabReports(c *gin.Context) {
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
		`SELECT lab_report_id,
		        COALESCE(appointment_id, 0),
		        COALESCE(test_name, ''),
		        COALESCE(result, ''),
		        COALESCE(status, ''),
		        COALESCE(created_at::text, NOW()::text)
		 FROM lab_reports
		 WHERE patient_id=$1
		 ORDER BY lab_report_id DESC`,
		patientID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch lab reports: %v", err)})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var labID, appointmentID int
		var testName, result, status string
		var createdAt string

		if err := rows.Scan(&labID, &appointmentID, &testName, &result, &status, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to parse lab reports: %v", err)})
			return
		}

		list = append(list, gin.H{
			"lab_report_id":  labID,
			"appointment_id": appointmentID,
			"test_name":      testName,
			"result":         result,
			"status":         status,
			"created_at":     createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate lab reports"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"lab_reports": list})
}
