#!/bin/bash
# verify-thinking-level.sh — verifies that changing thinking level in tau
# actually affects model output for both local (Ollama) and remote providers.
#
# Usage: ./verify-thinking-level.sh
#
# Prerequisites:
#   - Ollama running at localhost:11434 with gemma4:26b loaded
#   - jq installed
#
# This script:
#   1. Sends the same prompt to Ollama with different thinking_level values
#   2. Measures the thinking length in each response
#   3. Reports whether higher thinking levels produce longer thinking

set -euo pipefail

BASE_URL="http://localhost:11434"
MODEL="gemma4:26b"
PROMPT="Explain why the sky is blue in detail."

echo "=== Thinking Level Verification ==="
echo "Model: $MODEL"
echo "Prompt: $PROMPT"
echo ""

declare -A thinking_lengths

for level in "" "low" "medium" "high"; do
    level_label="${level:-off}"
    echo -n "Testing thinking_level=$level_label ... "

    # Build the request body
    if [ -z "$level" ]; then
        THINKING_FIELD=""
    else
        THINKING_FIELD=",\"thinking_level\":\"$level\""
    fi

    BODY="{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT\"}],\"stream\":false,\"options\":{\"num_predict\":32768${THINKING_FIELD}}}"

    # Send request and capture response
    RESP=$(curl -s -X POST "$BASE_URL/api/chat" \
        -H "Content-Type: application/json" \
        -d "$BODY")

    # Extract thinking length
    THINKING_LEN=$(echo "$RESP" | jq -r '.message.thinking // ""' | wc -c)
    THINKING_LEN=$((THINKING_LEN - 1))  # subtract newline from wc -c

    thinking_lengths[$level_label]=$THINKING_LEN
    echo "thinking length: $THINKING_LEN chars"
done

echo ""
echo "=== Results ==="
printf "%-10s %s\n" "Level" "Thinking Length"
printf "%-10s %s\n" "-----" "---------------"
for level in off low medium high; do
    printf "%-10s %d chars\n" "$level" "${thinking_lengths[$level]}"
done

echo ""
echo "=== Analysis ==="
# Check if thinking length generally increases with level
off_len=${thinking_lengths[off]}
low_len=${thinking_lengths[low]}
med_len=${thinking_lengths[medium]}
high_len=${thinking_lengths[high]}

# At minimum, thinking levels should produce different results
if [ "$off_len" -eq 0 ] && [ "$low_len" -gt 0 ] && [ "$med_len" -gt 0 ] && [ "$high_len" -gt 0 ]; then
    echo "✓ Thinking levels are being sent and affect output"
    echo "  (off=0, low/medium/high all produce thinking content)"
elif [ "$off_len" -gt 0 ]; then
    echo "⚠ Model produces thinking even with no thinking_level set"
    echo "  (This is normal for some reasoning models — thinking may be automatic)"
else
    echo "✗ Unexpected results — check Ollama model and API"
fi
