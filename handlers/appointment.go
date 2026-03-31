package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
)

func getCurrentUserID(c *gin.Context) (int, error) {
	raw, exists := c.Get("user_id")
	if !exists {
		return 0, fmt.Errorf("user id missing from token")
	}

	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		id, err := strconv.Atoi(v)
		if err != nil {
			return 0, err
		}
		return id, nil
	default:
		id, err := strconv.Atoi(fmt.Sprint(v))
		if err != nil {
			return 0, err
		}
		return id, nil
	}
}

func parsePrescriptionJSON(raw string) ([]gin.H, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" || trimmed == "{}" {
		return nil, false
	}

	var arr []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		result := make([]gin.H, 0, len(arr))
		for _, item := range arr {
			result = append(result, gin.H(item))
		}
		if len(result) > 0 {
			return result, true
		}
	}

	var single map[string]any
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil && len(single) > 0 {
		return []gin.H{gin.H(single)}, true
	}

	return nil, false
}

func getPrescriptionItemsByAppointment(appointmentID int) ([]gin.H, error) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT COALESCE(m.name, ''),
		        COALESCE(pi.dosage, ''),
		        COALESCE(pi.frequency, ''),
		        COALESCE(pi.duration_days, 0),
		        COALESCE(pi.instructions, '')
		 FROM prescriptions p
		 JOIN prescription_items pi ON pi.prescription_id = p.prescription_id
		 JOIN medicines m ON m.medicine_id = pi.medicine_id
		 WHERE p.appointment_id = $1
		 ORDER BY pi.item_id ASC`,
		appointmentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]gin.H, 0)
	for rows.Next() {
		var name, dosage, frequency, instructions string
		var durationDays int

		if err := rows.Scan(&name, &dosage, &frequency, &durationDays, &instructions); err != nil {
			return nil, err
		}

		dose := dosage
		if strings.TrimSpace(dose) == "" {
			dose = frequency
		}

		duration := "-"
		if durationDays > 0 {
			duration = fmt.Sprintf("%d Days", durationDays)
		}

		items = append(items, gin.H{
			"name":         name,
			"dosage":       dose,
			"duration":     duration,
			"instructions": instructions,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func durationToDays(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 1
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return 1
	}

	if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
		return n
	}

	return 1
}

func syncPrescriptionTables(appointmentID int, payload any, notes string) error {
	var patientID, doctorID int
	err := config.DB.QueryRow(context.Background(),
		"SELECT patient_id, doctor_id FROM appointments WHERE appointment_id=$1",
		appointmentID,
	).Scan(&patientID, &doctorID)
	if err != nil {
		return err
	}

	var prescriptionID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT prescription_id FROM prescriptions WHERE appointment_id=$1 ORDER BY prescription_id DESC LIMIT 1",
		appointmentID,
	).Scan(&prescriptionID)
	if err != nil {
		if err.Error() == "no rows in result set" || strings.Contains(strings.ToLower(err.Error()), "no rows") {
			err = config.DB.QueryRow(context.Background(),
				`INSERT INTO prescriptions (appointment_id, patient_id, doctor_id, notes, status)
				 VALUES ($1,$2,$3,$4,'pending')
				 RETURNING prescription_id`,
				appointmentID, patientID, doctorID, notes,
			).Scan(&prescriptionID)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		_, _ = config.DB.Exec(context.Background(), "DELETE FROM prescription_items WHERE prescription_id=$1", prescriptionID)
		_, _ = config.DB.Exec(context.Background(), "UPDATE prescriptions SET notes=$1 WHERE prescription_id=$2", notes, prescriptionID)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}

	for _, item := range items {
		medicineName := strings.TrimSpace(fmt.Sprint(item["name"]))
		if medicineName == "" {
			medicineName = strings.TrimSpace(fmt.Sprint(item["medicine"]))
		}
		if medicineName == "" {
			medicineName = strings.TrimSpace(fmt.Sprint(item["medicine_name"]))
		}
		if medicineName == "" {
			continue
		}

		dosage := strings.TrimSpace(fmt.Sprint(item["dosage"]))
		frequency := strings.TrimSpace(fmt.Sprint(item["frequency"]))
		if frequency == "" {
			frequency = dosage
		}
		instructions := strings.TrimSpace(fmt.Sprint(item["instructions"]))
		durationDays := durationToDays(fmt.Sprint(item["duration"]))

		var medicineID int
		if err := config.DB.QueryRow(context.Background(),
			"SELECT medicine_id FROM medicines WHERE LOWER(name)=LOWER($1) ORDER BY medicine_id LIMIT 1",
			medicineName,
		).Scan(&medicineID); err != nil {
			continue
		}

		_, _ = config.DB.Exec(context.Background(),
			`INSERT INTO prescription_items
			 (prescription_id, medicine_id, dosage, frequency, duration_days, quantity, instructions)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			prescriptionID,
			medicineID,
			dosage,
			frequency,
			durationDays,
			1,
			instructions,
		)
	}

	return nil
}

