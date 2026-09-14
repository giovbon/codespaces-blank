# Instruções do projeto

Finanças pessoais self-hosted. Documentação de arquitetura em `docs/` — **índice e resumo em [README.md](../README.md)**.

## Regras que não se negociam

- **Âmbito de formatos: CSV, OFX e CAMT.053.** **PDF está excluído** (ADR-016). Não propor extração de PDF, OCR, `poppler`, `pdfium`, `pdftotext`, `pdftoppm`, editor visual de colunas nem `layout_json`. Instituições que só emitem PDF usam o modo «apenas o total». O estudo está arquivado em `docs/99-fora-de-ambito-pdf.md` — é histórico, não é plano.
- **Stack: Go puro.** Sem Node, sem `npm`, sem `package.json`, sem *bundler*, sem SPA. UI = `templ` + HTMX + Alpine.js (apenas estado local) + Tailwind com binário *standalone*.
- **Sem regras de negócio em JavaScript.**
- **Base de dados: SQLite** via `modernc.org/sqlite` (Go puro). `CGO_ENABLED=0` sempre. **Nunca introduzir dependências cgo, ORMs ou binários externos no *runtime*.**
- **Sem IA em *runtime*.** Nada de `transformers.js`, ONNX ou LLM a categorizar. A escada de evolução permitida está no ADR-006.
- **Dinheiro é `int64` em cêntimos.** Nunca `float64` em contexto monetário — nem numa expressão intermédia. Datas de transação são `TEXT 'YYYY-MM-DD'` civis, não `time.Time` com fuso.
- **Toda a escrita passa pelo *mutator*** (transação + regras + `audit_log` + `revision`). Não escrever diretamente nas tabelas de negócio.

## Domínio que costuma ser ignorado

- **Um cartão de crédito é uma conta** (`type='credit'`); compras são negativas e o saldo é passividade.
- **O pagamento da fatura é uma transferência**, emparelhada pelo **total declarado** (valor exato, não heurística). Sem isto há dupla contagem — ADR-014.
- **Reconciliação declarada**: usar saldo, totais e contagens como invariantes. Divergência nunca é só «não fecha» — tem de explicar a causa.
- **Um lote pode ter contas-alvo distintas por linha** (`import_rows.target_account_id`).

## Documentação

- ADRs em `docs/05-decisoes-adr.md`, formato **Contexto → Decisão → Alternativas avaliadas → Consequências**, em português, com tabelas comparativas e diagramas Mermaid. Revisões são assinaladas no próprio ADR e no registo do topo.
- Alteração estrutural implica **propor um ADR**, não alterar silenciosamente um existente.
- **Ler apenas a secção necessária** e citar por ficheiro + secção em vez de reproduzir conteúdo.

## Código

- Estrutura fixa: `cmd/app`, `internal/{http,views,modules,domain,adapters,data,infra}`, `profiles/`, `migrations/`, `web/static`, `testdata`. Não inventar diretórios.
- Regra de dependência: `http → views → modules → domain`; `modules → data → infra`. **`internal/domain` não importa `data`, `infra`, `http` nem `adapters`.**
- SQL vive exclusivamente em `internal/data`.
- Comentários e mensagens de erro em português; identificadores em inglês.

## Testes

- `go test ./...`. Nunca reescrever *fixtures* do corpus em `testdata/` para fazer um teste passar — a *fixture* é a verdade.
- Testes *golden* comparam linha a linha; alterações de comportamento aparecem como *diff* revisto.
- Cada novo formato, banco ou correção de normalização exige *fixture* anonimizada. Perfil sem *fixture* não entra.

## Economia de contexto (ADR-017)

- Este ficheiro é lido em **todos** os pedidos: não acrescentar aqui conteúdo que já exista em `docs/`.
- Novas instruções usam `applyTo` específico. **`applyTo: "**"` é proibido.**
- Citar por ficheiro e secção em vez de reproduzir. Ler apenas a secção necessária. Não colar ficheiros grandes no chat.
- Tarefas repetitivas têm *prompt* próprio (ex.: `/novo-perfil-banco`); perfis e corpus têm agente restrito em `.github/agents/`.
- Orçamentos, inventário e antipadrões em `docs/08-otimizacao-de-contexto.md`.

## Respostas

- Responder em **português**.
- Ser conciso e direto. Não repetir conteúdo já presente nos documentos — referenciar.
- Ao propor alternativas, dar o porquê de cada uma ser rejeitada, como fazem os ADRs.
- Se uma pedido contrariar um ADR, dizê-lo explicitamente em vez de o cumprir em silêncio.
