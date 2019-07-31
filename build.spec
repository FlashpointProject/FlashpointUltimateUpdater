# -*- mode: python ; coding: utf-8 -*-

block_cipher = None


a = Analysis(['update.py'],
             binaries=[],
             datas=[],
             hiddenimports=[],
             hookspath=[],
             runtime_hooks=[],
             excludes=['matplotlib', 'numpy'],
             win_no_prefer_redirects=False,
             win_private_assemblies=False,
             cipher=block_cipher,
             noarchive=False)
pyz = PYZ(a.pure, a.zipped_data,
             cipher=block_cipher)
exe = EXE(pyz,
          a.scripts,
          a.binaries,
          a.zipfiles,
          a.datas,
          [],
          name='FlashpointUpdater',
          debug=False,
          manifest='asInvoker.manifest',
          bootloader_ignore_signals=False,
          strip=False,
          upx=True,
          upx_exclude=[],
          runtime_tmpdir=None,
          console=True , icon='icon.ico')

b = Analysis(['update_gui.py'],
             binaries=[],
             datas=[('icon.png', '.')],
             hiddenimports=[],
             hookspath=[],
             runtime_hooks=[],
             excludes=['matplotlib', 'numpy'],
             win_no_prefer_redirects=False,
             win_private_assemblies=False,
             cipher=block_cipher,
             noarchive=False)
pyz = PYZ(b.pure, b.zipped_data,
             cipher=block_cipher)
exe = EXE(pyz,
          b.scripts,
          b.binaries,
          b.zipfiles,
          b.datas,
          [],
          name='FlashpointUpdaterQt',
          debug=False,
          manifest='asInvoker.manifest',
          bootloader_ignore_signals=False,
          strip=False,
          upx=True,
          upx_exclude=[],
          runtime_tmpdir=None,
          console=False , icon='icon.ico')
