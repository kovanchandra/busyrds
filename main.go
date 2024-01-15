package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	faker "github.com/bxcodec/faker/v3"
	_ "github.com/lib/pq"
)

var (
	dbPool *sql.DB
	config Config
	delay  float64
)

type Config struct {
	Database struct {
		DSN string `json:"dsn"`
	} `json:"database"`
	TestRun    int `json:"test_run"`
	RPS        int `json:"rps"`
	MaxRetry   int `json:"max_retry"`
	DelayRetry int `json:"delay_retry"`
}

func init() {

	err := error(nil)
	config, err = loadConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}

	delay = 1000 / float64(config.RPS)
	connectDB()
	fmt.Println("Successfully connected to the database")
}

func connectDB() {
	err := error(nil)

	dbPool, err = sql.Open("postgres", config.Database.DSN)
	if err != nil {
		log.Fatal(err)
	}
	err = dbPool.Ping()
	if err != nil {
		log.Fatal(err)
	}
}

func loadConfig(filename string) (Config, error) {
	var config Config
	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(data, &config)
	log.Printf("Config : %+v", config)
	return config, err
}

func main() {

	usecasePtr := flag.Int("usecase", 0, "a usecase number")

	// Parse the flags
	flag.Parse()

	// Print the flag
	fmt.Println("Usecase:", *usecasePtr)

	// Do something based on the usecase value
	// createDatabase(dbPool, "busy_db")
	// createTableIfNotExists(dbPool)
	switch *usecasePtr {
	case 1:
		log.Println("Dummy Data Generator")
		dataGenerator()
	case 2:
		log.Println("RDS Failover Simulator")
		failoverSimulator()
	case 99:
		log.Println("Create DB & Table")
		dbInitialization()
	default:
		fmt.Println("Unknown usecase selected.")
	}

}

func dataGenerator() {
	log.Println("==============")
	log.Println("Start Dummy Data Generator")
	startTime := time.Now()
	log.Println("Start time: ", startTime)
	for i := 0; i < config.TestRun; i++ {
		generateDummyData()
	}
	duration := time.Since(startTime)
	log.Printf("End time: %v \n", time.Now())
	log.Printf("Duration: %s\n", duration)
}

func failoverSimulator() {
	log.Println("==============")
	log.Printf("Start Write Data Simulation at %d RPS", config.RPS)
	startTime := time.Now()
	log.Println("Start time: ", startTime)
	for i := 0; i < config.TestRun; i++ {
		generateDummyData()
		time.Sleep(time.Duration(delay) * time.Millisecond)

	}
	duration := time.Since(startTime)
	log.Printf("End time: %v \n", time.Now())
	log.Printf("Duration: %s\n", duration)
}

func dbInitialization() {
	createDatabase(dbPool, "busy_db")
	createTableIfNotExists(dbPool)
}

func createDatabase(db *sql.DB, dbName string) {
	// First, check if the database exists
	rows, err := db.Query(fmt.Sprintf("SELECT datname FROM pg_catalog.pg_database WHERE lower(datname) = lower('%s')", dbName))
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	// If the database does not exist, create it
	if !rows.Next() {
		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Database %s created successfully\n", dbName)
	} else {
		fmt.Printf("Database %s already exists\n", dbName)
	}
}

func createTableIfNotExists(db *sql.DB) {

	stmt := `CREATE TABLE IF NOT EXISTS busy_table(
		id SERIAL PRIMARY KEY,
		description VARCHAR(255),
		status VARCHAR(50),
		time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := db.Exec(stmt)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Table busy is ready")
}

func generateDummyData() {
	stmt := `INSERT INTO busy_table (description, status) VALUES ($1, $2)`

	data := faker.Email()
	retryCount := 1
	lastError := time.Now()
	for {
		_, err := dbPool.Exec(stmt, data, "idle")
		if err != nil {
			if retryCount == 1 {
				lastError = time.Now()
			}
			if retryCount >= config.MaxRetry {
				log.Fatalf("Failed to insert data: %s. Error: [%v]\n", data, err)
			}

			log.Printf("Failed to insert: %s. Error: [%v]. Retrying (%d/%d)...\n", data, err, retryCount, config.MaxRetry)

			if err.Error() == "pq: cannot execute INSERT in a read-only transaction" {
				dbPool.Close()
				dbPool = nil
				connectDB()
			}

			retryCount++
			time.Sleep(time.Duration(config.DelayRetry) * time.Second)

		} else {
			if retryCount > 1 {
				downTime := time.Since(lastError).Milliseconds()
				retryCount = 1
				log.Printf("DownTime: %dms\n", downTime)
			}
			log.Printf("Insert: %s success\n", data)
			break
		}

	}

}

type BusyRow struct {
	ID          int
	Description string
	Status      string
	Time        string
}

func readBusyTable(db *sql.DB) {
	rows, err := db.Query("SELECT * FROM busy ORDER BY id desc limit 1")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var r BusyRow
		err = rows.Scan(&r.ID, &r.Description, &r.Status, &r.Time)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ID: %d, Description: %s, Status: %s, Time: %s\n", r.ID, r.Description, r.Status, r.Time)
	}

	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
}
