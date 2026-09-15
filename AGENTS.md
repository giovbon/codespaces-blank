# AGENTS.md

Instruções para agentes de IA neste repositório. **Leia `.github/copilot-instructions.md` primeiro** — contém as regras que não se negociam (âmbito de formatos, stack Go, dinheiro em `int64`, cartão como conta).

## Ponto de entrada

| Quero… | Ler |
| --- | --- |
| Entender o projeto | [`README.md`](README.md) — índice e resumo das decisões |
| Saber o que está decidido e porquê | [`docs/05-decisoes-adr.md`](docs/05-decisoes-adr.md) — índice de ADRs no topo |
| Importação, cartões, cobertura | [`docs/07-muitas-contas-e-cartoes.md`](docs/07-muitas-contas-e-cartoes.md) |
| Pipeline de importação | [`docs/04-motor-importacao-csv.md`](docs/04-motor-importacao-csv.md) |
| Esquema da base de dados | [`docs/03-modelo-de-dados.md`](docs/03-modelo-de-dados.md) |
| Operação e deploy | [`docs/06-operacao-arm64.md`](docs/06-operacao-arm64.md) |
| Economia de contexto e customizações | [`docs/08-otimizacao-de-contexto.md`](docs/08-otimizacao-de-contexto.md) |
| O que extrair do Actual e o que descartar | [`docs/09-escopo-vs-actual.md`](docs/09-escopo-vs-actual.md) — portão de escopo |
| Estrutura de pastas e ordem de construção | [`docs/10-construcao-e-estrutura.md`](docs/10-construcao-e-estrutura.md) — onde vive o quê |

`docs/99-fora-de-ambito-pdf.md` é **arquivo histórico**, não plano.

## Como trabalhar aqui

1. **Ler antes de escrever.** Um ADR existente responde à maioria das dúvidas de desenho.
2. **Ler só a secção necessária.** Os documentos são grandes de propósito (detalhe justificado); citar por ficheiro e secção em vez de reproduzir.
3. **Alteração estrutural → propor ADR**, no formato do `docs/05`.
4. **Nunca alterar `testdata/` para fazer um teste passar.**
5. **Não introduzir dependências** sem justificação escrita: a lista de dependências diretas deve caber numa página. Proibidos por decisão: cgo, ORMs, Node/npm, bibliotecas de PDF, OCR, IA em *runtime*.
6. **Comandos:** `go test ./...`, `go build ./...`, `golangci-lint run`, `golangci-lint fmt`. Não existe passo de *build* de frontend: o CSS é gerado por `make css`, com o binário *standalone* do Tailwind. `make check` corre tudo.

## Convenções de escrita

- Tudo em **português** (documentos, comentários, mensagens de erro, commits).
- Documentos: tabelas comparativas e diagramas Mermaid onde ajudam; alternativas rejeitadas são tão importantes como a decisão.
- Código: identificadores em inglês, comentários em português.
