"""Captura una URL con Chrome headless via DevTools, esperando a que el JS y los WebSockets
carguen de verdad (el --screenshot de Chrome no espera al WebSocket del dashboard).

  python scripts/screenshot.py "http://localhost:8080/#monitor" docs/img/monitor.png 8 1600 1500
  python scripts/screenshot.py "http://localhost:3001/d/mediacase-main/mediacase?kiosk" docs/img/grafana.png 30

Requiere Chrome y `pip install websocket-client`.
"""
import base64, json, os, subprocess, sys, tempfile, time, urllib.request, websocket
url, out = sys.argv[1], sys.argv[2]
wait = float(sys.argv[3]) if len(sys.argv) > 3 else 8
w, h = (int(sys.argv[4]), int(sys.argv[5])) if len(sys.argv) > 5 else (1600, 1400)
chrome = r"C:\Program Files\Google\Chrome\Application\chrome.exe"
p = subprocess.Popen([chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
                      r"--user-data-dir=C:\tmp\mediacase\chrome-profile", "--remote-debugging-port=9333", "--remote-allow-origins=*",
                      f"--window-size={w},{h}", "about:blank"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(50):
        try:
            tabs = json.load(urllib.request.urlopen("http://127.0.0.1:9333/json")); break
        except Exception:
            time.sleep(0.3)
    ws = websocket.create_connection(tabs[0]["webSocketDebuggerUrl"], timeout=60)
    n = [0]
    def call(method, **params):
        n[0] += 1; ws.send(json.dumps({"id": n[0], "method": method, "params": params}))
        while True:
            r = json.loads(ws.recv())
            if r.get("id") == n[0]: return r.get("result", {})
    call("Emulation.setDeviceMetricsOverride", width=w, height=h, deviceScaleFactor=1, mobile=False)
    call("Page.enable"); call("Page.navigate", url=url)
    time.sleep(wait)
    data = call("Page.captureScreenshot", format="png", captureBeyondViewport=False)["data"]
    open(out, "wb").write(base64.b64decode(data)); print(out, len(data) // 1024, "KB")
finally:
    p.kill()
