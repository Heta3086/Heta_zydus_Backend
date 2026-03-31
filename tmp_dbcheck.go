//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	db, err := pgxpool.New(context.Background(), "postgres://postgres:admin@localhost:5432/Hospital")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	today := time.Now().Format("2006-01-02")
	fmt.Println("Today:", today)

	rows, err := db.Query(context.Background(), `
		SELECT a.appointment_id,
		       a.doctor_id,
		       COALESCE(u.name, CONCAT(COALESCE(p.first_name, ''), ' ', COALESCE(p.last_name, ''))) AS patient_name,
		       COALESCE(a.appointment_date::text, ''),
		       COALESCE(a.appointment_time::text, ''),
		       COALESCE(a.status, '')
		FROM appointments a
		LEFT JOIN patients p ON p.patient_id = a.patient_id
		LEFT JOIN users u ON u.user_id = p.user_id
		ORDER BY a.appointment_id DESC
		LIMIT 25`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	fmt.Println("Last 25 appointments:")
	for rows.Next() {
		var id, doctorID int
		var patientName, date, slot, status string
		if err := rows.Scan(&id, &doctorID, &patientName, &date, &slot, &status); err != nil {
			panic(err)
		}
		fmt.Printf("id=%d doctor=%d patient=%q date=%q time=%q status=%q\n", id, doctorID, patientName, date, slot, status)
	}

	rows2, err := db.Query(context.Background(), `
		SELECT COALESCE(status, '') AS status, COUNT(*)
		FROM appointments
		WHERE appointment_date = CURRENT_DATE
		GROUP BY status
		ORDER BY status`)
	if err != nil {
		panic(err)
	}
	defer rows2.Close()

	fmt.Println("Today status summary:")
	for rows2.Next() {
		var status string
		var count int
		if err := rows2.Scan(&status, &count); err != nil {
			panic(err)
		}
		fmt.Printf("status=%q count=%d\n", status, count)
	}

	rows3, err := db.Query(context.Background(), `
		SELECT lr.lab_report_id,
		       lr.patient_id,
		       COALESCE(lr.appointment_id, 0) AS appointment_id,
		       COALESCE(lr.test_name, ''),
		       COALESCE(lr.status, ''),
		       COALESCE(u.name, ''),
		       COALESCE(du.name, ''),
		       COALESCE(du2.name, '')
		FROM lab_reports lr
		LEFT JOIN patients p ON p.patient_id = lr.patient_id
		LEFT JOIN users u ON u.user_id = p.user_id
		LEFT JOIN appointments a ON a.appointment_id = lr.appointment_id
		LEFT JOIN doctors d ON d.doctor_id = a.doctor_id
		LEFT JOIN users du ON du.user_id = d.user_id
		LEFT JOIN LATERAL (
			SELECT a2.doctor_id
			FROM appointments a2
			WHERE a2.patient_id = lr.patient_id
			ORDER BY a2.appointment_date DESC, a2.appointment_time DESC, a2.appointment_id DESC
			LIMIT 1
		) latest_doctor ON TRUE
		LEFT JOIN doctors d2 ON d2.doctor_id = latest_doctor.doctor_id
		LEFT JOIN users du2 ON du2.user_id = d2.user_id
		ORDER BY lr.lab_report_id DESC
		LIMIT 25`)
	if err != nil {
		panic(err)
	}
	defer rows3.Close()

	fmt.Println("Last 25 lab reports with doctor mapping:")
	for rows3.Next() {
		var labID, patientID, appointmentID int
		var testName, status, patientName, doctorViaAppointment, doctorViaLatest string
		if err := rows3.Scan(&labID, &patientID, &appointmentID, &testName, &status, &patientName, &doctorViaAppointment, &doctorViaLatest); err != nil {
			panic(err)
		}
		fmt.Printf("lab=%d patient_id=%d patient=%q appointment_id=%d test=%q status=%q doctor_by_appointment=%q doctor_by_latest=%q\n",
			labID, patientID, patientName, appointmentID, testName, status, doctorViaAppointment, doctorViaLatest)
	}
}
