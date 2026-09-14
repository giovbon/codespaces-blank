---
description: "Convenções para editar a documentação de arquitetura em docs/. Use when: escrever ou editar ADR, atualizar secção de arquitetura, stack, modelo de dados, importação ou operação."
applyTo: "docs/**/*.md"
---

# Convenções da documentação

Diretório `docs/`. Índice e resumo das decisões em [`README.md`](../../README.md) — **atualizar sempre que se acrescenta ou renomeia um documento**.

## Estrutura dos documentos

| Ficheiro | Âmbito |
| --- | --- |
| `01-arquitetura.md` | Contexto, drivers, estilo, camadas, módulos, fluxos, roadmap, riscos, não-objetivos |
| `02-stack.md` | Linguagem e bibliotecas, com alternativas rejeitadas |
| `03-modelo-de-dados.md` | SQL completo, invariantes, índices, deduplicação |
| `04-motor-importacao-csv.md` | Estágios 2 a 7 do pipeline (comuns a todos os formatos) |
| `05-decisoes-adr.md` | ADRs |
| `06-operacao-arm64.md` | Deploy, backup, segurança, runbook |
| `07-muitas-contas-e-cartoes.md` | Logística de muitas contas e cartões |
| `08-otimizacao-de-contexto.md` | Economia de contexto para agentes de IA (contém o **inventário** das customizações) |
| `99-fora-de-ambito-pdf.md` | **Arquivo histórico.** Não é plano; não o atualizar como se fosse |

## ADRs

Formato obrigatório: **Contexto → Decisão → Alternativas avaliadas → Consequências**.

- As alternativas rejeitadas são tão importantes como a decisão: cada uma leva **o motivo concreto** da rejeição, não «não é adequado».
- Nunca reescrever a decisão de um ADR existente. **Criar um AO novo ou assinalar a revisão** com uma nota no topo do ADR e uma linha no registo de revisões, indicando data e ADR que a motivou.
- Toda a entrada nova entra na **tabela de índice do topo** de `05-decisoes-adr.md`, com estado (`Válido`, `Revisto`, `Novo`, `Estendido`).

## Estilo

- **Português** (variante europeia, consistente com o resto dos documentos).
- Tabelas comparativas para escolhas; diagramas Mermaid para fluxos e arquitetura.
- *Itálico* para termos técnicos em inglês (`*staging*`, `*matching*`, `*undo*`).
- Código, identificadores e comandos em `` ` ``.
- Datas em `YYYY-MM-DD` e decisões datadas quando relevantes.
- Explicar o **porquê**, não o quê. O quê lê-se no código.

## Âmbito

- **Formatos suportados: CSV, OFX, CAMT.053.** PDF está fora de âmbito (ADR-016) — não reintroduzir secções de PDF, OCR, `poppler`, editor de colunas ou `layout_json`.
- Não documentar funcionalidade que não está decidida. Se algo é hipótese, marcar explicitamente como tal ou ir para `99-*`.

## Verificação antes de terminar

1. Ligações relativas entre documentos válidas (e *anchors* corretos).
2. Índice do `README.md` atualizado.
3. Nenhuma contradição com um ADR vigente — se houver, é sinal de que falta um ADR de revisão.
4. Índice do topo de `05-decisoes-adr.md` e registo de revisões atualizados, no mesmo *pull request*.
5. Documentos **grandes** (> ~200 linhas) começam por um **índice navegável** — é o que permite decidir sem abrir o ficheiro ([ADR-017](../docs/05-decisoes-adr.md)). Obrigatório em documentos novos; nos existentes que ainda não têm, acrescentar quando forem editados (não em retrocompatibilidade forçada).
6. Ficheiros de customização em `.github/` criados ou renomeados → atualizar o inventário em [`docs/08` §2](../docs/08-otimizacao-de-contexto.md).
