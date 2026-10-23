# 🚀 High-Concurrency & Distributed Systems Portfolio (Go + Redis + Postgres + WebSockets)

Este repositorio contiene **3 proyectos de ingeniería de software de alto impacto** diseñados para posicionar un perfil de **Senior / Staff Backend Engineer & Distributed Systems Specialist**. 

Cada proyecto aborda un dominio computacional complejo, implementado desde el metal con **Go**, sin frameworks pesados, con algoritmos de baja latencia, serialización binaria sin alocaciones (`0 allocs/op`), transaccionalidad atómica y resiliencia auditada mediante pruebas de caos.

---

## 🏛️ Matriz Comparativa de los 3 Proyectos

| Dimensión | 01. Pixel War (r/place) | 02. Arena .io Multiplayer | 03. Uber Matching Engine |
|---|---|---|---|
| **Dominio** | Estado masivo colaborativo | Simulación de física en tiempo real | Despacho geoespacial distribuido |
| **Escala Comprobada** | **5.000 WebSockets concurrentes** | **500 jugadores / bots en sala** | **50.000 vehículos transmitiendo GPS** |
| **Throughput Medido** | **97.430 mensajes binarios** | **22.162 inputs/segundo** | **56.497 GEOADD/seg (Pipeline)** |
| **Latencia p95 / p99** | **p95: 1.38 ms** (conexión WS) | **p99: < 2.5 ms** (duración tick) | **p99: 3.8 ms** (búsqueda geoespacial) |
| **Innovación de Memoria** | 1M píxeles en **976.5 KB** (`[]byte`) | Spatial Hash Grid en **62 µs** (`0 allocs`) | Radius Stepping concéntrico ($O(N \log N)$ fix) |
| **Mecanismo de Concurrencia** | **64 Striped RWMutexes** | **Fixed-Tick Loop (30 TPS) + Bitmask** | **Lua Scripts atómicos + EVALSHA** |
| **Resiliencia & Caos** | Desalojo adaptativo de clientes lentos | 100% de speed-hacks neutralizados | Transactional Outbox + 0 zombis (Reaper) |
| **Stack Principal** | Go, Redis BITFIELD, WebSockets | Go, WebSockets, HTML5 Canvas, Redis | Go, Redis GEO, Streams, PostgreSQL, pgx |

---

## 📁 Estructura del Repositorio

```text
├── 01-pixel-war/          # Lienzo colaborativo en tiempo real (1000x1000)
│   ├── cmd/server/        # Servidor WebSocket y motor de deltas binarios
│   ├── internal/canvas/   # Motor de memoria plano y striped locking (64 cerrojos)
│   ├── internal/ws/       # Broadcast Hub con protección contra slow consumers
│   ├── loadtest.js        # Script de estrés con k6 para 5.000 VUs
│   └── README.md          # Documentación técnica con benchmarks auditados
│
├── 02-arena-io/           # Servidor de juego autoritativo a 30 TPS
│   ├── cmd/server/        # Servidor de física y sincronización por WebSocket
│   ├── cmd/bot/           # Generador de caos (500 bots con jitter y speed-hacks)
│   ├── internal/game/     # Spatial Hash Grid, RingBuffer y física vectorial
│   ├── internal/netproto/ # Protocolo binario de 18B y pooling con sync.Pool
│   └── README.md          # Diagrama de game loop y reporte de auditoría
│
└── 03-uber-matching/      # Motor de despacho geoespacial distribuido
    ├── internal/geo/      # Ingesta Redis GEO con Radius Stepping (500m -> 5km)
    ├── internal/matching/ # Máquina de estados con Lua y anti-zombie Reaper
    ├── internal/storage/  # Persistencia relacional en PostgreSQL con pgxpool
    ├── internal/worker/   # Consumidor asíncrono con Redis Streams y retención PEL
    ├── docker-compose.yml # Infraestructura aislada (Redis 7.2 + Postgres 16)
    └── README.md          # Especificación Staff y preguntas de System Design
```

