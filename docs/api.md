# API del coordinador

Todo vive en un solo puerto (`http://<ip-de-node-1>:8080`): el dashboard en `/`, la API en `/api/…`
y, por compatibilidad con los workers, el cliente CLI y los scripts, también sin prefijo
(`/cases`, `/jobs`, `/workers`, `/stats`, `/ws`, `/upload`, `/dataset`, `/connect`, `/download/`,
`/metrics`). Los ejemplos usan la forma sin prefijo. Las respuestas son JSON salvo donde se
indica; los errores son texto plano con el código HTTP que corresponde.

Tiempos en RFC 3339 UTC. Identificadores UUID v4.

## Casos

### `POST /cases` — enviar un caso

Un caso es **una sola solicitud** con 1..N archivos ya presentes en el bucket `dataset/` de MinIO
(subidos con `POST /upload` o con `ingest upload`). El coordinador inspecciona cada archivo, decide
la operación por su tipo (routing) y el pool que la atiende, registra el caso, lo descompone en
sub-tareas y las encola. El caso se acepta entero o se rechaza entero.

**Inspección de contenido:** el routing por extensión es solo el primer paso; el coordinador
también lee el encabezado real de cada archivo (`internal/cases/sniff.go`, firmas binarias —
ftyp, EBML, RIFF, ID3/cuadro MPEG, ADTS, Ogg, etc. — sin ejecutar ffmpeg) y lo compara contra lo
que decía la extensión. Si el contenido real es de OTRO tipo (p. ej. un `.mp4` que en realidad es
un mp3), re-enruta por el tipo real: si la operación pedida ya no aplica, usa el default de ese
tipo en vez de rechazar el caso entero. Si el tipo coincide pero el formato real es distinto (un
`.mp4` que en realidad es un `.mkv`) conserva el tipo detectado. En ambos casos la sub-tarea trae
`routing_note` con el detalle; un archivo vacío trae la nota correspondiente y se procesa por
extensión igual (fallará en el worker, que es lo esperado). Si MinIO no está disponible o la
lectura falla, se conserva el routing por extensión sin nota.

```json
{
  "name": "boda-2026-09-06",
  "priority": 6,
  "files": [
    { "key": "video_medium_14.mkv" },
    { "key": "image_8.jpg" },
    { "key": "video_medium_14.mkv", "operation": "extract_audio", "target": "flac" },
    { "key": "image_8.jpg", "operation": "thumbnail", "target": "webp", "width": 640 },
    { "key": "audio_2.aiff", "operation": "metadata" }
  ]
}
```

| Campo | Tipo | Notas |
|---|---|---|
| `name` | string | libre; `ingest cases` usa `<criterio>=<valor>` |
| `priority` | int 1-10 | 8-10 → cola `high`, 4-7 → `normal`, 1-3 → `low`; omitido = 5 |
| `files[].key` | string | clave del objeto en `dataset/` |
| `files[].operation` | string, opcional | sugerencia del cliente; solo se acepta si aplica al tipo detectado (ver routing) |
| `files[].target` | string, opcional | formato de salida; solo se acepta si aplica a la operación (ver tabla). Omitido = el primero de la lista que no sea el formato de origen |
| `files[].width` | int, opcional | ancho de la miniatura: 320 (default), 640 o 1280; se ignora en otras operaciones |

Routing por tipo (`internal/cases/router.go`): la extensión decide el tipo; la primera operación
de la lista es la que se elige si el cliente no pide ninguna.

| Tipo | Extensiones | Operaciones válidas (→ salidas; la primera es la default) |
|---|---|---|
| video | mp4 mkv avi mov webm m4v flv wmv ts mts 3gp mpg mpeg | `convert` → mp4 mkv webm · `extract_audio` → mp3 wav flac aac · `thumbnail` → jpg png webp · `metadata` → json |
| audio | mp3 wav flac aac ogg m4a opus wma aiff aif dsf dff | `convert_audio` → flac mp3 wav aac ogg · `thumbnail` (forma de onda) · `metadata` |
| image | jpg jpeg png gif webp bmp tif tiff | `thumbnail` → jpg png webp · `metadata` |

