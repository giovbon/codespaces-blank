# 06 — Operação em ARM64

Objetivo: um serviço que corre meses sem intervenção, que se atualiza sem dor e cujo restauro é um procedimento treinado, não uma esperança.

---

## 1. Dimensionamento

| Recurso | Mínimo | Recomendado | Justificativa |
| --- | --- | --- | --- |
| CPU | 1 núcleo ARM64 (Pi Zero 2 W, Pi 3) | 2–4 núcleos (Pi 4 4 GB, Pi 5 8 GB, mini-PC ARM) | O binário Go é leve; importações correm em goroutines. **Um host de 1 GB passa a ser viável** — era impossível com Node |
| RAM | 128 MB | 512 MB–1 GB | Binário Go + `page cache` de SQLite. O pico é uma importação de 50 000 linhas, e mesmo essa fica abaixo de 150 MB |
| Armazenamento | 8 GB | SSD de 64 GB+ (USB 3 ou NVMe) | Ficheiros, anexos e backups locais; **evitar cartão SD** pelo desgaste de escrita |
| Rede | — | Cabo Ethernet | Importações agendadas e IMAP beneficiam de ligação estável |

Carga esperada: menos de 2% de CPU em regime normal, com picos curtos durante importações. O consumo em repouso do processo é da ordem das dezenas de MB.

---

## 2. Layout de dados

Volume único montado em `/data`, versionado com o resto da configuração:

```
/data
  db.sqlite            base de dados (mais db.sqlite-wal e db.sqlite-shm)
  uploads/             ficheiros originais de importação, por ano/mês
  inbox/               pasta vigiada ÚNICA — o destino é decidido pelo conteúdo
  processed/           ficheiros já importados, movidos com o id do lote no nome
  failed/              ficheiros com erro ou sem perfil, preservados para diagnóstico
  attachments/         comprovativos
  exports/             exportações JSON/CSV agendadas
  backups/             VACUUM INTO noturno e snapshots restic
```