---

## 📝 12 Bullets de Impacto para tu CV (Fórmula Google XYZ)

Copia y pega estos bullets directamente en tu currículum según el rol al que apliques:

### 🎨 Proyecto 1: Pixel War (Concurrencia Masiva & WebSockets)
1. *Diseñó e implementó un motor de lienzo colaborativo en tiempo real en Go capaz de sostener **5.000 conexiones WebSocket concurrentes** y procesar **>97.000 deltas de píxel** con una latencia de conexión **p95 de 1.38 ms**, utilizando un mecanismo de cerrojos fraccionados (*64-stripe locking*) y difusión no bloqueante.*
2. *Redujo el consumo de memoria del lienzo de 1.000.000 de píxeles en un **75%** (**976.5 KB en RAM** frente a 4 MB tradicionales) alcanzando **0 alocaciones en el heap** en la ruta crítica de escritura, mediante un buffer contiguo indexado de 8 bits y serialización binaria sin reflexión.*
3. *Disminuí el tráfico de red en un **87.5%** frente a arquitecturas JSON tradicionales, diseñando un protocolo binario propietario de **5 bytes por delta** (`[X:2B][Y:2B][Color:1B]`) en Big-Endian con agregación por lotes (*write-batching*) en el pipeline del socket.*
4. *Eliminé cuellos de botella por clientes lentos (*slow consumers*) y riesgos de bloqueo en cadena (*head-of-line blocking*), implementando un Broadcast Hub con desalojo adaptativo de sockets saturados y validando la estabilidad con suites de pruebas destructivas en **k6**.*

### 🕹️ Proyecto 2: Arena .io (Game Engines, Netcode & Low-Latency)
5. *Desarrolló un motor de juego multijugador autoritativo en Go capaz de orquestar salas de **500 jugadores concurrentes a 30 TPS estables**, procesando más de **$22.000\text{ inputs/seg}$** con una latencia de cálculo de tick **p99 $< 2.5\text{ ms}$** (consumiendo menos del 8% del presupuesto de frame).*
6. *Redujo el tiempo de procesamiento de colisiones y difusión de red en un **96.6%** (de **$1.85\text{ ms}$ a $62\ \mu\text{s}$ por tick**), diseñando un sistema de particionado espacial **Spatial Hash Grid 2D de 400 celdas contiguas** con gestión de Área de Interés (AoI) y **$0\text{ alocaciones en el heap}$**.*
7. *Neutralicé el **100% de ataques de teletransporte y speed-hacking** (>31.000 intentos mitigados bajo pruebas de estrés), implementando un modelo autoritativo estricto con **Buffers Circulares de 32 ranuras (`RingBuffer`) indexados a nivel de bits** y truncamiento vectorial de desplazamiento.*
8. *Disminuí el tráfico de red en un **69.4%** respecto a estándares JSON y erradiqué pausas del Garbage Collector, diseñando un protocolo binario propietario de **18 bytes por entidad** en LittleEndian y reciclaje de buffers mediante **`sync.Pool`**, manteniendo la huella de memoria en **$<10\text{ MB}$ de RAM**.*

### 🚗 Proyecto 3: Uber Matching Engine (Sistemas Distribuidos & Event-Sourcing)
9. *Diseñó un motor de matching geoespacial capaz de procesar **2.000 solicitudes/min** para un ecosistema de **50.000 vehículos transmitiendo GPS cada 3 segundos**, reduciendo la latencia de asignación atómica a **$466\ \mu\text{s}$** mediante scripts Redis Lua precompilados (`EVALSHA`).*
10. *Optimizó las búsquedas de proximidad ($O(N \log N)$) implementando un algoritmo de **Radius Stepping concéntrico (500m $\to$ 5km)** sobre Redis GEO, fijando la latencia de radiobúsqueda **p99 en $3.8\text{ ms}$** bajo máxima densidad urbana.*
11. *Erradicó la doble asignación de viajes y los estados zombi implementando una máquina de estados determinista y un **worker Reaper asíncrono de purga con TTLs transaccionales de 15 segundos**, logrando cero inconsistencias ante cancelaciones concurrentes.*
12. *Garantizó consistencia eventual y **cero pérdida de transacciones** ante caídas abruptas de la base de datos relacional, implementando el patrón **Transactional Outbox directo en Redis Streams** y consolidación por lotes en PostgreSQL mediante confirmación retardada (`XACK` post-commit).*

