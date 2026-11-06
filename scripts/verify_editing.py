#!/usr/bin/env python3
"""Exercise M2 through the real native window in a disposable private profile."""
import argparse
import hashlib
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
    config = root / '.env'
    original = b'\xef\xbb\xbfAPP_NAME=WonderVision\r\nDEBUG=false\r\n'
    config.write_bytes(original)
    config.chmod(0o640)
    env = os.environ.copy()
    sockets = socket_dir()
    env.update(NO_AT_BRIDGE='0', XDG_CONFIG_HOME=str(scratch / 'config'), XDG_CACHE_HOME=str(scratch / 'cache'), MYGO_AUTOMATION=str(sockets / 'app.sock'))
    proc = native = None
    log = (out / 'app.log').open('w')
    checks, proof = [], {}
    desktop = Desktop()
    recovery = scratch / 'cache/wondervision/recovery'

    def launch():
        p = subprocess.Popen([str(binary)], env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
        manifest = {'pid': p.pid, 'binary': str(binary), 'socket': env['MYGO_AUTOMATION'], 'socket_dir': str(sockets), 'out': str(out), 'scratch': str(scratch), 'start': Path(f'/proc/{p.pid}/stat').read_text().split()[21]}
        (out / 'instance.json').write_text(json.dumps(manifest, indent=2))
        until(lambda: Path(env['MYGO_AUTOMATION']).exists() or p.poll() is not None, 'native startup')
        assert p.poll() is None, 'application exited during startup'
        (out / 'doctor.json').write_text(json.dumps(doctor(manifest), indent=2))
        desktop.pid = p.pid
        return p, Native(Path(env['MYGO_AUTOMATION']), out)

    def edit(text, label='Editor'):
        node = native.wait(label, 'textField')
        native.call('act', id=node['id'], action='setValue', text=text)
        until(lambda: text.splitlines()[0] in native.wait(label, 'textField').get('value', ''), 'edited value')

    def value_contains(text, label='Editor'):
        return (node := native.find(label, 'textField')) and text in node.get('value', '')

    def status(text):
        return any(text in n.get('label', '') and n['role'] == 'text' for n in native.nodes())

    def menu(label):
        native.click('File', 'menuButton')
        native.click(label, 'menuItem')

    def file_prompt(action, path):
        menu(action)
        edit(path, 'Path')
        native.key('Enter')
        until(lambda: not native.find('Path', 'textField'), 'file prompt submitted')

    def records():
        return [json.loads(p.open().readline()) for p in recovery.glob('*.draft')]

    try:
        proc, native = launch()
        native.click('Open Folder', 'button')
        desktop.pick('Open Folder', root, out)
        native.click('.env', 'treeItem')
        native.wait('.env', 'tab')
        edit('APP_NAME=WonderVision\nDEBUG=true\n')
        until(lambda: status('Unsaved'), 'dirty status')
        assert config.read_bytes() == original, 'typing wrote the original'
        native.capture('manual-save-dirty')
        native.key('CmdOrCtrl+S')
        saved = b'\xef\xbb\xbfAPP_NAME=WonderVision\r\nDEBUG=true\r\n'
        until(lambda: config.read_bytes() == saved and status('Saved'), 'explicit save')
        assert config.stat().st_mode & 0o777 == 0o640, 'save changed permissions'
        proof['manual_save'] = {'original_sha256': hashlib.sha256(original).hexdigest(), 'saved_sha256': hashlib.sha256(saved).hexdigest(), 'mode': '0640', 'bom': True, 'newline': 'CRLF', 'final_newline': True}
        checks.append('Typing is memory-only; Ctrl+S preserves UTF-8 BOM, CRLF, final newline and mode 0640')
        native.key('CmdOrCtrl+Shift+S')
        target = root / '.env.saved'
        desktop.pick('Save As', target, out, accept='Save')
        native.wait('.env.saved', 'tab')
        until(lambda: target.exists() and target.read_bytes() == saved, 'Save As destination')
        assert config.read_bytes() == saved, 'Save As changed the source'
        native.wait('.env.saved', 'treeItem')
        checks.append('Native Save As creates the chosen copy, retargets the tab and refreshes Explorer')
        native.click('.env', 'treeItem')
        native.wait('.env', 'tab')
        config.write_bytes(b'APP_NAME=clean_external\n')
        until(lambda: value_contains('clean_external') and status('Saved'), 'clean external reload')
        edit('APP_NAME=buffer_wins\n')
        config.write_bytes(b'APP_NAME=disk_first\n')
        native.wait('Overwrite', 'button')
        assert value_contains('buffer_wins'), 'dirty edits were reloaded away'
        native.capture('external-conflict')
        native.click('Compare', 'button')
        until(lambda: value_contains('disk_first', 'Disk version'), 'disk comparison')
        disk = native.wait('Disk version', 'textField')
        assert 'readOnly' in disk.get('states', []) and 'setValue' not in disk.get('actions', []), 'comparison is editable'
        native.capture('disk-comparison')
        native.click('Close comparison', 'button')
        native.click('Overwrite', 'button')
        native.wait('Overwrite disk changes?', 'alertDialog')
        config.write_bytes(b'APP_NAME=disk_newer\n')
        native.click('Overwrite Disk', 'button')
        until(lambda: status('Unsaved') and native.find('Overwrite', 'button'), 'newer change retained')
        assert config.read_bytes() == b'APP_NAME=disk_newer\n', 'stale approval overwrote a newer disk version'
        # Re-read the observed disk version before giving a new approval.
        native.click('Compare', 'button')
        until(lambda: value_contains('disk_newer', 'Disk version'), 'new disk comparison')
        native.click('Close comparison', 'button')
        native.click('Overwrite', 'button')
        native.click('Overwrite Disk', 'button')
        until(lambda: config.read_bytes() == b'APP_NAME=buffer_wins\n' and status('Saved'), 'freshly approved overwrite')
        native.capture('conflict-resolved')
        checks.append('Scoped watcher reloads clean files, preserves dirty buffers and rejects stale overwrite approval')
        native.key('CmdOrCtrl+W')
        until(lambda: not native.find('.env', 'tab'), 'clean tab closed')
        file_prompt('New Folder', 'notes')
        until(lambda: (root / 'notes').is_dir(), 'folder created')
        file_prompt('New File', 'notes/draft.md')
        native.wait('draft.md', 'tab')
        native.click('Edit', 'button')
        edit('# File operation draft\n')
        native.key('CmdOrCtrl+W')
        native.wait('Save changes to draft.md?', 'alertDialog')
        native.click('Cancel', 'button')
        native.wait('draft.md', 'tab')
        file_prompt('Rename', 'renamed.md')
        native.wait('renamed.md', 'tab')
        assert value_contains('File operation draft') and (root / 'notes/renamed.md').read_bytes() == b'', 'rename lost edits or silently saved'
        file_prompt('Move', 'moved.md')
        native.wait('moved.md', 'tab')
        assert value_contains('File operation draft') and (root / 'moved.md').read_bytes() == b'', 'move lost edits or silently saved'
        native.capture('file-operations')
        menu('Delete Permanently')
        native.wait('Delete permanently?', 'alertDialog')
        native.click('Cancel', 'button')
        assert (root / 'moved.md').exists(), 'cancelled deletion removed file'
        menu('Delete Permanently')
        native.click('Delete permanently', 'button')
        until(lambda: not (root / 'moved.md').exists() and not native.find('moved.md', 'tab'), 'confirmed deletion')
        checks.append('Create folder/file, rename and move retain dirty buffers; close and permanent delete require decisions')
        native.click('todo.md', 'treeItem')
        native.wait('todo.md', 'tab')
        native.click('Edit', 'button')
        draft = '# Recovered task\n\n- [ ] Finish review\n'
        edit(draft)
        todo_original = (root / 'todo.md').read_bytes()
        native.click('.env', 'treeItem')
        native.wait('.env', 'tab')
        edit('APP_NAME=must_not_recover\n')
        native.click('todo.md', 'tab')
        until(lambda: any(r['path'] == str(root / 'todo.md') for r in records()), 'edit-triggered recovery snapshot', seconds=25)
        before_crash = records()
        assert all(Path(r['path']).name != '.env' for r in before_crash), 'sensitive recovery was written'
        assert (root / 'todo.md').read_bytes() == todo_original and config.read_bytes() == b'APP_NAME=buffer_wins\n', 'recovery wrote an original file'
        assert all(p.stat().st_mode & 0o077 == 0 for p in recovery.glob('*.draft')), 'recovery copies are not private'
        proof['recovery'] = {'paths': [str(Path(r['path']).relative_to(root)) for r in before_crash], 'private_files': True, 'original_unchanged': True, 'sensitive_excluded': True}
        native.capture('before-crash')
        proc.kill()
        proc.wait(timeout=5)
        Path(env['MYGO_AUTOMATION']).unlink(missing_ok=True)
        proc, native = launch()
        native.wait('Recover drafts', 'button')
        native.click('Recover drafts', 'button')
        native.wait('Recover', 'button')
        native.capture('recovery-review')
        native.click('Recover', 'button')
        until(lambda: value_contains('Recovered task') and status('Unsaved'), 'recovered dirty draft')
        assert (root / 'todo.md').read_bytes() == todo_original, 'recover implicitly saved original'
        native.capture('recovered-draft')
        native.key('CmdOrCtrl+S')
        until(lambda: (root / 'todo.md').read_text() == draft and not records(), 'recovered draft saved and cleaned')
        checks.append('Crash/restart offers private non-sensitive recovery for review, excludes .env, and removes the copy after Save')
        edit('# Save before quit\n')
        desktop.chord('Alt_L', 'F4')
        native.click_in('Discard unsaved edits?', 'Save', settle=False)
        proc.wait(timeout=10)
        assert (root / 'todo.md').read_text() == '# Save before quit\n' and not records(), 'Save on quit failed to settle'
        checks.append('Save on quit completes filesystem work and recovery cleanup before exit')
        (out / 'filesystem-check.json').write_text(json.dumps(proof, indent=2))
        result = {'verdict': 'VERIFIED', 'checks': checks, 'display': os.environ.get('DISPLAY')}
        (out / 'result.json').write_text(json.dumps(result, indent=2))
        print(json.dumps(result, indent=2))
    except Exception as error:
        (out / 'result.json').write_text(json.dumps({'verdict': 'NOT VERIFIED', 'error': str(error), 'completed': checks}, indent=2))
        if proc is not None and proc.poll() is None and native is not None:
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
