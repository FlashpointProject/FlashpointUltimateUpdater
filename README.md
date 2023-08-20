# Flashpoint Ultimate Updater

![Screenshot](/screenshot.png?raw=true "Screenshot")

Flashpoint Ultimate Updater is designed to install extremely large numbers of immutable files from a remote server. Built on the back of the [grab](https://github.com/cavaliergopher/grab) library.

Features:
- Resumable install state
- Speed limitter
- Pause and start downloader
- Scan and repair existing files
- Resume partial file downloads
- Install information fetched from remote server
- Upgrade existing install to new version
- Force upgrade when version no longer available on remote server

## Setting up your remote server

1. Install python and install indexer dependencies
```
python -m pip install -r requirements.txt
```

2. Create an index for each install version you want to serve. Save the resulting sqlite file to somewhere accessible on your webserver you wish to keep the updater metadata.
 - `directory_to_scan` - Directory of static files to index
 - `version_name` - Version name to show in the updater
 - `serve_url` - URL to where the directory of static files will be available on

```
python ./index.py <directory_to_scan> <version_name> <serve_url> <output.sqlite>
```

3. Create the metadata file that is fetched by the updater. Save to `meta.json` somewhere accessible online.
 - `current` - Version name of the current version. Must match `version_name` from index above
 - `path` - URL to the current sqlite file.
 - `available` - List of available versions still being hosted, same format as `current`. If any are removed, users will be forced to upgrade to the current version.
```json
{
  "current": "Release 2.0",
  "path": "https://example.com/updater-data/release-2.0.sqlite",
  "available": [
    "Release 1.0",
    "Release 2.0"
  ]
}
```

4. Edit the built in config.json to point to the `meta.json` file and compile your version. Placing config.json manually next to any compiled executable will override this.
```json
{
  "meta_url": "https://example.com/updater-data/meta.json"
}
```

Keep reading on to **Building** to create an updater with the new config.json

## Building

1. Bundle the config json

`fyne bundle -o bundle.go config.json`

2. Package for desired operation system and set the icon

`fyne package -os <windows/linux/darwin> -icon icon.png`

Ta-da, there's a new executable in the folder!