El pool lo decide la operación: `convert` y `extract_audio` → `video`; `convert_audio` → `audio`;
`thumbnail` y `metadata` (livianas) → `metadata`. Cada sub-tarea devuelve `target`, `width` y,
una vez asignada, `assignment` (`afinidad` si la tomó un worker de su pool, `ayuda` si la tomó un
nodo libre de otro pool). `GET /catalog` devuelve estas tablas en JSON.

En `convert` y `convert_audio` el **formato de origen no se ofrece ni se acepta** como salida
(`video.mp4` con `target: "mp4"` → 400 *"ya está en mp4: convertirlo a mp4 no cambia el
formato"*); el default pasa a ser el siguiente de la lista (mp4 → MKV, mkv → MP4, flac → MP3,
mp3 → FLAC). Se consideran el mismo formato `jpeg`/`jpg`, `tiff`/`tif`, `aiff`/`aif`, `m4a`/`aac` y
`mpeg`/`mpg`. `thumbnail` no filtra: `png → png` a 320 px es un cambio de tamaño, no de formato, y
conserva la transparencia. El catálogo trae la regla en `identity_excluded_ops` y `ext_aliases`
para que el dashboard filtre igual que el coordinador.

Respuesta `201 Created`: el caso con sus sub-tareas (misma forma que `GET /cases/{id}`, todas en
`pending`). Errores: `400` si el body es inválido, `files` está vacío, supera 2000 archivos, una
clave falta, una extensión no se reconoce o la operación pedida no aplica al tipo (`archivo
foo.txt: tipo de archivo no soportado`).

### `GET /cases[?status=…]` — listar casos

Los 500 más recientes, opcionalmente filtrados por estado. Sin las sub-tareas.

```json
[
  { "id": "85a9da0f-…", "name": "session=concierto-s3", "status": "completed", "priority": 6,
    "total_jobs": 2, "created_at": "2026-09-11T07:41:44.84Z",
    "started_at": "2026-09-11T07:41:45.72Z", "completed_at": "2026-09-11T07:42:08.78Z" }
]
```

Estados: `queued` · `processing` · `retrying` · `completed` · `partially_completed` · `failed` ·
`cancelled`. Los cuatro últimos son terminales.

### `GET /cases/{id}` — un caso con sus sub-tareas

```json
{
  "id": "85a9da0f-90a0-4b44-9b1e-0b849ce1e78a",
  "name": "session=concierto-s3",
  "status": "completed",
  "priority": 6,
  "total_jobs": 2,
  "created_at": "2026-09-11T07:41:44.840449Z",
  "started_at": "2026-09-11T07:41:45.722776Z",
  "completed_at": "2026-09-11T07:42:08.789343Z",
  "jobs": [
    {
      "id": "2fda6147-8f7e-4874-8d19-548e187ff0a0",
      "case_id": "85a9da0f-90a0-4b44-9b1e-0b849ce1e78a",
      "file_path": "image_8.jpg",
      "file_type": "image",
      "pool": "metadata",
      "operation": "thumbnail",
      "status": "completed",
      "priority": 6,
      "worker_id": "node3",
      "progress": 100,
      "result_url": "http://172.24.83.164:9000/results/jobs/2fda6147-…/image_8_dlcbgamkbfrc_4.jpg",
      "created_at": "2026-09-11T07:41:44.843165Z",
      "started_at": "2026-09-11T07:41:45.703849Z",
      "completed_at": "2026-09-11T07:41:46.36525Z",
      "retries": 0,
      "max_retries": 3
    },
    { "…": "video_medium_14.mkv → convert en node1" }
  ]
}
```

Estados de una sub-tarea: `pending` · `assigned` · `running` · `completed` · `failed` ·
`cancelled`. `404` si no existe.

Cuando la inspección de contenido corrigió el routing (o el archivo estaba vacío), la sub-tarea
trae además `"routing_note": "extensión engañosa: .mp4 sugiere video, pero el contenido real es
audio (mp3); se enrutó por el contenido real"`. Ausente cuando la extensión era correcta.

