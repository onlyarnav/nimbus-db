package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
)

type AlertMessage struct {
	Status string `json:"status"`
	Labels map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt string `json:"startsAt"`
}

type AlertPayload struct {
	Receiver string `json:"receiver"`
	Status string `json:"status"`
	Alerts []AlertMessage `json:"alerts"`
}

type WebhookReceiver struct {
	mu sync.Mutex
	receivedAlerts []AlertMessage
}

func NewWebhookReceiver() *WebhookReceiver {
	return &WebhookReceiver{
		receivedAlerts: make([]AlertMessage, 0),
	}
}

func (wr *WebhookReceiver) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var payload AlertPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("failed to unmarshal alert payload", "error", err)
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	wr.mu.Lock()
	for _, a := range payload.Alerts {
		slog.Warn("ALERT RECEIVED",
			"status", a.Status,
			"alertname", a.Labels["alertname"],
			"severity", a.Labels["severity"],
			"summary", a.Annotations["summary"],
		)
		wr.receivedAlerts = append(wr.receivedAlerts, a)
	}
	wr.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

func (wr *WebhookReceiver) handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(wr.receivedAlerts)
}

func (wr *WebhookReceiver) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP", "service": "webhook-receiver"})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	wr := NewWebhookReceiver()

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", wr.handleWebhook)
	mux.HandleFunc("/alerts", wr.handleGetAlerts)
	mux.HandleFunc("/health", wr.handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9099"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	slog.Info("webhook receiver listening", "port", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("webhook receiver server error", "error", err)
	}
}
