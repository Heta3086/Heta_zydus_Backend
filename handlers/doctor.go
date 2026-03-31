package handlers

import (
	"context"
	"fmt"
	"net/http"

	"hospital-backend/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type DoctorInput struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	DepartmentID   int    `json:"department_id"`
	Specialization string `json:"specialization"`
	Qualification  string `json:"qualification"`
	ExperienceYrs  int    `json:"experience_yrs"`
}

// CREATE DOCTOR (ADMIN ONLY)
func CreateDoctor(c *gin.Context) {
	var input DoctorInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.DepartmentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid department_id is required"})
		return
	}

	// 🔐 hash password
	hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte(input.Password), 10)
	if hashErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Password hashing failed"})
		return
	}

	tx, err := config.DB.Begin(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(context.Background())

	// 1️⃣ create user
	var userID int
	err = tx.QueryRow(context.Background(),
		"INSERT INTO users (email, password_hash, role, name) VALUES ($1,$2,'doctor',$3) RETURNING user_id",
		input.Email, string(hashedPassword), input.Name,
	).Scan(&userID)

	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor email already exists"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": pgErr.Message})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation failed"})
		return
	}

	// 2️⃣ create doctor profile
	var doctorID int
	err = tx.QueryRow(context.Background(),
		`INSERT INTO doctors 
		(user_id, department_id, first_name, last_name, specialization, qualification, experience_yrs) 
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING doctor_id`,
		userID,
		input.DepartmentID,
		input.Name, // simplified
		"",
		input.Specialization,
		input.Qualification,
		input.ExperienceYrs,
	).Scan(&doctorID)

	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": pgErr.Message})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Doctor creation failed"})
		return
	}

	if err = tx.Commit(context.Background()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit doctor creation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Doctor created successfully",
		"doctor_id": doctorID,
		"user_id":   userID,
	})
}

func GetDoctors(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT d.doctor_id,
		        COALESCE(u.name, ''),
		        COALESCE(u.email, ''),
		        COALESCE(d.specialization, ''),
		        COALESCE(d.qualification, ''),
		        COALESCE(d.department_id, 0),
		        COALESCE(d.experience_yrs, 0)
		 FROM doctors d
		 LEFT JOIN users u ON d.user_id = u.user_id
		 ORDER BY d.doctor_id ASC`,
	)

	if err != nil {
		fmt.Println("[ERROR] GetDoctors query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Fetch failed"})
		return
	}
	defer rows.Close()

	var doctors []map[string]interface{}

	for rows.Next() {
		var id, deptID, experienceYrs int
		var name, email, spec, qualification string

		if err := rows.Scan(&id, &name, &email, &spec, &qualification, &deptID, &experienceYrs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse doctor rows"})
			return
		}

		doctors = append(doctors, gin.H{
			"id":             id,
			"name":           name,
			"email":          email,
			"specialization": spec,
			"qualification":  qualification,
			"experience_yrs": experienceYrs,
			"department_id":  deptID,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate doctor rows"})
		return
	}

	c.JSON(http.StatusOK, doctors)
}

func GetDoctorByID(c *gin.Context) {
	id := c.Param("id")

	var doctorID, deptID, experienceYrs int
	var name, email, specialization, qualification string

	err := config.DB.QueryRow(context.Background(),
		`SELECT d.doctor_id,
		        COALESCE(u.name, ''),
		        COALESCE(u.email, ''),
		        COALESCE(d.specialization, ''),
		        COALESCE(d.qualification, ''),
		        COALESCE(d.department_id, 0),
		        COALESCE(d.experience_yrs, 0)
		 FROM doctors d
		 LEFT JOIN users u ON d.user_id = u.user_id
		 WHERE d.doctor_id=$1`, id,
	).Scan(&doctorID, &name, &email, &specialization, &qualification, &deptID, &experienceYrs)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Doctor not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             doctorID,
		"name":           name,
		"email":          email,
		"specialization": specialization,
		"qualification":  qualification,
		"experience_yrs": experienceYrs,
		"department_id":  deptID,
	})
}

func UpdateDoctor(c *gin.Context) {
	id := c.Param("id")

	var input DoctorInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := config.DB.Begin(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(),
		`UPDATE users 
		 SET name = COALESCE(NULLIF($1, ''), name),
		     email = COALESCE(NULLIF($2, ''), email)
		 WHERE user_id = (SELECT user_id FROM doctors WHERE doctor_id = $3)`,
		input.Name,
		input.Email,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update doctor user profile"})
		return
	}

	_, err = tx.Exec(context.Background(),
		`UPDATE doctors
		 SET specialization = COALESCE(NULLIF($1, ''), specialization),
		     qualification = COALESCE(NULLIF($2, ''), qualification),
		     experience_yrs = CASE WHEN $3 > 0 THEN $3 ELSE experience_yrs END,
		     department_id = CASE WHEN $4 > 0 THEN $4 ELSE department_id END
		 WHERE doctor_id = $5`,
		input.Specialization,
		input.Qualification,
		input.ExperienceYrs,
		input.DepartmentID,
		id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Update failed"})
		return
	}

	if err = tx.Commit(context.Background()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit doctor update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Doctor updated"})
}

func DeleteDoctor(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		"DELETE FROM users WHERE user_id = (SELECT user_id FROM doctors WHERE doctor_id = $1)", id,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Doctor deleted"})
}
