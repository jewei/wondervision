#!/usr/bin/env python3
"""Recheck recovery lifetimes and reject a missing cross-process write lock."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def check(command, out):
    result = subprocess.run(command, cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    out.write_text(result.stdout)
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--out', type=Path, default=ROOT / 'build/verification/recovery-lock')
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    production = ROOT / 'internal/documents/recovery.go'
    source = production.read_text()
    lock = '\tlock, err := recoveryLock(ctx, root)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer lock.Close()\n'
    assert source.count(lock) == 2, 'recovery lock boundaries changed; review the mutation and concurrency test'
    # The first block guards Write. Keep Remove and the working tree unchanged.
    with tempfile.TemporaryDirectory(prefix='wondervision-recovery-mutant-') as raw:
        work = Path(raw)
        mutant = work / 'recovery.go'
        mutant.write_text(source.replace(lock, '', 1))
        overlay = work / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(production): str(mutant)}}))
        broken = check(['go', 'test', '-overlay=' + str(overlay), './internal/documents', '-run', 'TestConcurrentSessionRecoveryWrites', '-count=1', '-v'], out / 'mutant.txt')
        assert broken.returncode != 0 and 'one unreviewed draft was overwritten' in broken.stdout, broken.stdout
    restored = check(['go', 'test', './internal/documents', './internal/app', '-run', 'Test(Recovery|PreviousSession|SensitiveRecovery|UnreviewedRecovery|ConcurrentSessionRecoveryWrites|ReloadRemovesRecoveredClaim|ClaimDiscardFailureBlocksQuit|RecoveredRenameCleansOldClaim|DiscardedDraftCleanup|DiscardQuitBlocksWhenCleanupCannotBeQueued)', '-count=1', '-v'], out / 'restored.txt')
    assert restored.returncode == 0, restored.stdout
    print('VERIFIED: recovery regressions pass; removing the write lock reproduces concurrent-session data loss. This is empirical verification, not a formal proof.')


if __name__ == '__main__':
    main()
