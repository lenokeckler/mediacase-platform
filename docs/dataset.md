# Dataset de prueba

Cubre el punto "Dataset multimedia de prueba" de la consigna: 400-600 archivos, mezcla de audio y
video (más imágenes), diversidad de formatos y de tamaños (livianos, medianos y pesados),
organizado en casos homogéneos y heterogéneos, con metadatos en JSON, y documentado en
composición, criterios de agrupación y volumen.

## Resumen

| | |
|---|---|
| Archivos | **492** (`dataset/manifest.json`, versión 2) |
| Volumen | **14.33 GB (15 GB en disco)** en `dataset/files/` (no versionado; se regenera) |
| Tipos | video · audio · imagen |
| Formatos | video: mp4, mkv, avi, mov, webm · audio: mp3, wav, flac, aac, ogg · imagen: jpg, png, webp |
| Niveles de tamaño | liviano < 5 MB · mediano 20-50 MB · pesado 150-400 MB (medidos con `stat`, no estimados) |
| Metadatos por archivo | `filename, key, type, format, size_bytes, duration_s, tier, event, session, batch, user` |
| Generación | `bash dataset/scripts/generate_dataset.sh` (ffmpeg, semilla fija 42, reproducible) |
| Validación | `python dataset/scripts/check_manifest.py dataset/manifest.json` |
| Ingesta y casos | `bin/ingest upload` · `bin/ingest cases --group-by <criterio>` · `bin/ingest load` |

## Composición

Por tipo de contenido y nivel de tamaño (cantidad de archivos y volumen):

| Tipo | liviano (< 5 MB) | mediano (20-50 MB) | pesado (150-400 MB) | Total | Volumen |
|---|---:|---:|---:|---:|---:|
| video | 140 (0.36 GB) | 80 (2.71 GB) | 30 (6.37 GB) | 250 | 9.44 GB |
| audio | 100 (0.26 GB) | 60 (2.13 GB) | 12 (2.50 GB) | 172 | 4.88 GB |
| image | 70 (0.01 GB) | — | — | 70 | 0.01 GB |
| **total** | 310 | 140 | 42 | **492** | **14.33 GB** |

Por formato:

| Formato | Archivos | Volumen | Duración total |
|---|---:|---:|---:|
| mov | 56 | 2255 MB | 1h 03m |
| avi | 54 | 2038 MB | 0h 58m |
| mp4 | 52 | 2338 MB | 1h 03m |
| webm | 49 | 2175 MB | 1h 00m |
| wav | 46 | 1563 MB | 2h 34m |
| mkv | 39 | 855 MB | 0h 28m |
| flac | 35 | 2003 MB | 6h 17m |
| mp3 | 35 | 595 MB | 7h 12m |
| aac | 34 | 456 MB | 6h 33m |
| png | 29 | 6 MB | 0h 00m |
| webp | 23 | 4 MB | 0h 00m |
| ogg | 22 | 385 MB | 2h 27m |
| jpg | 18 | 5 MB | 0h 00m |

### Cómo se construye cada archivo

Todo se sintetiza con ffmpeg para no depender de descargas ni de licencias, pero con **contenido
real** que obliga al códec a trabajar al convertir:

- **Video:** fuentes `testsrc2`, `life`, `cellauto`, `smptebars` (y `mandelbrot` en los livianos)
  con audio de ruido rosa. Livianos a 640×360 y 1.5 Mb/s, medianos a 1280×720 y 4 Mb/s,
  pesados a 1920×1080 y 6 Mb/s. Los contenedores mp4/mkv/avi/mov llevan H.264 (x264 en CBR
  estricto, `nal-hrd=cbr`) y webm lleva VP8 + Vorbis.
- **Audio:** onda senoidal (220-880 Hz) mezclada con ruido marrón y trémolo, estéreo 44.1 kHz.
  mp3 a 192 kb/s, aac a 160 kb/s, ogg calidad 10, flac y wav sin pérdida. Los pesados son solo
  wav/flac (150 MB de mp3 serían horas de audio).
- **Imágenes:** un cuadro de las mismas fuentes, 800-1920 × 600-1080 px, jpg/png/webp. Todas
  livianas.

**El nivel es por tamaño real, no por duración.** Cada archivo recibe un tamaño objetivo dentro
de su nivel (por ejemplo 25-45 MB para un mediano), la duración se deriva del bitrate del códec,
y al terminar se mide con `stat`; si se salió del rango se reintenta una vez reescalando el
bitrate (video) o la duración (audio), y si aun así no entra el generador aborta. Por eso un wav
mediano dura ~3 minutos y un mp3 mediano ~25 minutos: pesan lo mismo.

### Reproducibilidad

El generador usa un generador congruencial propio en vez de `$RANDOM` (bash ≥ 5.1 lo
re-siembra en cada subshell, así que la semilla no servía). Con `--seed 42` salen los mismos
nombres, formatos, duraciones y metadatos en cualquier máquina; los tamaños en bytes pueden
variar unos KB según la versión de ffmpeg, pero siempre dentro del nivel. Es reanudable: si se
interrumpe, al volver a correrlo salta los archivos que ya existen y están dentro de rango.

