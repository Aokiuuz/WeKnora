"""Run frozen PDF samples through production Go adapters; secrets use stdin only."""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ENGINES = ('builtin', 'markitdown', 'opendataloader', 'weknoracloud', 'mineru', 'mineru_cloud', 'paddleocr_vl', 'paddleocr_vl_cloud')


def docker(*args: str) -> str:
    p = subprocess.run(['docker', *args], capture_output=True, text=True, encoding='utf-8', timeout=90)
    if p.returncode:
        raise RuntimeError(f'Docker operation failed: {args[0]} (exit {p.returncode})')
    return p.stdout.strip()


def read_credentials() -> dict:
    # Do not print inspect results, database values, headers, or the payload.
    container = json.loads(docker('inspect', 'weknora-personal-app-1'))[0]
    env = dict(s.split('=', 1) for s in container['Config']['Env'] if '=' in s)
    sql = "SELECT json_build_object('cloud',credentials::jsonb->'weknoracloud','parser_config',parser_engine_config) FROM tenants WHERE id=10000;"
    payload = json.loads(docker('exec', 'weknora-personal-postgres-1', 'psql', '-U', 'postgres', '-d', 'WeKnora', '-At', '-c', sql))
    payload['aes_key'] = env.get('SYSTEM_AES_KEY', '')
    config = payload.setdefault('parser_config', {}) or {}
    payload['parser_config'] = config
    config['mineru_endpoint'] = 'http://weknora-parser-mineru:8000'
    config['paddleocr_vl_endpoint'] = 'http://weknora-parser-paddle:8080'
    return payload


def container_path(value: str) -> str:
    path = Path(value).resolve()
    try:
        return '/workspace/' + path.relative_to(ROOT).as_posix()
    except ValueError:
        raise ValueError('Manifest and output must stay inside benchmark worktree') from None


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--manifest', required=True)
    p.add_argument('--output', required=True)
    p.add_argument('--engine', choices=ENGINES, required=True)
    p.add_argument('--binary', default='artifacts/parser-benchmark/bin/parser-benchmark', help='Frozen adapter executable; use a distinct path for another build')
    p.add_argument('--timeout', default='10m')
    p.add_argument('--execute', action='store_true')
    p.add_argument('--max-samples', type=int, default=100)
    p.add_argument('--shard-index', type=int, default=0)
    p.add_argument('--shard-count', type=int, default=1)
    args = p.parse_args()
    if not (1 <= args.shard_count <= 4 and 0 <= args.shard_index < args.shard_count):
        raise ValueError('invalid shard count/index')
    if args.shard_count > 1:
        original = json.loads(Path(args.manifest).read_text(encoding='utf-8'))
        if len(original['samples']) > args.max_samples:
            raise ValueError('Complete manifest exceeds cap before sharding')
        original['samples'] = original['samples'][args.shard_index::args.shard_count]
        original['shard'] = {'index': args.shard_index, 'count': args.shard_count}
        shard_path = ROOT / 'artifacts/parser-benchmark/shards' / (Path(args.manifest).stem + f'-{args.shard_index}-of-{args.shard_count}.json')
        shard_path.parent.mkdir(parents=True, exist_ok=True)
        content = json.dumps(original, ensure_ascii=False, indent=2) + '\n'
        if shard_path.exists() and shard_path.read_text(encoding='utf-8') != content:
            raise ValueError('Frozen shard identity changed')
        shard_path.write_text(content, encoding='utf-8')
        args.manifest = str(shard_path)
    cmd = ['docker', 'run', '--rm', '-i', '--name', f'weknora-parser-run-{args.engine}-{args.shard_index}', '--network', 'weknora-personal_default', '--memory', '1g', '--cpus', '2',
           '-e', 'SSRF_WHITELIST=weknora-parser-mineru,weknora-parser-paddle', '-e', 'LOG_LEVEL=fatal', '-e', 'JIEBA_DICT_DIR=/workspace/artifacts/parser-benchmark/bin/jieba',
           '-v', f'{ROOT}:/workspace', '-w', '/workspace', '--entrypoint', container_path(args.binary),
           'weknora-nogit-go:1.26', '--root', '/workspace', '--manifest', container_path(args.manifest), '--output', container_path(args.output),
           '--engine', args.engine, '--timeout', args.timeout, '--max-samples', str(args.max_samples), '--docreader', 'weknora-parser-docreader:50051']
    payload = b''
    if args.execute:
        cmd.append('--execute')
        payload = json.dumps(read_credentials(), ensure_ascii=False).encode('utf-8')
    # Only public paths and flags enter the command line. Secret bytes are never
    # persisted or included in tool output; Go emits redacted per-page records.
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE)
    proc.communicate(payload)
    return proc.returncode


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f'Benchmark launcher failed ({type(exc).__name__}); no credentials emitted.', file=sys.stderr)
        raise SystemExit(1)
