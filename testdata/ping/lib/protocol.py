import json
import sys
import time

from .job import transition

def log(message):
    sys.stderr.write(message)
    sys.stderr.write('\n')
    sys.stderr.flush()


def write_json_line(obj):
    sys.stdout.buffer.write(json.dumps(obj).encode())
    sys.stdout.buffer.write('\n'.encode())
    sys.stdout.flush()


def write_binary(data):
    sys.stdout.buffer.write(data)
    sys.stdout.flush()


def read_json_line():
    line = sys.stdin.buffer.readline()
    if line is None or len(line) == 0:
        return None

    return json.loads(line)


def load_payload(part):
    payload = b""
    boundary = part.get('boundary', '').encode('utf-8')

    if len(boundary) == 0:
        return

    boundary = b"\n" + boundary + b"\n"
    while True:
        byte = sys.stdin.buffer.read(1)
        if not byte:
            return

        payload += byte

        if payload.endswith(boundary):
            part['payload'] = payload[:-len(boundary)]
            return


def write_payload(boundary, payload):
    write_binary(payload)
    write_binary(boundary.encode('utf-8'))


def handle_work(switching, thrashing, cbs):
    while True:
        write_json_line({
            'kind': 'control',
            'type': 'processor.ready.v1',
            'state': {
                'switching': switching,
                'thrashing': thrashing,
            }
        })

        log('ready & waiting...')

        part = read_json_line()
        if part is None:
            return

        load_payload(part)
        log(f"received part: {part}")

        write_json_line({
            'kind': 'control',
            'type': 'processor.last.v1',
        })

        meta = None
        parts = []

        while True:
            if part['kind'] == 'control':
                continue

            if part['kind'] == 'meta' and part['type'] == 'processor.last.v1':
                break

            if part['kind'] == 'meta':
                meta = part
            else:
                parts.append(part)

            part = read_json_line()
            if part is None:
                break

            load_payload(part)
            log(f"received part: {part}")

        if meta is None:
            return

        if meta.get('done', False):
            log("job already done")

            write_json_line(meta)

            for part in parts:
                payload = part['payload']
                del part['payload']

                if len(part['boundary']) > 0:
                    write_json_line(part)
                    write_payload(part['boundary'], payload)

            write_json_line({
                'kind': 'meta',
                'type': 'processor.last.v1',
            })

            log('jobs done')

            return

        try:
            cb = cbs[meta['type']]
        except Exception:
            log(f"fatal, processor misconfigured, no handler for {meta['type']}")
            sys.exit(1)

        try:
            meta, parts = cb(meta, parts)
        except Exception as e:
            log(f"job failed: {e}")

            # TODO: emit error

            meta['done'] = True
            meta['ok'] = False

        log(f"sending job response: {meta}")

        if meta is None:
            continue

        for part in parts + [meta]:
            write_json_line(part)

            if 'boundary' in part and len(part['boundary']) > 0:
                payload = part['payload']
                del part['payload']

                write_payload(part['boundary'], payload)

        write_json_line({
            'kind': 'meta',
            'type': 'processor.last.v1',
        })

        log('jobs done')
