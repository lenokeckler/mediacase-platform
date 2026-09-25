---
tipo: I Proyecto Programado
titulo: Manual de usuario
subtitulo: Plataforma distribuida de procesamiento multimedia por casos (MediaCase)
---

# 1. Introducción

MediaCase procesa archivos de audio, video e imagen repartiendo el trabajo entre varias computadoras conectadas en red. Usted entrega un grupo de archivos, el sistema decide qué hacer con cada uno según su contenido, lo reparte entre las máquinas disponibles y, cuando todo termina, le entrega un reporte con el resultado de cada archivo y enlaces para descargar lo producido.

La unidad de trabajo se llama **caso**. Un caso es un conjunto de uno o más archivos que entra como una sola solicitud y cierra con un solo reporte consolidado. Puede ser homogéneo (por ejemplo, doce audios que se convierten a FLAC) o heterogéneo (videos, audios e imágenes mezclados, cada uno con su propia operación). Cada archivo del caso se convierte en una **sub-tarea**. Las sub-tareas corren en paralelo en distintas máquinas, y el caso solo cierra cuando todas terminaron, bien o mal.

Las operaciones disponibles son convertir video, convertir audio, extraer el audio de un video, generar miniaturas, consultar metadatos y enriquecer un archivo con portada, etiquetas y letra o descripción. Todas usan ffmpeg.

El sistema tiene tres tipos de pieza que conviene distinguir desde el inicio:

- **node-1** es la computadora principal. Guarda la base de datos, la cola de trabajo y los archivos, y ejecuta el coordinador, que es el programa que recibe los casos y reparte las sub-tareas. También sirve el dashboard.
- **Los workers** son los programas que hacen el procesamiento. Hay uno en node-1 y se pueden sumar más desde cualquier computadora de la red con un ZIP que se descarga del propio dashboard.
- **El dashboard** es la página web desde la que usted envía casos, los sigue y observa el estado de las máquinas.

Este manual está dirigido a dos lectores. El primero es quien opera node-1: enciende y apaga el sistema, envía casos y lee reportes (secciones 3 a 7). El segundo es quien presta una computadora para procesar: solo necesita la sección 8. La sección 9 cubre el uso por línea de comandos y la 10 reúne los problemas comunes con su solución.

:::figura Figura 1. Vista general del dashboard de MediaCase
Qué debe verse: el dashboard completo en la pestaña Casos, con la barra lateral desplegada (logo, secciones Casos, Monitor e Historial, accesos "Conectar esta PC" y "Compartir", indicador "En vivo" y botón de tema), la lista de casos con varios estados distintos (completado, parcial, procesando) y el botón "+ Nuevo caso" arriba a la derecha.
Cómo obtenerla: encender el sistema con MediaCase.bat, enviar dos o tres casos de prueba desde el formulario y capturar mientras uno sigue procesando.
Tema sugerido: oscuro. Captura existente que sirve como referencia: docs/img/dashboard-casos-carga.png.
:::

\pagebreak

# 2. Requisitos

## 2.1 Computadora principal (node-1)

node-1 necesita Windows 10 u 11 con **Docker Desktop** instalado. Docker ejecuta PostgreSQL (estado de casos y sub-tareas), Redis (la cola de trabajo), MinIO (almacenamiento de archivos de entrada y resultados), Prometheus y Grafana. El coordinador y el worker local no corren en Docker: son programas nativos que el lanzador compila y abre en ventanas propias.

Si la carpeta `bin\` del repositorio ya trae los ejecutables compilados (`mediacase-coordinator.exe` y `mediacase-worker-host.exe`), no hace falta instalar Go. Si Go está instalado, el lanzador vuelve a compilar antes de arrancar, y la primera compilación puede tardar más de un minuto.

Para que otras computadoras se conecten, node-1 debe permitir conexiones entrantes en los puertos 8080 (coordinador y dashboard) y 9000 (MinIO). La regla de firewall se crea una sola vez ejecutando `scripts\firewall-node1.ps1` como administrador. El lanzador avisa si la regla falta.

Para compartir el coordinador fuera de la red local hace falta además `cloudflared`, que se instala con `winget install --id Cloudflare.cloudflared`. Es opcional.

## 2.2 Otras computadoras (workers)

Una computadora que se suma como worker no necesita nada instalado. El ZIP de Windows trae el programa, ffmpeg y un archivo `worker.env` ya configurado con la dirección de node-1. En Linux el ZIP trae el programa y el archivo de configuración; ffmpeg se instala con el gestor de paquetes si falta (`sudo apt install ffmpeg` en Ubuntu, `sudo pacman -S ffmpeg` en Arch). El worker se probó en Windows 11, Ubuntu 24.04 y Arch Linux.

El worker no abre puertos: se conecta hacia el coordinador. Basta con que la computadora llegue a node-1 por la red, sea el mismo WiFi o un túnel (sección 8.6).

## 2.3 Navegador

El dashboard funciona en cualquier navegador actual. Se abre en `http://localhost:8080` desde node-1 y en `http://<ip-de-node-1>:8080` desde otra computadora de la misma red.

Tabla 1. Direcciones del sistema

| Servicio | Dirección | Para qué sirve |
|---|---|---|
| Dashboard y API | `http://<ip-de-node-1>:8080` | Enviar y seguir casos, monitorear nodos |
| Página de conexión | `http://<ip-de-node-1>:8080/connect` | Descargar el worker para otra PC |
| Grafana | `http://<ip-de-node-1>:3001` | Gráficas históricas de carga, sin inicio de sesión |
| Consola de MinIO | `http://localhost:9001` | Revisar archivos guardados (usuario y clave `minioadmin`) |

\pagebreak

# 3. Encender y apagar el sistema

## 3.1 Encender node-1

En la carpeta raíz del repositorio, haga doble clic en **`MediaCase.bat`**. Se abre una consola titulada "MediaCase - node-1" que muestra cada paso con una flecha `==>`:

1. **Docker Desktop.** Si Docker no está corriendo, el lanzador lo abre y espera hasta que responda. La primera vez tarda cerca de un minuto; el límite es de 3 minutos.
2. **Infraestructura.** Levanta los contenedores de Postgres, Redis, MinIO, Prometheus y Grafana con `docker compose`. Los datos de sesiones anteriores se conservan.
3. **Red.** Detecta la dirección IP de la computadora en el WiFi (descarta adaptadores virtuales de VirtualBox, WSL o WARP) y avisa si falta la regla de firewall.
4. **Coordinador y worker local.** Abre dos ventanas minimizadas: una con el coordinador y otra con el worker de node-1 (identificado como `node1`, rol video, 4 sub-tareas a la vez). El lanzador espera hasta 3 minutos a que el coordinador responda, porque primero se compila.
5. **Dashboard.** Abre el navegador en `http://localhost:8080`.

Al final la consola muestra un resumen con la dirección local, la dirección que deben usar otras computadoras de la red (`http://<ip>:8080/connect`) y el recordatorio de cómo apagar. Presione cualquier tecla para cerrar esa consola; el coordinador y el worker siguen corriendo en sus propias ventanas.

> No cierre las dos ventanas minimizadas del coordinador y del worker. Si las cierra, el sistema se detiene. Para apagar, use MediaCase-detener.bat.

