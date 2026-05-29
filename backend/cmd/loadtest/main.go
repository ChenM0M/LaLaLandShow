package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type config struct {
	baseURL       string
	mode          string
	requests      int
	concurrency   int
	adminUser     string
	adminPassword string
	studentStart  int
	restoreAfter  bool
	timeout       time.Duration
}

type result struct {
	status int
	dur    time.Duration
	err    error
}

type summary struct {
	Requests    int               `json:"requests"`
	Concurrency int               `json:"concurrency"`
	Errors      int               `json:"errors"`
	Statuses    map[int]int       `json:"statuses"`
	Latency     map[string]string `json:"latency"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.baseURL, "base-url", "http://127.0.0.1:8080", "backend base URL")
	flag.StringVar(&cfg.mode, "mode", "health", "load mode: health or booking")
	flag.IntVar(&cfg.requests, "requests", 100, "number of requests or booking attempts")
	flag.IntVar(&cfg.concurrency, "concurrency", 20, "maximum concurrent workers")
	flag.StringVar(&cfg.adminUser, "admin-user", "admin", "admin username for booking mode")
	flag.StringVar(&cfg.adminPassword, "admin-password", "change-me-admin", "admin password for booking mode")
	flag.IntVar(&cfg.studentStart, "student-start", 9000000, "first synthetic seven-digit student id")
	flag.BoolVar(&cfg.restoreAfter, "restore-after", true, "restore pre-test snapshot after booking mode")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Second, "HTTP client timeout")
	flag.Parse()
	if cfg.requests < 1 {
		cfg.requests = 1
	}
	if cfg.concurrency < 1 {
		cfg.concurrency = 1
	}
	return cfg
}

func run(ctx context.Context, cfg config) error {
	client := &http.Client{Timeout: cfg.timeout}
	started := time.Now().UTC()
	var results []result
	var err error
	switch cfg.mode {
	case "health":
		results = runHealth(ctx, client, cfg)
	case "booking":
		results, err = runBooking(ctx, client, cfg)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown mode %q", cfg.mode)
	}
	finished := time.Now().UTC()
	report := summarize(results, cfg, started, finished)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runHealth(ctx context.Context, client *http.Client, cfg config) []result {
	return runConcurrent(cfg.requests, cfg.concurrency, func(_ int) result {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.baseURL+"/healthz", nil)
		if err != nil {
			return result{dur: time.Since(start), err: err}
		}
		resp, err := client.Do(req)
		if err != nil {
			return result{dur: time.Since(start), err: err}
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return result{status: resp.StatusCode, dur: time.Since(start)}
	})
}

func runBooking(ctx context.Context, client *http.Client, cfg config) ([]result, error) {
	adminToken, err := adminLogin(ctx, client, cfg)
	if err != nil {
		return nil, err
	}
	var backup []byte
	if cfg.restoreAfter {
		backup, err = adminGet(ctx, client, cfg, adminToken, "/api/admin/backup/export")
		if err != nil {
			return nil, fmt.Errorf("export pre-test backup: %w", err)
		}
		defer func() {
			if err := restoreBackup(context.Background(), client, cfg, adminToken, backup); err != nil {
				fmt.Fprintf(os.Stderr, "restore-after failed: %v\n", err)
			}
		}()
	}

	seatIDs, err := freeSeatIDs(ctx, client, cfg, adminToken)
	if err != nil {
		return nil, err
	}
	if len(seatIDs) == 0 {
		return nil, fmt.Errorf("no free seats available")
	}

	return runConcurrent(cfg.requests, cfg.concurrency, func(i int) result {
		start := time.Now()
		studentID := fmt.Sprintf("%07d", cfg.studentStart+i)
		token, status, err := studentLogin(ctx, client, cfg, studentID)
		if err != nil {
			return result{status: status, dur: time.Since(start), err: err}
		}
		status, err = bookSeat(ctx, client, cfg, token, studentID, seatIDs[i%len(seatIDs)])
		return result{status: status, dur: time.Since(start), err: err}
	}), nil
}

func runConcurrent(total, concurrency int, fn func(int) result) []result {
	results := make([]result, total)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = fn(idx)
			}
		}()
	}
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func adminLogin(ctx context.Context, client *http.Client, cfg config) (string, error) {
	body := map[string]string{"username": cfg.adminUser, "password": cfg.adminPassword}
	raw, status, err := postJSON(ctx, client, cfg.baseURL+"/api/admin/login", "", body)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("admin login status %d: %s", status, string(raw))
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	return parsed.Token, nil
}

func adminGet(ctx context.Context, client *http.Client, cfg config, token, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s status %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw, nil
}

func freeSeatIDs(ctx context.Context, client *http.Client, cfg config, token string) ([]int64, error) {
	raw, err := adminGet(ctx, client, cfg, token, "/api/admin/seats")
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Seats []struct {
			SeatID    int64           `json:"seat_id"`
			IsBlocked bool            `json:"is_blocked"`
			Booking   json.RawMessage `json:"booking"`
		} `json:"seats"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	seatIDs := []int64{}
	for _, seat := range parsed.Seats {
		if !seat.IsBlocked && (len(seat.Booking) == 0 || bytes.Equal(seat.Booking, []byte("null"))) {
			seatIDs = append(seatIDs, seat.SeatID)
		}
	}
	return seatIDs, nil
}

