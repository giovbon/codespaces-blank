# Finanças Pessoais Self-Hosted — Arquitetura de Software

Definição de arquitetura para um aplicativo de finanças pessoais com **paridade funcional essencial ao [Actual Budget](https://github.com/actualbudget/actual)** e um **motor de importação de alta automação** como diferencial.

O problema que motiva o projeto é concreto e pessoal: **ter muitas contas bancárias e vários cartões de crédito, e não gastar uma tarde por mês a alimentar o sistema.** O sistema tem de entregar três coisas que o Actual não entrega:

1. **Zero configuração por instituição** — biblioteca de perfis pronta, versionada no repositório.
2. **Zero dupla contagem** — o cartão é uma conta e o pagamento da fatura é emparelhado pelo valor exato declarado no documento.
3. **Zero ambiguidade de âmbito** — CSV é o núcleo, OFX e CAMT.053 entram quando existem. **PDF está fora de âmbito** (ADR-016); instituições que só emitem PDF usam o modo «apenas o total».

**Restrições de projeto:**

- Executa *full-time* num host **ARM64** (ex.: Raspberry Pi 5, Ampere, Hetzner ARM, mini-PC ARM).
- Acesso **exclusivamente pelo navegador** (sem app desktop, sem app nativo).
- Uso pessoal / familiar (poucos utilizadores, um único operador de dados).

## Documentos

| Documento | Conteúdo |
| --- | --- |
| [docs/01-arquitetura.md](docs/01-arquitetura.md) | Contexto, drivers, estilo arquitetural, camadas, módulos, fluxos, roadmap |
| [docs/02-stack.md](docs/02-stack.md) | Go e bibliotecas escolhidas, com justificativa e alternativas rejeitadas |
| [docs/03-modelo-de-dados.md](docs/03-modelo-de-dados.md) | Esquema SQL completo, invariantes, índices, estratégia de deduplicação |
| [docs/04-motor-importacao-csv.md](docs/04-motor-importacao-csv.md) | Pipeline de importação em 7 estágios, deteção automática, dedupe, aprendizagem |
| [docs/05-decisoes-adr.md](docs/05-decisoes-adr.md) | 17 ADRs com contexto, alternativas avaliadas e consequências (com registo de revisões e índice no topo) |
| [docs/06-operacao-arm64.md](docs/06-operacao-arm64.md) | Deploy Docker multi-arch, TLS, backup, segurança, monitorização, plano de restauro |
| [docs/07-muitas-contas-e-cartoes.md](docs/07-muitas-contas-e-cartoes.md) | Muitas contas e vários cartões: roteamento por conteúdo, biblioteca de perfis, painel de cobertura, evitação de dupla contagem |
| [docs/08-otimizacao-de-contexto.md](docs/08-otimizacao-de-contexto.md) | Economia de contexto para agentes de IA: inventário das customizações, orçamentos, antipadrões e manutenção |
| [docs/99-fora-de-ambito-pdf.md](docs/99-fora-de-ambito-pdf.md) | **Arquivado.** Estudo técnico de extração de PDF, preservado porque a decisão o excluiu do âmbito (ADR-016) |

## Resumo das decisões estruturantes

1. **Monólito modular *server-centric*** — o servidor ARM64 é a fonte de verdade **e o único local onde existe lógica**. A UI é HTML renderizado no servidor (`templ`), com HTMX para interação e Alpine.js apenas para estado local de interface. Não há SPA, não há cliente de API, não há *bundler*.
2. **Go em todo o servidor** — binário único, `CGO_ENABLED=0`, `modernc.org/sqlite` (Go puro). Zero risco de módulos nativos em ARM64, RAM na casa das dezenas de MB, arranque em milissegundos.
3. **SQLite em modo WAL** como base de dados única — zero administração, ideal para ARM64 e 1 utilizador; caminho de migração para PostgreSQL definido e isolado em `internal/data`.
4. **Importação por adapters com *staging* e pré-visualização obrigatória** — CSV, OFX e CAMT.053 passam pelo mesmo pipeline. Nenhuma linha entra na base de dados sem um *diff* aprovável e reversível (`undo` de lote inteiro).
5. **A biblioteca de perfis por instituição substitui a configuração manual** — se o banco está coberto, o trabalho do utilizador é zero. Adicionar um banco é editar um ficheiro de dados, não escrever código.
6. **O cartão é uma conta e o pagamento da fatura é uma transferência emparelhada por valor exato** — é o que impede a despesa de ser contada duas vezes, o defeito mais comum em quem tem vários cartões.
7. **O painel de cobertura responde a «importei tudo este mês?»** — 9 contas × 12 meses deixam de ser 108 verificações mentais e passam a ser uma lista de pendências.
8. **Reconciliação declarada** — usar como invariantes todos os números que a fonte declara (saldo, totais, contagens), com explicação acionável da divergência em vez de um simples «não fecha».
9. **Regras e perfis como dados** — motor de regras em JSON, perfis em YAML versionados no repositório. O utilizador edita, exporta e partilha; o binário não muda.
10. **PDF fora de âmbito** — decisão explícita (ADR-016), que remove do sistema a dependência de `poppler`/OCR, o editor de colunas e um eixo de erro silencioso. O estudo técnico fica arquivado em `docs/99` para que reabrir seja barato se um dia se justificar.
11. **Economia de contexto como requisito de projeto** (ADR-017) — o trabalho é feito em parte com assistentes de IA, e o custo dominante é o contexto redescoberto em cada pedido. Instruções de agente curtas e com orçamento, contexto condicional por `applyTo` específico (nunca `**`), agentes com ferramentas mínimas, *hooks* para o que tem de ser determinístico, e índice navegável no topo dos documentos. Inventário e regras de manutenção em [docs/08](docs/08-otimizacao-de-contexto.md).

## O que deliberadamente **não** fazemos

Micro-serviços, Kubernetes, Redis, PostgreSQL, GraphQL, SPA e *framework* de frontend, `node_modules`, app Electron, motor de planilha completo, multi-moeda, *machine learning* em *runtime*, **extração de PDF e OCR**. Cada um destes é registado como não-objetivo com justificativa em [docs/01-arquitetura.md](docs/01-arquitetura.md#11-não-objetivos) e nos ADRs.

Sobre IA: `transformers.js` e modelos ONNX foram avaliados e **rejeitados nesta fase** — reintroduziriam um *runtime* JavaScript e contradiriam o ADR-006. A escada de evolução admitida (degraus 1 a 4) está registada no ADR-006, com a IA a entrar apenas como ferramenta *offline* de *build*, nunca como dependência do binário.

## Nota sobre licenciamento

O Actual Budget é distribuído sob licença **MIT**. Reimplementar funcionalidades é livre; reutilizar código-fonte obriga a preservar o aviso de copyright e a atribuição. A decisão registada (ADR-011) é **reimplementar com referência funcional**, não fazer *fork*. O mesmo critério foi aplicado a ferramentas como o Tabula: as ideias de UX e de algoritmo são referência, não código copiado.