func CreateAppointment(c *gin.Context) {
	var input struct {
		DoctorID int    `json:"doctor_id"`
		Date     string `json:"appointment_date"`
		Time     string `json:"appointment_time"`
		Reason   string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid user"})
		return
	}

	// get patient_id
	var patientID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT patient_id FROM patients WHERE user_id=$1",
		userID,
	).Scan(&patientID)

	if err != nil {
		c.JSON(400, gin.H{"error": "Patient not found"})
		return
	}

	_, err = config.DB.Exec(context.Background(),
		`INSERT INTO appointments 
		(patient_id, doctor_id, appointment_date, appointment_time, reason, status)
		VALUES ($1,$2,$3,$4,$5,'scheduled')`,
		patientID, input.DoctorID, input.Date, input.Time, input.Reason,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Booking failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Appointment requested"})
}

func GetAppointments(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT appointment_id,
		        patient_id,
		        doctor_id,
		        COALESCE(appointment_date::text, ''),
		        COALESCE(appointment_time::text, ''),
		        COALESCE(status, ''),
		        COALESCE(reason, '')
		 FROM appointments
		 ORDER BY appointment_id DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load appointments"})
		return
	}
	defer rows.Close()

	var list []gin.H

	for rows.Next() {
		var id, pid, did int
		var date, timeSlot, status, reason string

		if err := rows.Scan(&id, &pid, &did, &date, &timeSlot, &status, &reason); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse appointments"})
			return
		}

		list = append(list, gin.H{
			"id":               id,
			"patient_id":       pid,
			"doctor_id":        did,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"date":             date,
			"status":           status,
			"reason":           reason,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate appointments"})
		return
	}

	c.JSON(http.StatusOK, list)
}

