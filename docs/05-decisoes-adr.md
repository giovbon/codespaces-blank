# 05 — Decisões de Arquitetura (ADRs)

Formato: **Contexto → Decisão → Alternativas avaliadas → Consequências**.

**Registo de revisões.** ADR-001, ADR-003, ADR-004, ADR-005, ADR-006, ADR-008, ADR-010 e ADR-013 foram revistos em **2026-09-14**, no âmbito da passagem a Go com UI renderizada no servidor (ADR-012), da ingestão multi-fonte (ADR-013 a ADR-015) e da **exclusão de PDF do âmbito (ADR-016)**. Cada revisão está assinalada no próprio ADR. Os ADR-002, ADR-007, ADR-009, ADR-011, ADR-014, ADR-015, ADR-016 e ADR-017 permanecem válidos.

> **Nota de manutenção (ADR-017).** Este registo de revisões, o índice acima e o `README.md` têm de ser atualizados **no mesmo *pull request*** que cria ou revê um ADR. Instruções de agente que contradigam um ADR vigente são um modo de falha conhecido neste projeto.

| # | Título | Estado |
| --- | --- | --- |
| 001 | Monólito modular *server-centric* com UI renderizada no servidor | Revisto |
| 002 | SQLite em modo WAL como base de dados única | Válido |
| 003 | Go no servidor, com tipos como fonte única de verdade | Revisto (substitui TS ponta a ponta) |
| 004 | Interações por fragmentos HTML; REST/JSON + SSE na fronteira externa | Revisto |
| 005 | Dinheiro em inteiros e datas civis | Revisto (detalhe de linguagem) |
| 006 | Regras como dados (DSL JSON) e aprendizagem, em vez de ML | Revisto (escada de evolução) |
| 007 | *Journal* de auditoria com *soft delete* | Válido |
| 008 | Importação com *staging* persistido e pré-visualização obrigatória | Estendido (multi-formato) |
| 009 | Sem motor de planilha genérico | Válido |
| 010 | Deploy em Docker multi-arch com Litestream; sem Kubernetes | Revisto (binário Go *distroless*) |
| 011 | Não fazer *fork* do Actual Budget | Válido |
| 012 | Go com UI renderizada no servidor (`templ` + HTMX + Alpine.js) | Novo |
| 013 | Ingestão por adapters (CSV, OFX, CAMT.053) com roteamento por conteúdo | Revisto (sem PDF) |
| 014 | Cartão de crédito como conta; pagamento de fatura como transferência emparelhada | Novo |
| 015 | Biblioteca de perfis embutida no repositório | Novo |
| 016 | PDF fora de âmbito | Novo |
| 017 | Otimização de contexto para agentes de IA | Novo |

---

## ADR-001 — Monólito modular *server-centric* com UI renderizada no servidor

> **Revisto em 2026-09-14** (ver ADR-012). A versão original previa uma SPA com núcleo de domínio isomórfico em *web worker*. Essa parte foi removida.

**Contexto.** O Actual Budget é *local-first*: toda a lógica corre no browser sobre SQLite em WebAssembly, e a sincronização entre dispositivos é feita por mensagens de CRDT com *merkle trie* e resolução automática de conflitos. Isso é necessário porque o Actual suporta app desktop, uso offline e múltiplos dispositivos concorrentes. Aqui, existe sempre um servidor ligado e o acesso é apenas por navegador.

**Decisão.** O **servidor ARM64 é a fonte de verdade e o único local onde existe lógica**. O servidor renderiza HTML (`templ`), serve fragmentos de página em resposta a interações (HTMX) e emite SSE para eventos. O JavaScript no cliente limita-se a comportamento local de interface (Alpine.js) — não contém regras de negócio nem motor de importação.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| *Local-first* com CRDT como o Actual | Meses de trabalho em sync, mensagens, reparação e *merkle*; benefício nulo com servidor sempre ligado; introduz modos de falha difíceis de depurar |
| *Local-first* simplificado (IndexedDB + sync bruto por *timestamp*) | Sincronização por *timestamp* corrompe-se com relógios dessincronizados e escritas concorrentes; exigiria lógica de resolução de conflitos de qualquer forma |
| SPA com núcleo isomórfico em *web worker* (plano original) | Justificado por «pré-visualização instantânea de ficheiros grandes»; medição real: o servidor faz a pré-visualização de 3 000 linhas em menos de 2 s. Era otimização prematura a pagar com uma fronteira de tipos, um *bundler* e disciplina de «pureza» permanente |
| Cliente fino puro com SPA (React/Vue/Solid) | Duplica a superfície (API tipada, router, *cache* de estado no cliente, *bundler*) sem requisito de produto que a exija |
| Micro-serviços | Carga de um utilizador; custo operacional desproporcionado em ARM64 |

**Consequências.**

- (+) Simplicidade radical: uma base de dados, um processo, uma linguagem, uma migração.
- (+) **Uma só implementação da lógica**: não existe «a mesma regra no browser e no servidor», logo não existe divergência entre os dois.
- (+) Elimina o cliente de API tipado, o *router* do cliente, o *cache* de estado no cliente, a hidratação e o *bundler*.
- (+) Um `docker compose` substitui toda a orquestração do Actual.
- (−) Sem suporte offline real: sem rede, não há alterações. Aceitável para o caso de uso.
- (−) Interações muito dinâmicas custam uma ida ao servidor. Mitigado por fragmentos pequenos, SSE para progresso e paginação *server-side* nas listas grandes.
- (−) Abandona-se a «preview instantânea» como objetivo. Substituída por: progresso por SSE e pré-visualização com verificação de integridade (ADR-008, ADR-013).

---

## ADR-002 — SQLite em modo WAL como base de dados única

**Contexto.** Um utilizador, um servidor ARM64, dezenas de milhares de transações a crescer para centenas de milhares ao longo de anos. Custo operacional importa mais do que escala teórica.

**Decisão.** SQLite em WAL, base de dados única em volume persistente, acessível só pelo processo da aplicação (Go).

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| PostgreSQL | Serviço extra a administrar, atualizar, monitorizar e copiar; nenhum benefício de concorrência com um escritor único |
| MariaDB/MySQL | Mesma objeção, com menos vantagens adicionais |
| DuckDB | Excelente para analítica, inadequado como base de dados transacional (escritas concorrentes, transações) |
| Ficheiro JSON/NDJSON | Sem transações, sem índices, sem integridade; corrupção garantida com o tempo |

**Consequências.**

