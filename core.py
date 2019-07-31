from collections import namedtuple
from urllib.parse import urljoin
import threading
import requests
import datetime
import logging
import queue
import time
import lzma
import json
import math

# Allows you to retrieve the arguments passed to a function and
# an arbitrary value by passing a 'store' or '_store' argument
# as its return value. Good for use with concurrent.futures.
def wrap_call(function, *args, **kwargs):
    store = kwargs.pop('_store', kwargs.pop('store', None))
    Wrap = namedtuple('Wrap', ['result', 'store', 'args', 'kwargs'])
    return Wrap(function(*args, **kwargs), store, args, kwargs)

class IndexServer(object):
    # Index metadata schema:
    # { name.str: { path.str, lzma.bool, info.str }, ... }
    def __init__(self, endpoint):
        self.endpoint = endpoint
        r = requests.get(urljoin(self.endpoint, 'meta.json'))
        r.raise_for_status()
        self.meta = r.json()

    def available_indexes(self):
        return self.meta['indexes'].keys()

    def get_latest(self):
        return self.meta['latest']

    def get_anchor(self):
        return self.meta.get('anchor', None)

    def autodetect_anchor(self, anchor_hash):
        anchor = self.get_anchor()
        autodetect = dict()
        if anchor:
            autodetect = anchor['autodetect']
        return autodetect.get(anchor_hash, None)

    def info(self, name):
        return self.meta['indexes'][name]['info']

    def fetch(self, name, reporter, block_size=2048):
        index_meta = self.meta['indexes'][name]
        r = requests.get(urljoin(self.endpoint, index_meta['path']), stream=True)
        r.raise_for_status()
        data = bytearray()
        total_size = int(r.headers.get('content-length', 0))
        for chunk, report in reporter.task_it('Fetching index %s' % name, r.iter_content(block_size), length=math.ceil(total_size / block_size)):
            report()
            data += chunk
        if index_meta['lzma']:
            data = lzma.decompress(data)
        return json.loads(data)

class PoisonPill(object):
    pass

class Task(object):
    def __init__(self, title, unit, length):
        self.title = title
        self.unit = unit
        self.length = length

class ProgressReporter(object):
    def __init__(self, logger='reporter'):
        self._start = None
        self._stopped = False
        self._task = None
        self._report_event = threading.Event()
        self._step_queue = queue.Queue()
        self._task_queue = queue.Queue()
        self.logger = logging.getLogger(logger)

    def stop(self):
        self._stopped = True
        self._step_queue.put(PoisonPill())
        self._task_queue.put(PoisonPill())

    def is_stopped(self):
        return self._stopped

    def task(self, title, unit=None, length=None):
        if self._stopped:
            raise ValueError('operation on stopped reporter')
        self._task = Task(title, unit, length)
        self._task_queue.put(self._task)
        if not self._start:
            self._start = time.time()
        else:
            self._step_queue.put(PoisonPill())

    def task_it(self, title, iterator, unit=None, length=None):
        if not length:
            length = len(iterator) if iterator else None
        self.task(title, unit=unit, length=length)
        for item in iterator:
            if self._stopped:
                raise ValueError('operation on stopped reporter')
            yield item, self.report
            if not self._report_event.isSet():
                raise RuntimeError('report not called in previous iteration')
            self._report_event.clear()

    def report(self, payload=None):
        if not self._report_event.isSet():
            self._step_queue.put(payload)
            self._report_event.set()

    def get_current_task(self):
        return self._task

    def tasks(self):
        while True:
            payload = self._task_queue.get()
            if isinstance(payload, PoisonPill):
                break
            self.logger.info('*** Task Start: %s ***' % payload.title)
            yield payload
            self.logger.info('*** Task End: %s ***' % payload.title)

    def steps(self):
        while True:
            payload = self._step_queue.get()
            if isinstance(payload, PoisonPill):
                break
            #self.logger.info('Step > %s' % payload)
            yield payload

    def step(self, payload=None):
        if self._stopped:
            raise ValueError('operation on stopped reporter')
        self._step_queue.put(payload)

    def elapsed(self):
        elapsed = 0
        if self._start:
            elapsed = time.time() - self._start
        return str(datetime.timedelta(seconds=elapsed))
