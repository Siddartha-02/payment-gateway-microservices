package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type Merchant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Currency  string    `json:"currency"`
	APIKey    string    `json:"api_key"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // active, suspended
}

type CreateMerchantRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Currency string `json:"currency"`
}

type VerifyKeyRequest struct {
	APIKey string `json:"api_key"`
}

type Store struct {
	mu        sync.RWMutex
	merchants map[string]Merchant
	byAPIKey  map[string]string // apiKey -> merchantID
}

var store = &Store{
	merchants: make(map[string]Merchant),
	byAPIKey:  make(map[string]string),
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
		"service":     "merchant-service",
		"version":     "1.0.0",
		"status":      "running",
		"description": "Merchant Identity & API Key Management",
		"endpoints": []string{
			"GET  /health",
			"POST /api/v1/merchants",
			"GET  /api/v1/merchants",
			"GET  /api/v1/merchants/{id}",
			"POST /api/v1/merchants/verify",
		},
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service":   "merchant-service",
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func createMerchantHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateMerchantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Email == "" {
		http.Error(w, "name and email are required", http.StatusBadRequest)
		return
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	m := Merchant{
		ID:        "mch_" + generateRandomHex(8),
		Name:      req.Name,
		Email:     req.Email,
		Currency:  req.Currency,
		APIKey:    "sk_live_" + generateRandomHex(16),
		PublicKey: "pk_live_" + generateRandomHex(12),
		CreatedAt: time.Now().UTC(),
		Status:    "active",
	}

	store.mu.Lock()
	store.merchants[m.ID] = m
	store.byAPIKey[m.APIKey] = m.ID
	store.mu.Unlock()

	log.Printf("[MerchantService] Created merchant: %s (%s)", m.ID, m.Name)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(m)
}

func getMerchantHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store.mu.RLock()
	m, exists := store.merchants[id]
	store.mu.RUnlock()

	if !exists {
		http.Error(w, "merchant not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func verifyKeyHandler(w http.ResponseWriter, r *http.Request) {
	var req VerifyKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	store.mu.RLock()
	merchantID, exists := store.byAPIKey[req.APIKey]
	var m Merchant
	if exists {
		m = store.merchants[merchantID]
	}
	store.mu.RUnlock()

	if !exists || m.Status != "active" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "invalid or inactive API key"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"valid":       true,
		"merchant_id": m.ID,
		"currency":    m.Currency,
	})
}

func listMerchantsHandler(w http.ResponseWriter, r *http.Request) {
	store.mu.RLock()
	list := make([]Merchant, 0, len(store.merchants))
	for _, m := range store.merchants {
		list = append(list, m)
	}
	store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", rootHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /api/v1/merchants", createMerchantHandler)
	mux.HandleFunc("GET /api/v1/merchants", listMerchantsHandler)
	mux.HandleFunc("GET /api/v1/merchants/{id}", getMerchantHandler)
	mux.HandleFunc("POST /api/v1/merchants/verify", verifyKeyHandler)

	log.Printf("[MerchantService] Listening on port :%s ...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
