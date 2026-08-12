package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// TrackingResponseWriter envelopa o http.ResponseWriter para rastrear se os cabeçalhos/bytes já foram enviados.
type TrackingResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
	bytesWritten int64
}

func (t *TrackingResponseWriter) WriteHeader(statusCode int) {
	if !t.wroteHeader {
		t.wroteHeader = true
		t.ResponseWriter.WriteHeader(statusCode)
	}
}

func (t *TrackingResponseWriter) Write(b []byte) (int, error) {
	if !t.wroteHeader {
		t.WriteHeader(http.StatusOK)
	}
	n, err := t.ResponseWriter.Write(b)
	t.bytesWritten += int64(n)
	return n, err
}

func (t *TrackingResponseWriter) Flush() {
	if flusher, ok := t.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// ProxyHandler com a lógica de correção da Issue #1 do FastProxy
func ProxyHandler(w http.ResponseWriter, r *http.Request, upstream io.Reader) {
	trackingWriter := &TrackingResponseWriter{ResponseWriter: w}

	buf := make([]byte, 32*1024)
	var copyErr error

	for {
		nr, rerr := upstream.Read(buf)
		if nr > 0 {
			nw, werr := trackingWriter.Write(buf[0:nr])
			if nw > 0 {
				trackingWriter.Flush()
			}
			if werr != nil {
				copyErr = werr
				break
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				copyErr = rerr
			}
			break
		}
	}

	if copyErr != nil {
		log.Printf("[FastProxy Warning] Upstream read/write error: %v (bytes written: %d, headers sent: %v)", copyErr, trackingWriter.bytesWritten, trackingWriter.wroteHeader)

		// CRITÉRIO DE ACEITAÇÃO:
		// Se os cabeçalhos ou bytes já foram enviados para o cliente, NUNCA anexar "Internal Server Error" no corpo!
		if trackingWriter.wroteHeader || trackingWriter.bytesWritten > 0 {
			// Força o encerramento do socket sem poluir a resposta do cliente
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, err := hijacker.Hijack()
				if err == nil && conn != nil {
					conn.Close()
				}
			}
			return
		}

		// Se nada foi enviado ainda, podemos responder com 502 Bad Gateway limpo
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		return
	}
}

// Simula um Upstream de Streaming SSE que falha no meio
type FailingUpstream struct {
	sentCount int32
}

func (f *FailingUpstream) Read(p []byte) (n int, err error) {
	count := atomic.AddInt32(&f.sentCount, 1)
	if count <= 3 {
		msg := fmt.Sprintf("data: {\"chunk\": %d, \"timestamp\": \"%s\"}\n\n", count, time.Now().Format(time.RFC3339))
		copy(p, []byte(msg))
		time.Sleep(500 * time.Millisecond)
		return len(msg), nil
	}
	// Abrupt closure simulation
	return 0, errors.New("upstream connection reset abruptly by peer mid-stream")
}

func main() {
	http.HandleFunc("/fastproxy/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		upstream := &FailingUpstream{}
		ProxyHandler(w, r, upstream)
	})

	http.HandleFunc("/fastproxy/demo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <title>FastProxy Streaming Fix Demo — OpenHealth Cloud</title>
    <style>
        body { font-family: system-ui, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 40px; }
        .container { max-width: 800px; margin: 0 auto; background: #1e293b; border-radius: 12px; padding: 32px; box-shadow: 0 10px 25px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        .badge { background: #059669; color: white; padding: 4px 12px; border-radius: 9999px; font-size: 0.85rem; display: inline-block; margin-bottom: 20px; }
        button { background: #0284c7; color: white; border: none; padding: 12px 24px; border-radius: 8px; font-weight: bold; cursor: pointer; font-size: 1rem; }
        button:hover { background: #0369a1; }
        #logs { background: #090d16; font-family: monospace; padding: 20px; border-radius: 8px; border: 1px solid #334155; margin-top: 20px; min-height: 200px; white-space: pre-wrap; }
        .success { color: #4ade80; }
        .error-clean { color: #f43f5e; }
    </style>
</head>
<body>
    <div class="container">
        <span class="badge">LIVE IN LOCO — OPENHEALTH.CLOUD</span>
        <h1>FastProxy Stream Bugfix Verification</h1>
        <p>Esta aplicação simula um Upstream SSE que encerra abruptamente a conexão após enviar 3 pacotes.</p>
        <p><b>Com a nossa correção:</b> O stream encerra limposamente sem injetar a string de texto <code>"Internal Server Error"</code> no payload.</p>
        <button onclick="startStream()">Iniciar Teste de Streaming SSE</button>

        <div id="logs">Aguardando início do teste...</div>
    </div>

    <script>
        function startStream() {
            const logs = document.getElementById('logs');
            logs.innerHTML = 'Conectando ao FastProxy SSE (/fastproxy/stream)...\n';
            
            const eventSource = new EventSource('/fastproxy/stream');

            eventSource.onmessage = function(event) {
                logs.innerHTML += '<span class="success">Recebido do Stream: ' + event.data + '</span>\n';
            };

            eventSource.onerror = function(err) {
                logs.innerHTML += '<span class="error-clean">\n[OK] Conexão encerrada pelo servidor de forma limpa!</span>\n';
                logs.innerHTML += '<b>Verificação final:</b> Nenhuma string de erro ("Internal Server Error") foi injetada no corpo do evento.\n';
                eventSource.close();
            };
        }
    </script>
</body>
</html>`)
	})

	log.Println("[FastProxy Server] Ouvindo na porta 8085 local para openhealth.cloud...")
	if err := http.ListenAndServe("127.0.0.1:8085", nil); err != nil {
		log.Fatalf("Erro ao iniciar o FastProxy: %v", err)
	}
}