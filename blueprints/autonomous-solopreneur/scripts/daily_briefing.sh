#!/usr/bin/env bash
# daily_briefing.sh — Autonomous Solopreneur Daily Briefing Runner
# Queries Octarq instance and outputs a daily digest of links, emails, and domain health.

set -euo pipefail

OCTARQ_URL="${OCTARQ_URL:-http://localhost:8080}"
OCTARQ_API_TOKEN="${OCTARQ_API_TOKEN:-}"

if [[ -z "$OCTARQ_API_TOKEN" ]]; then
  echo "Error: OCTARQ_API_TOKEN is not set."
  echo "Usage: OCTARQ_API_TOKEN=oct_... ./daily_briefing.sh"
  exit 1
fi

echo "========================================================"
echo " ☕ Octarq Autonomous Solopreneur Daily Briefing"
echo " Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo " Host: ${OCTARQ_URL}"
echo "========================================================"

echo ""
echo "--- 🔗 Link Traffic Summary ---"
curl -s -H "Authorization: Bearer ${OCTARQ_API_TOKEN}" \
     "${OCTARQ_URL}/api/links?limit=5" | \
     grep -o '"slug":"[^"]*"' || echo "No recent links found."

echo ""
echo "--- 📬 Inbound Email Summary ---"
curl -s -H "Authorization: Bearer ${OCTARQ_API_TOKEN}" \
     "${OCTARQ_URL}/api/mail/inbox?limit=5" | \
     grep -o '"subject":"[^"]*"' || echo "No new emails."

echo ""
echo "--- 🌐 Domain Health ---"
curl -s -H "Authorization: Bearer ${OCTARQ_API_TOKEN}" \
     "${OCTARQ_URL}/api/domains" | \
     grep -o '"domain":"[^"]*"' || echo "No custom domains configured."

echo ""
echo "========================================================"
echo " Briefing complete. Have a productive day!"
echo "========================================================"
