package routes

import (
	"hospital-backend/handlers"
	"hospital-backend/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Backend running 🚀",
		})
	})

	// Register API
	r.POST("/register", handlers.Register)
	// Login API
	r.POST("/login", handlers.Login)

	// Protected routes// Protected routes
	auth := r.Group("")
	auth.Use(middleware.AuthMiddleware())
	{
		auth.GET("/profile", handlers.Profile)
		auth.GET("/admin", middleware.RoleMiddleware("admin"), handlers.AdminDashboard)
		auth.GET("/doctor", middleware.RoleMiddleware("doctor"), handlers.DoctorDashboard)
		auth.GET("/patient", middleware.RoleMiddleware("patient"), handlers.PatientDashboard)
		auth.POST("/departments", middleware.RoleMiddleware("admin"), handlers.CreateDepartment)
		auth.GET("/departments", handlers.GetDepartments)
		auth.GET("/departments/overview", handlers.GetDepartmentOverview)
		auth.GET("/floor-numbers", handlers.GetFloorNumbers)
		auth.POST("/doctors", middleware.RoleMiddleware("admin"), handlers.CreateDoctor)
		auth.GET("/doctors", handlers.GetDoctors)
		auth.GET("/doctors/:id", handlers.GetDoctorByID)
		auth.PUT("/doctors/:id", middleware.RoleMiddleware("admin"), handlers.UpdateDoctor)
		auth.DELETE("/doctors/:id", middleware.RoleMiddleware("admin"), handlers.DeleteDoctor)
		auth.POST("/admin/receptionists", middleware.RoleMiddleware("admin"), handlers.CreateReceptionistByAdmin)
		auth.GET("/admin/receptionists", handlers.GetReceptionists)
		auth.PUT("/admin/receptionists/:id", middleware.RoleMiddleware("admin"), handlers.UpdateReceptionist)
		auth.DELETE("/admin/receptionists/:id", middleware.RoleMiddleware("admin"), handlers.DeleteReceptionist)
		auth.POST("/admin/pharmacists", middleware.RoleMiddleware("admin"), handlers.CreatePharmacistByAdmin)
		auth.GET("/admin/pharmacists", handlers.GetPharmacists)
		auth.PUT("/admin/pharmacists/:id", middleware.RoleMiddleware("admin"), handlers.UpdatePharmacist)
		auth.DELETE("/admin/pharmacists/:id", middleware.RoleMiddleware("admin"), handlers.DeletePharmacist)
		auth.POST("/patients", middleware.RoleMiddleware("receptionist"), handlers.CreatePatient)
		auth.GET("/patients", handlers.GetPatients)
		auth.GET("/patients/:id", handlers.GetPatientByID)
		auth.PUT("/patients/:id", middleware.RoleMiddleware("receptionist"), handlers.UpdatePatient)
		auth.PUT("/departments/:id", middleware.RoleMiddleware("admin"), handlers.UpdateDepartment)
		auth.DELETE("/departments/:id", middleware.RoleMiddleware("admin"), handlers.DeleteDepartment)
		auth.POST("/logout", handlers.Logout)
		auth.POST("/appointments", middleware.RoleMiddleware("patient"), handlers.CreateAppointment)

		auth.GET("/appointments", handlers.GetAppointments)
		auth.GET("/receptionist/rejected-appointments", middleware.RoleMiddleware("receptionist"), handlers.GetReceptionistRejectedAppointments)
		auth.GET("/doctor/appointments", middleware.RoleMiddleware("doctor"), handlers.GetDoctorAppointments)

		auth.GET("/doctor/requests", middleware.RoleMiddleware("doctor"), handlers.GetDoctorRequests)
		auth.GET("/doctor/patients", middleware.RoleMiddleware("doctor"), handlers.GetDoctorPatients)
		auth.GET("/doctor/patients/:patientId/history", middleware.RoleMiddleware("doctor"), handlers.GetDoctorPatientHistory)
		auth.PUT("/appointments/:id/reschedule", middleware.RoleMiddleware("receptionist"), handlers.RescheduleAppointment)
		auth.DELETE("/appointments/:id", middleware.RoleMiddleware("receptionist"), handlers.DeleteCancelledAppointment)

		auth.PUT("/appointments/:id/accept", middleware.RoleMiddleware("doctor"), handlers.AcceptAppointment)

		auth.PUT("/appointments/:id/reject", middleware.RoleMiddleware("doctor"), handlers.RejectAppointment)

		auth.PUT("/appointments/:id/complete", middleware.RoleMiddleware("doctor"), handlers.CompleteAppointment)

		auth.GET("/my-appointments", middleware.RoleMiddleware("patient"), handlers.GetMyAppointments)
		auth.GET("/my-prescriptions", middleware.RoleMiddleware("patient"), handlers.GetMyPrescriptions)

		auth.POST("/pharmacy/items", middleware.RoleMiddleware("admin", "pharmacy"), handlers.CreatePharmacyItem)
		auth.GET("/pharmacy/items", middleware.RoleMiddleware("admin", "pharmacy", "doctor"), handlers.GetPharmacyItems)
		auth.GET("/pharmacy/items/:id", middleware.RoleMiddleware("admin", "pharmacy"), handlers.GetPharmacyItemByID)
		auth.PUT("/pharmacy/items/:id", middleware.RoleMiddleware("admin", "pharmacy"), handlers.UpdatePharmacyItem)
		auth.DELETE("/pharmacy/items/:id", middleware.RoleMiddleware("admin", "pharmacy"), handlers.DeletePharmacyItem)

		auth.POST("/lab-reports", middleware.RoleMiddleware("admin", "doctor", "receptionist"), handlers.CreateLabTest)
		auth.GET("/lab-reports", middleware.RoleMiddleware("admin", "doctor", "receptionist", "pharmacy"), handlers.GetLabReports)
		auth.GET("/lab-reports/:id", middleware.RoleMiddleware("admin", "doctor", "receptionist", "patient", "pharmacy"), handlers.GetLabTestByID)
		auth.PUT("/lab-reports/:id", middleware.RoleMiddleware("admin", "doctor", "receptionist", "pharmacy"), handlers.UpdateLabTest)
		auth.GET("/my-lab-reports", middleware.RoleMiddleware("patient"), handlers.GetMyLabReports)
		auth.GET("/my-bills", middleware.RoleMiddleware("patient"), handlers.GetMyBills)

		// Lab Tests Catalog (master data with pricing)
		auth.GET("/lab-tests-catalog", handlers.GetLabTestsCatalog)
		auth.GET("/lab-tests-catalog/:id", handlers.GetLabTestCatalogByID)
		auth.GET("/lab-tests-catalog/category/:category", handlers.GetLabTestsByCategory)
		auth.POST("/lab-tests-catalog", middleware.RoleMiddleware("admin"), handlers.CreateLabTestCatalog)
		auth.PUT("/lab-tests-catalog/:id", middleware.RoleMiddleware("admin"), handlers.UpdateLabTestCatalog)
		auth.DELETE("/lab-tests-catalog/:id", middleware.RoleMiddleware("admin"), handlers.DeleteLabTestCatalog)
	}
}
