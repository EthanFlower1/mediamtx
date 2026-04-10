#!/usr/bin/env bash
# KAI-247: Hardware validation script for Kaivue NVR appliances.
#
# Checks CPU cores, RAM, disk, and network against tier thresholds.
# Outputs a JSON report to stdout. Exit 0 if meets minimum (mini tier),
# exit 1 if insufficient.
#
# Usage: scripts/hw-validate.sh [recordings-path]

set -euo pipefail

RECORDINGS_PATH="${1:-.}"

# --- Gather hardware info ---

CPU_CORES=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 0)
CPU_ARCH=$(uname -m)
OS=$(uname -s)

# RAM in bytes
case "$OS" in
  Linux)
    TOTAL_RAM=$(awk '/MemTotal/ {print $2 * 1024}' /proc/meminfo 2>/dev/null || echo 0)
    ;;
  Darwin)
    TOTAL_RAM=$(sysctl -n hw.memsize 2>/dev/null || echo 0)
    ;;
  *)
    TOTAL_RAM=0
    ;;
esac

# Free disk in bytes
case "$OS" in
  Linux)
    FREE_DISK=$(df -B1 --output=avail "$RECORDINGS_PATH" 2>/dev/null | tail -1 | tr -d ' ' || echo 0)
    ;;
  Darwin)
    FREE_DISK=$(df -k "$RECORDINGS_PATH" 2>/dev/null | tail -1 | awk '{print $4 * 1024}' || echo 0)
    ;;
  *)
    FREE_DISK=0
    ;;
esac

# Network interfaces (non-loopback, up)
if command -v ip &>/dev/null; then
  NET_IFS=$(ip -o link show up | grep -v LOOPBACK | awk -F': ' '{print $2}' | tr '\n' ',' | sed 's/,$//')
else
  NET_IFS=$(ifconfig 2>/dev/null | grep -E '^[a-z]' | grep -v lo | awk -F: '{print $1}' | tr '\n' ',' | sed 's/,$//')
fi
NET_COUNT=$(echo "$NET_IFS" | tr ',' '\n' | grep -c . 2>/dev/null || echo 0)

# GPU detection (best effort)
GPU_DETECTED=false
if command -v nvidia-smi &>/dev/null; then
  GPU_DETECTED=true
fi

# --- Classify tier ---

# Thresholds (bytes)
MINI_RAM=$((4 * 1024 * 1024 * 1024))
MINI_DISK=$((100 * 1024 * 1024 * 1024))
MINI_CPU=2

MID_RAM=$((8 * 1024 * 1024 * 1024))
MID_DISK=$((500 * 1024 * 1024 * 1024))
MID_CPU=4

ENT_RAM=$((16 * 1024 * 1024 * 1024))
ENT_DISK=$((2 * 1024 * 1024 * 1024 * 1024))
ENT_CPU=8

TIER="insufficient"
if [ "$CPU_CORES" -ge "$ENT_CPU" ] && [ "$TOTAL_RAM" -ge "$ENT_RAM" ] && [ "$FREE_DISK" -ge "$ENT_DISK" ]; then
  TIER="enterprise"
elif [ "$CPU_CORES" -ge "$MID_CPU" ] && [ "$TOTAL_RAM" -ge "$MID_RAM" ] && [ "$FREE_DISK" -ge "$MID_DISK" ]; then
  TIER="mid"
elif [ "$CPU_CORES" -ge "$MINI_CPU" ] && [ "$TOTAL_RAM" -ge "$MINI_RAM" ] && [ "$FREE_DISK" -ge "$MINI_DISK" ]; then
  TIER="mini"
fi

# --- Output JSON ---

cat <<EOJSON
{
  "cpu_cores": $CPU_CORES,
  "cpu_arch": "$CPU_ARCH",
  "os": "$OS",
  "total_ram_bytes": $TOTAL_RAM,
  "free_disk_bytes": $FREE_DISK,
  "gpu_detected": $GPU_DETECTED,
  "network_interfaces": "$NET_IFS",
  "network_interface_count": $NET_COUNT,
  "tier": "$TIER",
  "meets_minimum": $([ "$TIER" != "insufficient" ] && echo true || echo false)
}
EOJSON

# Exit code
if [ "$TIER" = "insufficient" ]; then
  exit 1
fi
exit 0
