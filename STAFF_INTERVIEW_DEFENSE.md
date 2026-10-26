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

## 📚 Índice de las 5 Preguntas Trampa Previas

| # | Proyecto | Cuello de Botella Auditado | Concepto Clave a Defender |
|---|---|---|---|
| **1** | Pixel War | Garbage Collector en Hub de WebSockets | Array `noscan` en lienzo vs buffers reutilizables en red. |
| **2** | Pixel War | Thundering Herd en Reconexiones Masivas | Snapshot precalculado y comprimido en CDN + deltas incrementales. |
| **3** | Arena .io | Head-of-Line Blocking de TCP en juegos a 30 TPS | Snapshots absolutos con secuencia (`seq & 31`) y migración teórica a WebTransport/UDP. |
| **4** | Uber Engine | Consistencia Redis Lua ante caídas del proceso Go | Atomicidad de Outbox dentro del script Lua + Reaper worker con TTL 15s. |
| **5** | Uber Engine | Contención en Redis GEO con 50.000 updates/s | Pipelining de batching en Go + particionado de claves geográficas por celdas. |
