#!/usr/bin/env python3
"""Exercise M3 (command palette, Quick Open, workspace search) in the real native window."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from verify_native import Desktop, Native, doctor, remove_socket_dir, seed, socket_dir, until


def run(binary, out):
    out.mkdir(parents=True, exist_ok=False)
    scratch = Path(tempfile.mkdtemp(prefix='wondervision-verify-', dir=out.parent))
    scratch.chmod(0o700)
    root, _ = seed(scratch)
    env = os.environ.copy()
    sockets = socket_dir()
    env.update(NO_AT_BRIDGE='0', XDG_CONFIG_HOME=str(scratch / 'config'), XDG_CACHE_HOME=str(scratch / 'cache'), MYGO_AUTOMATION=str(sockets / 'app.sock'))
    proc = native = None
    log = (out / 'app.log').open('w')
    checks = []
    desktop = Desktop()

    def launch():
        p = subprocess.Popen([str(binary)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
        manifest = {'pid': p.pid, 'binary': str(binary), 'socket': env['MYGO_AUTOMATION'], 'socket_dir': str(sockets), 'out': str(out), 'scratch': str(scratch), 'start': Path(f'/proc/{p.pid}/stat').read_text().split()[21]}
        (out / 'instance.json').write_text(json.dumps(manifest, indent=2))
        until(lambda: Path(env['MYGO_AUTOMATION']).exists() or p.poll() is not None, 'native startup')
        assert p.poll() is None, 'application exited during startup'
        (out / 'doctor.json').write_text(json.dumps(doctor(manifest), indent=2))
        desktop.pid = p.pid
        return p, Native(Path(env['MYGO_AUTOMATION']), out)

    def type_query(label, text):
        node = native.wait(label)
        native.call('act', id=node['id'], action='setValue', text=text)
        until(lambda: (n := native.find(label)) and n.get('value', '') == text, label + ' query')

    def status(text):
        return any(text in n.get('label', '') and n['role'] == 'text' for n in native.nodes())

    def row(label):
        # Overlay rows, not explorer items or tabs with the same name.
        return next((n for n in native.nodes() if n.get('label') == label and n['role'] not in ('treeItem', 'tab', 'button')), None)

    try:
        proc, native = launch()
        native.click('Open Folder', 'button')
        desktop.pick('Open Folder', root, out)
        native.click('report.md', 'treeItem')
        native.wait('report.md', 'tab')
        native.click('Edit', 'button')
        native.click('Editor', 'textField')

        # Palette opens on the shortcut; Escape closes it and gives the
        # keyboard back to the editor (a real keystroke lands there).
        native.key('CmdOrCtrl+Shift+P')
        native.wait('Command palette')
        native.capture('palette')
        native.key('Escape')
        until(lambda: not native.find('Command palette'), 'palette closed')
        desktop.chord('x')
        until(lambda: status('Unsaved'), 'keystroke after Escape reached the editor')
        native.key('CmdOrCtrl+Z')
        until(lambda: status('Saved') and not status('Unsaved'), 'undo restored the saved text')
        checks.append('Ctrl+Shift+P opens the palette; Escape closes it and focus returns to the editor')

        # Palette runs a command chosen by typing and Enter.
        native.key('CmdOrCtrl+Shift+P')
        type_query('Command palette', 'toggle side')
        native.key('Enter')
        native.wait('Show Sidebar', 'button')
        native.capture('sidebar-hidden')
        native.key('CmdOrCtrl+B')
        native.wait('Hide Sidebar', 'button')
        checks.append('The palette runs Toggle Sidebar; Ctrl+B from the same command table toggles it back')

        # Quick Open finds a nested file by a partial, fuzzy name.
        native.key('CmdOrCtrl+P')
        type_query('Quick Open', 'nxt')
        until(lambda: row('next.md'), 'Quick Open result next.md')
        native.capture('quick-open')
        native.key('Enter')
        native.wait('next.md', 'tab')
        until(lambda: not native.find('Quick Open'), 'Quick Open closed')
        checks.append('Ctrl+P finds docs/next.md from "nxt" and opens it')

        # Workspace search finds a phrase and opens the file at the line.
        native.key('CmdOrCtrl+Shift+F')
        type_query('Search in workspace', 'native views')
        native.key('Enter')
        until(lambda: row('report.md line 3'), 'search hit in report.md line 3')
        native.capture('search')
        native.key('Enter')
        until(lambda: not native.find('Search in workspace'), 'search closed')
        until(lambda: status('Ln 3, Col 14'), 'cursor at the match (line 3, column 14)')
        native.capture('opened-at-line')
        checks.append('Ctrl+Shift+F finds "native views" and opens report.md at line 3, column 14')

        result = {'verdict': 'VERIFIED', 'checks': checks}
        (out / 'result.json').write_text(json.dumps(result, indent=2))
        print(json.dumps(result, indent=2))
    except Exception as error:
        (out / 'result.json').write_text(json.dumps({'verdict': 'NOT VERIFIED', 'error': str(error), 'completed': checks}, indent=2))
        if native is not None:
            try:
                native.capture('failure')
            except Exception:
                pass
        raise
    finally:
        if proc is not None and proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
        log.close()
        shutil.rmtree(scratch)
        socket_removed = remove_socket_dir(sockets)
        (out / 'cleanup.json').write_text(json.dumps({'process_exited': proc is None or proc.poll() is not None, 'scratch_removed': not scratch.exists(), 'socket_removed': socket_removed}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, default=Path('build/wondervision-e2e'))
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    run(args.binary.resolve(), args.out.resolve())
