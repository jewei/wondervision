#!/usr/bin/env python3
"""Measure WonderVision against the spec §9 performance contract.

Production binary (no automation linked): warm launch to a usable window and to
the restored workload, then idle CPU, memory, threads, file descriptors and
internet sockets over --idle-seconds in the foreground and again minimized.
E2E binary (automation socket): Quick Open appearance and cached-query latency
on the reference workload, and responsiveness while scanning 10,000 files.

Run under a virtual display with a window manager, like the GUI verifiers.
Results go to <out>/results.json and <out>/summary.md. Numbers from a container
with a software renderer track regressions; they don't certify a desktop.
"""
import argparse
import ctypes
import json
import os
from pathlib import Path
import platform
import shutil
import statistics
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, str(Path(__file__).resolve().parent))
from verify_native import Accessibility, Desktop, Native, remove_socket_dir, socket_dir, until  # noqa: E402

KEYSYMS = {'.': 'period', '-': 'minus', '_': 'underscore', '/': 'slash'}

ROOT = Path(__file__).resolve().parents[1]
CLK_TCK = os.sysconf('SC_CLK_TCK')
PAGE = os.sysconf('SC_PAGE_SIZE')


def seed_reference(root, files=1000, folders=50):
    """The spec §9 reference workload: ~1,000 text/Markdown files across 50
    folders, a 50 KiB Markdown report, a .env and a 100 KiB code file."""
    root.mkdir(parents=True)
    per = max(1, (files - 3) // folders)
    for f in range(folders):
        d = root / f'area{f:02d}'
        d.mkdir()
        for i in range(per):
            ext = 'md' if i % 2 == 0 else 'txt'
            (d / f'note-{f:02d}-{i:03d}.{ext}').write_text(f'# Note {f}-{i}\n\nReference workload text line {i}.\n')
    section = '## Section\n\nSome *Markdown* text with a [link](area00/note-00-000.md) and `code`.\n\n- item one\n- item two\n\n'
    (root / 'report.md').write_text('# Reference report\n\n' + section * (50 * 1024 // len(section)))
    (root / '.env').write_text('APP_NAME=WonderVision\nDEBUG=false\n')
    line = 'func f%05d(x int) int { return x*2 + 1 } // generated reference code\n'
    (root / 'main.go').write_text('package main\n\n' + ''.join(line % i for i in range(100 * 1024 // len(line % 0))))


def seed_large(root, files=10000):
    root.mkdir(parents=True)
    for i in range(files):
        d = root / f'd{i % 100:02d}' / f'e{i % 7}'
        d.mkdir(parents=True, exist_ok=True)
        (d / f'file-{i:05d}.md').write_text(f'# {i}\n')


class Proc:
    """Samples one process from /proc."""

    def __init__(self, pid):
        self.pid = pid

    def sample(self):
        stat = Path(f'/proc/{self.pid}/stat').read_text().rsplit(')', 1)[1].split()
        status = dict(line.split(':', 1) for line in Path(f'/proc/{self.pid}/status').read_text().splitlines() if ':' in line)
        fds = os.listdir(f'/proc/{self.pid}/fd')
        return {
            'time': time.monotonic(),
            'cpu_ticks': int(stat[11]) + int(stat[12]),  # utime + stime
            'rss_kib': int(status['VmRSS'].split()[0]),
            'threads': int(status['Threads']),
            'fds': len(fds),
            'internet_sockets': self.internet_sockets(fds),
        }

    def internet_sockets(self, fds):
        inodes = set()
        for fd in fds:
            try:
                target = os.readlink(f'/proc/{self.pid}/fd/{fd}')
            except OSError:
                continue
            if target.startswith('socket:['):
                inodes.add(target[8:-1])
        net = set()
        for table in ('tcp', 'tcp6', 'udp', 'udp6'):
            try:
                for line in Path(f'/proc/{self.pid}/net/{table}').read_text().splitlines()[1:]:
                    net.add(line.split()[9])
            except OSError:
                pass
        return len(inodes & net)


def cpu_percent(a, b):
    return 100.0 * (b['cpu_ticks'] - a['cpu_ticks']) / CLK_TCK / (b['time'] - a['time'])


def env_for(scratch, workspace, active):
    env = dict(os.environ, NO_AT_BRIDGE='0', XDG_CONFIG_HOME=str(scratch / 'config'), XDG_CACHE_HOME=str(scratch / 'cache'))
    prefs = scratch / 'config' / 'wondervision' / 'preferences.json'
    prefs.parent.mkdir(parents=True, exist_ok=True)
    prefs.write_text(json.dumps({'workspace': str(workspace), 'active_file': str(active), 'markdown_mode': 1, 'sidebar_width': 220}))
    return env


def accessible(acc, pid, test):
    for n in acc.walk(acc.a.atspi_get_desktop(0)):
        if acc.a.atspi_accessible_get_process_id(n, None) == pid and test(acc.role(n), acc.name(n)):
            return n
    return None


def launch_times(binary, scratch, workspace, active, runs):
    """Seconds from exec to a usable window ('Open File' exposed) and to the
    restored active document (its status line), for runs+1 launches; the first
    is a cold start and is reported separately."""
    out = []
    for _ in range(runs + 1):
        env = env_for(scratch, workspace, active)
        start = time.monotonic()
        p = subprocess.Popen([str(binary)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        acc = Accessibility(p.pid)
        try:
            until(lambda: accessible(acc, p.pid, lambda r, n: n == 'Open File' and 'button' in r), 'usable window', seconds=30)
            usable = time.monotonic() - start
            until(lambda: accessible(acc, p.pid, lambda r, n: n.startswith('Markdown |')), 'restored document', seconds=30)
            restored = time.monotonic() - start
        finally:
            p.terminate()
            p.wait(timeout=10)
        out.append({'usable_s': round(usable, 3), 'restored_s': round(restored, 3)})
    return out


def find_window(pid):
    tree = subprocess.check_output(['xwininfo', '-root', '-tree'], text=True, stderr=subprocess.DEVNULL)
    for line in tree.splitlines():
        if '"WonderVision"' in line:
            wid = line.split()[0]
            owner = subprocess.run(['xprop', '-id', wid, '_NET_WM_PID'], text=True, capture_output=True).stdout
            if owner.strip().endswith('= ' + str(pid)):
                return int(wid, 16)
    return None


def minimize(pid):
    x = ctypes.CDLL('libX11.so.6')
    x.XOpenDisplay.restype = ctypes.c_void_p
    x.XOpenDisplay.argtypes = [ctypes.c_char_p]
    x.XIconifyWindow.argtypes = [ctypes.c_void_p, ctypes.c_ulong, ctypes.c_int]
    x.XDefaultScreen.argtypes = [ctypes.c_void_p]
    x.XFlush.argtypes = [ctypes.c_void_p]
    d = x.XOpenDisplay(None)
    w = until(lambda: find_window(pid), 'WonderVision window')
    ok = x.XIconifyWindow(d, w, x.XDefaultScreen(d))
    x.XFlush(d)
    time.sleep(1)
    state = subprocess.run(['xprop', '-id', hex(w), 'WM_STATE'], text=True, capture_output=True).stdout
    return bool(ok) and 'Iconic' in state


def idle_phase(proc, seconds, label):
    samples = [proc.sample()]
    step = min(60, seconds)
    while samples[-1]['time'] - samples[0]['time'] < seconds - 0.5:
        time.sleep(min(step, seconds - (samples[-1]['time'] - samples[0]['time'])))
        samples.append(proc.sample())
    first, last = samples[0], samples[-1]
    return {
        'phase': label,
        'seconds': round(last['time'] - first['time'], 1),
        'cpu_percent_one_core': round(cpu_percent(first, last), 3),
        'cpu_seconds': round((last['cpu_ticks'] - first['cpu_ticks']) / CLK_TCK, 2),
        'rss_kib_start_end': [first['rss_kib'], last['rss_kib']],
        'rss_kib_max': max(s['rss_kib'] for s in samples),
        'threads_start_end': [first['threads'], last['threads']],
        'fds_start_end': [first['fds'], last['fds']],
        'internet_sockets_max': max(s['internet_sockets'] for s in samples),
        'trend_rss_kib': [s['rss_kib'] for s in samples],
    }


def idle_run(binary, scratch, workspace, out, seconds):
    env = env_for(scratch, workspace, workspace / 'report.md')
    p = subprocess.Popen([str(binary)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    acc = Accessibility(p.pid)
    try:
        return idle_measure(acc, p, out, seconds)
    except Exception:
        subprocess.run(['import', '-window', 'root', str(out / 'idle-failure.png')], stderr=subprocess.DEVNULL)
        raise
    finally:
        p.terminate()
        p.wait(timeout=10)


def open_tabs(acc, p):
    """Wait for the restored report, then open .env and main.go and return to
    the report, as a user would: real keystrokes into Quick Open (the Explorer
    is virtualized, so these rows are off screen). Returns the open tab names."""
    until(lambda: accessible(acc, p.pid, lambda r, n: n.startswith('Markdown |')), 'restored report.md', seconds=30)
    desktop = Desktop()
    for name in ('.env', 'main.go', 'report.md'):
        desktop.chord('Control_L', 'p')
        # The dialog moves focus into its field one frame after it shows.
        until(lambda: (q := accessible(acc, p.pid, lambda r, n: n == 'Quick Open')) and acc.state(q, 12), 'Quick Open focused')
        for ch in name:
            desktop.chord(KEYSYMS.get(ch, ch))
        until(lambda: accessible(acc, p.pid, lambda r, n: n == name and r not in ('tree item', 'page tab')), name + ' in Quick Open', seconds=30)
        time.sleep(0.5)  # let the last X11 key events reach the query before Enter
        desktop.chord('Return')
        until(lambda: accessible(acc, p.pid, lambda r, n: n == name and r == 'page tab'), name + ' tab', seconds=30)
    return sorted({acc.name(n) for n in acc.walk(acc.a.atspi_get_desktop(0)) if acc.a.atspi_accessible_get_process_id(n, None) == p.pid and acc.role(n) == 'page tab'})


def idle_measure(acc, p, out, seconds):
    tabs = open_tabs(acc, p)
    time.sleep(10)  # settle: scans, preview render, recovery timers
    subprocess.run(['import', '-window', 'root', str(out / 'idle-workload.png')], stderr=subprocess.DEVNULL)
    proc = Proc(p.pid)
    phases = [idle_phase(proc, seconds, 'foreground')]
    if minimize(p.pid):
        phases.append(idle_phase(proc, seconds, 'minimized'))
    else:
        phases.append({'phase': 'minimized', 'skipped': 'the window manager did not iconify the window'})
    return {'tabs': tabs, 'phases': phases}


def quick_open(e2e, scratch, workspace, out, trials, query, expect):
    env = env_for(scratch, workspace, workspace / 'report.md')
    sockets = socket_dir()
    env['MYGO_AUTOMATION'] = str(sockets / 'app.sock')
    p = subprocess.Popen([str(e2e)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        until(lambda: Path(env['MYGO_AUTOMATION']).exists() or p.poll() is not None, 'automation socket')
        native = Native(Path(env['MYGO_AUTOMATION']), out)

        def window_ready():
            # The automation socket comes up before the window exists.
            try:
                return native.find('Open File')
            except AssertionError as e:
                if 'no_window' in str(e):
                    return None
                raise
        until(window_ready, 'window with Open File', seconds=30)
        time.sleep(2)
        # One accessibility-tree fetch is the cost of each poll below; report
        # appearance with and without it.
        poll = []
        for _ in range(5):
            t = time.monotonic()
            native.nodes()
            poll.append(time.monotonic() - t)
        appear, query_s, tree_s = [], [], []
        first_scan = None
        for i in range(trials):
            start = time.monotonic()
            native.key('CmdOrCtrl+P')
            until(lambda: native.find('Quick Open'), 'Quick Open', seconds=30)
            appear.append(time.monotonic() - start)
            if i == 0:
                # First open: the scan runs; the UI must keep answering.
                scan_start = time.monotonic()
                while True:
                    t = time.monotonic()
                    nodes = native.nodes()
                    tree_s.append(time.monotonic() - t)
                    if not any(n.get('label') == 'Scanning…' for n in nodes):
                        break
                    if time.monotonic() - scan_start > 120:
                        raise AssertionError('scan did not finish in 120 s')
                first_scan = time.monotonic() - scan_start
            node = native.find('Quick Open')
            start = time.monotonic()
            native.call('act', id=node['id'], action='setValue', text=query)
            until(lambda: any(n.get('label') == expect for n in native.nodes()), expect, seconds=30)
            query_s.append(time.monotonic() - start)
            native.key('Escape')
            until(lambda: not native.find('Quick Open'), 'Quick Open closed')
        return {
            'trials': trials,
            'appear_s_median_max': [round(statistics.median(appear), 3), round(max(appear), 3)],
            'poll_s_median': round(statistics.median(poll), 3),
            'appear_minus_poll_s_median': round(max(0.0, statistics.median(appear) - statistics.median(poll)), 3),
            'cached_query_s_median_max': [round(statistics.median(query_s[1:] or query_s), 3), round(max(query_s[1:] or query_s), 3)],
            'first_scan_s': round(first_scan, 3),
            'ui_answer_during_scan_s_max': round(max(tree_s), 3),
            'note': 'Includes automation round trips and accessibility-tree serialization, so these are upper bounds.',
        }
    finally:
        if p.poll() is None:
            p.terminate()
            p.wait(timeout=10)
        remove_socket_dir(sockets)


def verdict(value, target, lower_is_better=True):
    ok = value <= target if lower_is_better else value >= target
    return 'met' if ok else 'MISSED'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, default=ROOT / 'build/wondervision')
    parser.add_argument('--e2e', type=Path, default=ROOT / 'build/wondervision-e2e')
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--idle-seconds', type=int, default=600, help='per idle phase (spec: 600)')
    parser.add_argument('--launches', type=int, default=5)
    parser.add_argument('--trials', type=int, default=5)
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    scratch = Path(tempfile.mkdtemp(prefix='wondervision-perf-'))
    try:
        ref = scratch / 'reference'
        seed_reference(ref)
        big = scratch / 'large'
        seed_large(big)
        partial = {}

        def keep(name, value):
            # Save after each phase so a later failure can't lose earlier results.
            partial[name] = value
            (out / 'partial.json').write_text(json.dumps(partial, indent=2))
            return value
        launches = keep('launch', launch_times(args.binary.resolve(), scratch / 'launch', ref, ref / 'report.md', args.launches))
        warm = launches[1:]
        idle = keep('idle', idle_run(args.binary.resolve(), scratch / 'idle', ref, out, args.idle_seconds))
        qo_ref = keep('quick_open_reference', quick_open(args.e2e.resolve(), scratch / 'qo-ref', ref, out, args.trials, 'note-42-007', 'note-42-007.txt'))
        qo_big = keep('quick_open_10000', quick_open(args.e2e.resolve(), scratch / 'qo-big', big, out, 2, 'file-09999', 'file-09999.md'))
    finally:
        shutil.rmtree(scratch, ignore_errors=True)

    fg = idle['phases'][0]
    results = {
        'machine': {'platform': platform.platform(), 'cpus': os.cpu_count(), 'python': platform.python_version(),
                    'renderer': 'MyGo software renderer under Xvfb + openbox (no GPU)'},
        'source': subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True, capture_output=True).stdout.strip(),
        'launch': {'cold': launches[0], 'warm_runs': warm,
                   'warm_usable_s_median': round(statistics.median(r['usable_s'] for r in warm), 3),
                   'warm_restored_s_median': round(statistics.median(r['restored_s'] for r in warm), 3)},
        'idle': idle,
        'quick_open_reference': qo_ref,
        'quick_open_10000': qo_big,
    }
    (out / 'results.json').write_text(json.dumps(results, indent=2))

    rows = [
        ('Warm launch to usable window (median)', f"{results['launch']['warm_usable_s_median']} s", '≤ 1.5 s', verdict(results['launch']['warm_usable_s_median'], 1.5)),
        ('Warm launch to restored report (median)', f"{results['launch']['warm_restored_s_median']} s", '—', 'recorded'),
    ]
    for ph in idle['phases']:
        if 'skipped' in ph:
            rows.append((f"Idle CPU, {ph['phase']}", 'not measured', '< 0.5%', ph['skipped']))
            continue
        rows.append((f"Idle CPU, {ph['phase']} ({ph['seconds']:.0f} s)", f"{ph['cpu_percent_one_core']}% of one core", '< 0.5%', verdict(ph['cpu_percent_one_core'], 0.5)))
        rows.append((f"RSS, {ph['phase']} start → end (max)", f"{ph['rss_kib_start_end'][0] / 1024:.1f} → {ph['rss_kib_start_end'][1] / 1024:.1f} MiB ({ph['rss_kib_max'] / 1024:.1f})", '≤ 150 MiB', verdict(ph['rss_kib_max'] / 1024, 150)))
        rows.append((f"Internet sockets, {ph['phase']}", str(ph['internet_sockets_max']), '0 (zero network)', verdict(ph['internet_sockets_max'], 0)))
        rows.append((f"Threads / fds, {ph['phase']} start → end", f"{ph['threads_start_end'][0]} → {ph['threads_start_end'][1]} / {ph['fds_start_end'][0]} → {ph['fds_start_end'][1]}", 'no growth', 'met' if ph['threads_start_end'][1] <= ph['threads_start_end'][0] and ph['fds_start_end'][1] <= ph['fds_start_end'][0] else 'MISSED'))
    rows += [
        ('Quick Open appears, measured (median, max)', f"{qo_ref['appear_s_median_max'][0] * 1000:.0f}, {qo_ref['appear_s_median_max'][1] * 1000:.0f} ms", '—', f"includes a {qo_ref['poll_s_median'] * 1000:.0f} ms tree poll"),
        ('Quick Open appears, minus one tree poll (median)', f"{qo_ref['appear_minus_poll_s_median'] * 1000:.0f} ms", '≤ 100 ms', verdict(qo_ref['appear_minus_poll_s_median'], 0.1)),
        ('Quick Open cached query, 1,000 paths (median, max)', f"{qo_ref['cached_query_s_median_max'][0] * 1000:.0f}, {qo_ref['cached_query_s_median_max'][1] * 1000:.0f} ms", '≤ 100 ms', verdict(qo_ref['cached_query_s_median_max'][0], 0.1)),
        ('10,000 paths: first scan', f"{qo_big['first_scan_s']} s", 'progressive', 'recorded'),
        ('10,000 paths: slowest UI answer during the scan', f"{qo_big['ui_answer_during_scan_s_max'] * 1000:.0f} ms", 'responsive', 'recorded'),
        ('10,000 paths: cached query (median)', f"{qo_big['cached_query_s_median_max'][0] * 1000:.0f} ms", '—', 'recorded'),
    ]
    md = ['| Metric | Observed | Target | Result |', '| --- | --- | --- | --- |'] + [f'| {a} | {b} | {c} | {d} |' for a, b, c, d in rows]
    summary = '\n'.join(md) + f"\n\nOpen tabs during idle: {', '.join(idle['tabs'])}. Quick Open timings include automation round trips (upper bounds).\n"
    (out / 'summary.md').write_text(summary)
    print(summary)


if __name__ == '__main__':
    main()
