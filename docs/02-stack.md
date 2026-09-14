# 02 — Linguagens e Bibliotecas

Critério de escolha, por ordem de prioridade:

1. **Corre em ARM64 sem compilação local** — binário pré-compilado, WASM, ou Go puro. Nada que exija *toolchain* de C no host.
2. **Tipagem estática forte** — o motor de importação lida com formatos ambíguos; os tipos são a primeira linha de defesa.
3. **Poucas dependências transitivas** — menos superfície de manutenção a longo prazo.
4. **Longevidade** — projetos ativos, sem dependência de um único mantenedor frágil.
5. **Respostas prontas na internet** — para um developer a solo que se ausenta e regressa, uma tecnologia popular vale mais do que uma elegante. Este critério é o que decide empates.

Contexto de decisão: [ADR-012](05-decisoes-adr.md#adr-012--go-com-ui-renderizada-no-servidor-templ--htmx--alpinejs) (linguagem e UI), [ADR-013](05-decisoes-adr.md#adr-013--ingestão-por-adapters-csv-ofx-camt053-com-roteamento-por-conteúdo) (ingestão por adapters) e [ADR-016](05-decisoes-adr.md#adr-016--pdf-fora-de-âmbito) (**PDF fora de âmbito**).

---

## 1. Linguagem

| Camada | Escolha | Justificativa |
| --- | --- | --- |
| Backend | **Go** (versão estável mais recente; `toolchain` fixado em `go.mod`) | Uma linguagem para HTTP, domínio, importação, *jobs* e CLI. Binário único, arranque em milissegundos, RAM em dezenas de MB, concorrência nativa |
| UI | **HTML renderizado no servidor** — `templ` + HTMX + Alpine.js | Sem SPA, sem *bundler*, sem cliente de API, sem `package.json` |
| CSS | **Tailwind CSS** com o binário ***standalone*** | Existe para `linux-arm64`; gerado em tempo de *build*. **Sem Node em lado nenhum do projeto** |
| Base de dados | **SQL** (SQLite) | Sem ORM; a linguagem de consulta é o próprio SQL |
| Acesso a dados | `database/sql` + SQL escrito à mão | Ver §3 |
| CLI | Go, no mesmo binário (`app import --file …`) | Reutiliza serviços e domínio; sem segundo *runtime* |

### Porque não as alternativas

| Alternativa | Porque não |
| --- | --- |
| **TypeScript ponta a ponta** | Foi a decisão anterior (ADR-003). O argumento decisivo era o núcleo isomórfico, removido com o ADR-001 revisto. Sem ele, resta RAM 3–5× superior, `node_modules` na imagem, risco de módulos nativos em ARM64 e ausência de binário único. Não é *falso*, é pior em todos os eixos que este projeto valoriza |
| **Python** | Excelente para *parsing* de PDF e para ML. Nenhum dos dois está no âmbito (ADR-016, ADR-006), e o pipeline real é heurística de texto e aritmética exata — tudo coberto por Go. Um segundo *runtime* quebraria a premissa de operação simples |
| **Rust** | Desempenho irrelevante aqui (o gargalo é I/O e rede bancária); custo de desenvolvimento e tempo de compilação proibitivos a solo |
| **Node + SPA (React/Solid)** | Ver ADR-012. Solid em particular: tecnicamente bom, ecossistema insuficiente (gráficos, componentes), e preterido pelo critério 5 |
| **Java/Kotlin** | Ecossistema maduro, mas peso de JVM e *runtime* desproporcionado num host ARM64 modesto |

### Notas de linguagem relevantes para o domínio

- `int64` explícito em tudo o que é dinheiro. Nunca `float64`, nunca `int` nu em contexto monetário.
- Datas civis como `string` no formato `YYYY-MM-DD` (ver ADR-005), não `time.Time` com *timezone*.
- `errors.Is`/`errors.As` com erros sentinela para diagnósticos: cada código de diagnóstico (`BALANCE_MISMATCH`, `DATE_AMBIGUOUS`, `CARD_PAYMENT_UNMATCHED`) é um valor de erro tipado, não uma string comparada.

---

## 2. Estrutura do repositório

Um **módulo Go único**. Não há monorepo, nem *workspaces*, nem pacotes publicáveis: o projeto tem um consumidor.

```
cmd/app/              # main: flag parsing, wiring, arranque
internal/
  http/               # rotas, middlewares, handlers, SSE
  views/              # templates templ: layouts, páginas, fragmentos
  modules/            # um diretório por módulo de domínio
    accounts/  transactions/  budgeting/  payees/
    rules/     imports/       categories/ reports/
    attachments/ auth/        jobs/       coverage/
  domain/             # puro, sem I/O: money, dates, budget, recurrence, matcher
  adapters/           # csv, ofx, camt053 + deteção e roteamento
  data/               # SQL, repositórios, migrações
  infra/              # sqlite, log, fila, ficheiros, imap, sse
profiles/             # biblioteca de perfis por instituição (embed.FS)
migrations/           # ficheiros .sql numerados, só para a frente
web/static/           # htmx.min.js, alpine.min.js, app.css gerado (embed.FS)
testdata/             # fixtures do corpus, por instituição e formato
```

**Regra de dependência:** `http`/`views` → `modules` → `domain`. `modules` → `data` → `infra`. **`domain` não importa nada de `data`, `infra` ou `http`** — é o que permite testar toda a contabilidade sem base de dados e sem servidor.

### Build

```
templ generate                          # .templ -> .go
tailwindcss -i web/src.css -o web/static/app.css --minify
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/app ./cmd/app
```

Sem Node, sem `package.json`, sem *bundler*. O artefacto é um ficheiro.

| Necessidade | Escolha | Notas |
| --- | --- | --- |
| Runner de tarefas | **`make`** (ou `task`) | Alvos simples: `generate`, `build`, `test`, `lint`, `dev` |
| Recarregamento em desenvolvimento | `air` ou `go run` + `templ generate --watch` | Binário Go, sem *daemon* de Node |
| Versão da linguagem | Diretiva `toolchain` em `go.mod` | Reprodutível sem `.nvmrc` |

---

## 3. Backend

| Necessidade | Escolha | Porquê esta, e não outra |
| --- | --- | --- |
| Servidor HTTP | **`net/http`** (padrão, com *routing* por método e padrão de Go 1.22+) | Suficiente: rotas com parâmetros, *middleware* por composição, SSE com `http.Flusher`. `chi` é a alternativa se os *middlewares* crescerem — mas o padrão elimina uma dependência |
| Templates | **`github.com/a-h/templ`** | Templates Go tipados, compilados. HTML inválido e campos em falta são erro de compilação, não *runtime* — o equivalente prático do que os tipos davam no frontend |
| Componentes de UI | Próprios, sobre Tailwind; ícones SVG inline (`lucide`) | Sem biblioteca de componentes a acompanhar; o padrão shadcn (`copiar o markup`) já era este |
| Validação | `github.com/go-playground/validator` para *structs* de formulário + validação explícita em valores monetários/datas | Dinheiro e datas têm regras próprias: `parseAmount` → `int64` com erro tipado, nunca *coercion* implícita |
| Driver SQLite | **`modernc.org/sqlite`** (Go puro, sem cgo) | **Zero risco de módulo nativo em ARM64** e *cross-compile* garantido. Alternativa mais ergonómica: `zombiezen.com/go/sqlite` (também sobre `modernc`). `mattn/go-sqlite3` está excluído: exige cgo |
| Migrações | `github.com/pressly/goose` ou ficheiros `.sql` numerados com *runner* próprio | Revisão de SQL em *pull request*; **apenas migrações para a frente** |
| Jobs em background | Goroutines + *pool* de tamanho fixo + tabela `jobs` em SQLite | Sem Redis. Estados `pending/running/done/failed`, *retry* com *backoff*. A concorrência trivial é a razão pela qual o Go simplifica aqui |
| Trabalho pesado | Goroutine dedicada por importação, com progresso por SSE | Substitui o `worker_threads` do plano em Node. Sem `event loop` a bloquear |
| Logs | `log/slog` (padrão) | JSON estruturado, `requestId` e `batchId` como atributos. Sem dependência |
| Autenticação | `golang.org/x/crypto/argon2` (Argon2id) + `github.com/alexedwards/scs` para sessões; `github.com/go-webauthn/webauthn` para *passkeys* | Argon2id é puro Go; sessões em *cookie* `HttpOnly`/`SameSite`; TOTP com `github.com/pquerna/otp` |
| Email / IMAP | `github.com/emersion/go-imap/v2` + `github.com/emersion/go-message` | Puro Go, ativo; traz os extratos enviados por email |
| Watch de ficheiros | `github.com/fsnotify/fsnotify` | Inbox de ficheiros. Verificar limites de *inotify* com muitas pastas |
| XML (OFX/CAMT.053) | `encoding/xml` (padrão) | OFX 2.x e CAMT.053 são XML. OFX 1.x é SGML — exige *tokenizer* próprio, pequeno |
| CSV | `encoding/csv` (padrão) | Ver §4 |
| Agendamento | `github.com/robfig/cron/v3` sobre a tabela `jobs` | Fuso `America/Sao_Paulo` explícito |
| Configuração | Variáveis de ambiente + YAML pequeno | Sem framework de configuração |
| CLI | `flag` (padrão) ou `github.com/spf13/cobra` | No mesmo binário, chamando os mesmos serviços |

---

## 4. Motor de importação (o diferencial)

Ordem de fontes: **OFX/CAMT.053 → CSV**. CSV é o núcleo (é o que está sempre disponível); OFX e CAMT são o degrau acima quando a instituição os oferecer, porque trazem identificadores exatos. Contexto e logística em [07-muitas-contas-e-cartoes.md](07-muitas-contas-e-cartoes.md).

| Necessidade | Escolha | Porquê |
| --- | --- | --- |
| *Parsing* CSV | **`encoding/csv`** com `FieldsPerRecord = -1` e `LazyQuotes = true`; máquina de estados própria onde for preciso preservar a linha crua | Tolerante a linhas irregulares e aspas soltas sem abortar. `InputOffset()` (Go 1.19+) permite recuperar os bytes originais para `raw_json` |
| Codificação | `golang.org/x/text/encoding/charmap` + `golang.org/x/text/encoding/unicode` + `github.com/saintfish/chardet` | UTF-8 estrito → CP1252 → ISO-8859-1, com aviso `ENCODING_FALLBACK`. `x/text` é a biblioteca canónica |
| BOM | `github.com/spkg/bom` (ou 6 linhas próprias) | Trivial |
| Deteção de delimitador e de cabeçalho | Heurística própria | Regras de negócio próprias (preâmbulos, totais). Não há biblioteca que as saiba |
| Datas | `time.Parse` com lista explícita de formatos + lógica de ambiguidade própria | `dd/MM` vs `MM/dd` resolve-se por dica de cabeçalho, contexto regional e **oráculo de saldo** — lógica de domínio, não de biblioteca |
| Decimais | Parser próprio `string → int64` cêntimos; `github.com/shopspring/decimal` só se necessário | O parser para cêntimos é pequeno, testável e elimina uma dependência. Nunca `float64` |
| Similaridade de *payee* | **`github.com/adrg/strutil`** (Jaro-Winkler, Levenshtein, Dice) e/ou `github.com/xrash/smetrics` (rápido, com Soundex) | Escolha de algoritmo e limiar explícitos e testáveis. Degrau 2 da escada do ADR-006: trigramas + TF-IDF em ~150 linhas próprias |
| Normalização de texto | `golang.org/x/text/unicode/norm` (NFD + remoção de marcas) + `golang.org/x/text/runes` | Remover acentos só para correspondência, preservando o original para exibição |
| Regex de ruído | `regexp` (RE2) | **Limitação conhecida: sem *lookaround* nem retroreferências.** Os padrões do domínio (PIX/TED/CNPJ/CPF/`PARCELA n/m`) são todos expressáveis em RE2 |
| Gramática PEG | **`github.com/mna/pigeon`** | Para as expressões de objetivos do orçamento (ADR-009). Nota: a gramática tem de ser reescrita em sintaxe `pigeon` |
| OFX | Parser próprio sobre `encoding/xml` (OFX 2.x) e *tokenizer* para OFX 1.x (é SGML, não XML válido) | `FITID` dá deduplicação exata e `ACCTID` dá roteamento exato — o melhor caso possível |
| CAMT.053 | `encoding/xml` | ISO 20022, XML válido; `AcctSvcrRef` e `IBAN` com o mesmo papel |
| Deteção de tipo de ficheiro | *magic bytes* próprios + `net/http.DetectContentType` | `<OFX`, `<Document`, ou texto delimitado. **Nunca pelo nome do ficheiro** |

**PDF não tem adapters.** Está fora de âmbito por [ADR-016](05-decisoes-adr.md#adr-016--pdf-fora-de-âmbito): nenhuma biblioteca de PDF, nenhum `poppler`, nenhum OCR, nenhuma dependência de sistema no *runtime*. Instituições sem exportação estruturada usam o modo «apenas o total» ([07 §8](07-muitas-contas-e-cartoes.md#8-modo-degradado-apenas-o-total)).

**Reconciliação declarada.** Não é biblioteca, é o mecanismo que torna a importação verificável: usar como invariantes todos os números que a fonte declara (`declared_open + Σ linhas == declared_close`, totais de compra, contagens). Em OFX, `<LEDGERBAL>`/`<AVAILBAL>` dão isto gratuitamente. Explicado em [07 §9](07-muitas-contas-e-cartoes.md#9-reconciliação-declarada-não-só-saldo).

---

## 5. Frontend

Sem `package.json`. Os dois ficheiros JS são vendidos no repositório e servidos pelo binário via `embed.FS`.

| Necessidade | Escolha | Porquê |
| --- | --- | --- |
| Templates | **`templ`** | Tipado e compilado; composição por funções Go |
| Interação | **HTMX** (~14 KB) | Fragmentos de página: filtros, paginação, edição por linha, modais que carregam dados. Elimina a camada de API do cliente |
| Estado local de UI | **Alpine.js** (~15 KB) | O que o HTMX não faz: `x-show`, separadores, *dropdowns*, modais, célula em edição. **Sem regras de negócio** |
| Estilo | **Tailwind CSS** + binário *standalone* | Utilitários, sem `node_modules`, sem passo de Node no *build* |
| Ícones | SVG inline (`lucide`, copiados) | Sem dependência |
| Gráficos | SVG gerado no servidor para os casos principais; biblioteca JS só onde a interatividade for indispensável | Substitui o Recharts. Aceita-se explicitamente menos polimento |
| Datas na UI | Formatação em Go (`time`), locale pt-BR | Fonte única: o servidor |
| i18n | Mapa de strings em Go, com `pt-BR` como base | Uma língua no MVP. Substitui o `i18next` |
| Atalhos e acessibilidade | HTML semântico, `<dialog>`, atributos ARIA diretos | O navegador dá mais do que se costuma usar |
| PWA | `manifest.json` + *service worker* mínimo, quando houver tempo | **Fora do MVP** — ver não-objetivos |

### Limites explícitos desta escolha

- **Nenhuma regra de negócio em JavaScript.** Se uma interação precisar de regra, é porque devia ser um fragmento do servidor.
- **A grelha de transações resolve-se por paginação e filtragem *server-side***, com edição por linha (`hx-patch`) e Alpine para o modo de edição. **Não** se virtualizam milhares de linhas no cliente.
- Ecossistema de componentes: próprio. Aceite em troca de não haver *build* de frontend.

---

## 6. Testes e qualidade

| Necessidade | Escolha |
| --- | --- |
| Testes unitários e de integração | **`testing`** (padrão) + `github.com/google/go-cmp` |
| Testes *golden* | `github.com/gotest.tools/v3/golden` ou comparação própria com `-update` |
| Testes de propriedades | **`pgregory.net/rapid`** — equivalente Go do `fast-check`, com *shrinking* |
| Testes de HTTP | `net/http/httptest` (padrão) |
| Testes de base de dados | SQLite em `t.TempDir()` — **sem Docker, sem contentores de teste** |
| E2E | `playwright-go` **opcional e depois**; os fluxos críticos são testáveis via `httptest` + *golden* de HTML |
| Lint / format | **`golangci-lint`** + `gofumpt`; `staticcheck` |
| Vulnerabilidades | `govulncheck` |
| Dependências mortas | `go mod tidy` + revisão de `go.mod` (a lista é curta e legível de propósito) |
| Cobertura mínima | `domain`, `adapters` e `imports`: alta obrigatória. Restante: sem meta rígida |

**O corpus *golden* continua a ser o ativo mais valioso do projeto** — cobre CSV, OFX e CAMT.053 por instituição, com *golden* linha a linha e a reconciliação declarada como teste obrigatório.

---

## 7. Operação

| Necessidade | Escolha | Notas ARM64 |
| --- | --- | --- |
| Contentor | **Build multi-stage → imagem `distroless/static` (ou `scratch`)** | ~20 MB de binário + ~10 MB de imagem. Sem sistema de operações, sem *shell*, sem *package manager* |
| Compilação | `CGO_ENABLED=0 go build` | `GOOS=linux GOARCH=arm64` a partir de qualquer máquina. **Sem `buildx` multi-arquitetura** |
| Ferramentas externas no *runtime* | **Nenhuma** | O binário é o único artefacto. Sem `poppler`, sem `tesseract`, sem OCR — consequência direta do ADR-016 |
| Reverse proxy + TLS | **Caddy** | Certificados automáticos, configuração de 5 linhas |
| Backup contínuo | **Litestream** | Binário Go com *release* `linux-arm64`; replica WAL para S3/B2 |
| Backup periódico | **restic** | *Snapshots* cifrados de anexos, uploads e exportações |
| Acesso privado | **Tailscale** | Elimina exposição de portas à internet |
| Monitorização | *Healthcheck* do Docker (`/healthz`) + Uptime Kuma ou alerta por email/Telegram | Sem Prometheus/Grafana no MVP |
| Atualizações | `docker compose pull && up -d` com *tag* explícita | Reversão = mudar a *tag* |
| Limites de memória | `GOMEMLIMIT=200MiB`, `mem_limit: 256m` | O *runtime* Go respeita o `GOMEMLIMIT` para o coletor. A memória é `page cache` de SQLite mais *heap* pequena |

### Dependências a evitar explicitamente

| Dependência | Motivo |
| --- | --- |
| `mattn/go-sqlite3` | Exige cgo: perde-se o *cross-compile* e o binário estático |
| Qualquer ORM (GORM, Ent, sqlboiler) | Sobreposição com SQL direto, *magic* de *reflection* e agregações difíceis de escrever. O SQL é explícito em `internal/data` |
| `lib/pq`/`pgx` | SQLite é a decisão (ADR-002) |
| Node.js, npm, `package.json`, *bundlers* | Justamente o que a decisão ADR-012 remove |
| `transformers.js` / ONNX em *runtime* | Reintroduziria um *runtime* JS. IA só *offline*, como ferramenta de *build* (ADR-006) |
| Bibliotecas de PDF (`pdfium`, `unipdf`, `pdfcpu`) | Fora de âmbito (ADR-016). Nenhuma entra, nem em Go nem em Python |
| `tesseract` / OCR | Fora de âmbito. Erro elevado em tabelas e ~100 MB de imagem |
| cgo em geral | Destrói o *cross-compile* e o binário estático. Se um dia for necessário, é uma decisão explícita e isolada |
| `k8s`/Helm | Não-objetivo |

---

## 8. Versões e política de atualização

- Versão de Go fixada na diretiva `toolchain` do `go.mod`.
- **Renovate ou Dependabot semanal**; *auto-merge* apenas de *patches*, e apenas com testes verdes.
- `govulncheck` em CI; `golangci-lint` obrigatório.
- `go.sum` revisto: a lista de dependências diretas deve caber numa página — se não couber, é sinal para reavaliar.
- Imagens Docker com *tag* de versão explícita (nunca `latest`); a base atualiza-se por *pull request* deliberado.
- Cliente HTTP dos adapters com `http.Client` próprio, `Timeout` explícito e *retry* com *backoff* apenas em `GET` idempotentes.
