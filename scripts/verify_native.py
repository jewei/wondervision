#!/usr/bin/env python3
"""Exercise the Linux native UI in an isolated profile and retain evidence."""
import argparse
import base64
import ctypes
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import tempfile
import time


class Desktop:
    def __init__(self):
        previous = os.environ.get('XAUTHORITY')
        if os.environ.get('WV_INPUT_XAUTHORITY'):
            os.environ['XAUTHORITY'] = os.environ['WV_INPUT_XAUTHORITY']
        self.x = ctypes.CDLL('libX11.so.6')
        self.t = ctypes.CDLL('libXtst.so.6')
        self.x.XOpenDisplay.argtypes = [ctypes.c_char_p]
        self.x.XOpenDisplay.restype = ctypes.c_void_p
        self.d = self.x.XOpenDisplay(None)
        if previous is None:
            os.environ.pop('XAUTHORITY', None)
        else:
            os.environ['XAUTHORITY'] = previous
        if not self.d:
            raise RuntimeError('cannot connect to the X11 desktop')
        self.x.XStringToKeysym.argtypes = [ctypes.c_char_p]
        self.x.XStringToKeysym.restype = ctypes.c_ulong
        self.x.XKeysymToKeycode.argtypes = [ctypes.c_void_p, ctypes.c_ulong]
        self.x.XKeysymToKeycode.restype = ctypes.c_uint
        self.x.XFlush.argtypes = [ctypes.c_void_p]
        self.x.XSync.argtypes = [ctypes.c_void_p, ctypes.c_int]
        self.x.XSetInputFocus.argtypes = [ctypes.c_void_p, ctypes.c_ulong, ctypes.c_int, ctypes.c_ulong]
        self.t.XTestFakeKeyEvent.argtypes = [ctypes.c_void_p, ctypes.c_uint, ctypes.c_int, ctypes.c_ulong]

    def chord(self, *keys):
        codes = [self.x.XKeysymToKeycode(self.d, self.x.XStringToKeysym(k.encode())) for k in keys]
        assert all(codes), keys
        for c in codes:
            self.t.XTestFakeKeyEvent(self.d, c, 1, 0)
        for c in reversed(codes):
            self.t.XTestFakeKeyEvent(self.d, c, 0, 0)
        self.x.XSync(self.d, 0)

    def pick(self, title, path, out, accept='Open'):
        def locate():
            tree = subprocess.check_output(['xwininfo', '-root', '-tree'], text=True, timeout=3)
            for found in re.finditer(r'(0x[0-9a-f]+) "' + re.escape(title) + r'"', tree):
                try:
                    owner = subprocess.check_output(['xprop', '-id', found[1], '_NET_WM_PID'], text=True, stderr=subprocess.DEVNULL, timeout=3)
                    if not owner.strip().endswith('= ' + str(self.pid)):
                        continue
                    detail = subprocess.check_output(['xwininfo', '-id', found[1]], text=True, stderr=subprocess.DEVNULL, timeout=3)
                    if 'Map State: IsViewable' in detail:
                        return int(found[1], 16), detail
                except subprocess.CalledProcessError:
                    continue
            return None
        window, tree = until(locate, 'native ' + title + ' dialog')
        (out / ('dialog-' + title.replace(' ', '-').lower() + '.txt')).write_text(tree)
        self.x.XSetInputFocus(self.d, window, 2, 0)
        self.x.XFlush(self.d)
        time.sleep(.7)
        self.chord('Control_L', 'l')
        time.sleep(.2)
        bridge = Accessibility(self.pid)
        dialog = until(lambda: bridge.find_dialog(title), 'accessible file chooser')
        entry = until(lambda: bridge.focused_entry(dialog), 'location entry')
        bridge.set_text(entry, str(path))
        (out / ('native-' + title.replace(' ', '-').lower() + '.json')).write_text(json.dumps(bridge.snapshot(dialog), indent=2))
        try:
            bridge.activate(entry)
            time.sleep(.3)
            if locate():
                def button_or_closed():
                    if not locate():
                        return 'closed'
                    current = bridge.find_dialog(title)
                    return bridge.find_button(current, accept) if current else None
                button = until(button_or_closed, accept + ' button or completed selection')
                if button != 'closed':
                    bridge.activate(button)
            until(lambda: not locate(), 'dialog selection accepted')
        except AssertionError:
            state = locate()
            (out / 'failed-dialog-state.json').write_text(json.dumps({'viewable': bool(state), 'window': hex(window)}))
            raise


