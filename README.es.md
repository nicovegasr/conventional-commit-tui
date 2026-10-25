# nib

[English](README.md) · **Español**

![CI](https://github.com/nicovegasr/nib-git/actions/workflows/ci.yml/badge.svg)

Una herramienta de terminal para hacer [Conventional Commits](https://www.conventionalcommits.org/) en una línea y **no olvidarte de nada**: mientras marcas archivos, nib te dice qué otros suelen cambiar con ellos según el historial de git del repo.

Pensada para tenerla abierta en la terminal de VSCode e ir commiteando al vuelo.

![nib: marcas un archivo y aparecen sus relacionados](assets/tui.gif)

---

## Qué te da

1. **Commits en una línea**: marcas archivos → eliges el tipo → escribes el mensaje → `enter`. Con ratón o teclado.
2. **Archivos relacionados**: qué suele cambiar con lo que has marcado, y cuáles de esos tienes modificados sin marcar (el típico test que se te olvida).
3. **Análisis desde la CLI**: `nib coupled <archivo>` explica qué cambia con un archivo, con porcentajes y los commits que lo demuestran.
4. **Dentro del TUI, a un `/`**: revisa cada diff y salta a tu editor (`/changes`), mira tus commits por tipo (`/stats`), encuentra los archivos que más cambian (`/audit`) y apaga lo pesado (`/settings`).
5. **Medido, no prometido**: `nib evaluate` comprueba en commits reales si las sugerencias aciertan. Resultados más abajo.

Todo es local: nib solo lee tu repo con `git` y guarda su índice dentro de `.git/`.

---

## Instalación

Requiere Go 1.26+ y git.

```bash
git clone https://github.com/nicovegasr/nib-git
cd nib-git
make build                     # genera ./nib
make install                   # instala nib en ~/go/bin (úsalo desde cualquier repo)
cp nib ~/.local/bin/           # o cualquier carpeta de tu PATH
```

Después, dentro de cualquier repo:

```bash
nib
```

---

## El TUI — commitear en una línea

| Acción | Teclado | Ratón |
|---|---|---|
| Marcar / desmarcar un archivo | `espacio` | click en el archivo |
| Moverse por la lista | `j` / `k` o flechas | — |
| Elegir el tipo (`feat`, `fix`, …) | `tab` + la inicial | click en `[ feat ▾ ]` |
| Escribir el mensaje y commitear | escribir + `enter` | click en el mensaje |
| Volver atrás / salir | `esc` · `q` · `ctrl+c` | — |

Debajo de la línea de commit verás el subject exacto que se va a crear (`→ feat: … .`), con el punto final ya puesto. Hace `git add` **solo de los archivos marcados**, nunca `git add .`.

La sesión no se cierra al commitear: la lista y el git log se recargan cada 2 s, así que lo que cambies en el editor, los `git add` o los commits que hagas desde otra terminal aparecen solos. Tus marcas se mantienen.

### Relacionados

Al marcar archivos, la columna derecha muestra los cinco que más suelen cambiar con ellos:

- `+` en amarillo: está **modificado y sin marcar**. Probablemente se te olvida.
- `·` en gris: suele cambiar con lo marcado, pero no lo has tocado.
- `↔` en la lista de archivos: el mismo aviso, junto al archivo.

El porcentaje es la proporción de commits del archivo marcado que también tocaron ese otro. El índice se crea solo la primera vez (en un repo grande tarda unos segundos, en segundo plano) y se mantiene al día tras cada commit, checkout o rebase. Si algo falla, el panel se queda en una línea gris y puedes commitear igual.

Con menos de 80 columnas de ancho, la columna derecha se oculta para no estorbar al flujo mínimo.

---

## Comandos dentro del TUI — `/`

Escribe `/` con el mensaje vacío (o haz click en `/ comandos`) para abrir la paleta, teclea unas letras y pulsa `enter`. `esc` te devuelve a la vista de commit. Puedes lanzar nib desde cualquier subcarpeta del repo.

### Revisa tus cambios — `/changes`

![/changes: los archivos cambiados y el diff del seleccionado](assets/changes.png)

A la izquierda los archivos cambiados; a la derecha el archivo entero con lo añadido en verde y lo borrado en rojo. Pensado para revisar lo que ha cambiado un agente o una sesión larga sin abrir un IDE.

| Acción | Teclado | Ratón |
|---|---|---|
| Filtrar archivos por ruta | `/` y escribir | click en el buscador |
| Cambiar de archivo / scroll del diff | `j` / `k` (`tab` cambia de panel) | click · rueda |
| Cambio siguiente / anterior | `n` / `p` | las marcas de color de la barra |
| Media página | `ctrl+d` / `ctrl+u` | arrastrar la barra |
| Marcar el archivo para el commit | `espacio` | — |
| Editar el archivo en ese cambio | `e` | — |

`e` le pasa la terminal a tu editor, abierto en el cambio que estás viendo, y al salir nib recarga el diff. El editor es `$VISUAL` o `$EDITOR` (p. ej. `export EDITOR=vim` en tu `.zshrc`); si no hay ninguno, `micro`, `nano` o `vi`. nib nunca edita archivos por sí mismo.

### Tus commits por tipo — `/stats`

![/stats: commits por tipo y actividad diaria](assets/stats.png)

Los commits de la rama actual por tipo y su actividad: por día hasta 30 días, por semana a partir de ahí. `d` o un click en el título cambia entre 7 · 30 · 90 · 365 días. Los commits sin prefijo `tipo:` cuentan como `sin tipo`; el culpable habitual es el título de squash de GitHub (`Feat/audit (#13)`), así que ponle a tus PR un título convencional.

### Los archivos que más cambian — `/audit`

![/audit: archivos ordenados por número de commits](assets/audit.png)

Archivos ordenados por **cuántos commits los tocaron** (los hotspots de Adam Tornhill); las líneas cambiadas son solo contexto. Los que pasan el umbral de `/settings` salen como `refactor?` y los que se acercan, `vigilar`. `d` cambia la ventana. Los lockfiles y el build se ignoran; añade tus patrones en un `.auditignore` en la raíz, con el mismo formato que `.nibignore`.

### Apaga lo que no uses — `/settings`

El panel de git log, el de relacionados, la captura del ratón (apagada = selección y scroll nativos de la terminal), el alcance del log (rama actual o todas), cada cuánto se recarga y los valores por defecto de `/audit`. Se aplica al momento y se guarda en `~/.config/nib/settings.toml`.

---

## ¿Qué suele cambiar con este archivo? — `nib coupled`

![nib coupled en la terminal](assets/coupled.png)

```bash
nib index                                      # crea o actualiza el índice
nib coupled internal/ui/model.go               # relacionados, con evidencia
nib coupled internal/ui/model.go --json        # salida para otras herramientas
nib coupled internal/ui/model.go --before v1.0 # solo historia anterior a un commit
```

Cada fila dice cuántos commits compartieron ambos archivos (`6/7`) y cita commits concretos que puedes abrir con `git show`. Por defecto cuenta cualquier co-cambio, ignora commits masivos (más de 50 archivos) y da más peso a lo reciente (vida media de 90 días).

Para quitar ruido (lockfiles, changelogs, generados), crea un `.nibignore` en la raíz con un glob o directorio por línea. No hace falta reindexar:

```gitignore
*.lock
package-lock.json
CHANGELOG.md
dist/
```

---

## ¿Funciona de verdad? — `nib evaluate`

`nib evaluate` esconde commits del pasado y pregunta a nib, usando **solo la historia anterior** a cada uno, qué otros archivos cambiaron. Compara con dos referencias: contar sin filtros qué cambió junto (`co-cambio`) y sugerir los archivos más tocados del repo (`frecuencia`).

Precisión de las 5 primeras sugerencias en los commits más recientes de cada repo (los parámetros se ajustaron con commits más antiguos):

| Repo | nib | co-cambio | frecuencia |
|---|---:|---:|---:|
| fzf (Go) | **30.6%** | 23.9% | 18.0% |
| Django (Python) | **13.1%** | 11.5% | 2.3% |
| OkHttp (Kotlin/Java) | 20.0% | **21.9%** | 1.9% |
| Flask (Python) | 18.6% | **19.3%** | 2.8% |
| ripgrep (Rust) | 13.3% | 13.3% | 4.2% |
| Astro (TypeScript) | 7.9% | 7.9% | 1.7% |
| Blog personal (Astro) | 22.3% | **23.3%** | 11.6% |
| media de 7 repos | **18.0%** | 17.3% | 6.1% |

Lectura honesta: fijarse en **lo que cambia con tu archivo** acierta de media unas 3 veces más que fijarse en lo que más cambia en general (desde 1,7× en fzf hasta 10× en OkHttp). Los filtros de nib aportan poco sobre el co-cambio simple en estas muestras pequeñas (24–52 casos por repo); en muestras más grandes la ventaja es de unos 2,5 puntos. Metodología y números completos en [docs/research/file-coupling-benchmarks.md](docs/research/file-coupling-benchmarks.md).

```bash
nib evaluate --prefix src/ --ext .ts,.tsx     # mide nib en tu propio repo
```

---

## Límites

- Un archivo es una **ruta**: los renombrados no unen su historia.
- Solo la historia alcanzable desde `HEAD`; los clones superficiales (`--depth`) se rechazan.
- Co-cambio histórico no es dependencia ni predicción de bugs: es una pista.

---

## Documentación

| Documento | Contenido |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Paquetes, flujo y reglas de diseño |
| [docs/adr/](docs/adr/) | Las decisiones, una por archivo (índice, evaluación, panel de relacionados…) |
| [docs/research/file-coupling-benchmarks.md](docs/research/file-coupling-benchmarks.md) | Benchmarks y evaluaciones en repos reales |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Qué viene después |
| [docs/contributing.md](docs/contributing.md) | Cómo trabajar en el repo |

---

## Desarrollo

```bash
make check   # formato, vet, lint y tests (pre-commit)
make ci      # todo lo anterior + cobertura ≥ 80% + integración con git real
make bench   # microbenchmarks del índice
```

Las capturas de este README se generan con [vhs](https://github.com/charmbracelet/vhs) sobre un clon de demo: `vhs assets/tapes/<nombre>.tape` (`tui`, `changes`, `stats`, `audit`, `coupled`).
