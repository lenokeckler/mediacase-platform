package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/lenokeckler/mediacase-platform/internal/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://media:media@localhost:5432/mediacase?sslmode=disable"
	}

	database, err := db.Connect(dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()

	rows, err := database.Query("SELECT id, operation, error_msg FROM jobs WHERE status='failed' LIMIT 10")
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	fmt.Println("Failed Jobs:")
	for rows.Next() {
		var id, op string
		var errMsg sql.NullString
		rows.Scan(&id, &op, &errMsg)
		fmt.Printf("ID: %s | OP: %s | Error: %s\n", id, op, errMsg.String)
	}
}
