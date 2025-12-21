package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Metrics
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"endpoint", "method", "status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_request_duration_seconds",
			Help:    "API request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	activeConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "api_active_connections",
			Help: "Number of active connections",
		},
	)

	db *sql.DB
)

type Response struct {
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Pod       string `json:"pod"`
}

type TaskResponse struct {
	TaskID    int    `json:"task_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	// Database connection (optional - will work without DB too)
	dbHost := os.Getenv("DB_HOST")
	if dbHost != "" {
		var err error
		connStr := fmt.Sprintf("host=%s port=5432 user=postgres password=postgres dbname=testdb sslmode=disable",
			dbHost)
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Printf("Warning: Could not connect to database: %v", err)
		} else {
			defer db.Close()
			if err := db.Ping(); err != nil {
				log.Printf("Warning: Database ping failed: %v", err)
			} else {
				log.Println("Connected to PostgreSQL")
				initDB()
			}
		}
	}

	// Routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/task", handleTask)
	http.HandleFunc("/api/heavy", handleHeavy)
	http.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initDB() {
	query := `
		CREATE TABLE IF NOT EXISTS tasks (
			id SERIAL PRIMARY KEY,
			status VARCHAR(50),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := db.Exec(query); err != nil {
		log.Printf("Warning: Could not create table: %v", err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	activeConnections.Inc()
	defer activeConnections.Dec()

	podName := os.Getenv("HOSTNAME")

	resp := Response{
		Message:   "Hello from Kedastral test app!",
		Timestamp: time.Now().Format(time.RFC3339),
		Pod:       podName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	requestsTotal.WithLabelValues("/", r.Method, "200").Inc()
	requestDuration.WithLabelValues("/").Observe(time.Since(start).Seconds())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleTask(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	activeConnections.Inc()
	defer activeConnections.Dec()

	// Simulate some CPU work
	simulateCPUWork(50 * time.Millisecond)

	var resp TaskResponse

	if db != nil {
		// Insert task into DB
		err := db.QueryRow("INSERT INTO tasks (status) VALUES ($1) RETURNING id, status, created_at",
			"completed").Scan(&resp.TaskID, &resp.Status, &resp.CreatedAt)
		if err != nil {
			log.Printf("DB error: %v", err)
			requestsTotal.WithLabelValues("/api/task", r.Method, "500").Inc()
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		// No DB - just return mock data
		resp = TaskResponse{
			TaskID:    rand.Intn(10000),
			Status:    "completed",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	requestsTotal.WithLabelValues("/api/task", r.Method, "200").Inc()
	requestDuration.WithLabelValues("/api/task").Observe(time.Since(start).Seconds())
}

func handleHeavy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	activeConnections.Inc()
	defer activeConnections.Dec()

	// Simulate heavier CPU work
	simulateCPUWork(200 * time.Millisecond)

	podName := os.Getenv("HOSTNAME")
	resp := Response{
		Message:   "Heavy computation completed",
		Timestamp: time.Now().Format(time.RFC3339),
		Pod:       podName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	requestsTotal.WithLabelValues("/api/heavy", r.Method, "200").Inc()
	requestDuration.WithLabelValues("/api/heavy").Observe(time.Since(start).Seconds())
}

func simulateCPUWork(duration time.Duration) {
	end := time.Now().Add(duration)
	for time.Now().Before(end) {
		_ = rand.Float64() * rand.Float64()
	}
}
