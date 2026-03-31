package config

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var DB *pgxpool.Pool

func ensureFeatureTables() error {
	statements := []string{
		`DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
				UPDATE users
				SET role = 'receptionist'
				WHERE lower(role) IN ('receptioniest', 'recertioniest', 'receiptionist');
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
				UPDATE users
				SET role = 'pharmacy'
				WHERE lower(role) = 'pharmacist';
			END IF;
		END $$`,
		`DO $$
		DECLARE
			r record;
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
				FOR r IN
					SELECT conname
					FROM pg_constraint
					WHERE conrelid = 'users'::regclass
					  AND contype = 'c'
					  AND pg_get_constraintdef(oid) ILIKE '%role%'
				LOOP
					EXECUTE format('ALTER TABLE users DROP CONSTRAINT IF EXISTS %I', r.conname);
				END LOOP;
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users') THEN
				ALTER TABLE users
				ADD CONSTRAINT users_role_check
				CHECK (lower(role) IN ('admin', 'doctor', 'patient', 'receptionist', 'pharmacy'))
				NOT VALID;
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS appointments (
			appointment_id SERIAL PRIMARY KEY,
			patient_id INT NOT NULL,
			doctor_id INT NOT NULL,
			appointment_date DATE NOT NULL,
			appointment_time TIME,
			reason TEXT,
			status VARCHAR(50) NOT NULL DEFAULT 'scheduled',
			diagnosis TEXT,
			symptoms TEXT,
			treatment TEXT,
			doctor_notes TEXT,
			prescription_json TEXT DEFAULT '[]',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (patient_id) REFERENCES patients(patient_id) ON DELETE CASCADE,
			FOREIGN KEY (doctor_id) REFERENCES doctors(doctor_id) ON DELETE CASCADE
					)`,
		`CREATE TABLE IF NOT EXISTS pharmacy_items (
			item_id SERIAL PRIMARY KEY,
			medicine_name VARCHAR(255) NOT NULL,
			quantity INT NOT NULL DEFAULT 0,
			unit_price INT NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS lab_reports (
			lab_report_id SERIAL PRIMARY KEY,
			patient_id INT NOT NULL,
			appointment_id INT,
			test_name VARCHAR(255) NOT NULL,
			result TEXT,
			status VARCHAR(50) NOT NULL DEFAULT 'ordered',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS billing (
			bill_id SERIAL PRIMARY KEY,
			patient_id INT NOT NULL,
			appointment_id INT,
			amount INT NOT NULL DEFAULT 0,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS receptionists (
			receptionist_id SERIAL PRIMARY KEY,
			user_id INT NOT NULL UNIQUE,
			first_name VARCHAR(255),
			last_name VARCHAR(255),
			phone VARCHAR(20),
			department_id INT,
			shift VARCHAR(50),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
			FOREIGN KEY (department_id) REFERENCES departments(department_id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pharmacists (
			pharmacist_id SERIAL PRIMARY KEY,
			user_id INT NOT NULL UNIQUE,
			first_name VARCHAR(255),
			last_name VARCHAR(255),
			license_number VARCHAR(100),
			phone VARCHAR(20),
			qualification VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS prescriptions (
			prescription_id SERIAL PRIMARY KEY,
			patient_id INT NOT NULL,
			doctor_id INT,
			appointment_id INT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (patient_id) REFERENCES patients(patient_id) ON DELETE CASCADE,
			FOREIGN KEY (doctor_id) REFERENCES doctors(doctor_id) ON DELETE SET NULL,
			FOREIGN KEY (appointment_id) REFERENCES appointments(appointment_id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS medicines (
			medicine_id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			category VARCHAR(100),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS prescription_items (
			item_id SERIAL PRIMARY KEY,
			prescription_id INT NOT NULL,
			medicine_id INT,
			dosage VARCHAR(100),
			frequency VARCHAR(100),
			duration_days INT,
			instructions TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (prescription_id) REFERENCES prescriptions(prescription_id) ON DELETE CASCADE,
			FOREIGN KEY (medicine_id) REFERENCES medicines(medicine_id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS lab_tests_catalog (
			test_id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			category VARCHAR(100),
			price INT DEFAULT 0,
			description TEXT,
			turnaround_time VARCHAR(100),
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE pharmacy_items ADD COLUMN IF NOT EXISTS medicine_name VARCHAR(255)`,
		`ALTER TABLE pharmacy_items ADD COLUMN IF NOT EXISTS quantity INT DEFAULT 0`,
		`ALTER TABLE pharmacy_items ADD COLUMN IF NOT EXISTS unit_price INT DEFAULT 0`,
		`ALTER TABLE pharmacy_items ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE prescription_items ADD COLUMN IF NOT EXISTS quantity INT DEFAULT 1`,
		`ALTER TABLE medicines ADD COLUMN IF NOT EXISTS unit_price INT DEFAULT 0`,
		`ALTER TABLE doctors ADD COLUMN IF NOT EXISTS consultation_fee INT DEFAULT 500`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS patient_id INT`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS appointment_id INT`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS test_name VARCHAR(255)`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS result TEXT`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'ordered'`,
		`ALTER TABLE lab_reports ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS patient_id INT`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS appointment_id INT`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS amount INT DEFAULT 0`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'pending'`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS description TEXT`,
		`ALTER TABLE billing ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS appointment_time TIME`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS reason TEXT`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'scheduled'`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS diagnosis TEXT`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS symptoms TEXT`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS treatment TEXT`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS doctor_notes TEXT`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS prescription_json TEXT DEFAULT '[]'`,
		`ALTER TABLE appointments ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE patients ADD COLUMN IF NOT EXISTS blood_type VARCHAR(10)`,
		`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'lab_reports' AND column_name = 'report_id'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'lab_reports' AND column_name = 'lab_report_id'
			) THEN
				EXECUTE 'ALTER TABLE lab_reports RENAME COLUMN report_id TO lab_report_id';
			END IF;
		END $$`,
		`UPDATE lab_reports lr
		 SET appointment_id = (
		 	SELECT a2.appointment_id
		 	FROM appointments a2
		 	WHERE a2.patient_id = lr.patient_id
		 	ORDER BY
		 		CASE
		 			WHEN lower(COALESCE(a2.status, '')) IN ('completed', 'confirmed', 'accepted', 'approved', 'in_progress', 'in progress', 'scheduled') THEN 0
		 			ELSE 1
		 		END,
		 		a2.appointment_date DESC,
		 		a2.appointment_time DESC NULLS LAST,
		 		a2.appointment_id DESC
		 	LIMIT 1
		 )
		 WHERE (lr.appointment_id IS NULL OR lr.appointment_id <= 0)
		   AND EXISTS (
		 	SELECT 1
		 	FROM appointments a3
		 	WHERE a3.patient_id = lr.patient_id
		   )`,
		`CREATE INDEX IF NOT EXISTS idx_appointments_patient_id ON appointments(patient_id)`,
		`CREATE INDEX IF NOT EXISTS idx_appointments_doctor_id ON appointments(doctor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_appointments_status ON appointments(status)`,
		`CREATE INDEX IF NOT EXISTS idx_pharmacy_items_name ON pharmacy_items(medicine_name)`,
		`CREATE INDEX IF NOT EXISTS idx_lab_reports_patient_id ON lab_reports(patient_id)`,
		`CREATE INDEX IF NOT EXISTS idx_lab_reports_status ON lab_reports(status)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_patient_id ON billing(patient_id)`,
		`CREATE INDEX IF NOT EXISTS idx_billing_status ON billing(status)`,
		`CREATE INDEX IF NOT EXISTS idx_receptionists_user_id ON receptionists(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_receptionists_department_id ON receptionists(department_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pharmacists_user_id ON pharmacists(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prescriptions_patient_id ON prescriptions(patient_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prescriptions_doctor_id ON prescriptions(doctor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prescriptions_appointment_id ON prescriptions(appointment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_prescription_items_prescription_id ON prescription_items(prescription_id)`,
		`CREATE INDEX IF NOT EXISTS idx_medicines_name ON medicines(name)`,
		`CREATE INDEX IF NOT EXISTS idx_lab_tests_catalog_name ON lab_tests_catalog(name)`,
		`CREATE INDEX IF NOT EXISTS idx_patients_blood_type ON patients(blood_type)`,
	}

	for _, stmt := range statements {
		if _, err := DB.Exec(context.Background(), stmt); err != nil {
			return err
		}
	}

	return nil
}

func ConnectDB() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("❌ Error loading .env file")
	}

	connStr := os.Getenv("DB_URL")

	DB, err = pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Database connection failed:", err)
	}

	if err = DB.Ping(context.Background()); err != nil {
		log.Fatal("❌ Database ping failed:", err)
	}

	if err = ensureFeatureTables(); err != nil {
		log.Fatal("❌ Failed to initialize pharmacy/lab tables:", err)
	}

	fmt.Println("✅ Database connected")
}
