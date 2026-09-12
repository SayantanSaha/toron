import json
import os
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler

request_count = 0

class OriginHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _send_json(self, status, payload, seq):
        body = json.dumps(payload).encode('utf-8')
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("X-Backend-Runtime", "python-uvicorn-h11")
        self.send_header("X-Backend-Seq", str(seq))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        global request_count
        request_count += 1
        seq = request_count

        if self.path == "/health":
            self._send_json(200, {
                "status": "ok",
                "runtime": "python",
                "engine": "uvicorn/h11",
                "requests_handled": seq
            }, seq)
        elif self.path == "/canary":
            self._send_json(200, {
                "status": "canary_ok",
                "runtime": "python",
                "engine": "uvicorn/h11",
                "sequence": seq
            }, seq)
        else:
            self._send_json(200, {
                "status": "ok",
                "path": self.path,
                "sequence": seq
            }, seq)

    def do_POST(self):
        global request_count
        request_count += 1
        seq = request_count

        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8', errors='replace')

        if self.path == "/echo":
            self._send_json(200, {
                "status": "echo",
                "runtime": "python",
                "engine": "uvicorn/h11",
                "sequence": seq,
                "body_length": len(body),
                "headers": dict(self.headers),
                "body": body
            }, seq)
        else:
            self._send_json(200, {
                "status": "ok",
                "path": self.path,
                "sequence": seq
            }, seq)

    def log_message(self, format, *args):
        pass

def run():
    port = int(os.environ.get("PORT", "9102"))
    server = HTTPServer(("0.0.0.0", port), OriginHandler)
    print(f"Python origin listening on port {port}")
    server.serve_forever()

if __name__ == "__main__":
    run()
