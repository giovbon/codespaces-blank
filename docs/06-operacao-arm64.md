# 06 — Operação em ARM64

Objetivo: um serviço que corre meses sem intervenção, que se atualiza sem dor e cujo restauro é um procedimento treinado, não uma esperança.

---

## 1. Dimensionamento

| Recurso | Mínimo | Recomendado | Justificativa |
| --- | --- | --- | --- |
| CPU | 2 núcleos ARM64 (Pi 4 4 GB) | 4 núcleos (Pi 5 8 GB, Ampere, mini-PC ARM) | Importações usam *worker thread*; resto é I/O |
| RAM | 2 GB | 4–8 GB | Node + SQLite + *cache* de páginas; o pico é o *parsing* de um CSV grande |
| Armazenamento | 16 GB | SSD de 128 GB+ (USB 3 ou NVMe) | Ficheiros, anexos e backups locais; **evitar cartão SD** pelo desgaste de escrita |
| Rede | — | Cabo Ethernet | Importações agendadas e IMAP beneficiaram de ligação estável |

Carga esperada: menos de 5% de CPU em regime normal, com picos curtos durante importações. Um Pi 5 sobra largamente.

---

## 2. Layout de dados

Volume único montado em `/data`, versionado com o resto da configuração:

```
/data
  db.sqlite            base de dados (mais db.sqlite-wal e db.sqlite-shm)
  uploads/             ficheiros originais de importação, por ano/mês
  inbox/               pasta vigiada: <slug-conta>/ para importação automática
  processed/           ficheiros já importados, movidos com o id do lote no nome
  failed/              ficheiros com erro, preservados para diagnóstico
  attachments/         comprovativos
  exports/             exportações JSON/CSV agendadas
  backups/             VACUUM INTO noturno e snapshots restic
```

`uploads/` guarda tudo indefinidamente: é a prova documental de cada importação e permite responder a «porque é que este valor está aqui?» dois anos depois.

---

## 3. `docker-compose.yml` de referência

```yaml
services:
  app:
    image: ghcr.io/<org>/financas:1.4.2        # tag explícita, nunca latest
    platform: linux/arm64
    restart: unless-stopped
    environment:
      NODE_ENV: production
      PORT: "3000"
      DATA_DIR: /data
      TZ: America/Sao_Paulo
      BASE_URL: https://financas.exemplo.pt
      SESSION_SECRET_FILE: /run/secrets/session_secret
      ARGON2_MEMORY_COST: "65536"
    volumes:
      - /srv/financas/data:/data
    secrets: [session_secret]
    healthcheck:
      test: ["CMD", "node", "-e",
             "fetch('http://127.0.0.1:3000/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 20s
    mem_limit: 1536m

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

Notas: `mem_limit` impede que uma importação problemática consuma toda a RAM; `TZ` fixa o fuso para os agendamentos; `secrets` evita segredos em variáveis de ambiente visíveis por `docker inspect`.

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
| CSRF | *Token* por sessão em métodos mutantes + `SameSite`; API aceita token por cabeçalho em vez de *cookie* |
| XSS | Política de segurança de conteúdo estrita no Caddy; sem `innerHTML` no cliente |
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
| Erros não tratados | contador em `/metrics` | > 5/hora |
| Desgaste do armazenamento | SMART quando disponível | setores realocados > 0 |

Canal de alerta: email ou Telegram a partir de um pequeno *script* no próprio host. Não vale a pena montar Prometheus + Grafana para um serviço único.

---

## 7. Ciclo de atualização

```bash
# 1. construir/obter nova imagem
docker compose pull

# 2. backup explícito antes de migração de esquema
docker compose exec app node dist/cli.js db:snapshot

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
- Testar a atualização em ARM64 antes de aplicar em produção: dependências nativas podem ter *prebuilds* apenas para `x64`.

---

## 8. *Runbook* de incidentes

| Sintoma | Diagnóstico | Ação |
| --- | --- | --- |
| Interface não abre | `docker compose ps`, `docker compose logs app` | Reiniciar contentor; se persistir, verificar espaço em disco |
| Aplicação lenta em toda a operação | `top`, tamanho do WAL, `PRAGMA wal_checkpoint` | Forçar *checkpoint*; verificar importação presa em `running` |
| Transações a faltar | Consultar `audit_log` por `origin = 'import'` | Usar `undo` do lote suspeito |
| Duplicados após importação | Verificar `imported_id` nulos nas linhas afetadas | Corrigir perfil (coluna de ID mal mapeada) e reimportar; remover duplicados com a ferramenta de fusão |
| Saldos errados numa conta | Comparar com extrato; correr verificação de reconciliação | Ajustar via transação de conciliação, não por edição de saldos |
| Base de dados *locked* | `busy_timeout`, jobs presos em `running` | Matar o *job* obsoleto, libertar `locked_at`; verificar transações longas |
| Falha de restauro do backup | Verificar integridade com `PRAGMA integrity_check` | Recorrer ao snapshot anterior; registar o incidente |

---

## 9. Custo e simplicidade

O sistema completo são três contentores, um volume, um ficheiro de base de dados e um *script* de backup. Não há orquestrador, não há *cluster*, não há mensageria. Toda a complexidade que resta está onde é útil: no motor de importação. Essa é a escolha arquitetural central — **simplicidade operacional comprada com decisões estruturais firmes** (ADR-001, ADR-002, ADR-009, ADR-010), e não com funcionalidades cortadas.
