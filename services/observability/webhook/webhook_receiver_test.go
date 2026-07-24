package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookReceiver_ReceiveAlert(t *testing.T) {
	wr := NewWebhookReceiver()

	payload := AlertPayload{
		Receiver: "webhook",
		Status:   "firing",
		Alerts: []AlertMessage{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "NodeDown",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "Node worker-india-1 is down",
				},
				StartsAt: "2026-07-24T18:00:00Z",
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()

	wr.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	// Verify alert was stored
	reqGet := httptest.NewRequest("GET", "/alerts", nil)
	recGet := httptest.NewRecorder()
	wr.handleGetAlerts(recGet, reqGet)

	var alerts []AlertMessage
	if err := json.NewDecoder(recGet.Body).Decode(&alerts); err != nil {
		t.Fatalf("failed to decode alerts response: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("expected 1 stored alert, got %d", len(alerts))
	}

	if alerts[0].Labels["alertname"] != "NodeDown" {
		t.Errorf("expected alertname NodeDown, got %s", alerts[0].Labels["alertname"])
	}
}
