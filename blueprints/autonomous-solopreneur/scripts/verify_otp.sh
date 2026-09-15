#!/usr/bin/env bash
# verify_otp.sh — Fetch latest OTP / verification code from Octarq inbound emails

set -euo pipefail

OCTARQ_URL="${OCTARQ_URL:-http://localhost:8080}"
OCTARQ_API_TOKEN="${OCTARQ_API_TOKEN:-}"

if [[ -z "$OCTARQ_API_TOKEN" ]]; then
  echo "Error: OCTARQ_API_TOKEN is not set."
  echo "Usage: OCTARQ_API_TOKEN=oct_... ./verify_otp.sh [mailbox_id]"
  exit 1
fi

MAILBOX_ID="${1:-}"
QUERY_PARAM=""
if [[ -n "$MAILBOX_ID" ]]; then
  QUERY_PARAM="?mailbox_id=${MAILBOX_ID}&limit=3"
else
  QUERY_PARAM="?limit=3"
fi

echo "🔍 Checking newest inbound emails on ${OCTARQ_URL}..."

RESP=$(curl -s -H "Authorization: Bearer ${OCTARQ_API_TOKEN}" "${OCTARQ_URL}/api/mail/inbox${QUERY_PARAM}")

if [[ -z "$RESP" || "$RESP" == "[]" ]]; then
  echo "No recent emails found in mailbox."
  exit 0
fi

echo "📬 Recent transactional emails:"
echo "$RESP" | grep -E '"(subject|from_addr|received_at)"' || echo "$RESP"

echo ""
echo "💡 Tip: Feed this response directly to Claude Code or Cursor with prompt: 'Extract OTP code'"