class Accessibility:
    def __init__(self, pid):
        self.pid = pid
        self.a = ctypes.CDLL('libatspi.so.0')
        self.a.atspi_init()
        declarations = {
            'atspi_get_desktop': ([ctypes.c_int], ctypes.c_void_p),
            'atspi_accessible_get_process_id': ([ctypes.c_void_p, ctypes.c_void_p], ctypes.c_uint),
            'atspi_accessible_get_child_count': ([ctypes.c_void_p, ctypes.c_void_p], ctypes.c_int),
            'atspi_accessible_get_child_at_index': ([ctypes.c_void_p, ctypes.c_int, ctypes.c_void_p], ctypes.c_void_p),
            'atspi_accessible_get_name': ([ctypes.c_void_p, ctypes.c_void_p], ctypes.c_char_p),
            'atspi_accessible_get_role_name': ([ctypes.c_void_p, ctypes.c_void_p], ctypes.c_char_p),
            'atspi_accessible_get_state_set': ([ctypes.c_void_p], ctypes.c_void_p),
            'atspi_state_set_contains': ([ctypes.c_void_p, ctypes.c_int], ctypes.c_int),
            'atspi_accessible_get_action_iface': ([ctypes.c_void_p], ctypes.c_void_p),
            'atspi_action_do_action': ([ctypes.c_void_p, ctypes.c_int, ctypes.c_void_p], ctypes.c_int),
            'atspi_action_get_n_actions': ([ctypes.c_void_p, ctypes.c_void_p], ctypes.c_int),
            'atspi_accessible_get_editable_text_iface': ([ctypes.c_void_p], ctypes.c_void_p),
            'atspi_editable_text_set_text_contents': ([ctypes.c_void_p, ctypes.c_char_p, ctypes.c_void_p], ctypes.c_int),
        }
        for name, (args, result) in declarations.items():
            f = getattr(self.a, name)
            f.argtypes, f.restype = args, result

    def walk(self, node, depth=0):
        if not node or depth > 18:
            return
        yield node
        for i in range(self.a.atspi_accessible_get_child_count(node, None)):
            yield from self.walk(self.a.atspi_accessible_get_child_at_index(node, i, None), depth + 1)

    def name(self, node):
        return (self.a.atspi_accessible_get_name(node, None) or b'').decode()

    def role(self, node):
        return (self.a.atspi_accessible_get_role_name(node, None) or b'').decode()

    def state(self, node, state):
        return self.a.atspi_state_set_contains(self.a.atspi_accessible_get_state_set(node), state)

    def find_dialog(self, title):
        return next((n for n in self.walk(self.a.atspi_get_desktop(0)) if self.role(n) == 'file chooser' and self.name(n) == title and self.a.atspi_accessible_get_process_id(n, None) == self.pid), None)

    def focused_entry(self, dialog):
        return next((n for n in self.walk(dialog) if self.role(n) == 'text' and self.state(n, 12) and self.state(n, 25)), None)

    def find_button(self, dialog, name):
        return next((n for n in self.walk(dialog) if self.role(n) == 'button' and self.name(n) == name and self.state(n, 25)), None)

    def set_text(self, node, text):
        interface = self.a.atspi_accessible_get_editable_text_iface(node)
        assert interface and self.a.atspi_editable_text_set_text_contents(interface, text.encode(), None), 'location entry is not editable'

    def activate(self, node):
        interface = self.a.atspi_accessible_get_action_iface(node)
        assert interface and self.a.atspi_action_get_n_actions(interface, None), 'native control has no action'
        assert self.a.atspi_action_do_action(interface, 0, None), 'native control action failed'

    def snapshot(self, dialog):
        return [{'role': self.role(n), 'name': self.name(n), 'showing': bool(self.state(n, 25))} for n in self.walk(dialog)]


