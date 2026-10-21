# Nib — Temporal Repository Intelligence Engine

## 1. Resumen

Nib nació como una herramienta TUI escrita en Go para trabajar con Git de forma más cómoda.

La oportunidad de convertirlo en un proyecto técnicamente excepcional consiste en cambiar su núcleo conceptual:

> Nib no debe ser una interfaz para Git.
> Nib debe ser un motor que aprende cómo evoluciona un repositorio a través de su historial.

El objetivo es transformar el historial Git en una base de conocimiento temporal capaz de responder preguntas que Git no responde directamente:

- ¿Qué archivos suelen cambiar juntos?
- ¿Qué símbolos suelen modificarse juntos?
- ¿Qué partes del repositorio son históricamente inestables?
- ¿Qué zona del código tiene mayor probabilidad de sufrir regresiones?
- Si modifico esta función, ¿qué otras partes debería revisar?
- ¿Qué tests suelen modificarse cuando cambia este código?
- ¿Este PR se comporta de manera anómala respecto al historial del repositorio?
- ¿Qué contexto histórico debería conocer un coding agent antes de modificar esta función?
- ¿Qué desarrolladores conocen históricamente mejor esta zona?
- ¿Qué cambios similares ocurrieron anteriormente?

La arquitectura final debería permitir tres productos construidos sobre el mismo motor:

```text
                     ┌─────────────────────┐
                     │      nib core       │
                     │                     │
                     │ Temporal Repository │
                     │ Intelligence Engine │
                     └──────────┬──────────┘
                                │
              ┌─────────────────┼──────────────────┐
              │                 │                  │
              ▼                 ▼                  ▼
         nib CLI/TUI       Agent Skill        GitHub App
              │                 │                  │
              ▼                 ▼                  ▼
         Developers        Coding agents       Pull Requests
```

La parte importante y técnicamente defendible es `nib core`.

La TUI, el GitHub bot y la integración con agentes son consumidores de ese motor.

---

# 2. Dónde estamos

Actualmente Nib tiene una orientación principalmente relacionada con productividad alrededor de Git.

Conceptualmente:

```text
User
  │
  ▼
Nib TUI
  │
  ▼
Git CLI
  │
  ▼
Repository
```

Nib facilita acciones existentes.

Eso tiene utilidad, pero limita considerablemente el techo técnico del proyecto.

Si toda operación compleja termina siendo:

```bash
git log
git diff
git status
git blame
```

seguido de procesamiento en Go, Nib continúa dependiendo de Git para prácticamente toda la inteligencia.

El objetivo debe ser evolucionar hacia:

```text
Git repository
      │
      ▼
History ingestion
      │
      ▼
Persistent Nib index
      │
      ▼
Temporal analysis engine
      │
      ▼
Repository intelligence
```

Git pasa a ser la fuente de datos.

Nib pasa a ser quien interpreta esos datos.

---

# 3. A dónde queremos llegar

La visión final es:

> Nib es un motor local de inteligencia temporal del código que utiliza la historia de un repositorio para entender relaciones, riesgo, impacto y patrones de evolución.

Una interacción final podría ser:

```bash
nib index
```

```text
Repository indexed

Commits       281,341
Files          48,281
Symbols       918,441
Relationships 3,821,991

Index updated in 1.84s
```

Después:

```bash
nib impact PaymentService.Process
```

```text
PaymentService.Process

Risk: HIGH

Historically coupled symbols

0.91  RetryPolicy.Apply
0.83  PaymentRepository.Save
0.72  CheckoutService.Complete

Likely affected tests

0.89  payment_retry_test.go
0.76  checkout_payment_test.go

Historical observations

- Changed 83 times
- 31 changes were associated with fixes
- 8 previous regressions touched this area
- 84% of changes also modified RetryPolicy.Apply
```

Y eventualmente:

```bash
nib pr-impact 1842
```

```text
PR #1842

Overall risk: HIGH

Changed symbols: 17

Potentially missing changes:

api/schema.go
  ↳ historically changes with client/generated.ts
  ↳ co-change confidence: 96%
  ↳ observed across 84 previous commits

payment/service.go
  ↳ historically changes with payment/service_test.go
  ↳ test file untouched in this PR

Suggested reviewers:

alice     relevance 0.91
bob       relevance 0.78
```

Eso es el producto.

---

# 4. Principio fundamental

Nib debe producir **evidencia**, no opiniones mágicas.

Evitar:

```text
AI thinks this PR is risky.
```

Preferir:

```text
This PR modifies PaymentService.Process.

Across 47 previous modifications:

- RetryPolicy.Apply changed alongside it 39 times
- payment_retry_test.go changed 35 times
- 18 changes were associated with subsequent fixes

Historical anomaly score: 0.87
```

Después un humano, Codex, Claude u otro sistema puede interpretar esos datos.

