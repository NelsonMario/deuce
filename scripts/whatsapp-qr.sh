#!/usr/bin/env bash
# Open the WhatsApp pairing QR code for the Deuce bot.
#
#   ./scripts/whatsapp-qr.sh <api-token> [base-url]
#   TOKEN=<api-token> ./scripts/whatsapp-qr.sh [base-url]
#
# The token is host.token from POST /api/v1/clubs. base-url defaults to
# http://localhost:8080 (the backend, not the website). Scan the code on the
# bot phone: WhatsApp -> Settings -> Linked devices -> Link a device. Codes
# expire after about 20 seconds; run the script again if the scan fails.
set -euo pipefail

if [[ $# -ge 1 && "$1" != http* ]]; then
  TOKEN="$1"
  shift
fi
BASE_URL="${1:-https://apideuce.sonneil.space}"
BASE_URL="${BASE_URL%/}"
if [[ -z "${TOKEN:-}" ]]; then
  echo "Usage: $0 <api-token> [base-url]" >&2
  exit 1
fi
OUT="${TMPDIR:-/tmp}/whatsapp-qr.png"

json_field() {
  python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))" 2>/dev/null || true
}

for attempt in 1 2 3 4 5; do
  if ! body=$(curl -sS -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/v1/whatsapp/qr"); then
    echo "Could not reach $BASE_URL. Is the backend running?" >&2
    exit 1
  fi
  status=$(printf '%s' "$body" | json_field status)

  case "$status" in
    PAIRED)
      echo "The bot is already paired."
      exit 0
      ;;
    READY)
      code=$(printf '%s' "$body" | json_field qr)
      npx -y qrcode@1.5.4 -o "$OUT" "$code" >/dev/null
      open "$OUT" 2>/dev/null || xdg-open "$OUT" 2>/dev/null || echo "QR code saved to $OUT"
      echo "Scan it on the bot phone: WhatsApp -> Settings -> Linked devices -> Link a device"
      exit 0
      ;;
    WAITING)
      echo "Waiting for a pairing code... ($attempt/5)"
      sleep 3
      ;;
    DISABLED)
      echo "The bot is disabled (WHATSAPP_ENABLED=false)." >&2
      exit 1
      ;;
    *)
      echo "Unexpected response from $BASE_URL: $body" >&2
      echo "Check the token, and that the URL is the backend (not the website)." >&2
      exit 1
      ;;
  esac
done

echo "No pairing code yet. Run the script again in a few seconds." >&2
exit 1
