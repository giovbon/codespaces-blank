# 05 — Decisões de Arquitetura (ADRs)

Formato: **Contexto → Decisão → Alternativas avaliadas → Consequências**.

---

## ADR-001 — Monólito modular *server-centric* com núcleo de domínio isomórfico

**Contexto.** O Actual Budget é *local-first*: toda a lógica corre no browser sobre SQLite em WebAssembly, e a sincronização entre dispositivos é feita por mensagens de CRDT com *merkle trie* e resolução automática de conflitos. Isso é necessário porque o Actual suporta app desktop, uso offline e múltiplos dispositivos concorrentes. Aqui, existe sempre um servidor ligado e o acesso é apenas por navegador.

**Decisão.** O **servidor ARM64 é a fonte de verdade**. O browser é uma SPA que consome REST/JSON e SSE, mas partilha os pacotes `contracts`, `domain`, `rules` e `import-core` com o servidor, executados num *web worker*, de modo que a pré-visualização de importação seja instantânea.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| *Local-first* com CRDT como o Actual | Meses de trabalho em sync, mensagens, reparação e *merkle*; benefício nulo com servidor sempre ligado; introduz modos de falha difíceis de depurar |
| *Local-first* simplificado (IndexedDB + sync bruto por *timestamp*) | Sincronização por *timestamp* corrompe-se com relógios dessincronizados e escritas concorrentes; exigiria lógica de resolução de conflitos de qualquer forma |
| Cliente fino puro (zero código partilhado) | A pré-visualização de importação passaria a depender de ida e volta ao servidor; pior experiência exatamente no fluxo mais crítico |
| Micro-serviços | Carga de um utilizador; custo operacional desproporcionado em ARM64 |

**Consequências.**

- (+) Simplicidade radical: uma base de dados, um processo, uma migração.
- (+) A lógica de importação corre nos dois lados sem duplicação de código.
- (+) Um `docker compose` substitui toda a orquestração do Actual.
- (−) Sem suporte offline real: sem rede, não há alterações. Aceitável para o caso de uso.
- (−) Exige disciplina para manter `domain` e `import-core` puros (sem importar `node:*`, sem `fs`, sem `process`).
- *Mitigação da disciplina*: proibir `node:*` em `packages/domain|rules|import-core` via regra de lint com `no-restricted-imports`.

---

## ADR-002 — SQLite em modo WAL como base de dados única

**Contexto.** Um utilizador, um servidor ARM64, dezenas de milhares de transações a crescer para centenas de milhares ao longo de anos. Custo operacional importa mais do que escala teórica.

**Decisão.** SQLite em WAL, base de dados única em volume persistente, acessível só pelo processo Node.

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
- *Caminho de saída*: todo o SQL isolado em `apps/server/src/data/`, migração para PostgreSQL como exercício localizado.

---

## ADR-003 — TypeScript ponta a ponta com Zod como fonte única de verdade

**Contexto.** Um único developer a manter backend, frontend, motor de importação e UI. Cada fronteira de tipos que exija `codegen` ou sincronização manual custa tempo de manutenção para sempre.

**Decisão.** TypeScript em todas as camadas; *schemas* Zod em `packages/contracts`, consumidos tanto para validar pedidos no servidor como para tipar formulários e respostas no cliente.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Go ou Python no backend | Duplicação da lógica de domínio e do motor de importação; perda da execução isomórfica no browser |
| Rust | Excelente desempenho, custo de desenvolvimento proibitivo para este âmbito |
| OpenAPI + `codegen` | Etapa adicional no *build*, *drift* garantido entre geração e uso |
| JavaScript sem tipos | O motor de importação lida com ambiguidade de formatos; tipos são a primeira linha de defesa |

**Consequências.**

- (+) Renomear um campo aparece como erro de compilação em todos os pontos de uso.
- (+) Validação e documentação derivam da mesma definição.
- (+) Refactoring seguro em todo o monorepo.
- (−) `strict: true` custa algum atrito inicial; compensa rapidamente.
- (−) Armadilha conhecida: `z.coerce` pode transformar `undefined` em `NaN` silenciosamente — usar `z.coerce.number().finite()` com validação explícita em valores monetários.

---

## ADR-004 — REST/JSON + SSE, e não tRPC, GraphQL ou WebSocket

**Contexto.** Cliente único (a SPA), mas com necessidade real de automação: *scripts*, *cron*, `curl`, integrações futuras.