Esto permite que Nib siga siendo valioso incluso sin ningún LLM.

---

# 5. Arquitectura conceptual

El proyecto debería evolucionar hacia estas capas:

```text
┌──────────────────────────────────────────────┐
│                   Consumers                  │
│                                              │
│ CLI │ TUI │ Agent Skill │ GitHub App │ API   │
└───────────────────────────┬──────────────────┘
                            │
┌───────────────────────────▼──────────────────┐
│             Intelligence Engine              │
│                                              │
│ Risk │ Impact │ Coupling │ Ownership │ Tests │
│ Anomalies │ Historical Context │ Reviewers   │
└───────────────────────────┬──────────────────┘
                            │
┌───────────────────────────▼──────────────────┐
│                Temporal Graph                │
│                                              │
│ File ↔ File                                  │
│ Symbol ↔ Symbol                              │
│ Symbol ↔ Test                                │
│ Author ↔ Symbol                              │
│ Commit ↔ Symbol                              │
└───────────────────────────┬──────────────────┘
                            │
┌───────────────────────────▼──────────────────┐
│                  Nib Index                   │
│                                              │
│ Commits │ Files │ Changes │ Symbols │ Edges  │
└───────────────────────────┬──────────────────┘
                            │
┌───────────────────────────▼──────────────────┐
│                Git ingestion                 │
└───────────────────────────┬──────────────────┘
                            │
                         Repository
```

Cada nivel debe ser independiente.

La TUI nunca debería contener lógica de análisis.

El bot de GitHub nunca debería implementar cálculo de riesgo.

La skill para agentes nunca debería conocer cómo se indexa Git.

Todo debe terminar consumiendo `nib core`.

---

# 6. Fase 0 — Reestructurar el proyecto

Antes de añadir features nuevas, separar correctamente las responsabilidades.

Una posible estructura:

```text
internal/
    git/
        repository.go
        commits.go
        diff.go

    index/
        indexer.go
        incremental.go

    storage/
        commits.go
        files.go
        changes.go
        symbols.go
        edges.go

    history/
        churn.go
        fixes.go
        ownership.go

    coupling/
        files.go
        symbols.go

    symbols/
        parser.go
        resolver.go

    graph/
        graph.go
        traversal.go

    intelligence/
        risk.go
        impact.go
        context.go
        anomalies.go

    tui/
        ...

cmd/
    index.go
    risk.go
    coupled.go
    history.go
    impact.go
    context.go
```

No hace falta que estos sean exactamente los nombres finales.

La regla arquitectónica importante es:

> UI y CLI llaman al engine.
> El engine nunca depende de UI.

---

# 7. Fase 1 — Construir el índice histórico

Este es el primer milestone real.

Implementar:

```bash
nib index
```

Nib debe recorrer el repositorio y almacenar como mínimo:

```text
Commit
    hash
    parent hashes
    author
    author email
    timestamp
    message

FileChange
    commit
    file
    previous path
    change type
    lines added
    lines deleted
```

Todavía no necesitamos símbolos.

## Primera decisión de diseño importante

Hay que decidir qué representa internamente un archivo.

Por ejemplo:

```go
type FileID uint32
type CommitID uint32
```

en lugar de repetir strings continuamente.

Una estructura aproximada podría ser:

```go
type Commit struct {
    ID        CommitID
    Hash      [20]byte
    Timestamp int64
    AuthorID  AuthorID
    ParentIDs []CommitID
}

type FileChange struct {
    CommitID CommitID
    FileID   FileID
    Added    uint32
    Deleted  uint32
    Kind     ChangeKind
}
```

El objetivo es empezar a pensar en volumen.

Un repositorio grande puede contener:

```text
1M commits
100k files
10M file changes
```

No quieres representar todo mediante mapas gigantes de strings si esperas que Nib sea rápido.

---

# 8. Persistencia

En la primera implementación seria puedes utilizar una base existente.

Opciones razonables:

```text
SQLite
Badger
Pebble
BoltDB
```

No hay ningún premio por escribir inmediatamente un storage engine.

Lo importante es que **el modelo de almacenamiento sea diseño tuyo**.

Más adelante, si Nib madura, partes particularmente críticas pueden migrarse a:

```text
append-only storage
mmap
custom indexes
roaring bitmaps
compressed posting lists
```

Pero eso debería ocurrir porque los benchmarks lo justifiquen.

No por postureo técnico.

---

# 9. Indexación incremental

Después del primer `nib index`:

```text
repository HEAD: A
nib indexed HEAD: A
```

Si posteriormente aparecen:

```text
A → B → C → D
```

Nib no debe reconstruir todo.

Debe procesar solamente:

```text
B
C
D
```

Por tanto `.nib` necesita almacenar algo equivalente a:

```text
last indexed commit
repository identity
index version
schema version
```

Debes pensar también qué ocurre con:

