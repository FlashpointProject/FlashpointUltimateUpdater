# Flashpoint Updater

The updater for BlueMaxima's Flashpoint.

## End-user Setup

### Windows

**GUI Version (Recommended)**

1. Download the [latest release](https://github.com/FlashpointProject/FlashpointUpdater/releases/latest).
2. Unpack anywhere.
3. Run FlashpointUpdaterQt.exe
4. Select your Flashpoint path and target version.
5. Click "Go!"

**CLI Version**

1. Download the latest release.
2. Unpack anywhere.
3. Run it from the command-line as such:
`FlashpointUpdater.exe <flashpoint-path> <current-version> <target-version>`

##### Example: `FlashpointUpdater.exe C:\Flashpoint 5.5 6.1`

### Mac/Linux

1. Install Python 3.
2. Clone the repository.
3. Run in the project root: `pip install -r requirements.txt`
4. Use it like: `update.py /media/ext1/Flashpoint <flashpoint-path> <current-version> <target-version>`

Or launch the GUI version with `update_gui.py`.

## Server Setup

The updater works by fetching differing files from two version indexes. These indexes contain SHA-1 hashes of all the files in the project mapped to an array of their paths.

The updater script expects an index listing to be available in a file named `meta.json` at the location specified in `config.json`. Example: `https://unstable.life/fp-index/meta.json`

This listing must obey the following structure (note the following is not valid JSON):

    {
      "latest": "1.0", # key of the latest index
      "indexes": {
        "1.0": {
          "path": "/fp-index/1.0.json.xz", # relative path to the file that holds the index
          "root": "/fp/", # relative path to where the flashpoint data resides
          "lzma": true, # whether LZMA compression was applied to the index file
          "info": "blank" # currently unused: should contain a changelog or description of some sort
        }
      },
      "anchor": {
        "file": "changelog.txt", # file that marks the "root" directory (for autodetection)
        "autodetect": {
          "3e8993cbe4bdb51d32c9468020fb72c4a74d08bd": "1.0", # detect the origin index based on the SHA-1 hash of the file
          "b0b0050795fa20a52657602141de8ede6b5db00f": "some_other_version"
        }
      }
    }

To generate indexes, use `index.py`: `index.py /media/ext1/Flashpoint 6.2.json.xz`
