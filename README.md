# Flashpoint Updater

The updater for BlueMaxima's Flashpoint.
Currently a work in progress.

## End-user Setup

### Windows

1. Download the latest release.
2. Unpack anywhere.
3. Run it from the command-line as such:
`update.exe <flashpoint-path> <current-version> <target-version>`

##### Example: `update.exe C:\Flashpoint 5.5 6.1`

### Mac/Linux

1. Install Python 3.
2. Clone the repository.
3. Run in the project root: `pip install -r requirements.txt`
4. Use it like: `update.py /media/ext1/Flashpoint <flashpoint-path> <current-version> <target-version>` 

## Server Setup

The updater works by fetching differing files from two version indexes. These indexes contain SHA-1 hashes of all the files in the project mapped to an array of their paths.

The updater script expects indexes to be available at the location specified by `index_endpoint` in `config.json`. Example: `https://unstable.life/fp-index/6.1.json.xz`

Similarly, files will be fetched in the location specified by `file_endpoint`.

To generate indexes, use `index.py`: `index.py /media/ext1/Flashpoint 6.2.json.xz`
