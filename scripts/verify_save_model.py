#!/usr/bin/env python3
"""Check the finite save protocol, mutant counterexamples, and real-code replays."""
import argparse
import hashlib
from pathlib import Path
import shutil
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
JAR_HASH = '7a0a1e90628ebff2e3e8733375f8b684fa59504d2e02bddc4cb51a7679bb7e04'
JAR_URL = 'https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar'


def run(command, cwd):
    return subprocess.run(command, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--out', type=Path, default=ROOT / 'build/verification/save-model')
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    jar = ROOT / 'build/tools/tla2tools-1.8.0.jar'
    if not jar.exists():
        jar.parent.mkdir(parents=True, exist_ok=True)
        with urllib.request.urlopen(JAR_URL) as response:
            jar.write_bytes(response.read())
    assert hashlib.sha256(jar.read_bytes()).hexdigest() == JAR_HASH, 'TLC jar checksum mismatch'
    version = subprocess.check_output(['go', 'version'], text=True)
    assert 'go1.27.3 ' in version, 'use the pinned Go 1.27.3 toolchain'
    source = (ROOT / 'verification/save/Save.tla').read_text()
    config = (ROOT / 'verification/save/Save.cfg').read_text()
    cases = [('baseline', None, None), ('broken-clean', 'BrokenClean', 'NoFalseClean'), ('broken-conflict', 'BrokenConflict', 'KnownConflictsNeverWritten'), ('broken-approval', 'BrokenApproval', 'ExpectedIsReviewed'), ('final-window-limit', None, 'NoRacedOverwrite')]
    summaries = []
    for name, broken, expected in cases:
        with tempfile.TemporaryDirectory(prefix='wondervision-save-model-') as scratch:
            work = Path(scratch)
            (work / 'Save.tla').write_text(source)
            cfg = config.replace(broken + ' = FALSE', broken + ' = TRUE') if broken else config
            if name == 'final-window-limit':
                cfg += '\nINVARIANT NoRacedOverwrite\n'
            (work / 'Save.cfg').write_text(cfg)
            result = run(['java', '-XX:+UseParallelGC', '-cp', str(jar), 'tlc2.TLC', '-workers', '1', '-metadir', str(work / 'states'), 'Save.tla', '-config', 'Save.cfg'], work)
            (out / (name + '.txt')).write_text(result.stdout)
            if expected:
                assert result.returncode != 0 and ('Invariant ' + expected + ' is violated') in result.stdout, name + ' failed for an unexpected reason:\n' + result.stdout
                summaries.append(name + ': counterexample found for ' + expected)
            else:
                assert result.returncode == 0 and 'No error has been found' in result.stdout, result.stdout
                summaries.extend(line for line in result.stdout.splitlines() if 'distinct states found' in line)
    baseline = run(['go', 'test', './internal/documents', './internal/app', '-run', 'Test(Save|FinalReplace|OverwriteApproval|DiscardApproval)', '-count=1', '-v'], ROOT)
    (out / 'real-code-baseline.txt').write_text(baseline.stdout)
    assert baseline.returncode == 0, baseline.stdout
    mutants = [
        ('clean', 'internal/documents/save.go', 'd.Path = result.Path', 'd.state.saved=d.revision()\n d.Path = result.Path', 'save completion marked newer edits clean'),
        ('approval', 'internal/app/external.go', 'r.doc.PrepareOverwrite(r.expected)', 'r.doc.PrepareOverwrite(a.external[r.doc].Fingerprint)', 'overwrite approval followed a newer unreviewed watcher version'),
    ]
    for name, path, old, new, failure in mutants:
        with tempfile.TemporaryDirectory(prefix='wondervision-save-mutant-') as scratch:
            work = Path(scratch)
            for mod in ('go.mod', 'go.sum'):
                shutil.copy2(ROOT / mod, work / mod)
            shutil.copytree(ROOT / 'internal', work / 'internal')
            file = work / path
            text = file.read_text()
            assert text.count(old) == 1, 'production transition changed; review the model and replay'
            file.write_text(text.replace(old, new, 1))
            result = run(['go', 'test', './internal/documents', './internal/app', '-run', 'Test(Save|OverwriteApproval)', '-count=1', '-v'], work)
            (out / ('real-code-mutant-' + name + '.txt')).write_text(result.stdout)
            assert result.returncode != 0 and failure in result.stdout, result.stdout
            summaries.append('real-code-' + name + ': behavioral counterexample reproduced')
    with tempfile.TemporaryDirectory(prefix='wondervision-save-conflict-mutant-') as scratch:
        work = Path(scratch)
        for mod in ('go.mod', 'go.sum'):
            shutil.copy2(ROOT / mod, work / mod)
        shutil.copytree(ROOT / 'internal', work / 'internal')
        file = work / 'internal/documents/save.go'
        text = file.read_text()
        start = text.index('\tif before != p.expected {')
        end = text.index('\n\tmode :=', start)
        file.write_text(text[:start] + text[end:])
        result = run(['go', 'test', './internal/documents', '-run', 'TestSaveRejectsKnownConflict', '-count=1', '-v'], work)
        (out / 'real-code-mutant-conflict.txt').write_text(result.stdout)
        assert result.returncode != 0 and 'known external change was not blocked' in result.stdout, result.stdout
        summaries.append('real-code-conflict: behavioral counterexample reproduced')
    restored = run(['go', 'test', './internal/documents', './internal/app', '-count=1'], ROOT)
    (out / 'real-code-restored.txt').write_text(restored.stdout)
    assert restored.returncode == 0, restored.stdout
    report = '\n'.join(summaries) + '\nVERIFIED within the finite model and replay scope. The final filesystem replace window is a reproduced limitation.\n'
    (out / 'summary.txt').write_text(report)
    print(report)


if __name__ == '__main__':
    main()
