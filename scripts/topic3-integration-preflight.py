#!/usr/bin/env python3
"""Read-only static integration gate. Exit 0: clear; 1: blocked; 2: invalid input.

Usage: python scripts/topic3-integration-preflight.py --manifest /path/to/integration-inputs.json
The manifest binds both worktrees, immutable commits, source hashes and required
files. Output is JSON on stdout; this tool never stages files or updates refs.
This static gate does not validate application behavior or database state.
"""
import argparse
from collections import defaultdict
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

MIGRATION = re.compile(r"^(\d+)[_-].+\.up\.sql$", re.IGNORECASE)
MARKER = re.compile(rb"^(?:<{7,}|>{7,}|\|{7,})(?:[ \t].*)?$")
SKIP_PARTS = {".git", "node_modules", "vendor", ".venv", "venv", "__pycache__"}
SKIP_EXTENSIONS = {".db", ".sqlite", ".sqlite3", ".pem", ".key", ".p12", ".pfx"}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git(repo, *args, optional=False):
    env = {**os.environ, "GIT_OPTIONAL_LOCKS": "0", "GIT_TERMINAL_PROMPT": "0"}
    result = subprocess.run(
        ["git", "-c", f"safe.directory={repo.as_posix()}", "-c", "core.fsmonitor=false", "-C", str(repo), *args],
        env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    if result.returncode and not optional:
        raise ValueError(f"Git read failed ({args[0]}): {result.stderr.decode('utf-8', errors='replace').strip()}")
    return result


def git_text(repo, *args):
    return git(repo, *args).stdout.decode("utf-8").strip()


def relative_file(root, name):
    path = root / name
    if Path(name).is_absolute() or not path.resolve().is_relative_to(root):
        raise ValueError(f"Input path leaves repository: {name}")
    return path


def permitted_text(name):
    path = Path(name)
    return not (
        any(part.lower() in SKIP_PARTS for part in path.parts)
        or path.name.lower().startswith(".env")
        or path.suffix.lower() in SKIP_EXTENSIONS
    )


def check(repo, manifest):
    required_keys = {
        "schema_version", "source", "candidate", "source_head", "source_branch", "candidate_branch",
        "checkpoint", "upstream", "source_index_sha256", "source_changed_hashes", "required_inputs",
    }
    if not isinstance(manifest, dict) or not required_keys.issubset(manifest) or manifest["schema_version"] != 1:
        raise ValueError("Manifest schema 1 and all identity/input fields are required")
    if not manifest["source_changed_hashes"] or not manifest["required_inputs"]:
        raise ValueError("Source hashes and required inputs must be nonempty")
    for field in ("source_head", "checkpoint", "upstream"):
        if not re.fullmatch(r"[0-9a-f]{40}", manifest[field]):
            raise ValueError(f"Manifest {field} must be a full commit ID")
    source = Path(manifest["source"]).resolve()
    candidate = Path(manifest["candidate"]).resolve()
    if source == candidate or repo == source:
        raise ValueError("Source and candidate must be separate worktrees; refusing to scan the source as candidate")
    if repo != candidate:
        raise ValueError("--repo does not match the candidate bound by the manifest")

    findings = []

    def block(code, message, **details):
        findings.append({"code": code, "message": message, **details})

    for path, label in ((repo, "candidate"), (source, "source")):
        if Path(git_text(path, "rev-parse", "--show-toplevel")).resolve() != path:
            raise ValueError(f"{label} must be its repository root")
    source_head = git_text(source, "rev-parse", "HEAD")
    head = git_text(repo, "rev-parse", "HEAD")
    source_branch = git_text(source, "symbolic-ref", "--short", "HEAD")
    branch = git_text(repo, "symbolic-ref", "--short", "HEAD")
    for field, actual in (("source_head", source_head), ("source_branch", source_branch), ("candidate_branch", branch)):
        if actual != manifest[field]:
            block("identity_mismatch", f"{field} differs from the manifest", expected=manifest[field], actual=actual)
    common = Path(git_text(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")).resolve()
    source_common = Path(git_text(source, "rev-parse", "--path-format=absolute", "--git-common-dir")).resolve()
    if common != source_common:
        block("identity_mismatch", "Source and candidate do not share the expected Git object store")
    for name in ("checkpoint", "upstream"):
        actual = git(repo, "rev-parse", "--verify", f"{manifest[name]}^{{commit}}", optional=True)
        if actual.returncode:
            block("missing_commit", f"Required {name} commit is unavailable", commit=manifest[name])
    if git(repo, "merge-base", "--is-ancestor", manifest["checkpoint"], head, optional=True).returncode:
        block("identity_mismatch", "Candidate HEAD does not descend from the complete working-state checkpoint")
    merge_head_result = git(repo, "rev-parse", "--verify", "MERGE_HEAD", optional=True)
    merge_head = merge_head_result.stdout.decode().strip() if merge_head_result.returncode == 0 else None
    if merge_head and merge_head != manifest["upstream"]:
        block("identity_mismatch", "Open merge uses a different upstream", actual=merge_head)
    if not merge_head and git(repo, "merge-base", "--is-ancestor", manifest["upstream"], head, optional=True).returncode:
        block("missing_upstream_merge", "The fixed upstream is neither merged nor the current merge input")
    source_index = Path(git_text(source, "rev-parse", "--path-format=absolute", "--git-path", "index"))
    if digest(source_index) != manifest["source_index_sha256"]:
        block("source_changed", "Source index differs from the preserved checkpoint evidence")
    for name, expected in manifest["source_changed_hashes"].items():
        path = relative_file(source, name)
        if not path.is_file() or digest(path) != expected:
            block("source_changed", "Preserved source file is missing or changed", path=name)
    relocations = manifest.get("relocated_inputs", {})
    if not isinstance(relocations, dict):
        raise ValueError("relocated_inputs must be an explicit path mapping")
    required = set(manifest["required_inputs"]) | set(manifest["source_changed_hashes"])
    if set(relocations) - required:
        raise ValueError("A relocation must identify a required preserved input")
    for name in sorted(required):
        relocation = relocations.get(name)
        target = relocation["path"] if relocation else name
        path = relative_file(repo, target)
        if not path.is_file():
            block("missing_input", "Required integration source file is missing", path=target, source_path=name)
        elif relocation:
            original = relative_file(source, name)
            expected = relocation["sha256"]
            candidate_matches = digest(path) == expected
            if relocation.get("line_endings") == "lf":
                canonical = relocation["lf_sha256"]
                candidate_matches = hashlib.sha256(path.read_bytes().replace(b"\r\n", b"\n")).hexdigest() == canonical
                if original.is_file():
                    candidate_matches = candidate_matches and hashlib.sha256(original.read_bytes().replace(b"\r\n", b"\n")).hexdigest() == canonical
            elif "line_endings" in relocation:
                raise ValueError("Only explicit LF normalization is supported for relocated SQL")
            if not original.is_file() or digest(original) != expected or not candidate_matches:
                block("relocated_input_changed", "Relocated input bytes differ from the preserved source", path=target, source_path=name)

    entries = git(repo, "ls-files", "--unmerged", "-z").stdout.split(b"\0")
    unmerged = defaultdict(set)
    for entry in entries:
        if entry:
            metadata, name = entry.split(b"\t", 1)
            unmerged[name.decode("utf-8")].add(int(metadata.split()[2]))
    for name, stages in sorted(unmerged.items()):
        block("git_unmerged", "Git index retains unresolved merge stages", path=name, stages=sorted(stages))

    files = sorted({p.decode("utf-8") for p in git(repo, "ls-files", "--cached", "--others", "--exclude-standard", "-z").stdout.split(b"\0") if p})
    migrations = defaultdict(list)
    marker_paths = []
    skipped_paths = []
    for name in files:
        path = relative_file(repo, name)
        match = MIGRATION.fullmatch(path.name)
        if match and path.is_file():
            migrations[(Path(name).parent.as_posix(), int(match.group(1)))].append(name)
        if not path.is_file() or not permitted_text(name):
            skipped_paths.append(name)
            continue
        with path.open("rb") as stream:
            probe = stream.read(8192)
            if b"\0" in probe:
                continue
            stream.seek(0)
            lines = [number for number, line in enumerate(stream, 1) if MARKER.fullmatch(line.rstrip(b"\r\n"))]
        if lines:
            marker_paths.append(name)
            block("text_conflict", "Working file contains unresolved conflict markers", path=name, lines=lines)
    duplicates = []
    for (directory, version), names in sorted(migrations.items()):
        if len(names) > 1:
            item = {"directory": directory, "version": version, "paths": names}
            duplicates.append(item)
            block("duplicate_migration", "Multiple forward migrations share one version in the same directory", **item)
    return {
        "result": "blocked" if findings else "clear", "scope": "read-only static integration preflight",
        "application_validated": False,
        "identity": {"source": source.as_posix(), "source_head": source_head, "source_branch": source_branch,
                     "candidate": repo.as_posix(), "candidate_branch": branch, "head": head,
                     "checkpoint": manifest["checkpoint"], "upstream": manifest["upstream"], "merge_head": merge_head},
        "summary": {"git_unmerged_files": len(unmerged), "text_conflict_files": len(marker_paths),
                    "duplicate_migration_versions": len(duplicates), "scanned_git_paths": len(files),
                    "source_hashes_checked": len(manifest["source_changed_hashes"]),
                    "required_inputs_checked": len(set(manifest["required_inputs"]) | set(manifest["source_changed_hashes"])),
                    "blocking_findings": len(findings)},
        "duplicate_migrations": duplicates, "excluded_runtime_or_secret_paths": skipped_paths,
        "findings": findings,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()
    try:
        manifest = json.loads(args.manifest.read_text(encoding="utf-8-sig"))
        result = check(args.repo.resolve(), manifest)
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(json.dumps({"result": "invalid_input", "error": str(error), "application_validated": False}, ensure_ascii=False))
        return 2
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 1 if result["findings"] else 0


if __name__ == "__main__":
    sys.exit(main())
