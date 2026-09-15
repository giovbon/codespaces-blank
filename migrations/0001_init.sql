-- 0001_init.sql — esquema inicial (fatias M0 e M1)
--
-- Âmbito: apenas o que o arranque e a importação de CSV precisam. É deliberado.
-- O modelo-alvo completo está em docs/03; o que fica de fora não se reserva
-- (docs/09 §7): etiquetas, anexos, agendamentos, orçamento, jobs e busca
-- textual entram por migração própria na fase que os usar.
--
-- Invariantes que o esquema impõe (docs/03 §1):
--   * dinheiro é INTEGER em cêntimos — nunca ponto flutuante;
--   * data de transação é TEXT 'YYYY-MM-DD' civil — nunca instante UTC;
--   * instantes são INTEGER em milissegundos epoch;
--   * nunca há DELETE físico: tombstone = 1;
--   * chaves primárias são ULID (TEXT de 26 caracteres).

-- ---------------------------------------------------------------- credenciais
-- Senha única, para até 2 pessoas que não usam ao mesmo tempo (ADR-019).
-- Existe exatamente uma linha. Não há registo, e-mail nem recuperação.
CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  password_hash TEXT NOT NULL,          -- argon2id, nunca em texto simples
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,      -- o token em claro nunca é persistido
  actor_name TEXT,                      -- 'quem está a usar' — só para o audit_log
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

-- -------------------------------------------------------------------- contas
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
  institution_slug     TEXT,             -- liga à biblioteca de perfis (ADR-015)
  account_number_mask  TEXT,
  external_account_id  TEXT,             -- ACCTID (OFX) ou IBAN (CAMT): roteamento exato
  -- Específicos de cartão de crédito (type = 'credit'); ver ADR-014
  card_brand           TEXT,
  card_mask            TEXT,
  card_holder          TEXT,             -- 'titular' | 'adicional:<nome>'
  closing_day          INTEGER,
  due_day              INTEGER,
  credit_limit_cents   INTEGER,
  balance_cents        INTEGER,          -- último saldo declarado pelo banco
  balance_date         TEXT,
  reconcile_start_date TEXT,
  created_at           INTEGER NOT NULL,
  updated_at           INTEGER NOT NULL,
  tombstone            INTEGER NOT NULL DEFAULT 0,
  revision             INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX ix_accounts_order ON accounts(off_budget, sort_order) WHERE tombstone = 0;

CREATE UNIQUE INDEX ux_accounts_external
  ON accounts(external_account_id)
  WHERE external_account_id IS NOT NULL AND tombstone = 0;

-- ---------------------------------------------------------------- categorias
CREATE TABLE category_groups (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order REAL NOT NULL DEFAULT 0,
  hidden     INTEGER NOT NULL DEFAULT 0,
  tombstone  INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);