```text
rebase
force push
branch deletion
history rewrite
```

Inicialmente puedes soportar solamente el camino feliz y detectar los demás casos.

Por ejemplo:

```text
History rewrite detected.

Expected ancestor:
81af34

Current HEAD does not descend from indexed HEAD.

Run:
nib index --rebuild
```

Mucho mejor que intentar resolver mágicamente todos los casos desde el principio.

---

# 10. Fase 2 — Inteligencia a nivel archivo

Antes de introducir parsers y ASTs, Nib debe ser útil solamente analizando archivos.

Implementar estas métricas.

## Churn

Cantidad de cambios sobre un archivo.

Puede combinar:

```text
commits touching file
lines added
lines removed
frequency
recent frequency
```

## Change frequency

Número de commits en ventanas temporales:

```text
7 days
30 days
90 days
all time
```

## Fix ratio

Proporción de cambios relacionados con correcciones.

Primera implementación:

```text
fix:
hotfix:
bug:
revert:
```

Si el proyecto utiliza Conventional Commits, mucho mejor.

Más adelante se puede mejorar.

## Ownership

Quién modifica cada área.

No quedarse únicamente en:

```text
alice: 47 commits
bob: 32 commits
```

Calcular también distribución.

Ejemplo:

```text
File A

Alice 90%
Bob   10%

ownership concentrated
```

frente a:

```text
File B

Alice 20%
Bob   18%
John  17%
Mary  15%
...

ownership fragmented
```

Puedes utilizar una métrica como Shannon entropy.

---

# 11. Temporal Coupling

Ésta debe ser una de las primeras features distintivas.

Definición básica:

Si A cambia en `N(A)` commits y A y B cambian juntos en `N(A,B)`:

```text
coupling(A → B) = N(A,B) / N(A)
```

Esto es direccional.

Puede ocurrir:

```text
A → B = 0.90
B → A = 0.40
```

porque B cambia muchísimo más a menudo.

Esto es importante.

La primera CLI:

```bash
nib coupled internal/payment/service.go
```

Resultado:

```text
Temporal coupling

internal/payment/service.go

0.87  internal/payment/retry.go
0.74  internal/checkout/payment.go
0.63  internal/payment/service_test.go
```

## Pregunta importante de diseño

¿Qué significa "cambiar juntos"?

Posibles definiciones:

```text
same commit
same PR
same branch
within N commits
within N hours
```

Para V1:

> Same commit.

Después podrá ampliarse.

---

# 12. Hacerlo rápido

No quieres calcular coupling recorriendo todos los commits en cada query.

Debes construir índices.

Conceptualmente:

```text
file → bitmap(commits)
```

Por ejemplo:

```text
service.go
001001101001010101...

retry.go
001001101000010101...
```

Entonces el número de commits compartidos es conceptualmente:

```text
AND(bitmap(service), bitmap(retry))
```

Esto abre la puerta a usar:

```text
Roaring Bitmaps
```

y ejecutar intersecciones extremadamente rápidas.

Éste es el tipo de decisión técnica que empieza a dar profundidad al proyecto.

---

# 13. Primera demo fuerte

Después de esta fase Nib debería poder hacer:

```bash
nib index
nib risk
nib coupled path/to/file.go
nib history path/to/file.go
```

Y todas las queries deberían responder rápidamente.

Éste es el primer punto donde merece la pena empezar a benchmarkear.

---

# 14. Fase 3 — Risk Engine

No hacer machine learning todavía.

Crear un scoring explicable.

Por ejemplo:

```text
risk =
    W1 * normalized_churn
  + W2 * normalized_fix_ratio
  + W3 * ownership_entropy
  + W4 * coupling_degree
  + W5 * recent_activity
```

No importa que inicialmente los pesos sean heurísticos.

Sí importa documentarlos.

Ejemplo:

```bash
nib risk payment/service.go
```

```text
Risk: 87/100

Factors

28/30  very high churn
21/25  high fix ratio
14/15  fragmented ownership
15/20  strong temporal coupling
 9/10  high recent activity
```

Nunca esconder el origen del score.

---

# 15. Validar si el riesgo significa algo

Esto es importantísimo.

No asumir que:

```text
high churn = dangerous
```

Hay que intentar comprobarlo históricamente.

Por ejemplo:

1. calcular el risk score usando información disponible hasta una fecha;
2. observar qué archivos recibieron fixes después;
3. comparar;
4. medir si archivos con score alto efectivamente reciben más correcciones.

Eso convierte una heurística en una hipótesis verificable.

---

# 16. Fase 4 — Symbol Intelligence

Aquí empieza el salto técnico importante.

Nib deja de entender solamente archivos.

Empieza a entender:

```text
functions
methods
types
interfaces
classes
structs
```

Usar Tree-sitter es probablemente una buena opción.

Pero inicialmente soportar solamente:

