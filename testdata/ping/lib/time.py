import datetime
import pytz


time_fmt_fs = '%Y-%m-%dT%H:%M:%S.%fZ'
time_fmt_s = '%Y-%m-%dT%H:%M:%SZ'


def parse_time(date_string):
    try:
        return datetime.datetime.strptime(date_string, time_fmt_fs).replace(tzinfo=pytz.UTC)
    except Exception:
        return datetime.datetime.strptime(date_string, time_fmt_s).replace(tzinfo=pytz.UTC)
