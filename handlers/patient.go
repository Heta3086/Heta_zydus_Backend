package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"hospital-backend/config"

	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type PatientInput struct {
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	Phone           string `json:"phone"`
	Gender          string `json:"gender"`
	BloodType       string `json:"blood_type"`
	DateOfBirth     string `json:"date_of_birth"`
	Address         string `json:"address"`
	DepartmentID    int    `json:"department_id"`
	DoctorID        int    `json:"doctor_id"`
	AppointmentDate string `json:"appointment_date"`
	AppointmentTime string `json:"appointment_time"`
	Reason          string `json:"reason"`
}

// CREATE PATIENT
func CreatePatient(c *gin.Context) {
	var input PatientInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Phone = normalizePatientPhone(input.Phone)
	input.BloodType = strings.TrimSpace(input.BloodType)
	input.Address = strings.TrimSpace(input.Address)
	input.AppointmentDate = strings.TrimSpace(input.AppointmentDate)
	input.AppointmentTime = strings.TrimSpace(input.AppointmentTime)
	input.Reason = strings.TrimSpace(input.Reason)
	if strings.TrimSpace(input.DateOfBirth) == "" {
		input.DateOfBirth = "1995-01-01"
	}
	gender, genderValid := normalizePatientGender(input.Gender)
	bloodType, bloodTypeValid := normalizePatientBloodType(input.BloodType)

	if input.FirstName == "" || input.Email == "" || input.Password == "" || input.Phone == "" || input.Address == "" || input.BloodType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "all patient details are required"})
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

	hasSchedulingInput := input.DepartmentID > 0 || input.DoctorID > 0 || input.AppointmentDate != "" || input.AppointmentTime != "" || input.Reason != ""
	if hasSchedulingInput {
		if input.DepartmentID <= 0 || input.DoctorID <= 0 || input.AppointmentDate == "" || input.AppointmentTime == "" || input.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "department, doctor, appointment date/time and reason are required when scheduling an appointment"})
			return
		}
	}

	// Hash password before writing anything in DB.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password hashing failed"})
		return
	}

	ctx := context.Background()
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not start transaction"})
		return
	}
	defer tx.Rollback(ctx)

	var existingPatientID int
	err = tx.QueryRow(ctx,
		"SELECT patient_id FROM patients WHERE phone=$1 LIMIT 1",
		input.Phone,
	).Scan(&existingPatientID)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Phone number already exists"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate phone uniqueness"})
		return
	}

	err = tx.QueryRow(ctx,
		`SELECT patient_id
		 FROM patients
		 WHERE lower(first_name)=lower($1)
		   AND lower(last_name)=lower($2)
		   AND phone=$3
		   AND gender=$4
		   AND lower(COALESCE(address,''))=lower($5)
		 LIMIT 1`,
		input.FirstName,
		input.LastName,
		input.Phone,
		gender,
		input.Address,
	).Scan(&existingPatientID)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Duplicate patient details found"})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate duplicate patient details"})
		return
	}

	// 1) create user
	var userID int
	err = tx.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, role, name) VALUES ($1,$2,'patient',$3) RETURNING user_id",
		input.Email, string(hashedPassword), strings.TrimSpace(input.FirstName+" "+input.LastName),
	).Scan(&userID)

	if err != nil {
		c.JSON(resolveDBStatus(err), gin.H{"error": resolveDBMessage(err, "User creation failed")})
		return
	}

	// 2) create patient profile
	var patientID int
	err = tx.QueryRow(ctx,
		`INSERT INTO patients 
		(user_id, first_name, last_name, date_of_birth, gender, blood_type, phone, address)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING patient_id`,
		userID,
		input.FirstName,
		input.LastName,
		input.DateOfBirth,
		gender,
		bloodType,
		input.Phone,
		input.Address,
	).Scan(&patientID)

	if err != nil {
		c.JSON(resolveDBStatus(err), gin.H{"error": resolveDBMessage(err, "Patient creation failed")})
		return
	}

	if input.DoctorID > 0 {
		appointmentDate := input.AppointmentDate
		appointmentTime := input.AppointmentTime
		reason := input.Reason

		_, err = tx.Exec(ctx,
			`INSERT INTO appointments
			(patient_id, doctor_id, appointment_date, appointment_time, reason, status)
			VALUES ($1,$2,$3,$4,$5,'scheduled')`,
			patientID,
			input.DoctorID,
			appointmentDate,
			appointmentTime,
			reason,
		)

		if err != nil {
			c.JSON(resolveDBStatus(err), gin.H{"error": resolveDBMessage(err, "Appointment creation failed")})
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize patient creation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Patient created successfully"})
}

func resolveDBStatus(err error) int {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23505", pgErr.Code == "23503", pgErr.Code == "22007":
			return http.StatusBadRequest
		case strings.HasPrefix(pgErr.Code, "42"):
			// SQL/schema errors should surface clearly for fast diagnosis.
			return http.StatusBadRequest
		}
	}
	return http.StatusInternalServerError
}

