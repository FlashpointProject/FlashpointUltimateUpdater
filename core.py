from collections import namedtuple
from urllib.parse import urljoin
import concurrent.futures
import threading
import requests
import datetime
import logging
import queue
import time
import lzma
import json
import math

# Calls a function and returns an object with the arguments used, its
# return value and an arbitrary value provided by 'store' or '_store'.
# In addition, a callable may be passed to either 'cancel' or '_cancel'
# that may override the return value if it evaluates to a truthy value,
# in which case, the original call will not be made. The callable must
# take no arguments. This is intended for use with concurrent.futures.
def wrap_call(function, *args, **kwargs):
    store = kwargs.pop('_store', kwargs.pop('store', None))
    cancel = kwargs.pop('_cancel', kwargs.pop('cancel', lambda: None))
    Wrap = namedtuple('Wrap', ['result', 'store', 'args', 'kwargs'])
    return Wrap(cancel() or function(*args, **kwargs), store, args, kwargs)

class BufferedExecutor(object):
    def __init__(self, submit_size, *args, **kwargs):
        self._submit_size = submit_size
        self._executor = concurrent.futures.ThreadPoolExecutor(*args, **kwargs)
        self._buffer = list()
        self._shutdown = False

    def submit(self, fn, *args, **kwargs):
        self._buffer.append((fn, args, kwargs))

    def __submit_from_buffer(self):
        fn, args, kwargs = self._buffer.pop(0)
        return self._executor.submit(fn, *args, **kwargs)

    def as_completed(self):
        submitted = [self.__submit_from_buffer() for _ in range(self._submit_size)]
        while self._buffer and not self._shutdown:
            done, _ = concurrent.futures.wait(submitted, return_when=concurrent.futures.FIRST_COMPLETED)
            for future in done:
                submitted.remove(future)
                submitted.append(self.__submit_from_buffer())
                yield future

    def shutdown(self, wait=True):
        self._shutdown = True
        self._executor.shutdown(wait=wait)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.shutdown(wait=True)
        return False

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

    def available_updates(self):
        return list(map(lambda x: x[0], filter(lambda x: 'root' in x[1], self.meta['indexes'].items())))

    def get_latest(self):
        return self.meta['latest']

    def get_anchor(self):
        return self.meta.get('anchor', None)

    def get_root(self, name):
        return urljoin(self.endpoint, self.index(name)['root'])

    def autodetect_anchor(self, anchor_hash):
        anchor = self.get_anchor()
        autodetect = dict()
        if anchor:
            autodetect = anchor['autodetect']
        return autodetect.get(anchor_hash, None)

    def index(self, name):
        return self.meta['indexes'][name]

    def fetch(self, name, reporter, block_size=2048):
        index_meta = self.index(name)
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
