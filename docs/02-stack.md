# 02 — Linguagens e Bibliotecas

Critério de escolha, por ordem de prioridade:

1. **Corre em ARM64 sem compilação local** (binário pré-compilado, WASM, Go, ou JS puro).
2. **Tipagem estática forte** e boa integração com TypeScript.
3. **Poucas dependências transitivas** — menos superfície de manutenção a longo prazo.
4. **Longevidade** — projetos ativos, sem dependência de um único mantenedor frágil.

---

## 1. Linguagem

| Camada | Escolha | Justificativa |
| --- | --- | --- |
| Backend | **TypeScript 5.x em Node.js 22 LTS** (ou 24 LTS quando estável) | Um só idioma em todo o projeto; `node:sqlite` disponível; *prebuilds* oficiais para `linux-arm64`; partilha de código de domínio com o frontend |
| Frontend | **TypeScript + React 19** | Ecossistema maduro para tabelas densas, gráficos e acessibilidade |
| Base de dados | **SQL** (SQLite) | Sem ORM-obrigatório; a linguagem de consulta é o próprio SQL |
| Automação/CLI | TypeScript com `tsx` | Reutiliza os mesmos pacotes de domínio e importação |

**Porque não Go/Python/Rust no backend?** Seriam mais rápidos ou mais «limpos», mas quebrariam a partilha de código com o browser. O motor de importação precisa de correr *nos dois lados* (preview no browser, commit no servidor); isso é decisivo. Go fica no radar apenas para um utilitário de importação em massa, se algum dia for necessário.

---

## 2. Monorepo e build

| Necessidade | Escolha | Notas |
| --- | --- | --- |
| Gestão de workspace | **pnpm workspaces** | *Store* com *hardlinks*, rápido, disciplina de dependências |
| Orquestração de tarefas | **Turborepo** | *Cache* de builds e execução de tarefas por pacote |
| Bundler do frontend | **Vite** | *Dev server* instantâneo, HMR, suporte a Web Workers e WASM |
| Compilação do backend | **tsc** (ou `tsup` para empacotar) | Simples e previsível |
| Compilação de gramáticas | **Peggy** + pequeno plugin Vite | Necessário para a linguagem de expressões de orçamento |
| Recolha de tipos | `tsc --noEmit` em CI | Barato e apanha a maioria dos erros |

### Layout do monorepo

```
apps/
  server/          # Fastify: API, jobs, motor de importação no servidor, CLI
  web/             # React SPA + PWA + web worker de importação
packages/
  contracts/       # Schemas Zod + tipos de API partilhados
  domain/          # dinheiro, datas, orçamento, recorrências (puro, sem I/O)
  rules/           # motor de regras DSL
  import-core/     # pipeline CSV isomórfico: parse, perfil, normalização, dedupe
  adapters/        # um módulo por banco: deteção, mapeamento, normalização
  ui/              # componentes partilhados
data/              # volume persistente: db.sqlite, uploads/, inbox/, attachments/
```

---

## 3. Backend

| Necessidade | Escolha | Porquê esta, e não outra |
| --- | --- | --- |
| Servidor HTTP | **Fastify** | Desempenho, sistema de *plugins*, suporte de primeira classe a *streams* e SSE. Alternativa Hono é mais leve, mas menos «baterias incluídas» |
| Validação | **Zod** (+ `fastify-type-provider-zod`) | Fonte única de verdade partilhada com o frontend; inferência de tipos automática |
| Acesso a dados | **Drizzle ORM** sobre `better-sqlite3` | Tipagem em TypeScript, migrações versionadas, e escape fácil para SQL cru (necessário para agregações e para a busca) |
| Driver SQLite | **better-sqlite3** (primário) / **`node:sqlite`** (alternativa sem dependência nativa) | `better-sqlite3` tem *prebuilds* `linux-arm64`; `node:sqlite` elimina a dependência nativa por completo, ao custo de menor funcionalidade |
| Migrações | **drizzle-kit** + ficheiros SQL versionados | Revisão de SQL em *pull request*, execução no arranque com *lock* |
| Jobs em background | **fila persistida em SQLite** + `croner` | Evita Redis. Estados `pending/running/done/failed` com *retry* e *backoff*; agendamento com fuso `America/Sao_Paulo` |
| Trabalho pesado em CPU | **`node:worker_threads`** | Isola a importação de ficheiros grandes do *event loop* do servidor |
| Logs | **pino** | JSON estruturado, rápido, baixo *overhead* |
| Autenticação | **better-auth** ou sessões próprias com **`@node-rs/argon2`** | Argon2id com *prebuilds* `aarch64`; suporte a *passkeys* e TOTP |
| Email/IMAP | **imapflow** | Puro JS, sem compilação nativa; traz os extratos enviados por email |
| Watch de ficheiros | **chokidar** | *Inbox* de ficheiros: largar o CSV numa pasta partilhada e ser importado |
| XML (CAMT.053) | **fast-xml-parser** | ISO 20022 é o formato mais estável de alguns bancos |

---

## 4. Motor de importação CSV (o diferencial)