### `GET /cases/{id}/report` — reporte consolidado

Disponible cuando el barrier cerró el caso (`completed`, `partially_completed`, `failed`) o se
canceló. Antes responde `409 Conflict` con `el caso aún no ha terminado (estado: processing)`.
La misma copia queda en MinIO en `results/cases/<id>/report.json`.

```json
{
  "case_id": "85a9da0f-90a0-4b44-9b1e-0b849ce1e78a",
  "name": "session=concierto-s3",
  "status": "completed",
  "created_at": "2026-09-11T07:41:44.840449Z",
  "started_at": "2026-09-11T07:41:45.722776Z",
  "completed_at": "2026-09-11T07:42:08.789343Z",
  "duration_seconds": 23.066567,
  "summary": "de 2 archivos — 1 miniatura generada, 1 video convertido",
  "totals": { "total": 2, "completed": 2, "failed": 0, "cancelled": 0 },
  "by_type_and_operation": [
    { "file_type": "image", "operation": "thumbnail", "completed": 1, "failed": 0, "cancelled": 0 },
    { "file_type": "video", "operation": "convert",   "completed": 1, "failed": 0, "cancelled": 0 }
  ],
  "sub_tasks": [
    { "job_id": "2fda6147-…", "file": "image_8.jpg", "file_type": "image", "operation": "thumbnail",
      "status": "completed", "worker_id": "node3", "result_url": "http://…/image_8_….jpg",
      "started_at": "…", "completed_at": "…", "duration_seconds": 0.661401 },
    { "job_id": "ff48e733-…", "file": "video_medium_14.mkv", "file_type": "video", "operation": "convert",
      "status": "completed", "worker_id": "node1", "result_url": "http://…/video_medium_14_….mp4",
      "started_at": "…", "completed_at": "…", "duration_seconds": 21.94 }
  ]
}
```

Una sub-tarea fallida trae además `"error": "ffmpeg: Invalid data found when processing input"`.
Una sub-tarea re-enrutada por su contenido real trae `routing_note` (ver `GET /cases/{id}`), y el
`summary` cuenta cuántas hubo: `"…, 1 archivo con extensión engañosa, enrutado por su contenido
real"`.

### `POST /cases/{id}/cancel` — cancelar

Marca el caso `cancelled` y cancela sus sub-tareas aún no iniciadas (`pending`/`assigned`). Las
que ya corren terminan, pero el caso no vuelve a cambiar de estado; el reporte se genera en ese
momento. `200` si se canceló; `409` si no existe o ya era terminal.

## Sub-tareas sueltas (pruebas y compatibilidad)

| Método y ruta | Qué hace |
|---|---|
| `POST /jobs` `{file_path, operation?, priority?}` | una sub-tarea sin caso; mismo routing; `201` con el job |
| `GET /jobs[?status=…]` | las 2000 más recientes |
| `GET /jobs/{id}` | una sub-tarea |

## Workers

Los workers **abren ellos** la conexión hacia el coordinador; ningún worker escucha un puerto para
recibir trabajo. Estas rutas las usa `cmd/worker`, no un cliente humano.

