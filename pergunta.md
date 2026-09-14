Vou registar esta análise como questão aberta na memória do projeto antes de responder.

Updated memory file 

Sim, Go é totalmente viável — e a tua pergunta expõe um ponto fraco da minha justificação anterior. Vou ser direto sobre isso.

## 1. Go vs TypeScript

O argumento mais forte que dei para TypeScript foi o **núcleo de importação isomórfico** (correr no browser para *preview* instantâneo). Esse argumento é frágil: o servidor ARM64 faz o *preview* de 3 000 linhas em menos de 2 s. Foi otimização prematura vendida como decisão arquitetural.

O outro argumento possível — «reaproveitar o código TS do Actual» — está morto pelo ADR-011 (não fazemos *fork*). Logo, **a linguagem está livre**. Nota importante: o facto de estares a reimplementar é *neutro*, não favorece Go. O que decide é a tua fluência.

| Critério | TypeScript | Go |
| --- | --- | --- |
| Partilha de tipos com o frontend | Nativa | Exige OpenAPI + `openapi-typescript` (setup único, fiável) |
| RAM *idle* / pico | 100–250 MB / 400–500 MB | 20–50 MB / 100–150 MB |
| Módulos nativos em ARM64 | Risco (`better-sqlite3`, `argon2`) | Zero com `modernc.org/sqlite` (puro Go) |
| Deploy | Node + `node_modules` (~250 MB de imagem) | Binário único (~20 MB) + distroless (~30 MB) |
| Jobs/importações concorrentes | *worker threads*, gestão manual | Goroutines, trivial |
| Arranque a frio | 200–800 ms | 5–20 ms |
| Bibliotecas de CSV/fuzzy/validação | Mais ricas (`csv-parse`, `fuzzball`, Zod) | Equivalentes funcionais (ver abaixo) |
| Custo humano | Uma linguagem, um *debugger* | Duas linguagens + fronteira de *codegen* |

Em Go não perdes nada de essencial: `encoding/csv` (com `FieldsPerRecord = -1` para tolerar linhas irregulares), `github.com/adrg/strutil` para Jaro-Winkler/Levenshtein, `github.com/mna/pigeon` em vez de Peggy, `golang.org/x/crypto/argon2`, `emersion/go-imap`, `fsnotify`, `shopspring/decimal`. E `embed.FS` serve a SPA a partir do próprio binário.

**O que muda nos ADRs:** ADR-003 (linguagem) reescrito; ADR-001 simplifica-se ainda mais (deixa de haver núcleo isomórfico — o *preview* passa a ser 100% server-side, eliminando o *web worker* e a restrição de «pureza» dos pacotes); ADR-002 mantém-se. Regras, perfis e heurísticas de data/decimal são idênticos em qualquer linguagem — é lógica, não biblioteca.

**Veredicto:** Go para o backend é uma escolha **igual ou melhor** que TS aqui, *se* escreveres Go com naturalidade. Se escreves TS mais depressa, uma linguagem só bate tudo: o teu tempo é o recurso escasso, não os 100 MB de RAM.

## 2. SolidJS

Tecnicamente é excelente para este caso (reatividade fina, *bundles* pequenos, grelhas de milhares de linhas beneficiariam). O custo é o ecossistema:

| Necessidade | Solid | Comentário |
| --- | --- | --- |
| Primitivas acessíveis | **Kobalte** | Porte de Radix para Solid; maduro |
| Componentes shadcn | **solid-ui** | Porte de shadcn; menos mantido que o original |
| Dados/tabelas/rotas | TanStack Query/Table/Router | Têm adapters Solid |
| Gráficos | ECharts, Chart.js ou SVG próprio | **Recharts é React-only** — terias de escrever gráficos |
| Respostas prontas na internet | Muito menos | Cada obstáculo custa mais tempo a ti |

Como esta app é formulários, tabelas e gráficos — **não é limitada por desempenho de rendering** — a escolha deve ser feita por *alavancagem de ecossistema*, e aí React ganha com folga. Solid só se valorizares muito o modelo de *signals*. É defensável, mas não é uma simplificação: é uma troca de conforto por mais trabalho.

## 3. RAM estimada por contentor

Estimativas para medir depois com `docker stats`:

| Componente | Node/TS | Go |
| --- | --- | --- |
| App *idle* | 70–120 MB | 15–30 MB |
| App sob carga normal | 120–200 MB | 30–60 MB |
| Pico em importação de 50k linhas | 250–450 MB | 80–150 MB |
| Caddy | 15–25 MB | 15–25 MB |
| Litestream | 10–20 MB | 10–20 MB |
| **Total *idle*** | **~100–170 MB** | **~40–75 MB** |
| **Total no pico** | **~350–500 MB** | **~120–200 MB** |

