# 10 — Construção: estrutura do repositório e plano de arranque

Este documento responde a duas perguntas: **como o repositório está organizado** e **em que ordem se constrói**. É escrito para ser seguido por uma pessoa e por agentes de IA que trabalham em cima do projeto, por isso a estrutura não é só arrumação — é **contrato**.

Objetivo do produto, para não se perder de vista: **agilidade na importação de CSV**. Tudo o que não sirva isso é adiado ([ADR-018](05-decisoes-adr.md#adr-018--o-objetivo-primário-é-o-tempo-de-importação), [09](09-escopo-vs-actual.md)).

## Índice

1. [Princípios que a estrutura tem de servir](#1-princípios-que-a-estrutura-tem-de-servir)
2. [Árvore do repositório](#2-árvore-do-repositório)
3. [Regras que a estrutura impõe](#3-regras-que-a-estrutura-impõe)
4. [Convenções que evitam idas-e-voltas](#4-convenções-que-evitam-idas-e-voltas)
5. [Ordem de construção: fatias verticais](#5-ordem-de-construção-fatias-verticais)
6. [O primeiro dia](#6-o-primeiro-dia)
7. [Verificação](#7-verificação)
8. [O que fica proibido](#8-o-que-fica-proibido)
9. [Manutenção](#9-manutenção)

---

## 1. Princípios que a estrutura tem de servir

Um agente de IA trabalha bem quando consegue responder, **sem explorar o repositório**, a três perguntas: *onde vive isto?*, *o que posso importar daqui?* e *como sei que não quebrei nada?*.

| Princípio | Tradução concreta |
| --- | --- |
| **Uma pasta, um assunto** | Encontrar pelo nome, não por busca. `internal/modules/reconcile` só tem reconciliação |
| **Fronteiras verificáveis** | A regra de dependência é verificada por um **teste** que falha, não por disciplina |
| **Toda a decisão escrita** | `docs/` é citável. O agente não redescobre nem re-litiga (ADR-011, ADR-017) |
| **Dados reais no repositório** | `testdata/` tem *fixtures* anonimizadas; o agente valida sem inventar entrada |
| **Erros com código estável** | UI e testes reagem a `DIAGNOSTIC_CODE`, nunca a texto de mensagem |
| **Um comando para tudo** | `make check` faz formato, *build*, *lint* e testes. Sem sequências a memorizar |
| **Nada de geração implícita** | O único artefacto gerado é o CSS do Tailwind, e é determinístico |

---

## 2. Árvore do repositório

Estrutura **fechada**: criar diretórios fora desta lista exige justificação num ADR.

Convenção da árvore: **sem marca** significa que já existe (M0); **(M1)**, **(M2)** e afins marcam o que ainda não deve ser criado.

```
.
├── README.md                     índice e resumo das decisões
├── AGENTS.md                     ponto de entrada para agentes
├── assets.go                     embeds: migrations/ e web/static (pacote raiz)
├── Makefile                      check, run, test, lint, fmt, generate, css
├── go.mod  go.sum                Go, CGO_ENABLED=0
├── Dockerfile                    multi-arch, sem CGO
├── .golangci.yml  .editorconfig  .gitignore
│
├── cmd/
│   └── app/
│       ├── main.go               único ponto de entrada: flags, arranque, sinais, CLI de senha
│       └── main_test.go          leitura de senha e separação de flags
│
├── internal/
│   ├── domain/                   tipos e regras puras — NÃO importa as outras camadas
│   │   ├── money.go              Cents (int64), análise e formatação pt-BR
│   │   ├── date.go               Date civil 'YYYY-MM-DD' (texto, nunca time.Time com fuso)
│   │   ├── id.go                 ULID: chave primária ordenável por tempo
│   │   ├── errors.go             erros sentinela, usados com errors.Is
│   │   ├── fronteira_test.go     falha se este pacote importar as outras camadas
│   │   ├── account.go      (M1)  conta, tipo (checking | credit | investment), fecho
│   │   ├── transaction.go  (M1)  lançamento, transfer_id, imported_id, imported_payee
│   │   ├── importing.go    (M1)  Batch, Row, RowStatus, Profile
│   │   ├── diagnostic.go   (M1)  Diagnostic + catálogo de códigos estáveis
│   │   └── rule.go         (M1)  regra (stages pre | default | post), condição, ação
│   │
│   ├── modules/                  casos de uso; orquestram domain e data. Não falam HTTP
│   │   ├── auth/                 senha única, sessões, argon2id, limite de tentativas
│   │   ├── system/               estado da base, para o ecrã inicial ser verificável
│   │   ├── ingest/         (M1)  deteção de formato, assinatura, roteamento, criação do lote
│   │   ├── normalize/      (M1)  datas, valores, sinais, limpeza de payee, ignorados
│   │   ├── reconcile/      (M1)  dedupe T0–T4, fatura matcher, reconciliação declarada
│   │   ├── rules/          (M1)  motor de regras e índice por primeiro caractere
│   │   ├── learning/       (M3)  payee_mappings: hits, misses, confiança
│   │   ├── coverage/       (M2)  painel de cobertura e pendências
│   │   └── budget/         (M4)  alocação mensal e agregados materializados
│   │
│   ├── adapters/            (M1) leitura de formatos → estruturas do domain
│   │   ├── csvsource/            encoding, delimitador, cabeçalho, linhas irregulares
│   │   ├── ofxsource/      (M3.5) FITID, ACCTID, LEDGERBAL
│   │   ├── camtsource/     (M3.5) IBAN, AcctSvcrRef
│   │   └── imapsource/     (M4.5) entrega automática por email
│   │
│   ├── data/                     TODO o SQL vive aqui, e só aqui
│   │   ├── db.go                 abertura, PRAGMAs (WAL, busy_timeout, foreign_keys)
│   │   ├── migrate.go            migrações para a frente, aplicadas por ordem
│   │   ├── mutator.go            única via de escrita: transação + auditoria
│   │   ├── audit.go              leitura do journal, para o undo do lote
│   │   ├── auth.go               credencial e sessões
│   │   ├── settings.go           app_settings
│   │   ├── accounts.go  transactions.go  imports.go      (M1)
│   │   └── payees.go    rules.go         budget.go       (M1/M3/M4)
│   │
│   ├── http/                     rotas, handlers, sessão
│   │   ├── server.go             montagem, rotas, cabeçalhos, ficheiros estáticos
│   │   ├── auth.go               entrar, sair, primeiro arranque, middleware de sessão
│   │   └── sse.go           (M1) progresso de importação
│   │
│   ├── views/                    templ: páginas e fragments HTMX
│   │   ├── layout/               documento base e enquadramento autenticado
│   │   ├── auth/                 primeiro arranque e entrada
│   │   ├── home/                 estado da fundação
│   │   ├── imports/         (M1) upload, triagem, pré-visualização POR EXCEÇÃO
│   │   ├── accounts/  transactions/  coverage/  budget/  (M2/M4)
│   │   └── components/      (M1) tabela, campo, botão, badge de estado, dialog
│   │
│   └── infra/                    o que toca o mundo exterior
│       ├── config/               flags, variáveis de ambiente, valores por omissão
│       ├── log/                  log estruturado
│       ├── clock/                relógio injetável e tzdata embutida
│       └── inbox/           (M2) vigilância de /data/inbox
│
├── migrations/
│   └── 0001_init.sql             esquema de M0/M1; 0002_*.sql acrescenta fases seguintes
│
├── profiles/                (M1) perfis de instituição — DADOS, não código (ADR-015)
│   ├── SCHEMA.md                 formato do perfil, campos obrigatórios
│   ├── institutions.yaml         tabela de conhecimento por instituição
│   └── bancos/<slug>.yaml
│
├── web/
│   └── static/
│       ├── tokens.css            paleta Nord, fonte única das cores (ADR-020)
│       ├── tailwind.css          entrada do Tailwind: importa tokens e as vistas
│       ├── app.css               GERADO por `make css` — não editar
│       ├── htmx.min.js
│       └── alpine.min.js
│
├── testdata/                (M1) fixtures anonimizadas e resultados esperados
│   ├── fixtures/<slug>/
│   ├── golden/<slug>/
│   └── README.md                 como anonimizar e como acrescentar
│
└── .github/
    ├── copilot-instructions.md   ≤ 60 linhas, sempre carregado (ADR-017)
    ├── instructions/             docs/
    ├── prompts/                  novo-perfil-banco
    ├── agents/                   corpus-perfis
    └── hooks/                    formatação e verificação determinísticas
```

**Porque `assets.go` está na raiz.** `//go:embed` só alcança ficheiros da própria pasta para baixo, e `migrations/` e `web/static/` ficam na raiz por decisão de estrutura. O pacote raiz expõe cada sistema de ficheiros **já com a raiz resolvida** — `financas.Migrations` contém `0001_init.sql`, e não `migrations/0001_init.sql`. Sem essa simetria, o servidor resolvia o prefixo e o migrator não: o binário arrancava sem esquema e falhava com «no such table: users», enquanto os testes passavam por lerem do disco. O `assets_test.go` da raiz cobre exatamente esse caminho.

---

## 3. Regras que a estrutura impõe

**Regra de dependência** (uma seta é permitida, nunca o inverso):

```
cmd → http → views → modules → domain
                 ↓
              data → infra
```

| Regra | Como se verifica |
| --- | --- |
| `internal/domain` não importa `data`, `infra`, `http` nem `adapters` | **Teste** em `internal/domain/fronteira_test.go` que lê os imports e falha |
| `modules` não fala HTTP nem HTML | Revisão; `views` chama `modules`, nunca o contrário |
| SQL só em `internal/data` | Revisão + *lint* a procurar `SELECT`/`INSERT` fora de `data` |
| `adapters` não escreve na base de dados | Só devolve estruturas do `domain` |
| `views` não contém regra de negócio | Lógica em `modules`, dados já resolvidos |
| Toda a escrita passa pelo `mutator` | *Lint* a procurar `db.Exec` fora de `data` |

**Onde acrescentar as coisas** — a tabela que evita a exploração:

| Quero… | Vou a |
| --- | --- |
| Suportar um banco novo | `profiles/bancos/<slug>.yaml` + `testdata/fixtures/<slug>/` |
| Um diagnóstico novo | `internal/domain/diagnostic.go` + consumidores na UI |
| Uma consulta nova | `internal/data/<agregado>.go` |
| Um caso de uso | `internal/modules/<assunto>/` |
| Um ecrã | `internal/views/<assunto>/` + rota em `http/routes.go` |
| Uma tabela nova | `migrations/NNNN_*.sql` + `internal/data/` + ADR se estrutural |
| Uma cor | `web/static/tokens.css` — em nenhum outro lugar |

---

## 4. Convenções que evitam idas-e-voltas

Estas são as que mais tempo poupam a quem (humano ou agente) chega ao código sem contexto.

| Convenção | Porquê |
| --- | --- |
| **Diagnósticos com código**, texto só na apresentação | Testes e UI comparam códigos; mudar texto não quebra nada |
| **`Money` é `int64` em cêntimos, sempre** | Nenhuma expressão intermédia em `float64` (ADR-005) |
| **Datas como texto `YYYY-MM-DD`** | Comparação e ordenação por texto; sem fuso horário |
| **`context.Context` no primeiro argumento** de tudo o que toca I/O | Cancelamento e *timeouts* previsíveis |
| **Erros envolvidos com `%w`** | `errors.Is`/`errors.As` em vez de comparação de texto |
| **Um teste *golden* por instituição** | O comportamento do normalizador é observável num *diff* revisto |
| **Comentários em português, identificadores em inglês** | Convenção do projeto |
| **Sem `init()` e sem variáveis globais mutáveis** | Dependência explícita, testável sem truques |
| **`internal/` em tudo** | Nada é API pública: não há compatibilidade a manter |
| **`templ generate` é determinístico** | Os ficheiros `*_templ.go` são gerados e versionados |

---

## 5. Ordem de construção: fatias verticais

Cada fatia atravessa todas as camadas e **entrega algo utilizável**. Não se constrói «a camada de dados» e depois «a UI»: constrói-se uma fatia que importa um CSV real de ponta a ponta.

A ordem é por **minutos devolvidos** ao utilizador (ADR-018), não por dificuldade técnica.

A **ordem canónica** está em [01](01-arquitetura.md#9-roadmap-por-fases) §9. Aqui fica a decomposição executável de cada fase: o que entra, onde vive e o critério de pronto. As etiquetas `M` coincidem com as de lá — não renomear nenhuma sem alterar os dois documentos.

### M0 — Esqueleto que corre

Objetivo: `make run` abre uma página e cria a base de dados.

| Entrega | Onde |
| --- | --- |
| `go.mod`, Makefile, `.golangci.yml`, `.editorconfig` | raiz |
| Ligação SQLite + *migrator* + `0001_init.sql` (esquema do [03](03-modelo-de-dados.md)) | `internal/data` |
| `mutator` com transação, `revision` e `audit_log` | `internal/data/mutator.go` |
| Servidor HTTP + *layout* Nord + ecrã de *login* | `internal/http`, `internal/views` |
| Tailwind *standalone* + `tokens.css` | `web/static` |

**Pronto quando:** arranco o binário, defino a senha, entro, e a base de dados existe com o esquema completo.

### M1 — Importar um CSV de verdade (o coração)

Esta é a fatia que decide o projeto. Tudo o resto é depois.

| Entrega | Onde |
| --- | --- |
| Leitura de CSV: encoding, delimitador, cabeçalho, linhas irregulares | `internal/adapters/csvsource` |
| Deteção de perfil por assinatura de colunas | `internal/modules/ingest` |
| Normalização: datas, valores, sinais, limpeza de *payee* | `internal/modules/normalize` |
| *Staging*: `import_batches` + `import_rows` | `internal/data/imports.go` |
| **Pré-visualização por exceção** ([04](04-motor-importacao-csv.md) §8.1) | `internal/views/imports` |
| *Commit* transacional + *undo* de lote | `internal/modules/ingest`, `data/mutator.go` |
| Motor de regras (stages) | `internal/modules/rules` |
| Dedupe T0–T2 | `internal/modules/reconcile` |
| **Um perfil real** + *fixture* anonimizada + teste *golden* | `profiles/`, `testdata/` |

**Pronto quando:** importo o CSV do meu banco **com zero decisões**, em menos de um minuto, e desfaço com um clique.

### M2 — Muitas contas e cartões

| Entrega | Onde |
| --- | --- |
| `/data/inbox` + roteamento por conteúdo | `internal/infra/inbox`, `modules/ingest` |
| Conta de cartão (`type='credit'`) | `domain`, `data/accounts.go` |
| *Fatura matcher* pelo total declarado (ADR-014) | `internal/modules/reconcile` |
| Contas-alvo distintas por linha (`target_account_id`) | `data/imports.go` |
| Reconciliação declarada (saldo, totais, contagens) | `internal/modules/reconcile` |
| Painel de cobertura e lista de pendências | `internal/modules/coverage`, `views/coverage` |

**Pronto quando:** largo 9 ficheiros na pasta, o sistema encaminha tudo, emparelha os pagamentos de fatura e não conta nada duas vezes.

### M3 — Aprendizagem e perfis

| Entrega | Onde |
| --- | --- |
| `payee_mappings` com `hits`/`misses`/`confidence` | `internal/modules/learning` |
| Biblioteca de perfis (várias instituições) e tabela de conhecimento | `profiles/bancos/`, `profiles/institutions.yaml` |
| Regras de *payee* brasileiras | `profiles/bancos/*.yaml` |

**Pronto quando:** uma instituição nova coberta deixa de exigir configuração, e depois de dois meses a maioria dos lançamentos entra categorizada sozinha.

### M3.5 — OFX e CAMT.053

| Entrega | Onde |
| --- | --- |
| Leitura de OFX (`FITID`, `ACCTID`, `LEDGERBAL`) | `internal/adapters/ofxsource` |
| Leitura de CAMT.053 (`IBAN`, `AcctSvcrRef`) | `internal/adapters/camtsource` |
| Roteamento por identificador de conta | `domain`, `accounts.external_account_id` |

**Pronto quando:** uma conta com OFX deixa de ter qualquer trabalho manual, e o roteamento é **associação direta** e não heurística.

### M4 — Orçamento essencial

Categorias e grupos, orçamento por envelope com *rollover*, agregados materializados (`budget_month_cache`), transferências, *splits* e busca. **Única exceção consciente** à ordem do ADR-018 §6: não reduz o tempo de importação, mas é o que torna os dados úteis — sem ele não há razão para importar.

### M4.5 — Entrega automática

Adapter **IMAP** ([07](07-muitas-contas-e-cartoes.md) §2, degrau 4) e `auto_commit` por perfil quando todos os diagnósticos são `ok`. É o maior ganho que resta no tempo mensal, e depende dos perfis do M3 — sem perfil, entrega automática só significa ficheiros a chegar mais depressa a um sistema que ainda pergunta a conta.

### M5 — Polimento

Atalhos de teclado, exportação portável, *passkeys* opcionais. Nada aqui altera o tempo de importação.

### Critério de pronto de qualquer fatia

1. *Fixture* anonimizada no repositório + teste *golden*, quando há dados.
2. Diagnósticos visíveis na UI, com código estável.
3. Documentação atualizada no mesmo *pull request*.
4. Nenhuma dependência nova sem justificação escrita (AGENTS.md, regra 5).
5. `make check` verde.

---

## 6. O primeiro dia — **concluído (2026-09-15)**

Sequência executável, na ordem. Nada aqui dependia de decisão em aberto.

| # | Passo | Comando ou ficheiro | Estado |
| --- | --- | --- | --- |
| 1 | Módulo Go | `go mod init financas` | feito |
| 2 | Ferramentas de qualidade | `.golangci.yml`, `.editorconfig`, `.gitignore`, `Makefile` | feito |
| 3 | Esquema | `migrations/0001_init.sql` a partir do [03](03-modelo-de-dados.md) | feito |
| 4 | Acesso a dados | `internal/data/db.go` (WAL + PRAGMAs), `migrate.go` | feito |
| 5 | Escrita | `internal/data/mutator.go` + `audit_log` | feito |
| 6 | Teste de fronteira | `internal/domain/fronteira_test.go` — falha se `domain` importar outra camada | feito |
| 7 | Servidor | `internal/http/server.go` + `views/layout` | feito |
| 8 | Autenticação | `internal/modules/auth` + `internal/http/auth.go` (ADR-019) | feito |
| 9 | Tema | `web/static/tokens.css` + `make css` (ADR-020) | feito |
| 10 | Verificação | `make check` verde | feito |

**M0 aceito pelos critérios do §5:** o binário arranca, o primeiro arranque define a senha, a entrada abre sessão, e a base de dados existe com o esquema completo — verificável no ecrã inicial. Sem senha definida, tudo encaminha para a definição inicial, e o ecrã desaparece (404) depois de concluído.

### Dois defeitos que só a execução apanhou

Ambos passaram por `go build` e pelos testes, e só apareceram a correr o binário. Ficam registados porque a lição é sobre o que se testa, não sobre o que se escreveu.

| Defeito | Causa | Correção |
| --- | --- | --- |
| Arranque sem esquema (`no such table: users`) | O servidor resolvia `web/static` com `fs.Sub`, o migrator não resolvia `migrations/`: o mesmo contrato era cumprido a meio. Os testes liam do disco com a raiz certa, e por isso nunca viram o embutido | `financas.Migrations` e `financas.Web` passam a expor a **raiz já resolvida**, e `assets_test.go` percorre o caminho do binário |
| Troca de senha por CLI dizia que as senhas não coincidiam | Um `bufio.Reader` novo por chamada: a primeira leitura levava as duas linhas para o seu buffer | Um leitor único, partilhado pelas duas leituras, com teste próprio |

**Depois do M0**, a próxima tarefa é escolher **um** banco real, anonimizar a *fixture* e escrever o perfil. Sem isso não há como saber se o motor está certo — e o motor é o M1.

---

## 7. Verificação

Comandos já decididos (AGENTS.md):

```bash
go build ./...          # compila
go test ./...           # testes
golangci-lint run       # lint
golangci-lint fmt       # formatação
make css                # recompila web/static/app.css
templ generate          # regenera os *_templ.go
```

`make check` corre tudo isto, exceto `make css` e `templ generate` — que estão em `make generate`, executado primeiro pelo `check`. Não existe passo de *build* de frontend além do CSS.

**Uma só autoridade de formatação.** O `gofumpt` instalado e o formatador embutido no `golangci-lint` discordavam sobre o agrupamento de imports, e o resultado era `make fmt` deixar o repositório num estado que o `make lint` recusava. O projeto usa o formatador do próprio linter (`golangci-lint fmt`): uma ferramenta, uma resposta.

---

## 8. O que fica proibido

Regras curtas, porque são as que se esquecem:

- **Criar diretórios fora da §2.** Se falta um, propõe-se ADR.
- **SQL fora de `internal/data`.**
- **Escrever nas tabelas de negócio sem passar pelo `mutator`.**
- **`float64` em qualquer expressão monetária**, incluindo intermédias.
- **`time.Time` com fuso** em data de transação.
- **Regra de negócio em JavaScript.** Alpine.js é só estado local de interface.
- **Cor literal fora de `tokens.css`.**
- **Dependência que exija cgo**, ORM, Node/npm, OCR ou PDF (ADR-016).
- **Alterar `testdata/` para fazer um teste passar.** A *fixture* é a verdade.
- **Reservar campo ou tabela para funcionalidade futura.** O esquema implementa o que existe ([09](09-escopo-vs-actual.md) §7).

---

## 9. Manutenção

- Esta árvore é **contrato**: alterá-la é alteração estrutural, logo passa por ADR.
- Quando uma pasta nova aparecer, entra também na tabela «onde acrescentar as coisas» (§3) e no inventário do [08](08-otimizacao-de-contexto.md) §2 se trouxer ficheiro de agente.
- O `Makefile` é a interface única de comandos. Se um comando novo for usado duas vezes, entra lá.
