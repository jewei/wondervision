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

        # In-editor Find and Replace (spec §5, journey 2). MyGo draws its find
        # bar inside the editor and does not expose it to accessibility, so
        # it is driven with real keys and checked through the caret and text.
        def editor_text():
            return native.find('Editor', 'textField').get('value', '')

        def text_input(text):
            native.call('input', events=[{'type': 'text', 'text': text}])
            native.call('idle', timeout=5000)

        desktop.chord('Control_L', 'Home')
        until(lambda: status('Ln 1, Col 1'), 'caret at the start')
        desktop.chord('Control_L', 'f')
        native.call('idle', timeout=5000)
        text_input('Local only')
        desktop.chord('Return')
        until(lambda: status('Ln 11,'), 'Find moved to "Local only" on line 11')
        native.capture('find')
        desktop.chord('Escape')
        native.call('idle', timeout=5000)
        desktop.chord('x')
        until(lambda: status('Unsaved'), 'keystroke after closing Find reached the editor')
        desktop.chord('Control_L', 'z')
        until(lambda: status('Saved') and not status('Unsaved'), 'undo restored the saved text')
        checks.append('Ctrl+F finds "Local only" on line 11; Escape returns the keyboard to the editor')

        before = editor_text()
        desktop.chord('Control_L', 'h')  # Cmd+Shift+H or Cmd+Alt+F on macOS
        native.call('idle', timeout=5000)
        text_input('native')
        # Tab past the bar's eight buttons (match case, whole word, regex, in
        # selection, previous, next, toggle replace, close) to the replacement.
        for _ in range(9):
            desktop.chord('Tab')
        native.call('idle', timeout=5000)
        text_input('plain')
        # Enter replaces the selected match and moves to the next. (MyGo's
        # Cmd/Ctrl+Enter for Replace All never reaches a single-line field,
        # so Replace All is the button only; see README.)
        desktop.chord('Return')
        until(lambda: 'plain views' in editor_text(), 'Replace changed line 3')
        desktop.chord('Return')
        until(lambda: 'plain GFM' in editor_text(), 'Replace changed line 10')
        native.capture('replace')
        after = editor_text()
        assert after == before.replace('native', 'plain').replace('Native', 'plain'), 'Replace changed more or less than the two matches'
        assert status('Unsaved'), 'replacing did not mark the document dirty'
        assert 'plain' not in (root / 'report.md').read_text(), 'Replace wrote to disk before Save'
        desktop.chord('Escape')
        native.call('idle', timeout=5000)
        desktop.chord('Control_L', 'z')
        desktop.chord('Control_L', 'z')
        until(lambda: editor_text() == before and status('Saved'), 'undo takes back both replacements')
        checks.append('Ctrl+H, a query, the replacement and Enter twice replace both matches (case-insensitive) in the buffer only; undo restores it')

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
