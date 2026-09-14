#!/usr/bin/env bash
# Hook Stop — verificação rápida quando a sessão do agente termina.
#
# Faz o que é barato e determinístico: confirma que formatação e compilação
# estão limpas. NÃO corre os testes completos — isso é responsabilidade do
# agente, que deve interpretar o diff em vez de delegar em automação cega.
#
# Sai sempre com 0. O objetivo é informar, não bloquear.

set -uo pipefail

cat >/dev/null 2>&1 || true

# Sem código Go no repositório, não há nada a verificar.
if ! find . -name '*.go' -not -path './vendor/*' -print -quit 2>/dev/null | grep -q .; then
  exit 0
fi

problems=""

if command -v gofumpt >/dev/null 2>&1; then
  unformatted="$(gofumpt -l . 2>/dev/null | head -n 20)"
  [ -n "$unformatted" ] && problems="${problems}Formatacao pendente (correr: gofumpt -l -w .):
${unformatted}
"
fi

if command -v go >/dev/null 2>&1; then
  if ! build_out="$(go build ./... 2>&1)"; then
    problems="${problems}go build falhou:
${build_out}
"
  fi
fi

if [ -n "$problems" ]; then
  printf '%s' "$problems"
fi

exit 0