- (+) Um único ficheiro para copiar; backups triviais e verificáveis (`VACUUM INTO`).
- (+) Desempenho de leitura superior a qualquer alternativa em rede; sem latência de socket.
- (+) Zero administração: sem utilizadores, portas, autenticação, *tuning*.
- (+) `FTS5` disponível nativamente para busca com remoção de acentos.
- (−) Escritor único: mitigado por transações curtas, `BEGIN IMMEDIATE`, WAL e *jobs* serializados.
- (−) Sem replicação nativa: resolvido por Litestream (ver ADR-010).
- *Caminho de saída*: todo o SQL isolado em `internal/data/`, migração para PostgreSQL como exercício localizado.

---

## ADR-003 — Go no servidor, com tipos como fonte única de verdade

> **Revisto e substituído em 2026-09-14** (ver ADR-012). Decidia TypeScript ponta a ponta com Zod partilhado. A premissa que o sustentava — a partilha de tipos com o browser — desapareceu com o ADR-001 revisto.

**Contexto.** Um único developer a manter backend, motor de importação, UI e operação. Cada fronteira de tipos que exija `codegen` ou sincronização manual custa tempo de manutenção para sempre. O motor de importação lida com formatos ambíguos: os tipos são a primeira linha de defesa contra erro silencioso.

**Decisão.** **Go** em todo o servidor (HTTP, domínio, motor de importação, *jobs*, CLI). Os tipos do domínio são *structs* Go com campos tipados e construtores que validam — não há *schema* separado a manter em paralelo, porque **não existe um segundo consumidor desses tipos**. Documentação de API é gerada por reflexão para uso externo (`curl`, automação), não para o cliente interno.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| TypeScript ponta a ponta (decisão anterior) | O argumento decisivo era o núcleo isomórfico, que deixou de existir (ADR-001). Sem ele, resta: RAM 3–5× superior, risco de módulos nativos em ARM64, `node_modules` na imagem, sem binário único |
| Go + SPA em TypeScript | Reintroduz a fronteira de tipos que ADR-001 eliminou, e agora sem contrapartida |
| Go + Python (pipeline em Python) | Um segundo *runtime*, segundo conjunto de dependências e o corpus *golden* dividido por duas linguagens. Ver ADR-013 para a única exceção admitida |
| Rust | Desempenho excelente, irrelevante aqui; custo de desenvolvimento proibitivo para um developer a solo |
| Gerar tipos a partir de OpenAPI | Etapa de *build* adicional e *drift* garantido — problema que a UI *server-rendered* simplesmente não tem |

**Consequências.**

- (+) **O problema da fronteira de tipos deixa de existir**, em vez de ser resolvido com *codegen*.
- (+) Binário único, ~20 MB, arranque em milissegundos, RAM em repouso na casa das dezenas de MB.
- (+) `modernc.org/sqlite` é Go puro: **zero risco de módulo nativo em ARM64**, e *cross-compile* sem *toolchain* de C.
- (+) Concorrência trivial para *jobs* e importações (goroutines), sem gestão manual de *worker threads*.
- (+) Renomear um campo continua a ser um erro de compilação em todos os pontos de uso.
- (−) Não há validação partilhada com o browser: a validação de formulários é *server-side* e o HTML devolve os erros no próprio fragmento. Aceitável — era exatamente o que a UI *server-rendered* já fazia.
- (−) Sem OpenAPI automático a partir de `structs`: o contrato de API externa é escrito à mão e testado. Superfície pequena e deliberadamente estável.
- (−) Duas linguagens no repositório (Go + alguns ficheiros JS vendidos sem *build*). Mitigado por não haver passo de *build* de frontend.
- (−) Armadilha conhecida: `int` em Go tem largura dependente da plataforma em contexto de *overflow*. **Usar `int64` explicitamente em todo o domínio monetário** e nunca `float64` para dinheiro.

---

## ADR-004 — Interações por fragmentos HTML; REST/JSON + SSE na fronteira externa

> **Revisto em 2026-09-14.** A versão original previa que a SPA consumisse REST/JSON em todas as interações. Com a UI renderizada no servidor (ADR-001 revisto), isso deixa de ser verdade para o tráfego interno.

**Contexto.** Cliente único, mas com necessidade real de automação: *scripts*, *cron*, `curl`, ingestão de ficheiros, integrações futuras.

**Decisão.** Duas fronteiras distintas, com regras claras:

| Uso | Mecanismo | Exemplo |
| --- | --- | --- |
| Interação da UI | **Fragmentos HTML** devolvidos pelo servidor | `hx-get`, `hx-post`, `hx-patch` sobre rotas que devolvem pedaços de página |
| Automação e integrações | **REST/JSON** versionado em `/api/v1` | `curl` para disparar importação, `cron`, clientes externos |
| Eventos do servidor | **SSE** em `/api/v1/events` | Progresso de *jobs*, invalidação, avisos |

Nunca se escreve a mesma funcionalidade duas vezes: as rotas HTML e as rotas JSON partilham o mesmo serviço de domínio, e apenas a rota JSON constitui contrato público.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| tRPC | Pressupõe cliente TypeScript, que não existe. Fecharia a porta à automação por `curl`/`cron` — requisito explícito para a importação automática |
| GraphQL | Complexidade de esquema e *resolvers* sem consumidores heterogéneos que a justifiquem |
| WebSocket | Bidirecional que não é necessário; mais código de reconexão, *heartbeat* e estado. SSE resolve o caso real (*servidor → cliente*) |
| JSON para todas as interações da UI | Anula a razão de ser da UI *server-rendered*: obrigaria a templates no cliente e a serialização/desserialização sem benefício |

**Consequências.**

- (+) A importação automática pode ser disparada de qualquer sítio (`curl`, `cron`, *script*, Tailscale).
- (+) SSE dá reconexão nativa, funciona através de *proxies* e serve progresso de *jobs* e invalidação.
- (+) O contrato público é uma superfície pequena e estável — mantível à mão e testável (ver ADR-003).
- (−) Duas superfícies de rota a manter. Limitado por disciplina: **regra de negócio vive no serviço, nunca na rota**.
- (−) Sem OpenAPI gerado: os contratos externos são escritos e cobertos por testes.

---

## ADR-005 — Dinheiro em inteiros e datas civis

**Contexto.** É o sistema mais suscetível a erro subtil de todo o projeto. `0.1 + 0.2 !== 0.3` em ponto flutuante; e uma data de transação representada como instante UTC pode aparecer no dia anterior.

