package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

type Posting struct {
	AccountID string    `json:"account_id"`
	Direction Direction `json:"direction"`
	Amount    int64     `json:"amount"` // cents
}

type JournalEntry struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	Description   string    `json:"description"`
	Postings      []Posting `json:"postings"`
	PrevHash      string    `json:"prev_hash"`
	Hash          string    `json:"hash"` // SHA-256 tamper-evident chain
	Timestamp     time.Time `json:"timestamp"`
}

type ProcessEntryRequest struct {
	TransactionID string `json:"transaction_id"`
	MerchantID    string `json:"merchant_id"`
	Amount        int64  `json:"amount"` // in cents
	Currency      string `json:"currency"`
	Type          string `json:"type"` // "PAYMENT" or "REFUND"
}

type LedgerStore struct {
	mu       sync.RWMutex
	entries  []JournalEntry
	balances map[string]int64 // accountID -> current balance (cents)
	lastHash string
}

var store = &LedgerStore{
	entries:  make([]JournalEntry, 0),
	balances: make(map[string]int64),
	lastHash: "0000000000000000000000000000000000000000000000000000000000000000",
}

func calculateHash(prevHash string, entryID string, postings []Posting, ts time.Time) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte(entryID))
	h.Write([]byte(ts.Format(time.RFC3339Nano)))
	for _, p := range postings {
		h.Write([]byte(fmt.Sprintf("%s:%s:%d", p.AccountID, p.Direction, p.Amount)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service":     "ledger-service",
		"version":     "1.0.0",
		"status":      "running",
		"description": "Double-Entry Financial Accounting & Cryptographic Audit Ledger",
		"endpoints": []string{
			"GET  /health",
			"POST /api/v1/ledger/entries",
			"GET  /api/v1/ledger/accounts/{id}/balance",
			"GET  /api/v1/ledger/accounts",
			"GET  /api/v1/ledger/audit",
		},
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"service":   "ledger-service",
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func recordEntryHandler(w http.ResponseWriter, r *http.Request) {
	var req ProcessEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, "amount must be positive", http.StatusBadRequest)
		return
	}

	// Calculate fee: standard 2.9% + 30c
	fee := (req.Amount*29)/1000 + 30
	if fee > req.Amount {
		fee = req.Amount
	}
	netMerchant := req.Amount - fee

	clearingAcc := "clearing:processor"
	merchantAcc := "merchant:" + req.MerchantID
	feeAcc := "platform:revenue"

	var postings []Posting
	description := ""

	if req.Type == "REFUND" {
		description = fmt.Sprintf("Refund for transaction %s", req.TransactionID)
		postings = []Posting{
			{AccountID: clearingAcc, Direction: Credit, Amount: req.Amount},
			{AccountID: merchantAcc, Direction: Debit, Amount: netMerchant},
			{AccountID: feeAcc, Direction: Debit, Amount: fee},
		}
	} else {
		// Standard Payment
		description = fmt.Sprintf("Settlement for payment %s", req.TransactionID)
		postings = []Posting{
			{AccountID: clearingAcc, Direction: Debit, Amount: req.Amount},
			{AccountID: merchantAcc, Direction: Credit, Amount: netMerchant},
			{AccountID: feeAcc, Direction: Credit, Amount: fee},
		}
	}

	// Validate double-entry balance: sum(debits) must equal sum(credits)
	var totalDebits, totalCredits int64
	for _, p := range postings {
		if p.Direction == Debit {
			totalDebits += p.Amount
		} else {
			totalCredits += p.Amount
		}
	}

	if totalDebits != totalCredits {
		http.Error(w, fmt.Sprintf("unbalanced entry: debits(%d) != credits(%d)", totalDebits, totalCredits), http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	store.mu.Lock()
	defer store.mu.Unlock()

	entryID := fmt.Sprintf("jnl_%d", len(store.entries)+1)
	entryHash := calculateHash(store.lastHash, entryID, postings, now)

	entry := JournalEntry{
		ID:            entryID,
		TransactionID: req.TransactionID,
		Description:   description,
		Postings:      postings,
		PrevHash:      store.lastHash,
		Hash:          entryHash,
		Timestamp:     now,
	}

	// Update account balances
	for _, p := range postings {
		if p.Direction == Credit {
			store.balances[p.AccountID] += p.Amount
		} else {
			store.balances[p.AccountID] -= p.Amount
		}
	}

	store.entries = append(store.entries, entry)
	store.lastHash = entryHash

	log.Printf("[LedgerService] Created journal entry %s (hash: %s..)", entry.ID, entryHash[:12])
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	store.mu.RLock()
	balance, exists := store.balances[accountID]
	store.mu.RUnlock()

	if !exists {
		balance = 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"account_id":       accountID,
		"balance_cents":    balance,
		"balance_formatted": fmt.Sprintf("$%.2f", float64(balance)/100.0),
		"as_of":            time.Now().UTC().Format(time.RFC3339),
	})
}

func listAccountsHandler(w http.ResponseWriter, r *http.Request) {
	store.mu.RLock()
	copyBalances := make(map[string]map[string]any)
	for acc, bal := range store.balances {
		copyBalances[acc] = map[string]any{
			"balance_cents":     bal,
			"balance_formatted": fmt.Sprintf("$%.2f", float64(bal)/100.0),
		}
	}
	store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(copyBalances)
}

func auditHandler(w http.ResponseWriter, r *http.Request) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total_entries": len(store.entries),
		"latest_hash":   store.lastHash,
		"entries":       store.entries,
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", rootHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /api/v1/ledger/entries", recordEntryHandler)
	mux.HandleFunc("GET /api/v1/ledger/accounts/{id}/balance", getBalanceHandler)
	mux.HandleFunc("GET /api/v1/ledger/accounts", listAccountsHandler)
	mux.HandleFunc("GET /api/v1/ledger/audit", auditHandler)

	log.Printf("[LedgerService] Listening on port :%s ...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