:::figura Figura 2. Consola del lanzador MediaCase.bat al terminar
Qué debe verse: la consola de Windows titulada "MediaCase - node-1" con los pasos "==> Docker Desktop", "==> Infraestructura (Postgres, Redis, MinIO, Prometheus, Grafana)", "==> Red", "==> Coordinador y worker local" y "==> Dashboard" en celeste, sus mensajes en verde ("Docker listo", "contenedores arriba", "esta maquina en la red: ...", "coordinador en http://localhost:8080") y el resumen final con "node-1 encendido." y la línea "Para otras PCs (WiFi)".
Cómo obtenerla: con el sistema apagado, doble clic en MediaCase.bat y capturar la consola cuando aparece "Presione una tecla para continuar".
Tema sugerido: el de la consola por defecto.
:::

Si el coordinador ya estaba corriendo, el lanzador no abre otro y lo avisa en amarillo. Lo mismo ocurre con el worker local. Por eso puede ejecutar `MediaCase.bat` de nuevo sin riesgo.

> Si en la computadora está abierto **NVIDIA Broadcast**, ciérrelo antes de encender. Ese programa ocupa el puerto 8080 y el coordinador no puede arrancar.

## 3.2 Apagar node-1

Haga doble clic en **`MediaCase-detener.bat`**. El script cierra el coordinador, el worker local y los túneles que estuvieran abiertos, detiene los contenedores (no los borra: casos, sub-tareas y archivos se conservan) y cierra Docker Desktop para liberar memoria. Al final muestra "node-1 apagado.".

Si prefiere dejar Docker Desktop abierto, ejecute `scripts\stop-node1.ps1 -KeepDocker` desde PowerShell.

Los workers de otras computadoras no se apagan con este script. Mientras node-1 está apagado siguen intentando reconectarse, y cuando node-1 vuelve se conectan solos.

\pagebreak

# 4. Recorrido por el dashboard

## 4.1 Barra lateral

A la izquierda está la barra lateral. Arriba muestra el logo y el nombre "MediaCase", con el subtítulo "Procesamiento por casos". Debajo, bajo el rótulo **Operación**, están las tres secciones:

- **Casos**: enviar casos nuevos, ver la lista y abrir el detalle y el reporte de cada uno. El número a la par indica cuántos casos siguen abiertos.
- **Monitor**: estado de las máquinas, colas por pool, casos activos y sub-tareas en curso. El número indica cuántos nodos hay conectados.
- **Historial**: todas las sub-tareas, también las terminadas.

Bajo el rótulo **Nodos** hay dos accesos. **Conectar esta PC** abre la página `/connect` en una pestaña nueva, y **Compartir** lleva a la tarjeta del Monitor con la dirección para otras computadoras.

Al pie de la barra aparecen tres elementos. El indicador **En vivo** (punto verde) confirma que el navegador recibe el estado del sistema en tiempo real; si dice **Reconectando…**, el coordinador no responde. Debajo se lee el conteo de nodos, por ejemplo "2 nodos · 1 ocupado". El último es el botón de tema, que dice **Tema claro** o **Tema oscuro** según el tema que esté activo.

El botón con la flecha junto al logo pliega la barra y deja solo los iconos, útil en pantallas pequeñas. El navegador recuerda tanto el tema como el estado de la barra.

## 4.2 Direcciones directas

Cada sección tiene su propia dirección, lo que permite guardar marcadores o enviar un enlace a otra persona:

Tabla 2. Direcciones directas dentro del dashboard

| Dirección | Qué abre |
|---|---|
| `http://<ip>:8080/#casos` | La sección Casos |
| `http://<ip>:8080/#monitor` | La sección Monitor |
| `http://<ip>:8080/#historial` | La sección Historial |
| `http://<ip>:8080/#nuevo` | Casos con el formulario de nuevo caso abierto |
| `http://<ip>:8080/#caso=<id>` | El detalle de un caso concreto |
| `http://<ip>:8080/#nodo=<id>` | La vista ampliada de un nodo |
| `?theme=light` o `?theme=dark` | Fuerza el tema claro u oscuro |
| `?sidebar=collapsed` | Abre con la barra lateral plegada |

## 4.3 Sección Casos

La sección muestra la lista de casos del más nuevo al más viejo, con las columnas Caso (nombre y los primeros 8 caracteres del identificador), Estado, Sub-tareas, Prioridad, Creado y Duración. La duración de un caso abierto termina en "…" porque todavía corre.

Los botones de arriba filtran por estado: todos, en cola, procesando, reintentando, completados, parciales, fallidos y cancelados. Un clic en cualquier fila abre el detalle del caso encima de la lista (sección 6).

## 4.4 Sección Historial

Muestra todas las sub-tareas del sistema con sus columnas Sub-tarea, Archivo, Operación, Estado, Progreso, Worker, Prioridad, Creada, Duración y Resultado / error. Los filtros son todas, completadas, fallidas, canceladas, en ejecución, pendientes y asignadas, cada uno con su conteo. El cuadro "Buscar por id, archivo, operación o worker…" filtra por texto, y **↻ Actualizar** vuelve a consultar.

Un clic en una fila despliega el detalle: identificador completo, archivo, caso al que pertenece, inicio, fin, reintentos (hechos y máximo permitido), el error completo si falló y la dirección del resultado si terminó.

:::figura Figura 3. Historial con el detalle de una sub-tarea fallida desplegado
Qué debe verse: la sección Historial con el filtro "fallidas" activo y una fila desplegada que muestra Id completo, Archivo (por ejemplo edge_truncado.mp4), Caso, Inicio, Fin, Reintentos y el Error completo con el motivo de ffmpeg.
Cómo obtenerla: enviar el caso de prueba "Casos límite y fallos", esperar a que cierre, abrir Historial, elegir "fallidas" y hacer clic en la fila de edge_truncado.mp4.
Tema sugerido: claro.
:::

## 4.5 Tema claro y oscuro

Todas las pantallas funcionan en los dos temas. El tema oscuro resulta más cómodo para dejar el Monitor abierto en una pantalla durante una demostración; el claro se lee mejor en capturas impresas.

:::figura Figura 4. El Monitor en tema claro
Qué debe verse: la sección Monitor en tema claro con al menos una tarjeta de nodo, las colas por pool y la tarjeta Compartir.
Cómo obtenerla: abrir http://localhost:8080/?theme=light#monitor.
Captura existente que sirve: docs/img/dashboard-monitor-claro.png.
:::

\pagebreak

# 5. Enviar un caso

En la sección **Casos**, haga clic en **+ Nuevo caso**. Se abre el panel "Nuevo caso" encima de la lista. Para cerrarlo sin enviar, use la ✕ de la esquina.

:::figura Figura 5. Formulario de nuevo caso vacío
Qué debe verse: el panel "Nuevo caso" con los campos "Nombre del caso" (con el texto de ejemplo "p. ej. boda-garcia, sesion-3, lote-2026-09") y "Prioridad", la zona "Subir desde esta PC" con su selector de archivos y la zona "Elegir del dataset (542)" con el buscador, los filtros Tipo, Formato, Tamaño y Origen, los botones de selección masiva y el selector "Cargar caso de prueba". El botón "Enviar caso (0 archivos)" aparece deshabilitado.
Cómo obtenerla: sección Casos, botón "+ Nuevo caso", o abrir http://localhost:8080/#nuevo.
Tema sugerido: oscuro.
:::

## 5.1 Nombre y prioridad

Escriba un **Nombre del caso** que lo identifique, por ejemplo `boda-garcia` o `lote-2026-09`. El nombre aparece en la lista, en el Monitor y en el reporte. Puede dejarlo vacío, pero entonces el caso se muestra como "(sin nombre)".

La **Prioridad** va de 1 a 10. Los valores 8, 9 y 10 aparecen marcados "(alta)" y van a la cola de prioridad alta; los valores 1, 2 y 3 aparecen marcados "(baja)" y van a la cola baja; del 4 al 7 es prioridad normal. El valor inicial es 5. Cuando hay trabajo acumulado, los workers toman primero lo de prioridad alta.