CREATE TABLE categories (
  id         TEXT PRIMARY KEY,
  group_id   TEXT NOT NULL REFERENCES category_groups(id),
  name       TEXT NOT NULL,
  sort_order REAL NOT NULL DEFAULT 0,
  hidden     INTEGER NOT NULL DEFAULT 0,
  is_income  INTEGER NOT NULL DEFAULT 0,
  goal_json  TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  tombstone  INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_categories_name
  ON categories(group_id, lower(name)) WHERE tombstone = 0;

-- ------------------------------------------------- estabelecimentos e aprendizagem
CREATE TABLE payees (
  id                  TEXT PRIMARY KEY,
  name                TEXT NOT NULL,     -- nome canónico apresentado ao utilizador
  default_category_id TEXT REFERENCES categories(id),
  notes               TEXT,
  created_at          INTEGER NOT NULL,
  updated_at          INTEGER NOT NULL,
  tombstone           INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX ux_payees_name ON payees(lower(name)) WHERE tombstone = 0;

-- Aprendizagem: liga a string bruta normalizada do banco ao payee e categoria
-- escolhidos. Aplicação automática quando hits >= 3 e confidence >= 0.85.
CREATE TABLE payee_mappings (
  raw_normalized TEXT PRIMARY KEY,       -- sem acentos, maiúsculas, sem ruído de banco
  raw_example    TEXT NOT NULL,          -- exemplo original, só para diagnóstico
  payee_id       TEXT REFERENCES payees(id),
  category_id    TEXT REFERENCES categories(id),
  hits           INTEGER NOT NULL DEFAULT 0,
  misses         INTEGER NOT NULL DEFAULT 0,
  confidence     REAL    NOT NULL DEFAULT 0,
  last_used_at   INTEGER
);

-- ----------------------------------------------------------------- importação
-- Duas origens de perfil: a biblioteca embutida no binário (profiles/*.yaml,
-- ADR-015) e os overrides/aprendizagens do utilizador, que vivem aqui.
CREATE TABLE import_profiles (
  id                   TEXT PRIMARY KEY,
  account_id           TEXT REFERENCES accounts(id),
  institution_slug     TEXT,
  label                TEXT NOT NULL,
  source_format        TEXT NOT NULL CHECK (source_format IN ('csv','ofx','camt053')),
  -- Assinatura de deteção, por formato:
  --   ofx/camt053 -> ACCTID / IBAN (roteamento exato, sem heurística)
  --   csv         -> hash dos nomes de coluna normalizados
  signature            TEXT NOT NULL,
  detection_json       TEXT,
  section_rules_json   TEXT,             -- divisão por cartão (ADR-014)
  card_rules_json      TEXT,             -- pagamento de fatura e emparelhamento
  declared_totals_json TEXT,             -- invariantes declarados pela fonte
  origin               TEXT NOT NULL DEFAULT 'user'
                       CHECK (origin IN ('library','user','learned')),
  library_version      TEXT,
  mapping_json         TEXT NOT NULL,
  parse_options_json   TEXT NOT NULL,
  ignore_rules_json    TEXT,
  payee_rules_json     TEXT,
  auto_commit          INTEGER NOT NULL DEFAULT 0,
  hits                 INTEGER NOT NULL DEFAULT 0,
  last_used_at         INTEGER,
  created_at           INTEGER NOT NULL
);

CREATE UNIQUE INDEX ux_profiles_signature
  ON import_profiles(source_format, signature, coalesce(account_id, ''));

CREATE INDEX ix_profiles_institution ON import_profiles(institution_slug, source_format);

CREATE TABLE import_batches (
  id                    TEXT PRIMARY KEY,
  -- account_id é a conta PRINCIPAL do lote; o destino por linha está em
  -- import_rows.target_account_id (fatura com titular e adicional).
  account_id            TEXT REFERENCES accounts(id),
  profile_id            TEXT REFERENCES import_profiles(id),
  source_format         TEXT NOT NULL CHECK (source_format IN ('csv','ofx','camt053')),
  origin                TEXT NOT NULL CHECK (origin IN
                          ('upload','watcher','imap','api','manual','reprocess')),
  original_filename     TEXT,
  original_path         TEXT,            -- relativo a /data/uploads: permite reprocessar
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
  -- Reconciliação: o que a fonte declarou contra o que as linhas calculam
  declared_totals_json  TEXT,
  reconciliation_json   TEXT,
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
CREATE INDEX ix_batches_period  ON import_batches(account_id, period_end DESC);

CREATE UNIQUE INDEX ux_batches_file_committed
  ON import_batches(coalesce(account_id, ''), file_sha256) WHERE status = 'committed';

CREATE TABLE import_rows (
  id                   TEXT PRIMARY KEY,
  batch_id             TEXT NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
  line_no              INTEGER NOT NULL,
  raw_json             TEXT NOT NULL,    -- linha tal como veio, sem interpretação
  normalized_json      TEXT,
  row_hash             TEXT NOT NULL,
  occurrence_index     INTEGER NOT NULL DEFAULT 0,
  derived_imported_id  TEXT,
  target_account_id    TEXT REFERENCES accounts(id),
  section_label        TEXT,
  status               TEXT NOT NULL CHECK (status IN
                         ('new','duplicate','updated','ignored','error','committed')),
  matched_tx_id        TEXT,
  planned_action       TEXT,
  planned_changes_json TEXT,
  diagnostics_json     TEXT,
  committed_tx_id      TEXT,
  UNIQUE (batch_id, row_hash, occurrence_index)
);

CREATE INDEX ix_import_rows_batch  ON import_rows(batch_id, status);
CREATE INDEX ix_import_rows_target ON import_rows(batch_id, target_account_id);

-- --------------------------------------------------------------- transações
CREATE TABLE transactions (
  id              TEXT PRIMARY KEY,
  account_id      TEXT NOT NULL REFERENCES accounts(id),
  date            TEXT NOT NULL,         -- YYYY-MM-DD
  amount_cents    INTEGER NOT NULL,      -- negativo = saída
  currency        TEXT NOT NULL DEFAULT 'BRL',
  payee_id        TEXT REFERENCES payees(id),
  category_id     TEXT REFERENCES categories(id),
  notes           TEXT,
  cleared         INTEGER NOT NULL DEFAULT 0,
  reconciled      INTEGER NOT NULL DEFAULT 0,
  is_parent       INTEGER NOT NULL DEFAULT 0,
  parent_id       TEXT REFERENCES transactions(id),
  transfer_id     TEXT,                  -- par de transferência entre contas
  imported_id     TEXT,                  -- identificador determinístico da linha
  imported_payee  TEXT,                  -- descrição bruta, preservada
  import_batch_id TEXT REFERENCES import_batches(id),
  source          TEXT NOT NULL DEFAULT 'manual'
                  CHECK (source IN ('manual','csv','ofx','camt','api')),
  sort_order      REAL NOT NULL DEFAULT 0,
  raw_data        TEXT,                  -- JSON da linha original, para auditoria
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  tombstone       INTEGER NOT NULL DEFAULT 0,
  revision        INTEGER NOT NULL DEFAULT 1
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

-- -------------------------------------------------------------------- regras
CREATE TABLE rules (
  id              TEXT PRIMARY KEY,
  stage           TEXT NOT NULL DEFAULT 'default'
                  CHECK (stage IN ('pre','default','post')),
  conditions_json TEXT NOT NULL,
  actions_json    TEXT NOT NULL,
  enabled         INTEGER NOT NULL DEFAULT 1,
  sort_order      INTEGER NOT NULL DEFAULT 0,
  description     TEXT,
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  tombstone       INTEGER NOT NULL DEFAULT 0
);

-- ----------------------------------------------------------------- auditoria
-- Nunca há UPDATE nem DELETE nesta tabela: é o que torna o undo do lote e a
-- explicação de uma decisão possíveis (ADR-007).
CREATE TABLE audit_log (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  action      TEXT NOT NULL,
  before_json TEXT,
  after_json  TEXT,
  actor       TEXT,                      -- 'user:<nome>' | 'system' | 'job:<id>'
  origin      TEXT,                      -- 'ui','import','rule','api'
  at          INTEGER NOT NULL
);

CREATE INDEX ix_audit_entity ON audit_log(entity, entity_id, at DESC);
CREATE INDEX ix_audit_at     ON audit_log(at DESC);