def until(check, what, seconds=15):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.05)
    raise AssertionError('timed out waiting for ' + what)


class Native:
    def __init__(self, sock, out):
        self.sock, self.out = sock, out
        self.serial = 0

    def call(self, method, **params):
        self.serial += 1
        request = {'id': self.serial, 'method': method, 'params': params}
        with socket.socket(socket.AF_UNIX) as client:
            client.settimeout(20)
            client.connect(str(self.sock))
            client.sendall((json.dumps(request) + '\n').encode())
            line = client.makefile().readline()
            assert line, 'native automation disconnected; inspect app.log for the application failure'
            answer = json.loads(line)
        if 'error' in answer:
            raise AssertionError(answer['error'])
        if method != 'capture':
            with (self.out / 'actions.jsonl').open('a') as log:
                log.write(json.dumps({'request': request, 'result': answer['result']}) + '\n')
        return answer['result']

    def nodes(self):
        return self.call('tree').get('nodes', [])

    def find(self, label, role=None):
        return next((n for n in self.nodes() if n.get('label') == label and (role is None or n['role'] == role)), None)

    def wait(self, label, role=None):
        return until(lambda: self.find(label, role), label)

    def click(self, label, role, settle=True):
        n = self.wait(label, role)
        b = n['bounds']
        self.call('input', events=[{'type': 'click', 'x': b['x'] + b['width'] * .6, 'y': b['y'] + b['height'] / 2}])
        if settle:
            self.call('idle', timeout=5000)

    def key(self, key):
        self.call('input', events=[{'type': 'key', 'key': key}])

    def capture(self, name):
        self.call('idle', timeout=5000)
        result = self.call('capture')
        (self.out / (name + '.png')).write_bytes(base64.b64decode(result['png']))
        (self.out / (name + '.json')).write_text(json.dumps(self.call('tree'), indent=2))


def doctor(manifest):
    pid = manifest['pid']
    os.kill(pid, 0)
    assert Path(f'/proc/{pid}/exe').resolve() == Path(manifest['binary']).resolve(), 'wrong process'
    info = Path(manifest['socket']).stat()
    assert info.st_uid == os.getuid() and info.st_mode & 0o777 == 0o600, 'socket is not private'
    n = Native(Path(manifest['socket']), Path(manifest['out']))
    windows = n.call('windows')['windows']
    assert len(windows) == 1 and windows[0]['title'] == 'WonderVision' and windows[0]['kind'] == 'content', windows
    n.wait('Open File', 'button')
    return {'verified': True, 'pid': pid, 'window': windows[0], 'binary': manifest['binary']}


def seed(scratch):
    root = scratch / 'workspace'
    (root / 'docs').mkdir(parents=True)
    (root / 'docs' / 'next.md').write_text('# Next document\n\n[Back](../report.md)\n')
    (root / 'report.md').write_text('# Workspace review\n\nLocal files, native views.\n\n- [x] Open a folder\n- [ ] Review configuration\n\n| Check | Result |\n| --- | --- |\n| Rendering | Native GFM |\n| Images | Local only |\n\n![Local image](local.svg)\n\n```go\nfunc main() {\n    println("WonderVision")\n}\n```\n\n[Next document](docs/next.md)\n')
    (root / 'local.svg').write_text('<svg xmlns="http://www.w3.org/2000/svg" width="240" height="70"><rect width="240" height="70" rx="8" fill="#dbeafe"/><rect x="16" y="16" width="38" height="38" rx="6" fill="#2563eb"/><circle cx="88" cy="35" r="19" fill="#16a34a"/><rect x="124" y="16" width="96" height="38" rx="6" fill="#7c3aed"/></svg>')
    for name, text in {'.env': 'APP_NAME=WonderVision\nDEBUG=false\n', 'todo.md': '# Tasks\n\n- [ ] Review\n', 'config.yaml': 'local: true\n', 'AGENTS.md': '# Ordinary document\n'}.items():
        (root / name).write_text(text)
    outside = scratch / 'standalone.txt'
    outside.write_text('Opened without a workspace.\n')
    return root, outside