`inbox/` é **uma só pasta**, sem subpastas por conta: arrastar 9 ficheiros para o mesmo sítio é todo o trabalho mensal de entrega. O encaminhamento é feito por *magic bytes* e impressão digital do emissor (ver [07](07-muitas-contas-e-cartoes.md#3-roteamento-automático-por-conteúdo)). Subpastas por conta continuam a funcionar como *override* manual, para quem preferir impor o destino.

`uploads/` guarda tudo indefinidamente: é a prova documental de cada importação, **e é o que permite reprocessar um lote** com um perfil corrigido, sem novo *upload* (ADR-008).

---

## 3. `docker-compose.yml` de referência

```yaml
services:
  app:
    image: ghcr.io/<org>/financas:1.4.2        # tag explícita, nunca latest
    platform: linux/arm64
    restart: unless-stopped
    environment:
      PORT: "3000"
      DATA_DIR: /data
      TZ: America/Sao_Paulo
      BASE_URL: https://financas.exemplo.pt
      SESSION_SECRET_FILE: /run/secrets/session_secret
      ARGON2_MEMORY_COST: "65536"
      GOMEMLIMIT: 200MiB
    volumes:
      - /srv/financas/data:/data
    secrets: [session_secret]
    healthcheck:
      # imagem distroless não tem shell, curl nem wget:
      # o próprio binário implementa o subcomando de verificação
      test: ["CMD", "/app/app", "healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s                          # arranque é imediato
    mem_limit: 256m
    read_only: true
    tmpfs: [/tmp]                               # espaço temporário com o sistema de ficheiros só de leitura
    security_opt: ["no-new-privileges:true"]
    user: "65532:65532"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports: ["80:80", "443:443"]
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on: [app]

  litestream:
    image: litestream/litestream:0.3
    restart: unless-stopped
    command: replicate -config /etc/litestream.yml
    volumes:
      - ./litestream.yml:/etc/litestream.yml:ro
      - /srv/financas/data:/data
    depends_on: [app]

volumes:
  caddy_data:
  caddy_config:

secrets:
  session_secret:
    file: ./secrets/session_secret
```

`Caddyfile`:

```
financas.exemplo.pt {
    encode zstd gzip
    reverse_proxy app:3000
}
```

Notas:

- `mem_limit: 256m` com `GOMEMLIMIT=200MiB` é suficiente para o pico normal. O `GOMEMLIMIT` faz o coletor do Go trabalhar mais cedo em vez de crescer, evitando `OOM kill`. A memória do SQLite é `page cache` recuperável, não pressão real.
- `read_only: true` com `tmpfs` em `/tmp` é possível porque o binário é estático e não escreve fora de `/data`.
- `TZ` fixa o fuso para os agendamentos; `secrets` evita segredos em variáveis de ambiente visíveis por `docker inspect`.
- **Compromisso de imagem — resolvido.** Com PDF fora de âmbito (ADR-016) não há `poppler` nem `tesseract` a instalar: a imagem final pode ser **`distroless/static`** ou `scratch`, com o binário e mais nada. O tempo em que se ponderava `debian-slim` + `poppler-utils` (~90 MB) desapareceu com a decisão.
- **Sem Node na imagem.** Não há `node_modules`, nem `npm ci` no `build`, nem `prebuilds` a verificar por arquitetura.

---

## 4. Backup e recuperação

Três camadas complementares, por ordem de frequência:

| Camada | Frequência | Retenção | Mecanismo |
| --- | --- | --- | --- |
| Replicação contínua | segundos | 7 dias de histórico | Litestream → Backblaze B2 / S3 / R2 |
| Snapshot consistente | diária, 03:00 | 30 dias | `sqlite3 db.sqlite "VACUUM INTO '/data/backups/db-YYYYMMDD.sqlite'"` |
| Exportação portável | semanal | 12 semanas | JSON + CSVs de todas as tabelas (`exports/`), depois *snapshot* restic cifrado para fora do host |

Configuração Litestream:

```yaml
dbs:
  - path: /data/db.sqlite
    replicas:
      - type: s3
        bucket: financas-backup
        path: pi5
        endpoint: https://s3.us-west-004.backblazeb2.com
        region: us-west-004
        sync-interval: 10s
        retention: 168h
```

**Teste de desgaste:** verificar periodicidade de *checkpoint* do WAL (`wal_autocheckpoint`, `journal_size_limit`) para não escrever mais do que o necessário em armazenamento flash.

### Procedimento de restauro (treinado mensalmente)

1. `docker compose down`.
2. Restaurar `db.sqlite` mais recente de `backups/`, **ou** `litestream restore -o /data/db.sqlite s3://financas-backup/pi5`.
3. Remover `db.sqlite-wal` e `db.sqlite-shm` remanescentes (evita inconsistência entre ficheiro e WAL).
4. Restaurar o volume `attachments/` mais recente por restic.
5. `docker compose up -d`; validar com `GET /healthz`, abrir o mês corrente e conferir o saldo de uma conta.
6. Registar o resultado do ensaio (data, tempo de recuperação, versão do *backup*).

**Regra de ouro**: um backup cujo restauro nunca foi ensaiado não é um backup — é uma suposição.

---

## 5. Segurança

| Vetor | Mitigação |
| --- | --- |
| Acesso público | Fechar a porta 3000 no *host*; única exposição é o proxy com TLS; preferir acesso apenas por **Tailscale** |
| Autenticação | Argon2id (`m=64 MB, t=3, p=4`), sessões em *cookie* `HttpOnly` + `Secure` + `SameSite=Lax`, `token_hash` na base de dados |
| Força bruta | *Rate limit* por IP e por conta, bloqueio exponencial após 5 falhas, TOTP ou *passkey* como segundo fator |
| CSRF | *Token* por sessão em métodos mutantes + `SameSite`; API aceita token por cabeçalho. No HTMX, o token vai num `<meta>` e é injetado globalmente via `hx-headers` no `<body>` — não se esquece nenhuma rota |
| XSS | Escape automático do `templ` (não existe interpolação crua por omissão) + **CSP estrita** no Caddy |
| CSP e `unsafe-eval` | **Ponto de atenção real:** o Alpine.js usa `new Function` para avaliar expressões e o HTMX avalia `hx-on`/`hx-vals` com `js:`. Com uma CSP estrita, é preciso usar a **build CSP do Alpine** (`@alpinejs/csp`, expressões sem `eval`) e **desativar `htmx.config.allowEval`**. Alternativa: aceitar `unsafe-eval` — pior, e desnecessário se a primeira opção for adotada desde o início |
| Conteúdo não confiável na grelha | Dados vindos de extratos são texto, nunca HTML; nenhum fragmento é construído por concatenação de string com dados de ficheiro |
| Segredos | `docker secrets` ou ficheiros com permissão `600`; nunca em variáveis de ambiente nem no repositório |
| Dados em repouso | Opcional: SQLCipher (custo de desempenho) ou cifra do volume no *host*. Em ARM64 sem aceleração de AES, medir antes de adotar |
| Dados em backup | Cifrados pelo restic e pelo Litestream; *bucket* privado com chave de aplicação restrita |
| Dependências | Auditoria periódica; imagem construída a partir de `bookworm-slim` com utilizador não-*root* |
| Superfície de rede | Sem portas de administração abertas; SSH por chave e desativado na internet pública quando possível |

Dados financeiros pessoais são dados sensíveis: o esforço de proteger deve ser proporcional ao dano de os expor, e o dano aqui é alto.

---

## 6. Monitorização e alertas

| O que vigiar | Como | Limiar de alerta |
| --- | --- | --- |
| Serviço vivo | `healthcheck` do Docker + *ping* externo | 2 falhas consecutivas |
| Replicação Litestream | *log* de sincronização | atraso > 60 s |
| Espaço em disco | `df` em *job* diário | > 80% |
| Tamanho do WAL | *job* diário | > 100 MB (indica *checkpoint* a falhar) |
| Falha de importação agendada | estado do *job* `failed` | imediato |
| Divergência de saldo | `balance_check = 'mismatch'` | resumo diário |
| Lote em `staging` por aprovar | *job* diário | > 3 dias |
| Ficheiro de banco que deixou de importar | diagnóstico `ROUTING_AMBIGUOUS` ou `HEADER_ROW_GUESSED` | imediato (indica formato alterado no banco) |
| Pagamento de fatura não emparelhado | diagnóstico `CARD_PAYMENT_UNMATCHED` | resumo diário (indica fatura em falta) |
| Conta sem importação no mês | **painel de cobertura** ([07](07-muitas-contas-e-cartoes.md#6-painel-de-cobertura--importei-tudo-este-mês)) | resumo mensal |
| Erros não tratados | contador em `/metrics` | > 5/hora |
| Desgaste do armazenamento | SMART quando disponível | setores realocados > 0 |

Canal de alerta: email ou Telegram a partir de um pequeno *script* no próprio host. Não vale a pena montar Prometheus + Grafana para um serviço único.

---

## 7. Ciclo de atualização

```bash
# 1. construir/obter nova imagem
docker compose pull

# 2. backup explícito antes de migração de esquema
docker compose exec app /app/app db snapshot

# 3. subir (as migrações correm no arranque, numa transação, com lock)
docker compose up -d

# 4. validar
curl -fsS https://financas.exemplo.pt/healthz
```

Regras:

- Migrações são **sempre aditivas** na mesma versão (adicionar coluna/tabela), com a remoção de campos obsoletos só numa versão posterior. Isto permite reverter a imagem sem reverter a base de dados.
- Cada migração é revista como código em PR e testada contra um *dump* anonimizado da base de dados real.
- Não usar `latest`: uma *tag* de versão permite saber o que está em execução e reverter com precisão.
- Janela de manutenção não é necessária: o serviço é de um utilizador; alguns segundos de indisponibilidade são aceitáveis.
- **`CGO_ENABLED=0` significa que não há dependências nativas para compilar por arquitetura.** A construção é `GOOS=linux GOARCH=arm64 go build` — sem `buildx`, sem *cross-toolchain*, e sem binários externos no *runtime* (ADR-016).

---

## 8. *Runbook* de incidentes

| Sintoma | Diagnóstico | Ação |
| --- | --- | --- |
| Interface não abre | `docker compose ps`, `docker compose logs app` | Reiniciar contentor; se persistir, verificar espaço em disco |
| Aplicação lenta em toda a operação | `top`, tamanho do WAL, `PRAGMA wal_checkpoint` | Forçar *checkpoint*; verificar importação presa em `running` |
| Transações a faltar | Consultar `audit_log` por `origin = 'import'` | Usar `undo` do lote suspeito |
| Duplicados após importação | Verificar `imported_id` nulos nas linhas afetadas | Corrigir perfil (coluna de ID mal mapeada) e reimportar; remover duplicados com a ferramenta de fusão |
| Saldos errados numa conta | Comparar com extrato; correr verificação de reconciliação | Ajustar via transação de conciliação, não por edição de saldos |
| Ficheiro de um banco deixou de importar | Ver `import_batches.diagnostics_json` do lote; procurar `ROUTING_AMBIGUOUS` ou `RAGGED_ROW` | Abrir o ecrã de perfil, corrigir o mapeamento e reprocessar o lote — o original está em `uploads/`, não é preciso novo *upload* |
| Reconciliação «não fecha» numa conta | Ver o diagnóstico explicativo (`RECONCILE_EXPLAINED`) e a linha apontada | Corrigir `ignore_rules_json` (linha-resumo ou bloco de parceladas) e reprocessar |
| Despesas duplicadas por causa de um cartão | Procurar `CARD_PAYMENT_UNMATCHED` e faturas não importadas | Importar a fatura em falta e reprocessar o lote da conta corrente; o emparelhamento é automático |
| Ficheiro em `failed/` | Ver o diagnóstico associado ao lote | Sem perfil para essa instituição → criar perfil na biblioteca; voltar a colocar o ficheiro em `inbox/` |
| Base de dados *locked* | `busy_timeout`, jobs presos em `running` | Matar o *job* obsoleto, libertar `locked_at`; verificar transações longas |
| Falha de restauro do backup | Verificar integridade com `PRAGMA integrity_check` | Recorrer ao snapshot anterior; registar o incidente |

---

## 9. Custo e simplicidade

O sistema completo são três contentores, um volume, um ficheiro de base de dados e um *script* de backup. Não há orquestrador, não há *cluster*, não há mensageria. Toda a complexidade que resta está onde é útil: no motor de importação. Essa é a escolha arquitetural central — **simplicidade operacional comprada com decisões estruturais firmes** (ADR-001, ADR-002, ADR-009, ADR-010), e não com funcionalidades cortadas.
