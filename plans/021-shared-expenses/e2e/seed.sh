#!/usr/bin/env bash
set -euo pipefail
B=http://localhost:8080/api/v1
j() { curl -sf -X "$1" "$B$2" -H "Content-Type: application/json" ${TOKEN:+-H "Authorization: Bearer $TOKEN"} ${3:+-d "$3"}; }
j POST /auth/register '{"email":"demo@kuberan.app","password":"Password123!","first_name":"Demo","last_name":"User"}' >/dev/null || true
TOKEN=$(j POST /auth/login '{"email":"demo@kuberan.app","password":"Password123!"}' | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d.get("access_token") or d["tokens"]["access_token"])')
export TOKEN
BANK=$(j POST /accounts/cash '{"name":"Maybank Savings","currency":"MYR","initial_balance":500000}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["account"]["id"])')
j POST /accounts/credit-card '{"name":"CIMB Visa","currency":"MYR","credit_limit":1000000,"interest_rate":18}' >/dev/null
FOOD=$(j POST /categories '{"name":"Food & Dining","type":"expense","color":"#F97316","icon":"🍔"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["category"]["id"])')
TRAVEL=$(j POST /categories '{"name":"Travel","type":"expense","color":"#0EA5E9","icon":"✈️"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["category"]["id"])')
SAL=$(j POST /categories '{"name":"Salary","type":"income","color":"#22C55E","icon":"💼"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["category"]["id"])')
j POST /budgets "{\"category_id\":\"$FOOD\",\"name\":\"Food budget\",\"amount\":80000,\"period\":\"monthly\"}" >/dev/null
TODAY=$(date -u +%Y-%m-%dT10:00:00Z)
j POST /transactions "{\"account_id\":\"$BANK\",\"category_id\":\"$SAL\",\"type\":\"income\",\"amount\":650000,\"description\":\"October salary\",\"date\":\"$(date -u +%Y-%m-01T09:00:00Z)\"}" >/dev/null
j POST /transactions "{\"account_id\":\"$BANK\",\"category_id\":\"$FOOD\",\"type\":\"expense\",\"amount\":4550,\"description\":\"Groceries\",\"date\":\"$(date -u -d '-3 day' +%Y-%m-%dT12:00:00Z)\"}" >/dev/null
echo "BANK=$BANK FOOD=$FOOD TRAVEL=$TRAVEL"
