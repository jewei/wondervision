#!/usr/bin/env python3
"""Idle soak test (spec §9 and §13): run the production binary with the
reference workload and a few open files for --hours, sampling memory, threads,
file descriptors, CPU and internet sockets every --interval seconds.

  python3 scripts/soak.py run --out build/verification/soak --hours 24
  python3 scripts/soak.py summarize build/verification/soak

Samples are appended to samples.jsonl as they are taken, so an interrupted run
can still be summarized. Pass criteria (spec §9): after a settling period, no
steady memory climb and at most 20 MiB sustained increase over the settled
baseline; threads and file descriptors don't grow; zero internet sockets.
Run under a virtual display with a window manager, like the GUI verifiers.
"""
import argparse
import json
from pathlib import Path
import shutil
import statistics
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, str(Path(__file__).resolve().parent))
from measure_performance import CLK_TCK, Proc, env_for, open_tabs, seed_reference  # noqa: E402
from verify_native import Accessibility  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
GROWTH_LIMIT_MIB = 20
SLOPE_LIMIT_MIB_PER_HOUR = 0.5


def run(args):
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    scratch = Path(tempfile.mkdtemp(prefix='wondervision-soak-'))
    meta = {
        'source': subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True, capture_output=True).stdout.strip(),
        'planned_hours': args.hours, 'interval_s': args.interval,
        'started': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
    }
    p = None
    try:
        ref = scratch / 'reference'
        seed_reference(ref)
        p = subprocess.Popen([str(args.binary.resolve())], env=env_for(scratch, ref, ref / 'report.md'),
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        acc = Accessibility(p.pid)
        meta['tabs'] = open_tabs(acc, p)
        meta['pid'] = p.pid
        time.sleep(30)
        subprocess.run(['import', '-window', 'root', str(out / 'soak-workload.png')], stderr=subprocess.DEVNULL)
        (out / 'meta.json').write_text(json.dumps(meta, indent=2))
        proc = Proc(p.pid)
        start = time.monotonic()
        end = start + args.hours * 3600
        with (out / 'samples.jsonl').open('a') as f:
            while True:
                if p.poll() is not None:
                    f.write(json.dumps({'exited': p.returncode, 'hours': (time.monotonic() - start) / 3600}) + '\n')
                    f.flush()
                    break
                s = proc.sample()
                s['hours'] = round((s['time'] - start) / 3600, 4)
                f.write(json.dumps(s) + '\n')
                f.flush()
                if s['time'] >= end:
                    break
                time.sleep(min(args.interval, max(0.0, end - time.monotonic())))
    finally:
        if p is not None and p.poll() is None:
            p.terminate()
            p.wait(timeout=10)
        shutil.rmtree(scratch, ignore_errors=True)
    print(summarize(out))
    # A leak verdict fails the run (and CI); the memory target line doesn't.
    if json.loads((out / 'summary.json').read_text())['verdict'] == 'FAIL':
        sys.exit(1)


def slope(xs, ys):
    """Least-squares slope of ys over xs."""
    mx, my = statistics.fmean(xs), statistics.fmean(ys)
    den = sum((x - mx) ** 2 for x in xs)
    return 0.0 if den == 0 else sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / den


def summarize(out):
    out = Path(out)
    meta = json.loads((out / 'meta.json').read_text())
    lines = [json.loads(line) for line in (out / 'samples.jsonl').read_text().splitlines() if line.strip()]
    exited = next((s for s in lines if 'exited' in s), None)
    samples = [s for s in lines if 'rss_kib' in s]
    if len(samples) < 3:
        return 'Not enough samples yet.'
    hours = samples[-1]['hours']
    # Settle: the first hour, or the first 20% of a shorter run, while Go
    # returns startup memory to the OS.
    settle = min(1.0, hours * 0.2)
    window = max(settle, min(1.0, hours * 0.2))
    settled = [s for s in samples if s['hours'] >= settle]
    base = [s for s in settled if s['hours'] < settle + window] or settled[:1]
    last = [s for s in settled if s['hours'] >= hours - window] or settled[-1:]
    mib = lambda s: s['rss_kib'] / 1024  # noqa: E731
    baseline, final = statistics.median(map(mib, base)), statistics.median(map(mib, last))
    growth = final - baseline
    trend = slope([s['hours'] for s in settled], [mib(s) for s in settled]) if len(settled) > 2 else 0.0
    cpu = 100.0 * (samples[-1]['cpu_ticks'] - samples[0]['cpu_ticks']) / CLK_TCK / max(1e-9, samples[-1]['time'] - samples[0]['time'])
    first, end = settled[0], samples[-1]
    checks = {
        'still running': exited is None,
        f'sustained RSS growth ≤ {GROWTH_LIMIT_MIB} MiB': growth <= GROWTH_LIMIT_MIB,
        # A trend over minutes is noise; judge it only on runs of 2 hours or more.
        f'RSS trend ≤ {SLOPE_LIMIT_MIB_PER_HOUR} MiB/h after settling (judged from 2 h)': hours < 2 or trend <= SLOPE_LIMIT_MIB_PER_HOUR,
        'threads do not grow': end['threads'] <= first['threads'],
        'file descriptors do not grow': end['fds'] <= first['fds'],
        'zero internet sockets': max(s['internet_sockets'] for s in samples) == 0,
    }
    status = 'complete' if hours >= meta['planned_hours'] - 0.01 else f'partial ({hours:.2f} of {meta["planned_hours"]} h)'
    verdict = 'PASS' if all(checks.values()) else 'FAIL'
    if status != 'complete' and verdict == 'PASS':
        verdict = 'PASS so far'
    result = {
        'source': meta['source'], 'started': meta['started'], 'status': status, 'verdict': verdict,
        'hours': round(hours, 2), 'samples': len(samples), 'settle_hours': round(settle, 2),
        'rss_mib': {'start': round(mib(samples[0]), 1), 'baseline': round(baseline, 1), 'final': round(final, 1),
                    'max': round(max(map(mib, samples)), 1), 'growth': round(growth, 1), 'trend_per_hour': round(trend, 3)},
        'threads': [first['threads'], end['threads']], 'fds': [first['fds'], end['fds']],
        'cpu_percent_one_core': round(cpu, 3), 'internet_sockets_max': max(s['internet_sockets'] for s in samples),
        'checks': checks, 'exited': exited, 'tabs': meta.get('tabs'),
    }
    (out / 'summary.json').write_text(json.dumps(result, indent=2))
    rows = [f"Soak {status}: {verdict}. {result['hours']} h, {len(samples)} samples, source {meta['source'][:7]}.",
            f"RSS start {result['rss_mib']['start']} MiB, settled baseline {result['rss_mib']['baseline']} MiB, "
            f"final {result['rss_mib']['final']} MiB (growth {result['rss_mib']['growth']:+} MiB, trend "
            f"{result['rss_mib']['trend_per_hour']:+} MiB/h, max {result['rss_mib']['max']} MiB).",
            f"Threads {first['threads']} → {end['threads']}; fds {first['fds']} → {end['fds']}; "
            f"CPU {result['cpu_percent_one_core']}% of one core; internet sockets {result['internet_sockets_max']}."]
    rows += [f"- {'ok ' if ok else 'FAIL'} {name}" for name, ok in checks.items()]
    # The spec's memory budget is a separate target, not a leak criterion.
    peak = result['rss_mib']['max']
    rows.append(f"Spec §9 memory target (≤ 150 MiB desired): {'met' if peak <= 150 else 'MISSED'} at peak {peak} MiB.")
    text = '\n'.join(rows) + '\n'
    (out / 'summary.md').write_text(text)
    return text


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest='command', required=True)
    r = sub.add_parser('run')
    r.add_argument('--binary', type=Path, default=ROOT / 'build/wondervision')
    r.add_argument('--out', type=Path, required=True)
    r.add_argument('--hours', type=float, default=24)
    r.add_argument('--interval', type=float, default=300, help='seconds between samples')
    s = sub.add_parser('summarize')
    s.add_argument('out', type=Path)
    args = parser.parse_args()
    if args.command == 'run':
        run(args)
    else:
        print(summarize(args.out))


if __name__ == '__main__':
    main()
