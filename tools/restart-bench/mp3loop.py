# Serves one MP3 file as an endless live stream, at its own bitrate, for
# tools/restart-bench/restart.mjs: a source that never ends and is never late.
#   python3 mp3loop.py <file.mp3> <port> <bytes per second>
# third_party/goxiph/mp3/testdata/clairance/station-8k-vbr.mp3: 1622 B/s.
import sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
path, port, rate = sys.argv[1], int(sys.argv[2]), float(sys.argv[3])
data = open(path, "rb").read()
class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.0"
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "audio/mpeg")
        self.end_headers()
        start, sent, i = time.monotonic(), 0, 0
        try:
            while True:
                chunk = data[i:i + 1024]
                i = (i + 1024) % len(data)
                self.wfile.write(chunk)
                sent += len(chunk)
                ahead = sent / rate - (time.monotonic() - start)
                if ahead > 0.5:
                    time.sleep(ahead - 0.5)
        except (BrokenPipeError, ConnectionResetError):
            pass
    def log_message(self, *a): pass
ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
