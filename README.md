# Finanças Pessoais Self-Hosted — Arquitetura de Software

Definição de arquitetura para um aplicativo de finanças pessoais com **paridade funcional essencial ao [Actual Budget](https://github.com/actualbudget/actual)** e um **motor de importação CSV de alta automação** como diferencial competitivo.

**Restrições de projeto:**

- Executa *full-time* num host **ARM64** (ex.: Raspberry Pi 5, Ampere, Hetzner ARM, mini-PC ARM).
- Acesso **exclusivamente pelo navegador** (sem app desktop, sem app nativo).
- Uso pessoal / familiar (poucos utilizadores, um único operador de dados).

## Documentos

| Documento | Conteúdo |
| --- | --- |
| [docs/01-arquitetura.md](docs/01-arquitetura.md) | Contexto, drivers, estilo arquitetural, camadas, módulos, fluxos, roadmap |
| [docs/02-stack.md](docs/02-stack.md) | Linguagens, bibliotecas e justificativa, com notas de compatibilidade ARM64 |
| [docs/03-modelo-de-dados.md](docs/03-modelo-de-dados.md) | Esquema SQL completo, invariantes, índices, estratégia de deduplicação |
| [docs/04-motor-importacao-csv.md](docs/04-motor-importacao-csv.md) | Pipeline de importação em 7 estágios, deteção automática, dedupe, aprendizagem |
| [docs/05-decisoes-adr.md](docs/05-decisoes-adr.md) | 11 ADRs com contexto, alternativas avaliadas e consequências |
| [docs/06-operacao-arm64.md](docs/06-operacao-arm64.md) | Deploy Docker multi-arch, TLS, backup, segurança, monitorização, plano de restauro |

## Resumo das decisões estruturantes

1. **Monólito modular *server-centric*** — o servidor ARM64 é a fonte de verdade; o navegador é uma SPA fina, mas com o *core* de domínio partilhado (`packages/*`) para pré-visualizar importações sem ir ao servidor.
2. **SQLite em modo WAL** como base de dados única — zero administração, ideal para ARM64 e 1 utilizador; caminho de migração para PostgreSQL definido e isolado no repositório de dados.
3. **TypeScript ponta a ponta** com *schemas* Zod partilhados em `packages/contracts` — uma só definição de tipos e validação para browser e servidor.
4. **Importação em *staging* com pré-visualização obrigatória** — nenhuma linha de CSV entra na base de dados sem um *diff* aprovável e reversível (`undo` de lote inteiro).
5. **Aprendizagem incremental de perfis por banco/conta** — o sistema memoriza mapeamento de colunas, formato de data/decimal e categorização de estabelecimentos, reduzindo o trabalho manual a zero a partir da segunda importação.

## O que deliberadamente **não** fazemos

Micro-serviços, Kubernetes, Redis, PostgreSQL, GraphQL, app Electron, motor de planilha completo, *machine learning* no MVP. Cada um destes é registado como não-objetivo com justificativa em [docs/01-arquitetura.md](docs/01-arquitetura.md#11-não-objetivos).

## Nota sobre licenciamento

O Actual Budget é distribuído sob licença **MIT**. Reimplementar funcionalidades é livre; reutilizar código-fonte obriga a preservar o aviso de copyright e a atribuição. A decisão registada (ADR-010) é **reimplementar com referência funcional**, não fazer *fork*.
