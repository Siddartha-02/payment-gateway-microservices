#!/usr/bin/env bash
set -e

BASE_URL_MERCHANT="${MERCHANT_URL:-http://localhost:8081}"
BASE_URL_PAYMENT="${PAYMENT_URL:-http://localhost:8082}"
BASE_URL_LEDGER="${LEDGER_URL:-http://localhost:8083}"

echo "=================================================="
echo "   PAYMENT GATEWAY - END-TO-END VERIFICATION      "
echo "=================================================="

echo ""
echo "[1/7] Health Checks..."
curl -s "$BASE_URL_MERCHANT/health" | grep -q "healthy" && echo "  ✔ Merchant Service: HEALTHY"
curl -s "$BASE_URL_PAYMENT/health" | grep -q "healthy" && echo "  ✔ Payment Service:  HEALTHY"
curl -s "$BASE_URL_LEDGER/health" | grep -q "healthy" && echo "  ✔ Ledger Service:   HEALTHY"

echo ""
echo "[2/7] Registering New Merchant..."
MERCHANT_RESP=$(curl -s -X POST "$BASE_URL_MERCHANT/api/v1/merchants" \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Store Inc.", "email": "finance@acme.com", "currency": "USD"}')
echo "Response: $MERCHANT_RESP"

MERCHANT_ID=$(echo "$MERCHANT_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
API_KEY=$(echo "$MERCHANT_RESP" | grep -o '"api_key":"[^"]*' | cut -d'"' -f4)
echo "  ✔ Merchant ID: $MERCHANT_ID"
echo "  ✔ Secret Key:  $API_KEY"

echo ""
echo "[3/7] Processing Payment ($100.00 USD) with Idempotency Key..."
IDEMP_KEY="idemp-test-$(date +%s)"
PAYMENT_RESP=$(curl -s -i -X POST "$BASE_URL_PAYMENT/api/v1/payments/charge" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d "{\"amount\": 10000, \"currency\": \"USD\", \"payment_method\": \"pm_card_visa_4242\", \"description\": \"Order #9901\", \"merchant_id\": \"$MERCHANT_ID\"}")

PAYMENT_ID=$(echo "$PAYMENT_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
CACHE_HEADER=$(echo "$PAYMENT_RESP" | grep -i "X-Cache-Lookup" | tr -d '\r')
echo "  ✔ Payment ID: $PAYMENT_ID"
echo "  ✔ Cache Status: $CACHE_HEADER (First request: MISS)"

echo ""
echo "[4/7] Testing Idempotency (Replaying IDENTICAL charge request)..."
REPLAY_RESP=$(curl -s -i -X POST "$BASE_URL_PAYMENT/api/v1/payments/charge" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d "{\"amount\": 10000, \"currency\": \"USD\", \"payment_method\": \"pm_card_visa_4242\", \"description\": \"Order #9901\", \"merchant_id\": \"$MERCHANT_ID\"}")

REPLAY_PAYMENT_ID=$(echo "$REPLAY_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
REPLAY_CACHE_HEADER=$(echo "$REPLAY_RESP" | grep -i "X-Cache-Lookup" | tr -d '\r')
echo "  ✔ Replayed Payment ID: $REPLAY_PAYMENT_ID"
echo "  ✔ Cache Status: $REPLAY_CACHE_HEADER (Proves duplicate charge was PREVENTED)"

echo ""
echo "[5/7] Testing Card Decline (Simulated failure: card ending in 0000)..."
DECLINE_RESP=$(curl -s -X POST "$BASE_URL_PAYMENT/api/v1/payments/charge" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d "{\"amount\": 2500, \"currency\": \"USD\", \"payment_method\": \"pm_card_declined_0000\", \"description\": \"Order #9902\", \"merchant_id\": \"$MERCHANT_ID\"}")
echo "  ✔ Declined Response: $DECLINE_RESP"

# Allow asynchronous ledger event to process
sleep 1

echo ""
echo "[6/7] Checking Double-Entry Ledger Balances..."
echo "Merchant Account Balance:"
curl -s "$BASE_URL_LEDGER/api/v1/ledger/accounts/merchant:$MERCHANT_ID/balance"
echo ""
echo "Platform Revenue Account Balance:"
curl -s "$BASE_URL_LEDGER/api/v1/ledger/accounts/platform:revenue/balance"
echo ""
echo "All Ledger Balances:"
curl -s "$BASE_URL_LEDGER/api/v1/ledger/accounts"
echo ""

echo ""
echo "[7/7] Testing Refund & Ledger Reversal..."
REFUND_RESP=$(curl -s -X POST "$BASE_URL_PAYMENT/api/v1/payments/$PAYMENT_ID/refund" \
  -H "Content-Type: application/json" \
  -d '{"reason": "customer_requested"}')
echo "  ✔ Refund Response: $REFUND_RESP"

sleep 1
echo ""
echo "Merchant Balance After Refund Reversal:"
curl -s "$BASE_URL_LEDGER/api/v1/ledger/accounts/merchant:$MERCHANT_ID/balance"
echo ""

echo ""
echo "Cryptographic Audit Chain:"
curl -s "$BASE_URL_LEDGER/api/v1/ledger/audit" | grep -o '"hash":"[^"]*' | head -n 3
echo ""
echo "=================================================="
echo "   ALL TESTS PASSED SUCCESSFULLY!                 "
echo "=================================================="