## 5.2 Subir archivos desde su computadora

En la zona **Subir desde esta PC**, use el selector de archivos para elegir uno o varios archivos de video, audio o imagen. El selector solo muestra extensiones que el sistema reconoce:

- Video: mp4, mkv, avi, mov, webm, m4v, flv, wmv, ts, mts, 3gp, mpg, mpeg.
- Audio: mp3, wav, flac, aac, ogg, m4a, opus, wma, aiff, aif, dsf, dff.
- Imagen: jpg, jpeg, png, gif, webp, bmp, tif, tiff.

Los archivos no se suben en ese momento. Se agregan a la lista del caso con la marca "esta PC" y se suben a MinIO cuando usted presiona Enviar. Los nombres conservan tildes, ñ, espacios y paréntesis. Si elige un archivo con una extensión desconocida, el formulario muestra el aviso `"<nombre>": formato no soportado` y no lo agrega.

## 5.3 Elegir archivos del dataset

La zona **Elegir del dataset** lista los archivos que ya están cargados en el sistema. Con el dataset completo son 542: 265 videos, 200 audios y 77 imágenes, en 28 formatos. Cada fila muestra una casilla, el tipo, el nombre, una etiqueta de origen y el tamaño. Si una fila tiene el símbolo ⓘ, al pasar el cursor encima verá una nota sobre ese archivo.

Arriba de la lista, el buscador filtra por nombre y el contador indica, por ejemplo, "46 de 542 · 12 elegidos".

**Filtros.** Hay cuatro grupos de botones y cada botón muestra cuántos archivos tiene:

Tabla 3. Filtros del selector del dataset

| Grupo | Opciones | Uso típico |
|---|---|---|
| Tipo | Video, Audio, Imagen | Armar un caso homogéneo de un solo tipo |
| Formato | Una opción por extensión (MP4, MKV, FLAC, WMA…) | Probar formatos poco comunes |
| Tamaño | Liviano, Mediano, Pesado | Generar carga pesada o una prueba rápida |
| Origen | Real, Sintético, Límite | Real = material con licencia libre; Sintético = generado; Límite = archivos dañados o engañosos a propósito |

Puede activar varias opciones del mismo grupo (por ejemplo MP4 y MKV a la vez) y combinar grupos (Video + Pesado + Real). Un segundo clic desactiva la opción.

**Selección rápida.** Hay tres maneras de marcar archivos sin hacer clic uno por uno:

- **Arrastrar.** Presione el botón del mouse sobre una fila y, sin soltarlo, deslice hacia arriba o hacia abajo. Todas las filas por las que pasa toman el mismo estado. Si la primera fila estaba sin marcar, el arrastre marca; si estaba marcada, desmarca. Un clic simple sin arrastrar solo cambia esa fila.
- **Shift+clic.** Haga clic en una fila y luego Shift+clic en otra: se marca (o desmarca) todo el rango entre las dos.
- **Botones masivos.** **Marcar todo lo filtrado** agrega todas las filas visibles con los filtros actuales. **Desmarcar filtrados** quita solo las visibles. **Limpiar selección** quita todos los archivos del dataset que eligió, sin tocar los que subió desde su PC.

Un procedimiento común para un caso heterogéneo: active el filtro Video y Pesado, presione **Marcar todo lo filtrado**, cambie a Audio y Liviano, marque algunos con arrastre, y así sucesivamente. La selección se conserva al cambiar de filtro.

:::figura Figura 6. Selección por arrastre con filtros activos
Qué debe verse: la zona "Elegir del dataset" con los filtros Tipo "Video" y Origen "Real" activos, el contador mostrando algo como "30 de 542 · 8 elegidos", y un bloque de 6 a 8 filas contiguas marcadas (resaltadas) producto de un arrastre.
Cómo obtenerla: abrir el formulario, activar Video y Real, y arrastrar el mouse sobre las casillas de varias filas seguidas; capturar con el mouse todavía sobre la última fila.
Tema sugerido: oscuro.
:::

## 5.4 Cargar un caso de prueba

El selector **Cargar caso de prueba** ofrece 11 casos armados de antemano. Cada opción muestra el nombre, si es homogéneo o heterogéneo y cuántos archivos tiene. Al elegir uno, sus archivos se agregan a la lista con la operación, el formato de salida y los datos de enriquecimiento ya definidos, y si el campo de nombre estaba vacío toma el nombre del caso de prueba.

Tabla 4. Casos de prueba incluidos

| Caso de prueba | Clase | Qué muestra |
|---|---|---|
| Película abierta en varios formatos | heterogéneo | Un mismo material en varios contenedores y operaciones |
| Álbum clásico enriquecido | homogéneo | Audios con portada, etiquetas y letra |
| Archivo fotográfico NASA | homogéneo | Miniaturas de imágenes reales |
| Podcast y audiolibro | heterogéneo | Voz hablada en varios formatos |
| Formatos raros | heterogéneo | WMV, FLV, 3GP, AIFF, WMA y otros poco comunes |
| Casos límite y fallos | heterogéneo | Archivos dañados y extensiones engañosas; cierra como parcial a propósito |
| Carga de video pesado | homogéneo | Conversiones largas, incluida una 4K a 60 fps |
| Paisajes y sonidos de campo | heterogéneo | Audio de campo e imágenes |
| Cine con recursos asociados | homogéneo | Videos enriquecidos con descripción |
| Transparencia, animación y resoluciones extremas | homogéneo | PNG y WebP con transparencia, GIF animado, imágenes enormes |
| Caso mixto grande (40 archivos) | heterogéneo | Reparto entre varias máquinas |

Después de cargarlo puede modificar cualquier fila o agregar más archivos antes de enviar.

:::figura Figura 7. Caso de prueba cargado en el formulario
Qué debe verse: el formulario con el nombre "Película abierta en varios formatos" ya escrito y la lista "Archivos del caso (12)" agrupada en videos, audios e imágenes, cada fila con su operación y formato de salida (por ejemplo "mkv → MP4", "mp4 → MP3").
Cómo obtenerla: en el formulario, "Cargar caso de prueba" → "Película abierta en varios formatos · heterogéneo · 12 archivos".
Tema sugerido: oscuro.
:::

## 5.5 Operación y formato de salida por archivo

Debajo de las zonas de selección aparece **Archivos del caso (N)**. Los archivos están agrupados por tipo (videos, audios, imágenes) y cada fila tiene cuatro columnas: Archivo, Operación, Salida y el botón ✕ para quitarlo.

No hace falta tocar nada. El coordinador inspecciona cada archivo y elige la operación que corresponde a su tipo. En el selector de operación esa opción aparece con la marca "(automática)", y en el de salida el formato elegido aparece con "(automático)". Si pasa el cursor sobre una fila, verá una descripción de la operación.

Si quiere otra cosa, cambie la operación o el formato en esa fila. Las opciones dependen del tipo de archivo:

Tabla 5. Operaciones disponibles por tipo de archivo