| Necessidade | Escolha | Porquê |
| --- | --- | --- |
| Parsing CSV no servidor | **csv-parse** | *Streaming*, `relax_column_count`, BOM, tolerância a aspas mal formadas |
| Parsing no browser | **PapaParse** | Funciona em *web worker*, com *preview* progressivo |
| Deteção de codificação | **chardet** + **iconv-lite** | Bancos brasileiros emitem com frequência CP1252/ISO-8859-1, não UTF-8 |
| Deteção de delimitador e de cabeçalho | heurística própria + `csv-sniffer` como referência | Regras de negócio próprias (linhas de preâmbulo, totais) |
| Datas | **date-fns** | Imutável, modular, com `parse` por formato e validação. Evitar `moment` (legado) e `Date` nativo para datas civis |
| Decimais | **decimal.js** apenas na fronteira | A conversão `"1.234,56" → 123456` é feita com parser próprio; o resto do sistema só conhece inteiros |
| Similaridade de *payee* | **fuzzball** (`token_sort_ratio` + `partial_ratio`) | Escolha de limiar e de algoritmo explícita e testável |
| Normalização de texto | **NFKD** nativo + mapa de sinónimos próprio | Remover acentos e ruído bancário, preservando o texto original para exibição |
| Parsing de OFX | **ofx-js** / parser próprio | Alternativa ao CSV para bancos que exportam OFX (mais fiável: traz `FITID`) |
| PDF (fase posterior) | **unpdf** | Puro JS/WASM, sem binários nativos |

Todas estas bibliotecas são JavaScript puro ou WASM — **nenhuma exige toolchain de compilação em ARM64**.

---

## 5. Frontend

| Necessidade | Escolha | Porquê |
| --- | --- | --- |
| Estado de servidor | **TanStack Query** | *Cache*, invalidação, revalidação e concorrência otimista com pouco código |
| Estado de UI | **Zustand** | Pequeno e explícito. Evita a cerimónia do Redux do Actual |
| Rotas | **TanStack Router** | Rotas tipadas, *search params* como estado de filtros (partilhável por URL) |
| Tabelas | **TanStack Table** + **TanStack Virtual** | Listas de transações com milhares de linhas exigem virtualização |
| Formulários | **react-hook-form** + **Zod resolver** | Mesmos *schemas* do servidor |
| Estilo/UI | **Tailwind CSS** + **shadcn/ui** | Velocidade de desenvolvimento e controlo total do markup |
| Gráficos | **Recharts** | Mesma escolha do Actual; declarativo e suficiente |
| Datas na UI | **date-fns** | Consistência com o backend |
| i18n | **i18next** + `react-i18next` | Português (pt-BR/pt-PT) desde o início |
| PWA | **vite-plugin-pwa** | Instalável no telemóvel com ícone e *cache* de recursos |
| Ícones | **lucide-react** | Consistente e leve |

**Porque não Next.js?** Não há necessidade de SSR nem de SEO; é uma SPA autenticada. Vite gera um *bundle* estático servido pelo próprio Fastify, mantendo um único processo em ARM64.

---

## 6. Testes e qualidade

| Necessidade | Escolha |
| --- | --- |
| Testes unitários e de integração | **Vitest** |
| Testes de propriedades | **fast-check** |
| E2E | **Playwright** (correr em CI x64; em ARM64 é possível mas pesado) |
| Lint / format | **ESLint** + **Prettier**; **oxlint** opcional para velocidade |
| Dependências mortas | **knip** |
| Cobertura mínima | `import-core` e `domain`: 90%; restante: sem meta rígida |

---

## 7. Operação

| Necessidade | Escolha | Notas ARM64 |
| --- | --- | --- |
| Contentor | **Docker com `node:22-bookworm-slim` multi-arch** | Evitar Alpine: *prebuilds* de módulos nativos assumem glibc; musl causa recompilação em runtime |
| Reverse proxy + TLS | **Caddy** | Certificados automáticos, configuração de 5 linhas |
| Backup contínuo | **Litestream** | Binário Go com *release* `linux-arm64`; replica WAL para S3/B2 |
| Backup periódico | **restic** | Snapshots cifrados de anexos e exportações |
| Acesso privado | **Tailscale** | Elimina a exposição de portas à internet |
| Monitorização | *Healthcheck* do Docker + **Uptime Kuma** ou alerta simples por email/Telegram | Evitar stack Prometheus/Grafana no MVP |
| Atualizações | `docker compose pull && up -d` via *script* ou **Watchtower** com janela de manutenção | Manter *tag* de versão explícita, não `latest` |

### Dependências a evitar explicitamente

| Biblioteca | Motivo para evitar |
| --- | --- |
| `bcrypt` | Compilação nativa; `@node-rs/argon2` tem *prebuilds* `aarch64` |
| `canvas` / `node-canvas` | Exige `cairo`/`pango`; manutenção penosa |
| `sharp` | Funciona em ARM64, mas só é necessário se houver redimensionamento de imagens — não é o caso |
| `moment` | Legado, *bundle* grande; usar `date-fns` |
| `sql.js` no servidor | Só faz sentido no browser; no Node usar `better-sqlite3`/`node:sqlite` |
| Prisma | Motor binário pesado e sobreposição com Drizzle; desnecessário neste âmbito |
| Redis / BullMQ | Serviço extra para uma carga que uma tabela SQLite resolve |
| `puppeteer` | *Download* de Chromium ARM64 desnecessário no runtime |

---

## 8. Versões e política de atualização

- `engines.node` fixado no `package.json`; `.nvmrc` no repositório.
- Dependabot/Renovate **semanal**, com *auto-merge* só para *patches* e apenas com testes verdes.
- Atualizações de dependência nativa (SQLite) testadas em CI ARM64 antes de entrar em `main`.
- Imagens Docker com *tag* de versão; a base atualiza-se por PR deliberado, não silenciosamente.