func studentLogin(ctx context.Context, client *http.Client, cfg config, studentID string) (string, int, error) {
	raw, status, err := postJSON(ctx, client, cfg.baseURL+"/api/student/login", "", map[string]string{"student_id": studentID})
	if err != nil {
		return "", status, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", status, fmt.Errorf("student login status %d: %s", status, string(raw))
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", status, err
	}
	return parsed.Token, status, nil
}

func bookSeat(ctx context.Context, client *http.Client, cfg config, token, studentID string, seatID int64) (int, error) {
	body := []map[string]any{{"seat_id": seatID, "student_id": studentID}}
	raw, status, err := postJSON(ctx, client, cfg.baseURL+"/api/seats/book", token, body)
	if err != nil {
		return status, err
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusConflict && status != http.StatusTooManyRequests {
		return status, fmt.Errorf("book status %d: %s", status, string(raw))
	}
	return status, nil
}

func restoreBackup(ctx context.Context, client *http.Client, cfg config, token string, snapshot []byte) error {
	planReq := append([]byte(`{"snapshot":`), snapshot...)
	planReq = append(planReq, '}')
	raw, status, err := postRawJSON(ctx, client, cfg.baseURL+"/api/admin/backup/plan", token, planReq)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("restore plan status %d: %s", status, string(raw))
	}
	var plan struct {
		Token string `json:"confirmation_token"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		return err
	}
	restoreReq := append([]byte(`{"snapshot":`), snapshot...)
	restoreReq = append(restoreReq, []byte(`,"confirmation_token":`)...)
	tokenJSON, _ := json.Marshal(plan.Token)
	restoreReq = append(restoreReq, tokenJSON...)
	restoreReq = append(restoreReq, '}')
	raw, status, err = postRawJSON(ctx, client, cfg.baseURL+"/api/admin/backup/restore", token, restoreReq)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("restore status %d: %s", status, string(raw))
	}
	return nil
}

func postJSON(ctx context.Context, client *http.Client, url, token string, body any) ([]byte, int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	return postRawJSON(ctx, client, url, token, raw)
}

func postRawJSON(ctx context.Context, client *http.Client, url, token string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

func summarize(results []result, cfg config, started, finished time.Time) summary {
	statuses := map[int]int{}
	errors := 0
	durations := make([]time.Duration, 0, len(results))
	for _, result := range results {
		if result.err != nil {
			errors++
		}
		statuses[result.status]++
		durations = append(durations, result.dur)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return summary{
		Requests:    len(results),
		Concurrency: cfg.concurrency,
		Errors:      errors,
		Statuses:    statuses,
		Latency: map[string]string{
			"p50": quantile(durations, 0.50).String(),
			"p95": quantile(durations, 0.95).String(),
			"p99": quantile(durations, 0.99).String(),
			"max": quantile(durations, 1.00).String(),
		},
		StartedAt:  started,
		FinishedAt: finished,
	}
}

func quantile(values []time.Duration, q float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	idx := int(float64(len(values)-1) * q)
	return values[idx]
}
