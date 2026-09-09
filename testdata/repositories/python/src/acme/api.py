"""Run the API service independently with python -m acme.api."""
from http.server import BaseHTTPRequestHandler, HTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ready")

def main():
    server = HTTPServer(("127.0.0.1", 8111), Handler)
    server.serve_forever()

if __name__ == "__main__":
    main()