| Tipo | Operación | Formatos de salida | Qué produce |
|---|---|---|---|
| Video | convertir video (automática) | MP4, MKV, WebM | El video recodificado (H.264 en MP4 y MKV, VP9 en WebM) |
| Video | extraer audio | MP3, WAV, FLAC, AAC | Solo la pista de audio |
| Video | miniatura | JPG, PNG, WebP (320, 640 o 1280 px) | Una imagen del primer fotograma |
| Video | metadatos | JSON | Duración, códecs, resolución, bitrate y etiquetas |
| Video | enriquecer | MP4, MKV | El mismo video con portada, etiquetas y descripción |
| Audio | convertir audio (automática) | FLAC, MP3, WAV, AAC, OGG | El audio en otro formato |
| Audio | miniatura | JPG, PNG, WebP | Una imagen con la forma de onda |
| Audio | metadatos | JSON | Duración, códec, canales, muestreo y etiquetas |
| Audio | enriquecer | MP3, FLAC, OGG, M4A | El mismo audio con portada, etiquetas y letra |
| Imagen | miniatura (automática) | JPG, PNG, WebP | La imagen reducida al ancho elegido |
| Imagen | metadatos | JSON | Dimensiones, formato y datos técnicos |

**No se convierte al mismo formato.** En convertir video y convertir audio, el formato que el archivo ya tiene no aparece en la lista: un `.mp4` ofrece MKV y WebM, y un `.flac` ofrece MP3, WAV, AAC y OGG. El sistema trata como iguales las variantes de un mismo formato (jpeg y jpg, tiff y tif, aiff y aif, m4a y aac, mpeg y mpg). La miniatura sí permite repetir el formato (`png → PNG`), porque ahí cambia el tamaño.

**Ancho de la miniatura.** Cuando la operación es miniatura aparece un tercer selector con 320 px (el valor por omisión), 640 px o 1280 px.

**Aplicar a todo un tipo.** Cuando un grupo tiene más de un archivo, en su encabezado aparece el botón **Aplicar a todos los videos**, **Aplicar a todos los audios** o **Aplicar a todas las imágenes**. El botón copia la operación, el formato de salida y el ancho del **primer** archivo del grupo a los demás. Si ese formato no sirve para algún archivo (por ejemplo, copiar "MKV" a un archivo que ya es `.mkv`), ese archivo vuelve al formato automático.

:::figura Figura 8. Archivos del caso agrupados por tipo con operaciones distintas
Qué debe verse: la lista "Archivos del caso" con tres grupos (videos, audios, imágenes), el botón "Aplicar a todos los videos" en el encabezado del grupo de videos, una fila de video con "extraer audio" y salida "mp4 → FLAC", y una fila de imagen con "miniatura", salida WebP y el selector de ancho en 640 px.
Cómo obtenerla: elegir tres videos, dos audios y dos imágenes del dataset; cambiar la operación del primer video a "extraer audio" y la de una imagen a WebP con 640 px.
Tema sugerido: claro.
:::

## 5.6 Enriquecer un audio o un video

La operación **enriquecer** integra recursos asociados dentro del mismo archivo, sin producir un archivo aparte. Al elegirla, debajo de la fila se despliega el recuadro **Recursos asociados** con una línea que explica qué va a pasar: la portada que se genera (la forma de onda en audio, el fotograma del segundo 1 en video) y si el archivo conserva su formato sin recodificar o se re-empaqueta a otro.

Los campos son:

Tabla 6. Campos del editor de enriquecimiento

| Campo en audio | Campo en video | Qué guarda | Si lo deja vacío |
|---|---|---|---|
| Título | Título | Etiqueta de título | El nombre del archivo sin extensión, con espacios en lugar de guiones |
| Artista | Autor | Etiqueta de artista | Queda sin esa etiqueta |
| Álbum / evento | Evento / serie | Etiqueta de álbum | El nombre del caso |
| Fecha | Fecha | Etiqueta de fecha | Queda sin esa etiqueta; el año en gris es solo una sugerencia |
| Comentario | Comentario | Etiqueta de comentario | Queda sin esa etiqueta |
| Letra | Descripción | Letra (audio) o descripción (video), admite varias líneas | Queda sin ese texto |

En Título y Álbum, el texto gris muestra el valor que el sistema pondrá si usted no escribe nada; en los demás campos es solo un ejemplo. La portada siempre se genera, en JPEG de 640 px.

Si hay dos o más archivos con enriquecer, el recuadro muestra el botón **Aplicar a todos**, que copia artista, álbum y fecha a los demás archivos enriquecidos del caso. El título, el comentario y la letra no se copian porque normalmente son distintos en cada archivo.

Formatos que admiten enriquecimiento: mp3, flac, ogg y m4a en audio; mp4 y mkv en video. Si el archivo ya está en uno de ellos, conserva el formato. Si no (por ejemplo un `.wav` o un `.avi`), se re-empaqueta al primer formato de la lista y usted puede elegir otro. Un `.mov` se enriquece a MP4, porque el formato QuickTime no guarda portada ni descripción con ffmpeg.

La letra que usted escriba queda en la etiqueta de letras del archivo, que leen reproductores como VLC o foobar2000 y la mayoría de los teléfonos.

:::figura Figura 9. Editor de recursos asociados desplegado
Qué debe verse: una fila de audio (por ejemplo un .flac) con la operación "enriquecer" y, debajo, el recuadro "Recursos asociados" con el subtítulo "Se integran dentro del audio · portada con la forma de onda · se conserva el FLAC sin recodificar", los campos Título, Artista, Álbum / evento, Fecha, Comentario y Letra (con algunas líneas de letra escritas) y el botón "Aplicar a todos" en la esquina.
Cómo obtenerla: elegir dos audios FLAC del dataset, cambiar ambos a "enriquecer", llenar Artista y Letra en el primero.
Tema sugerido: oscuro.
:::

## 5.7 Enviar

Revise la lista y presione **Enviar caso (N archivos)**. Mientras se suben los archivos locales y se crea el caso, el botón dice "Enviando…". Al terminar, el formulario se cierra y el detalle del caso nuevo se abre solo.

El caso se acepta entero o se rechaza entero. Si algún archivo no sirve (una extensión desconocida, un formato de salida inválido para ese archivo), aparece un mensaje en rojo que dice cuál es el problema y ningún archivo del caso se encola. Corrija esa fila y vuelva a enviar.

Al recibir el caso, el coordinador lee los primeros 64 KB de cada archivo para confirmar qué contiene de verdad. Si la extensión no coincide con el contenido (un MP3 renombrado como `.mp4`, por ejemplo), procesa el archivo según su contenido real y deja un aviso en la sub-tarea (sección 6.3).

\pagebreak

# 6. Seguir un caso y leer el reporte

## 6.1 Detalle del caso

Haga clic en un caso de la lista para abrir su detalle. En la cabecera verá el nombre, el estado del caso, el identificador completo y una línea con la prioridad, cuántas sub-tareas van resueltas (por ejemplo "7/12 resueltas"), la hora de creación, de inicio y de cierre, y la duración. A la derecha están los botones **Cancelar caso** (mientras el caso está abierto), **Reporte (JSON)** (cuando ya cerró) y **Cerrar**.

Debajo está la tabla de sub-tareas con las columnas Archivo, Tipo, Operación, Pool, Estado, Progreso, Worker, Inicio, Duración y Resultado. La tabla se actualiza sola mientras el caso está abierto. La columna Operación muestra el cambio de formato (por ejemplo `mkv → MP4`) seguido del nombre de la operación. La barra de progreso avanza según lo que informa ffmpeg: es índigo mientras corre, verde al completar y roja si falla.

:::figura Figura 10. Detalle de un caso heterogéneo en ejecución
Qué debe verse: el detalle de un caso en estado "procesando", con la línea "sub-tareas 5/12 resueltas", varias sub-tareas en "en ejecución" con barras de progreso a distintos porcentajes, otras "completada" con el enlace "Descargar" y otras "pendiente". Debe distinguirse la columna Pool (video, audio, metadata) y la columna Worker.
Cómo obtenerla: enviar el caso de prueba "Película abierta en varios formatos" y capturar a los 30 o 40 segundos.
Tema sugerido: oscuro.
:::