**Decisão.**
- Quantias: `INTEGER` em cêntimos, `int64` em todo o código. Nenhum valor monetário existe como tipo fracionário (`float64`) em nenhum ponto do sistema.
- Datas de transação: `TEXT 'YYYY-MM-DD'`, tratadas como datas civis, sem fuso.
- Instantes de sistema: `INTEGER` em milissegundos *epoch*.
- A aritmética decimal existe **apenas** na fronteira de *parsing* (`shopspring/decimal`, ou parser próprio que devolve cêntimos diretamente — preferível, por ter menos superfície). Fora dela, só `int64`.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| `number` com cêntimos implícitos | Perde-se a intenção; alguém multiplica por 1.1 e o dinheiro deixa de ser exato |
| `DECIMAL`/`NUMERIC` do SQLite | O SQLite não tem tipo decimal real; `NUMERIC` cai em `REAL` |
| Guardar em string e somar em código | Impede `SUM()` em SQL e agregações eficientes |
| `Date`/`TIMESTAMP` UTC para datas de transação | O erro mais comum em apps de finanças: lançamentos a mudar de dia |
| `BigInt` para cêntimos | Desnecessário; `INTEGER` de 64 bits cobre qualquer património realista com folga |

**Consequências.**

- (+) Somatórios exatos em SQL; nada de arredondamento acumulado.
- (+) Comparações de saldo fiáveis — a base de toda a reconciliação.
- (−) Toda a conversão passa por utilitários em `internal/domain/money` (`ParseAmount`, `FormatAmount`, `SumCents`), testados com `rapid` (propriedades).
- (−) Um `±1` cêntimo em conversões de moeda é inevitável; o arredondamento tem de estar localizado e documentado numa única função.

---

## ADR-006 — Regras como dados (DSL JSON) e aprendizagem, em vez de ML

**Contexto.** Categorizar automaticamente é o que torna a importação útil. As regras precisam de ser editáveis pelo utilizador, portáveis e auditáveis. O host é ARM64 modesto.

**Decisão.** Motor de regras com condições e ações em JSON, avaliadas por um interpretador com índices, executadas em três *stages* (`pre`, `default`, `post`). Complementado por aprendizagem estatística simples em `payee_mappings` (contagem de acertos e confiança). Sem *machine learning* no MVP.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Regras em código | O utilizador não pode editar; cada mudança exige *deploy* |
| Modelo de ML embutido | Exige *dataset* rotulado e avaliação; corre risco de classificar errado em silêncio e sem explicação |
| LLM a categorizar cada linha | Custo por chamada, latência, dependência de rede, privacidade de dados financeiros, resultados não determinísticos |
| `transformers.js` (ONNX no browser) | Reintroduz um *runtime* JavaScript que a UI *server-rendered* eliminou (ADR-001/ADR-012); ~10 MB de WASM + pesos; põe o custo em cada cliente; e o ADR-006 continua a valer |
| Apenas regras por *payee* histórico | Pouco expressivo; impossível capturar padrões como «valor entre X e Y e conta Z» |

**Consequências.**

- (+) Totalmente auditável: a pré-visualização mostra exatamente qual regra disparou.
- (+) Regras exportáveis e partilháveis pela comunidade.
- (+) A aprendizagem melhora resultados sem infraestrutura, e uma correção humana vale mais do que um *retrain*.
- (+) Zero dependências de *runtime* para IA: o binário não sabe que IA existe.
- (−) Requer um índice eficiente para não avaliar todas as regras em todas as linhas.
- (−) Fronteira conhecida: casos não capturáveis por padrões simples. Aceitável — a categorização manual continua a ser possível e rápida.

### Escada de evolução (só se e quando a aprendizagem estagnar)

Ordem obrigatória, do mais barato para o mais caro. **Cada degrau só se sobe quando o anterior estiver esgotado.**

| Degrau | Técnica | Dependências | Onde corre |
| --- | --- | --- | --- |
| 1 | Jaro-Winkler + limpeza de ruído (o que já existe) | nenhuma | — |
| 2 | **Similaridade por trigramas de caracteres + TF-IDF** sobre *payees* normalizados, agrupados por limiar. Resolve identidade de comerciante (`UBER *TRIP 1234` ≡ `UBER DO BRASIL TECNOLOGIA`) | nenhuma (puro Go, ~150 linhas) | servidor |
| 3 | **Embeddings gerados *offline*, kNN em Go** sobre vetores guardados em `BLOB`. Sugestão apenas | 1 *script* de dev + 1 *job* batch | servidor |
| 4 | Classificador leve supervisionado, **como sugestão, nunca decisão** | *dataset* rotulado + avaliação | servidor |

O degrau 3 é a forma **correta** de usar um modelo: o modelo é **ferramenta de desenvolvimento, não dependência de *runtime***. Gera-se os vetores uma vez, na máquina do developer, e *commita-se* o resultado. Um nível 0 adicional e subestimado: usar um modelo local *offline* para **expandir a tabela de sinónimos** de bancos e variantes de comerciantes, *commitando* o JSON gerado no repositório.

*Registo explícito*: `transformers.js` foi avaliado e rejeitado nesta fase. Reabrir esta decisão exige demonstrar, com medição, que o degrau 2 falhou.

---

## ADR-007 — *Journal* de auditoria com *soft delete*, em vez de event sourcing ou CRDT

**Contexto.** Importar ficheiros errados, com o mapeamento errado, para a conta errada é um risco real e frequente. O utilizador precisa de desfazer com confiança. A auditoria também é valiosa para perceber «porque é que esta transação está assim».

**Decisão.** Estado atual nas tabelas de negócio, mais um `audit_log` append-only que regista `before_json`/`after_json` de cada mutação, com `actor` e `origin`. Remoções são `tombstone = 1`. O *undo* de uma importação reaplica o inverso do *journal* em ordem inversa.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Event sourcing puro | Elegante, mas obriga a reconstruir o estado em cada leitura e complica todas as consultas; custo desproporcionado |
| CRDT como o Actual | Resolve conflitos entre dispositivos que aqui não existem |
| Sem auditoria, apenas *backup* | Restaurar a base de dados inteira não é «desfazer aquela importação»; não serve a necessidade real |
| *Triggers* SQL a escrever auditoria | Espalha a lógica, dificulta incluir `actor` e `origin`, difícil de testar |

**Consequências.**

