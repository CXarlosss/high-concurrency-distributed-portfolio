import ws from 'k6/ws';
import { check } from 'k6';
import { sleep } from 'k6';

export const options = {
  stages: [
    { duration: '20s', target: 1000 }, // Rampa de subida agresiva
    { duration: '40s', target: 5000 }, // Mantenimiento de carga extrema (5k WebSockets)
    { duration: '20s', target: 0 },    // Rampa de bajada
  ],
};

export default function () {
  // Ajusta el puerto si tu servidor Go expone otro distinto
  const url = 'ws://127.0.0.1:8080/ws'; 

  const res = ws.connect(url, {}, function (socket) {
    socket.on('open', () => {
      // Simulamos que el usuario pinta un píxel cada 3 segundos
      socket.setInterval(function timeout() {
        let buffer = new ArrayBuffer(5);
        let view = new DataView(buffer);
        // Coordenadas aleatorias de 0 a 999 (Big-Endian, 'false' en k6 DataView)
        view.setUint16(0, Math.floor(Math.random() * 1000), false); // X
        view.setUint16(2, Math.floor(Math.random() * 1000), false); // Y
        view.setUint8(4, Math.floor(Math.random() * 32));           // Color 0-31
        
        socket.send(buffer);
      }, 3000);
    });

    socket.on('message', (msg) => {
      // Consumimos el broadcast de deltas masivo para medir ancho de banda real
    });

    socket.setTimeout(function () {
      socket.close();
    }, 60000); // Forzamos desconexión tras 60s para evaluar limpieza de goroutines
  });

  check(res, { 'Conexión WS exitosa (101)': (r) => r && r.status === 101 });
}