## Criterios de agrupación en casos

Cada archivo lleva cuatro metadatos de agrupación asignados de forma determinista, que son los
criterios que la consigna cita para la generación automática (evento, sesión, usuario, lote de
ingesta):

| Metadato | Valores | Qué modela |
|---|---|---|
| `event` | boda, concierto, clase, entrevista, partido, documental | el evento grabado |
| `session` | `<event>-s1` … `<event>-s4` (24 sesiones) | una sesión de grabación dentro del evento |
| `batch` | `lote-2026-09-01` … `lote-2026-09-06` | el lote de ingesta (día en que entró) |
| `user` | leno, jennifer, jonathan | quién lo subió |

**Casos homogéneos y heterogéneos por construcción.** Al agrupar por `session`, la sesión **s1**
de cada evento solo contiene video, la **s2** solo audio y las **s3/s4** mezclan video, audio e
imágenes. Así el mismo criterio produce los dos conjuntos que pide la consigna sin intervención
manual. Además `--group-by type` y `--group-by tier` dan siempre casos homogéneos, y `event`,
`batch` y `user` dan siempre heterogéneos (son grupos grandes con los tres tipos).

| Criterio | Valores | Archivos por caso | Casos homogéneos | Casos heterogéneos |
|---|---:|---:|---:|---:|
| event | 6 | 72–95 | 0 | 6 |
| session | 24 | 5–39 | 12 | 12 |
| batch | 6 | 71–99 | 0 | 6 |
| user | 3 | 155–173 | 0 | 3 |
| type | 3 | 70–250 | 3 | 0 |
| tier | 3 | 42–310 | 0 | 3 |

El coordinador decide la operación de cada archivo por su tipo (routing): video → `convert`,
audio → `convert_audio`, imagen → `thumbnail`. Un caso heterogéneo por tanto ejecuta tres
operaciones distintas en tres pools distintos (`worker-video`, `worker-audio`,
`worker-metadata`).

## Cómo se usa

```bash
# 1. Generar (una vez, ~1 h, ~14 GB). Perfil quick para probar el flujo en 3 min.
bash dataset/scripts/generate_dataset.sh                  # o --profile quick
python dataset/scripts/check_manifest.py dataset/manifest.json --markdown

# 2. Subir al bucket de entradas (MinIO dataset/). Reanudable: salta lo que ya está.
go build -o bin/ingest.exe ./cmd/ingest
bin/ingest upload --dir dataset/files --manifest dataset/manifest.json --concurrency 4

# 3. Generación automática de casos
bin/ingest cases --group-by session --dry-run           # ver la agrupación sin crear nada
bin/ingest cases --group-by session --limit 10          # crea 10 casos (homogéneos y heterogéneos)
bin/ingest cases --group-by event --only heterogeneous  # 6 casos grandes, uno por evento
bin/ingest cases --group-by type --priority 8           # 3 casos homogéneos: todo el video, todo el audio…

# 4. Generador de carga: M casos concurrentes con N envíos en paralelo, espera y resume
bin/ingest load --cases 20 --concurrency 5 --group-by session --wait
```

`ingest cases` imprime una tabla caso → nº de archivos → tipos → clase (homogéneo /
heterogéneo) → id. `ingest load` imprime el tiempo de creación de cada caso y, con `--wait`,
el conteo por estado cada 3 s hasta que el barrier cierra todos.

### Carga por lotes y análisis de tiempos

El dataset permite las cuatro cosas que la consigna pide observar:

- **Concurrencia intra-caso:** un caso `event=documental` tiene decenas de sub-tareas que
  corren en paralelo en los tres pools.
- **Concurrencia inter-casos:** `ingest load --cases 20 --concurrency 5` mantiene 20 casos vivos
  a la vez; en el dashboard se ve `by_pool.video > 0` sostenido y los workers al 100 % de CPU.
- **Carga por lotes:** `--group-by batch` crea un caso por lote de ingesta.
- **Análisis de tiempos:** el reporte consolidado de cada caso (`GET /cases/{id}/report`) trae
  inicio, fin y duración del caso y de cada sub-tarea; el `tier` del manifest permite comparar
  livianos contra pesados (`docs/informe-pruebas.md`).

## Hito de la fase (`tests/dataset_scenario.sh`)

Sube el dataset (o verifica que está), crea casos por `session` con `--limit 10`, espera a que
cierren y comprueba: ≥ 1 caso homogéneo `completed`, ≥ 1 heterogéneo `completed` o
`partially_completed`, y todas las sub-tareas con `worker_id`. Termina con `HITO OK`.

## Licencias

Todo el contenido es sintético (generado con filtros de ffmpeg), sin material de terceros, así
que no hay licencias que respetar ni atribuir. No se incluyeron clips públicos (Big Buck Bunny,
Sintel…): habrían obligado a descargar cientos de MB en cada máquina y la consigna no los pide.
