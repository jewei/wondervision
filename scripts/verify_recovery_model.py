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
# (name, model flag, source edit (old, new, count) on PRODUCTION, Go test, expected failure text)
MUTANTS = [
    # LOCK guards Write and remove; the first occurrence is Write's.
    ('lock', 'BrokenLock', (LOCK, '', 2),
     'TestConcurrentSessionRecoveryWrites', 'one unreviewed draft was overwritten'),
    ('claim', 'BrokenClaim',
     ('(plan.claim == nil || plan.claim.Path != r.Path || previous != *plan.claim)',
      '(plan.claim == nil || plan.claim.Path != r.Path)', 1),
     'TestStaleRecoveryClaimCannotReplaceNewerDraft', 'stale claim replaced a newer unreviewed draft'),
    ('discard', 'BrokenDiscard',
     ('(approved == nil || approved.Path != path || r != *approved)',
      '(approved == nil || approved.Path != path)', 1),
     'TestPreviousSessionDraftRequiresReview', 'stale discard approval deleted a newer copy'),
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


def check_model(jar, out, name, flag):
    source = (ROOT / 'verification/recovery/Recovery.tla').read_text()
    config = (ROOT / 'verification/recovery/Recovery.cfg').read_text()
    with tempfile.TemporaryDirectory(prefix='wondervision-recovery-model-') as scratch:
        work = Path(scratch)
        (work / 'Recovery.tla').write_text(source)
        if flag:
            assert config.count(flag + ' = FALSE') == 1, 'model flag missing: ' + flag
            config = config.replace(flag + ' = FALSE', flag + ' = TRUE')
        (work / 'Recovery.cfg').write_text(config)
        result = run(['java', '-XX:+UseParallelGC', '-cp', str(jar), 'tlc2.TLC', '-workers', '1',
                      '-metadir', str(work / 'states'), 'Recovery.tla', '-config', 'Recovery.cfg'], work)
    (out / ('model-' + name + '.txt')).write_text(result.stdout)
    if flag:
        assert result.returncode != 0 and 'Invariant NoUnreviewedLoss is violated' in result.stdout, \
            'model mutant ' + name + ' failed for an unexpected reason:\n' + result.stdout
        return 'model-' + name + ': counterexample found for NoUnreviewedLoss'
    assert result.returncode == 0 and 'No error has been found' in result.stdout, result.stdout
    return 'model-baseline: ' + next(l for l in result.stdout.splitlines() if 'distinct states found' in l)


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
    for name, flag, edit, test, failure in MUTANTS:
        summaries.append(check_model(jar, out, name, flag))
        summaries.append(check_code_mutant(out, name, edit, test, failure))
    report = '\n'.join(summaries) + ('\nVERIFIED: no unreviewed draft is replaced or deleted, for two sessions '
                                     'and up to 10 steps including crashes; each model mutant is rejected in real code.\n')
    (out / 'summary.txt').write_text(report)
    print(report)


if __name__ == '__main__':
    main()
