"""End-to-end query smoke test against the local Grafana + OpenObserve."""
import json
import os
import subprocess
import sys
import time

GRAFANA = os.environ.get("GRAFANA_URL", "http://127.0.0.1:3000")
AUTH = "admin:admin"
DS_UID = sys.argv[1] if len(sys.argv) > 1 else "P4A5AAF418F5D059C"


def query(model, frm=None, to=None, max_points=200):
    now = int(time.time() * 1000)
    payload = {
        "queries": [
            {
                "refId": "A",
                "datasource": {"type": "linpr-openobserve-datasource", "uid": DS_UID},
                "intervalMs": 60000,
                "maxDataPoints": max_points,
                **model,
            }
        ],
        "from": str(frm if frm is not None else now - 3600 * 1000),
        "to": str(to if to is not None else now),
    }
    proc = subprocess.run(
        ["curl", "-s", "-u", AUTH, "-X", "POST", f"{GRAFANA}/api/ds/query",
         "-H", "Content-Type: application/json", "-d", json.dumps(payload)],
        capture_output=True, text=True,
    )
    try:
        return json.loads(proc.stdout)
    except Exception:
        return {"raw": proc.stdout}


def summarize(resp):
    res = resp.get("results", {}).get("A", {})
    out = {"status": res.get("status"), "error": res.get("error"), "frames": []}
    for frame in res.get("frames", []):
        schema = frame.get("schema", {})
        meta = schema.get("meta", {}) or {}
        values = frame.get("data", {}).get("values", [])
        out["frames"].append({
            "name": schema.get("name"),
            "preferred": meta.get("preferredVisualisationType"),
            "type": meta.get("type"),
            "fields": [(f["name"], f.get("type")) for f in schema.get("fields", [])][:6],
            "rows": len(values[0]) if values else 0,
        })
    return out


CASES = [
    ("logs", {"queryType": "logs", "rawSql": 'SELECT * FROM "vnet_bff_dev" LIMIT 5', "format": "logs"}),
    ("sql table", {"queryType": "logs", "rawSql": 'SELECT service_name, count(*) AS c FROM "vnet_bff_dev" GROUP BY service_name ORDER BY c DESC LIMIT 10', "format": "table"}),
    ("histogram", {"queryType": "logs", "rawSql": "SELECT histogram(_timestamp, '1 minute') AS ts, count(*) AS cnt FROM \"vnet_bff_dev\" GROUP BY ts ORDER BY ts", "format": "timeseries"}),
    ("metrics multi-series", {"queryType": "metrics", "rawSql": "SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS val, service_name FROM \"go_goroutine_count\" GROUP BY ts, service_name ORDER BY ts", "format": "auto"}),
    ("promql", {"queryType": "metrics", "queryMode": "promql", "expr": "go_goroutine_count"}),
    ("traces", {"queryType": "traces", "rawSql": 'SELECT * FROM "prd" ORDER BY start_time DESC LIMIT 200', "format": "trace"}),
    ("adhoc filter", {"queryType": "logs", "rawSql": 'SELECT * FROM "vnet_bff_dev" LIMIT 3', "format": "table",
                      "adhocFilters": [{"key": "service_name", "value": "netops-job-scheduler", "operator": "="}]}),
    ("template variable", {"queryType": "logs", "rawSql": 'SELECT DISTINCT service_name FROM "vnet_bff_dev" LIMIT 10', "format": "table"}),
    ("annotations", {"queryType": "logs", "rawSql": 'SELECT _timestamp AS time, body AS text FROM "vnet_bff_dev" LIMIT 3', "format": "annotations"}),
    ("bad sql", {"queryType": "logs", "rawSql": 'SELECT nope FROM "no_such_stream"', "format": "table"}),
    ("non-select", {"queryType": "logs", "rawSql": 'DELETE FROM "vnet_bff_dev"', "format": "table"}),
]

if __name__ == "__main__":
    failures = 0
    for name, model in CASES:
        summary = summarize(query(model))
        expected_error = name in ("bad sql", "non-select")
        ok = (summary["error"] is not None) if expected_error else (summary["error"] is None and summary["frames"])
        if not ok:
            failures += 1
        print(f"[{'PASS' if ok else 'FAIL'}] {name}: {json.dumps(summary, ensure_ascii=False)[:400]}")
    print("failures:", failures)
    sys.exit(1 if failures else 0)
