#!/usr/bin/env python3
from core import IndexServer, Task, ProgressReporter
from tqdm import tqdm
from urllib.parse import quote, urljoin
from concurrent.futures import as_completed
import concurrent.futures
import threading
import urllib3
import datetime
import requests
import backoff
import shutil
import index
import stat
import json
import lzma
import time
import core
import sys
import os

@backoff.on_exception(backoff.expo, (requests.exceptions.RequestException, urllib3.exceptions.ProtocolError, urllib3.exceptions.ReadTimeoutError), logger='reporter')
def download_file(session, url, dest):
    with session.get(url, stream=True, timeout=10) as r:
        with open(dest, 'wb') as f:
            shutil.copyfileobj(r.raw, f)

# Fix for "read-only" files on Windows
def chown_file(path):
    os.chmod(path, stat.S_IRWXU | stat.S_IRWXG | stat.S_IRWXO)

def perform_update(flashpoint, current, target, file_endpoint, reporter):
    tmp = os.path.join(flashpoint, '.tmp')
    try:
        os.mkdir(tmp)
    except FileExistsError:
        reporter.logger.info('Temp folder exists. We are resuming.')
    to_download = list()
    for hash, report in reporter.task_it('Preparing contents...', target['files'], unit='hash'):
        report(hash)
        tmpPath = os.path.join(tmp, hash)
        if hash in current['files']:
            path = os.path.normpath(current['files'][hash][0])
            if not os.path.isfile(tmpPath):
                try:
                    os.rename(os.path.join(flashpoint, path), tmpPath)
                except FileNotFoundError:
                    reporter.logger.warning('File from current index not found. Queued for download: %s (%s)' % (path, hash))
                    to_download.append(hash)
        elif not (os.path.isfile(tmpPath) and index.hash(tmpPath, 'sha1') == hash):
            to_download.append(hash)
        else:
            reporter.logger.info('File from target index already in temp folder. Skipped: %s' % hash)

    session = requests.Session()
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
        tasks = list()
        for hash in to_download:
            path = target['files'][hash][0]
            url = urljoin(file_endpoint, quote(path))
            tasks.append(executor.submit(core.wrap_call, download_file, session, url, os.path.join(tmp, hash), store=path))
        for future, report in reporter.task_it('Downloading new data...', as_completed(tasks), length=len(tasks), unit='file'):
            report(os.path.basename(future.result().store))

    reporter.task('Removing obsolete files...')
    for r, d, f in os.walk(flashpoint, topdown=False):
        if r == tmp:
            continue
        for x in f:
            path = os.path.join(r, x)
            chown_file(path)
            os.remove(path)
        for x in d:
            path = os.path.join(r, x)
            if path != tmp:
                chown_file(path)
                os.rmdir(path)

    for hash, report in reporter.task_it('Creating file structure...', target['files'], unit='hash'):
        report(hash)
        paths = target['files'][hash]
        while paths:
            path = os.path.normpath(paths.pop(0))
            parent = os.path.dirname(path)
            if parent:
                os.makedirs(os.path.join(flashpoint, parent), exist_ok=True)
            tmpfile = os.path.join(tmp, hash)
            dest = os.path.join(flashpoint, path)
            if paths:
                shutil.copy(tmpfile, dest)
            else: # No more paths, we can move instead
                os.rename(tmpfile, dest)

    for path in target['empty']:
        os.makedirs(os.path.join(flashpoint, os.path.normpath(path)))

    os.rmdir(tmp)
    reporter.stop()

if __name__ == '__main__':

    with open('config.json', 'r') as f:
        config = json.load(f)

    if len(sys.argv) != 4:
        print('Usage: update.py <flashpoint-path> <current-version> <target-version>')
        sys.exit(0)

    flashpoint = index.win_path(sys.argv[1])
    if not os.path.isdir(flashpoint):
        print('Error: Flashpoint path not found.')
        sys.exit(0)

    try:
        server = IndexServer(config['index_endpoint'])
    except requests.exceptions.RequestException as e:
        print('Could not retrieve index metadata: %s' % str(e))
        sys.exit(0)

    def worker(reporter, root_path, server, current, target):
        if target not in server.available_indexes():
            print('Could not find index: %s' % target)
            reporter.stop()
            return
        if target not in server.available_updates():
            print('Target version not available as an update')
            reporter.stop()
            return
        remote_root = server.get_root(target)
        try:
            current, target = server.fetch(current, reporter), server.fetch(target, reporter)
        except requests.exceptions.RequestException as e:
            print('Could not retrieve index: %s' % str(e))
            reporter.stop()
            return
        perform_update(root_path, current, target, remote_root, reporter)

    reporter = ProgressReporter()
    threading.Thread(target=worker, args=(reporter, flashpoint, server, sys.argv[2], sys.argv[3])).start()
    for task in reporter.tasks():
        print(task.title)
        for step in tqdm(reporter.steps(), total=task.length, unit=task.unit or 'it', ascii=True):
            pass

    print('Update completed in %s' % reporter.elapsed())
