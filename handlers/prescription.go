package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type PrescriptionInput struct {
	AppointmentID int    `json:"appointment_id"`
	PatientID     int    `json:"patient_id"`
	MedicineName  string `json:"medicine_name"`
	Dosage        string `json:"dosage"`
	Instructions  string `json:"instructions"`
}

func getContextUserID(c *gin.Context) (int, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func CreatePrescription(c *gin.Context) {
	var input PrescriptionInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.PatientID <= 0 || input.MedicineName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id and medicine_name are required"})
		return
	}

	userID, ok := getContextUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var doctorID int
	err := config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve doctor"})
		return
	}

	var prescriptionID int
	err = config.DB.QueryRow(context.Background(),
		`INSERT INTO prescriptions
		(appointment_id, patient_id, doctor_id, medicine_name, dosage, instructions)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING prescription_id`,
		input.AppointmentID,
		input.PatientID,
		doctorID,
		input.MedicineName,
		input.Dosage,
		input.Instructions,
	).Scan(&prescriptionID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prescription creation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Prescription created",
		"prescription_id": prescriptionID,
	})
}

func GetPrescriptionByID(c *gin.Context) {
	id := c.Param("id")

	var prescriptionID, appointmentID, patientID, doctorID int
	var medicineName, dosage, instructions string
	var createdAt time.Time

	err := config.DB.QueryRow(context.Background(),
		`SELECT prescription_id, appointment_id, patient_id, doctor_id,
		        COALESCE(medicine_name, ''), COALESCE(dosage, ''), COALESCE(instructions, ''), created_at
		 FROM prescriptions
		 WHERE prescription_id=$1`,
		id,
	).Scan(&prescriptionID, &appointmentID, &patientID, &doctorID, &medicineName, &dosage, &instructions, &createdAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"prescription_id": prescriptionID,
		"appointment_id":  appointmentID,
		"patient_id":      patientID,
		"doctor_id":       doctorID,
		"medicine_name":   medicineName,
		"dosage":          dosage,
		"instructions":    instructions,
		"created_at":      createdAt,
	})
}

func GetMyPrescriptions(c *gin.Context) {
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
		`SELECT p.prescription_id,
		        p.appointment_id,
		        COALESCE(m.name, ''),
		        COALESCE(pi.dosage, ''),
		        COALESCE(pi.frequency, ''),
		        COALESCE(pi.duration_days, 1),
		        COALESCE(pi.instructions, ''),
		        COALESCE(du.name, CONCAT(COALESCE(d.first_name, ''), ' ', COALESCE(d.last_name, ''))),
		        COALESCE(p.created_at, NOW())
		 FROM prescriptions p
		 LEFT JOIN prescription_items pi ON pi.prescription_id = p.prescription_id
		 LEFT JOIN medicines m ON m.medicine_id = pi.medicine_id
		 LEFT JOIN doctors d ON d.doctor_id = p.doctor_id
		 LEFT JOIN users du ON du.user_id = d.user_id
		 WHERE p.patient_id=$1
		 ORDER BY p.prescription_id DESC, pi.item_id ASC`,
		patientID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var prescriptionID, appointmentID, durationDays int
		var medicineName, dosage, frequency, instructions, doctorName string
		var createdAt time.Time

		if err := rows.Scan(&prescriptionID, &appointmentID, &medicineName, &dosage, &frequency, &durationDays, &instructions, &doctorName, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse prescriptions"})
			return
		}

		if dosage == "" {
			dosage = frequency
		}

		list = append(list, gin.H{
			"prescription_id": prescriptionID,
			"appointment_id":  appointmentID,
			"medicine_name":   medicineName,
			"dosage":          dosage,
			"duration":        strconv.Itoa(durationDays) + " Days",
			"instructions":    instructions,
			"doctor_name":     doctorName,
			"created_at":      createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate prescriptions"})
		return
	}

	// Backward-compatible fallback for historical data where prescriptions/prescription_items
	// were not populated but appointment prescription_json exists.
	if len(list) == 0 {
		appointmentRows, err := config.DB.Query(context.Background(),
			`SELECT a.appointment_id,
			        COALESCE(a.prescription_json, '[]'),
			        COALESCE(du.name, CONCAT(COALESCE(d.first_name, ''), ' ', COALESCE(d.last_name, ''))),
			        COALESCE(a.created_at, NOW())
			 FROM appointments a
			 LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
			 LEFT JOIN users du ON du.user_id = d.user_id
			 WHERE a.patient_id = $1
			   AND lower(COALESCE(a.status, '')) = 'completed'
			 ORDER BY a.appointment_id DESC`,
			patientID,
		)
		if err == nil {
			defer appointmentRows.Close()

			for appointmentRows.Next() {
				var appointmentID int
				var prescriptionJSON, doctorName string
				var createdAt time.Time

				if err := appointmentRows.Scan(&appointmentID, &prescriptionJSON, &doctorName, &createdAt); err != nil {
					continue
				}

				trimmed := strings.TrimSpace(prescriptionJSON)
				if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
					continue
				}

				var arr []map[string]any
				if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
					var single map[string]any
					if err := json.Unmarshal([]byte(trimmed), &single); err != nil || len(single) == 0 {
						continue
					}
					arr = []map[string]any{single}
				}

				for _, item := range arr {
					medicineName := strings.TrimSpace(fmt.Sprint(item["name"]))
					if medicineName == "" {
						medicineName = strings.TrimSpace(fmt.Sprint(item["medicine"]))
					}
					if medicineName == "" {
						medicineName = strings.TrimSpace(fmt.Sprint(item["medicine_name"]))
					}

					dosage := strings.TrimSpace(fmt.Sprint(item["dosage"]))
					if dosage == "" {
						dosage = strings.TrimSpace(fmt.Sprint(item["frequency"]))
					}

					duration := strings.TrimSpace(fmt.Sprint(item["duration"]))
					instructions := strings.TrimSpace(fmt.Sprint(item["instructions"]))

					if medicineName == "" && dosage == "" && duration == "" && instructions == "" {
						continue
					}

					if duration == "" {
						duration = "-"
					}

					list = append(list, gin.H{
						"prescription_id": 0,
						"appointment_id":  appointmentID,
						"medicine_name":   medicineName,
						"dosage":          dosage,
						"duration":        duration,
						"instructions":    instructions,
						"doctor_name":     doctorName,
						"created_at":      createdAt,
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"prescriptions": list})
}

func GetDoctorPrescriptions(c *gin.Context) {
	userID, ok := getContextUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var doctorID int
	err := config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Doctor profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve doctor"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT prescription_id,
		        patient_id,
		        COALESCE(medicine_name, ''),
		        COALESCE(dosage, ''),
		        COALESCE(instructions, ''),
		        created_at
		 FROM prescriptions
		 WHERE doctor_id=$1
		 ORDER BY prescription_id DESC`,
		doctorID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var prescriptionID, patientID int
		var medicineName, dosage, instructions string
		var createdAt time.Time

		if err := rows.Scan(&prescriptionID, &patientID, &medicineName, &dosage, &instructions, &createdAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse prescriptions"})
			return
		}

		list = append(list, gin.H{
			"prescription_id": prescriptionID,
			"patient_id":      patientID,
			"medicine_name":   medicineName,
			"dosage":          dosage,
			"instructions":    instructions,
			"created_at":      createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate prescriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"prescriptions": list})
}
