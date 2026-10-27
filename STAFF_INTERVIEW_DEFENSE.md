# 🛡️ Staff & Principal Engineer Technical Defense Guide
> **Manual de Defensa Técnica en Entrevistas de Arquitectura, Concurrencia y Sistemas Distribuidos**  
> Repositorio: [high-concurrency-distributed-portfolio](https://github.com/CXarlosss/high-concurrency-distributed-portfolio)

---

## 🎯 Checklist de Actitud en la Entrevista
1. **Valida el dolor antes de responder:** En lugar de ponerte a la defensiva, di: *"Ese es exactamente el cuello de botella que identifiqué al medir con pprof..."*
2. **Cita la métrica antes del código:** Nombra microsegundos, allocations y bytes antes de explicar los structs.
3. **Reconoce los límites de tus herramientas:** Admitir que TCP tiene limitaciones frente a UDP o que Redis monohilo requiere particionado manual demuestra madurez de Staff Engineer.
4. **Presenta el Trade-Off:** Todo diseño de software de alto rendimiento sacrifica algo (consistencia por latencia, memoria por CPU, o complejidad operativa por throughput).

---

## 🎨 Pixel War: Autenticación y Rate Limiting a Gran Escala (100.000 Conexiones)

### La Pregunta del Entrevistador
> *"En Pixel War permites pintar libremente para la demo local. Pero en producción masiva con 100.000 usuarios concurrentes y una regla de '1 píxel cada 30 segundos por usuario', ¿cómo diseñarías la autenticación y el rate limiting sin que la base de datos o Redis se conviertan en el cuello de botella del canvas?"*

### La Trampa Oculta
1. **Auth por mensaje:** Si validas un token JWT o consultas una base de datos/Redis en cada delta de 5 bytes, generas 100k QPS de autenticación innecesaria en el hot path.
2. **Token Bucket clásico:** Mantener un Token Bucket complejo en Redis con timestamps flotantes y recarga dinámica (`capacity, tokens, last_refill`) añade una sobrecarga atroz para una regla fija de intervalo ("1 píxel cada 30s").
3. **Multi-Tab Exploit:** Si el rate limit vive solo en la memoria de la goroutine de Go, un usuario abre 10 pestañas en navegadores o se conecta a diferentes nodos del clúster para pintar 10 veces más rápido.
4. **Ataques Sybil:** Granjas de bots creando 20.000 cuentas falsas para monopolizar el canvas.

---

### Respuesta Modelo (Nivel Staff / Principal)

#### 1. Autenticación Zero-Overhead en el Hot Path (Handshake Pinning)
> *"El primer principio es **sacar la autenticación completamente del hot path de deltas**. La autenticación ocurre una única vez durante el HTTP Upgrade del WebSocket mediante un JWT firmado asimétricamente con **Ed25519** o cookie de sesión segura."*

- La clave pública de verificación reside en la memoria de cada nodo de Go; validar la firma toma **menos de 15 microsegundos** con **0 consultas de red**.
- Una vez verificado, el `UserID` (un `uint64`) queda inmutablemente fijado en el contexto de la estructura `Client` de esa conexión WebSocket. Los paquetes de píxel entrantes siguen midiendo solo 5 bytes (`uint16 X, uint16 Y, uint8 Color`), sin transportar tokens de auth repetitivos.

```text
[Cliente] --- HTTP Upgrade (JWT Ed25519 en Header) ---> [Gateway / Go Worker]
                                                               |
                                            Firma válida en memoria (15 µs)
                                                               v
                                        Conexión fijada: Client{ UserID: 48291 }
                                                               |
[Cliente] === Delta binario 5B [X: uint16, Y: uint16, C: uint8] ===> [Hot Path sin Auth]
```

---

#### 2. Algoritmo de Rate Limiting: Atomic Timestamp CAS vs Token Bucket
> *"Un Token Bucket con recarga fraccional es una sobreingeniería costosa para una regla discreta de cooldown de 30 segundos. El mecanismo óptimo es un **Timestamp Cooldown Check** con evaluación de dos niveles: local en memoria y atómico en Redis."*

**Nivel 1 (Filtro local en memoria de la goroutine):**
- Cada estructura `Client` mantiene un campo `nextAllowedUnixMs int64`.
- Si `now < client.nextAllowedUnixMs`, el paquete se descarta de inmediato a nivel de socket en **menos de 20 nanosegundos**, sin tocar la red ni Redis.

**Nivel 2 (Consistencia distribuida multi-nodo contra apertura de múltiples pestañas):**
- Para evitar que el usuario pinte desde 10 conexiones en paralelo, el nodo ejecuta una operación atómica en Redis con `SET NX PX`:

```bash
# Intenta adquirir el derecho de pintar por 30 segundos (30.000 ms)
SET cooldown:user:<user_id> 1 NX PX 30000
```
- Si Redis retorna `OK`, el usuario puede pintar. Si retorna `nil`, el píxel es rechazado y se le devuelve un frame binario de error (código `0xFE` con milisegundos restantes).
- Al ser una primitiva nativa `SET NX PX`, la operación toma **menos de 180 microsegundos** en Redis.

---

#### 3. Escalado a 100.000 Usuarios: Consistent Hashing y Redis Cluster
> *"Si 100.000 usuarios intentaran pintar activamente al expirar su cooldown, Redis recibiría picos de escritura. Para evitar saturar el event loop de Redis:"*

1. **Particionado de claves por UserID:** Las claves `cooldown:user:<id>` se distribuyen mediante hash slots en un Redis Cluster de 3 nodos maestros (`CRC16(user_id) % 16384`), repartiendo la carga a ~1.000 ops/s por nodo.
2. **Sticky Routing en Nginx Gateway:** El proxy inverso balancea las conexiones WebSocket entrantes usando `hash $cookie_user_id consistent;`. Así, si un usuario abre varias pestañas, todas terminan en el mismo worker de Go, permitiendo que el **Nivel 1 en memoria local intercepte el 99% de las trampas** sin siquiera consultar a Redis.

---

#### 4. Mitigación de Granjas de Bots y Ataques Sybil
> *"Un rate limiting por usuario no sirve si un atacante genera 50.000 cuentas efímeras. La defensa requiere capas de coste asimétrico:"*

- **Restricción de Antigüedad y Verificación:** Requisito de cuenta con más de 7 días de antigüedad o verificación por OAuth (GitHub / Google) para obtener un cooldown base de 30 segundos; cuentas recién creadas reciben un cooldown degradado de 5 a 15 minutos.
- **Proof of Work (PoW) en Handshake:** Al conectar el WebSocket, el cliente debe resolver un desafío criptográfico liviano (ej. encontrar un hash SHA-256 con 4 ceros iniciales, ~80 ms en el navegador). Para 1 usuario legítimo es invisible; para una botnet intentando levantar 20.000 sockets simultáneos agota la CPU de las máquinas de ataque.
- **Sliding Window por Subred IP en Nginx:** Límite estricto de máximo 5 handshakes WebSocket concurrentes por `/24` de IPv4 para neutralizar proxies masivos económicos.

---

### Resumen de Trade-Offs (Cierre de Alto Nivel)
> *"El trade-off clave es que delegamos la estricta consistencia global a Redis solo para conexiones concurrentes del mismo usuario, mientras que el 99% de la protección vive en memoria local y en la validación asimétrica del handshake. Esto permite que el motor de canvas mantenga su rendimiento de **cero contención en los 64 cerrojos** y latencia sub-milisegundo sin comprometer la integridad del lienzo."*

---

## 🚀 Pixel War: Escalado a 100.000 Usuarios en Clúster Multihost Distribuido

### La Pregunta del Entrevistador
> *"Tu implementación monohost de Pixel War soporta 5.000 VUs en un único proceso Go. Supón que el producto se vuelve viral y necesitas escalar a 100.000 usuarios concurrentes en un clúster de Kubernetes. ¿Cómo rediseñas la arquitectura para evitar que el broadcast de píxeles colapse la red y los nodos se desincronicen?"*

### La Trampa Oculta
1. **La Catástrofe del Fan-Out de Red:** Con 100.000 usuarios y 2.000 escrituras de píxeles/segundo, un broadcast delta a delta significa:
   $$\text{Mensajes de Red} = 2.000\ \text{píxeles/s} \times 100.000\ \text{conexiones} = 200.000.000\ \text{msg/s}$$
   A 50 bytes por frame (cabeceras TCP/IP + WebSocket + payload), esto equivale a **10 GB/s de ancho de banda de salida**. Ningún nodo o broker resiste semejante saturación de NIC.
2. **La Trampa de Redis Pub/Sub:** Intentar usar Redis Pub/Sub estándar como bus de mensajería para 200 millones de mensajes por segundo funde el event loop monohilo de Redis en milisegundos.
3. **Split-Brain en el Lienzo:** Si cada worker de Go escribe de forma autónoma en su memoria local sin coordinación central, clientes conectados a distintos nodos verán píxeles incompatibles en la misma coordenada `(X, Y)`.

---

### Respuesta Modelo (Estrategia Staff / Principal en 5 Capas)

#### 1. Arquitectura de Dos Capas: Edge Gateways vs Canvas State Owner
> *"El secreto para escalar WebSockets masivos es **desacoplar la terminación de conexiones del cómputo de estado**. Separamos el clúster en dos grupos de nodos:"*

```text
[100.000 Clientes WebSockets]
             |
             v (L4 Load Balancer: AWS NLB / Maglev)
+-----------------------------------------------------------------------+
|  CAPA EDGE GATEWAY (10 nodos Go: 10k conexiones c/u - epoll / kqueue)  |
+-----------------------------------------------------------------------+
      | (Píxeles entrantes)                          ^ (Batch Broadcast)
      v (gRPC / NATS JetStream)                      | (20 Hz - 50 ms)
+-----------------------------------------------------------------------+
|  CAPA CANVAS OWNER (Particionado Espacial en Memoria de 1M de Píxeles)|
|         - Partición Norte (500k px)  |  Partición Sur (500k px)        |
|         - 64 sync.RWMutex Shards      |  Secuenciador Global (seq_id)   |
+-----------------------------------------------------------------------+
```

1. **Capa Edge Gateway (Stateless):** Un pool de 10 pods de Go termina 10.000 conexiones WebSocket cada uno (~120 MB RAM por pod usando `gobwas/ws` o `epoll` directo en Linux sin goroutines zombi).
2. **Capa Canvas Core (Stateful Single-Writer):** El lienzo reside en memoria en 1 o 2 nodos maestros particionados espacialmente (ej. Norte/Sur o cuadrantes). Cada escritura se serializa a través de un canal gRPC streaming, garantizando un orden total con un número de secuencia monótono (`uint64 seq_id`). Cero bloqueos distribuidos.

---

#### 2. Broadcast Coalescing & Tick-Based Batching (Solución al Fan-Out)
> *"Para erradicar la saturación de 200M msg/s, aplicamos **Coalescing Temporal a 20 Hz (50 ms)**, idéntico a los motores de videojuegos competitivos:"*

- En lugar de propagar cada píxel de 5 bytes de forma inmediata, el Edge Gateway acumula los deltas entrantes durante una ventana fija de 50 ms.
- Si en 50 ms se han pintado 80 píxeles en el lienzo, el worker empaqueta un **único batch binario de 400 bytes** ($80 \times 5\text{B}$) y lo difunde a sus 10.000 clientes locales.
- **Impacto Matemático:** La tasa de llamadas a `write()` en el kernel pasa de 20.000.000 syscalls/s por nodo a solo:
  $$20\ \text{ticks/s} \times 10.000\ \text{clientes} = 200.000\ \text{frames/s por pod}$$
  Una reducción del **99% en syscalls y saturación de paquetes TCP**.

---

#### 3. Interest Management y Filtrado por Viewport (AoI)
> *"Un usuario en un teléfono o monitor con zoom x4 solo observa un recorte de $250 \times 250$ píxeles del lienzo total de $1000 \times 1000$."*

- El cliente envía su bounding box visible cuando hace pan o zoom: `viewport: {x1, y1, x2, y2}`.
- El Gateway filtra el batch de 50 ms contra el viewport de cada cliente (utilizando la misma técnica de **Spatial Hash Grid 2D** que diseñamos para Arena .io).
- Si un píxel se pintó en la esquina opuesta, se descarta para esa conexión, ahorrando otro 75% del ancho de banda de salida.

---

#### 4. Thundering Herd en la Carga Inicial (S3 + CDN Edge Caching)
> *"Si 10.000 usuarios recargan la página a la vez, pedir el lienzo de 1 MB a los servidores Go generaría 10 GB de transferencia instantánea y pararía el event loop."*

- El nodo Canvas Core genera un snapshot binario del millón de píxeles cada 5 segundos y lo publica comprimido en formato **Brotli / Zstandard (~140 KB)** en un bucket de S3 detrás de **Cloudflare / CloudFront CDN**.
- El navegador descarga el lienzo completo desde el CDN en <25 ms con caché de borde HTTP/3.
- Inmediatamente después, el WebSocket se suscribe al Gateway pidiendo únicamente los deltas perdidos desde el `snapshot_seq_id`, logrando una reconexión instantánea sin tocar la base de datos ni los nodos de cómputo.

---

#### 5. Resumen de Trade-Offs (Defensa Final)
> *"El compromiso de diseño es que introducimos una latencia artificial máxima de 50 ms por el coalescing por lotes (imperceptible para un canvas colaborativo donde el cooldown de usuario es de 30 segundos), a cambio de recortar el tráfico de salida de 10 GB/s a menos de 80 MB/s en todo el clúster. Esto permite correr 100.000 usuarios concurrentes sobre hardware modesto de $300/mes en lugar de una infraestructura inmanejable de decenas de miles de dólares."*

---

## 📚 Índice de las 5 Preguntas Trampa Previas

| # | Proyecto | Cuello de Botella Auditado | Concepto Clave a Defender |
|---|---|---|---|
| **1** | Pixel War | Garbage Collector en Hub de WebSockets | Array `noscan` en lienzo vs buffers reutilizables en red. |
| **2** | Pixel War | Thundering Herd en Reconexiones Masivas | Snapshot precalculado y comprimido en CDN + deltas incrementales. |
| **3** | Arena .io | Head-of-Line Blocking de TCP en juegos a 30 TPS | Snapshots absolutos con secuencia (`seq & 31`) y migración teórica a WebTransport/UDP. |
| **4** | Uber Engine | Consistencia Redis Lua ante caídas del proceso Go | Atomicidad de Outbox dentro del script Lua + Reaper worker con TTL 15s. |
| **5** | Uber Engine | Contención en Redis GEO con 50.000 updates/s | Pipelining de batching en Go + particionado de claves geográficas por celdas. |
