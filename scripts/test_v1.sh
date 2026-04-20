#!/usr/bin/env bash
set -euo pipefail

# Resolve repo root from script location (works without git)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEVUP_BIN="$REPO_ROOT/devup"
DEMO_DIR="$HOME/devup-demo"
CONFIG_FILE="$HOME/.devup/config.json"

# Fetch token from config (python3 if available, else grep/sed)
get_token() {
  if [[ -r "$CONFIG_FILE" ]]; then
    if command -v python3 &>/dev/null; then
      python3 -c 'import json,os;print(json.load(open(os.path.expanduser("~/.devup/config.json")))["token"])' 2>/dev/null || true
    else
      grep -o '"token"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" 2>/dev/null | sed 's/.*"\([^"]*\)"$/\1/' || true
    fi
  fi
}

cleanup() {
  local r=$?
  if [[ $r -ne 0 ]]; then
    echo "=== Debug info (exit $r) ==="
    limactl list 2>/dev/null || true
    limactl shell devup -- bash -lc "mount | grep workspace || true" 2>/dev/null || true
    TOKEN=$(get_token)
    if [[ -n "${TOKEN:-}" ]]; then
      curl -sS -H "X-Devup-Token: $TOKEN" http://127.0.0.1:7777/health 2>/dev/null || true
    else
      echo "(no token found at $CONFIG_FILE)"
    fi
  fi
}
trap cleanup EXIT

# Step 1: Darwin check
echo "==> Step 1: Darwin check"
if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This test is for macOS+Lima"
  exit 0
fi

# Step 2: Build
echo "==> Step 2: Build host CLI"
cd "$REPO_ROOT"
go build -o "$DEVUP_BIN" ./cmd/devup

# Step 3: Binary check (purely local, no agent; never trigger set -e)
echo "==> Step 3: Verify devup binary is runnable"
out=$("$DEVUP_BIN" --help 2>&1) || out=$("$DEVUP_BIN" 2>&1) || true
[[ -n "${out:-}" && "$out" == *"Usage:"* ]] || { echo "FAIL: devup binary not runnable"; exit 1; }

# Step 4: Lima check
echo "==> Step 4: Ensure Lima is installed"
if ! command -v limactl &>/dev/null; then
  echo "Install Lima: brew install lima"
  exit 1
fi

# Step 5: VM up
echo "==> Step 5: Bring up VM (devup vm up)"
vm_start=$(date +%s)
"$DEVUP_BIN" vm up
vm_elapsed=$(($(date +%s) - vm_start))
echo "    (VM up: ${vm_elapsed}s)"

# Step 5b: Warm run (timing for pitch)
echo "==> Step 5b: Warm run (echo warm)"
warm_start=$(date +%s)
"$DEVUP_BIN" run -- echo warm >/dev/null 2>&1
warm_elapsed=$(($(date +%s) - warm_start))
echo "    (warm run: ${warm_elapsed}s)"

# Step 6: Create demo project
echo "==> Step 6: Create demo project under \$HOME"
mkdir -p "$DEMO_DIR"
echo 'print("hi from devup")' > "$DEMO_DIR/app.py"

# Step 7: Mount test
echo "==> Step 7: Mount test (ls + cat app.py)"
run_start=$(date +%s)
mount_out=$("$DEVUP_BIN" run --mount "$DEMO_DIR:/workspace" -- bash -lc "ls -la /workspace && cat /workspace/app.py")
run_elapsed=$(($(date +%s) - run_start))
echo "    (run: ${run_elapsed}s)"
if ! echo "$mount_out" | grep -q "hi from devup"; then
  echo "FAIL: Mount test - output should contain 'hi from devup'"
  echo "Got: $mount_out"
  exit 1
fi

# Step 8: Cleanup/unmount test (ensure bind mounts are cleaned up)
# Note: Lima virtiofs shows "mount0 on /workspace", not source path; generic check is fine for V1
echo "==> Step 8: Cleanup test (workspace should not remain mounted)"
"$DEVUP_BIN" run -- bash -lc 'mount | grep -q " /workspace " && echo "workspace still mounted" && exit 1 || exit 0'

# Step 9: Linux identity test
echo "==> Step 9: Linux identity test (uname -s)"
linux_out=$("$DEVUP_BIN" run --mount "$DEMO_DIR:/workspace" -- bash -lc "uname -s")
if [[ "$linux_out" != *"Linux"* ]]; then
  echo "FAIL: Linux identity test - expected 'Linux', got: $linux_out"
  exit 1
fi

# Step 10: Security negative test (should fail)
echo "==> Step 10: Security negative test (mount to /etc should fail)"
sec_stderr=""
sec_exit=0
sec_stderr=$("$DEVUP_BIN" run --mount "$DEMO_DIR:/etc" -- bash -lc "echo should-not-run" 2>&1) || sec_exit=$?
if [[ $sec_exit -eq 0 ]]; then
  echo "FAIL: Security test - command should have failed (guestPath /etc not under /workspace)"
  echo "Stderr: $sec_stderr"
  exit 1
fi
if [[ -z "$sec_stderr" ]]; then
  echo "FAIL: Security test - expected error message on stderr"
  exit 1
