# 🎬 Guiones Técnicos de Demostración en Vídeo (60 Segundos)
> **Ecosistema de Sistemas Distribuidos y Alta Concurrencia en Go**  
> Formato optimizado para Engineering Managers, Tech Leads y Comités de Contratación Staff/Senior.

---

## 🧭 Estructura Maestra de los 60 Segundos
Cada vídeo debe seguir una narrativa de ingeniería quirúrgica: **cero saludos vacíos, cero rodeos corporativos**.

```text
[00:00 - 00:10] EL DOLOR TÉCNICO      -> Qué problema de concurrencia/escala se resuelve.
[00:10 - 00:35] LA DECISIÓN Y EN VIVO -> Cómo la arquitectura resuelve el problema en tiempo real.
[00:35 - 00:50] PRUEBA DE ESTRÉS/CAOS -> Inyección de carga o modo de bajo nivel que valida el trade-off.
[00:50 - 01:00] MÉTRICA EMPÍRICA      -> El número exacto p95/p99 que respalda el resultado.
```

---

## 🎨 01. Pixel War (`:8080` / `http://localhost:8080`)
> **Tesis:** Manejo de 1M de píxeles en memoria plana (`976.5 KB`) con protocolo binario de 5 bytes y 64 cerrojos particionados para eliminar la contención.

### ⏱️ Timeline & Guion de Grabación

| Segundo | Acción en Pantalla (Cursor & Interacción) | Locución en Español (Voiceover) | Locución en Inglés (International) |
|---|---|---|---|
| **00:00 - 00:10** | Pantalla completa de Pixel War. Pasa el cursor por el lienzo blanco y pinta un trazo rápido con el dock inferior. | *"En un lienzo colaborativo de 1 millón de píxeles con miles de conexiones, enviar payloads JSON colapsa la red y los locks globales congelan el servidor."* | *"In a 1-million-pixel canvas with thousands of concurrent users, JSON payloads saturate bandwidth and global locks bottleneck the server."* |
| **00:10 - 00:25** | Presiona la tecla <kbd>T</kbd> para abrir el drawer de telemetría. Muestra el selector de color y la gráfica en tiempo real. | *"Diseñé un protocolo binario plano de 5 bytes por delta sobre WebSockets: 2 bytes X, 2 bytes Y y 1 byte color. Esto reduce el consumo de ancho de banda un 87% frente a JSON."* | *"I implemented a flat 5-byte binary protocol over WebSockets: 2 bytes X, 2 bytes Y, 1 byte color. This slashes bandwidth usage by 87% compared to JSON."* |
| **00:25 - 00:40** | Presiona la tecla <kbd>M</kbd> (Modo Arquitectura). Pasa el cursor sobre los píxeles para mostrar el inspector HUD con `sync.RWMutex[X]` y el memory offset hexadecimal `+0x...`. | *"Para la memoria, el estado reside en un array continuo de 976 KB particionado en 64 cerrojos sync.RWMutex según la coordenada. Como ves en el modo arquitectura, cada franja opera de forma aislada."* | *"For in-memory state, 976 KB of flat heap is striped across 64 sync.RWMutex locks mapped by coordinates. In architecture mode, every stripe operates completely independently."* |
| **00:40 - 00:55** | Haz clic en `⚡ Simular Bots (50 VUs)`. Muestra los destellos blancos en el canvas y los cerrojos activos parpadeando en la matriz lateral. | *"Al inyectar tráfico masivo de bots, la matriz de telemetría confirma un balance de carga uniforme y 0.00% de contención o tiempo de espera en locks."* | *"Under synthetic bot traffic, the telemetry grid confirms uniform load balancing with 0.00% lock contention wait time."* |
| **00:55 - 01:00** | Presiona <kbd>M</kbd> para volver al lienzo real. Haz zoom 3x en el minimapa. | *"Auditado con k6 a 5.000 VUs simultáneos, sosteniendo más de 120.000 deltas/segundo con latencia p95 de 1.38 milisegundos."* | *"Benchmarked with k6 at 5,000 concurrent VUs, sustaining over 120,000 deltas/s with a p95 latency of 1.38 milliseconds."* |

---

## 🕹️ 02. Arena .io Multiplayer (`:8081` / `http://localhost:8081`)
> **Tesis:** Servidor autoritativo a 30 TPS con Spatial Hash Grid 2D en 62 µs, zero GC stalls vía `sync.Pool`, división celular y reconciliación client-side.

### ⏱️ Timeline & Guion de Grabación

