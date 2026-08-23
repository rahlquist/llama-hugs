#!/usr/bin/env python3
"""Export nightly-sweep benchmark_runs (SQLite) to Llama Hugs JSON lines.

One JSON object per line, shaped for POST /api/hugs/bench/ingest's sibling
format: {"model","task","tokens_per_s","run_at_unix","notes"}.
Model names resolved via the models table (filename stem).
"""
import json
import sqlite3
import sys

DB = "/home/rahlquist/wimpy-setup/benching/bench.db"
OUT = sys.argv[1] if len(sys.argv) > 1 else "/tmp/hugs-bench.jsonl"

db = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
models = {mid: fn for mid, fn in db.execute("SELECT id, filename FROM models")}

n = 0
with open(OUT, "w") as f:
    for model_id, run_at, test_name, avg_ts, notes in db.execute(
        "SELECT model_id, date_run, test_name, avg_ts, notes FROM benchmark_runs"
    ):
        fname = models.get(model_id, f"model-{model_id}")
        rec = {
            "model": fname.removesuffix(".gguf"),
            "task": test_name,
            "tokens_per_s": float(avg_ts),
            "run_at_unix": int(run_at),
            "notes": notes or "",
        }
        f.write(json.dumps(rec) + "\n")
        n += 1
print(f"wrote {n} records to {OUT}")
