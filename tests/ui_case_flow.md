# Hito: todo el flujo de un caso desde el navegador

Guion manual. Cada paso deja una captura en `docs/img/` (evidencia para el informe).
Ejecutado el 2026-09-11 en la laptop de Leno (coordinador nativo, `http://localhost:8080`),
con Chrome automatizado; los pasos 1-2 en la PC de lila se repiten el día de la demo.

| # | Paso | Esperado | Resultado 2026-09-11 |
|---|---|---|---|
| 1 | Abrir `http://<ip-del-coordinador>:8080` desde otra PC | Dashboard, pestaña **Casos** primero | OK (misma laptop; lila el 10/09 por `/connect`) |
| 2 | *Conectar esta PC* → bajar ZIP → `start-worker.bat` | El worker aparece en **Monitor** con su rol | OK (lila, 10/09) |
| 3 | **Casos → + Nuevo caso** → elegir video + audio + imagen del dataset → nombre → Enviar | Caso creado, se abre el detalle con 3 sub-tareas y su pool | OK `caso-completo-ui` |
| 4 | Ver las sub-tareas avanzar | Estado, progreso y worker cambian en vivo (1 s) | OK: video→node1, audio→w-audio, imagen→w-meta |
| 5 | El caso cierra | Badge `completado`, **Reporte consolidado** con resumen y grupos | OK: "de 3 archivos — 1 audio convertido, 1 miniatura generada, 1 video convertido" |
| 6 | Clic en **Descargar** de una sub-tarea | Descarga el resultado desde MinIO | OK (3/3, HTTP 200, tipos mp4/wav/jpeg) |
| 7 | Caso con archivo corrupto | `parcial`, error visible en la fila | OK `prueba-desde-navegador` |
| 8 | Caso en cola → **Cancelar caso** → confirmar | `cancelado`, sub-tareas `cancelada`, reporte "N cancelados" | OK `cancelar-en-cola` |
| 9 | **Monitor** con workers de 3 roles | Tarjetas con rol y pools; colas por pool con la saturación real | OK ("2 en espera · VIDEO 2" tras cancelar sin workers) |
| 10 | Enviar un `.txt` | Mensaje `formato no soportado`, no se crea nada | OK (validación en el navegador y en el servidor) |

Bugs encontrados y corregidos durante el hito: `XLEN` contaba entradas ya procesadas (ahora
`XINFO GROUPS` lag+pending y `XDEL` al confirmar); `ffprobe` ausente se reportaba como "sin stream
de video" (ahora el worker no arranca sin ffmpeg/ffprobe y la sonda propaga el error real).
