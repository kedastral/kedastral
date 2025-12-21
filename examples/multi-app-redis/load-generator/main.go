package main

import (
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

type LoadPattern struct {
	Name        string
	Description string
	Calculate   func(t time.Time) int // Returns requests per minute
}

var patterns = map[string]LoadPattern{
	"constant": {
		Name:        "Constant",
		Description: "Steady load - 60 RPS",
		Calculate: func(t time.Time) int {
			return 60
		},
	},
	"hourly-spike": {
		Name:        "Half-Hour Spike",
		Description: "Spikes every 30 minutes at :00 and :30 - great for testing prediction",
		Calculate: func(t time.Time) int {
			minute := t.Minute()
			// Spike at :00 and :30
			if minute < 5 || (minute >= 30 && minute < 35) {
				return 200 // High load for first 5 minutes
			} else if minute < 10 || (minute >= 35 && minute < 40) {
				return 120 // Tapering off
			}
			return 30 // Baseline
		},
	},
	"business-hours": {
		Name:        "Business Hours",
		Description: "High during 9-5, low otherwise",
		Calculate: func(t time.Time) int {
			hour := t.Hour()
			if hour >= 9 && hour < 17 {
				// Business hours - add some sine wave variation
				variation := math.Sin(float64(hour-9) * math.Pi / 8)
				return int(100 + 50*variation)
			}
			return 20 // Off hours
		},
	},
	"sine-wave": {
		Name:        "Sine Wave",
		Description: "Smooth sine wave pattern - 2 hour period",
		Calculate: func(t time.Time) int {
			// 2 hour period
			minutes := float64(t.Hour()*60 + t.Minute())
			wave := math.Sin(minutes * math.Pi / 120)
			return int(80 + 60*wave) // Range: 20-140
		},
	},
	"double-peak": {
		Name:        "Double Peak",
		Description: "Morning and afternoon peaks (9am, 3pm)",
		Calculate: func(t time.Time) int {
			hour := t.Hour()
			minute := t.Minute()
			totalMinutes := hour*60 + minute

			// Morning peak at 9am (540 minutes)
			morningPeak := math.Exp(-math.Pow(float64(totalMinutes-540), 2) / 1800)
			// Afternoon peak at 3pm (900 minutes)
			afternoonPeak := math.Exp(-math.Pow(float64(totalMinutes-900), 2) / 1800)

			peak := math.Max(morningPeak, afternoonPeak)
			return int(40 + 120*peak) // Range: 40-160
		},
	},
}

func main() {
	target := os.Getenv("TARGET_URL")
	if target == "" {
		target = "http://simple-app:8080"
	}

	patternName := os.Getenv("PATTERN")
	if patternName == "" {
		patternName = "hourly-spike"
	}

	pattern, ok := patterns[patternName]
	if !ok {
		log.Printf("Unknown pattern: %s, using hourly-spike", patternName)
		pattern = patterns["hourly-spike"]
	}

	log.Printf("Starting load generator")
	log.Printf("Target: %s", target)
	log.Printf("Pattern: %s - %s", pattern.Name, pattern.Description)

	// Wait for target to be ready
	waitForTarget(target)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var requestCount, errorCount int
	var lastLog time.Time

	for range ticker.C {
		now := time.Now()
		rps := pattern.Calculate(now)

		// Send requests
		for i := 0; i < rps; i++ {
			go func() {
				endpoint := "/api/task"
				// Occasionally hit the heavy endpoint
				if time.Now().Unix()%10 == 0 {
					endpoint = "/api/heavy"
				}

				resp, err := client.Get(target + endpoint)
				if err != nil {
					errorCount++
					return
				}
				resp.Body.Close()
				requestCount++
			}()
		}

		// Log stats every 10 seconds
		if now.Sub(lastLog) >= 10*time.Second {
			log.Printf("[%s] RPS target: %d, Requests sent: %d, Errors: %d",
				now.Format("15:04:05"), rps, requestCount, errorCount)
			requestCount = 0
			errorCount = 0
			lastLog = now
		}
	}
}

func waitForTarget(target string) {
	log.Printf("Waiting for target %s to be ready...", target)
	client := &http.Client{Timeout: 2 * time.Second}

	for i := 0; i < 60; i++ {
		resp, err := client.Get(target + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			log.Printf("Target is ready!")
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}

	log.Printf("Warning: Target not ready after 2 minutes, proceeding anyway...")
}
