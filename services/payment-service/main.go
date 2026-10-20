package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type PaymentStatus string

const (
	StatusInitiated  PaymentStatus = "INITIATED"
	StatusAuthorized PaymentStatus = "AUTHORIZED"
	StatusCaptured   PaymentStatus = "CAPTURED"
	StatusFailed     PaymentStatus = "FAILED"
	StatusRefunded   PaymentStatus = "REFUNDED"
)

type Payment struct {
	ID             string        `json:"id"`
	MerchantID     string        `json:"merchant_id"`
	Amount         int64         `json:"amount"` // in smallest currency unit (e.g., cents)
	Currency       string        `json:"currency"`
	Status         PaymentStatus `json:"status"`
	PaymentMethod  string        `json:"payment_method"`
	Description    string        `json:"description"`
	IdempotencyKey string        `json:"idempotency_key"`
	FailureReason  string        `json:"failure_reason,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type ChargeRequest struct {
	Amount        int64  `json:"amount"` // cents
	Currency      string `json:"currency"`
	PaymentMethod string `json:"payment_method"` // e.g., "pm_card_valid" or "pm_card_declined_0000"
	Description   string `json:"description"`
	MerchantID    string `json:"merchant_id,omitempty"`
}

type RefundRequest struct {
	Reason string `json:"reason"`
}

type Store struct {
	mu           sync.RWMutex
	payments     map[string]*Payment
	idempotency  map[string]*Payment // idempotencyKey -> Payment
}

var store = &Store{
	payments:    make(map[string]*Payment),
	idempotency: make(map[string]*Payment),
}

var (
	merchantSvcURL = getEnv("MERCHANT_SERVICE_URL", "http://merchant-service:8081")
	ledgerSvcURL   = getEnv("LEDGER_SERVICE_URL", "http://ledger-service:8083")
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service": "payment-service",
		"version": "1.0.0",
		"status":  "running",
		"description": "Core Payment Processing Engine with Idempotency",
		"endpoints": []string{
			"GET  /health",
			"POST /api/v1/payments/charge",
			"GET  /api/v1/payments",
			"GET  /api/v1/payments/{id}",
			"POST /api/v1/payments/{id}/refund",
		},
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service":   "payment-service",
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// verifyMerchantWithAuthService calls merchant-service or falls back gracefully
func verifyMerchantWithAuthService(apiKey, merchantID string) (string, error) {
	if apiKey == "" && merchantID != "" {
		return merchantID, nil
	}

	payload, _ := json.Marshal(map[string]string{"api_key": apiKey})
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(merchantSvcURL+"/api/v1/merchants/verify", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[PaymentService] Warning: merchant service unreachable (%v), proceeding with merchant_id", err)
		if merchantID != "" {
			return merchantID, nil
		}
		return "mch_default", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid API key")
	}

	var res struct {
		Valid      bool   `json:"valid"`
		MerchantID string `json:"merchant_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || !res.Valid {
		return "", fmt.Errorf("merchant verification failed")
	}

	return res.MerchantID, nil
}

// notifyLedger records double-entry bookkeeping journal entries
func notifyLedger(payment *Payment, isRefund bool) {
	entryType := "PAYMENT"
	if isRefund {
		entryType = "REFUND"
	}

	payload, _ := json.Marshal(map[string]any{
		"transaction_id": payment.ID,
		"merchant_id":    payment.MerchantID,
		"amount":         payment.Amount,
		"currency":       payment.Currency,
		"type":           entryType,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})

	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Post(ledgerSvcURL+"/api/v1/ledger/entries", "application/json", bytes.NewBuffer(payload))
		if err != nil {
			log.Printf("[PaymentService] Ledger notification failed: %v", err)
			return
		}
		defer resp.Body.Close()
		log.Printf("[PaymentService] Ledger recorded entry for payment %s (status: %d)", payment.ID, resp.StatusCode)
	}()
}

func chargeHandler(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	apiKey := r.Header.Get("X-API-Key")

	var req ChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, "amount must be greater than zero", http.StatusBadRequest)
		return
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	// 1. Idempotency Check
	if idempotencyKey != "" {
		store.mu.RLock()
		existing, ok := store.idempotency[idempotencyKey]
		store.mu.RUnlock()
		if ok {
			log.Printf("[PaymentService] Idempotency cache hit for key: %s (Payment ID: %s)", idempotencyKey, existing.ID)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache-Lookup", "HIT")
			json.NewEncoder(w).Encode(existing)
			return
		}
	}

	// 2. Merchant Authentication
	merchantID, err := verifyMerchantWithAuthService(apiKey, req.MerchantID)
	if err != nil {
		http.Error(w, "authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// 3. Process Payment through simulated payment network
	now := time.Now().UTC()
	payment := &Payment{
		ID:             "pay_" + generateRandomHex(10),
		MerchantID:     merchantID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		PaymentMethod:  req.PaymentMethod,
		Description:    req.Description,
		IdempotencyKey: idempotencyKey,
		Status:         StatusInitiated,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Simulated Bank / Card Network Authorization
	// Rule: Cards ending in "0000" trigger a simulated decline
	if strings.HasSuffix(req.PaymentMethod, "0000") {
		payment.Status = StatusFailed
		payment.FailureReason = "card_declined_insufficient_funds"
		log.Printf("[PaymentService] Payment %s DECLINED by card network", payment.ID)
	} else {
		payment.Status = StatusCaptured
		log.Printf("[PaymentService] Payment %s CAPTURED successfully ($%.2f %s)", payment.ID, float64(payment.Amount)/100.0, payment.Currency)
		// Post-charge: asynchronous double-entry ledger event
		notifyLedger(payment, false)
	}

	// 4. Save to Store & Idempotency Cache
	store.mu.Lock()
	store.payments[payment.ID] = payment
	if idempotencyKey != "" {
		store.idempotency[idempotencyKey] = payment
	}
	store.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache-Lookup", "MISS")
	if payment.Status == StatusFailed {
		w.WriteHeader(http.StatusPaymentRequired)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(payment)
}

func getPaymentHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store.mu.RLock()
	p, ok := store.payments[id]
	store.mu.RUnlock()

	if !ok {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func refundHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store.mu.Lock()
	p, ok := store.payments[id]
	if !ok {
		store.mu.Unlock()
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	if p.Status != StatusCaptured {
		store.mu.Unlock()
		http.Error(w, fmt.Sprintf("cannot refund payment in %s state", p.Status), http.StatusBadRequest)
		return
	}

	p.Status = StatusRefunded
	p.UpdatedAt = time.Now().UTC()
	store.mu.Unlock()

	// Notify ledger to reverse transaction
	notifyLedger(p, true)

	log.Printf("[PaymentService] Payment %s REFUNDED", p.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func listPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	store.mu.RLock()
	list := make([]*Payment, 0, len(store.payments))
	for _, p := range store.payments {
		list = append(list, p)
	}
	store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", rootHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /api/v1/payments/charge", chargeHandler)
	mux.HandleFunc("GET /api/v1/payments", listPaymentsHandler)
	mux.HandleFunc("GET /api/v1/payments/{id}", getPaymentHandler)
	mux.HandleFunc("POST /api/v1/payments/{id}/refund", refundHandler)

	log.Printf("[PaymentService] Listening on port :%s ...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