## 6.2 Estados

Un caso y sus sub-tareas tienen estados distintos. El estado del caso resume el de sus sub-tareas, y solo se decide como terminado cuando el coordinador tiene el resultado de todas.

Tabla 7. Estados de un caso

| Estado en el dashboard | Qué significa |
|---|---|
| en cola | El caso fue aceptado y ninguna sub-tarea ha empezado |
| procesando | Al menos una sub-tarea está asignada o en ejecución |
| reintentando | Un worker se cayó a mitad de trabajo y sus sub-tareas volvieron a la cola |
| completado | Todas las sub-tareas terminaron bien |
| parcial | Todas terminaron, pero al menos una falló |
| fallido | Todas las sub-tareas fallaron |
| cancelado | Usted canceló el caso antes de que terminara |

Tabla 8. Estados de una sub-tarea

| Estado en el dashboard | Qué significa |
|---|---|
| pendiente | En la cola, esperando un worker |
| asignada | Entregada a un worker que todavía no empieza |
| en ejecución | ffmpeg está trabajando; la barra muestra el avance |
| completada | Terminó y su resultado está guardado |
| fallida | No se pudo procesar; la columna Resultado muestra el motivo |
| cancelada | El caso se canceló antes de que empezara |

Un caso **parcial** no es un caso perdido. Las sub-tareas que sí terminaron tienen su resultado disponible para descargar; solo las fallidas quedan sin resultado.

## 6.3 Chips de la tabla

Junto a algunos valores de la tabla aparecen etiquetas pequeñas:

- **ayuda** (amarilla, en la columna Worker): la sub-tarea la procesó una máquina de otro pool porque estaba libre. Por ejemplo, un nodo dedicado a video que convierte un audio mientras el nodo de audio está ocupado.
- **⚠ aviso** (amarilla, en la columna Operación): el coordinador dejó una nota sobre cómo enrutó ese archivo. Al pasar el cursor encima se lee la nota completa. Lo más común es una extensión engañosa, por ejemplo "extensión engañosa: .mp4 sugiere video, pero el contenido real es audio (mp3); se enrutó por el contenido real".
- **portada**, **N etiquetas** y **letra** o **descripción** (en sub-tareas de enriquecer): resumen de lo que se integró en el archivo. El cursor encima muestra los valores.

:::figura Figura 11. Detalle del caso "Casos límite y fallos" con avisos y fallos
Qué debe verse: el detalle del caso en estado "parcial", con el chip "⚠ aviso" en las filas de los archivos con extensión engañosa (mp3 con .mp4, mkv con .mp4, png con .jpg) y el recuadro de la nota visible al pasar el cursor sobre uno de ellos; también las tres filas "fallida" con su motivo en rojo (moov atom not found, archivo vacío (0 bytes), Invalid data found when processing input).
Cómo obtenerla: enviar el caso de prueba "Casos límite y fallos" (tarda unos 17 s), abrir su detalle y dejar el cursor sobre un chip "⚠ aviso".
Tema sugerido: claro.
:::

## 6.4 Reporte consolidado

Cuando el caso cierra, arriba de la tabla aparece el recuadro **Reporte consolidado**. El color del recuadro indica cómo terminó: normal si se completó, ámbar si es parcial y gris si se canceló o falló. Contiene:

- Una línea de resumen que parte del total de archivos y cuenta qué se hizo con ellos: cuántos videos se convirtieron, cuántos audios se enriquecieron, cuántos archivos tenían extensión engañosa y cuántos fallaron, con sus motivos. Cuando un mismo motivo se repite, el resumen lo agrupa (por ejemplo, "2 × moov atom not found").
- Un bloque por cada combinación de tipo, operación y formato, con cuántas sub-tareas terminaron bien y cuántas fallaron, por ejemplo "video · convertir video → MP4: 7 ok".
- La duración total del caso.

La tabla de sub-tareas sigue debajo con el inicio, la duración y el worker de cada una. El botón **Reporte (JSON)** abre el reporte completo en una pestaña nueva, con los tiempos exactos de inicio y fin del caso y de cada sub-tarea, el worker responsable, el resultado o el error de cada archivo y los archivos agrupados por tipo y operación. El mismo reporte queda guardado en MinIO en `results/cases/<id>/report.json`.

:::figura Figura 12. Reporte consolidado de un caso completado
Qué debe verse: el detalle de un caso en estado "completado" con el recuadro "Reporte consolidado", su línea de resumen, los bloques por tipo y operación (por ejemplo "audio · enriquecer → FLAC: 12 ok") y "duración total"; debajo, la tabla con todas las sub-tareas "completada" y el enlace "Descargar" en cada una.
Cómo obtenerla: enviar el caso de prueba "Álbum clásico enriquecido" (unos 48 s) y abrirlo al cerrar.
Tema sugerido: oscuro.
:::

## 6.5 Descargar resultados

Cada sub-tarea completada tiene el enlace **Descargar** en la columna Resultado. El enlace apunta a MinIO y descarga el archivo producido: el video convertido, el audio extraído, la miniatura, el JSON de metadatos o el archivo enriquecido. Los resultados de un caso se guardan juntos, en la carpeta `results/cases/<id>/` de MinIO.

Si una sub-tarea falló, en lugar del enlace aparece el motivo en rojo. El mensaje incluye las últimas líneas de diagnóstico de ffmpeg, lo que suele bastar para saber si el archivo está dañado o tiene un formato que no corresponde.

## 6.6 Cancelar un caso

Mientras el caso está abierto, presione **Cancelar caso**. El botón cambia a "¿Seguro? Clic otra vez para cancelar"; un segundo clic confirma. Si hace clic en otra parte, la cancelación se descarta.

Al cancelar, las sub-tareas que no habían empezado quedan canceladas. Las que ya estaban corriendo terminan su trabajo, porque ffmpeg no se interrumpe a medias. El caso queda **cancelado** de inmediato y se genera su reporte con lo que alcanzó a hacerse; ese reporte no se actualiza si alguna sub-tarea termina después.

\pagebreak

# 7. Monitor

La sección **Monitor** muestra el estado de todo el sistema. El botón **↻ Limpiar** de arriba a la derecha refresca la vista de sub-tareas en curso. De arriba hacia abajo:

## 7.1 Resumen

Cinco contadores con las sub-tareas de todo el sistema por estado: Pendientes, Asignadas, En ejecución, Completadas y Fallidas.

## 7.2 Nodos y rendimiento

Hay una tarjeta por cada máquina conectada, parecida a la pestaña Rendimiento del Administrador de tareas de Windows. Cada tarjeta muestra:

- El nombre del nodo, con un punto verde si está libre o índigo si está ocupado, y el nombre del equipo si es distinto.
- Chips de color con su pool principal (video, audio o metadata; un nodo con rol "Todo" muestra los tres) y el estado **libre** u **ocupado**.
- El sistema operativo, la leyenda "ayuda en ..." con los pools que atiende cuando está libre y hace cuánto se reportó ("visto hace 2 s").
- Un bloque de **CPU** con el modelo del procesador, núcleos e hilos, el porcentaje de uso y una gráfica de los últimos 60 segundos.
- Un bloque de **Memoria** con la memoria instalada, la usada y el porcentaje.
- Un bloque por cada **GPU** (la integrada y la dedicada, si hay dos) con su nombre, porcentaje, VRAM usada y temperatura cuando el equipo la informa.
- Al pie, **"N de M cupos ocupados"** y el porcentaje de disco.

