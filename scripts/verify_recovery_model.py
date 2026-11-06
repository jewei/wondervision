#!/usr/bin/env python3
"""Model-check cross-session recovery drafts, then replay each model mutant in real code."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import urllib.request

from verify_save_model import JAR_HASH, JAR_URL

ROOT = Path(__file__).resolve().parents[1]
PRODUCTION = ROOT / 'internal/documents/recovery.go'
LOCK = '\tlock, err := recoveryLock(ctx, root)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer lock.Close()\n'

# Each model mutant has a real-code mutant that the Go tests must reject.
# (name, model flag, invariant it must violate, source edit (old, new, count) on PRODUCTION,
#  Go test, expected failure text)
MUTANTS = [
    # LOCK guards Write and remove; the first occurrence is Write's.
    ('lock', 'BrokenLock', 'NoUnreviewedLoss', (LOCK, '', 2),
     'TestConcurrentSessionRecoveryWrites', 'one unreviewed draft was overwritten'),
    ('claim', 'BrokenClaim', 'NoUnreviewedLoss',
     ('(plan.claim == nil || plan.claim.Path != r.Path || previous != *plan.claim)',
      '(plan.claim == nil || plan.claim.Path != r.Path)', 1),
     'TestStaleRecoveryClaimCannotReplaceNewerDraft', 'stale claim replaced a newer unreviewed draft'),
    ('discard', 'BrokenDiscard', 'NoUnreviewedLoss',
     ('(approved == nil || approved.Path != path || r != *approved)',
      '(approved == nil || approved.Path != path)', 1),
     'TestPreviousSessionDraftRequiresReview', 'stale discard approval deleted a newer copy'),
    ('cleanup', 'BrokenCleanup', 'NoLiveTempDeleted',
     ('if lock != nil && strings.HasPrefix(entry.Name(), recoveryTempPrefix)',
      'if strings.HasPrefix(entry.Name(), recoveryTempPrefix)', 1),
     'TestListRemovesOnlyOrphanedRecoveryTemps', 'List removed a temp file while another session held the lock'),
]


def run(command, cwd):
    return subprocess.run(command, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


def tlc_jar():
    jar = ROOT / 'build/tools/tla2tools-1.8.0.jar'
    if not jar.exists():
        jar.parent.mkdir(parents=True, exist_ok=True)
        with urllib.request.urlopen(JAR_URL) as response:
            jar.write_bytes(response.read())
    assert hashlib.sha256(jar.read_bytes()).hexdigest() == JAR_HASH, 'TLC jar checksum mismatch'
    return jar


def check_model(jar, out, name, flag, invariant=None):
    source = (ROOT / 'verification/recovery/Recovery.tla').read_text()
    config = (ROOT / 'verification/recovery/Recovery.cfg').read_text()
    with tempfile.TemporaryDirectory(prefix='wondervision-recovery-model-') as scratch:
        work = Path(scratch)
        (work / 'Recovery.tla').write_text(source)
        if flag:
            assert config.count(flag + ' = FALSE') == 1, 'model flag missing: ' + flag
            config = config.replace(flag + ' = FALSE', flag + ' = TRUE')
            # Check only the invariant this mutant targets: TLC reports the first
            # violation it reaches, which could otherwise be a different one.
            lines = [l for l in config.splitlines() if l.startswith('INVARIANTS ')]
            assert len(lines) == 1 and invariant in lines[0].split(), 'unknown invariant ' + invariant
            config = config.replace(lines[0], 'INVARIANTS ' + invariant)
        (work / 'Recovery.cfg').write_text(config)
        result = run(['java', '-XX:+UseParallelGC', '-cp', str(jar), 'tlc2.TLC', '-workers', '1',
                      '-metadir', str(work / 'states'), 'Recovery.tla', '-config', 'Recovery.cfg'], work)
    (out / ('model-' + name + '.txt')).write_text(result.stdout)
    if flag:
        assert result.returncode != 0 and ('Invariant ' + invariant + ' is violated') in result.stdout, \
            'model mutant ' + name + ' failed for an unexpected reason:\n' + result.stdout
        return 'model-' + name + ': counterexample found for ' + invariant
    assert result.returncode == 0 and 'No error has been found' in result.stdout, result.stdout
    return 'model-baseline: ' + [l for l in result.stdout.splitlines() if 'distinct states found' in l][-1]


def check_code_mutant(out, name, edit, test, failure):
    old, new, count = edit
    source = PRODUCTION.read_text()
    assert source.count(old) == count, \
        'recovery.go changed where the ' + name + ' mutant applies; review verification/recovery and this script'
    with tempfile.TemporaryDirectory(prefix='wondervision-recovery-mutant-') as scratch:
        work = Path(scratch)
        mutant = work / 'recovery.go'
        mutant.write_text(source.replace(old, new, 1))
        overlay = work / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(PRODUCTION): str(mutant)}}))
        result = run(['go', 'test', '-overlay=' + str(overlay), './internal/documents', '-run', test, '-count=1', '-v'], ROOT)
    (out / ('code-' + name + '.txt')).write_text(result.stdout)
    assert result.returncode != 0 and failure in result.stdout, \
        'real-code ' + name + ' mutant was not rejected by ' + test + ':\n' + result.stdout
    return 'code-' + name + ': ' + test + ' rejects it'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--out', type=Path, default=ROOT / 'build/verification/recovery-model')
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    # The unmodified code first: a real break shows up here as a failing test.
    baseline = run(['go', 'test', './internal/documents', '-count=1', '-v'], ROOT)
    (out / 'code-baseline.txt').write_text(baseline.stdout)
    assert baseline.returncode == 0, 'internal/documents tests fail on the current code:\n' + baseline.stdout
    jar = tlc_jar()
    summaries = ['code-baseline: internal/documents tests pass', check_model(jar, out, 'baseline', None)]
    for name, flag, invariant, edit, test, failure in MUTANTS:
        summaries.append(check_model(jar, out, name, flag, invariant))
        summaries.append(check_code_mutant(out, name, edit, test, failure))
    report = '\n'.join(summaries) + ('\nVERIFIED: no unreviewed draft is replaced or deleted and no in-progress temp file is cleaned up, '
                                     'for two sessions and up to 15 steps including crashes; each model mutant is rejected in real code.\n')
    (out / 'summary.txt').write_text(report)
    print(report)


if __name__ == '__main__':
    main()