func GetDoctorAppointments(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user session"})
		return
	}

	var doctorID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Doctor profile not found"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT a.appointment_id,
		        a.patient_id,
		        a.doctor_id,
		        COALESCE(a.appointment_date::text, ''),
		        COALESCE(a.appointment_time::text, ''),
		        COALESCE(a.status, ''),
		        COALESCE(a.reason, ''),
		        COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, '')))
		 FROM appointments a
		 LEFT JOIN patients p ON p.patient_id = a.patient_id
		 LEFT JOIN users u ON u.user_id = p.user_id
		 WHERE a.doctor_id=$1
		 ORDER BY a.appointment_id DESC`,
		doctorID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to load doctor appointments"})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id, pid, did int
		var date, timeSlot, status, reason, patientName string

		if err := rows.Scan(&id, &pid, &did, &date, &timeSlot, &status, &reason, &patientName); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse doctor appointments"})
			return
		}

		list = append(list, gin.H{
			"id":               id,
			"patient_id":       pid,
			"doctor_id":        did,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"status":           status,
			"reason":           reason,
			"patient": gin.H{
				"name": patientName,
			},
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to read doctor appointments"})
		return
	}

	c.JSON(200, list)
}

func GetDoctorRequests(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user session"})
		return
	}

	var doctorID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Doctor profile not found"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT a.appointment_id,
		        a.patient_id,
		        a.doctor_id,
		        COALESCE(a.appointment_date::text, ''),
		        COALESCE(a.appointment_time::text, ''),
		        a.status,
		        COALESCE(a.reason, ''),
		        COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, '')))
		 FROM appointments a
		 LEFT JOIN patients p ON p.patient_id = a.patient_id
		 LEFT JOIN users u ON u.user_id = p.user_id
		 WHERE a.doctor_id=$1 AND a.status='scheduled'`,
		doctorID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to load doctor requests"})
		return
	}

	defer rows.Close()

	var list []gin.H

	for rows.Next() {
		var id, pid, did int
		var date, timeSlot, status, reason, patientName string

		if err := rows.Scan(&id, &pid, &did, &date, &timeSlot, &status, &reason, &patientName); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse doctor requests"})
			return
		}

		list = append(list, gin.H{
			"id":               id,
			"patient_id":       pid,
			"doctor_id":        did,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"status":           status,
			"reason":           reason,
			"patient": gin.H{
				"name": patientName,
			},
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to read doctor requests"})
		return
	}

	c.JSON(200, list)
}

func GetDoctorPatients(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user session"})
		return
	}

	var doctorID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Doctor profile not found"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT DISTINCT p.patient_id,
		        COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, ''))),
		        COALESCE(p.phone, ''),
		        COALESCE(p.gender, ''),
		        COALESCE(p.blood_type, '')
		 FROM appointments a
		 JOIN patients p ON p.patient_id = a.patient_id
		 LEFT JOIN users u ON u.user_id = p.user_id
		 WHERE a.doctor_id=$1
		   AND a.status = 'completed'
		 ORDER BY p.patient_id DESC`,
		doctorID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to load doctor patients"})
		return
	}
	defer rows.Close()

	var patients []gin.H
	for rows.Next() {
		var id int
		var name, phone, gender, bloodType string
		if err := rows.Scan(&id, &name, &phone, &gender, &bloodType); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse doctor patients"})
			return
		}

		patients = append(patients, gin.H{
			"id":         id,
			"name":       name,
			"phone":      phone,
			"gender":     gender,
			"blood_type": bloodType,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to read doctor patients"})
		return
	}

	c.JSON(200, patients)
}

func GetDoctorPatientHistory(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user session"})
		return
	}

	patientID := c.Param("patientId")

	var doctorID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT doctor_id FROM doctors WHERE user_id=$1",
		userID,
	).Scan(&doctorID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Doctor profile not found"})
		return
	}

	var patientName, phone, gender, bloodType string
	err = config.DB.QueryRow(context.Background(),
		`SELECT COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, ''))),
		        COALESCE(p.phone, ''),
		        COALESCE(p.gender, ''),
		        COALESCE(p.blood_type, '')
		 FROM patients p
		 LEFT JOIN users u ON u.user_id = p.user_id
		 WHERE p.patient_id = $1`,
		patientID,
	).Scan(&patientName, &phone, &gender, &bloodType)
	if err != nil {
		c.JSON(404, gin.H{"error": "Patient not found"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT a.appointment_id,
		        COALESCE(a.appointment_date::text, ''),
		        COALESCE(a.appointment_time::text, ''),
		        COALESCE(a.status, ''),
		        COALESCE(a.reason, ''),
		        COALESCE(a.diagnosis, ''),
		        COALESCE(a.symptoms, ''),
		        COALESCE(a.treatment, ''),
		        COALESCE(a.doctor_notes, ''),
		        COALESCE(a.prescription_json, '[]')
		 FROM appointments a
		 WHERE a.patient_id = $1 AND a.doctor_id = $2 AND a.status = 'completed'
		 ORDER BY a.appointment_date DESC, a.appointment_time DESC, a.appointment_id DESC`,
		patientID,
		doctorID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to load patient history"})
		return
	}
	defer rows.Close()

	var history []gin.H
	for rows.Next() {
		var id int
		var date, timeSlot, status, reason, diagnosis, symptoms, treatment, doctorNotes, prescriptionJSON string
		if err := rows.Scan(&id, &date, &timeSlot, &status, &reason, &diagnosis, &symptoms, &treatment, &doctorNotes, &prescriptionJSON); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse patient history"})
			return
		}

		var prescription any = []any{}
		if err := json.Unmarshal([]byte(prescriptionJSON), &prescription); err != nil {
			prescription = []any{}
		}

		history = append(history, gin.H{
			"appointment_id":   id,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"status":           status,
			"reason":           reason,
			"diagnosis":        diagnosis,
			"symptoms":         symptoms,
			"treatment":        treatment,
			"doctor_notes":     doctorNotes,
			"prescription":     prescription,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to read patient history"})
		return
	}

	c.JSON(200, gin.H{
		"patient": gin.H{
			"id":         patientID,
			"name":       patientName,
			"phone":      phone,
			"gender":     gender,
			"blood_type": bloodType,
		},
		"history": history,
	})
}

