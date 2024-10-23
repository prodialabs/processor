import datetime
import pytz
from . import time


def transition(meta, to, message):
    now = datetime.datetime.utcnow().replace(tzinfo=pytz.UTC)
    now_str = now.strftime(time.time_fmt_fs)

    if len(meta['state']['history']) > 0:
        last = meta['state']['history'][-1]
        last_at = time.parse_time(last['at'])
        dwell = now - last_at
        last['dwell'] = dwell.total_seconds()

    meta['updated_at'] = now_str
    meta['state']['history'].append({
        'from': meta['state']['current'],
        'to': to,
        'at': now_str,
        'message': message,
    })
    meta['state']['current'] = to


def set_metrics(meta, start, stop, steps=None):
    metrics = meta.get('metrics', {})

    elapsed = stop - start
    metrics['elapsed'] = elapsed

    if steps is not None:
        ips = float(steps) / elapsed
        metrics['ips'] = ips

    meta['metrics'] = metrics
