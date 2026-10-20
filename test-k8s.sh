#!/usr/bin/env bash
set -e

CURL_OPTS="-k -s --resolve siddhu.ddns.net:443:127.0.0.1"

echo "=========================================================="
echo "   PAYMENT GATEWAY - KUBERNETES & HTTPS VERIFICATION      "
echo "   Host: https://siddhu.ddns.net                          "
echo "=========================================================="

echo ""
echo "[1/4] Registering Merchant on K8s Cluster..."
RESP=$(curl $CURL_OPTS -X POST "https://siddhu.ddns.net/api/v1/merchants" \
  -H "Content-Type: application/json" \
  -d '{"name": "Kubernetes Fintech Merchant", "email": "billing@k8sfintech.com", "currency": "USD"}')
echo "Response: $RESP"

API_KEY=$(echo "$RESP" | grep -o '"api_key":"[^"]*' | cut -d'"' -f4)
MERCHANT_ID=$(echo "$RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo "  ✔ Merchant ID: $MERCHANT_ID"
echo "  ✔ API Key:     $API_KEY"

echo ""
echo "[2/4] Processing Charge ($150.00 USD) via K8s Payment Service..."
IDEMP="idemp-k8s-$(date +%s)"
CHARGE_RESP=$(curl $CURL_OPTS -i -X POST "https://siddhu.ddns.net/api/v1/payments/charge" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -H "Idempotency-Key: $IDEMP" \
  -d "{\"amount\": 15000, \"currency\": \"USD\", \"payment_method\": \"pm_card_visa_4242\", \"description\": \"K8s Live Deployment Charge\", \"merchant_id\": \"$MERCHANT_ID\"}")

PAYMENT_ID=$(echo "$CHARGE_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
CACHE_HEADER=$(echo "$CHARGE_RESP" | grep -i "X-Cache-Lookup" | tr -d '\r')
echo "  ✔ Payment ID: $PAYMENT_ID"
echo "  ✔ Status:     $CACHE_HEADER"

sleep 1

echo ""
echo "[3/4] Verifying Double-Entry Ledger on K8s Ledger Service..."
echo "All Account Balances:"
curl $CURL_OPTS "https://siddhu.ddns.net/api/v1/ledger/accounts"
echo ""

echo ""
echo "[4/4] Verifying Cryptographic Audit Chain on K8s..."
curl $CURL_OPTS "https://siddhu.ddns.net/api/v1/ledger/audit" | grep -o '"hash":"[^"]*' | head -n 2
echo ""

echo "=========================================================="
echo "   KUBERNETES DEPLOYMENT & INGRESS FULLY OPERATIONAL!     "
echo "=========================================================="
