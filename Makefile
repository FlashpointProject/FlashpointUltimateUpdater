bundle:
	fyne bundle -o bundle.go config.json

build-mac:
	fyne package -os darwin

build-linux:
	fyne package -os linux

build-win:
	fyne package -os windows

build:
	fyne package