Los **cupos** son cuántas sub-tareas procesa ese nodo a la vez. El worker los calcula al arrancar según su hardware: un cupo por cada 2 hilos del procesador y por cada 2 GB de RAM aproximadamente, lo que alcance primero, entre 1 y 8. Una laptop de 12 hilos y 15 GB tiene 6 cupos; una máquina virtual de 2 hilos tiene 1. El worker de node-1 tiene 4 fijos, porque comparte la máquina con la base de datos, la cola y el almacenamiento. El coordinador reparte el trabajo en proporción a los cupos libres de cada nodo, de modo que una máquina más grande recibe más.

Lo que una máquina no puede medir aparece como "no disponible", nunca como un cero. Las GPU se muestran como información del equipo: la codificación de video se hace en el procesador (x264), no en la tarjeta gráfica.

Un clic en una tarjeta abre la **vista ampliada del nodo**: gráficas grandes de los últimos 5 minutos con promedio y máximo, los datos completos del equipo (sistema, procesador, memoria, disco, pools, "Capacidad: N sub-tareas a la vez") y la tabla "Sub-tareas en este nodo" con su archivo, operación, estado, progreso y si llegó por afinidad o por ayuda. Se cierra con **Cerrar (Esc)**, con la tecla Esc o con un clic afuera.

:::figura Figura 13. Tarjetas de nodo con cupos ocupados
Qué debe verse: la sección "Nodos y rendimiento" con al menos dos tarjetas en estado "ocupado", cada una con su CPU arriba de 50 %, memoria, al menos una GPU, y al pie "3 de 4 cupos ocupados" en node1 y "4 de 6 cupos ocupados" en otra PC.
Cómo obtenerla: conectar una segunda PC con el ZIP de /connect, enviar el caso de prueba "Caso mixto grande (40 archivos)" y capturar a los 20 o 30 segundos.
Captura existente que sirve como referencia: docs/img/monitor-3-nodos-vagrant.png (tres nodos, anterior a los cupos) y docs/img/dashboard-nodo-rendimiento.png (una tarjeta con dos GPU).
Tema sugerido: oscuro.
:::

:::figura Figura 14. Vista ampliada de un nodo
Qué debe verse: el recuadro modal de node1 con los datos Sistema, Procesador, Memoria, Disco de trabajo, Pools, Sub-tareas activas y "Capacidad: 4 sub-tareas a la vez", las gráficas grandes de CPU y Memoria con los ejes "-5 min" a "ahora", y la tabla "Sub-tareas en este nodo" con al menos dos filas, una de ellas con el chip "ayuda".
Cómo obtenerla: durante un caso grande, clic en la tarjeta de node1 en el Monitor.
Captura existente que sirve como referencia: docs/img/dashboard-monitor-rendimiento.png.
Tema sugerido: oscuro.
:::

## 7.3 Colas por pool

La tarjeta **Colas por pool** muestra cuántas sub-tareas esperan un worker de cada pool (video, audio y metadata) con una barra para cada uno, el total "en espera" y el desglose por prioridad (alta, normal, baja). El chip de carga dice **carga normal** hasta 200 sub-tareas en espera, **carga alta** por encima de 200 y **carga crítica** por encima de 1000.

Esta tarjeta sirve para decidir si hace falta otra máquina. Si la barra de video crece mientras audio y metadata están en cero, el trabajo pendiente es de video: conecte otra PC con el rol Video (sección 8).

## 7.4 Compartir este coordinador

La tarjeta **Compartir este coordinador** tiene dos filas. **Mismo WiFi** muestra la dirección `http://<ip>:8080/connect` con el botón **Copiar**; si la computadora tiene varias interfaces de red, debajo aparecen las demás. **Otra red (túnel)** muestra el estado del túnel (cerrado, abriendo…, abierto o error) y el botón **Publicar en internet** (sección 8.6).

## 7.5 Casos activos y sub-tareas en curso

**Casos activos** muestra una barra por cada caso abierto con sus sub-tareas por color: verde listas, índigo en ejecución, ámbar en espera y rojo fallidas, más el conteo en texto. Un clic en un caso lo abre en la sección Casos. **Sub-tareas en curso** lista lo pendiente, asignado o en ejecución, con archivo, pool, worker y progreso.

:::figura Figura 15. Colas por pool y casos activos bajo carga
Qué debe verse: la tarjeta "Colas por pool" con la barra de video mayor que las demás y el chip de carga, y debajo "Casos activos" con tres o más casos y sus barras de colores.
Cómo obtenerla: ejecutar bin\ingest load --cases 20 --concurrency 5 --group-by session y capturar el Monitor a los 30 segundos.
Captura existente que sirve: docs/img/dashboard-monitor-casos-activos.png.
Tema sugerido: oscuro.
:::

\pagebreak

# 8. Conectar otra computadora como worker

Cualquier computadora que llegue a node-1 por la red puede sumarse a procesar. No hay que instalar nada ni abrir puertos.

## 8.1 Abrir la página de conexión

En la computadora que va a sumarse, abra en el navegador la dirección que muestra la tarjeta Compartir, `http://<ip-de-node-1>:8080/connect`. También puede llegar desde el dashboard con **Conectar esta PC** en la barra lateral.

La página se titula "Conectar esta PC como worker" y explica en una línea el procedimiento: descargar, descomprimir y ejecutar.

## 8.2 Elegir qué va a procesar la PC

En el recuadro **¿Qué va a procesar esta PC?** elija una opción:

Tabla 9. Roles disponibles en la página de conexión

| Opción | Pool | Cuándo elegirla |
|---|---|---|
| Todo (recomendado) | los tres | Si no sabe qué elegir; la PC toma lo que haga falta |
| Video | video | Para la máquina más potente: conversiones pesadas y 4K |
| Audio | audio | Conversión y extracción de audio |
| Imágenes y metadatos | metadata | Miniaturas, metadatos y enriquecer |

Elegir un tipo no deja la PC ociosa cuando no hay trabajo de ese tipo. El planificador da preferencia a la máquina del pool que corresponde (afinidad), pero si esa máquina ya está a la mitad de su capacidad y otra está más libre, la segunda ayuda aunque sea de otro pool. Esas sub-tareas aparecen con el chip "ayuda".

No tiene que indicar cuántas sub-tareas procesa a la vez: el worker lo calcula solo según sus núcleos y su RAM.

## 8.3 Descargar y ejecutar en Windows

1. Presione **Descargar para Windows**. Se descarga un ZIP de unos 85 MB con el worker, ffmpeg y el archivo `worker.env`.
2. Antes de descomprimir, haga clic derecho en el ZIP, elija **Propiedades**, marque **Desbloquear** y presione **Aceptar**. Así Windows no trata el programa como "archivo descargado de internet" y no pide permisos de administrador.
3. Descomprima el ZIP en cualquier carpeta.
4. Haga doble clic en `start-worker.bat`.

Se abre una ventana de consola. En las primeras líneas verá "=== MediaCase Worker ===" y una línea con el nombre del worker, el rol, la capacidad y su origen (por ejemplo "capacidad=6 (según el hardware)") y la dirección del coordinador. Después aparecen "[register] registrado como ..." y "[stream] canal abierto con ...". A partir de ahí, cada sub-tarea que llega deja líneas "[assign] job ... aceptado", "[job ...] inicio" y "[job ...] COMPLETADO".

En unos segundos la máquina aparece en el Monitor de node-1, con el nombre de la computadora, y empieza a recibir trabajo.