- (+) `undo` de lote é uma funcionalidade de primeira classe e barata de implementar.
- (+) Consultas continuam simples e rápidas (estado materializado).
- (+) Histórico permite à UI mostrar «o que mudou» numa transação.
- (−) Cada escritor tem de lembrar-se de escrever no *journal*: mitigado por um mutator único obrigatório, com testes que verificam que escritas diretas não existem.
- (−) O `audit_log` cresce indefinidamente: política de retenção (por exemplo, 24 meses) para ações de baixo valor, mantendo importações e remoções para sempre.

---

## ADR-008 — Importação com *staging* persistido e pré-visualização obrigatória

**Contexto.** Um único erro de mapeamento pode inserir centenas de transações erradas. Detetá-lo depois é caro; evitá-lo antes é barato.

**Decisão.** O pipeline grava todo o resultado intermédio em `import_rows` dentro do mesmo `import_batch`. Nada é escrito em `transactions` antes de um *commit* explícito, exceto quando o perfil tem `auto_commit` e todos os diagnósticos são `ok`.
> **Estendido em 2026-09-14** (ADR-013/ADR-014). O *staging* é **agnóstico ao formato**: CSV, OFX e CAMT.053 alimentam as mesmas tabelas. Duas extensões: (a) a condição de `auto_commit` passa a exigir também a **reconciliação contra os totais declarados** da fonte, e não apenas ausência de erros; (b) um lote pode ter **contas-alvo distintas por linha** (`import_rows.target_account_id`), mantendo um único *commit* e um único *undo*.
**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Importação direta com *undo* apenas | O utilizador vê o resultado depois de o ter; a correção de mapeamento exige reverter e refazer |
| Pré-visualização calculada em memória, sem persistir | Perde-se a explicação, a aprovação por linha e a reprodutibilidade; um *refresh* de página perde tudo |
| Guardar apenas as linhas rejeitadas | Perde-se a possibilidade de explicar decisões e de reauditar lotes antigos |

**Consequências.**

- (+) Todas as decisões do motor são interrogáveis linha a linha, com o `raw_json` original ao lado.
- (+) A aprovação por linha permite excluir casos duvidosos sem abandonar o resto do ficheiro.
- (+) O lote torna-se um artefacto auditável: o que veio no ficheiro, o que decidimos, o que ficou gravado.
- (+) Reprocessar com um perfil diferente é possível sem novo *upload*.
- (−) `import_rows` duplica temporariamente o volume de dados (raw + normalizado): resolvido por *job* de limpeza que colapsa `raw_json` em lotes com mais de N meses, mantendo o resumo.
- (−) Disco: ficheiros originais guardados indefinidamente. São pequenos (dezenas de KB por mês) e são a prova documental.

---

## ADR-009 — Sem motor de planilha genérico

**Contexto.** O Actual inclui um motor de *spreadsheet* com fórmulas, gramáticas Peggy para objetivos e uma camada de *sheet* com grafo de dependências. É uma fonte enorme de complexidade e de *bugs* subtis.

**Decisão.** Não replicar. Substituir por:
1. **Expressões restritas** (gramática PEG com Peggy) para objetivos de categoria: `#template 200`, `#template 15% of income`, `#template schedule Netflix`.
2. **Agregações materializadas** em `budget_month_cache`, recalculadas por *job* após escritas.
3. **Campos derivados simples** calculados no servidor (disponível, transportado, média de 3 meses).

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Portar o motor do Actual | Muito código interdependente, difícil de manter sem a equipa original |
| HyperFormula ou similar | Poderoso e pesado; permite ao utilizador criar dependências circulares e folhas de cálculo inescrutáveis dentro de um orçamento |
| Sem objetivos, apenas valores fixos | Perde-se a funcionalidade mais valorizada do orçamento por envelope |

**Consequências.**

- (+) Orçamento previsível, rápido e compreensível.
- (+) Sem problemas de dependências circulares nem de recálculo em cascata.
- (−) Menos flexível para casos exóticos. Aceitável: o objetivo é gerir dinheiro, não construir folhas de cálculo.
- (−) O `budget_month_cache` tem de ser invalidado corretamente: cada escrita que afete um mês enfileira o *job* `budget.recompute` para esse mês.

---

## ADR-010 — Deploy em Docker multi-arch com Litestream; sem Kubernetes

**Contexto.** Host ARM64 doméstico, sempre ligado, com armazenamento possivelmente em SSD USB ou cartão SD (desgaste por escrita). Operação tem de ser trivial e local.

**Decisão.** Um `docker compose` com três componentes: aplicação (**binário Go estático** em imagem `linux/arm64` *distroless*, ~30 MB), Caddy como *reverse proxy* com TLS, e Litestream a replicar a base de dados para armazenamento de objetos. Acesso restrito por Tailscale. Restic para anexos e exportações. O binário é compilado para `linux/arm64` com `CGO_ENABLED=0` (`modernc.org/sqlite` é Go puro), pelo que **não existe *toolchain* de compilação no host nem recompilação em runtime**.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Kubernetes / k3s | Complexidade operacional absurda para um serviço |
| Proxmox LXC | Válido, mas Docker Compose é mais simples de atualizar e versionar |
| Binário + systemd, sem container | Com Go e `CGO_ENABLED=0` a objeção antiga («dependências tornam-se manuais») praticamente desaparece: o artefacto é um ficheiro. Continua a ser pior para **reversão e reprodutibilidade** (três binários: app, Caddy, Litestream), que é o que o Compose resolve de forma trivial |
| Base Alpine | Deixou de ser relevante: sem `node_modules`, glibc vs musl não é problema. `distroless`/`scratch` é ainda menor e mais seguro |
| Postgres em contentor com backup por `pg_dump` | Ver ADR-002; mais peças, mais modos de falha |

**Consequências.**

- (+) Atualização é `docker compose pull && docker compose up -d`; reversão é mudar a *tag*.
- (+) Litestream dá *point-in-time recovery* contínuo sem parar a aplicação.
- (+) Desgaste de armazenamento controlado: WAL com *checkpoint* adequado, `synchronous = NORMAL`, sem escritas de *log* desnecessárias, dados em SSD USB em vez de cartão SD.
- (−) Litestream é mais um processo a vigiar: *healthcheck* do contentor e alerta se a replicação atrasar.
- (−) Um único ponto de falha físico. Mitigado por: réplica contínua fora do host, `VACUUM INTO` noturno, *snapshot* restic, e **ensaio de restauro mensal documentado** — um backup nunca testado não é um backup.

---

## ADR-011 — Não fazer *fork* do Actual Budget

**Contexto.** O Actual Bundle é MIT: pode ser usado, modificado e redistribuído, desde que o aviso de copyright e a licença sejam preservados. «Copiar todas as funcionalidades essenciais» é uma decisão de produto, não de código.

