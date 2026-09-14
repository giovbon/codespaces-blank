# 03 — Modelo de Dados

Base de dados única SQLite, acessível exclusivamente pelo servidor. Migrações versionadas e revistas como código.

## 1. Configuração e invariantes globais

```sql
PRAGMA journal_mode = WAL;      -- leituras concorrentes com um escritor
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;    -- durabilidade adequada com WAL
PRAGMA temp_store = MEMORY;
```

**Invariantes não negociáveis:**

1. **Dinheiro é inteiro.** Toda a quantia é guardada em cêntimos (`*_cents INTEGER`). Ponto flutuante nunca representa dinheiro. `-1234` significa 12,34 de saída.
2. **Datas de transação são civis**, em `TEXT 'YYYY-MM-DD'`. Um lançamento bancário é uma data de calendário, não um instante — representá-lo como instante UTC é a origem clássica de lançamentos a «cair» no dia anterior.
3. **Instantes** (`created_at`, `updated_at`, `run_at`) são `INTEGER` em milissegundos *epoch*.
4. **Nunca há `DELETE` físico** em entidades de negócio: `tombstone = 1`. Isto preserva o *undo*, a auditoria e a possibilidade de reimportar um lançamento apagado.
5. **Toda a escrita passa por um mutator** que abre transação, aplica regras, escreve no journal e incrementa `revision`.
6. **Chaves primárias são ULID** (`TEXT`, 26 caracteres) — ordenáveis por tempo, o que ajuda no *debug* e na paginação.

---

## 2. Esquema

### 2.1 Contas

```sql
CREATE TABLE accounts (
  id                   TEXT PRIMARY KEY,
  name                 TEXT NOT NULL,
  type                 TEXT NOT NULL CHECK (type IN
                         ('checking','savings','credit','investment','loan','cash','other')),
  currency             TEXT NOT NULL DEFAULT 'BRL',
  off_budget           INTEGER NOT NULL DEFAULT 0,
  closed               INTEGER NOT NULL DEFAULT 0,
  sort_order           REAL    NOT NULL DEFAULT 0,
  institution          TEXT,
  institution_slug     TEXT,             -- 'itau','nubank','c6',... liga à biblioteca de perfis
  account_number_mask  TEXT,
  external_account_id  TEXT,             -- ACCTID (OFX) ou IBAN (CAMT): roteamento exato de ficheiros
  -- Campos específicos de cartão de crédito (type = 'credit'); ver ADR-014
  card_brand           TEXT,             -- 'visa','mastercard','elo','amex','hipercard'
  card_mask            TEXT,             -- '1234'
  card_holder          TEXT,             -- 'titular' | 'adicional:<nome>'
  closing_day          INTEGER,          -- dia de fecho da fatura (1-31)
  due_day              INTEGER,          -- dia de vencimento (1-31)
  credit_limit_cents   INTEGER,
  -- Saldos
  balance_cents        INTEGER,          -- último saldo conhecido conforme o banco
  balance_date         TEXT,
  reconcile_start_date TEXT,
  created_at           INTEGER NOT NULL,
  updated_at           INTEGER NOT NULL,
  tombstone            INTEGER NOT NULL DEFAULT 0,
  revision             INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX ix_accounts_order ON accounts(off_budget, sort_order) WHERE tombstone = 0;
CREATE UNIQUE INDEX ux_accounts_external
  ON accounts(external_account_id) WHERE external_account_id IS NOT NULL AND tombstone = 0;
```

Notas de modelação:

- **Um cartão de crédito é uma conta** (`type = 'credit'`). As compras são negativas e o saldo é uma passividade. Sem isto não há forma correta de representar dívida de cartão, e o pagamento da fatura na conta corrente seria contado como despesa além das compras já importadas — dupla contagem (ADR-014).
- `external_account_id` é o **roteamento exato** de ficheiros: OFX traz `ACCTID` e CAMT.053 traz `IBAN`. Um ficheiro com este identificador não precisa de heurística nenhuma para encontrar a conta ([07 §3](07-muitas-contas-e-cartoes.md#3-roteamento-automático-por-conteúdo)).
- `closing_day`/`due_day` alimentam o painel de cobertura e o emparelhamento de faturas (saber qual fatura está em falta).

### 2.2 Categorias

```sql
CREATE TABLE category_groups (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order REAL NOT NULL DEFAULT 0,
  hidden     INTEGER NOT NULL DEFAULT 0,
  tombstone  INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);

CREATE TABLE categories (
  id                TEXT PRIMARY KEY,
  group_id          TEXT NOT NULL REFERENCES category_groups(id),
  name              TEXT NOT NULL,
  sort_order        REAL NOT NULL DEFAULT 0,
  hidden            INTEGER NOT NULL DEFAULT 0,
  is_income         INTEGER NOT NULL DEFAULT 0,
  goal_json         TEXT,               -- objetivo: tipo, valor, prazo
  created_at        INTEGER NOT NULL,
  updated_at        INTEGER NOT NULL,
  tombstone         INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_categories_name ON categories(group_id, lower(name)) WHERE tombstone = 0;
```

### 2.3 Estabelecimentos e aprendizagem

```sql
CREATE TABLE payees (
  id                  TEXT PRIMARY KEY,
  name                TEXT NOT NULL,          -- nome canónico apresentado ao utilizador
  default_category_id TEXT REFERENCES categories(id),
  notes               TEXT,
  created_at          INTEGER NOT NULL,
  updated_at          INTEGER NOT NULL,
  tombstone           INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_payees_name ON payees(lower(name)) WHERE tombstone = 0;

-- Tabela de aprendizagem: liga a string bruta do banco ao payee e categoria escolhidos
CREATE TABLE payee_mappings (
  raw_normalized  TEXT PRIMARY KEY,   -- normalizado: sem acentos, maiúsculas, sem ruído
  raw_example     TEXT NOT NULL,      -- exemplo original, só para diagnóstico
  payee_id        TEXT REFERENCES payees(id),
  category_id     TEXT REFERENCES categories(id),
  hits            INTEGER NOT NULL DEFAULT 0,
  misses          INTEGER NOT NULL DEFAULT 0,
  confidence      REAL    NOT NULL DEFAULT 0,
  last_used_at    INTEGER
);
```

Regra de aceitação automática: aplicar o mapeamento sem perguntar quando `hits >= 3` **e** `confidence >= 0.85`. Abaixo disso, sugerir na pré-visualização e contar como `hits`/`misses` após decisão humana.

### 2.4 Transações

```sql
CREATE TABLE transactions (
  id                TEXT PRIMARY KEY,
  account_id        TEXT NOT NULL REFERENCES accounts(id),
  date              TEXT NOT NULL,             -- YYYY-MM-DD
  amount_cents      INTEGER NOT NULL,          -- negativo = saída
  currency          TEXT NOT NULL DEFAULT 'BRL',
  payee_id          TEXT REFERENCES payees(id),
  category_id       TEXT REFERENCES categories(id),
  notes             TEXT,
  cleared           INTEGER NOT NULL DEFAULT 0,
  reconciled        INTEGER NOT NULL DEFAULT 0,
  is_parent         INTEGER NOT NULL DEFAULT 0,   -- split
  parent_id         TEXT REFERENCES transactions(id),
  transfer_id       TEXT,                          -- par de transferência entre contas
  schedule_id       TEXT REFERENCES schedules(id),
  imported_id       TEXT,                          -- id determinístico da linha do extrato
  imported_payee    TEXT,                          -- descrição bruta, preservada
  import_batch_id   TEXT REFERENCES import_batches(id),
  source            TEXT NOT NULL DEFAULT 'manual'
                    CHECK (source IN ('manual','csv','ofx','camt','api')),
  sort_order        REAL NOT NULL DEFAULT 0,
  raw_data          TEXT,                          -- JSON da linha original, para auditoria
  created_at        INTEGER NOT NULL,
  updated_at        INTEGER NOT NULL,
  tombstone         INTEGER NOT NULL DEFAULT 0,
  revision          INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX ux_tx_imported_id
  ON transactions(account_id, imported_id)
  WHERE imported_id IS NOT NULL AND tombstone = 0;

CREATE INDEX ix_tx_account_date ON transactions(account_id, date DESC) WHERE tombstone = 0;
CREATE INDEX ix_tx_match        ON transactions(account_id, date, amount_cents) WHERE tombstone = 0;
CREATE INDEX ix_tx_payee_date   ON transactions(payee_id, date DESC) WHERE tombstone = 0;
CREATE INDEX ix_tx_category     ON transactions(category_id, date DESC) WHERE tombstone = 0;
CREATE INDEX ix_tx_parent       ON transactions(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX ix_tx_batch        ON transactions(import_batch_id) WHERE import_batch_id IS NOT NULL;
```

Notas de modelação:

- **`splits`** são linhas filhas com `parent_id` definido e `is_parent = 1` no pai que carrega o total. É o modelo do Actual e funciona bem.
- **`transfer_id`** liga dois lançamentos (um negativo, um positivo) em contas diferentes. A UI apresenta-os como uma transferência única.
- **`imported_id` + `imported_payee`** são a memória da importação: permitem reconciliar, reimportar e auditar sem perder a descrição original do banco.

### 2.5 Etiquetas, notas e anexos

```sql
CREATE TABLE tags (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, color TEXT,
  tombstone INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_tags_name ON tags(lower(name)) WHERE tombstone = 0;

CREATE TABLE transaction_tags (
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  tag_id         TEXT NOT NULL REFERENCES tags(id),
  PRIMARY KEY (transaction_id, tag_id)
);

-- Notas em qualquer entidade: 'account', 'category', 'budget_month', 'payee'
CREATE TABLE notes (
  entity_type TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  updated_at  INTEGER NOT NULL,
  PRIMARY KEY (entity_type, entity_id)
);

CREATE TABLE attachments (
  id             TEXT PRIMARY KEY,
  transaction_id TEXT REFERENCES transactions(id) ON DELETE SET NULL,
  filename       TEXT NOT NULL,
  mime           TEXT NOT NULL,
  size_bytes     INTEGER NOT NULL,
  sha256         TEXT NOT NULL,
  blob_path      TEXT NOT NULL,       -- relativo a /data/attachments
  created_at     INTEGER NOT NULL
);
```

### 2.6 Orçamento por envelope

```sql
CREATE TABLE budget_allocations (
  id             TEXT PRIMARY KEY,
  month          TEXT NOT NULL,                     -- YYYY-MM
  category_id    TEXT NOT NULL REFERENCES categories(id),
  budgeted_cents INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL,
  revision       INTEGER NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX ux_budget_month_category ON budget_allocations(month, category_id);

CREATE TABLE budget_month_state (
  month          TEXT PRIMARY KEY,
  carryover_json TEXT,      -- envelope negativo/positivo transportado
  closed         INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL
);

-- Agregados materializados: a UI nunca soma transações em tempo real para mostrar o mês
CREATE TABLE budget_month_cache (
  month             TEXT NOT NULL,
  category_id       TEXT NOT NULL,
  spent_cents       INTEGER NOT NULL DEFAULT 0,
  received_cents    INTEGER NOT NULL DEFAULT 0,
  available_cents   INTEGER NOT NULL DEFAULT 0,
  computed_at       INTEGER NOT NULL,
  PRIMARY KEY (month, category_id)
);
```

`budget_month_cache` é reconstruído por *job* após qualquer escrita que afete o mês. Evita repetir agregações sobre dezenas de milhares de transações a cada carregamento de ecrã — exatamente o tipo de custo que dói num CPU ARM modesto.

### 2.7 Regras e agendamentos

```sql
CREATE TABLE rules (
  id              TEXT PRIMARY KEY,
  stage           TEXT NOT NULL DEFAULT 'default'
                  CHECK (stage IN ('pre','default','post')),
  conditions_json TEXT NOT NULL,      -- { op: 'and', children: [...] }
  actions_json    TEXT NOT NULL,      -- [{ op:'set', field:'category_id', value:'...' }]
  enabled         INTEGER NOT NULL DEFAULT 1,
  sort_order      INTEGER NOT NULL DEFAULT 0,
  description     TEXT,
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  tombstone       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE schedules (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  account_id    TEXT REFERENCES accounts(id),
  payee_id      TEXT REFERENCES payees(id),
  category_id   TEXT REFERENCES categories(id),
  amount_cents  INTEGER,
  rule_json     TEXT NOT NULL,        -- condições de reconhecimento
  recurrence    TEXT NOT NULL,        -- RRULE ou expressão simples
  next_date     TEXT,
  completed     INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  tombstone     INTEGER NOT NULL DEFAULT 0
);
```

### 2.8 Importação

```sql
-- Perfil de importação multi-formato. Duas origens: a biblioteca embutida no
-- binário (profiles/*.yaml, ADR-015) e os overrides/aprendizagens do utilizador,
-- que vivem aqui. O utilizador nunca parte do zero se a instituição for coberta.
CREATE TABLE import_profiles (
  id                 TEXT PRIMARY KEY,
  account_id         TEXT REFERENCES accounts(id),
  institution_slug   TEXT,                         -- 'nubank','itau','bb','bradesco','c6',...
  label              TEXT NOT NULL,

  source_format      TEXT NOT NULL CHECK (source_format IN ('csv','ofx','camt053')),
  -- Assinatura de deteção, com semântica por formato:
  --   ofx/camt053 -> ACCTID / IBAN (roteamento exato, sem heurística)
  --   csv         -> hash dos nomes de coluna normalizados
  signature          TEXT NOT NULL,
  detection_json     TEXT,                         -- tokens de cabeçalho, confiança mínima

  section_rules_json TEXT,                         -- [{ match:'final (?P<last4>\\d{4})', action:'set_target_account' }]
  card_rules_json    TEXT,                         -- deteção de pagamento de fatura e emparelhamento (ADR-014)
  -- Números que a fonte declara e que servem de invariantes de verificação.
  -- Em CSV vêm de linhas-resumo (regex); em OFX, de <LEDGERBAL>/<AVAILBAL>.
  declared_totals_json TEXT,                       -- { open:'...', close:'...', purchases:'...', count:... }

  origin             TEXT NOT NULL DEFAULT 'user'
                     CHECK (origin IN ('library','user','learned')),
  library_version    TEXT,                         -- versão do perfil de biblioteca substituído
  mapping_json       TEXT NOT NULL,                -- { date:'Data', amount:'Valor', payee:'Histórico', ... }
  parse_options_json TEXT NOT NULL,                -- { delimiter, dateFormat, decimalSeparator, skipLines, encoding }
  ignore_rules_json  TEXT,                         -- linhas-resumo a descartar
  payee_rules_json   TEXT,                         -- expressões de limpeza específicas do banco
  auto_commit        INTEGER NOT NULL DEFAULT 0,   -- importar sem confirmação se confiança alta
  hits               INTEGER NOT NULL DEFAULT 0,
  last_used_at       INTEGER,
  created_at         INTEGER NOT NULL
);
CREATE UNIQUE INDEX ux_profiles_signature
  ON import_profiles(source_format, signature, coalesce(account_id, ''));
CREATE INDEX ix_profiles_institution ON import_profiles(institution_slug, source_format);

CREATE TABLE import_batches (
  id                    TEXT PRIMARY KEY,
  -- account_id é a conta PRINCIPAL do lote. Um lote pode alimentar várias contas
  -- (fatura com titular e adicional); o destino por linha está em import_rows.
  account_id            TEXT REFERENCES accounts(id),
  profile_id            TEXT REFERENCES import_profiles(id),
  source_format         TEXT NOT NULL CHECK (source_format IN ('csv','ofx','camt053')),
  origin                TEXT NOT NULL CHECK (origin IN ('upload','watcher','imap','api','manual','reprocess')),
  original_filename     TEXT,
  original_path         TEXT,                      -- relativo a /data/uploads: permite reprocessar
  file_sha256           TEXT NOT NULL,
  file_bytes            INTEGER,
  status                TEXT NOT NULL CHECK (status IN
                          ('staging','ready','committed','failed','undone')),
  rows_total            INTEGER NOT NULL DEFAULT 0,
  rows_added            INTEGER NOT NULL DEFAULT 0,
  rows_updated          INTEGER NOT NULL DEFAULT 0,
  rows_skipped          INTEGER NOT NULL DEFAULT 0,
  rows_error            INTEGER NOT NULL DEFAULT 0,
  period_start          TEXT,
  period_end            TEXT,
  -- Reconciliação: valores declarados pela fonte vs. calculados a partir das linhas
  declared_totals_json  TEXT,                      -- { open, close, purchases, payments, count, ... }
  reconciliation_json   TEXT,                      -- [{ kind:'balance'|'total'|'count', declared, computed, delta, ok }]
  declared_open_cents   INTEGER,
  declared_close_cents  INTEGER,
  computed_open_cents   INTEGER,
  computed_close_cents  INTEGER,
  balance_check         TEXT CHECK (balance_check IN ('ok','mismatch','unavailable')),
  diagnostics_json      TEXT,
  created_at            INTEGER NOT NULL,
  committed_at          INTEGER,
  undone_at             INTEGER
);
CREATE INDEX ix_batches_account ON import_batches(account_id, created_at DESC);
CREATE INDEX ix_batches_period  ON import_batches(account_id, period_end DESC);  -- painel de cobertura
CREATE UNIQUE INDEX ux_batches_file_committed
  ON import_batches(coalesce(account_id, ''), file_sha256) WHERE status = 'committed';

CREATE TABLE import_rows (
  id                    TEXT PRIMARY KEY,
  batch_id              TEXT NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  line_no               INTEGER NOT NULL,          -- linha no ficheiro original
  raw_json              TEXT NOT NULL,             -- linha tal como veio, sem interpretação
  normalized_json       TEXT,                      -- { date, amount_cents, payee, notes, ... }
  row_hash              TEXT NOT NULL,             -- hash do conteúdo normalizado
  occurrence_index      INTEGER NOT NULL DEFAULT 0,-- desambigua lançamentos idênticos no mesmo dia
  derived_imported_id   TEXT,
  -- Conta de destino desta linha: permite dividir uma fatura por cartão num só lote (ADR-014)
  target_account_id     TEXT REFERENCES accounts(id),
  section_label         TEXT,                      -- 'Cartao final 1234', para agrupar na pré-visualização
  status                TEXT NOT NULL CHECK (status IN
                          ('new','duplicate','updated','ignored','error','committed')),
  matched_tx_id         TEXT,                      -- transação existente que será fundida
  planned_action        TEXT,                      -- 'insert' | 'update' | 'skip' | 'transfer_pair'
  planned_changes_json  TEXT,
  diagnostics_json      TEXT,                      -- [{level, code, field, value}]
  committed_tx_id       TEXT,
  UNIQUE (batch_id, row_hash, occurrence_index)
);
CREATE INDEX ix_import_rows_batch ON import_rows(batch_id, status);
CREATE INDEX ix_import_rows_target ON import_rows(batch_id, target_account_id);
```

Guardar **todas** as linhas em `staging`, incluindo as descartadas, é o que permite: pré-visualizar sem escrever, aprovar linha a linha, explicar porque algo foi ignorado e reverter o lote inteiro depois.

**Reprocessamento.** Como `original_path` fica registado, um lote existe independentemente do ficheiro: corrigir o mapeamento de um perfil e chamar `POST /imports/{id}/reprocess` produz um novo lote a partir do mesmo original, sem novo *upload*. É o que permite corrigir um mês inteiro retroativamente quando o perfil estava errado.

**Emparelhamento de fatura (ADR-014).** O `imported_id` do lançamento de pagamento é o `batch_id` da fatura emparelhada. Isto torna o emparelhamento determinístico e idempotente: reprocessar não cria uma segunda transferência.

### 2.9 Auditoria e jobs

```sql
CREATE TABLE audit_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,         -- 'transaction','account','category','budget_allocation',...
  entity_id   TEXT NOT NULL,
  action      TEXT NOT NULL,         -- 'create','update','delete','merge','import_commit','import_undo'
  before_json TEXT,
  after_json  TEXT,
  actor       TEXT,                  -- 'user:<id>' | 'system' | 'job:<id>'
  origin      TEXT,                  -- 'ui','import','rule','api'
  at          INTEGER NOT NULL
);
CREATE INDEX ix_audit_entity ON audit_log(entity, entity_id, at DESC);
CREATE INDEX ix_audit_at     ON audit_log(at DESC);

CREATE TABLE jobs (
  id            TEXT PRIMARY KEY,
  type          TEXT NOT NULL,       -- 'import.watcher','import.imap','budget.recompute','backup.snapshot'
  payload_json  TEXT,
  run_at        INTEGER NOT NULL,
  status        TEXT NOT NULL CHECK (status IN
                  ('pending','running','done','failed','cancelled')),
  priority      INTEGER NOT NULL DEFAULT 0,
  attempts      INTEGER NOT NULL DEFAULT 0,
  max_attempts  INTEGER NOT NULL DEFAULT 5,
  locked_at     INTEGER,
  progress      REAL,
  last_error    TEXT,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX ix_jobs_ready ON jobs(status, run_at, priority DESC);
```

### 2.10 Autenticação e definições

```sql
CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT,                -- Argon2id
  totp_secret   TEXT,
  locale        TEXT NOT NULL DEFAULT 'pt-BR',
  timezone      TEXT NOT NULL DEFAULT 'America/Sao_Paulo',
  currency      TEXT NOT NULL DEFAULT 'BRL',
  created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,   -- o token em claro nunca é persistido
  expires_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  user_agent TEXT,
  ip         TEXT
);
CREATE INDEX ix_sessions_expiry ON sessions(expires_at);

CREATE TABLE app_settings (
  key        TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);
```

### 2.11 Busca textual

```sql
CREATE VIRTUAL TABLE transactions_fts USING fts5(
  imported_payee,
  payee_name,
  notes,
  content='',
  tokenize="unicode61 remove_diacritics 2"
);
```

`remove_diacritics 2` é essencial em português: permite encontrar «mercado» ao procurar «mérCAdo». A tabela é mantida por gatilhos ou pelo mutator, e a UI usa-a para a barra de busca global.

---

## 3. Estratégia de deduplicação

O coração da importação fiável. Três mecanismos complementares:

### 3.1 `imported_id` determinístico

```
1. Se o perfil mapeia uma coluna de identificador estável (FITID de OFX,
   «ID», «Nº documento», «Identificador»):
      imported_id = "<bank_slug>:<valor normalizado>"
2. Caso contrário:
      imported_id = sha1(account_id | date | amount_cents |
                         payee_normalizado | occurrence_index)
      onde occurrence_index = ordinal da linha entre as linhas do MESMO
      lote com a mesma tupla (date, amount_cents, payee_normalizado)
```

Consequência prática: **reimportar o mesmo ficheiro é inócuo** — cada linha reconhece o seu identificador. E dois cafés iguais no mesmo dia geram `occurrence_index` 0 e 1, ficando ambos registados em vez de um ser descartado como duplicado.

### 3.2 Hash de ficheiro

`import_batches(account_id, file_sha256)` com índice único parcial em `status = 'committed'` deteta a reintrodução do mesmo ficheiro e marca o lote como já importado, sem sequer percorrer as linhas.

### 3.3 Casamento por conteúdo (*matching* em camadas)

Ordem de tentativa, da maior para a menor fidelidade:

| Camada | Critério | Ação |
| --- | --- | --- |
| T0 | `imported_id` igual, mesma conta | Fundir (atualizar campos), nunca duplicar |
| T1 | Ficheiro já importado (`file_sha256`) | Marcar tudo como `duplicate`, não escrever |
| T2 | Mesma conta, `|Δdata| ≤ 7d`, mesmo `amount_cents`, similaridade de payee ≥ 0.85 | Fundir com a transação existente (preferir os dados do banco para data/valor) |
| T3 | Candidatos restantes: emparelhamento guloso 1-para-1 pelo mais próximo em data | Evita que uma transação «absorva» várias linhas |
| T4 | Conta diferente, valor oposto, `\|Δdata\| ≤ 3d` | Marcar transferência (`transfer_id`) |
| T5 | **Pagamento de fatura**: débito na conta corrente cujo valor iguala o `declared_close_cents` (ou a soma do ciclo) de um lote de fatura de um cartão | Transferência automática para a conta do cartão, com `imported_id` = `batch_id` da fatura (ADR-014) |

A camada T5 é a que impede o erro de contabilidade mais provável em quem tem vários cartões: a fatura traz as compras e a conta corrente traz uma linha de pagamento. Sem emparelhamento, a despesa é contada duas vezes.

Onde existe um identificador exato (o total declarado da fatura), **não se usa heurística**. As camadas T2/T4 usam janelas de data porque não há melhor informação; T5 não precisa.

Campos em conflito seguem a regra: dados do banco vencem em `date`, `amount_cents` e `imported_payee`; dados do utilizador vencem em `category_id`, `notes`, `payee_id`, `cleared` e `reconciled`. Esta regra é **explícita no código** e testada, porque é a origem mais comum de «perdi a minha categorização».

---

## 4. Migração futura para PostgreSQL

Isolamento deliberado: todo o SQL vive em `internal/data/`. Se o volume crescer (multi-utilizador real, dezenas de milhões de transações) ou se for necessária replicação, a migração implica:

1. Substituir o *driver* (`modernc.org/sqlite` → `pgx`) e o dialeto SQL.
2. Converter `REAL` de `sort_order` em `bigint` com *gaps*.
3. Substituir `FTS5` por `tsvector`.
4. Reescrever as consultas de agregação específicas.
5. Reavaliar as `CHECK` e índices parciais (funcionam igual, mas com sintaxe a confirmar).

Nada disto toca em `internal/domain`, `internal/modules` nem em `internal/adapters`. É exatamente esse o benefício de manter a lógica de negócio fora da camada de dados — e em Go isso é verificável por lint: **`internal/domain` não pode importar `internal/data`**.
