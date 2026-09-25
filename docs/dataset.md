# Dataset de prueba

Cubre el punto "Dataset multimedia de prueba" de la consigna: 400-600 archivos, mezcla de audio y
video (más imágenes), diversidad de formatos y de tamaños (livianos, medianos y pesados),
organizado en casos homogéneos y heterogéneos, con metadatos en JSON, y documentado en
composición, criterios de agrupación y volumen.

Desde la versión 3 el dataset combina tres procedencias:

- **sintético**: generado con ffmpeg, controla con precisión los tamaños y la cantidad (la base de
  carga);
- **real**: 63 obras reales con licencia libre (películas abiertas de Blender, NASA, Wikimedia
  Commons, Musopen, LibriVox, Prelinger) y 39 variantes derivadas de ellas con ffmpeg. Aportan
  contenido de verdad, formatos y códecs poco comunes y nombres de archivo con tildes y espacios;
- **edge**: 8 casos límite y fallos a propósito (vacío, truncado, extensión que miente, texto
  disfrazado de audio…) para comprobar que un caso termina `partially_completed` con el error
  explicado en el reporte.

## Resumen

| | |
|---|---|
| Archivos | **542** (`dataset/manifest.json`, versión 3): 432 sintéticos, 102 reales, 8 límite |
| Volumen | **16.53 GB** en `dataset/files/` (no versionado; se regenera) |
| Tipos | video 265 · audio 200 · imagen 77 |
| Formatos | 28: video mp4, mov, avi, webm, mkv, m4v, mpeg, mpg, ts, flv, 3gp, wmv · audio mp3, wav, flac, aac, ogg, m4a, opus, aiff, wma · imagen png, jpg, webp, gif, tif, tiff, bmp |
| Niveles de tamaño | liviano · mediano · pesado (medidos en disco; ver [Niveles](#niveles-de-tamaño)) |
| Metadatos por archivo | `filename, key, type, format, size_bytes, duration_s, tier, event, session, batch, user` + v3: `source, origin, license, author, url, note` (y `title`, `page`, `license_url`, `derived_from`, `expected`, `extension` cuando aplican) |
| Casos de prueba curados | **11** en `test_cases` del manifest (fuente: `dataset/test_cases.json`) |
| Generación | `generate_dataset.sh` (sintético) + `fetch_real.sh` (real y límite) + `build_manifest.py` |
| Validación | `python dataset/scripts/check_manifest.py dataset/manifest.json` |
| Créditos | `dataset/CREDITS.md` (obligatorio por las licencias CC BY y CC BY-SA) |
| Ingesta y casos | `bin/ingest upload` · `bin/ingest cases --group-by <criterio>` · `bin/ingest load` |

## Composición

Tablas generadas con `check_manifest.py --markdown` sobre el manifest real.

Por procedencia:

| Procedencia | Archivos | video | audio | imagen | Volumen |
|---|---:|---:|---:|---:|---:|
| sintético | 432 | 230 | 157 | 45 | 14.24 GB |
| real | 102 | 30 | 41 | 31 | 2.26 GB |
| límite (edge) | 8 | 5 | 2 | 1 | 0.03 GB |
| **total** | **542** | 265 | 200 | 77 | **16.53 GB** |

De los 102 reales, 63 son descargas originales (2.05 GB) y 39 son variantes derivadas con ffmpeg
(212 MB, clips de 3 a 180 s). Por origen de las descargas: Wikimedia Commons 18, Blender Foundation
14, Musopen 12, NASA 9, LibriVox 6, Prelinger Archives 4.

Por tipo de contenido y nivel de tamaño:

| Tipo | liviano | mediano | pesado | Total | Volumen |
|---|---:|---:|---:|---:|---:|
| video | 140 (0.36 GB) | 90 (3.08 GB) | 35 (7.49 GB) | 265 | 10.93 GB |
| audio | 115 (0.30 GB) | 72 (2.43 GB) | 13 (2.63 GB) | 200 | 5.36 GB |
| imagen | 68 (0.05 GB) | 9 (0.19 GB) | — | 77 | 0.24 GB |
| **total** | 323 | 171 | 48 | **542** | **16.53 GB** |

Por formato (la columna *límite* cuenta el formato **real** del contenido, no la extensión):

| Formato | Tipo | Archivos | sintéticos | reales | límite | Volumen | Duración total |
|---|---|---:|---:|---:|---:|---:|---:|
| mp4 | video | 61 | 47 | 11 | 3 | 2766 MB | 1h 43m |
| mov | video | 56 | 52 | 4 | — | 2939 MB | 1h 35m |
| avi | video | 52 | 50 | 2 | — | 2058 MB | 1h 07m |
| webm | video | 46 | 45 | 1 | — | 2166 MB | 0h 59m |
| mkv | video | 42 | 36 | 4 | 2 | 877 MB | 0h 29m |
| m4v | video | 2 | — | 2 | — | 126 MB | 0h 10m |
| 3gp | video | 1 | — | 1 | — | 1 MB | 0h 00m |
| flv | video | 1 | — | 1 | — | 2 MB | 0h 00m |
| mpeg | video | 1 | — | 1 | — | 245 MB | 0h 09m |
| mpg | video | 1 | — | 1 | — | 4 MB | 0h 00m |
| ts | video | 1 | — | 1 | — | 2 MB | 0h 00m |
| wmv | video | 1 | — | 1 | — | 3 MB | 0h 00m |
| mp3 | audio | 47 | 32 | 14 | 1 | 652 MB | 8h 05m |
| wav | audio | 46 | 41 | 4 | 1 | 1609 MB | 2h 40m |
| flac | audio | 40 | 34 | 6 | — | 2266 MB | 6h 49m |
| aac | audio | 30 | 29 | 1 | — | 445 MB | 6h 24m |
| ogg | audio | 28 | 21 | 7 | — | 399 MB | 2h 45m |
| m4a | audio | 5 | — | 5 | — | 108 MB | 2h 05m |
| opus | audio | 2 | — | 2 | — | 5 MB | 0h 11m |
| aiff | audio | 1 | — | 1 | — | 5 MB | 0h 00m |
| wma | audio | 1 | — | 1 | — | 2 MB | 0h 01m |
| png | imagen | 28 | 19 | 8 | 1 | 25 MB | — |
| jpg | imagen | 25 | 13 | 12 | — | 122 MB | — |
| webp | imagen | 16 | 13 | 3 | — | 2 MB | — |
| gif | imagen | 4 | — | 4 | — | 3 MB | — |
| tif | imagen | 2 | — | 2 | — | 69 MB | — |
| bmp | imagen | 1 | — | 1 | — | 4 MB | — |
| tiff | imagen | 1 | — | 1 | — | 18 MB | — |

### Niveles de tamaño

El nivel (`tier`) siempre sale del tamaño medido en disco, nunca de la duración:

| Procedencia | liviano | mediano | pesado |
|---|---|---|---|
| sintético | < 5 MB | 20-50 MB | 150-400 MB |
| real y límite | < 10 MB | 10-100 MB | ≥ 100 MB |

El generador sintético apunta a franjas estrictas y separadas (si un archivo se sale, reintenta o
aborta). El material real pesa lo que pesa: una obra de 7 MB o de 60 MB no cae en ninguna franja
sintética, así que se le asigna **el nivel más cercano** con cortes redondos (10 MB y 100 MB, que
quedan cerca de los puntos medios geométricos de los huecos 5-20 MB y 50-150 MB). Quedan 66 reales
livianos, 30 medianos y 6 pesados (de 115 a 355 MB). `check_manifest.py` valida las franjas
estrictas solo para los sintéticos y la regla de cortes para el resto.

### Qué aporta el material real

| Propiedad | Archivos |
|---|---|
| Video 4K real (3840×2160, 60 fps) | `real_nasa_atlas_v_4k.mp4` (197 MB), `real_atlas_4k_clip.mp4` (10 s) |
| Resoluciones de 176×144 a 4K | 3gp QCIF, BBB 180p/360p/480p, Sintel 480p/720p/1080p |
| Video vertical 9:16 | `real_sintel_vertical_9x16.mp4` (1080×1920) |
| Video sin pista de audio | `real_bbb_sin_audio.mp4`, `Despegue en cámara lenta.mov` |
| Frecuencia de cuadro variable | `real_tos_vfr.mkv` (tramos a 24 fps y a 8 fps) |
| Códecs poco comunes | Cinepak, MPEG-1, MPEG-2, H.263, Sorenson Spark (flv), WMV8, XviD, HEVC, VP9, AV1 |
| Audio de alta resolución y de baja calidad | FLAC 24 bits/96 kHz · WAV mono 8 kHz · MP3 mono 22 kHz · Opus 24 kb/s en modo voz |
| Audio sin pérdida en m4a | `real_chopin_*.m4a` (ALAC, no AAC) |
| Voz hablada | LibriVox (Martí, Girondo, Esopo) y un podcast de la NASA |
| Grabaciones de campo | Ciudad de México, un cruce de avenidas, autos sobre un puente, un ave del Chirripó de 1.6 s |
| Imágenes grandes | JPEG de 9917×3563 px (25 MB), TIFF de 16 bits en 4K, TIFF de 47 MB, BMP sin compresión |
| Transparencia y animación | PNG RGBA, WebP con alfa, 4 GIF animados, WebP animado |
| Imagen diminuta | `real_icono_16px.png` (la miniatura sale más grande que el original) |
| Nombres con tildes, espacios y paréntesis | `Volcán Irazú 01.jpg`, `Refugio Caño Negro.jpg`, `Grieg - La mañana (Peer Gynt).flac`, `Vals del minuto (Chopin).mp3`, `Entrevista día 2.m4a`, `Paisaje sonoro Taxqueña.wav`, `Despegue en cámara lenta.mov`, `Portada álbum.png`, `Iglesia de Orosi.jpg` |

### Casos límite (`source: "edge"`)

Cada uno lleva en el manifest una `note` con el comportamiento esperado y un campo `expected`
(`completed`, `failed` o `completed_or_failed`). El coordinador inspecciona el contenido de cada
archivo (`internal/cases/sniff.go`): si la extensión miente, enruta por el tipo real; si el
contenido no se reconoce, conserva el routing por extensión y el fallo ocurre en el worker.

| Archivo | Qué es | Esperado | Qué demuestra |
|---|---|---|---|
| `edge_truncado.mp4` | primer 30 % de `real_bbb_180p.mp4`, cuyo índice `moov` está al final | falla | el worker falla la sub-tarea con el error de ffmpeg y el caso sigue |
| `edge_vacio.mp4` | 0 bytes | falla | un archivo vacío no tumba el caso ni al worker |
| `edge_mp3_con_extension_mp4.mp4` | MP3 (con ID3) renombrado a .mp4 | completa | routing por contenido: se procesa como **audio**, no como video |
| `edge_mkv_con_extension_mp4.mp4` | Matroska HEVC renombrado a .mp4 | completa | el tipo coincide pero el contenedor real es mkv; convertir a mp4 sí cambia el formato |
| `edge_png_con_extension_jpg.jpg` | PNG RGBA renombrado a .jpg | completa | el olfateo detecta PNG; la miniatura resuelve la transparencia |
| `edge_texto_con_extension_wav.wav` | 180 bytes de texto plano | falla | sin cabecera RIFF: error claro en el reporte |
| `edge_extension_mayusculas.MP4` | tráiler de Sintel con extensión en mayúsculas | completa | el routing normaliza la extensión |
| `edge_bytes_corruptos.mkv` | mkv válido con 64 KiB en cero al 40 % | completa o falla | ffmpeg suele terminar con advertencias; lo que no puede pasar es que el worker se cuelgue |

Todos quedan en la sesión `pruebas-s3`, así que `ingest cases --group-by session` también produce
un caso con todos los límites juntos.

### Cómo se construyen los sintéticos

Todo se sintetiza con ffmpeg, pero con **contenido real** que obliga al códec a trabajar al
convertir:

- **Video:** fuentes `testsrc2`, `life`, `cellauto`, `smptebars` (y `mandelbrot` en los livianos)
  con audio de ruido rosa. Livianos a 640×360 y 1.5 Mb/s, medianos a 1280×720 y 4 Mb/s,
  pesados a 1920×1080 y 6 Mb/s. Los contenedores mp4/mkv/avi/mov llevan H.264 (x264 en CBR
  estricto, `nal-hrd=cbr`) y webm lleva VP8 + Vorbis.
- **Audio:** onda senoidal (220-880 Hz) mezclada con ruido marrón y trémolo, estéreo 44.1 kHz.
  mp3 a 192 kb/s, aac a 160 kb/s, ogg calidad 10, flac y wav sin pérdida. Los pesados son solo
  wav/flac (150 MB de mp3 serían horas de audio).
- **Imágenes:** un cuadro de las mismas fuentes, 800-1920 × 600-1080 px, jpg/png/webp. Todas
  livianas.

Cada archivo recibe un tamaño objetivo dentro de su nivel (por ejemplo 25-45 MB para un
mediano), la duración se deriva del bitrate del códec, y al terminar se mide con `stat`; si se
salió del rango se reintenta una vez reescalando el bitrate (video) o la duración (audio), y si
aun así no entra el generador aborta. Por eso un wav mediano dura ~3 minutos y un mp3 mediano
~25 minutos: pesan lo mismo.

El generador escribe **`dataset/manifest.synthetic.json`** (versión 2, 492 archivos). El manifest
v3 excluye 60 sintéticos livianos para hacer lugar al material real sin salir del rango
400-600: la lista fija está en `dataset/scripts/dropped_synthetic.txt` (`video_light_121..140`,
`audio_light_86..100`, `image_46..70`, los de numeración más alta de cada tipo). **Esos archivos
siguen existiendo en `dataset/files`** (el generador los crea igual, por semilla); solo quedan
fuera del manifest, así que `ingest upload` y `ingest cases` no los usan.

### Reproducibilidad

- **Sintético:** generador congruencial propio en vez de `$RANDOM` (bash ≥ 5.1 lo re-siembra en
  cada subshell). Con `--seed 42` salen los mismos nombres, formatos, duraciones y metadatos en
  cualquier máquina; los bytes pueden variar unos KB según la versión de ffmpeg, siempre dentro
  del nivel.
- **Real:** cada descarga tiene su `sha256` en `dataset/real_sources.json` (y `file_sha256` del
  archivo extraído cuando viene en .zip). `fetch_real.sh` aborta si un hash no coincide. La única
  excepción son los dos PNG que Wikimedia renderiza a partir de un SVG (`verify: "warn"`): el
  servidor puede regenerarlos, así que un hash distinto solo produce un aviso.
- **Derivados:** se generan con `-fflags +bitexact` y `-map_metadata -1` (sin versión del
  codificador ni fechas en el archivo). Con la misma versión de ffmpeg dan los mismos bytes; con
  otra, el mismo contenido.
- **Metadatos de agrupación del material real:** el evento viene del spec (música → `concierto`,
  voz → `clase`, podcast → `entrevista`, NASA → `documental`, películas → `cine`, fotos y
  grabaciones de campo → `naturaleza`, límites → `pruebas`); sesión y usuario salen de un hash
  SHA-1 del nombre del archivo, así que son los mismos en cualquier máquina.
- Todo es reanudable: el generador salta los archivos que ya están en rango, y `fetch_real.sh`
  salta los que ya tienen el hash correcto, retoma descargas cortadas (`curl -C -`) y no vuelve a
  codificar derivados existentes.

## Criterios de agrupación en casos

Cada archivo lleva cuatro metadatos de agrupación asignados de forma determinista, que son los
criterios que la consigna cita para la generación automática (evento, sesión, usuario, lote de
ingesta):

| Metadato | Valores | Qué modela |
|---|---|---|
| `event` | boda, concierto, clase, entrevista, partido, documental, cine, naturaleza, pruebas | el evento grabado |
| `session` | `<event>-s1` … `<event>-s4` (31 sesiones) | una sesión de grabación dentro del evento |
| `batch` | `lote-2026-09-01` … `lote-2026-09-06` (sintético), `lote-2026-09-20` (descargas reales), `-21` (derivados), `-22` (límites) | el lote de ingesta |
| `user` | leno, jennifer, jonathan | quién lo subió |

**Casos homogéneos y heterogéneos por construcción.** Al agrupar por `session`, la sesión **s1**
de cada evento solo contiene video, la **s2** solo audio y las **s3/s4** mezclan video, audio e
imágenes. La misma regla se aplica al material real, así que el mismo criterio produce los dos
conjuntos que pide la consigna sin intervención manual. Además `--group-by type` y
`--group-by tier` dan siempre casos homogéneos, y `event`, `batch` y `user` dan casos
heterogéneos.

| Criterio | Valores | Archivos por caso | Casos homogéneos | Casos heterogéneos |
|---|---:|---:|---:|---:|
| event | 9 | 8–101 | 0 | 9 |
| session | 31 | 2–44 | 14 | 17 |
| batch | 9 | 8–87 | 0 | 9 |
| user | 3 | 173–188 | 0 | 3 |
| type | 3 | 77–265 | 3 | 0 |
| tier | 3 | 48–323 | 0 | 3 |

El coordinador decide la operación de cada archivo por su tipo (routing): video → `convert`,
audio → `convert_audio`, imagen → `thumbnail`. Un caso heterogéneo por tanto ejecuta tres
operaciones distintas en tres pools distintos (`worker-video`, `worker-audio`,
`worker-metadata`).

## Casos de prueba curados (`test_cases`)

Además de la agrupación automática, el manifest trae 11 casos armados a mano, con operación,
formato de salida, ancho de miniatura y recursos asociados por archivo. Se editan en
`dataset/test_cases.json` y `build_manifest.py` los copia al manifest; `check_manifest.py`
comprueba que cada clave exista y que la operación, el destino y el ancho sean los que acepta
el coordinador (lee las reglas de `internal/cases/router.go`).

| id | Nombre | Clase | Archivos | Qué ejercita |
|---|---|---|---:|---|
| `tc01-pelicula-formatos` | Película abierta en varios formatos | heterogéneo | 12 | Big Buck Bunny en mp4, m4v, mov, flv, 3gp, mpg, webm VP9, mkv AV1 y gif: convert, extract_audio, thumbnail, convert_audio y metadata |
| `tc02-album-clasico` | Álbum clásico enriquecido | homogéneo | 12 | `enrich_audio` sobre grabaciones Musopen (flac, mp3, ogg, m4a ALAC) con título, compositor, álbum, año y nota |
| `tc03-fotos-nasa` | Archivo fotográfico NASA | homogéneo | 7 | miniaturas de 320, 640 y 1280 px en jpg, png y webp, desde JPEG, PNG, TIFF y escala de grises |
| `tc04-podcast-audiolibro` | Podcast y audiolibro | heterogéneo | 12 | voz hablada: convert_audio y metadata sobre mp3, ogg, aac, opus, wav 8 kHz y m4a/m4b |
| `tc05-formatos-raros` | Formatos raros | heterogéneo | 16 | ts, flv, 3gp, mpeg, wmv, avi Cinepak/XviD, m4v, HEVC, opus, aiff, wma, flac 24/96, bmp, tiff, webp animado |
| `tc06-casos-limite` | Casos límite y fallos | heterogéneo | 11 | los 8 límites sin operación + 3 controles; **esperado `partially_completed`** |
| `tc07-video-pesado` | Carga de video pesado | homogéneo | 12 | conversiones de video real de 100-370 MB, el 4K a 60 fps y 6 pesados sintéticos: satura el pool de video |
| `tc08-paisajes-sonoros` | Paisajes y sonidos de campo | heterogéneo | 13 | fotos de Costa Rica con tildes en el nombre y grabaciones de campo: thumbnail, metadata, convert_audio, enrich_audio |
| `tc09-cine-enriquecido` | Cine con recursos asociados | homogéneo | 6 | `enrich_video` con portada, título, autor, año y descripción; mov y m4v salen como mp4 |
| `tc10-imagenes-alfa-animadas` | Transparencia, animación y resoluciones extremas | homogéneo | 14 | miniaturas de PNG/WebP con alfa, GIF y WebP animados, TIFF y PNG de 16 bits en 4K, ícono de 16 px |
| `tc11-mixto-grande` | Caso mixto grande (40 archivos) | heterogéneo | 40 | 20 reales + 20 sintéticos sin operación: el coordinador enruta todo por tipo |

"Homogéneo" aquí significa un solo tipo de contenido **y** una sola operación; `check_manifest.py`
rechaza un caso cuya clase no coincide con sus archivos.

## Cómo se usa

```bash
# 1. Sintético (una vez, ~1 h, ~14 GB). Perfil quick para probar el flujo en 3 min.
bash dataset/scripts/generate_dataset.sh                  # escribe dataset/manifest.synthetic.json

# 2. Real y casos límite (~2 GB de descarga, ~6 min con buena conexión). Reanudable.
bash dataset/scripts/fetch_real.sh

# 3. Manifest v3 = sintético (menos dropped_synthetic.txt) + real + límite + test_cases
python dataset/scripts/build_manifest.py
python dataset/scripts/check_manifest.py dataset/manifest.json --markdown

# 4. Subir al bucket de entradas (MinIO dataset/). Reanudable: salta lo que ya está.
go build -o bin/ingest.exe ./cmd/ingest
bin/ingest upload --dir dataset/files --manifest dataset/manifest.json --concurrency 4

# 5. Generación automática de casos
bin/ingest cases --group-by session --dry-run           # ver la agrupación sin crear nada
bin/ingest cases --group-by session --limit 10          # crea 10 casos (homogéneos y heterogéneos)
bin/ingest cases --group-by event --only heterogeneous  # un caso grande por evento
bin/ingest cases --group-by type --priority 8           # 3 casos homogéneos: todo el video, todo el audio…
bin/ingest cases --test-cases tc02-album-clasico,tc06-casos-limite   # casos curados (o "all")

# 6. Generador de carga: M casos concurrentes con N envíos en paralelo, espera y resume
bin/ingest load --cases 20 --concurrency 5 --group-by session --wait
```

`fetch_real.sh` acepta `--keep-cache` (conserva los .zip de Blender en `dataset/.cache`, ignorado
por git), `--force-derived` (vuelve a generar derivados) y, solo para mantenimiento, `--pin`
(anota en el spec los hashes que falten al agregar una fuente nueva). Para agregar material real:
se añade la entrada en `dataset/real_sources.json` (con `url`, `key`, `license`, `author`,
`page`, `event`), se corre `fetch_real.sh --pin`, se revisa el hash anotado y se reconstruye el
manifest. Cada obra nueva debe sumarse a `dataset/CREDITS.md`.

`ingest cases` imprime una tabla caso → nº de archivos → tipos → clase (homogéneo /
heterogéneo) → id. `ingest load` imprime el tiempo de creación de cada caso y, con `--wait`,
el conteo por estado cada 3 s hasta que el barrier cierra todos.

### Carga por lotes y análisis de tiempos

El dataset permite las cuatro cosas que la consigna pide observar:

- **Concurrencia intra-caso:** un caso `event=documental` o `tc11-mixto-grande` tiene decenas de
  sub-tareas que corren en paralelo en los tres pools.
- **Concurrencia inter-casos:** `ingest load --cases 20 --concurrency 5` mantiene 20 casos vivos
  a la vez; en el dashboard se ve `by_pool.video > 0` sostenido y los workers al 100 % de CPU.
- **Carga por lotes:** `--group-by batch` crea un caso por lote de ingesta.
- **Análisis de tiempos:** el reporte consolidado de cada caso (`GET /cases/{id}/report`) trae
  inicio, fin y duración del caso y de cada sub-tarea; el `tier` y el `source` del manifest
  permiten comparar livianos contra pesados y sintético contra real
  (`docs/informe-pruebas.md`).

## Hito de la fase (`tests/dataset_scenario.sh`)

Sube el dataset (o verifica que está), crea casos por `session` con `--limit 10`, espera a que
cierren y comprueba: ≥ 1 caso homogéneo `completed`, ≥ 1 heterogéneo `completed` o
`partially_completed`, y todas las sub-tareas con `worker_id`. Termina con `HITO OK`.

## Licencias

El dataset mezcla material propio y de terceros:

- **Sintético** (432 archivos): generado con filtros de ffmpeg por el equipo; no tiene material de
  terceros.
- **Real** (102 archivos): obras publicadas con licencias que permiten copiarlas, redistribuirlas
  y transformarlas, incluso con fines comerciales. Por cantidad de archivos (descargas y
  derivados): CC BY 3.0 (24), dominio público de Musopen, LibriVox, Prelinger y Commons (32), dominio público
  de la NASA (14), CC0 (13), CC BY 2.5 (8), CC BY-SA 3.0 (7), CC BY-SA 4.0 (3) y CC BY 4.0 (1). No
  se usó nada con cláusula NC (no comercial) ni ND (sin derivadas): por eso se descartó la banda
  sonora de *Tears of Steel*, que es CC BY-ND.
- **Límite** (8 archivos): copias, recortes o alteraciones de obras reales (heredan su licencia) o
  contenido propio (el vacío y el de texto).

Las licencias CC BY y CC BY-SA exigen reconocer la autoría: **`dataset/CREDITS.md`** lista cada
obra con su autor, licencia, página y URL de descarga, y qué archivos del dataset salen de ella.
Cada entrada del manifest repite `license`, `author`, `url` (y `license_url`/`page` en el
material real), así que cualquier resultado procesado se puede rastrear hasta su obra original.
Las variantes derivadas de obras CC BY-SA quedan bajo la misma licencia. Los archivos no se
versionan en el repositorio: cada máquina los descarga desde su origen.
