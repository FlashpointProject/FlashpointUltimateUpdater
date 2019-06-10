import sys
import os
from cx_Freeze import setup, Executable

# Dependencies are automatically detected, but it might need fine tuning.
build_exe_options = {"packages": ["os", "asyncio", "idna.idnadata"], "excludes": ["numpy", "matplotlib"], 'include_files': ['config.json']}

PYTHON_INSTALL_DIR = os.path.dirname(os.path.dirname(os.__file__))
if sys.platform == "win32":
    build_exe_options['include_files'] += [
        os.path.join(PYTHON_INSTALL_DIR, 'DLLs', 'libcrypto-1_1.dll'),
        os.path.join(PYTHON_INSTALL_DIR, 'DLLs', 'libssl-1_1.dll'),
    ]

setup(  name = "flashpoint-updater",
        version = "0.1",
        description = "Updater for BlueMaxima's Flashpoint",
        options = {"build_exe": build_exe_options},
        executables = [Executable("update.py"), Executable("index.py")])