> Go.

No intentar:

```text
Go
Rust
Python
TypeScript
Java
C++
C#
Ruby
```

desde el principio.

Un lenguaje excelente es mejor que diez lenguajes mediocres.

---

# 17. Identidad de símbolos

Ésta es una de las decisiones difíciles.

Algo como:

```text
package + receiver + function
```

podría producir:

```text
github.com/foo/payment.PaymentService.Process
```

Pero aparecen problemas:

```text
renames
moves
signature changes
package moves
methods becoming functions
```

Para V1 puedes aceptar que un rename crea un símbolo nuevo.

Después puedes investigar detección de continuidad.

Lo importante es documentar esta limitación.

---

# 18. Detectar símbolos cambiados

Para cada commit:

```text
parent tree
    ↓
current tree
```

Parsear versiones relevantes y determinar:

```text
symbols added
symbols modified
symbols removed
```

Eso permite almacenar:

```text
SymbolChange

commit_id
symbol_id
change_type
```

A partir de ahí tienes:

```text
symbol → commits
commit → symbols
```

y todo lo construido para archivos puede repetirse para símbolos.

---

# 19. Symbol Temporal Coupling

Ahora:

```bash
nib coupled PaymentService.Process
```

puede producir:

```text
0.91 RetryPolicy.Apply
0.83 PaymentRepository.Save
0.71 CheckoutService.Complete
```

Esto es muchísimo más interesante que file coupling.

Porque empiezas a descubrir relaciones arquitectónicas implícitas.

---

# 20. Temporal Graph

Conceptualmente Nib empieza a tener un grafo:

```text
PaymentService.Process
       │
       ├── historical coupling ── RetryPolicy.Apply
       │
       ├── historical coupling ── PaymentRepository.Save
       │
       ├── modified by ─────────── Alice
       │
       ├── modified in ─────────── Commit 81af2
       │
       └── historically with ───── payment_retry_test.go
```

No necesitas obligatoriamente una graph database.

Probablemente no la usaría.

Puedes representar relaciones con índices propios.

El concepto importa más que la tecnología.

---

# 21. Fase 5 — Impact Analysis

Ésta puede convertirse en la feature más atractiva.

```bash
nib impact PaymentService.Process
```

Debe combinar múltiples señales.

Por ejemplo:

```text
temporal coupling
dependency relationships
shared commit history
test coupling
recent regressions
ownership
```

Salida:

```text
Potential blast radius

HIGH

RetryPolicy.Apply
  historical coupling: 0.91
  structural dependency: yes

PaymentRepository.Save
  historical coupling: 0.83

MEDIUM

CheckoutService.Complete
  historical coupling: 0.64

payment_retry_test.go
  changed alongside this symbol in 81% of commits
```

La palabra fundamental es:

> Potential.

No afirmar causalidad que no puedes demostrar.

---

# 22. Tests como entidades especiales

Detectar qué archivos parecen tests.

Para Go inicialmente:

```text
*_test.go
```

Entonces construir:

```text
symbol → historically associated tests
```

Ejemplo:

```text
PaymentService.Process

payment_service_test.go    0.91
payment_retry_test.go      0.78
checkout_test.go           0.54
```

Esto puede ser extremadamente útil para agentes.

---

# 23. Fase 6 — `nib context`

Ésta debería ser la interfaz principal para coding agents.

```bash
nib context PaymentService.Process
```

Salida humana:

```text
Historical Context

PaymentService.Process

Changed
  83 times

Fix-associated changes
  31

Strong historical relationships
  RetryPolicy.Apply          0.91
  PaymentRepository.Save     0.83

Relevant tests
  payment_service_test.go    0.89
  payment_retry_test.go      0.76

Previous relevant changes
  81af2  fix: prevent duplicate payment
  a74dc  feat: exponential retry
  119ba  fix: payment timeout regression

Warnings
  High historical fix ratio.
  Strong dependency on retry subsystem.
```

Pero además:

```bash
nib context PaymentService.Process --json
```

JSON estable y pensado para herramientas.

Eso es crítico.

---

# 24. Nib como skill para coding agents

La integración con agentes debería ser extremadamente pequeña.

El agente aprende una regla:

> Antes de modificar una zona importante, consulta Nib.

Ejemplo conceptual:

```text
User:
Fix duplicate payment retries.

Agent:
1. Locate PaymentService.Process
2. Run nib context PaymentService.Process --json
3. Inspect historically coupled symbols
4. Inspect suggested tests
5. Modify implementation
6. Run nib impact --working-tree
```

Aquí Nib tiene una ventaja importante:

El agente puede leer el repositorio actual.

Pero el agente normalmente no conoce bien:

```text
5 años de historia
patrones de regresión
ownership histórico
archivos que siempre cambian juntos
cambios similares antiguos
```

Nib proporciona esa memoria.

---