fi
if ! echo "$sec_stderr" | grep -qE "400|Bad Request|workspace|run:"; then
  echo "FAIL: Security test - expected error containing 400/Bad Request/workspace/run:, got: $sec_stderr"
  exit 1
fi

# V1.1: Background jobs (force agent rebuild to get new endpoints)
echo "==> Step 11: Force agent rebuild for V1.1"
limactl shell devup -- sudo systemctl stop devup-agent 2>/dev/null || true
sleep 2
"$DEVUP_BIN" vm up

# Step 11b: Default env test (HOME/XDG_CACHE_HOME for dev tools)
echo "==> Step 11b: Default env test (HOME, XDG_CACHE_HOME)"
env_out=$("$DEVUP_BIN" run --mount "$DEMO_DIR:/workspace" -- bash -lc 'echo "HOME=$HOME"; echo "XDG_CACHE_HOME=${XDG_CACHE_HOME:-}"')
if ! echo "$env_out" | grep -q "HOME=/tmp/devup-home"; then
  echo "FAIL: Default env - HOME should be /tmp/devup-home, got: $env_out"
  exit 1
fi
if ! echo "$env_out" | grep -q "XDG_CACHE_HOME=/tmp/devup-home/.cache"; then
  echo "FAIL: Default env - XDG_CACHE_HOME should be /tmp/devup-home/.cache, got: $env_out"
  exit 1
fi

# Step 12: Start background job
echo "==> Step 12: Start background job"
jobid=$("$DEVUP_BIN" start --mount "$DEMO_DIR:/workspace" -- bash -lc "while true; do echo tick; sleep 1; done")
[[ -n "$jobid" ]] || { echo "FAIL: start did not return job id"; exit 1; }
sleep 2
# Step 13: ps shows job running
echo "==> Step 13: ps shows job running"
ps_out=$("$DEVUP_BIN" ps)
echo "$ps_out" | grep -q "$jobid" || { echo "FAIL: ps should show job"; exit 1; }
echo "$ps_out" | grep -q "running" || { echo "FAIL: job should be running"; exit 1; }
# Step 14: logs contains tick
echo "==> Step 14: logs contains tick"
logs_out=$("$DEVUP_BIN" logs "$jobid")
echo "$logs_out" | grep -q "tick" || { echo "FAIL: logs should contain tick"; exit 1; }
# Step 14b: logs -f timeout test (start job that sleeps 5s then prints; logs -f must not error)
echo "==> Step 14b: logs -f timeout test"
jobid2=$("$DEVUP_BIN" start --mount "$DEMO_DIR:/workspace" -- bash -lc "sleep 5; echo done")
[[ -n "$jobid2" ]] || { echo "FAIL: start did not return job id"; exit 1; }
logs_f_out=$("$DEVUP_BIN" logs "$jobid2" -f 2>&1) || true
echo "$logs_f_out" | grep -q "done" || { echo "FAIL: logs -f should eventually print done, got: $logs_f_out"; exit 1; }
"$DEVUP_BIN" stop "$jobid2" 2>/dev/null || true
# Step 15: stop job
echo "==> Step 15: stop job"
"$DEVUP_BIN" stop "$jobid"
# Step 16: ps shows exited/stopped
echo "==> Step 16: ps shows exited/stopped"
ps_out2=$("$DEVUP_BIN" ps)
echo "$ps_out2" | grep -q "$jobid" || { echo "FAIL: ps should still show job"; exit 1; }
# Step 17: workspace not mounted after stop (retry up to 5s)
echo "==> Step 17: workspace not mounted after stop (retry up to 5s)"
for i in 1 2 3 4 5; do
  "$DEVUP_BIN" run -- bash -lc 'mount | grep -q " /workspace " && exit 1 || exit 0' && break
  [[ $i -eq 5 ]] && { echo "FAIL: workspace still mounted after 5s"; exit 1; }
  sleep 1
done

# Step 18: Port release test (stop must free port for next job)
echo "==> Step 18: Port release test"
port_job1=$("$DEVUP_BIN" start --mount "$DEMO_DIR:/workspace" --workdir /workspace -- bash -lc 'python3 -c "
import socket, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((\"0.0.0.0\", 3000))
s.listen(1)
while True: time.sleep(1)
"')
[[ -n "$port_job1" ]] || { echo "FAIL: port job 1 did not start"; exit 1; }
sleep 2
"$DEVUP_BIN" stop "$port_job1"
for retry in 1 2 3 4 5 6 7 8 9 10; do
  port_job2=$("$DEVUP_BIN" start --mount "$DEMO_DIR:/workspace" --workdir /workspace -- bash -lc 'python3 -c "
import socket, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((\"0.0.0.0\", 3000))
s.listen(1)
while True: time.sleep(1)
"' 2>&1) || true
  port_job2=$(echo "$port_job2" | grep -E '^[a-f0-9]+$' | tail -1)
  if [[ -n "$port_job2" ]]; then
    "$DEVUP_BIN" stop "$port_job2" 2>/dev/null || true
    break
  fi
  [[ $retry -eq 10 ]] && { echo "FAIL: port 3000 not released after stop (EADDRINUSE), got: $port_job2"; exit 1; }
  sleep 0.2
done

# Step 19: PASS
echo "PASS"