**Decisão.** Reimplementar com o repositório do Actual como **referência funcional e de comportamento**. Reutilizar ideias, fluxos e formatos de dados; não copiar ficheiros de código.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| *Fork* direto do Actual e acrescentar o motor CSV | Herdaria a arquitetura *local-first* com CRDT, três *builds* de plataforma, Electron e um motor de planilha completo — precisamente o que foi decidido evitar. Além disso, manter um *fork* divergente é insustentável a solo |
| Usar o Actual como dependência e escrever *plugins* | A arquitetura de *plugins* não cobre nem a camada de dados nem a importação ao nível de detalhe pretendido |
| Contribuir para montante do Actual | Válido e recomendável para correções pontuais, mas não entrega o modelo *server-centric* nem o motor de importação desejado |

**Consequências.**

- (+) Arquitetura adequada ao contexto real (servidor sempre ligado, ARM64, só browser).
- (+) Liberdade total sobre o modelo de importação, que é o diferencial.
- (−) Reimplementar o que já existe: contas, transações, orçamento, relatórios. Trabalho considerável — daí o roadmap faseado em [01-arquitetura.md](01-arquitetura.md#9-roadmap-por-fases), com valor entregue a cada fase.
- (−) Risco de divergência de comportamento em relação ao Actual: mitigado por testes de conceito comparativos.
- *Obrigação legal*: se algum fragmento de código do Actual for efetivamente reutilizado, o aviso de copyright MIT e a atribuição têm de constar no repositório e nos *binaries* distribuídos.

---

## ADR-012 — Go com UI renderizada no servidor (`templ` + HTMX + Alpine.js)

**Contexto.** Um developer a solo, host ARM64 modesto, um diferencial funcional (importação) e um risco conhecido: o projeto crescer até à complexidade do Actual e não terminar. A decisão anterior (SPA + núcleo isomórfico) foi justificada pela pré-visualização de importação «instantânea» — benefício não quantificado. Medição: o servidor faz a pré-visualização de 3 000 linhas em menos de 2 s. A justificação não se sustenta.

Com a SPA removida, a fronteira de tipos com o browser — único custo real do Go — desaparece. A linguagem fica, portanto, livre.

**Decisão.**

| Camada | Escolha |
| --- | --- |
| Backend | **Go** (HTTP com `net/http` de Go 1.22+; serviços de domínio; *jobs* em goroutines) |
| Templates HTML | **`templ`** — templates tipados em Go, compilados; HTML inválido é erro de compilação |
| Interação | **HTMX** — fragmentos de página, sem estado de cliente |
| Estado local de UI | **Alpine.js** — ~15 KB, vendido no repositório, sem *build*. Divide-se assim: HTMX = estado do servidor; Alpine = estado do ecrã (`x-show`, modais, célula em edição) |
| CSS | **Tailwind CSS** com o **binário *standalone*** — sem Node, sem `package.json` |
| Assets | Vendidos/servidos pelo binário via `embed.FS` |
| Base de dados | `modernc.org/sqlite` (Go puro, sem cgo) |

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| TypeScript ponta a ponta (SPA React) | Exige API tipada, *router*, *cache* de estado no cliente, *bundler* e `node_modules`. Mais peças e mais RAM, para um produto que é formulários, tabelas e gráficos — não é limitado por desempenho de *rendering* |
| Go + SPA em TypeScript | Reintroduz a fronteira de tipos e o *codegen* que ADR-001 eliminou |
| **SolidJS** | Tecnicamente muito bom para o caso (reatividade fina, *bundles* pequenos). Perde no ecossistema: gráficos (Recharts é React-only), componentes e — decisivo para um developer a solo — **quantidade de respostas prontas na internet** |
| Go + `templ` + **Datastar** (em vez de HTMX + Alpine) | Só um ficheiro em vez de dois, com *signals* + SSE integrados. Tecnicamente mais elegante; preterido pelo critério «tecnologias aborrecidas e populares» — HTMX + Alpine tem uma ordem de magnitude mais exemplos e respostas |
| Go + `templ` + `_hyperscript` | Menos legível e menos comum que Alpine |
| Python (Django/FastAPI) | Ver ADR-003 |

**Consequências.**

- (+) **Uma linguagem, um processo, um binário.** Elimina-se: API tipada de cliente, *router* de cliente, *cache* de estado, hidratação, *bundler*, `node_modules` e o `docker build` com `npm ci`.
- (+) Estima-se **eliminação de aproximadamente metade do código total** face à opção SPA.
- (+) HTML inválido e campos em falta são erros de compilação (`templ`), não *runtime*.
- (+) Menos peças a quebrar em ARM64: sem módulos nativos, sem *toolchain* de Node.
- (+) Navegação e ligações profundas funcionam sem código: são URLs reais.
- (−) **A grelha de transações com edição *inline* é o pior caso.** Mitigação obrigatória: filtragem e **paginação *server-side***, com edição por linha via `hx-patch` e Alpine para o modo de edição. **Não** se tentam *swaps* de milhares de linhas; se um ecrã exigir 5 000 linhas em *scroll* contínuo, é tratado como exceção pontual e localizada, não como reversão para SPA.
- (−) Gráficos interativos custam mais do que com Recharts. Mitigação: SVG gerado no servidor para os gráficos principais; biblioteca JS apenas onde a interatividade for indispensável.
- (−) Interações muito dinâmicas pagam uma ida ao servidor. Aceitável em rede local/Tailscale, e mitigado por fragmentos pequenos e SSE.
- (−) Alpine.js pode degradar para «lógica no cliente» por descuido. Disciplina explícita: **nenhuma regra de negócio em JavaScript**; se precisar de regra, é porque devia ser um fragmento do servidor.

---

## ADR-013 — Ingestão por adapters (CSV, OFX, CAMT.053) com roteamento por conteúdo

> **Revisto em 2026-09-14** (ver ADR-016). A versão original incluía PDF como fonte de primeira classe. Foi removido do âmbito.

**Contexto.** O utilizador tem muitas contas e cartões: o trabalho não está na leitura de um ficheiro, está em encaminhar vários ficheiros e em garantir que nada se perde. Os formatos variam por instituição, e forçar um único formato obrigaria a trabalho manual justamente onde ele é maior.

**Decisão.**

1. **Uma interface de adapter para as fontes suportadas**, no mesmo pipeline. CSV, OFX e CAMT.053 produzem a mesma estrutura `RawRow{LineNo, PageNo, Cells[], RawRef}` e entram no pipeline no estágio 2. Perfil, normalização, regras, dedupe, pré-visualização, *commit* e *undo* são código partilhado.
2. **Roteamento por conteúdo, não por nome de ficheiro.** Uma única pasta `/data/inbox/`; tipo detetado por *magic bytes*; instituição por impressão digital (`FI`/`ACCTID` em OFX, `IBAN` em CAMT, assinatura de colunas em CSV). **OFX e CAMT identificam a conta exatamente**, sem heurística.
3. **OFX e CAMT.053 antes de qualquer outra coisa.** Trazem `FITID`/`AcctSvcrRef` (identificador estável do lançamento, logo deduplicação exata) e `ACCTID`/`IBAN` (roteamento exato). Cada instituição convertida para OFX nunca mais exige trabalho.
4. **Reconciliação declarada.** Usar como invariantes todos os números que a fonte declara: saldo (`declared_open + Σ linhas == declared_close`), totais de compra e contagens, quando existirem. Em OFX, `<LEDGERBAL>`/`<AVAILBAL>` dão isto gratuitamente. Divergência gera **explicação acionável** (linha em falta, duplicada, resumo indevido, sinal invertido), não apenas «não fecha». Um lote só tem `auto_commit` se a reconciliação fechar e não houver diagnósticos `error`.
5. **Modo degradado honesto.** Para uma instituição que não exporte nenhum formato suportado, importa-se **apenas o total declarado** como uma transação única. Saldos e dívida do cartão ficam corretos; perde-se o detalhe por comerciante. É uma troca consciente e reversível (ADR-016).

**PDF está fora de âmbito.** Ver [ADR-016](#adr-016--pdf-fora-de-âmbito).

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Só CSV, sem OFX/CAMT | Perde-se a deduplicação exata (`FITID`) e o roteamento exato (`ACCTID`/`IBAN`) — precisamente nas contas onde há mais trabalho. CSV continua a ser o núcleo; OFX/CAMT são o degrau acima quando existirem |
| Um pipeline separado por formato | Duplicaria perfil, dedupe, pré-visualização, *commit* e *undo*. Todos os formatos entram no pipeline partilhado, no mesmo ponto |
| Roteamento por nome de ficheiro ou por pasta por conta | Nomes de ficheiros de bancos são inconsistentes e ilegíveis; obriga a trabalho manual no passo que devia ser automático. Subpastas continuam a funcionar como *override* manual |
| Reconciliar apenas o saldo | Deixa de fora os totais de compra e as contagens, que detetam erros que o saldo não deteta (bloco de resumo incluído, parceladas futuras) |
| **Suportar PDF** | Ver [ADR-016](#adr-016--pdf-fora-de-âmbito). Estudo técnico preservado em [99-fora-de-ambito-pdf.md](99-fora-de-ambito-pdf.md) |
| Agregadores (Pluggy/Belvo) já no MVP | Fiabilidade alta, mas custo mensal por conexão e dependência de terceiros para dados financeiros. Permanecem como adapter futuro na mesma interface |

**Consequências.**

- (+) Um só caminho de código a partir do estágio 2; adicionar formato = adicionar adapter.
- (+) OFX/CAMT tornam roteamento e deduplicação **exatos**, eliminando as duas maiores fontes de heurística.
- (+) A reconciliação declarada é verificável e explica a divergência em vez de a reportar.
- (+) **Zero dependências de sistema no *runtime*.** Sem `poppler`, sem `tesseract`, sem cgo: a imagem pode ser `distroless/static` e o binário é o único artefacto.
- (+) Banco que muda o formato não parte nada: o diagnóstico aponta a causa e o mapeamento corrige-se no ecrã de perfil.
- (−) Um formato a menos significa menos cobertura: instituições que só exportam PDF ficam no modo degradado, com o detalhe por comerciante em falta nessa conta.
- (−) Quatro adapters (CSV, OFX, CAMT.053) em vez de um. Mitigado por emitirem a mesma estrutura.
- (−) O modo degradado exige disciplina de uso: é fácil cair nele e esquecer que a categorização daquela conta está incompleta.

---

## ADR-014 — Cartão de crédito como conta; pagamento de fatura como transferência emparelhada

**Contexto.** Este é o erro de contabilidade mais provável em quem tem vários cartões e queixa-se de trabalho manual: a fatura traz todas as compras e o extrato da conta corrente traz **uma única linha** — «PAGAMENTO FATURA CARTAO». Se ambos forem importados como despesa, **a despesa é contada duas vezes** e o orçamento fica errado em dois sentidos.

**Decisão.**

1. **Cada cartão é uma conta** (`type = 'credit'`), com `closing_day`, `due_day`, `brand` e `mask`. Compras são negativas; o saldo é passividade e entra no património líquido como tal.
2. **A linha de pagamento de fatura é convertida em transferência** (`transfer_id`) entre a conta corrente e a conta do cartão — deixando de ser despesa.
3. **O emparelhamento é exato, não heurístico**: o valor do pagamento é o **total declarado da fatura**, que é conhecido. Ordem de tentativa: (a) `declared_close_cents` de uma fatura do cartão igual ao valor do débito; (b) soma dos lançamentos do cartão no ciclo; (c) payee contém `FATURA`/`CARTAO` com conta candidata única → sugerir; (d) nada → **manter como despesa e avisar** com `CARD_PAYMENT_UNMATCHED`.
4. **Uma fatura pode alimentar várias contas.** `import_rows.target_account_id` permite dividir um ficheiro por cartão (titular/adicional), mantendo **um único *commit* e um único *undo*** para o lote. Secções como `Cartão final 1234` são reconhecidas por `section_rules` do perfil; os subtotais por secção têm de fechar com o total declarado.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Cartão como categoria/etiqueta, não como conta | Não representa dívida; o património fica errado no intervalo entre a compra e o pagamento |
| Compra contabilizada na conta corrente, ignorando a fatura | Perde-se o detalhe por comerciante e a data real da compra; e a fatura fica impossível de reconciliar |
| Pagamento tratado como despesa (comportamento por omissão de muitas ferramentas) | **Dupla contagem** — o defeito que motiva este ADR |
| Emparelhamento por heurística *fuzzy* de payee e data | Desnecessário: o total declarado está no documento e é exato. Usar heurística onde existe um identificador exato é escolher fragilidade |
| Ignorar o pagamento (deixar a conta corrente sem o débito) | Saldo da conta corrente errado |

**Consequências.**

- (+) Elimina o erro de dupla contagem — o ganho de correção mais importante do motor de importação.
- (+) O emparelhamento é determinístico e explicável («emparelhado com a fatura de agosto, total 4.231,88»).
- (+) O `imported_id` do pagamento passa a ser o `batch_id` da fatura, tornando o emparelhamento estável e idempotente.
- (+) O mesmo mecanismo serve `APLICACAO`/`RESGATE` entre conta corrente e investimento.
- (−) Exige atenção do utilizador para caso (d): um pagamento não emparelhado é sinal de que falta importar a fatura. É por isso que é um **aviso visível**, e não um silêncio.
- (−) O `flip_amount` e as convenções de sinal ficam mais importantes numa conta de cartão: um erro de sinal inverte o sentido da dívida. Daí a reconciliação de saldo ser obrigatória também aqui.

---

## ADR-015 — Biblioteca de perfis embutida no repositório

**Contexto.** O maior custo humano do produto não é o *parsing*: é configurar mapeamentos. Com muitas contas e cartões, cada instituição nova é uma sessão de configuração. Perfis são conhecimento sobre o mundo, não configuração pessoal — e repetem-se por todo o país.

**Decisão.** Perfis vivem como **dados versionados no repositório** (`profiles/*.yaml`, embutidos com `embed.FS`), cobrindo, como objetivo inicial, as principais instituições brasileiras (bancos, fintechs e cartões). Cada perfil traz: assinatura (CSV/OFX/PDF), mapeamento, opções de *parsing*, regras de `ignore`, limpeza de *payee*, códigos de tipo de operação, `section_rules` e expressões de totais declarados. Perfis do utilizador são *overrides* por cima da biblioteca, nunca substituições.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Cada utilizador cria os seus perfis do zero (plano anterior: «aprendizagem») | A aprendizagem só ajuda a partir da segunda vez e **por conta**. Não resolve o arranque, que é onde está o trabalho |
| Perfis guardados apenas na base de dados | Não são revistos, versionados nem partilháveis; um erro de perfil não aparece num *diff* |
| Perfis descarregados de um serviço remoto | Dependência de rede e de terceiros para dados que não são sensíveis; incompatível com o princípio *self-hosted* |
| Deteção puramente por heurística, sem perfis | Já existe e continua a existir como *fallback*; os perfis dão o resultado determinístico |
| Comandos/interfaces por banco (como catálogos de adaptadores por banco) | Um perfil de dados é mais simples de ler, escrever e contribuir do que código por instituição |

**Consequências.**

- (+) O arranque deixa de ser configuração: se a instituição está coberta, o trabalho do utilizador é **zero**.
- (+) Adicionar suporte a um banco é editar um ficheiro — sem código, sem *deploy*, sem testes novos (ADR-006 aplicado aos perfis).
- (+) Perfis são revistos em *pull request*, com *fixtures* anonimizadas a acompanhar.
- (+) Os perfis tornam-se um ativo partilhável, com o mesmo espírito das regras exportáveis.
- (−) Exige manutenção quando os bancos mudam de formato. Mitigado: uma alteração de formato é um *diff* de ficheiro, e o diagnóstico aponta a causa.
- (−) Requer um formato de perfil estável e documentado, mais dois níveis de resolução (biblioteca → override do utilizador). Complexidade aceite porque substitui trabalho recorrente.
- (−) Risco de perfis de baixa qualidade. Mitigado: perfis sem *fixture* de teste anonimizada não entram.

---

## ADR-016 — PDF fora de âmbito

**Contexto.** A versão anterior do plano tratava PDF como fonte de primeira classe, com extração posicional, editor visual de colunas e perfil de layout por instituição. Era, de longe, a peça mais cara do sistema — e a mais frágil: extração de tabelas por geometria falha em silêncio quando a tolerância de agrupamento está mal calibrada.

Em paralelo, o objetivo declarado é reduzir o **trabalho mensal** com muitas contas e cartões. As alavancas que o fazem — roteamento por conteúdo, biblioteca de perfis, painel de cobertura e emparelhamento de faturas — são independentes do formato e entregam muito mais, mais depressa e com menos risco.

Verificou-se ainda que a maioria das instituições brasileiras oferece CSV ou OFX, por vezes escondido num ecrã secundário ou na versão desktop do portal. O esforço de descobrir essa exportação é ordens de magnitude menor do que construir o extrator.

**Decisão.** **PDF está fora de âmbito.** As fontes suportadas são **CSV, OFX e CAMT.053**. Para instituições que não exportem nenhum destes formatos, aplica-se o **modo «apenas o total»**: uma transação com o valor total declarado, mantendo saldos, dívida de cartão e património corretos.

O estudo técnico completo é preservado em [99-fora-de-ambito-pdf.md](99-fora-de-ambito-pdf.md), para que reabrir o tema seja barato se um dia for necessário.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Manter PDF Tier A como planeado | Custo de construção e manutenção desproporcionado face ao benefício; introduz a única dependência de sistema no *runtime* (`poppler`), uma peça de UI dedicada (editor de colunas) e um eixo de erro silencioso. O sistema passa a ter duas classes de fontes, com garantias diferentes |
| PDF só para OCR leve (Tesseract) | Pior ainda: erro elevado em tabelas, mais uma dependência pesada (~100 MB com dados de português) e resultados que exigiriam revisão linha a linha — anulando a poupança de tempo |
| Sidecar Python (`pdfplumber`/`camelot`) para PDF | Reintroduz um segundo *runtime*, segundo conjunto de dependências, mais um contentor, e divide o corpus *golden* por duas linguagens. Defende-se em teoria; na prática, o custo de operação recai sobre um developer a solo |
| Agregador Open Banking (Pluggy/Belvo) para as contas só-PDF | Boa solução e recomendada **quando fizer falta**: é *outsourcing*, não construção. Fica como adapter futuro da mesma interface, com custo mensal |
| Modo «apenas o total» para as contas sem exportação | **Escolhida.** Zero risco de dados errados, um lançamento por mês, e reversível |
| Deixar essas contas completamente fora do sistema | Rejeitada: perde-se a dívida do cartão no património líquido, que é informação essencial |

**Consequências.**

- (+) **Remoção de peças inteiras**: adapter PDF, extração posicional, editor visual de colunas, perfil de layout, dependência de `poppler-utils` e `tesseract`, imagens de página, `layout_json`, diagnósticos `PDF_*`.
- (+) **A imagem Docker volta a poder ser `distroless/static`** (~10 MB), sem binários externos. O binário Go é o único artefacto.
- (+) Um eixo de fragilidade a menos: sem extração por geometria, não há erro silencioso dessa origem.
- (+) Corpus de teste mais simples: ficheiros estruturados, comparáveis linha a linha sem tolerância.
- (+) Foco: o esforço vai para o que reduz trabalho de facto — roteamento, perfis, cobertura, *fatura matcher*.
- (−) **Perde-se o detalhe** nas contas sem exportação estruturada: o total entra, os lançamentos individuais não. O orçamento por categoria fica incompleto nessas contas, ainda que os saldos estejam corretos.
- (−) Se muitas contas caírem nesse caso, o valor da categorização automática cai proporcionalmente. É uma regressão no *detalhe*, não na *correção*.
- (−) O painel de cobertura é menos informativo nessas contas: sabe-se que o total foi lançado, não que os lançamentos estão todos lá.
- *Reabertura*: exige uma decisão nova, justificada com **um documento concreto** de uma instituição específica, e seguindo a ordem do documento 99 (agregador → extração pontual → generalização).
- *Ideia preservada*: a **reconciliação por totais declarados** nasceu do estudo de PDF e foi mantida e generalizada em CSV/OFX/CAMT — é o único contributo do estudo que ficou no âmbito ativo (ADR-013).

---

## ADR-017 — Otimização de contexto para agentes de IA

**Contexto.** Boa parte do desenvolvimento deste projeto corre com assistentes de IA. O custo dominante não é o tamanho do repositório: é o **contexto que tem de ser redescoberto em cada pedido**. Sem regras escritas, cada pedido paga repetidamente por: (a) o agente propor decisões já rejeitadas (PDF, Node, ORM, SPA); (b) reler documentos inteiros por falta de índice navegável; (c) responder a perguntas que um ADR já responde; (d) explorar a estrutura do repositório. Nenhum destes custos aparece numa fatura, e todos se pagam diariamente.

**Decisão.** Tratar a **economia de contexto como requisito de primeira classe**, com regras concretas e verificáveis:

1. **Contexto permanente curto.** `.github/copilot-instructions.md` contém apenas o que é não negociável, violado com frequência e caro de descobrir — âmbito de formatos, stack, invariantes de domínio, estrutura, testes. **Orçamento: ≤ 60 linhas.**
2. **Contexto condicional por omissão.** Instruções de ficheiro usam `applyTo` específico (`docs/**/*.md`, `**/*.go`). **`applyTo: "**"` é proibido** — carrega em todos os pedidos, mesmo nos irrelevantes.
3. **Tarefas repetitivas empacotadas** em *prompts* (`.github/prompts/`); conhecimento de domínio acessível sob demanda por *skills*, não residente.
4. **Agentes restritos** (`.github/agents/`) com o conjunto mínimo de ferramentas para a função. Menos ferramentas significa menos exploração e menos tokens.
5. **O que tem de ser garantido vai para *hooks*.** Formatação e verificação de compilação são determinísticas e **não** devem passar pelo modelo. Instruções guiam; *hooks* enforçam.
6. **Índice navegável no topo dos documentos grandes** — permite decidir sem abrir o ficheiro. É a otimização de maior retorno já aplicada neste repositório.
7. **Higiene de sessão**: uma sessão por tarefa; estado em ficheiros em vez do histórico; referenciar por ficheiro e secção em vez de colar conteúdo.
8. **Estrutura de código previsível** e diagnósticos com código estável — ambos reduzem a exploração e o diálogo.
9. **Modelo por tarefa**: pequeno para trabalho mecânico verificado por testes; grande para decisões irreversíveis.
10. **Manutenção explícita**: revisão trimestral das instruções contra os ADRs vigentes, e regra de remoção — instrução que não evita pelo menos uma ida-e-volta por semana sai.

Detalhe operacional, inventário dos ficheiros e antipadrões em [08-otimizacao-de-contexto.md](08-otimizacao-de-contexto.md).

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Não ter política; deixar ao critério de cada sessão | O custo é invisível e recorrente. É precisamente o tipo de decisão que se paga por não ser tomada — e obriga a re-explicar o âmbito em cada sessão nova |
| Um único documento de contexto extenso, sempre carregado | Paga-se integralmente em **todos** os pedidos, incluindo os que nada têm a ver. É o oposto do objetivo: transforma custo variável em custo fixo alto |
| Copiar os ADRs para o contexto permanente | Duplicação paga diariamente e sujeita a *drift*. O ADR é a fonte; o contexto permanente cita, não resume |
| Confiar só em instruções para formatação e testes | Instruções são não determinísticas. O que tem de acontecer sempre vai para *hooks* |
| `applyTo: "**"` em todas as instruções | Carrega tudo em todos os pedidos. É o antipadrão central desta área |
| Dividir `05-decisoes-adr.md` num ficheiro por ADR | Reduz o custo de ler um ADR isolado, mas fragmenta 17 ficheiros e exige índice redundante. **Adiado**: reavaliar se passar de ~60 KB |
| Não usar *hooks* (só instruções) | Deixa formatação e compilação ao critério do modelo, acrescentando idas-e-voltas que um `gofumpt -w` resolve sem gastar contexto |

**Consequências.**

- (+) **Custo por pedido previsível e menor**: o âmbito e as invariantes deixam de ser redescobertos.
- (+) As decisões rejeitadas ganham retorno direto — o agente não as propõe outra vez, porque estão no contexto permanente como proibições explícitas.
- (+) Menos contexto gasto em exploração, por a estrutura ser previsível e por haver índice nos documentos.
- (+) Formatação e compilação passam a ser determinísticas, fora do caminho pago.
- (+) O retorno de escrever ADRs aumenta: além de documentar para humanos, elimina re-litígio pelo agente.
- (−) **Custo de manutenção real**: ficheiros de customização apodrecem em silêncio. Modo de falha verificado neste projeto — `.github/copilot-instructions.md` afirma que PDF está excluído, e se um dia essa decisão for revista sem atualizar o ficheiro, o agente passa a contrariar o ADR vigente.
- (−) O próprio contexto permanente é um custo fixo: daí o orçamento de 60 linhas e a regra de remoção.
- (−) Risco de *over-fitting*: regras demasiado específicas envelhecem como as decisões que descrevem.
- (−) *Hooks* introduzem código de *shell* no repositório, que precisa de ser revisto como qualquer código.
- *Métrica de saúde*: se uma instrução não evitou pelo menos uma ida-e-volta por semana, deve ser removida.
