#!/usr/bin/env bash
# Hook PostToolUse — formata o ficheiro Go que acabou de ser editado.
#
# Recebe o evento em JSON no stdin. Sai sempre com 0: formatação nunca deve
# bloquear o fluxo. Ficheiros não-Go e eventos sem caminho são ignorados, para
# que o hook seja inofensivo em pedidos que só leem ou pesquisam.

set -uo pipefail

payload="$(cat)"

# O nome do campo varia entre ferramentas: file_path, filePath, path.
file="$(printf '%s' "$payload" \
  | grep -o '"file_\{0,1\}[Pp]ath"[[:space:]]*:[[:space:]]*"[^"]*"' \
  | head -n 1 \
  | sed 's/.*:[[:space:]]*"//; s/"$//')"

[ -n "$file" ] || exit 0

case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac

[ -f "$file" ] || exit 0

if command -v gofumpt >/dev/null 2>&1; then
  gofumpt -w "$file" >/dev/null 2>&1 || true
elif command -v gofmt >/dev/null 2>&1; then
  gofmt -w "$file" >/dev/null 2>&1 || true
fi

exit 0