func GetReceptionistRejectedAppointments(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT a.appointment_id,
		        a.patient_id,
		        a.doctor_id,
		        COALESCE(a.appointment_date::text, ''),
		        COALESCE(a.appointment_time::text, ''),
		        COALESCE(a.status, ''),
		        COALESCE(a.reason, ''),
		        COALESCE(pu.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, ''))),
		        COALESCE(du.name, CONCAT(COALESCE(d.first_name, ''), ' ', COALESCE(d.last_name, '')))
		 FROM appointments a
		 LEFT JOIN patients p ON p.patient_id = a.patient_id
		 LEFT JOIN users pu ON pu.user_id = p.user_id
		 LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
		 LEFT JOIN users du ON du.user_id = d.user_id
		 WHERE a.status='cancelled'
		 ORDER BY a.appointment_id DESC`)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to load rejected appointments"})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id, pid, did int
		var date, timeSlot, status, reason, patientName, doctorName string

		if err := rows.Scan(&id, &pid, &did, &date, &timeSlot, &status, &reason, &patientName, &doctorName); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse rejected appointments"})
			return
		}

		list = append(list, gin.H{
			"id":               id,
			"patient_id":       pid,
			"doctor_id":        did,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"status":           status,
			"reason":           reason,
			"patient": gin.H{
				"name": patientName,
			},
			"doctor": gin.H{
				"name": doctorName,
			},
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to read rejected appointments"})
		return
	}

	c.JSON(200, list)
}

func RescheduleAppointment(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Date   string `json:"appointment_date"`
		Time   string `json:"appointment_time"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if input.Date == "" || input.Time == "" {
		c.JSON(400, gin.H{"error": "appointment_date and appointment_time are required"})
		return
	}

	reason := input.Reason
	if reason == "" {
		reason = "Rescheduled by receptionist"
	}

	result, err := config.DB.Exec(context.Background(),
		`UPDATE appointments
		 SET appointment_date=$1,
		     appointment_time=$2,
		     reason=$3,
		     status='scheduled'
		 WHERE appointment_id=$4`,
		input.Date, input.Time, reason, id,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to reschedule appointment"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "Appointment not found"})
		return
	}

	c.JSON(200, gin.H{"message": "Appointment rescheduled and sent to doctor"})
}

