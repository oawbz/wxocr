#!/usr/bin/env bash
set -euo pipefail
umask 077
APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="$APP_DIR/ocr-web.pid"
LOCK_DIR="$APP_DIR/.ocr-web.start.lock"
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  echo '另一个启动操作正在执行，请稍后重试。' >&2
  exit 1
fi
trap 'rmdir "$LOCK_DIR"' EXIT
[[ -x "$APP_DIR/ocr-web" && -f "$APP_DIR/config.yaml" ]] || { echo '缺少可执行程序或 config.yaml。' >&2;exit 1; }

is_our_process() {
  local pid="$1" executable command_line
  [[ "$pid" =~ ^[1-9][0-9]*$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  if [[ -d /proc ]]; then
    executable="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
    executable="${executable% (deleted)}"
    [[ "$executable" == "$APP_DIR/ocr-web" ]] || return 1
    command_line="$(tr '\0' '\n' < "/proc/$pid/cmdline" 2>/dev/null || true)"
    [[ "$command_line" == "$APP_DIR/ocr-web"$'\n'"-config"$'\n'"$APP_DIR/config.yaml" ]]
  else
    command_line="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    [[ "$command_line" == "$APP_DIR/ocr-web -config $APP_DIR/config.yaml" ]]
  fi
}

# Recover processes started by the previous foreground-only script.
if [[ ! -f "$PID_FILE" ]]; then
  if [[ -d /proc ]]; then
    for entry in /proc/[0-9]*/exe; do
      existing_pid="${entry#/proc/}";existing_pid="${existing_pid%/exe}"
      if is_our_process "$existing_pid"; then printf '%s\n' "$existing_pid" > "$PID_FILE";break;fi
    done
  else
    while read -r existing_pid command_line; do
      if [[ "$command_line" == "$APP_DIR/ocr-web -config $APP_DIR/config.yaml" ]]; then
        printf '%s\n' "$existing_pid" > "$PID_FILE";break
      fi
    done < <(ps -axo pid=,command= 2>/dev/null || true)
  fi
fi

if [[ -f "$PID_FILE" ]]; then
  old_pid="$(cat "$PID_FILE")"
  if is_our_process "$old_pid"; then
    echo "正在停止旧实例（PID $old_pid）…"
    kill -TERM "$old_pid"
    # Wait for native inference and graceful shutdown; do not force-kill it.
    for ((i=0; i<120; i++)); do
      is_our_process "$old_pid" || break
      sleep 1
    done
    if is_our_process "$old_pid"; then
      echo '旧实例尚未退出，未启动新实例，请稍后重试。' >&2
      exit 1
    fi
  fi
  rm -f "$PID_FILE"
fi

nohup "$APP_DIR/ocr-web" -config "$APP_DIR/config.yaml" </dev/null >>"$APP_DIR/ocr-web.log" 2>&1 &
pid=$!
printf '%s\n' "$pid" > "$PID_FILE"
# Catch immediate configuration, model or listen failures before reporting success.
sleep 2
if ! kill -0 "$pid" 2>/dev/null; then
  wait "$pid" 2>/dev/null || true
  rm -f "$PID_FILE"
  echo '启动失败，请查看 ocr-web.log。' >&2
  exit 1
fi
echo "OCR 服务已在后台启动（PID $pid）。再次运行此脚本即可重启。"