func resolveDBMessage(err error, fallback string) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "users_email_key" {
				return "Email already exists"
			}
			return "Record already exists"
		case "23503":
			return "Invalid doctor or related record"
		case "22007":
			return "Invalid date or time format"
		case "42P01":
			return "Required table is missing in database"
		case "42703":
			return "Required column is missing in database"
		}
		if pgErr.Message != "" {
			return pgErr.Message
		}
	}
	return fallback
}

func GetPatients(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT p.patient_id,
		        COALESCE(u.name, ''),
		        COALESCE(p.phone, ''),
		        COALESCE(p.gender, ''),
		        COALESCE(p.blood_type, '')
		 FROM patients p
		 LEFT JOIN users u ON p.user_id = u.user_id
		 ORDER BY p.patient_id ASC`,
	)

	if err != nil {
		fmt.Println("[ERROR] GetPatients query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Fetch failed"})
		return
	}
	defer rows.Close()

	var patients []map[string]interface{}

	for rows.Next() {
		var id int
		var name, phone, gender, bloodType string

		if err := rows.Scan(&id, &name, &phone, &gender, &bloodType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse patient rows"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate patient rows"})
		return
	}

	c.JSON(http.StatusOK, patients)
}

func GetPatientByID(c *gin.Context) {
	id := c.Param("id")

	var patientID int
	var name, phone, gender, bloodType string

	err := config.DB.QueryRow(context.Background(),
		`SELECT p.patient_id,
		        COALESCE(u.name, ''),
		        COALESCE(p.phone, ''),
		        COALESCE(p.gender, ''),
		        COALESCE(p.blood_type, '')
		 FROM patients p
		 LEFT JOIN users u ON p.user_id = u.user_id
		 WHERE p.patient_id=$1`, id,
	).Scan(&patientID, &name, &phone, &gender, &bloodType)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Patient not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         patientID,
		"name":       name,
		"phone":      phone,
		"gender":     gender,
		"blood_type": bloodType,
	})
}

func UpdatePatient(c *gin.Context) {
	id := c.Param("id")

	var input PatientInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gender, genderValid := normalizePatientGender(input.Gender)
	if !genderValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gender must be Male, Female, or Other"})
		return
	}

	phone := normalizePatientPhone(input.Phone)
	if !isValidPatientPhone(phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone must be exactly 10 digits"})
		return
	}

	bloodType := ""
	if strings.TrimSpace(input.BloodType) != "" {
		var bloodTypeValid bool
		bloodType, bloodTypeValid = normalizePatientBloodType(input.BloodType)
		if !bloodTypeValid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "blood_type must be one of A+, A-, B+, B-, AB+, AB-, O+, O-"})
			return
		}
	}

	_, err := config.DB.Exec(context.Background(),
		`UPDATE patients 
		 SET phone=$1,
		     gender=$2,
		     address=$3,
		     blood_type = COALESCE(NULLIF($4, ''), blood_type)
		 WHERE patient_id=$5`,
		phone,
		gender,
		input.Address,
		bloodType,
		id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Update failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Patient updated"})
}

func normalizePatientGender(value string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "male", "m":
		return "male", true
	case "female", "f":
		return "female", true
	case "other", "o":
		return "other", true
	default:
		return "", false
	}
}

func normalizePatientPhone(value string) string {
	v := strings.TrimSpace(value)
	re := regexp.MustCompile(`[^0-9]`)
	return re.ReplaceAllString(v, "")
}

func isValidPatientPhone(value string) bool {
	return regexp.MustCompile(`^[0-9]{10}$`).MatchString(strings.TrimSpace(value))
}

func normalizePatientBloodType(value string) (string, bool) {
	v := strings.ToUpper(strings.TrimSpace(value))
	valid := map[string]bool{
		"A+":  true,
		"A-":  true,
		"B+":  true,
		"B-":  true,
		"AB+": true,
		"AB-": true,
		"O+":  true,
		"O-":  true,
	}
	return v, valid[v]
}