:::figura Figura 16. Página de conexión /connect
Qué debe verse: la página "Conectar esta PC como worker" con el logo, el recuadro "¿Qué va a procesar esta PC?" con las cuatro opciones (Todo marcada), los botones "Descargar para Windows" y "Descargar para Linux", la lista de tres pasos y la nota final con la dirección del coordinador.
Cómo obtenerla: abrir http://<ip-de-node-1>:8080/connect desde otra computadora de la red.
Tema sugerido: el único disponible (la página no tiene tema oscuro).
:::

:::figura Figura 17. Ventana del worker en Windows conectado y procesando
Qué debe verse: la consola de start-worker.bat con las líneas "=== MediaCase Worker ===", "ID=... | rol=all ... | capacidad=6 (según el hardware) | coordinator=http://...:8080", "[register] registrado como ...", "[stream] canal abierto con ..." y varias líneas "[assign] job ... aceptado" y "[job ...] COMPLETADO".
Cómo obtenerla: en una segunda PC, descargar el ZIP de /connect, desbloquearlo, descomprimirlo, doble clic en start-worker.bat y enviar un caso desde node-1.
Tema sugerido: el de la consola por defecto.
:::

:::figura Figura 18. Opción Desbloquear en las propiedades del ZIP
Qué debe verse: la ventana Propiedades del ZIP descargado, pestaña General, con la sección Seguridad y la casilla "Desbloquear" marcada.
Cómo obtenerla: clic derecho en el ZIP recién descargado → Propiedades.
Tema sugerido: el de Windows por defecto.
:::

### Smart App Control

Algunas instalaciones nuevas de Windows 11 traen activo **Smart App Control**, que bloquea cualquier programa sin firma digital y no ofrece la opción "ejecutar de todos modos". El worker no tiene firma (un certificado de firma de código tiene costo y queda fuera del alcance del proyecto). Si la ventana no abre y Windows muestra un aviso de Smart App Control, hay dos salidas:

- Apagarlo en **Seguridad de Windows → Control de aplicaciones y navegador → Smart App Control → Desactivado**. Tenga presente que es **irreversible**: solo se vuelve a activar reinstalando Windows.
- Usar otra computadora, una máquina virtual o Linux.

## 8.4 Descargar y ejecutar en Linux

1. Presione **Descargar para Linux** y descomprima el ZIP.
2. Si ffmpeg no está instalado, instálelo: `sudo apt install ffmpeg` (Ubuntu, Debian) o `sudo pacman -S ffmpeg` (Arch).
3. En una terminal, dentro de la carpeta, ejecute `bash start-worker.sh`.

La salida es la misma que en Windows. Deje la terminal abierta.

## 8.5 Cambiar la configuración a mano

El archivo `worker.env` que viene en el ZIP ya está listo. Si quiere cambiar algo, edítelo con un editor de texto antes de arrancar el worker:

```
WORKER_ROLE=audio        # video | audio | metadata | all
WORKER_POOL_SIZE=auto    # auto = según núcleos y RAM; un número la fija
WORKER_ID=laptop-jenn    # nombre en el dashboard; vacío = nombre de la máquina
```

Use un número en `WORKER_POOL_SIZE` solo si quiere reservar recursos de la computadora para otra cosa; por ejemplo, `2` en una laptop que también se usa para trabajar.

## 8.6 Conectar desde otra red (túnel)

Si la computadora que se quiere sumar no está en la misma red que node-1, en el dashboard de node-1 vaya a **Monitor → Compartir este coordinador** y presione **Publicar en internet**. El chip pasa a "abriendo…" y el mensaje dice "conectando con Cloudflare (hasta 45 s)…". Cuando el chip dice "abierto", aparece una dirección `https://....trycloudflare.com/connect` con el botón **Copiar**. Envíe esa dirección a la otra persona; los pasos son los mismos de las secciones 8.1 a 8.4.

El ZIP descargado por el túnel ya viene configurado para usarlo, sin nada que editar. La dirección cambia cada vez que se abre el túnel, así que el ZIP debe descargarse con el túnel ya abierto. Para cerrarlo, presione **Cerrar túnel**; también se cierra al apagar node-1.

Algunas redes bloquean la salida hacia Cloudflare. El WiFi del TEC es una de ellas. En ese caso el chip pasa a "error" y la tarjeta muestra el motivo y una sugerencia: encienda **Cloudflare WARP** (o una VPN) en node-1 y vuelva a intentar. Desde una red doméstica o con datos del celular funciona directo. Si falta `cloudflared` en node-1, el botón aparece deshabilitado y la tarjeta muestra el comando para instalarlo.

:::figura Figura 19. Tarjeta Compartir con el túnel abierto
Qué debe verse: la tarjeta "Compartir este coordinador" con la fila "Mismo WiFi" y su dirección, y la fila "Otra red (túnel)" con el chip "abierto", la dirección https://....trycloudflare.com/connect, el botón "Copiar" y el botón rojo "Cerrar túnel".
Cómo obtenerla: Monitor → "Publicar en internet" desde una red que no bloquee Cloudflare, o con WARP encendido.
Captura existente que sirve: docs/img/dashboard-compartir-tunel.png (y docs/img/dashboard-compartir-tunel-bloqueado.png para el caso de error).
:::

## 8.7 Desconectar una computadora

Cierre la ventana del worker, o presione Ctrl+C dentro de ella. El worker avisa al coordinador antes de salir ("coordinador avisado; sus sub-tareas vuelven a la cola"), y las sub-tareas que tenía en curso vuelven a la cola en el mismo segundo para que otro nodo las tome. El caso afectado pasa a **reintentando** y sigue normalmente.

Si la computadora se apaga de golpe o pierde la red, el coordinador lo detecta al dejar de recibir sus reportes periódicos y hace lo mismo. Si la PC vuelve a la red, el worker se reconecta solo; no hace falta ejecutarlo de nuevo.

\pagebreak

# 9. Uso sin navegador

Los casos también se pueden enviar desde una terminal en node-1, dentro de la carpeta del repositorio y con el coordinador encendido. Sirve para cargar muchos casos a la vez o para automatizar pruebas.

## 9.1 Programa ingest

`bin\ingest` (en Linux, `bin/ingest`) tiene tres modos.

**Subir el dataset a MinIO.** Solo hace falta la primera vez o después de regenerar los archivos:

```
bin\ingest upload --dir dataset/files --manifest dataset/manifest.json --concurrency 4
```

**Crear casos automáticamente.** El programa lee la lista de archivos del dataset, los agrupa según un criterio y envía un caso por grupo:

```
bin\ingest cases --group-by session --limit 10
bin\ingest cases --group-by event --only heterogeneous --dry-run
bin\ingest cases --group-by user --enrich
```

Tabla 10. Opciones de ingest cases

| Opción | Valores | Efecto |
|---|---|---|
| `--group-by` | event, session, batch, user, folder, type, tier | Criterio de agrupación: evento, sesión, lote de ingesta, usuario, carpeta, tipo o tamaño |
| `--only` | homogeneous, heterogeneous | Envía solo los casos homogéneos o solo los heterogéneos |
| `--limit` | número | Máximo de casos a crear (0 = todos) |
| `--priority` | 1 a 10 | Prioridad de los casos (5 por omisión) |
| `--dry-run` | | Muestra la agrupación sin crear nada |
| `--enrich` | | Audios y videos se envían como enriquecer, con usuario como artista y evento como álbum |
| `--test-cases` | all, o ids separados por coma | Envía los casos de prueba en lugar de agrupar |
| `--coordinator` | URL | Otro coordinador (por omisión `http://localhost:8080`) |

