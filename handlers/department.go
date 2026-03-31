package handlers

import (
	"context"
	"net/http"

	"hospital-backend/config"

	"fmt"

	"github.com/gin-gonic/gin"
)

type Department struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Floor       string `json:"floor_number"`
}

type DepartmentResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Floor       string `json:"floor_number"`
}

type DepartmentOverviewResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Floor         string `json:"floor_number"`
	DoctorsCount  int    `json:"doctors_count"`
	PatientsCount int    `json:"patients_count"`
}

// CREATE
func CreateDepartment(c *gin.Context) {
	var dept Department

	if err := c.ShouldBindJSON(&dept); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := config.DB.Exec(context.Background(),
		"INSERT INTO departments (hospital_id, name, description, floor_number) VALUES ($1,$2,$3,$4)",
		1, dept.Name, dept.Description, dept.Floor,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Insert failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Department created"})
}

// READ
func GetDepartments(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		"SELECT department_id, name, description, floor_number FROM departments ORDER BY department_id ASC",
	)
	if err != nil {
		fmt.Println("[ERROR] GetDepartments query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch departments"})
		return
	}
	defer rows.Close()

	departments := make([]DepartmentResponse, 0)
	for rows.Next() {
		var dept DepartmentResponse
		if err := rows.Scan(&dept.ID, &dept.Name, &dept.Description, &dept.Floor); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse departments"})
			return
		}
		departments = append(departments, dept)
	}

	if rows.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate departments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"departments": departments})
}

// READ DEPARTMENT OVERVIEW (DB-DRIVEN COUNTS)
func GetDepartmentOverview(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		`SELECT
			d.department_id,
			d.name,
			d.floor_number,
			COUNT(DISTINCT doc.doctor_id) AS doctors_count,
			COUNT(DISTINCT CASE
				WHEN LOWER(TRIM(COALESCE(a.status, ''))) IN ('confirmed', 'accepted', 'approved', 'in_progress', 'in progress', 'completed')
				THEN a.patient_id
			END) AS patients_count
		 FROM departments d
		 LEFT JOIN doctors doc ON doc.department_id = d.department_id
		 LEFT JOIN appointments a ON a.doctor_id = doc.doctor_id
		 GROUP BY d.department_id, d.name, d.floor_number
		 ORDER BY d.department_id ASC`,
	)
	if err != nil {
		fmt.Println("[ERROR] GetDepartmentOverview query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch department overview"})
		return
	}
	defer rows.Close()

	overview := make([]DepartmentOverviewResponse, 0)
	for rows.Next() {
		var row DepartmentOverviewResponse
		if err := rows.Scan(&row.ID, &row.Name, &row.Floor, &row.DoctorsCount, &row.PatientsCount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse department overview"})
			return
		}
		overview = append(overview, row)
	}

	if rows.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate department overview"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"overview": overview})
}

// UPDATE
func UpdateDepartment(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Floor       string `json:"floor_number"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	_, err := config.DB.Exec(context.Background(),
		`UPDATE departments 
		 SET name=$1, description=$2, floor_number=$3 
		 WHERE department_id=$4`,
		input.Name, input.Description, input.Floor, id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Update failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Department updated"})
}

// DELETE
func DeleteDepartment(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec(context.Background(),
		"DELETE FROM departments WHERE department_id=$1",
		id,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Delete failed"})
		return
	}

	c.JSON(200, gin.H{"message": "Department deleted"})
}

// GET UNIQUE FLOOR NUMBERS
func GetFloorNumbers(c *gin.Context) {
	rows, err := config.DB.Query(context.Background(),
		"SELECT DISTINCT TRIM(floor_number) AS floor_number FROM departments WHERE floor_number IS NOT NULL AND TRIM(floor_number) <> '' ORDER BY floor_number ASC",
	)
	if err != nil {
		fmt.Println("[ERROR] GetFloorNumbers query failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch floor numbers"})
		return
	}
	defer rows.Close()

	floorNumbers := make([]string, 0)
	for rows.Next() {
		var floor string
		if err := rows.Scan(&floor); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse floor numbers"})
			return
		}
		floorNumbers = append(floorNumbers, floor)
	}

	if rows.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to iterate floor numbers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"floor_numbers": floorNumbers})
}