# 25. Nib no debe meter un LLM dentro todavía

Evitar:

```bash
nib ask "what do you think?"
```

durante las primeras etapas.

La arquitectura correcta:

```text
Nib
   produces evidence
        │
        ▼
Codex / Claude / human
   performs reasoning
```

No:

```text
Nib
   calls OpenAI
   returns vague answer
```

La inteligencia diferencial está en los datos.

---

# 26. Fase 7 — Working Tree Impact

Antes de llegar a GitHub, implementar:

```bash
nib impact --working-tree
```

Nib analiza cambios locales.

Ejemplo:

```text
Current changes

Modified:
  PaymentService.Process
  RetryPolicy.Apply

Potential missing changes:

payment_retry_test.go

Reason:
  touched alongside these symbols in 84% of historical commits
```

Esto permite probar la lógica antes de introducir GitHub.

---

# 27. Fase 8 — PR Impact

Generalizar:

```bash
nib impact --base main --head feature/foo
```

Eso es prácticamente un PR.

Calcular:

```text
changed files
changed symbols
historical relationships
missing usual companions
tests historically affected
owners
risk
```

Resultado:

```text
PR Impact

Risk: HIGH

17 changed symbols
4 high-risk symbols

Potential anomalies

PaymentService.Process modified
RetryPolicy.Apply untouched

Historical co-change:
84%

Observed:
63 / 75 relevant commits

Potentially missing test:
payment_retry_test.go

Historical test relationship:
79%
```

---

# 28. Fase 9 — Anomaly Detection

Esta puede convertirse en una de las killer features.

Nib aprende:

```text
A normalmente cambia con B
```

Pero observa un PR:

```text
A changed
B unchanged
```

Eso genera una anomalía.

Formalmente:

```text
P(B changes | A changes) = 0.96
```

Pero en este PR:

```text
A = changed
B = unchanged
```

Entonces:

```text
anomaly confidence = high
```

Ejemplo extremadamente claro:

```text
api/schema.go
        ↓
client/generated.ts
```

Históricamente:

```text
96% co-change
84 observations
```

PR actual:

```text
schema changed
generated client unchanged
```

Nib:

```text
Possible missing generated client update.
```

Esto puede tener valor real.

---

# 29. Validar anomaly detection científicamente

Éste puede ser uno de los experimentos más importantes del proyecto.

Tomar PRs o commits históricos.

Para cada uno:

```text
real changed set = {A, B, C, D}
```

Ocultar alguno:

```text
observed = {A, B, C}
hidden = D
```

Preguntar:

```text
Given A,B,C, what else would Nib expect?
```

Y comprobar si Nib predice D.

Esto permite medir:

```text
precision
recall
top-k accuracy
false positive rate
```

Entonces puedes poner en el README:

```text
Historical change prediction

Dataset:
42 repositories
183k commits

Top-3 recall: 72%
Precision: 64%
```

Estos números son ilustrativos.

Los números reales deben salir de tus experimentos.

Esto sería increíblemente más fuerte que decir simplemente:

> Nib uses AI to detect risky changes.

---

# 30. Fase 10 — GitHub App

Solo ahora construir el bot.

El bot recibe:

```text
PR opened
PR synchronized
PR reopened
```

Ejecuta Nib sobre:

```text
base SHA
head SHA
```

y publica:

```text
Nib Impact Report
```

La aplicación debe ser muy silenciosa.

No comentar veinte cosas.

Un único resumen.

---

# 31. Diseño del comentario de PR

Ejemplo:

```text
## Nib Impact

Risk: HIGH

### Strong historical relationships

PaymentService.Process
→ RetryPolicy.Apply — 84%
→ payment_retry_test.go — 79%

### Potential anomalies

⚠ PaymentService.Process changed without RetryPolicy.Apply

Observed together in:
63 / 75 historical changes

### Suggested tests

payment_service_test.go
payment_retry_test.go

### Suggested reviewers

alice — historical relevance 0.91
bob — historical relevance 0.78
```

Todo dato debe poder justificarse.

---

# 32. Reviewer Recommendation

No usar simplemente:

```text
git blame
```

Puedes calcular expertise mediante:

```text
recent modifications
historical modifications
symbol ownership
related symbol ownership
review history, eventually
```

Algo conceptual:

```text
reviewer_score =
    direct_symbol_experience
  + related_symbol_experience
  + recency
  + subsystem_experience
```

No necesitas hacerlo pronto.

Pero encaja naturalmente.

---

# 33. Arquitectura final del ecosistema

