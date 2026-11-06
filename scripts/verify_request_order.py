#!/usr/bin/env python3
"""Check bounded callback schedules and prove the checks reject broken guards."""
import argparse
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
PATTERN = 'Test(File|Folder)CompletionSchedules'


def check(root, pattern):
    return subprocess.run(['go', 'test', './internal/app', '-run', pattern, '-count=1', '-v'], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--mutation-check', action='store_true')
    parser.add_argument('--out', type=Path, default=ROOT / 'build/verification/request-order')
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    version = subprocess.check_output(['go', 'version'], text=True)
    assert 'go1.27.3 ' in version, 'use the pinned Go 1.27.3 toolchain'
    original = check(ROOT, PATTERN)
    (args.out / 'baseline.txt').write_text(version + original.stdout)
    assert original.returncode == 0, original.stdout
    print(original.stdout, end='')
    if args.mutation_check:
        for method, field, message in [('openFile', 'openID', 'stale file completion'), ('openFolder', 'folderID', 'stale folder completion')]:
            with tempfile.TemporaryDirectory(prefix='wondervision-mutant-') as scratch:
                clone = Path(scratch)
                for name in ('go.mod', 'go.sum'):
                    shutil.copy2(ROOT / name, clone / name)
                shutil.copytree(ROOT / 'internal', clone / 'internal')
                source = clone / 'internal/app/app.go'
                text = source.read_text()
                start = text.index('func (a *App) ' + method + '(')
                prefix, body = text[:start], text[start:]
                guard = f'\t\t\tif id != a.{field} || ctx.Err() != nil {{\n\t\t\t\treturn\n\t\t\t}}'
                assert body.count(guard) == 1, 'guard shape changed; review the verification against the code'
                source.write_text(prefix + body.replace(guard, '\t\t\t_ = id', 1))
                mutant = check(clone, PATTERN)
                (args.out / ('mutant-' + method + '.txt')).write_text(mutant.stdout)
                assert mutant.returncode != 0 and message in mutant.stdout, 'mutant did not fail for the expected behavioral reason:\n' + mutant.stdout
                print(method + ': removed stale-result guard was rejected by a behavioral counterexample')
        restored = check(ROOT, PATTERN)
        (args.out / 'restored.txt').write_text(restored.stdout)
        assert restored.returncode == 0, restored.stdout
        print('Unmodified production code passes again. No working-tree mutation was made.')
    print('VERIFIED within the bounds of four file requests and three folder requests. This is exhaustive bounded testing, not an unbounded proof.')


if __name__ == '__main__':
    main()
