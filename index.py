#!/usr/bin/env python3
from tqdm import tqdm
import os
import sys
import hashlib
import posixpath
import sqlite3


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
                path = path[1::]  # Remove leading slash
            path = prefix + path
    return path


def hash(file, bufsize=2 ** 16):
    h = hashlib.new('sha1')
    with open(file, 'rb') as f:
        buf = f.read(bufsize)
        while len(buf) > 0:
            h.update(buf)
            buf = f.read(bufsize)
    return h.hexdigest()


def index(path, name, base_url, db_file):
    # Delete the database file if it already exists
    if os.path.exists(db_file):
        os.remove(db_file)

    # Create Database
    conn = sqlite3.connect(db_file)
    cur = conn.cursor()

    # Set up database
    insert_empty_dir_query = "INSERT INTO empty_dirs (path) VALUES (?)"
    insert_file_query = "INSERT INTO files (path, size, sha1) VALUES (?, ?, ?)"
    overview_schema = """
    CREATE TABLE overview (
        name TEXT PRIMARY KEY,
        total_files INTEGER,
        total_size INTEGER,
        base_url TEXT
    );
    """
    files_schema = """
    CREATE TABLE files (
        path TEXT PRIMARY KEY,
        size INTEGER,
        sha1 INTEGER,
        taken INTEGER DEFAULT false,
        done INTEGER DEFAULT false
    );
    """
    dirs_schema = """
    CREATE TABLE empty_dirs (
        path TEXT PRIMARY KEY,
        done INTEGER DEFAULT false
    );
    """
    index_statement = """
    CREATE INDEX sha_idx ON files (sha1);
    """
    index2_statement = """
    CREATE INDEX done_idx ON files (done, taken);
    """
    cur.execute(overview_schema)
    cur.execute(dirs_schema)
    cur.execute(files_schema)
    cur.execute(index_statement)
    cur.execute(index2_statement)
    conn.commit()

    cur = conn.cursor()

    path = win_path(path)
    entries = list()
    with tqdm(unit=' files') as pbar:
        for root, dirs, files in os.walk(path):
            # Include empty folders
            rel = os.path.relpath(root, path).replace(os.path.sep, '/')
            if len(dirs) == 0 and len(files) == 0:
                cur.execute(insert_empty_dir_query, (rel,))
            else:
                for x, f in ((x if rel == '.' else posixpath.join(rel, x), os.path.join(root, x)) for x in files):
                    size = os.path.getsize(f)
                    entries.append((x, size, hash(f)))
                    pbar.update(1)
            if len(entries) > 100:
                cur.executemany(insert_file_query, entries)
                entries.clear()
    # Get overview info
    cur.execute("SELECT SUM(size), COUNT(*) FROM files;")
    res = cur.fetchone()
    cur.execute("INSERT INTO overview (name, total_size, total_files, base_url) VALUES (?,?,?,?);",
                (name, res[0], res[1], base_url))
    conn.commit()
    return


if __name__ == '__main__':
    if len(sys.argv) != 5:
        print('Usage: index.py <path> <index_name> <base_url> <out.sqlite>')
        sys.exit(0)

    index(sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4])