```text
                         Repository
                             │
                             ▼
                    ┌─────────────────┐
                    │   Git Reader    │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │    Indexer      │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Local Storage  │
                    │                 │
                    │ commits         │
                    │ files           │
                    │ symbols         │
                    │ changes         │
                    │ relationships   │
                    └────────┬────────┘
                             │
                             ▼
                  ┌──────────────────────┐
                  │ Intelligence Engine  │
                  │                      │
                  │ coupling             │
                  │ ownership            │
                  │ risk                 │
                  │ impact               │
                  │ anomalies            │
                  │ context              │
                  └──────────┬───────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
           ▼                 ▼                 ▼
       CLI / TUI         Agent Skill       GitHub App
```

---

# 34. Orden mínimo de implementación

No desviarse de este orden salvo que aparezca una razón técnica fuerte:

```text
01. Separar core de TUI
02. Implementar nib index
03. Persistir commits/files/changes
04. Incremental indexing
05. File history
06. File churn
07. Ownership
08. File temporal coupling
09. nib coupled
10. Risk engine
11. Benchmarks
12. Tree-sitter Go
13. Symbol extraction
14. Symbol history
15. Symbol temporal coupling
16. Test relationships
17. nib impact
18. nib context
19. JSON interface
20. Agent skill
21. Working-tree impact
22. PR impact
23. Anomaly detection
24. Historical evaluation framework
25. GitHub App
26. Improve TUI around intelligence engine
```

No saltar directamente al número 25.

---

# 35. Lo que NO debemos construir todavía

Evitar scope creep.

No dedicar tiempo inicialmente a:

```text
GitHub issue management
PR creation
LLM chat
commit message generation mejorado
multi-provider AI
cloud dashboard
team accounts
billing
editor integrations
20 lenguajes
remote indexing
distributed processing
custom database engine
```

Todas esas cosas pueden venir después.

El núcleo debe demostrar primero:

> El historial del repositorio contiene conocimiento útil que Nib puede extraer.

---

# 36. Los primeros comandos importantes

Idealmente:

```bash
nib index
nib history
nib coupled
nib risk
nib impact
nib context
```

Todo lo demás es secundario.

---

# 37. Benchmarking

Los benchmarks deben formar parte del proyecto desde temprano.

Repositorios de distintas escalas:

```text
small
10k commits

medium
100k commits

large
1M+ commits
```

Medir:

```text
initial indexing time
incremental indexing time
index size
memory usage
coupling query latency
history query latency
impact query latency
```

Ejemplo de README futuro:

```text
Repository:
1,024,871 commits
91,281 files
8.7M file changes

Initial index:
18.4s

Incremental index:
41ms / commit

Queries:

history        p50 0.8ms
coupling       p50 2.1ms
impact         p50 7.4ms
```

Nunca inventar benchmarks.

Guardar scripts reproducibles.

---

# 38. Correctness antes que performance

Debes mantener invariantes.

Por ejemplo:

```text
Every indexed commit exists exactly once.

Every FileChange references an existing commit.

Every FileChange references an existing file.

Incremental indexing produces the same index
as full indexing.

Coupling(A,B) calculated incrementally equals
coupling(A,B) calculated from complete history.
```

Crear tests específicamente para estas propiedades.

---

# 39. Golden repositories

Crear pequeños repos Git artificiales dentro de tests.

Ejemplo:

```text
commit 1
A

commit 2
A B

commit 3
A B

commit 4
B

commit 5
A B C
```

Entonces puedes saber exactamente:

```text
A changed 4 times
B changed 4 times
A+B changed 3 times

coupling(A→B) = 3/4
coupling(B→A) = 3/4
```

Esto permite comprobar algoritmos con precisión.

---

# 40. Property testing

Muy recomendable para partes sensibles.

Generar historiales aleatorios:

```text
commits
files
changes
```

Y comprobar:

```text
incremental index == full rebuild

optimized coupling == brute force coupling
```

La implementación lenta y obvia puede convertirse en el oracle para validar la optimizada.

---

# 41. Decisiones que debes tomar TÚ antes de pedírselas a Codex

Codex puede implementar.

No debería decidir la arquitectura central.

Antes de cada componente debes escribir respuestas a:

## Storage

```text
¿Qué entidades existen?

¿Qué claves tienen?

¿Qué consultas necesitamos optimizar?

¿Qué datos son append-only?

¿Qué datos se pueden reconstruir?

¿Qué índices necesitamos?

¿Qué ocurre cuando cambia el schema?
```

## Temporal coupling

```text
¿Qué significa "changed together"?

¿La métrica es direccional?

¿Cuántas observaciones mínimas necesitamos?

¿Cómo evitamos relaciones espurias?

¿Penalizamos commits enormes?

¿Damos más peso a historia reciente?
```

## Risk

```text
¿Qué significa realmente riesgo?

¿Qué señales usamos?

¿Por qué?

¿Cómo normalizamos?

¿Cómo explicamos el resultado?

¿Cómo validamos que el score tenga utilidad?
```

## Symbols

