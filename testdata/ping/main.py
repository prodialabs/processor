#!/usr/bin/env python3
import os
import sys
import time


import lib


lib.log('starting...')

switching = {}
thrashing = 1

bootstrap = os.environ.get('PROCESSOR_BOOTSTRAP', 'false')
if bootstrap == 'true':
    sys.exit(0)

x_factor = 1


def handle_ping(meta, parts):
    meta_config = meta.get('config', {})

    start = time.time()

    delay = meta_config.get('delay', 0.1)

    lib.log(f"delaying for {delay} seconds")
    time.sleep(delay / x_factor)

    stop = time.time()

    lib.set_metrics(meta, start, stop)
    lib.transition(meta, 'processed', 'Job processed.')

    meta['done'] = True
    meta['ok'] = True

    return meta, parts


def handle_optimize(meta, parts):
    del parts
    global x_factor
    global thrashing

    meta_config = meta.get('config', {})

    start = time.time()
    x_factor = meta_config.get('x_factor', 1)
    stop = time.time()

    thrashing += stop - start
    #lib.set_metrics(job, start, stop)
    #lib.transition(job, 'processed', 'Job processed.')

    meta['done'] = True
    meta['ok'] = True

    return meta, []


lib.handle_work(switching, thrashing, {
    'ping.v1': handle_ping,
    'ping.optimize.v1': handle_optimize,
})