| Segundo | Acción en Pantalla (Cursor & Interacción) | Locución en Español (Voiceover) | Locución en Inglés (International) |
|---|---|---|---|
| **00:00 - 00:10** | Canvas de Arena .io. Mueve el ratón rápidamente para controlar la célula y come 3-4 pellets luminosos. | *"En un juego multijugador en tiempo real con 500 entidades, comprobar colisiones O(N²) bloquea el game loop y el recolector de basura de Go provoca micro-tirones."* | *"In a real-time multiplayer arena with 500 entities, O(N²) collision checks stall the game loop and Go's garbage collector causes stutter."* |
| **00:10 - 00:25** | Presiona <kbd>Espacio</kbd> para dividir la célula (Split). Muestra la subcélula impulsada y presiona <kbd>W</kbd> para eyectar masa. | *"El servidor corre un fixed-tick loop autoritativo a 30 TPS. Resolví las colisiones y el Área de Interés con un Spatial Hash Grid 2D que solo comprueba las 9 celdas adyacentes, bajando el cálculo de tick a 62 microsegundos."* | *"The authoritative server runs at a fixed 30 TPS loop. I optimized collisions and Area of Interest using a 2D Spatial Hash Grid querying only 9 adjacent cells, dropping tick physics to 62 microseconds."* |
| **00:25 - 00:40** | Abre el drawer de física lateral. Muestra el uso de memoria (<10 MB) y el badge de `0 GC Pauses`. | *"Para eliminar pausas de GC, todas las estructuras del hot path y los paquetes de 18 bytes se reciclan mediante sync.Pool, manteniendo la huella de RAM bajo 10 MB para 500 bots."* | *"To eradicate GC pauses, all hot-path entities and 18-byte packets are pooled via sync.Pool, keeping total heap consumption under 10 MB for 500 bots."* |
| **00:40 - 00:55** | Arrastra el slider de Latencia a 200 ms y 30 ms de jitter. Muestra cómo el círculo fantasma (posición autoritativa) y la célula del jugador se reconcilian con interpolación Lerp sin tirones. | *"Para lidiar con el lag, el cliente ejecuta predicción local y reconciliación lineal. Incluso simulando 200 milisegundos de ping y jitter, el movimiento se mantiene completamente fluido."* | *"To handle network latency, the client performs local prediction and Lerp reconciliation. Even simulating 200 ms of ping and jitter, movement stays completely smooth."* |
| **00:55 - 01:00** | Devora un bot en pantalla. Muestra el contador de speed-hacks bloqueados en la esquina. | *"El servidor valida el desplazamiento vectorial con ring-buffers, neutralizando más de 31.000 ataques de velocidad simulados en pruebas de estrés."* | *"Authoritative vector validation with input ring-buffers neutralizes 100% of speed-hacks across 31,000 synthetic attack packets."* |

---

## 🚗 03. Uber Matching Engine (`:8082` / `http://localhost:8082`)
> **Tesis:** Ingesta de 56.497 GPS/s, matching atómico con scripts Lua precompilados (`EVALSHA`) en 466 µs, Radius Stepping y Transactional Outbox con Redis Streams.

### ⏱️ Timeline & Guion de Grabación

| Segundo | Acción en Pantalla (Cursor & Interacción) | Locución en Español (Voiceover) | Locución en Inglés (International) |
|---|---|---|---|
| **00:00 - 00:10** | Vista del mapa oscuro cyberpunk con vehículos moviéndose en tiempo real. | *"Gestionar 50.000 conductores transmitiendo telemetría GPS continua colapsa las bases de datos relacionales y genera condiciones de carrera de doble reserva."* | *"Tracking 50,000 drivers streaming continuous GPS updates overloads relational databases and causes double-booking race conditions."* |
| **00:10 - 00:25** | Muestra el HUD de throughput GPS superando las 50k actualizaciones por segundo. | *"Implementé un pipeline en Go sobre índices Redis GEO capaz de procesar más de 56.000 actualizaciones GPS por segundo con cero bloqueos de red."* | *"I built a Go ingestion pipeline over Redis GEO indexes processing over 56,000 GPS updates per second with zero network blocking."* |
| **00:25 - 00:40** | Haz clic en `📍 Solicitar Viaje (Gran Vía)` o en cualquier punto del mapa. Observa los anillos concéntricos expandiéndose y la asignación instantánea en verde. | *"El despacho utiliza un algoritmo concéntrico de Radius Stepping de 500 metros a 5 kilómetros. Cuando encuentra candidatos, ejecuta un script Lua atómico precompilado con EVALSHA, cerrando el match en 466 microsegundos."* | *"Dispatch relies on concentric Radius Stepping expanding from 500m to 5km. Upon candidate detection, a precompiled Redis Lua script (EVALSHA) executes atomic assignment in 466 microseconds."* |
| **00:40 - 00:52** | Haz clic en el botón `⚡ Inyectar Caos (Simular Caída de Postgres)`. Muestra cómo los eventos se retienen en Redis Streams PEL y se sincronizan al recuperarse. | *"Para la persistencia, apliqué el patrón Transactional Outbox desacoplado con Redis Streams y confirmación manual XACK. Ante caídas de la base de datos, los eventos se retienen en la PEL sin perder un solo viaje."* | *"For resilience, I implemented the Transactional Outbox pattern decoupled via Redis Streams and manual XACK. If PostgreSQL fails, events buffer safely in the Pending Entries List."* |
| **00:52 - 01:00** | Muestra el panel de estados del Reaper worker liberando viajes huérfanos. | *"Un Reaper worker en segundo plano con TTLs de 15 segundos garantiza cero conductores zombis ante desconexiones repentinas del cliente."* | *"A background Reaper worker with 15-second transactional TTLs guarantees zero zombie driver states upon abrupt client dropouts."* |