Notas que valem mais do que os números:

- **O RSS do Node não baixa** depois de um pico: o V8 mantém a *heap* alocada. Um contentor Node parece «vazar» sem vazar. **Solução concreta:** correr a importação numa *worker thread* descartável e terminá-la no fim do *job* — o SO recupera os 200–300 MB de uma vez.
- Ajustes recomendados: Node com `--max-old-space-size=384` e `mem_limit: 768m`; Go com `GOMEMLIMIT=200MiB` e `mem_limit: 256m`.
- SQLite: `PRAGMA cache_size = -40000` (40 MB) e `wal_autocheckpoint = 1000`. Essa memória é *page cache* recuperável, não conta como pressão real.
- Conclusão prática: **em qualquer host com 4 GB, a RAM é irrelevante para ambas as stacks.** Só passa a ser argumento decisivo em hardware com 1 GB (Pi Zero 2 W), onde Go permite correr tudo em ~150 MB e Node não.

## 4. O que simplifica mesmo o desenvolvimento a solo

Ordenado por impacto real:

1. **Uma linguagem, um repositório, um processo.** Esta é a maior de todas. Se o backend for Go e o frontend TS, paga-se com *codegen* automático — nunca com tipos escritos à mão nos dois lados.
2. **Go + `templ` + HTMX/Datastar em vez de SPA (opção radical).** Elimina: camada de API, cliente tipado, router, *bundler*, cache de estado do cliente, hidratação. O servidor devolve HTML; as interações usam pedaços de página. Corta talvez **metade do código total**. Custa-te a grelha de transações com edição inline fluida e os gráficos interativos. Para um MVP solo, é a simplificação com melhor relação esforço/resultado.
3. **Regras e perfis de banco como dados** (JSON/YAML no repositório, não código). Adicionar suporte a um banco novo = editar um ficheiro. Sem *deploy*, sem *release*, sem testes novos.
4. **Agregados materializados em vez de motor de fórmula** (ADR-009). Já está no plano; é a decisão que mais complexidade remove.
5. **Comprar em vez de construir**: autenticação por biblioteca, UI kit pronto, virtualização pronta, gráficos prontos. Não escrevas FTS, não escrevas motor de regras com UI própria na v1.
6. **Cortar âmbito a sério.** Sem multi-moeda, multi-utilizador, anexos, etiquetas, relatórios configuráveis, OFX, PWA, i18n e temas no MVP. São ~20 ecrãs, não os ~120 do Actual.
7. **Corpus de testes *golden* desde o dia 1.** Sem QA, os teus testes são o teu QA. Permite refatorizar o normalizador sem medo — é o que impede o projeto de apodrecer.
8. **Uma base de dados, um ficheiro, um comando de deploy.** Build no próprio host ou *cross-compile*; sem matriz de CI, sem `buildx` multi-arch, sem Kubernetes.
9. **Só migrações para a frente.** Nunca escrevas migrações reversíveis.
10. **Evitar dependências nativas.** Já está no plano, e em Go com `modernc.org/sqlite` torna-se garantido.
11. **Escolher tecnologias aborrecidas e populares.** Vais ausentar-te três semanas e voltar sem memória do código; respostas prontas na internet são trabalho gratuito que fazes a ti próprio. É o argumento mais forte contra Solid e contra qualquer escolha exótica.
12. **ADRs escritos** (já feitos) para não re-litigares cada decisão daqui a seis meses.

## Recomendação

| Pacote | Composição | Escolhe se |
| --- | --- | --- |
| **A — Uma linguagem** | TS em tudo: Fastify + Drizzle + React | Escreves TS mais rápido; queres manter o *preview* no browser |
| **B — Backend forte** | Go (`modernc.org/sqlite`, `pigeon`, `strutil`) + React/TS com OpenAPI gerado | Escreves Go com naturalidade; valorizas binário único, RAM baixa e zero dor de módulos nativos em ARM64 |
| **C — Mínimo de código** | Go + `templ` + HTMX + Tailwind, sem SPA | Queres o menor número de linhas e de peças possível, e aceitas recarregamentos parciais nos ecrãs |

Para uma pessoa só, **B** é o melhor equilíbrio se souberes Go; **C** é o que chega mais depressa a um produto utilizável; **A** é o mais seguro se o teu tempo de aprendizagem for caro.

Uma pergunta para fechar isto: escreves Go ou TypeScript com mais fluência — e preferias uma SPA ou HTML renderizado no servidor? Com a resposta reescrevo o `02-stack.md` e adiciono um ADR-012 a revisitar o ADR-003, em vez de deixar dois caminhos em aberto na documentação.