"""Summarize observed artifacts; missing or failed stages fail the run."""
import hashlib
import json
from pathlib import Path
import sys


def read(path):
    return json.loads(path.read_text(encoding="utf-8")) if path.is_file() else None


def main():
    root = Path(sys.argv[1])
    codes = dict(zip(("golden", "public_check", "public_tests", "version", "http"), map(int, sys.argv[2:]), strict=True))
    golden = read(root / "golden/evaluation-regression.json")
    http = read(root / "http/manifest.json")
    identity = read(root / "source-identity.json")
    passed = (all(code == 0 for code in codes.values()) and golden is not None
              and golden.get("regression", {}).get("passed") is True
              and golden.get("commit") == identity["commit"]
              and http is not None and http.get("status") == "passed"
              and http.get("quality_metrics_equal_after_rebuild") is True
              and http.get("paid_provider_requests") == 0)
    summary = {
        "status": "passed" if passed else "failed",
        "scope": "offline_fixture_engineering_regression",
        "commit": identity["commit"], "exit_codes": codes,
        "golden_dataset": golden.get("dataset") if golden else None,
        "golden_metrics": {m["metric"]: m["current"] for m in golden["regression"]["checks"]} if golden else None,
        "http_corpus_sha256": http.get("corpus_sha256") if http else None,
        "http_rounds": [{"round": r["round"], "attempts": r["attempts"], "metric": r["metric"]}
                        for r in http.get("rounds", [])] if http else [],
        "paid_provider_requests": http.get("paid_provider_requests") if http else None,
        "cost_scope": "Fixture ledger arithmetic only; no OpenRouter charge or catalog estimate.",
        "limitation": "Fixed answers and hash embeddings do not establish real model quality or supplier prompt cache benefit.",
    }
    (root / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    files = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
             for p in sorted(root.rglob("*")) if p.is_file() and p.name != "evidence-files.sha256.json"}
    (root / "evidence-files.sha256.json").write_text(json.dumps(files, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
