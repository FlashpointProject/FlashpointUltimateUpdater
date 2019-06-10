#!/usr/bin/env python3
from tqdm import tqdm
import json
import lzma
import os
import sys
import hashlib
import posixpath

# Allows accessing files that exceed MAX_PATH in Windows
# See: https://docs.microsoft.com/en-us/windows/desktop/fileio/naming-a-file#maximum-path-length-limitation
def win_path(path):
    if os.name == 'nt':
        path = os.path.abspath(path)
        prefix = '\\\\?\\'
        if not path.startswith(prefix):
            # Handle shared paths
            if path.startswith('\\\\'):
                prefix += 'UNC'
                path = path[1::] # Remove leading slash
            path = prefix + path
    return path

def hash(file, hashalg, bufsize=2**16):
    hash = hashlib.new(hashalg)
    with open(file, 'rb') as f:
        buf = f.read(bufsize)
        while len(buf) > 0:
            hash.update(buf)
            buf = f.read(bufsize)
    return hash.hexdigest()

def index(path, hashalg):
    files = dict()
    empty = list()
    path = win_path(path)
    with tqdm(unit=' files') as pbar:
        for r, d, f in os.walk(path):
            # Include empty folders
            rel = os.path.relpath(r, path).replace(os.path.sep, '/')
            if len(d) == 0 and len(f) == 0:
                empty.append(rel)
            else:
                for x, f in ((x if rel == '.' else posixpath.join(rel, x), os.path.join(r, x)) for x in f):
                    files.setdefault(hash(f, hashalg), list()).append(x)
                    pbar.update(1)
    return files, empty

if __name__ == '__main__':

    if len(sys.argv) != 3:
        print('Usage: index.py <path> <out.json.xz>')
        sys.exit(0)

    files, empty = index(sys.argv[1], 'sha1')
    print('Applying LZMA compression...')
    with lzma.open(sys.argv[2], 'wt', encoding='utf-8', preset=9) as f:
        json.dump({'files': files, 'empty': empty}, f, separators=(',', ':'), ensure_ascii=False)