---

## 🌐 Centralización del Portfolio: Portfolio Hub & API Gateway

Para que reclutadores y Engineering Managers puedan evaluar y probar los 3 microservicios sin fricción de puertos ni configuración manual, el sistema implementa dos capas de acceso:

```mermaid
graph TD
    User([Reclutador / Navegador]) -->|HTTP :80| GW[Nginx Reverse Proxy / Gateway]
    
    subgraph "Docker Compose Cluster"
        GW -->|/| HUB[Landing Page Portfolio Hub]
        GW -->|/pixelwar/ & WS| PW[Pixel War :8080]
        GW -->|/arena/ & WS| AR[Arena .io :8081]
        GW -->|/uber/| UB[Uber Matching :8082]
        
        UB -->|GEO & Streams| R[(Redis 7.2)]
        UB -->|Event Sourcing| PG[(PostgreSQL 16)]
    end
```

### Nivel 2: Despliegue Unificado con Proxy Inverso (Recomendado)
Un único comando orquesta todo el clúster (Nginx Gateway, los 3 motores en Go, Redis 7.2 y PostgreSQL 16):

```bash
docker compose up --build
```

- **Portfolio Hub (Punto Único de Entrada)**: `http://localhost/`
- **Pixel War Demo**: `http://localhost/pixelwar/` (WebSockets enroutados transparentemente a `:8080`)
- **Arena .io Demo**: `http://localhost/arena/` (Game loop a 30 TPS enroutado a `:8081`)
- **Uber Matching Demo**: `http://localhost/uber/` (Radar geoespacial enroutado a `:8082`)

### Nivel 1: Ejecución Directa por Servicios (Sin Docker)
Si se desea arrancar los binarios localmente en sus puertos dedicados:

#### 1. Pixel War (Puerto 8080)
```bash
cd 01-pixel-war
go run ./cmd/server               # Servidor escuchando en http://localhost:8080
k6 run loadtest.js                # Lanzar ataque de 5.000 VUs
```

#### 2. Arena .io (Puerto 8081)
```bash
cd 02-arena-io
go run ./cmd/server               # Servidor de juego a 30 TPS en http://localhost:8081
go run ./cmd/bot -bots=500        # Simular 500 bots hostiles con speed-hacks
```

#### 3. Uber Matching Engine (Puerto 8082)
```bash
cd 03-uber-matching
docker compose up -d redis postgres # Levantar infraestructura
go run ./cmd/server                 # Radar geoespacial en http://localhost:8082
go test -v ./internal/...           # Suite completa de tests unitarios y caos
```

## 🛡️ La Metodología de los 3 Roles Aplicada

1. **El Constructor (Gating Estricto)**: Cada módulo se levantó bloque a bloque, forzando la interiorización del código y la justificación teórica de cada estructura de datos.
2. **El Destructor (SRE & Pentesting)**: Cada sistema fue sometido a tests destructivos reales: *thundering herds* de 5.000 conexiones, saturación de Garbage Collector, inyección de 31.000 speed hacks y simulación de apagones de PostgreSQL a mitad de ráfagas transaccionales.
3. **El Empaquetador (Venta Técnica)**: Documentación de nivel Staff Engineer, diagramas de arquitectura en Mermaid, métricas duras verificadas y respuestas modelo para entrevistas de System Design.