```text
¿Cómo identificamos un símbolo?

¿Qué ocurre con renames?

¿Qué ocurre con movimientos?

¿Qué ocurre cuando cambia la firma?

¿Qué lenguajes soportamos?

¿Qué ocurre si el parser falla?
```

## Impact

```text
¿Qué significa impact?

¿Dependencia estructural?

¿Relación histórica?

¿Ambas?

¿Cómo combinamos señales?

¿Cómo representamos confidence?
```

## Anomalies

```text
¿Qué comportamiento consideramos normal?

¿Cuántas observaciones necesitamos?

¿Qué threshold usamos?

¿Cómo evitamos generar ruido?

¿Qué false-positive rate consideramos aceptable?
```

---

# 42. Preguntas que deberías poder responder en una entrevista

Si quieres demostrar ownership, tienes que poder responder con seguridad a estas preguntas.

## Arquitectura

**¿Por qué Nib mantiene un índice propio en lugar de ejecutar `git log` para cada consulta?**

Debes poder explicar:

```text
latency
repeated computation
query patterns
precomputed relationships
incremental updates
```

---

**¿Por qué no utilizaste una graph database?**

Una posible respuesta:

> Porque los patrones de acceso son muy específicos y un conjunto pequeño de índices especializados proporciona menor complejidad operativa y mejor localidad. El grafo es el modelo conceptual, no necesariamente la representación física.

Pero debes llegar a tu propia conclusión.

---

**¿Por qué Tree-sitter?**

Debes explicar:

```text
incremental parsing
multi-language ecosystem
concrete syntax trees
performance
error tolerance
```

Y también sus limitaciones.

---

# 43. Preguntas sobre datos

**¿Cómo representas un millón de commits eficientemente?**

**¿Por qué utilizas IDs numéricos internos?**

**¿Cómo representas las relaciones commit↔file?**

**¿Cómo calculas coupling sin hacer O(N²) sobre todos los archivos?**

**¿Cómo manejas repositorios donde un commit modifica 10.000 archivos?**

Ésta última es especialmente importante.

Un commit masivo puede crear relaciones falsas entre miles de archivos.

Necesitas decidir si:

```text
ignoras commits enormes
reduces su peso
usas thresholds
clasificas mechanical commits
```

---

# 44. Preguntas sobre estadística

**¿Correlation significa dependency?**

No.

Nib debe dejarlo claro.

Temporal coupling significa:

> Estas entidades han cambiado juntas históricamente.

No:

> A depende necesariamente de B.

---

**¿Cómo sabes que un score alto significa riesgo real?**

La respuesta correcta no debería ser:

> Porque tiene sentido.

Deberías hablar de evaluación histórica.

---

**¿Cómo evitas relaciones con una sola observación?**

Ejemplo:

```text
A changed once
A+B changed once

coupling = 100%
```

Pero eso no significa que la relación sea fuerte.

Necesitas considerar:

```text
minimum support
confidence
observation count
```

Muy importante.

---

# 45. Preguntas sobre Git

Debes conocer especialmente bien:

```text
Git object model
commits
trees
blobs
refs
packfiles
object IDs
merge commits
renames
diff semantics
rebase
shallow clones
partial clones
worktrees
```

No necesitas reimplementar Git.

Pero si construyes una herramienta basada profundamente en Git, debes entender su modelo.

---

# 46. Preguntas sobre performance

**¿Cuál es tu operación más cara?**

**¿Qué complejidad tiene?**

**¿Qué memoria consume el índice?**

**¿Qué partes están precomputadas?**

**¿Qué partes se calculan bajo demanda?**

**¿Por qué?**

**¿Dónde utilizarías mmap?**

**¿Dónde utilizarías roaring bitmaps?**

**¿Qué pasa con cache locality?**

No necesitas implementar todas esas optimizaciones.

Pero debes saber cuándo tendrían sentido.

---

# 47. Preguntas sobre incremental computation

Una de las mejores señales técnicas del proyecto será poder explicar:

> Cómo actualizar el conocimiento del repo cuando aparece un solo commit sin recalcular toda la historia.

Ejemplo:

```text
new commit:
A B C changed
```

Necesitas actualizar:

```text
file histories
change counters
author ownership
A-B coupling
A-C coupling
B-C coupling
risk inputs
symbol relationships
```

Eso debería estar bien diseñado.

---

# 48. Cómo trabajar con Codex

Nunca:

```text
"Implement Nib temporal intelligence."
```

Sí:

```text
Goal:
Implement an append-only CommitStore.

Invariants:
- Commit IDs are unique.
- Insert order must be preserved.
- Lookup by OID must be O(1) expected or O(log n).
- Store must survive process restart.

Interfaces:
...

Non-goals:
- no branch support
- no compression
- no caching

Tests required:
...
```

Cada tarea debe contener:

```text
context
goal
constraints
interface
invariants
non-goals
tests
acceptance criteria
```

Entonces revisas tú el resultado.

---

