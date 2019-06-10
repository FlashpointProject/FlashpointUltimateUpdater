#!/usr/bin/env python3
from index import win_path
from tqdm import tqdm
from urllib.parse import quote
from concurrent.futures import as_completed
import concurrent.futures
import urllib3
import datetime
import requests
import backoff
import shutil
import stat
import json
import lzma
import time
import sys
import os

@backoff.on_exception(backoff.expo, (requests.exceptions.RequestException, urllib3.exceptions.ProtocolError))
def download_file(session, url, dest):
    with session.get(url, stream=True, timeout=10) as r:
        with open(dest, 'wb') as f:
            shutil.copyfileobj(r.raw, f)

# Fix for "read-only" files on Windows
def chown_file(path):
    os.chmod(path, stat.S_IRWXU | stat.S_IRWXG | stat.S_IRWXO)

def fetch_index(version, endpoint):
    r = requests.get('%s/%s.json.xz' % (endpoint, version))
    return json.loads(lzma.decompress(r.content))

if __name__ == '__main__':

    with open('config.json', 'r') as f:
        config = json.load(f)

    if len(sys.argv) != 4:
        print('Usage: update.py <flashpoint-path> <current-version> <target-version>')
        sys.exit(0)

    flashpoint = win_path(sys.argv[1])
    if not os.path.isdir(flashpoint):
        print('Error: Flashpoint path not found.')
        sys.exit(0)

    endpoint = config['index_endpoint']
    try:
        current, target = fetch_index(sys.argv[2], endpoint), fetch_index(sys.argv[3], endpoint)
    except requests.exceptions.RequestException:
        print('Could not retrieve indexes for the versions specified.')
        sys.exit(0)

    start = time.time()
    tmp = os.path.join(flashpoint, '.tmp')
    os.mkdir(tmp)
    to_download = list()
    print('Preparing contents...')
    for hash in tqdm(target['files'], unit=' files', ascii=True):
        if hash in current['files']:
            path = os.path.normpath(current['files'][hash][0])
            os.rename(os.path.join(flashpoint, path), os.path.join(tmp, hash))
        else:
            to_download.append(hash)

    print('Downloading new data...')
    session = requests.Session()
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
        tasks = list()
        for hash in to_download:
            url = '%s/%s' % (config['file_endpoint'], quote(target['files'][hash][0]))
            tasks.append(executor.submit(download_file, session, url, os.path.join(tmp, hash)))
        for future in tqdm(as_completed(tasks), total=len(tasks), unit=' files', ascii=True):
            future.result()

    print('Removing obsolete files...')
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

    print('Creating file structure...')
    for hash in tqdm(target['files'], unit=' files', ascii=True):
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
    print('Update completed in %s' % str(datetime.timedelta(seconds=time.time() - start)))