Con `--dry-run` la salida es una tabla con el nombre de cada caso, la cantidad de archivos, los tipos y si es homogéneo o heterogéneo.

**Enviar los casos de prueba:**

```
bin\ingest cases --test-cases all
bin\ingest cases --test-cases tc06-casos-limite,tc11-mixto-grande
```

**Generar carga.** Envía muchos casos concurrentes para observar saturación y reparto:

```
bin\ingest load --cases 20 --concurrency 5 --group-by session --wait
```

`--cases` es la cantidad de casos, `--concurrency` cuántos envíos simultáneos, y `--wait` espera a que todos cierren e imprime un resumen por estado. Si pide más casos que grupos, los grupos se repiten.

## 9.2 Programa client

El cliente de línea de comandos envía un caso con archivos que ya están en MinIO y sigue su avance:

```
go run ./cmd/client -case -name "prueba" -files "video_light_1.mp4,audio_light_2.flac" -watch
go run ./cmd/client -case-status <id>
```

En `-files` puede forzar la operación de un archivo con dos puntos, por ejemplo `pelicula.mp4:extract_audio`. `-watch` sigue el caso hasta que cierra, y `-case-status` sigue un caso existente e imprime su reporte al final.

## 9.3 Enviar por la API desde PowerShell

Si arma el JSON a mano en PowerShell y los nombres llevan tildes o ñ, envíelo en UTF-8. El coordinador rechaza con error 400 un cuerpo en otra codificación, y el mensaje de error explica cómo corregirlo. La referencia completa de la API está en `docs/api.md`.

\pagebreak

# 10. Solución de problemas

Tabla 11. Problemas frecuentes

| Síntoma | Causa probable | Qué hacer |
|---|---|---|
| El lanzador dice que el coordinador no respondió en 3 minutos | Otro programa ocupa el puerto 8080, normalmente NVIDIA Broadcast | Cierre NVIDIA Broadcast, ejecute MediaCase-detener.bat y luego MediaCase.bat. Si persiste, abra la ventana minimizada del coordinador y lea el error |
| "Docker Desktop no respondio en 3 minutos" | Docker Desktop no terminó de iniciar o falló | Abra Docker Desktop a mano, espere a que diga que está corriendo y ejecute MediaCase.bat de nuevo |
| "No encuentro Docker Desktop" | Docker Desktop no está instalado en la ruta estándar | Instale Docker Desktop |
| El dashboard dice "Reconectando…" | El coordinador está caído o reiniciando | Espere unos segundos. Si no vuelve, revise la ventana del coordinador. Los workers esperan y se reconectan solos |
| El worker de otra PC dice "no se pudo conectar" y reintenta | La PC no llega a node-1: otra red, cambió la IP o falta la regla de firewall | Confirme que están en el mismo WiFi, ejecute `scripts\firewall-node1.ps1` como administrador en node-1 y descargue de nuevo el ZIP desde /connect (trae la IP actual) |
| El worker aparece pero sus sub-tareas fallan al descargar la entrada | La PC no llega a MinIO (puerto 9000) | Mismo diagnóstico de la fila anterior |
| La ventana del worker no abre en Windows 11 | Smart App Control o el bloqueo de archivos descargados | Desbloquee el ZIP en Propiedades antes de descomprimir; si es Smart App Control, vea la sección 8.3 |
| "Falta ffmpeg" en Linux | ffmpeg no está instalado | `sudo apt install ffmpeg` o `sudo pacman -S ffmpeg` |
| Un caso se queda "en cola" mucho tiempo | No hay ningún worker conectado, o todos están llenos | Revise el Monitor: si no hay nodos, conecte uno; si la cola de un pool crece, sume una PC con ese rol |
| Un caso pasó a "reintentando" | Un worker se cayó o se cerró a mitad de trabajo | No hace falta hacer nada: sus sub-tareas volvieron a la cola y otro nodo las toma |
| Una sub-tarea tiene el chip "⚠ aviso" | La extensión del archivo no coincide con su contenido | Lea la nota con el cursor. El archivo se procesó según su contenido real; conviene renombrarlo |
| Una sub-tarea falló con "moov atom not found" o "Invalid data found when processing input" | El archivo está incompleto, dañado o no es multimedia | Revise el archivo original. El resto del caso sigue y cierra como parcial |
| Una sub-tarea falló con "archivo vacío (0 bytes)" | El archivo no tiene contenido | Vuelva a copiar el archivo desde su origen |
| "formato no soportado" al elegir un archivo | La extensión no está en la lista de la sección 5.2 | Convierta el archivo con otro programa o renómbrelo si la extensión está mal |
| Error 400 al enviar un JSON por la API | El cuerpo no está en UTF-8 (tildes o ñ en otra codificación) | Guarde el JSON en UTF-8 o envíelo como bytes UTF-8; el mensaje de error indica cómo |
| Error 400 al pedir, por ejemplo, mp4 → mp4 | Convertir al mismo formato no es una conversión | Elija otro formato de salida, o use miniatura si quiere cambiar el tamaño |
| La PC se vuelve muy lenta o una máquina virtual se congela | Poca RAM libre con varias máquinas virtuales encendidas | Apague las máquinas virtuales que no use antes de encender otras (`vagrant halt` en `infra/vagrant`) |
| "Publicar en internet" termina en error | La red bloquea la salida a Cloudflare (por ejemplo, el WiFi del TEC) | Encienda Cloudflare WARP o una VPN en node-1 y vuelva a intentar |

\pagebreak

# 11. Glosario

Tabla 12. Términos usados en este manual

| Término | Significado |
|---|---|
| Afinidad | Preferencia del planificador por enviar una sub-tarea a un nodo de su mismo pool |
| Ayuda | Sub-tarea que procesa un nodo de otro pool porque estaba más libre que el afín |
| Caso | Conjunto de archivos que entra como una sola solicitud y cierra con un reporte consolidado |
| Caso heterogéneo | Caso con archivos de distintos tipos u operaciones |
| Caso homogéneo | Caso con archivos del mismo tipo y la misma operación |
| Caso de prueba | Caso armado de antemano con el dataset, disponible en el formulario y en `ingest` |
| Coordinador | Programa de node-1 que recibe los casos, decide la operación de cada archivo, reparte las sub-tareas y cierra el caso |
| Cupo | Espacio para una sub-tarea simultánea en un nodo; la capacidad es la cantidad de cupos |
| Dashboard | Página web para enviar, seguir y monitorear casos |
| Dataset | Colección de 542 archivos de prueba (video, audio e imagen) cargada en MinIO |
| Enriquecer | Operación que integra portada, etiquetas y letra o descripción dentro del mismo archivo |
| Extensión engañosa | Archivo cuya extensión no coincide con su contenido real |
| ffmpeg | Programa libre que hace las conversiones y extracciones |
| MinIO | Almacenamiento de archivos de node-1, donde viven las entradas y los resultados |
| node-1 | Computadora principal: base de datos, cola, almacenamiento, coordinador y un worker |
| Pool | Grupo de trabajo por tipo de contenido: video, audio o metadata |
| Prioridad | Número de 1 a 10 que ordena la cola; 8 o más es prioridad alta |
| Reporte consolidado | Resumen del caso al cerrar: resultados, tiempos, workers y errores |
| Rol | Pool principal que se le asigna a un worker al descargarlo |
| Sub-tarea | Trabajo sobre un solo archivo dentro de un caso |
| Túnel | Conexión a través de Cloudflare que permite sumar workers desde otra red |
| Worker | Programa que procesa sub-tareas en una computadora conectada |