func DeleteCancelledAppointment(c *gin.Context) {
	id := c.Param("id")

	result, err := config.DB.Exec(context.Background(),
		"DELETE FROM appointments WHERE appointment_id=$1 AND status='cancelled'",
		id,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete appointment"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "Cancelled appointment not found"})
		return
	}

	c.JSON(200, gin.H{"message": "Cancelled appointment deleted"})
}

func AcceptAppointment(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		"UPDATE appointments SET status='confirmed' WHERE appointment_id=$1",
		id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Update failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Appointment accepted"})
}

func RejectAppointment(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		"UPDATE appointments SET status='cancelled' WHERE appointment_id=$1",
		id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Update failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Appointment rejected"})
}

func GetMyAppointments(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user session"})
		return
	}

	var patientID int
	err = config.DB.QueryRow(context.Background(),
		"SELECT patient_id FROM patients WHERE user_id=$1",
		userID,
	).Scan(&patientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Patient profile not found"})
		return
	}

	rows, err := config.DB.Query(context.Background(),
		`SELECT a.appointment_id,
		        a.patient_id,
		        a.doctor_id,
		        COALESCE(a.appointment_date::text, ''),
		        COALESCE(a.appointment_time::text, ''),
		        COALESCE(a.status, ''),
		        COALESCE(a.reason, ''),
		        COALESCE(a.diagnosis, ''),
		        COALESCE(a.symptoms, ''),
		        COALESCE(a.treatment, ''),
		        COALESCE(a.doctor_notes, ''),
		        COALESCE(a.prescription_json, '[]'),
		        COALESCE(du.name, CONCAT(COALESCE(d.first_name, ''), ' ', COALESCE(d.last_name, '')))
		 FROM appointments a
		 LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
		 LEFT JOIN users du ON du.user_id = d.user_id
		 WHERE a.patient_id=$1
		 ORDER BY a.appointment_date DESC, a.appointment_time DESC, a.appointment_id DESC`,
		patientID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load appointments"})
		return
	}
	defer rows.Close()

	list := make([]gin.H, 0)
	for rows.Next() {
		var id, pid, did int
		var date, timeSlot, status, reason, diagnosis, symptoms, treatment, doctorNotes, prescriptionJSON, doctorName string

		if err := rows.Scan(&id, &pid, &did, &date, &timeSlot, &status, &reason, &diagnosis, &symptoms, &treatment, &doctorNotes, &prescriptionJSON, &doctorName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse appointments"})
			return
		}

		var prescriptionPayload any = []gin.H{}
		if parsed, ok := parsePrescriptionJSON(prescriptionJSON); ok {
			prescriptionPayload = parsed
		} else {
			if items, err := getPrescriptionItemsByAppointment(id); err == nil && len(items) > 0 {
				prescriptionPayload = items
			} else {
				prescriptionPayload = []gin.H{}
			}
		}

		list = append(list, gin.H{
			"id":               id,
			"patient_id":       pid,
			"doctor_id":        did,
			"appointment_date": date,
			"appointment_time": timeSlot,
			"status":           status,
			"reason":           reason,
			"diagnosis":        diagnosis,
			"symptoms":         symptoms,
			"treatment":        treatment,
			"doctor_notes":     doctorNotes,
			"prescription":     prescriptionPayload,
			"doctor": gin.H{
				"name": doctorName,
			},
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate appointments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"appointments": list})
}

func StartAppointment(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		"UPDATE appointments SET status='in_progress' WHERE appointment_id=$1",
		id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Start failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Appointment started"})
}