| Método y ruta | Body / respuesta |
|---|---|
| `POST /workers/register` | `{id, instance, hostname, role, capabilities[]}` → `200 {"status":"registered"}`. Si `id` ya existía con otra `instance` (proceso nuevo), sus sub-tareas en vuelo se re-encolan. |
| `POST /workers/{id}/heartbeat` | `{cpu_percent, mem_percent, active_jobs, metrics}` cada 1 s → `200`; `404` si no está registrado (el worker se vuelve a registrar). `metrics` = `{sampled_at, cpu_percent, mem_used_bytes, mem_total_bytes, mem_percent, disk_percent, gpus:[{index, percent, vram_used_bytes, temp_c}]}`; los campos que la máquina no puede medir vienen en `null`. |
| `GET /workers/{id}/stream` | WebSocket: el canal por el que bajan las sub-tareas. Mensajes `{"type":"assign","job":{…}}` ↓ y `{"type":"accept"\|"reject","job_id":…,"reason":…}` ↑, más `ping`/`pong`. Un `reject` (pool lleno) re-encola sin contar reintento. |
| `POST /workers/{id}/unregister` | `{instance}`: despedida ordenada; sus sub-tareas vuelven a la cola de inmediato. |
| `POST /jobs/{id}/progress` | `{progress, status, result_url?, error?}`. `status` = `running` (avance) · `completed` · `failed`. Es la **única** fuente de verdad del estado de una sub-tarea; un avance rezagado nunca regresa una terminada a `running`. Los reportes terminales se reintentan desde el worker hasta 15 min si el coordinador no responde. |
| `GET /workers` | los workers vivos: `[{id, instance, hostname, role, capabilities, status, active_jobs, cpu_percent, mem_percent, last_seen, hardware, metrics}]`. `hardware` (del registro) = `{os, arch, cpu_model, cpu_cores, cpu_threads, mem_total_bytes, gpus:[{index, name, vendor, integrated, vram_total_bytes, source}]}`; `metrics` = el último heartbeat. |

Un worker sin heartbeat por 15 s se expulsa y sus sub-tareas se re-encolan (el caso pasa a
`retrying` hasta que alguna vuelva a correr). Una sub-tarea en `running` sin noticias durante
15 min se marca fallida; una en `assigned` sin noticias durante 15 min vuelve a la cola.

## Entradas (dataset)

| Método y ruta | Qué hace |
|---|---|
| `POST /upload` (multipart, campo `file`, repetible) | sube al bucket `dataset/` con clave = nombre saneado; valida el tipo de cada archivo antes de subir nada → `201 {"keys":["a.mp4","b.jpg"]}` |
| `GET /dataset` | lista el bucket: `[{key, size_bytes, type, last_modified, format?, tier?, source?, duration_s?, event?, session?, license?, note?}]` (lo usa el dashboard para elegir archivos) |
| `GET /dataset/test-cases` | los casos de prueba ya armados en el manifest (campo `test_cases`, v3): `[{id, name, description?, kind, files:[{key, operation?, target?, width?, enrichment?}]}]`; `[]` si el manifest no define ninguno |

Para el dataset completo (492 archivos, 14 GB) es más práctico `bin/ingest upload` (reanudable);
también sube el manifest al bucket como `dataset/.manifest.json` (objeto interno: no aparece en
`GET /dataset`, que oculta claves que empiezan con `.`). Los campos opcionales de `GET /dataset`
salen de ese manifest (o, si el bucket todavía no lo tiene, del archivo local `DATASET_MANIFEST`,
default `dataset/manifest.json`); el coordinador lo cachea y lo refresca cada 30 s si cambió.

## Monitoreo

### `GET /stats`

Conteo de sub-tareas por estado y, además, los casos abiertos con sus sub-tareas agrupadas por
estado (lo que la consigna pide ver en el monitoreo):

```json
{
  "pending": 86, "assigned": 14, "running": 7, "completed": 1489, "failed": 22, "cancelled": 2,
  "by_case": [
    { "case_id": "16cb3b7e-…", "name": "carga-15-session=documental-s3", "status": "processing",
      "priority": 5, "total": 34, "running": 2, "pending": 2, "completed": 30, "failed": 0 }
  ]
}
```

### `GET /ws` — snapshot para el dashboard

WebSocket que emite cada segundo:

```json
{
  "workers":     [ "…como GET /workers…" ],
  "jobs":        [ "…solo las sub-tareas vivas (pending/assigned/running)…" ],
  "stats":       { "…como GET /stats…" },
  "by_case":     [ "…como stats.by_case…" ],
  "queue_depth": { "high": 0, "normal": 171, "low": 0,
                   "by_pool": { "video": 93, "audio": 65, "metadata": 13 } }
}
```

`queue_depth` es lo que de verdad espera en Redis (entradas no entregadas + entregadas sin
confirmar), por prioridad y por pool.

### `GET /metrics` — Prometheus

Familias `mediacase_*`. Las tres primeras re-exportan el heartbeat de cada worker, porque los
workers remotos no tienen puerto que Prometheus pueda scrapear.