# 49. ADRs

Crear:

```text
docs/adr/
```

Architecture Decision Records.

Ejemplos:

```text
001-storage-engine.md
002-internal-identifiers.md
003-temporal-coupling-definition.md
004-tree-sitter-symbol-model.md
005-incremental-indexing.md
006-risk-scoring.md
```

Formato:

```text
Context
Decision
Alternatives considered
Consequences
```

Esto te obliga a tener ownership real.

Y queda increíblemente bien para alguien leyendo el repo.

---

# 50. Research notebook

También mantendría:

```text
docs/research/
```

Por ejemplo:

```text
temporal-coupling.md
ownership-entropy.md
commit-classification.md
symbol-identity.md
impact-evaluation.md
```

Aquí documentas experimentos y errores.

Especialmente:

```text
What I expected
What happened
Why
What changed
```

Esto demuestra mucha más madurez que solamente código limpio.

---

# 51. README final

El README no debe empezar con:

> Nib is a modern Git TUI written in Go.

Debería acabar convirtiéndose en algo parecido a:

```text
Nib

Temporal code intelligence from Git history.

Nib indexes repository history and learns how code
evolves over time.

It identifies:

- historically coupled code
- risky areas
- likely blast radius
- relevant tests
- repository-specific change anomalies

Designed for humans and coding agents.
```

Después demo inmediatamente.

---

# 52. Demo definitiva

Idealmente grabar algo extremadamente sencillo:

```bash
git clone <large-project>
cd <large-project>

nib index
```

Luego:

```bash
nib risk
```

Seleccionas una función conocida.

```bash
nib impact SomeFunction
```

Y finalmente modificas una parte:

```bash
nib impact --working-tree
```

Nib detecta que falta otra modificación históricamente relacionada.

La demo debe poder entenderse sin conocer internals.

---

# 53. Qué convierte realmente esto en un proyecto WTF

No es tener muchas features.

No es usar IA.

No es la TUI.

No es Go.

No es Tree-sitter.

La combinación poderosa es:

```text
huge Git history
        +
persistent temporal index
        +
source-code structure
        +
statistical relationships
        +
incremental computation
        +
fast queries
        +
historical validation
        +
real developer workflow
```

Si todo funciona conjuntamente, tienes algo técnicamente raro y defendible.

---

# 54. Definition of Done del núcleo

Antes de trabajar seriamente en el GitHub bot, debería ser posible:

```bash
nib index
nib coupled <file>
nib coupled <symbol>
nib risk <symbol>
nib impact <symbol>
nib context <symbol> --json
nib impact --working-tree
```

y todo debería:

```text
ser rápido
ser reproducible
tener tests
tener explicación
tener benchmarks
funcionar incrementalmente
producir evidencia útil
```

Ese es el checkpoint principal.

---

# 55. Definition of Done del proyecto WTF

Consideraría que Nib ha llegado al nivel que buscamos cuando puedas demostrar estas cinco cosas:

### 1. Escala

Indexar un repositorio grande de manera razonable.

### 2. Velocidad

Consultas complejas en milisegundos o tiempos suficientemente bajos y documentados.

### 3. Inteligencia

Descubrir relaciones que no son evidentes leyendo únicamente el código actual.

### 4. Validación

Demostrar mediante historia pasada que algunas predicciones tienen valor.

### 5. Integración

Que humanos y coding agents puedan utilizar esa información dentro de su workflow.

Si consigues las cinco, ya no importa demasiado que Nib empezase como una TUI.

Será un proyecto de sistemas, análisis de software, tooling, almacenamiento, parsing y developer infrastructure.

---

# 56. La frase que tienes que poder defender

Cuando alguien te pregunte:

> What is Nib?

La respuesta debería poder ser:

> Nib is a local temporal code-intelligence engine. It indexes Git history at file and symbol level, learns how a repository evolves, and uses those historical relationships to estimate change impact, detect anomalous pull requests and provide repository-specific context to developers and coding agents.

Y después tienes que poder explicar exactamente cómo funciona cada palabra de esa frase.

Si puedes hacerlo, tienes ownership.

---

# 57. La regla principal durante todo el desarrollo

Ante cualquier nueva feature, preguntarte:

> ¿Esto mejora la capacidad de Nib para entender cómo evoluciona el software?

Si la respuesta es no, probablemente no toca construirlo todavía.

El camino es:

```text
Git TUI
    ↓
Historical index
    ↓
Temporal relationships
    ↓
Symbol intelligence
    ↓
Impact analysis
    ↓
Change anomaly detection
    ↓
Agent context
    ↓
PR intelligence
```

No:

```text
Git TUI
    ↓
más botones
    ↓
ChatGPT
    ↓
GitHub integration
    ↓
200 features
```

El primero puede convertirse en una pieza seria de developer infrastructure.

El segundo acaba siendo otra Git UI.