---

## 🌐 04. Hub Central & Auditoría Descargable (`:80` / `index.html`)
> **Tesis:** Portal de orquestación, observabilidad en vivo de los 3 microservicios y reporte descargable de auditoría técnica con reproducibilidad local.

### ⏱️ Timeline & Guion de Grabación

| Segundo | Acción en Pantalla (Cursor & Interacción) | Locución en Español (Voiceover) | Locución en Inglés (International) |
|---|---|---|---|
| **00:00 - 00:10** | Carga `http://localhost:3000` (o el puerto :80 del gateway). Muestra la animación de los 4 contadores superiores. | *"Para facilitar la evaluación de los tres sistemas distribuidos, construí un Hub Central que unifica el clúster Docker, proxy Nginx y observabilidad en tiempo real."* | *"To simplify technical evaluation of all three distributed engines, I built a Central Hub unifying the Docker cluster, Nginx proxy, and real-time observability."* |
| **00:10 - 00:25** | Muestra los badges de salud `● LIVE :8080`, `● LIVE :8081`, `● LIVE :8082` y las tres pantallas interactivas de previsualización en canvas. | *"Cada microservicio cuenta con healthchecks en vivo por polling cada 3 segundos y soporte de enrutamiento dual: acceso directo a puertos o reverse proxy mediante Nginx."* | *"Each microservice reports live healthchecks via 3-second polling with dual-mode routing: direct developer ports or unified Nginx reverse proxy."* |
| **00:25 - 00:42** | Haz clic en el botón superior `📊 Auditoría & Benchmarks`. Navega por las pestañas: Métricas (p50/p95/p99) y Matriz de Resiliencia. | *"El hub incluye un módulo de auditoría técnica interactivo con tablas comparativas de latencia p50, p95 y p99, y una matriz de resiliencia frente a fallos inyectados."* | *"The hub features an interactive technical audit modal detailing p50, p95, and p99 latency percentiles alongside a chaos resilience matrix."* |
| **00:42 - 00:52** | Cambia a la pestaña de Reproducibilidad. Haz clic en `Copiar` en el comando de k6 y muestra el feedback `✓ Copiado`. | *"Cualquier evaluador puede reproducir los benchmarks localmente con los one-liners de k6, pprof y Go tests incluidos en la pestaña de reproducibilidad."* | *"Evaluators can replicate every benchmark locally using the one-liner k6, pprof, and Go test commands directly copyable from the modal."* |
| **00:52 - 01:00** | Haz clic en `📦 Descargar JSON` y `📄 Descargar Markdown`. Muestra los archivos descargados. | *"Y en 1 solo clic, el evaluador puede exportar el informe completo en JSON estructurado o Markdown técnico para su revisión offline."* | *"And in a single click, evaluators can export the full empirical report in structured JSON or technical Markdown for offline review."* |

---

## 🛠️ Guía Rápida para la Grabación del Vídeo

1. **Resolución y Frame Rate:** Graba a **1920x1080 (1080p) a 60 FPS**. En herramientas como Loom u OBS, mantén un bitrate mínimo de 6.000 a 8.000 Kbps para que los números pequeños de la telemetría se lean nítidos.
2. **Cursor del Ratón:** Activa un resaltador sutil del cursor (halo amarillo o azul tenue) para que el espectador no pierda de vista tus clics en los docks o interruptores.
3. **Atajos de Teclado Rápidos:**
   - En **Pixel War**: pulsa <kbd>M</kbd> para alternar el modo de 64 Mutexes y <kbd>T</kbd> para la telemetría.
   - En **Arena .io**: pulsa <kbd>Espacio</kbd> para Split y <kbd>W</kbd> para Eject.
   - En el **Hub**: pulsa <kbd>Esc</kbd> para cerrar el modal suavemente.
4. **Hosting del Vídeo:**
   - Sube cada vídeo a Loom, YouTube (modo Oculto/Unlisted) o intégralos como GIFs/WebM de alta calidad en los respectivos `README.md`.
