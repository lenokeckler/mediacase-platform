# Checklist de la rúbrica

Qué evidencia hay de cada rubro de la consigna v2.0 y dónde está. Escala 0-5 ponderada.

| Peso | Rubro | Qué pide | Evidencia | Dónde |
|---:|---|---|---|---|
| 20 % | **Implementación distribuida** | múltiples nodos reales, cooperación efectiva, no monolítico | node-1 (coordinador + infra) y workers en procesos/máquinas separadas con comunicación por red (HTTP + WebSocket saliente + S3 + Redis). Worker descargable desde `/connect`; probado en una PC ajena (`lila`) el 2026-09-10; VMs de Vagrant (node2/node3) y laptops del equipo en la Fase 7 | `docs/architecture.md` §2, §7, §9 · `docs/informe-pruebas.md` §1, §4, §7 · `tests/distributed_smoke.sh` |
| 20 % | **Gestión de procesos, casos y concurrencia** | descomposición de casos, routing por tipo, colas, ejecución concurrente, barrier/join | `POST /cases` → routing por tipo en el coordinador → N sub-tareas en 9 colas pool × prioridad → pools de goroutines en cada worker → barrier con `SELECT … FOR UPDATE` → 7 estados de caso. 20 casos concurrentes / 412 sub-tareas cerrados sin fallos | `internal/cases/{router,barrier}.go` · `internal/coordinator/scheduler.go` · `docs/architecture.md` §3-§5 · `docs/informe-pruebas.md` §2 · `tests/case_scenario.sh`, `tests/dataset_scenario.sh` |
| 15 % | **Arquitectura del sistema** | diagramas, nodos, flujo de casos y sub-tareas, colas, comunicación | 6 diagramas Mermaid (componentes, secuencia del caso, dos máquinas de estados, colas, topología), tablas de puertos y variables, decisiones justificadas | `docs/architecture.md` |
| 15 % | **Monitoreo y balanceo de recursos** | CPU/memoria/carga, visualizar o reaccionar ante cambios de carga | `/metrics` con CPU/mem/activas por worker (remotos vía heartbeat), colas por pool, casos y sub-tareas por estado; Grafana con 13 paneles; dashboard con colas por pool y sub-tareas agrupadas por caso. Reacción: least-loaded por pool, backpressure (`reject`), expulsión por heartbeat y re-encolado, redistribución probada matando un worker bajo carga | `internal/coordinator/metrics.go` · `infra/grafana/dashboards/mediacase.json` · `docs/img/grafana-*.png` · `docs/informe-pruebas.md` §5.2, §6 · `tests/monitoring_scenario.sh`, `tests/failure_scenario.sh` |
| 10 % | **Procesamiento multimedia distribuido** | conversión, extracción de audio, recursos asociados, workers genéricos o especializados | `convert` (H.264/AAC), `extract_audio` (MP3), `convert_audio` (WAV), `thumbnail` (frame o forma de onda; imágenes); pools especializados `video`/`audio`/`metadata` justificados con la Unidad 1; ffmpeg con prioridad baja para no matar de hambre al coordinador | `internal/multimedia/` · `docs/architecture.md` §6 · `docs/informe-pruebas.md` §3 |
| 10 % | **Gestión de casos, archivos y resultados** | repositorio de resultados, seguimiento de estado, calidad del reporte consolidado | MinIO como repositorio (`dataset/` entradas, `results/jobs/<id>/`, `results/cases/<id>/report.json`), justificado; reporte con archivos por tipo y operación, resultado y error por sub-tarea, tiempos de caso y sub-tarea, worker responsable, resumen agregado; descarga desde el dashboard | `internal/cases/report.go` · `docs/api.md` (`GET /cases/{id}/report`) · `docs/architecture.md` §8 |
| 5 % | **Interfaz / dashboard** | enviar casos, observar por caso y por sub-tarea | pestaña Casos (enviar con subida o dataset, seguir sub-tareas, reporte, cancelar), Monitor (nodos, colas, casos activos, sub-tareas), Historial; una sola URL | `dashboard/` · `docs/manual-usuario.md` §1-§3 · `docs/img/dashboard-*.png` · `tests/ui_case_flow.md` |
| 5 % | **Documentación técnica y manual** | diagramas, despliegue, arquitectura, guía de uso | README, arquitectura, API, manual de usuario (con Windows 11 y diagnóstico), dataset, informe de pruebas, checklist | `README.md` · `docs/` |

## Requisitos duros de la consigna

| Requisito | Estado | Evidencia |
|---|---|---|
| El caso es la unidad de trabajo (no "etiqueta sobre archivos idénticos") | ✅ | routing por tipo dentro del caso, barrier, estado agregado, reporte consolidado |
| Casos homogéneos **y** heterogéneos | ✅ | por construcción en el dataset (`session` s1/s2 vs s3/s4); `ingest cases --group-by session` da 12 y 12 |
| Generación manual y automática de casos | ✅ | dashboard/CLI · `ingest cases --group-by event\|session\|batch\|user\|folder\|type\|tier` |
| Dataset 400-600 archivos, audio + video, formatos y tamaños variados, metadatos | ✅ | 492 archivos, 13 formatos, 3 niveles medidos, `manifest.json` v2; `docs/dataset.md` |
| Estados por sub-tarea (6) y por caso (7); `completed` ⟺ todas OK; `partially_completed` ⟺ alguna falló | ✅ | `internal/models`, `internal/cases/barrier.go`, `docs/architecture.md` §4 |
| Reporte consolidado con los 6 elementos mínimos | ✅ | `docs/api.md` |
| Mínimo 3 nodos worker en entidades separadas, comunicación por red | ✅ tres máquinas con IP propia: host + 2 VMs Vagrant, `HITO OK` 2026-09-11; ⏳ laptops físicas del equipo (7.4) | `docs/informe-pruebas.md` §7 |
| Cliente: envío, consulta por caso y sub-tarea, resultados y reportes, casos concurrentes | ✅ | `cmd/client`, `cmd/ingest load`, dashboard |
| Seis entregables | ✅ arquitectura · repo · sistema · dashboard · docs + manual · informe | este repositorio |

## Pendiente para cerrar (Fase 7)

- [x] 7.1 `vagrant up` (node2 audio, node3 metadata) + `tests/pools_scenario.sh` con node1 host → 3 máquinas. `HITO OK` 2026-09-11.
- [x] 7.2 Túnel Cloudflare y un worker desde otra red. `completed` 3/3 por el túnel, 2026-09-11 (informe §7).
- [ ] 7.3 Binario en la VM Arch.
- [ ] 7.4 Laptops de Jennifer y Jonathan con `tests/pools_scenario.sh` y captura del dashboard con 3 hostnames; agregar a `docs/informe-pruebas.md` §7.
- [ ] 6.6 Un compañero levanta un worker solo con el manual; anotar qué preguntó.
- [ ] Primer `git push` y colaboradores en GitHub.
