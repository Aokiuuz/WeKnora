"""Connect verified local parsers to the personal app without replacing credentials."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
ENDPOINTS = {'mineru_endpoint': 'http://weknora-parser-mineru:8000',
             'paddleocr_vl_endpoint': 'http://weknora-parser-paddle:8080'}
HOSTS = ['weknora-parser-mineru', 'weknora-parser-paddle']


def command(args, data=None):
    r = subprocess.run(args, input=data, capture_output=True, text=True, encoding='utf-8')
    if r.returncode:
        raise RuntimeError(f'{args[0]} {args[1]} failed (exit {r.returncode}); private output suppressed')
    return r.stdout.strip()


def query(sql):
    return command(['docker', 'exec', '-i', 'weknora-personal-postgres-1', 'psql',
                    '-U', 'postgres', '-d', 'WeKnora', '-At', '-v', 'ON_ERROR_STOP=1'], sql)


def add_hosts(text):
    lines = text.splitlines()
    key = 'SSRF_WHITELIST_EXTRA='
    indices = [i for i, line in enumerate(lines) if line.startswith(key)]
    if len(indices) > 1:
        raise ValueError('Duplicate SSRF_WHITELIST_EXTRA entries require inspection')
    values = [] if not indices else lines[indices[0]][len(key):].strip().strip('\"\'').split(',')
    values = [v.strip() for v in values if v.strip()]
    for host in HOSTS:
        if host not in values:
            values.append(host)
    line = key + ','.join(values)
    if indices:
        lines[indices[0]] = line
    else:
        lines.append(line)
    return '\n'.join(lines) + '\n'


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--app-root', type=Path, default=ROOT.parent / 'WeKnora-topic3-0.8')
    p.add_argument('--tenant-id', type=int, default=10000)
    p.add_argument('--apply', action='store_true')
    args = p.parse_args()
    if args.tenant_id < 1:
        raise ValueError('Invalid tenant ID')
    app = args.app_root.resolve()
    env_path = app / '.local-service/runtime.env'
    original_env = env_path.read_bytes()
    updated_env = add_hosts(original_env.decode('utf-8-sig')).encode('utf-8')
    original = json.loads(query(f'SELECT coalesce(parser_engine_config,\'{{}}\'::jsonb) FROM tenants WHERE id={args.tenant_id};'))
    if not args.apply:
        print(json.dumps({'action': 'preflight', 'endpoints': ENDPOINTS, 'existing_configuration_preserved': True,
                          'will_add_exact_hosts': HOSTS, 'network_or_database_mutations': 0}))
        return
    # Local liveness is checked before changing application configuration.
    for port, path in [(18081, '/docs'), (18082, '/health')]:
        with urllib.request.urlopen(f'http://127.0.0.1:{port}{path}', timeout=10) as response:
            if response.status != 200:
                raise RuntimeError('Parser HTTP endpoint is not ready')
    stamp = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    backup = app / '.local-service/backups' / ('parser-connection-' + stamp)
    backup.mkdir(parents=True, exist_ok=False)
    (backup / 'runtime.env').write_bytes(original_env)
    (backup / 'parser-engine-config.json').write_text(json.dumps(original, ensure_ascii=False, indent=2), encoding='utf-8')
    patch = json.dumps(ENDPOINTS)
    sql = (f"BEGIN; UPDATE tenants SET parser_engine_config=coalesce(parser_engine_config,'{{}}'::jsonb) || '{patch}'::jsonb, "
           f"updated_at=now() WHERE id={args.tenant_id}; COMMIT;")
    query(sql)
    after = json.loads(query(f'SELECT parser_engine_config FROM tenants WHERE id={args.tenant_id};'))
    if {k: v for k, v in original.items() if k not in ENDPOINTS} != {k: v for k, v in after.items() if k not in ENDPOINTS}:
        raise RuntimeError('Concurrent configuration change detected; inspect private backup before continuing')
    env_path.write_bytes(updated_env)
    command(['docker', 'compose', '--env-file', str(env_path), '-f', str(app / 'docker-compose.personal.yml'),
             'up', '-d', '--no-deps', '--wait', '--wait-timeout', '120', 'app'])
    # The Nginx template resolves the app hostname when its config is loaded.
    command(['docker', 'exec', 'weknora-personal-frontend-1', 'nginx', '-s', 'reload'])
    report = {'connected_at': stamp, 'tenant_id': args.tenant_id, 'endpoints': ENDPOINTS,
              'precise_allowlist_hosts': HOSTS, 'other_parser_fields_unchanged': True,
              'previous_env_sha256': hashlib.sha256(original_env).hexdigest(),
              'current_env_sha256': hashlib.sha256(updated_env).hexdigest(), 'private_backup': str(backup)}
    output = ROOT / 'artifacts/parser-benchmark/connection.json'
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report))


if __name__ == '__main__':
    main()