func CompleteAppointment(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Diagnosis    string `json:"diagnosis"`
		Symptoms     string `json:"symptoms"`
		Treatment    string `json:"treatment"`
		DoctorNotes  string `json:"doctor_notes"`
		Prescription any    `json:"prescription"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	prescriptionJSON := "[]"
	if input.Prescription != nil {
		if raw, err := json.Marshal(input.Prescription); err == nil {
			prescriptionJSON = string(raw)
		}
	}

	_, err := config.DB.Exec(context.Background(),
		`UPDATE appointments 
		 SET status='completed', diagnosis=$1, symptoms=$2, treatment=$3, doctor_notes=$4, prescription_json=$5
		 WHERE appointment_id=$6`,
		input.Diagnosis, input.Symptoms, input.Treatment, input.DoctorNotes, prescriptionJSON, id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Complete failed"})
		return
	}

	appointmentID, convErr := strconv.Atoi(id)
	if convErr == nil {
		notes := strings.TrimSpace(input.DoctorNotes)
		if notes == "" {
			notes = strings.TrimSpace(input.Treatment)
		}
		_ = syncPrescriptionTables(appointmentID, input.Prescription, notes)

		// Auto-generate bill when appointment is completed
		_ = generateBillForAppointment(appointmentID)
	}

	c.JSON(200, gin.H{"message": "Appointment completed"})
}

// generateBillForAppointment creates a bill when an appointment is completed
func generateBillForAppointment(appointmentID int) error {
	// Check if bill already exists for this appointment
	var existingBill int
	err := config.DB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM billing WHERE appointment_id=$1`,
		appointmentID,
	).Scan(&existingBill)

	if err == nil && existingBill > 0 {
		return nil // Bill already exists
	}

	// Get appointment details with consultation fee
	var patientID int
	var doctorID int
	var consultationFee int = 500 // Default consultation fee
	err = config.DB.QueryRow(context.Background(),
		`SELECT a.patient_id, a.doctor_id
		 FROM appointments a
		 WHERE a.appointment_id=$1`,
		appointmentID,
	).Scan(&patientID, &doctorID)

	if err != nil {
		return fmt.Errorf("failed to get appointment details: %v", err)
	}

	// Get doctor's consultation fee
	err = config.DB.QueryRow(context.Background(),
		`SELECT COALESCE(CAST(consultation_fee AS INTEGER), 500)
		 FROM doctors WHERE doctor_id=$1`,
		doctorID,
	).Scan(&consultationFee)

	// Calculate medicine charges from prescription
	var medicineTotal int = 0
	medicineRows, err := config.DB.Query(context.Background(),
		`SELECT COALESCE(SUM(CAST(pi.quantity AS INTEGER) * CAST(m.unit_price AS INTEGER)), 0)
		 FROM prescriptions p
		 JOIN prescription_items pi ON p.prescription_id = pi.prescription_id
		 JOIN medicines m ON pi.medicine_id = m.medicine_id
		 WHERE p.appointment_id=$1`,
		appointmentID,
	)
	if err == nil {
		defer medicineRows.Close()
		if medicineRows.Next() {
			medicineRows.Scan(&medicineTotal)
		}
	}

	// Calculate lab test charges
	var labTestTotal int = 0
	labRows, err := config.DB.Query(context.Background(),
		`SELECT COALESCE(SUM(CAST(ltc.price AS INTEGER)), 0)
		 FROM lab_reports lr
		 JOIN lab_tests_catalog ltc ON lr.test_name = ltc.name
		 WHERE lr.appointment_id=$1`,
		appointmentID,
	)
	if err == nil {
		defer labRows.Close()
		if labRows.Next() {
			labRows.Scan(&labTestTotal)
		}
	}

	// Calculate total amount
	totalAmount := consultationFee + medicineTotal + labTestTotal

	// Create description with breakdown
	description := fmt.Sprintf("Consultation: ₹%d | Medicines: ₹%d | Lab Tests: ₹%d", consultationFee, medicineTotal, labTestTotal)

	// Insert bill with total amount
	_, err = config.DB.Exec(context.Background(),
		`INSERT INTO billing (patient_id, appointment_id, amount, status, description, created_at)
		 VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)`,
		patientID, appointmentID, totalAmount, "pending", description,
	)

	if err != nil {
		return fmt.Errorf("failed to create bill: %v", err)
	}

	return nil
}