**Decisão.** REST versionado em `/api/v1` com validação Zod e OpenAPI gerado; SSE para eventos do servidor.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| tRPC | Excelente para um só cliente TypeScript, mas fecha a porta a automação por `curl`/`cron` — que é justamente um requisito para a importação automática |
| GraphQL | Complexidade de esquema e *resolvers* sem consumidores heterogéneos que a justifiquem |
| WebSocket | Bidirecional que não é necessário; mais código de reconexão, *heartbeat* e estado |

**Consequências.**

- (+) A importação automática pode ser disparada de qualquer sítio.
- (+) SSE dá reconexão nativa, funciona através de proxies e resolve invalidação de *cache* e progresso de *jobs*.
- (−) Ausência de tipos automáticos: mitigada por cliente tipado gerado a partir do OpenAPI, *build-time*, sem `codegen` manual no ciclo de desenvolvimento.
- (−) Limite de conexões SSE em HTTP/1.1: irrelevante, são poucas abas; o proxy usa HTTP/2.

---

## ADR-005 — Dinheiro em inteiros e datas civis

**Contexto.** É o sistema mais suscetível a erro subtil de todo o projeto. `0.1 + 0.2 !== 0.3` em ponto flutuante; e uma data de transação representada como instante UTC pode aparecer no dia anterior.

**Decisão.**
- Quantias: `INTEGER` em cêntimos. Nenhum valor monetário existe como `number` fracionário em nenhum ponto do sistema.
- Datas de transação: `TEXT 'YYYY-MM-DD'`, tratadas como datas civis, sem fuso.
- Instantes de sistema: `INTEGER` em milissegundos *epoch*.
- `decimal.js` usado exclusivamente na fronteira de parsing de CSV.

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
- (−) Toda a conversão passa por utilitários em `packages/domain/money.ts` (`parseAmount`, `formatAmount`, `sumCents`), testados com `fast-check`.
- (−) Um `±1` cêntimo em conversões de moeda é inevitável; o arredondamento tem de estar localizado e documentado numa única função.

---

## ADR-006 — Regras como dados (DSL JSON) e aprendizagem, em vez de ML

**Contexto.** Categorizar automaticamente é o que torna a importação útil. As regras precisam de ser editáveis pelo utilizador, portáveis e auditáveis. O host é ARM64 modesto.

**Decisão.** Motor de regras com condições e ações em JSON, avaliadas por um interpretador com índices, executadas em três *stages* (`pre`, `default`, `post`). Complementado por aprendizagem estatística simples em `payee_mappings` (contagem de acertos e confiança). Sem *machine learning* no MVP.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Regras em código TypeScript | O utilizador não pode editar; cada mudança exige *deploy* |
| Modelo de ML embutido | Exige *dataset* rotulado e avaliação; corre risco de classificar errado em silêncio e sem explicação |
| LLM a categorizar cada linha | Custo por chamada, latência, dependência de rede, privacidade de dados financeiros, resultados não determinísticos |
| Apenas regras por *payee* histórico | Pouco expressivo; impossível capturar padrões como «valor entre X e Y e conta Z» |

**Consequências.**

- (+) Totalmente auditável: a pré-visualização mostra exatamente qual regra disparou.
- (+) Regras exportáveis e partilháveis pela comunidade.
- (+) A aprendizagem melhora resultados sem infraestrutura, e uma correção humana vale mais do que um *retrain*.
- (−) Requer um índice eficiente para não avaliar todas as regras em todas as linhas.
- (−) Fronteira conhecida: casos não capturáveis por padrões simples. Aceitável — a categorização manual continua a ser possível e rápida.
- *Evolução possível*: `import-core` devolve, por linha, um vetor de características; um classificador leve (regressão logística sobre *hashes* de *payee*) pode ser acrescentado depois **como sugestão**, nunca como decisão automática.

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

**Decisão.** Um `docker compose` com três componentes: aplicação (imagem `linux/arm64` baseada em `node:22-bookworm-slim`), Caddy como *reverse proxy* com TLS, e Litestream a replicar a base de dados para armazenamento de objetos. Acesso restrito por Tailscale. Restic para anexos e exportações.

**Alternativas avaliadas.**

| Alternativa | Porque não |
| --- | --- |
| Kubernetes / k3s | Complexidade operacional absurda para um serviço |
| Proxmox LXC | Válido, mas Docker Compose é mais simples de atualizar e versionar |
| Binário + systemd sem container | Mais leve (menos 100 MB de RAM), mas atualizações e dependências tornam-se manuais e frágeis |
| Base Alpine | Menor, mas musl vs glibc causa recompilação de módulos nativos; ganho de dezenas de MB não compensa o risco |
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
