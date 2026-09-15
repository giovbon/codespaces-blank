# 09 — Escopo: o que extrair do Actual e o que descartar

O [Actual Budget](https://github.com/actualbudget/actual) é a **referência funcional** deste projeto (ADR-011), não a base de código. Este documento é o **portão de escopo**: para cada módulo do repo de origem, diz se aproveitamos a ideia, adaptamos com divergência, adiamos ou descartamos.

Serve para uma coisa só: impedir que o projeto cresça até à complexidade do Actual e não termine — risco já registado no ADR-012.

## Índice

1. [Como usar este documento](#1-como-usar-este-documento)
2. [O que o repo do Actual é (e não é) para nós](#2-o-que-o-repo-do-actual-é-e-não-é-para-nós)
3. [Mapa do repo de origem](#3-mapa-do-repo-de-origem)
4. [Matriz de escopo](#4-matriz-de-escopo)
5. [Achados que mudam prioridade](#5-achados-que-mudam-prioridade)
6. [Divergências conscientes](#6-divergências-conscientes)
7. [Filtro de admissão de escopo](#7-filtro-de-admissão-de-escopo)
8. [Manutenção](#8-manutenção)

---

## 1. Como usar este documento

Antes de escrever código para qualquer módulo, procure a linha do módulo equivalente nesta matriz.

| Decisão | Significado |
| --- | --- |
| **Reutilizar** | A ideia do Actual já resolve. Copiar o *comportamento*, escrever em Go. Nunca copiar o ficheiro (ADR-011) |
| **Adaptar** | A ideia serve, mas com divergência deliberada — a divergência está na [§6](#6-divergências-conscientes) e tem de ter motivo |
| **Adiar** | Não entra no MVP. Não construir, não desenhar, não reservar espaço no esquema |
| **Descartar** | Contradiz um ADR vigente ou serve um caso de uso que não temos. **Não reabrir sem um ADR novo** |
| **Decidir** | Ponto em aberto. Não construir antes de estar resolvido aqui ou num ADR |

**Regra de leitura.** O código do Actual é TypeScript com React, SQL e arquitetura *local-first* — nenhuma linha é portável. O que é portável é a **decisão**: quais campos guardar, em que ordem correr as verificações, o que preferir em caso de empate.

---

## 2. O que o repo do Actual é (e não é) para nós

| É | Não é |
| --- | --- |
| Um **dicionário de decisões de domínio já testadas** em produção | Um repositório de código para traduzir |
| A prova de que certos problemas são reais (dupla contagem, dedupe, fusão de duplicados) | Uma lista de funcionalidades a atingir |
| Fonte de **formatos de dados maduros** (contas, transações, regras, orçamento) | Uma arquitetura a imitar (é *local-first* com CRDT) |
| Fonte de **casos de teste da vida real** (as suas *fixtures* cobrem bancos estrangeiros) | Um produto com o nosso caso de uso (ele é multiusuário e multi-dispositivo) |

O ganho real está em não repetir erros que custaram anos a corrigir. Exemplo concreto: o Actual mantém `imported_id` e `imported_payee` **separados** dos campos editados pelo utilizador. Isso não é detalhe de implementação — é o que permite reimportar e fundir sem destruir trabalho manual. Já adotamos os dois.

---

## 3. Mapa do repo de origem

Dez pacotes. Três interessam.

| Pacote | Conteúdo | Interesse |
| --- | --- | --- |
| `loot-core` | Toda a lógica: base de dados, regras, orçamento, importação | **Alto** — é onde estão as decisões |
| `desktop-client` | UI React (inclui o ecrã de importação e o de *bank sync*) | Médio — referência de fluxo, não de código |
| `api` | API pública de automação | Baixo — adiado |
| `sync-server` | Servidor de sincronização multiusuário | Nenhum (ADR-001) |
| `crdt` | *Merkle trie* e resolução de conflitos | Nenhum (ADR-001) |
| `desktop-electron` | Invólucro desktop | Nenhum (só navegador) |
| `component-library` | Componentes visuais | Nenhum (usamos Tailwind) |
| `plugins-service` | *Plugins* de terceiros | Nenhum no MVP |
| `eslint-plugin-actual` | Regras de *lint* internas | Nenhum |
| `docs` | Documentação do produto | Baixo — útil para entender comportamento |

Dentro de `loot-core/src/server/`, os módulos relevantes são: `accounts` (com `sync`, `link`, `title`), `transactions` (com `merge` e `transaction-rules`), `rules`, `payees`, `budget`, `mutators`, `undo`, `migrate`, `models`, `preferences`. A lista de aplicações registadas em `server/main.ts` dá o inventário completo: `accounts`, `account-groups`, `admin`, `auth`, `budget`, `budgetfiles`, `dashboard`, `encryption`, `filters`, `forecast`, `formulas`, `notes`, `payees`, `preferences`, `reports`, `rules`, `schedules`, `spreadsheet`, `sync`, `tags`, `transactions`, `tools`.

---

## 4. Matriz de escopo

### 4.1 Importação e dados — o núcleo

| Módulo do Actual | O que faz | Decisão | Nota |
| --- | --- | --- | --- |
| `accounts/sync.ts` → `matchTransactions` | Dedupe em passagens: `imported_id` exato → ±7 dias com mesmo valor → *payee* igual → primeira disponível | **Reutilizar** | É o nosso T0–T3 ([04](04-motor-importacao-csv.md) §7). A ordem das passagens é a decisão valiosa |
| `accounts/sync.ts` → `reconcileTransactions` | Funde importado com existente, com `isPreview` e `reimportDeleted` | **Reutilizar** | O modo *preview* separado do *commit* confirma ADR-008 |
| `accounts/sync.ts` → `normalizeActions` / `normalizePayeeName` | Limpeza de *payee*: **apenas** `trim` + *title-case* | **Adaptar** | Insuficiente para bancos brasileiros. Ver [§6](#6-divergências-conscientes) |
| `accounts/link.ts` | Liga lançamentos como transferência | **Reutilizar** | O nosso *fatura matcher* (ADR-014) é a generalização disto |
| `accounts/title/` | *Title-case* com dicionário de exceções | **Descartar** | Consequência de manter *title-case*: estraga siglas (`NY` → `Ny`) |
| `transactions/merge.ts` → `determineKeepDrop` | Escolhe qual duplicado sobrevive | **Adaptar** | Divergência de mérito: ver [§6](#6-divergências-conscientes) |
| `transactions/transaction-rules.ts` | Regras com *stages*, `RuleIndexer` por primeiro caractere | **Reutilizar** | Já é o nosso [04](04-motor-importacao-csv.md) §6 e ADR-006 |
| `transactions/transaction-rules.ts` → `updatePayeeRenameRule` | Renomear um *payee* cria/estende uma regra `pre` | **Reutilizar** | É a «aprendizagem» que o Actual tem de facto: **regra**, não estatística |
| `payees/app.ts` | *Payee* com `default_category_id`; *payee* de transferência por conta | **Reutilizar** | Duas ideias fortes: categoria herdada e *payee* de transferência |
| `rules/app.ts` | CRUD de regras, condições → ações | **Reutilizar** | ADR-006 |
| `notes/` | Nota por transação | **Reutilizar** | Barato e usado no dia a dia |
| `models/`, `db/`, `migrate/` | Camada de modelos, acesso a dados, migrações | **Reutilizar a ideia** | Em Go: `internal/data` + `migrations/` para a frente |
| `mutators/`, `undo/` | Toda a escrita passa por mutador; *undo* | **Reutilizar** | ADR-007. Já é invariante do projeto |
| `aql/` (Actual Query Language) | Linguagem de consulta própria | **Descartar** | SQL vive em `internal/data`; uma DSL de consulta é um segundo produto |
| `importers/ynab4`, `importers/ynab5` | Migração de orçamentos YNAB | **Descartar** | Não somos um migrador |
| `spreadsheet/`, `formulas/` | Motor de planilha e fórmulas (PEGGY) | **Descartar** | ADR-009 |
| `encryption/` | Encriptação do ficheiro de orçamento | **Decidir** | Ver [§7](#7-filtro-de-admissão-de-escopo). Sem decisão, não construir |

### 4.2 Domínio financeiro

| Módulo do Actual | O que faz | Decisão | Nota |
| --- | --- | --- | --- |
| `budget/` | Orçamento mensal por categoria, transações-mãe/filha | **Adaptar** | Mantemos o modelo; trocamos o motor por agregados materializados do ADR-009 |
| `accounts/app.ts` → contas *off-budget*, `closed` | Distinção orçamento/fora e conta fechada | **Reutilizar** | `closed` alimenta o painel de cobertura ([07](07-muitas-contas-e-cartoes.md) §6) |
| `schedules/` | Transações agendadas/recorrentes | **Adiar** | Não reduz o tempo de importação. É funcionalidade de paridade |
| `reports/` | Relatórios configuráveis | **Adiar** | v1 tem o essencial; relatórios entram por último |
| `dashboard/` | Painel de widgets | **Adiar** | O painel que interessa é o de **cobertura**, que é nosso |
| `filters/` | Filtros guardados | **Adiar** | Vive como regra quando for preciso |
| `tags/` e `transaction_tags` | Etiquetas | **Descartar** | Fora do MVP por decisão |
| `attachments/` | Anexos de transação | **Descartar** | Fora do MVP por decisão |
| `forecast/` | Projeção de saldo futuro | **Descartar** | Fora do âmbito declarado |
| `account-groups/` | Agrupamento de contas | **Adiar** | Só se o número de contas o exigir |

### 4.3 Plataforma e infraestrutura

| Módulo do Actual | O que faz | Decisão | Nota |
| --- | --- | --- | --- |
| `sync/` + pacote `crdt` | Sincronização entre dispositivos | **Descartar** | ADR-001. Conflito de replicação não existe com um servidor |
| `sync-server/` | Servidor de sincronização multiusuário | **Descartar** | ADR-001 |
| `cloud-storage/` | *Backup* para armazenamento remoto | **Descartar** | ADR-010: Litestream no volume + `restic` ([06](06-operacao-arm64.md)) |
| `admin/` | Gestão multiusuário, transferência de propriedade | **Descartar** | Um operador de dados |
| `auth/` | *Login*, *openid*, tokens de sessão | **Adaptar** | Uma tabela `users` e uma sessão; sem registo, sem recuperação de senha |
| `preferences/` | Preferências globais e sincronizadas | **Adaptar** | Reduzir a `app_settings` (já no esquema) |
| `api/` (REST de automação) | API externa com token | **Adiar** | A fronteira REST já está prevista (ADR-004) para automação; não é v1 |
| `tools/` | Utilitários internos de manutenção | **Descartar** | Não aplicável |
| `desktop-client/` (React, `mobile/`, `responsive/`) | Toda a UI | **Descartar** | ADR-012: `templ` + HTMX + Alpine.js |
| `component-library/` | Biblioteca de componentes | **Descartar** | Tailwind |

### 4.4 O que o Actual tem e nós não tínhamos previsto

| Módulo do Actual | O que faz | Decisão |
| --- | --- | --- |
| `accounts/banksync` + `desktop-client/src/components/banksync/` | Integração nativa com agregadores: **SimpleFIN**, **Pluggy.ai (Brasil)**, GoCardless, Akahu, EnableBanking | **Descartar a integração** — a ideia é boa, o preço não (ADR-021). Ver [5.1](#51-agregadores-de-extratos-pagos-e-caros) |
| `desktop-client/.../ImportTransactionsModal/` | Ecrã de importação: mapeamento de campos, `reconcile`, pré-visualização com tabela | **Adaptar com divergência forte** — é exatamente o ecrã que faz o trabalho voltar ([§6](#6-divergências-conscientes)) |
| `accounts/sync.ts` → `custom-sync-mappings-{accountId}` | Mapeamento de campos personalizado **por conta**, guardado como preferência | **Reutilizar** — coincide com `import_profiles` ter âmbito por conta |

---

## 5. Achados que mudam prioridade

Quatro coisas encontradas no repo de origem que alteram o plano.

### 5.1 Agregadores de extratos: pagos e caros

`packages/loot-core/src/server/accounts/sync.ts` contém `downloadSimpleFinTransactions`, `downloadPluggyAiTransactions`, `downloadAkahuTransactions` e `downloadEnableBankingTransactions`. **Pluggy.ai é brasileiro** e cobre bancos e cartões nacionais — e foi por isso que, numa primeira leitura, pareceu a resposta ao maior bloco de tempo que resta: obter os ficheiros.

**A realidade de custo derruba a ideia.** Verificado em uso real: a tentativa de usar o Actual com o Pluggy foi abandonada porque **a API é paga e cara** para uso pessoal. O mesmo se aplica ao Belvo. Um agregador não é um custo de infraestrutura de self-hosting — é uma assinatura mensal recorrente, que contradiz o pressuposto de operação a custo marginal zero. Decisão registada no ADR-021.

**O que fica no lugar.** A via de entrega automática que resta é o **anexo de email por IMAP** ([07](07-muitas-contas-e-cartoes.md) §2, degrau 4): gratuita, e muitos bancos já enviam OFX/CSV por email sem o utilizador saber.

**A leitura correta para o desenho:** não construir nenhuma integração de agregador, e **não reservar espaço no esquema para ela**. O que se mantém é a exigência de que a interface de *adapter* do ADR-013 não presuma que a origem é sempre um ficheiro — o IMAP entrega um anexo, que é *quase* um ficheiro.

### 5.2 O Actual não tem a aprendizagem estatística que planeamos

Duas buscas confirmam: **não existe tabela de mapeamento com contagem de acertos**. O que existe é:

- `updatePayeeRenameRule`: renomear um *payee* durante a importação cria ou estende uma regra `pre` com condição `imported_payee oneOf [...]`;
- `payee.default_category_id`: *payee* resolvido herda a categoria padrão;
- regras de *payee* definidas pelo utilizador.

Ou seja, a «aprendizagem» do Actual é **geração automática de regras**, não estatística. O nosso `payee_mappings` com `hits`/`misses`/`confidence` ([03](03-modelo-de-dados.md) §2.3, ADR-006) é mais ambicioso do que a referência.

**Consequência prática:** não há comportamento de referência para calibrar os limiares. `hits >= 3` e `confidence >= 0.85` são um palpite informado e **têm de ser validados com uso real** antes de se confiar neles para `auto_commit`. A aprendizagem do Actual é, no fundo, o *fallback* garantido: se os limiares se revelarem ruins, cai-se para regras explícitas.

### 5.3 A normalização de *payee* do Actual é fraca

`PAYEE_NAME_NORMALIZATIONS` tem dois valores: `'original'` e `'title-case'`. O padrão é `title-case`, e o efeito é visível nos próprios testes: `Nintendo Store New York NY` → `Nintendo Store New York Ny`.

O Actual mantém o nome bruto em `imported_payee` e aplica regras por cima — o que é sólido — mas não remove ruído de banco: `PIX`, `TED`, datas embutidas, CNPJ, CPF, número de documento. O [04](04-motor-importacao-csv.md) §5.6 faz isso. **A nossa abordagem é melhor para o objetivo**, e a referência não serve para calibrá-la. Fica registado para não se «simplificar» para `title-case` por parecer mais próximo do Actual.

### 5.4 O Actual mantém três ideias que devem ser copiadas tal e qual

| Ideia | Onde | Porque copiar |
| --- | --- | --- |
| `imported_id` e `imported_payee` **separados** dos campos editados | `sync.ts`, `merge.ts` | Permite reimportar e fundir sem destruir trabalho manual. É a base do nosso `undo` |
| Passagens de dedupe em **ordem decrescente de fidelidade** | `sync.ts` `transactionsStep1/2/3` | Cada passagem consome o resultado da anterior (`hasMatched`), o que evita fusões cruzadas |
| Resolver o *payee* **antes** de correr as regras | `sync.ts` `normalizeTransactions` | O comentário no código de origem é explícito: resolver conta e *payee* cedo faz com que as regras recebam os dados certos. Já é o nosso [04](04-motor-importacao-csv.md) §5.6 antes do §6 |

---

## 6. Divergências conscientes

Onde decidimos **diferente** do Actual, com o motivo. Divergência sem motivo é regressão.

| Tema | Actual | Nós | Porque |
| --- | --- | --- | --- |
| Dedupe de duplicados — qual sobrevive | `determineKeepDrop`: sobrevive o de *bank sync*, depois o importado, depois o manual; empate resolvido **pelo mais antigo** | Preferimos os dados do **ficheiro mais recente** em `date`/`amount_cents` ([07](07-muitas-contas-e-cartoes.md) §5.3) | Em cartões de crédito o lançamento muda de data quando migra de ciclo entre faturas. Ficar com o mais antigo perpetua a data errada |
| Normalização de *payee* | `trim` + `title-case`, original preservado | NFC, remoção de acentos para comparação, maiúsculas, remoção de ruído de banco, extração de semântica | O texto dos bancos brasileiros não sobrevive a `title-case` |
| Aprendizagem | Regra `pre` criada ao renomear | Estatística com `hits`/`misses`/`confidence` + regras | Uma regra por *payee* renomeado cresce sem limite e não tem confiança |
| Pré-visualização | Tabela com todas as linhas, com seleção e fusão linha a linha | **Por exceção**: só o que tem diagnóstico sobe à UI (ADR-018) | É o que separa «importar» de «importar com trabalho». Ver [§7](#7-filtro-de-admissão-de-escopo) |
| Dupla contagem cartão ↔ conta | Existe o modelo (cartão é conta), mas o emparelhamento da fatura é manual | **Emparelhamento automático pelo total declarado** (ADR-014) | O valor é exato e está declarado no documento: não é caso para heurística *fuzzy* |
| Cobertura de importação | Não existe vista equivalente | **Painel de cobertura** ([07](07-muitas-contas-e-cartoes.md) §6) | Ataca diretamente a fricção F6, que é tempo |
| Deteção de perfil | *Field mapping* por conta, escolhido pelo utilizador | **Assinatura de conteúdo** + biblioteca de perfis (ADR-013/ADR-015) | Zero configuração é o objetivo do produto |
| Arquitetura de dados | *Local-first*, SQLite em WASM, CRDT | Servidor único, SQLite em Go (ADRs 001, 002, 012) | Servidor sempre ligado |

**Ponto em aberto registado.** A divergência de `determineKeepDrop` **não está resolvida no esquema**: `docs/03` §2.4 declara a preferência pelo ficheiro mais recente, mas é preciso confirmar que a fusão ([04](04-motor-importacao-csv.md) §7, T3) usa esse critério e não a ordem de chegada. Se não usar, é um defeito silencioso.

---

## 7. Filtro de admissão de escopo

Três perguntas, respondidas **antes** de escrever código. Se qualquer resposta for «não», a funcionalidade não entra.

1. **Já resolveram isso?** → adotar a ideia, escrever em Go. Não copiar ficheiros (ADR-011).
2. **Reduz o tempo de importação, ou é necessário para a correção dos dados?** → se não, é paridade funcional; entra na coluna **Adiar** e não se constrói.
3. **Existe um ficheiro real (fixture) que prova a necessidade?** → perfil sem *fixture* não entra, e funcionalidade sem caso real também não.

A pergunta 2 é a que mais trabalho evita. O critério de tempo e a ordem de construção estão no [ADR-018](05-decisoes-adr.md#adr-018--o-objetivo-primário-é-o-tempo-de-importação).

**Antipadrão a evitar.** «Reservar espaço no esquema para o futuro». Um campo não usado é custo de migração, de teste e de contexto, sem retorno. O esquema implementa o que existe.

---

## 8. Manutenção

- Revisão **trimestral** desta matriz, em conjunto com a revisão das instruções exigida pelo ADR-017.
- Cada módulo do Actual que passar a **Reutilizar** deve citar o ficheiro de origem na linha, para que verificar uma decisão seja barato.
- Código do Actual efetivamente reutilizado (fragmento, não ideia) obriga ao aviso MIT e à atribuição (ADR-011). **Situação atual: nenhum fragmento reutilizado.**
- Divergência nova entra na [§6](#6-divergências-conscientes) no mesmo *pull request* que a introduz.
