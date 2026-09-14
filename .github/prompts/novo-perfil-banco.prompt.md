---
description: "Gera um perfil de importação e a fixture de teste para uma instituição (banco ou cartão), CSV ou OFX."
mode: agent
---

# Novo perfil de instituição

Preciso de suporte para a instituição **${input:instituicao}**, com formato **${input:formato:csv}**.

## O que fazer

1. **Confirmar o âmbito.** Formatos aceites: `csv`, `ofx`, `camt053`. Se for PDF, **parar** — está fora de âmbito (ADR-016) e a resposta é o modo «apenas o total». Dizê-lo ao utilizador em vez de avançar.

2. **Ler o contrato do perfil** em [`docs/03-modelo-de-dados.md`](../docs/03-modelo-de-dados.md) (tabela `import_profiles`) e a secção de identificação de perfil em [`docs/04-motor-importacao-csv.md`](../docs/04-motor-importacao-csv.md). Não inventar campos.

3. **Criar o ficheiro** em `profiles/${input:slug}.yaml` com:
   - `institution_slug`, `source_format`, `signature` (hash das colunas normalizadas, ou `ACCTID`/`IBAN` em OFX/CAMT)
   - `mapping_json` — mapeamento de colunas/campos, usando os sinónimos em português do documento 04
   - `parse_options_json` — delimitador, formato de data, separador decimal, linhas a saltar, codificação
   - `ignore_rules_json` — linhas-resumo a descartar (`SALDO ANTERIOR`, `RESUMO`, bloco de parceladas)
   - `payee_rules_json` — limpeza de ruído bancário (`PIX`, `TED`, `PARCELA n/m`, CNPJ, CPF, datas embutidas)
   - `declared_totals_json` — expressões para os números que a fonte declara (saldo, totais, contagem)
   - `card_rules_json` / `section_rules_json` quando for cartão de crédito

4. **Criar a *fixture* anonimizada** em `testdata/fixtures/${input:slug}/${input:formato}/` com o ficheiro de entrada e o JSON *golden* esperado. **Anonimizar dados pessoais; preservar estrutura, formatos de data/valor e ruído bancário.**

5. **Verificar coerência:** a soma das linhas tem de fechar com o total declarado. Se não fechar, o problema está no perfil, não na fonte — indicar a causa provável (linha-resumo incluída? bloco de parceladas? sinal invertido?).

6. **Atualizar** a tabela de conhecimento em `profiles/institutions.yaml`, se existir.

## Regras

- **Não alterar o motor.** Se o perfil não conseguir expressar algo, esse é um achado importante: dizê-lo, em vez de escrever código pontual.
- **Perfil sem *fixture* não entra.**
- Se o banco só exportar PDF, não criar perfil nenhum: explicar o modo degradado («apenas o total», [doc 07 §8](../docs/07-muitas-contas-e-cartoes.md)) e terminar.
