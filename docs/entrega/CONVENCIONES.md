# Convenciones de las fuentes Markdown de los Word de entrega

Cada entregable Word se genera con `python docs/entrega/build_docx.py` a partir de un `.md` de esta
carpeta. El script copia la portada oficial del equipo (indagatoria de SO) y escribe el cuerpo con
Times New Roman 12, interlineado 1.5, carta, márgenes de 2.54 cm, cuerpo justificado con sangría de
primera línea de 1.27 cm y títulos sin sangría.

## Encabezado del archivo (obligatorio, primeras líneas)

```
---
tipo: I Proyecto Programado
titulo: Informe de pruebas
subtitulo: Plataforma distribuida de procesamiento multimedia por casos (MediaCase)
---
```

## Sintaxis admitida

- `# Título` → Título 1 (numerar a mano: `# 1. Introducción`). `##` → Título 2, `###` → Título 3.
- Párrafo = líneas seguidas sin línea en blanco entre ellas. Se justifica con sangría de primera línea.
- `**negrita**`, `*cursiva*`, `` `código` `` (monoespaciado) dentro del texto.
- Listas: `- ` viñeta, `1. ` numerada (un nivel; para subniveles, dos espacios + `- `).
- Tablas Markdown con `|`. La fila siguiente a la tabla puede ser `Tabla N. Título` en su propia línea
  ANTES de la tabla (formato APA: número en negrita arriba, título en cursiva debajo). Ejemplo:
  ```
  Tabla 3. Resultados de los casos de prueba

  | Caso | Estado | Duración |
  |---|---|---|
  | … | … | … |
  ```
- Bloques de código con tres comillas invertidas: se imprimen en letra monoespaciada, sin sangría.
- **Espacio para una captura** (la toma Leno después; se ve como un recuadro con la descripción):
  ```
  :::figura Figura 4. Monitor con tres nodos procesando un caso heterogéneo
  Qué debe verse: pestaña Monitor del dashboard con las tarjetas de node1 (video), node2 (audio) y node3
  (metadata) en estado OCUPADO, cada una con su barra de CPU arriba de 50 % y "N de M cupos ocupados".
  Cómo obtenerla: vagrant up en infra/vagrant, enviar tc11 desde el formulario y capturar a los 20 s.
  Captura existente que sirve: docs/img/monitor-3-nodos-vagrant.png (11 set).
  :::
  ```
  La primera línea da número y título; el cuerpo describe con precisión qué debe mostrar la imagen, cómo
  obtenerla y, si existe, qué captura de `docs/img/` sirve. Siempre en tercera persona o impersonal.
- **Diagrama** (lo dibuja el script con Mermaid; no es captura):
  ````
  :::diagrama Figura 1. Componentes y nodos de MediaCase
  ```mermaid
  flowchart LR
    ...
  ```
  :::
  ````
  Mermaid simple: flowchart, sequenceDiagram o stateDiagram-v2. Etiquetas cortas, en español, sin
  paréntesis dentro de corchetes (usar comillas: `A["Coordinador (node-1)"]`).
- Nota de uso (recuadro): una línea que empiece con `> ` se imprime como nota destacada.
- `\pagebreak` en su propia línea: salto de página.

## Reglas de redacción

- Español de Costa Rica, registro técnico formal. El manual trata al lector de **usted** ("abra",
  "descargue"), nunca voseo. Los otros dos documentos, impersonales.
- Aplicar la skill humanizer (C:\Users\lenok\.claude\skills\humanizer\SKILL.md) en modo embebido:
  sin rayas (— ni –; usar punto, coma, dos puntos o paréntesis), sin vocabulario inflado ("crucial",
  "robusto", "clave" como adjetivo, "potenciar", "cabe destacar", "en resumen", "es importante señalar"),
  sin tríadas forzadas, sin listas con encabezado en negrita + dos puntos en cada ítem, sin conclusiones
  genéricas, sin anunciar lo que se va a decir. Oraciones de largo variado. Cifras concretas.
- Describir el sistema como es, no como historia de cambios ("el coordinador inspecciona", no "se agregó").
  Excepción: el informe de pruebas sí narra defectos encontrados y corregidos.
- No inventar datos: todo número sale de docs/entrega/HECHOS_2026-09-25.md, de docs/*.md o del código.
- Comillas rectas "así". Nombres de archivos, variables y endpoints en `código`.