def run(binary, out):
    out.mkdir(parents=True, exist_ok=False)
    scratch = Path(tempfile.mkdtemp(prefix='wondervision-verify-', dir=out.parent))
    scratch.chmod(0o700)
    root, outside = seed(scratch)
    original = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob('*') if p.is_file()}
    env = os.environ.copy()
    env['NO_AT_BRIDGE'] = '0'
    env.update(XDG_CONFIG_HOME=str(scratch / 'config'), XDG_CACHE_HOME=str(scratch / 'cache'), MYGO_AUTOMATION=str(scratch / 'app.sock'))
    proc = None
    log = (out / 'app.log').open('w')
    results = []
    try:
        def launch():
            p = subprocess.Popen([str(binary)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
            manifest = {'pid': p.pid, 'binary': str(binary), 'socket': env['MYGO_AUTOMATION'], 'out': str(out), 'scratch': str(scratch), 'start': Path(f'/proc/{p.pid}/stat').read_text().split()[21]}
            (out / 'instance.json').write_text(json.dumps(manifest, indent=2))
            until(lambda: Path(env['MYGO_AUTOMATION']).exists() or p.poll() is not None, 'native startup')
            assert p.poll() is None, 'application exited during startup'
            d = doctor(manifest)
            (out / 'doctor.json').write_text(json.dumps(d, indent=2))
            return p, Native(Path(env['MYGO_AUTOMATION']), out)
        desktop = Desktop()
        proc, native = launch()
        desktop.pid = proc.pid
        native.wait('No folder open')
        native.capture('empty')
        native.click('Open File', 'button')
        desktop.pick('Open File', outside, out)
        native.wait('standalone.txt', 'tab')
        native.wait('No folder open')
        native.capture('standalone')
        native.key('CmdOrCtrl+O')
        desktop.pick('Open File', outside, out)
        native.wait('standalone.txt', 'tab')
        assert len([n for n in native.nodes() if n['role'] == 'tab']) == 1, 'reopening the same file created another buffer'
        results.append('Open File toolbar and shortcut work without a workspace')
        native.key('CmdOrCtrl+Shift+O')
        desktop.pick('Open Folder', root, out)
        native.wait('.env', 'treeItem')
        native.click('Open Folder', 'button')
        desktop.pick('Open Folder', root, out)
        native.wait('.env', 'treeItem')
        until(lambda: (b := native.find('Refresh', 'button')) and 'disabled' not in b.get('states', []), 'folder read completed')
        docs = native.wait('docs', 'treeItem')
        native.call('act', id=docs['id'], action='expand')
        native.wait('next.md', 'treeItem')
        results.append('Open Folder toolbar and shortcut, visible dotfiles, and lazy expansion work')
        native.click('report.md', 'treeItem')
        native.wait('Workspace review', 'heading')
        until(lambda: (n := native.find('Local image', 'image')) and n['bounds']['width'] == 240, 'local image load')
        native.capture('preview')
        native.click('Edit', 'button')
        native.wait('Editor', 'textField')
        native.capture('edit')
        native.click('Split', 'button')
        ed = native.wait('Editor', 'textField')
        native.call('act', id=ed['id'], action='selectText', **{'from': 0, 'to': 0})
        native.call('act', id=ed['id'], action='insertText', text='# Unsaved heading\n\n')
        native.wait('Unsaved heading', 'heading')
        native.capture('split')
        results.append('Edit, Preview, and Split use the same live buffer and display a local image')
        native.click('.env', 'treeItem')
        native.wait('.env', 'tab')
        assert not native.find('Preview', 'button'), 'plain file built Markdown controls'
        ed = native.wait('Editor', 'textField')
        native.call('act', id=ed['id'], action='insertText', text='# in memory\n')
        native.click('report.md', 'tab')
        native.click('Preview', 'button')
        native.click('Next document', 'link')
        native.wait('next.md', 'tab')
        native.wait('Next document', 'heading')
        native.click('report.md', 'tab')
        native.wait('Unsaved heading', 'heading')
        native.key('CmdOrCtrl+B')
        native.wait('Show Sidebar', 'button')
        assert not any(n['role'] == 'tree' for n in native.nodes()), 'hidden sidebar is still built'
        native.key('CmdOrCtrl+B')
        native.wait('Hide Sidebar', 'button')
        results.append('Local Markdown links, tab buffers, and sidebar shortcut work')
        current = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob('*') if p.is_file()}
        assert current == original, 'viewing or typing changed workspace files'
        prefs = scratch / 'config' / 'wondervision' / 'preferences.json'
        until(lambda: prefs.exists() and json.loads(prefs.read_text()).get('active_file') == str(root / 'report.md'), 'preferences persisted')
        saved = json.loads(prefs.read_text())
        assert set(saved) <= {'workspace', 'active_file', 'markdown_mode', 'sidebar_hidden', 'sidebar_width'}, saved
        (out / 'filesystem-check.json').write_text(json.dumps({'before': original, 'after': current, 'preferences_keys': list(saved)}, indent=2))
        native.capture('before-quit')
        desktop.chord('Alt_L', 'F4')
        native.wait('Discard unsaved edits?', 'alertDialog')
        native.click('Keep Editing', 'button')
        assert proc.poll() is None, 'keep editing closed the app'
        desktop.chord('Alt_L', 'F4')
        native.wait('Discard unsaved edits?', 'alertDialog')
        native.click('Discard and Quit', 'button', settle=False)
        proc.wait(timeout=10)
        until(lambda: not Path(env['MYGO_AUTOMATION']).exists(), 'automation socket cleanup')
        proc, native = launch()
        desktop.pid = proc.pid
        native.wait('report.md', 'tab')
        native.wait('Workspace review', 'heading')
        assert len([n for n in native.nodes() if n['role'] == 'tab']) == 1, 'restore reopened every buffer'
        assert not native.find('next.md', 'treeItem'), 'restore expanded a descendant directory'
        assert not native.find('Unsaved heading', 'heading'), 'draft was silently written to disk'
        native.capture('restored')
        results.append('Quit guard and lazy restart restore work; no content was written')
        (out / 'result.json').write_text(json.dumps({'verdict': 'VERIFIED', 'checks': results}, indent=2))
        print(json.dumps({'verdict': 'VERIFIED', 'out': str(out), 'checks': results}, indent=2))
    except Exception as error:
        (out / 'result.json').write_text(json.dumps({'verdict': 'NOT VERIFIED', 'error': str(error), 'completed': results}, indent=2))
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
        (out / 'cleanup.json').write_text(json.dumps({'process_exited': proc is None or proc.poll() is not None, 'scratch_removed': not scratch.exists()}))


def cleanup(manifest):
    pid = manifest['pid']
    proc = Path(f'/proc/{pid}')
    if proc.exists() and (proc / 'exe').exists():
        assert (proc / 'exe').resolve() == Path(manifest['binary']).resolve(), 'PID belongs to another executable'
        if 'start' in manifest:
            assert (proc / 'stat').read_text().split()[21] == manifest['start'], 'PID was reused'
        os.kill(pid, 15)
        until(lambda: not (proc / 'exe').exists(), 'owned process cleanup')
    scratch = Path(manifest.get('scratch', str(Path(manifest['socket']).parent)))
    assert scratch.parent in (Path(tempfile.gettempdir()), Path(manifest['out']).parent) and scratch.name.startswith('wondervision-verify-'), 'unexpected scratch path'
    shutil.rmtree(scratch, ignore_errors=True)
    print(json.dumps({'process_exited': True, 'scratch_removed': not scratch.exists(), 'evidence': manifest['out']}))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['run', 'doctor', 'cleanup'])
    parser.add_argument('--binary', type=Path, default=Path('build/wondervision-e2e'))
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    out = args.out.resolve()
    if args.command == 'doctor':
        print(json.dumps(doctor(json.loads((out / 'instance.json').read_text())), indent=2))
    elif args.command == 'cleanup':
        cleanup(json.loads((out / 'instance.json').read_text()))
    else:
        run(args.binary.resolve(), out)


if __name__ == '__main__':
    main()