| Métrica | Tipo | Etiquetas | Qué mide |
|---|---|---|---|
| `mediacase_worker_cpu_percent` | gauge | worker, role | CPU del host del worker |
| `mediacase_worker_mem_percent` | gauge | worker, role | memoria usada del host del worker |
| `mediacase_worker_mem_bytes` | gauge | worker, role, kind=used\|total | memoria del host en bytes |
| `mediacase_worker_disk_percent` | gauge | worker, role | uso del disco de trabajo |
| `mediacase_worker_gpu_percent` | gauge | worker, role, gpu, name | uso de cada GPU (como el Administrador de tareas) |
| `mediacase_worker_gpu_vram_bytes` | gauge | worker, role, gpu, name, kind=used | VRAM usada por GPU |
| `mediacase_worker_gpu_temp_celsius` | gauge | worker, role, gpu, name | temperatura por GPU (cuando el driver la da) |
| `mediacase_worker_active_jobs` | gauge | worker, role | sub-tareas en ejecución |
| `mediacase_worker_up` | gauge | worker, role | 1 mientras está registrado y vivo |
| `mediacase_queue_depth` | gauge | pool, priority | sub-tareas esperando worker |
| `mediacase_cases` | gauge | status | casos por estado |
| `mediacase_active_cases` | gauge | — | casos abiertos |
| `mediacase_jobs` | gauge | status, pool | sub-tareas por estado y pool |
| `mediacase_jobs_resolved_total` | counter | status, pool | resueltas (throughput con `rate()`) |
| `mediacase_case_duration_seconds` | histogram | status | creación → cierre del caso |

Prometheus (`infra/prometheus.yml`) scrapea solo `host.docker.internal:8080`; Grafana
(`http://<ip>:3001`, lectura sin login) tiene el dashboard **MediaCase** provisionado desde
`infra/grafana/dashboards/mediacase.json`.

## Conectar otra computadora

| Método y ruta | Qué hace |
|---|---|
| `GET /connect` | página HTML con instrucciones y los enlaces de descarga |
| `GET /download/worker?os=windows\|linux` | ZIP con el binario del worker, ffmpeg (Windows) y un `worker.env` ya apuntando a este coordinador (URL y esquema tomados de `Host` y `X-Forwarded-Proto`: por IP de LAN da `http://`, por túnel da `https://` y MinIO por el túnel que dejó `scripts/tunnel.ps1` en `infra/env/tunnel.env`) |
| `GET /catalog` | operaciones y formatos que acepta el coordinador: `ops_by_type`, `targets_by_op`, `pool_by_op`, `thumbnail_widths`, `extensions`, `identity_excluded_ops`, `ext_aliases`. Lo usa el formulario del dashboard |
| `GET /share` | cómo llegar a este coordinador desde otra máquina: `primary_url` (la IP anunciada a los workers), `lan_urls`, `tunnel` (`status` off/starting/on/error, `coordinator_url`, `minio_url`, `error`, `hint`) y `cloudflared_installed` |
| `POST /tunnel` | abre dos quick tunnels de Cloudflare (coordinador y MinIO) como procesos hijos; responde 202 `starting` y el estado se consulta en `/share`. Si la red bloquea el 7844, en ≤45 s pasa a `error` con la pista de encender WARP |
| `DELETE /tunnel` | cierra los túneles |

## Clientes que ya usan esta API

- **Dashboard** (`dashboard/src/api.js`): casos, reporte, cancelar, `/upload`, `/dataset`, `/ws`.
- **`cmd/client`**: `-case`, `-case-status`, `-stats`, `-batch` (sub-tareas sueltas).
- **`cmd/ingest`**: `upload` (MinIO directo + `dataset/.manifest.json`), `cases` (`POST /cases`
  por grupo, o `--test-cases id1,id2|all` para enviar los del manifest tal cual), `load`
  (`POST /cases` concurrentes + `GET /cases` para el resumen).
- **Scripts de hito** en `tests/`: `curl` + `python` sobre estas rutas.
