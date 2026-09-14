# 08 — Otimização de contexto para agentes de IA

Requisito de projeto, registado em [ADR-017](05-decisoes-adr.md#adr-017--otimização-de-contexto-para-agentes-de-ia). Este é o documento que **se mantém** — sempre que se acrescenta, renomeia ou remove um ficheiro de customização, atualiza-se a secção 2.

---

## 1. Porque é uma decisão e não uma preferência

O custo dominante de trabalhar com assistentes de IA neste repositório **não é o tamanho do projeto**: é o contexto que tem de ser redescoberto em cada pedido.

Sem regras escritas, cada pedido paga repetidamente:

| Custo oculto | Exemplo real neste projeto |
| --- | --- |
| Redescobrir decisões estruturais | O agente volta a propor PDF, Node, ORM ou SPA — decisões já tomadas e justificadas (ADR-012, ADR-016) |
| Reler documentos inteiros | Pedir algo sobre um ADR obriga a ler 50 KB se não houver índice navegável |
| Responder a perguntas que um ADR já responde | «Porque é que não usamos GraphQL?» está respondido no ADR-004 |
| Explorar a estrutura | Um agente que não sabe onde vive o SQL procura em todo o repositório |

Nenhum destes aparece numa fatura. Todos se pagam diariamente.

---

## 2. Inventário do que existe no repositório

**Esta tabela é a parte que se mantém.** Ao criar ou remover um ficheiro de customização, atualizar aqui.

| Ficheiro | Tipo | Quando carrega | Custo por pedido |
| --- | --- | --- | --- |
| `.github/copilot-instructions.md` | Instruções de projeto | **Sempre** | Fixo — por isso tem de ser curto |
| `AGENTS.md` | Instruções de projeto (portátil) | Sob demanda de outros agentes | Baixo |
| `.github/instructions/docs.instructions.md` | Instruções de ficheiro | Só ao editar `docs/**/*.md` | Zero fora desse âmbito |
| `.github/prompts/novo-perfil-banco.prompt.md` | *Prompt* | Invocado por `/novo-perfil-banco` | Zero até ser invocado |
| `.github/agents/corpus-perfis.agent.md` | Agente restrito | Ao delegar, ou escolhido à mão | Zero até ser usado |
| `.github/hooks/hooks.json` + `format-go.sh` + `verify-go.sh` | *Hooks* | Em eventos do ciclo de vida | Nenhum (correm fora do contexto do modelo) |

### Orçamento de contexto

| Ficheiro | Limite | Motivo |
| --- | --- | --- |
| `.github/copilot-instructions.md` | **≤ 60 linhas** | É pago em todos os pedidos, para sempre |
| `AGENTS.md` | ≤ 60 linhas | É um índice, não um manual |
| Instruções com `applyTo` | ≤ 100 linhas | Só pagas quando relevante |
| Agente restrito | ≤ 80 linhas | Deve caber na cabeça do modelo sem diluir o foco |

Se um destes ficheiros crescer para além do limite, é sinal de que conteúdo de **domínio** está no sítio errado: deve ir para `docs/` e ser citado por referência.

---

## 3. As três alavancas

### 3.1 Contexto permanente — só o que evita re-explicação

Deve conter **apenas** o que é: (a) não negociável, (b) violado com frequência, (c) caro de descobrir. Nesta ordem:

1. Âmbito de formatos (CSV/OFX/CAMT.053; **PDF excluído**) — a violação mais frequente.
2. Stack (Go puro; sem Node, npm, SPA, `package.json`).
3. Invariantes de domínio (dinheiro em `int64`, datas civis, cartão como conta).
4. Estrutura de diretórios e regra de dependência.
5. Testes (o corpus é a verdade; nunca reescrever *fixtures*).

**Não deve conter**: descrições de ficheiros, listas de endpoints, esquema de base de dados, exemplos de código. Tudo isso existe em `docs/` e é carregado quando interessa.

### 3.2 Contexto condicional — a maior economia

O princípio é simples: **um ficheiro de contexto só deve custar quando é relevante.**

| Mecanismo | Como |
| --- | --- |
| `applyTo` específico | `docs/**/*.md`, `**/*.go` — **nunca `applyTo: "**"`** |
| `description` com gatilhos | O agente decide carregar com base na descrição; sem palavras-gatilho, o ficheiro é invisível |
| *Prompts* por tarefa | Empacotam um processo repetitivo em vez de o re-explicar |
| *Skills* | Conhecimento carregado só quando o agente decide que é relevante |
| Agentes com ferramentas mínimas | Menos ferramentas = menos exploração = menos tokens |

### 3.3 Higiene de sessão — o custo que não se vê

| Prática | Porquê |
| --- | --- |
| **Uma sessão por tarefa** | O histórico acumulado é reenviado em cada mensagem; o custo cresce de forma quadrática |
| Estado em ficheiros, não no chat | `docs/`, `/memories/`, notas de sessão são lidos quando interessam |
| **Referenciar, não colar** | «Ver `docs/04` §5.4» em vez de colar a secção |
| Pedir a secção, não o documento | Um ADR, não o ficheiro dos ADRs |
| Buscas com padrão específico | `includePattern` e regex alternada em vez de explorar diretórios |
| Nunca «explica-me o projeto todo» | O `README.md` responde a isso com uma fração do custo |

---

## 4. Estrutura do repositório como fator de tokens

Regras que se aplicam à escrita dos documentos, não à configuração dos agentes:

1. **Índice navegável no topo de cada documento grande.** Em `05-decisoes-adr.md`, a tabela com uma linha por ADR permite decidir *sem abrir* o ficheiro. Foi a otimização de maior retorno até hoje.
2. **Documentos por assunto, não por tamanho.** `07-muitas-contas-e-cartoes.md` existe porque a logística de contas é um assunto independente do pipeline.
3. **Arquivo fora do âmbito numerado `99-`.** `99-fora-de-ambito-pdf.md` fica visivelmente fora do plano, para que nem humanos nem agentes o tratem como especificação ativa.
4. **Estrutura de código previsível.** Com `internal/{domain,adapters,data,modules}`, o agente sabe onde procurar sem explorar. Diretórios arbitrários custam tokens em cada busca.
5. **Diagnósticos com código estável.** `CARD_PAYMENT_UNMATCHED` resolve uma dúvida sem diálogo; uma mensagem vaga obriga a ida e volta.

*Alternativa avaliada e adiada:* dividir `05-decisoes-adr.md` num ficheiro por ADR. Reduz o custo de leitura de um ADR isolado, mas fragmenta 17 ficheiros e obriga a um índice redundante. Reavaliar se o documento passar de ~60 KB.

---

## 5. Modelo e ferramenta por tarefa

| Tarefa | Escolha | Motivo |
| --- | --- | --- |
| Perfil YAML, *fixture*, adaptar teste *golden*, formatação | **Modelo rápido/pequeno**, agente restrito | Trabalho mecânico, verificado por `go test` |
| Bug no motor de importação, refactor | Modelo intermédio | Precisa de raciocínio, mas o âmbito é claro |
| ADR, modelo de dados, decisão estrutural | **Modelo grande** | Decisão irreversível; aqui o custo do erro excede o do token |
| Verificação de formatação e compilação | ***Hooks***, não o modelo | Determinístico e verificável; não deve passar pelo modelo |

---

## 6. Manutenção

Estas regras evitam que a própria otimização apodreça:

1. **Revisão trimestral** de `.github/copilot-instructions.md` contra os ADRs vigentes. Contradição com um ADR revisto é o modo de falha mais provável — aconteceu já com o PDF (ADR-016).
2. **Regra de remoção**: uma instrução que não evitou pelo menos uma ida-e-volta por semana sai. Instrução que não muda comportamento é custo puro.
3. **Nunca duplicar `docs/` no contexto permanente.** Se uma regra precisa de mais de três linhas, pertence a um documento e é citada.
4. **Ficheiros de customização entram em *pull request* como código.** São configuração do projeto, não preferências pessoais.
5. **Ao acrescentar um documento a `docs/`, atualizar o índice do `README.md`** e, se for estrutura, a tabela da secção 2 deste documento.

### Pendentes conhecidos

Itens identificados mas ainda não executados. Manter esta lista honesta e curta.

| Item | Motivo de estar pendente |
| --- | --- |
| Índice navegável no topo de `docs/03`, `docs/04` e `docs/07` | Têm 300–530 linhas e não começam por índice. Acrescentar quando forem editados, não em retrocompatibilidade forçada. `docs/05` já tem |
| Divisão de `docs/05-decisoes-adr.md` por ADR | ~50 KB e a crescer. Reavaliar aos ~60 KB, ou quando um pedido sobre um único ADR obrigar a leitura completa |

---

## 7. Antipadrões

| Antipadrão | Porquê é mau |
| --- | --- |
| `applyTo: "**"` numa instrução | Paga-se em **todos** os pedidos, mesmo nos irrelevantes |
| Despejar o conteúdo dos ADRs no contexto permanente | Duplicação paga diariamente; o ADR é a fonte, não o resumo |
| Instruções que descrevem o que o código já diz | Custo sem contrapartida; envelhece mal |
| Colar ficheiros grandes no chat «para o agente ver» | Pagamento por igual do que se pode referenciar |
| Manter uma sessão aberta o dia inteiro | O histórico cresce e é reenviado; o custo cresce de forma quadrática |
| Confiar em instruções para formatação e testes | Não é determinístico. O que tem de ser garantido vai para *hooks* |
| Deixar *hooks* lentos no caminho crítico | `go test ./...` em cada edição bloqueia o trabalho sem acrescentar informação |

---

## 8. Relação com as decisões de arquitetura

Várias decisões do projeto têm **retorno direto em tokens**, o que não era o seu motivo original:

| Decisão | Efeito na economia de contexto |
| --- | --- |
| ADR-016 — PDF fora de âmbito | Remove uma área inteira de contexto, dependências e diagnósticos |
| ADR-012 — UI renderizada no servidor | Elimina `node_modules`, *bundler* e cliente de API do contexto do agente |
| ADR-006 — regras como dados | Um perfil novo é editar YAML, não raciocinar sobre código |
| ADR-008 — *staging* com pré-visualização | O *diff* explica-se sozinho; menos perguntas ao agente |
| Testes *golden* como QA | O *diff* do teste é a resposta — não é preciso o modelo raciocinar sobre o código |
