# 99 — Fora de âmbito: extração de PDF

> **Estado: ARQUIVADO em 2026-09-14.** Este documento não descreve o sistema em construção. Existe para que a decisão [ADR-016](05-decisoes-adr.md#adr-016--pdf-fora-de-âmbito) fique fundamentada e para que reabrir o tema seja barato, se algum dia for necessário.

---

## 1. A decisão

**PDF está fora de âmbito.** As fontes suportadas são **CSV, OFX e CAMT.053**.

Para instituições que não exportam nenhum destes formatos, a resposta do sistema é o **modo «apenas o total»**: uma transação com o valor total declarado do documento (ver [07 §8](07-muitas-contas-e-cartoes.md#8-modo-degradado-apenas-o-total)).

## 2. Porque foi arquivado

| Razão | Detalhe |
| --- | --- |
| **Custo de construção desproporcionado** | Extração posicional + reconstrução de tabela por geometria + editor visual de colunas + perfil de layout por instituição + corpus próprio de *fixtures*. É, de longe, a maior peça do sistema |
| **Custo de manutenção permanente** | Bancos mudam o layout dos PDFs sem aviso. Cada mudança é uma interrupção que exige interpretação humana no editor de colunas |
| **É o elo mais frágil** | Extração de tabelas a partir de geometria falha em silêncio quando a tolerância de agrupamento em `y` está mal calibrada. Foi por isso que a reconciliação por totais declarados seria obrigatória |
| **Existem alternativas melhores** | OFX/CAMT resolvem o mesmo problema com identificadores estáveis (`FITID`, `ACCTID`). Um agregador (Pluggy, Belvo) resolve-o com custo mensal |
| **Amplitude de uso duvidosa** | A maioria dos bancos brasileiros oferece CSV ou OFX, por vezes escondido. O esforço de descobrir a exportação é ordens de magnitude menor do que construir o extrator |
| **Foco** | O objetivo é reduzir o trabalho mensal com muitas contas e cartões. **Roteamento, biblioteca de perfis, painel de cobertura e *fatura matcher* entregam mais, mais depressa e com muito menos risco** |

## 3. O que se perde (consequência honesta)

- Numa instituição *sem* CSV/OFX, os lançamentos individuais **não entram no sistema**. O total entra como uma linha. O orçamento por categoria fica incompleto nessa conta, ainda que os saldos e a dívida do cartão fiquem corretos.
- Se várias contas caírem neste caso, o valor da categorização automática cai proporcionalmente. É a única regressão real face ao plano anterior, e é uma regressão no *detalhe*, não na *correção*.
- Perde-se o painel de cobertura completo para essas contas: sabe-se que o total foi lançado, não que os lançamentos estão todos lá.

## 4. O que se ganha

- **Remoção de peças inteiras do sistema**: adapter PDF, extração posicional, editor visual de colunas, perfil de layout, dependência de `poppler-utils` e `tesseract`, imagens de página, tabelas `layout_json`, diagnósticos `PDF_*`.
- **A imagem Docker volta a poder ser `distroless/static`** (~10 MB), sem binários externos — não há `poppler` nem `tesseract` a instalar nem a versionar.
- Um eixo de fragilidade a menos: não há extração de tabelas por geometria, logo não há erro silencioso dessa origem.
- Menos superfície de teste: o corpus são ficheiros estruturados, comparáveis linha a linha sem tolerância.

## 5. Como reabrir (se algum dia fizer sentido)

Condição mínima para reabrir: **existir um documento concreto** — um extrato ou fatura de uma instituição específica, que não exporte nada estruturado e que seja relevante o suficiente para justificar o trabalho.

Ordem obrigatória, do mais barato para o mais caro:

1. Agregador Open Banking (Pluggy/Belvo) para essa instituição. Resolve e é *outsourcing*.
2. Extração posicional apenas para essa instituição, sem generalizar.
3. Generalização — só se ≥ 3 instituições o exigirem.

Análise técnica preservada abaixo, para que o passo 2 comece a partir daqui e não do zero.

---

## 6. Estudo técnico preservado

### 6.1 Três níveis de dificuldade

É obrigatório distinguir, porque a estratégia é completamente diferente.

| Nível | Descrição | Estratégia | Confiança típica |
| --- | --- | --- | --- |
| **Tier A** | PDF gerado por sistema, com camada de texto («PDF nativo») | Extração posicional + reconstrução de tabela + perfil de layout | 95–99% |
| **Tier B** | PDF digitalizado (imagem dentro do PDF), sem texto | OCR (`tesseract`) + reconciliação obrigatória | 70–90% |
| **Tier C** | Fotografia, inclinação, baixa resolução | Não vale a pena | — |

Deteta-se o nível em segundos: se a extração de texto devolver menos de ~20 caracteres por página, é Tier B. **A maioria dos extratos brasileiros é Tier A.**

### 6.2 Extração com coordenadas

**O erro a evitar:** `pdftotext` em modo linear (ou qualquer extração que devolva apenas texto) **destrói a tabela**. Junta colunas, perde a correspondência entre descrição e valor, e desalinha linhas. É inutilizável com colunas `Data | Histórico | Valor | Saldo`.

A extração tem de ser **posicional**: cada palavra com a sua caixa delimitadora, e a tabela reconstruída por geometria.

| Opção | Como | Veredicto |
| --- | --- | --- |
| **`pdftotext -bbox-layout`** (poppler-utils) | Subprocesso; devolve XHTML com `<word xMin yMin xMax yMax>` por palavra | **Preferida.** Pacote `poppler-utils` existe em `arm64`; o *parsing* é `encoding/xml` em Go; zero cgo |
| `go-pdfium` (pdfium via WASM/wazero) | Em processo, `FPDFText_GetCharBox` | Alternativa para evitar binário externo; verificar disponibilidade do `.wasm` para `arm64` sem cgo |
| `pdfplumber` / `camelot` (Python) | Sidecar | **Último recurso.** Reintroduz um segundo *runtime*, mais um contentor e o corpus dividido por duas linguagens |

Nota de licença e de abordagem: Tabula e camelot são **referência de UX e de algoritmo**, não código a reutilizar (mesma disciplina do ADR-011).

### 6.3 Algoritmo de reconstrução

```
1. Agrupar palavras por página.
2. Cortar bandas de cabeçalho/rodapé: linhas cujo texto se repete em >= 60%
   das páginas são ruído de paginação, não dados.
3. Detetar colunas: projetar todos os intervalos-x das palavras e encontrar
   os "vales" de espaço vazio consistentes ao longo das linhas.
4. Detetar linhas: agrupar palavras por y com tolerância (~2-3 pt).
5. Resolver continuações: linha sem data e sem valor na primeira/última
   coluna é continuação e é colada à linha anterior.
6. Repetir 3-5 por página, reutilizando as fronteiras de coluna da página 1.
7. Emitir RawRow{LineNo, PageNo, Cells[], BBox} - a mesma estrutura do adapter CSV.
```

O passo 7 é a decisão que torna isto suportável: **o PDF entraria no pipeline no mesmo ponto que o CSV**. Perfil, normalização, regras, dedupe, pré-visualização, *commit* e *undo* seriam código partilhado.

A tolerância de agrupamento em `y` (passo 4) é o parâmetro crítico: demasiado apertada parte linhas, demasiado larga funde transações distintas. Falha em silêncio nos dois casos — é a razão pela qual a reconciliação por totais declarados seria **obrigatória**, não opcional.

### 6.4 Perfil de layout

```jsonc
{
  "page": { "crop_top": 0.14, "crop_bottom": 0.92 },
  "columns": {
    "date":    { "x0": 0.06, "x1": 0.16 },
    "payee":   { "x0": 0.16, "x1": 0.62 },
    "amount":  { "x0": 0.62, "x1": 0.82 },
    "balance": { "x0": 0.82, "x1": 1.00 }
  },
  "date_format": "dd/MM/yyyy",
  "continuation": { "require_date": true, "indent_min": 0.16 },
  "section_rules": [
    { "match": "Cartao final (?P<last4>\\d{4})", "action": "set_target_account" }
  ],
  "ignore_lines": [ "^(SALDO ANTERIOR|SALDO DO DIA|SALDO FINAL|RESUMO|LIMITE)" ],
  "declared_totals": {
    "close":     "SALDO FINAL\\s+R?\\$?\\s*([\\d.,]+)",
    "open":      "SALDO ANTERIOR\\s+R?\\$?\\s*([\\d.,]+)",
    "purchases": "TOTAL DE COMPRAS\\s+R?\\$?\\s*([\\d.,]+)"
  }
}
```

Coordenadas em **frações da página** (0–1), não em pontos: o perfil resiste a mudanças de resolução.

### 6.5 Editor visual de colunas — a peça que faz isto funcionar na prática

Configurar o perfil em JSON à mão é inviável e transformaria o PDF numa fonte de segunda classe. A UI desenharia o perfil sobre a própria página, inspirada no Tabula:

1. O servidor renderiza a página com `pdftoppm -png -r 150` e envia a imagem.
2. O utilizador **desenha as fronteiras de coluna** a arrastar sobre a imagem (Alpine faz o overlay com `<div>` posicionados por percentagem).
3. Atribui um papel a cada faixa (`data`, `payee`, `amount`, `balance`) e confirma as linhas de total.
4. A pré-visualização ao lado mostra as linhas extraídas **em tempo real**, para as primeiras ~30 linhas — validação visual imediata, não um *deploy* de configuração.

Guardar = criar/atualizar o perfil. **Sem Node, sem `pdf.js`, sem *bundler***: imagem no servidor, overlay em Alpine.

### 6.6 Reconciliação por totais declarados

O que torna a extração de PDF assustadora é o erro silencioso. Mas **um extrato declara sempre os seus próprios totais**: saldo anterior, saldo final, total de compras, total da fatura, total a pagar.

Esses números são **invariantes gratuitos** fornecidos pela própria fonte. Usados como verificação, transformam «extração não confiável» em **«extração verificável»**:

```
verificação_de_saldo:    declared_open + Σ linhas == declared_close
verificação_de_compra:   Σ linhas de compra == declared_purchases
verificação_de_contagem: nº de linhas == declared_count
```

Quando não fecha, explicar a causa em vez de reportar a divergência:

| Hipótese | Busca | Mensagem |
| --- | --- | --- |
| Linha não extraída | Existe linha com valor `Δ`? | «A linha de 123,45 não foi reconhecida» |
| Linha duplicada | Duas linhas somam `Δ`? | «Duas linhas somam exatamente a diferença» |
| Linha-resumo indevida | Alguma linha ignorada tem valor `Δ`? | «A linha "SALDO ANTERIOR" foi incluída por engano» |
| Sinal invertido | `2 × Σ invertido == Δ`? | «Suspeita de sinal invertido» |
| Parcela errada | Linha com `Parcela n/m` fora de competência? | «Parcela futura incluída» |

A busca por subconjunto limitada a 1–2 elementos é `O(n)` e instantânea.

**Esta ideia foi preservada e generalizada** em [07 §9](07-muitas-contas-e-cartoes.md#9-reconciliação-declarada-não-só-saldo) — aplica-se a CSV e OFX, e é o único contributo do estudo de PDF que ficou no âmbito ativo.

### 6.7 Corpus de teste que seria necessário

| Tipo | Conteúdo | Verificação |
| --- | --- | --- |
| PDF Tier A | PDFs reais anonimizados por instituição | *Golden* linha a linha + reconciliação declarada |
| PDF adversarial | Página dividida a meio de uma linha, cabeçalho repetido, duas colunas, rodapé com bloco de parceladas, PDF sem camada de texto | Diagnóstico explícito — nunca resultado silenciosamente errado |

Invariante obrigatório: `Σ linhas == total declarado` para todo o corpus Tier A.

---

## 7. Registos de decisão relacionados

- [ADR-013](05-decisoes-adr.md#adr-013--ingestão-multi-fonte-com-adapters) — ingestão multi-fonte por adapters (revisado: sem PDF)
- [ADR-016](05-decisoes-adr.md#adr-016--pdf-fora-de-âmbito) — PDF fora de âmbito *(decisão que arquiva este documento)*
- [ADR-011](05-decisoes-adr.md#adr-011--não-fazer-fork-do-actual-budget) — não copiar código de terceiros; ideias e algoritmos como referência
