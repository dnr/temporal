#!/usr/bin/env python

import sys, os, datetime, subprocess
from time import sleep

serverwd = '/home/dnr/t/temporal'
starterwd = workerwd = '/home/dnr/t/samples-go/repro'

outbase = os.path.join(serverwd, f'run.out.{datetime.datetime.now().isoformat("T", "seconds").replace(":","")}')
os.makedirs(outbase)

_idx = 1
liveprocs = []

def proc(logname, wd, args):
    global liveprocs, _idx

    logname = ('%05d' % _idx) + '.' + logname
    _idx += 1
    out = open(os.path.join(outbase, logname), 'w')
    print('+', ' '.join(args))
    p = subprocess.Popen(args, stdout=out, stderr=subprocess.STDOUT, cwd=wd)
    liveprocs.append(p)
    return p

def cleanup():
    global liveprocs
    print('+ cleaning up...')
    liveprocs = [p for p in liveprocs if p.poll() is None]
    for p in liveprocs: p.terminate()
    while liveprocs:
        sleep(0.5)
        liveprocs = [p for p in liveprocs if p.poll() is None]

server = lambda logname, env, services: proc(
        logname, serverwd,
        ['./temporal-server', '--allow-no-auth', f'--env={env}', 'start'] +
        [f'--service={s}' for s in services])

rest = lambda: server(f'rest', 'r1', ['frontend', 'history', 'worker'])
matching = lambda i: server(f'matching-{i}', f'r{i}', ['matching'])

_starter = lambda logname, rps: proc(logname, starterwd, ['go', 'run', './starter', '--rps', str(rps)])
starter = lambda i: _starter(f'starter-{i}', 20)

_worker = lambda logname, pollers, tps: proc(
        logname, workerwd, ['go', 'run', './worker', '--pollers', str(pollers), '--tps', str(tps)])
worker = lambda i: _worker(f'worker-{i}', 2, 35)


def cycle():
    r = rest()
    #sleep(1)
    m1 = matching(1)
    sleep(1)
    m2 = matching(2)
    sleep(1)
    m3 = matching(3)

    sleep(2)

    w1 = worker(1)
    w2 = worker(2)

    sleep(2)

    s1 = starter(1)
    s2 = starter(2)

    for i in range(10):
        sleep(10)
        m1.terminate()
        sleep(5)
        m1 = matching(1)

    sleep(10)

    cleanup()


def main():
    try:
        cycle()
    finally:
        cleanup()


if __name__ == '__main__':
    main()
