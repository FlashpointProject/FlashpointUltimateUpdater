#!/usr/bin/env python3
from core import IndexServer, Task, ProgressReporter
from PyQt5.QtGui import QIcon
from PyQt5.QtCore import QThread, QSize, Qt, pyqtSignal
from PyQt5.QtWidgets import *
from datetime import datetime
import threading
import requests
import logging
import shutil
import ctypes
import update
import index
import json
import glob
import time
import sys
import os
import re

class UpdateThread(QThread):
    sig_exc = pyqtSignal(Exception)

    def __init__(self, reporter, root_path, server, current, target):
        QThread.__init__(self)
        self.reporter = reporter
        self.root_path = root_path
        self.server = server
        self.current = current
        self.target = target

    def create_backup(self, backup_paths):
        files = list()
        for rel_glob in backup_paths:
            path_glob = os.path.join(self.root_path, rel_glob)
            for path in glob.glob(path_glob):
                if not os.path.isfile(path):
                    continue
                rel = os.path.relpath(path, self.root_path)
                files.append((path, rel))
        if not files:
            return
        backup = os.path.join('backups', time.strftime('%Y-%m-%d_%H-%M-%S'))
        self.reporter.logger.info('Backup folder: %s' % backup)
        for (path, rel), report in self.reporter.task_it('Backing up files...', files):
            report(rel)
            dest = os.path.join(backup, rel)
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            shutil.copy(path, dest)
            self.reporter.logger.info('Saved: %s' % rel)

    def run(self):
        backup_paths = self.server.get_backup_paths(self.current)
        try:
            if backup_paths:
                self.create_backup(backup_paths)
            current = self.server.fetch(self.current, self.reporter)
            target = self.server.fetch(self.target, self.reporter)
            update.perform_update(self.root_path, current, target, server.get_root(self.target), self.reporter)
        except Exception as e:
            if not self.reporter.is_stopped(): # Interrupted from outside (on exit)
                self.reporter.logger.critical('Update failed', exc_info=True)
                self.reporter.stop()
                self.sig_exc.emit(e)

class ReporterThread(QThread):
    sig_task = pyqtSignal(str, str, int)
    sig_step = pyqtSignal(object)
    sig_done = pyqtSignal(str)

    def __init__(self, reporter):
        QThread.__init__(self)
        self.reporter = reporter

    def run(self):
        for task in self.reporter.tasks():
            self.sig_task.emit(task.title, task.unit, task.length or 0)
            for step in self.reporter.steps():
                self.sig_step.emit(step)
        self.sig_done.emit(self.reporter.elapsed())

class Updater(QDialog):
    def __init__(self, server, parent=None):
        super(Updater, self).__init__(parent)
        self.server = server
        self.reporter_thread = None
        self.update_thread = None
        self.step_unit = None
        self.progress = 0

        if getattr(sys, 'frozen', False):
            self.app_path = os.path.abspath(sys.executable)
        else:
            self.app_path = os.path.abspath(__file__)

        self.root_path = QLineEdit()
        self.root_path.textChanged[str].connect(self.on_path_changed)
        self.browse_button = QPushButton('Browse...')
        self.browse_button.clicked.connect(self.on_browse_button)
        self.progress_bar = QProgressBar()
        self.progress_bar.setValue(0)
        self.from_combo_box = QComboBox()
        self.from_combo_box.addItems(self.server.available_indexes())
        self.from_combo_box.setEnabled(False)
        self.autodetect_checkbox = QCheckBox('Autodetect')
        self.autodetect_checkbox.setChecked(True)
        self.autodetect_checkbox.toggled.connect(self.on_autodetect_checked)
        self.to_combo_box = QComboBox()
        self.to_combo_box.addItems(self.server.available_updates())
        self.update_button = QPushButton('Go!')
        self.update_button.setEnabled(False)
        self.update_button.clicked.connect(self.on_update_button)
        self.status_label = QLabel('Idle.')
        self.step_label = QLabel()

        # Update to the latest version by default
        pos = self.to_combo_box.findText(self.server.get_latest())
        if pos != -1:
            self.to_combo_box.setCurrentIndex(pos)

        self.win_taskbar = None
        if os.name == 'nt':
            from PyQt5.QtWinExtras import QWinTaskbarButton
            self.win_taskbar = QWinTaskbarButton(self)
            self.win_taskbar.progress().setVisible(True)

        bottom = QVBoxLayout()
        bottom.addWidget(self.status_label)
        bottom.addWidget(self.progress_bar)
        bottom.addWidget(self.step_label)

        layout = QGridLayout()
        layout.addWidget(QLabel('Root Path'), 0, 0)
        layout.addWidget(self.root_path, 0, 1)
        layout.addWidget(self.browse_button, 0, 2)
        layout.addWidget(QLabel('From'), 1, 0)
        layout.addWidget(self.from_combo_box, 1, 1)
        layout.addWidget(self.autodetect_checkbox, 1, 2)
        layout.addWidget(QLabel('To'), 2, 0)
        layout.addWidget(self.to_combo_box, 2, 1)
        layout.addWidget(self.update_button, 2, 2)
        layout.addLayout(bottom, 3, 0, 2, 0)

        self.setLayout(layout)
        self.setGeometry(100, 100, 350, 100)
        self.setWindowFlag(Qt.WindowContextHelpButtonHint, False)

    def set_task(self, task, unit, length):
        self.status_label.setText(task)
        self.progress_bar.setRange(0, length)
        self.progress_bar.setValue(0)
        if self.win_taskbar:
            self.win_taskbar.progress().setRange(0, length)
            self.win_taskbar.progress().setValue(0)
        self.step_unit = unit
        if not self.step_unit:
            self.step_label.setText('')

    def step(self, payload):
        self.progress_bar.setValue(self.progress_bar.value() + 1)
        if self.win_taskbar:
            self.win_taskbar.progress().setValue(self.progress_bar.value())
        if self.step_unit:
            self.step_label.setText('Current %s: %s' % (self.step_unit, payload))

    def set_done(self, elapsed):
        self.update_button.setEnabled(True)
        self.status_label.setText('Finished in %s' % elapsed)
        self.step_label.setText('')

    def update_failed(self, exception):
        QMessageBox.critical(self, 'Update failed', str(exception))

    def perform_autodetect(self):
        path = str(self.root_path.text())
        if self.autodetect_checkbox.isChecked() and self.server.get_anchor():
            try:
                hash = index.hash(os.path.join(path, self.server.get_anchor()['file']), 'sha1')
                pos = self.from_combo_box.findText(self.server.autodetect_anchor(hash))
                if pos != -1:
                    self.from_combo_box.setCurrentIndex(pos)
            except FileNotFoundError:
                pass

    def on_autodetect_checked(self):
        self.from_combo_box.setEnabled(not self.autodetect_checkbox.isChecked())
        self.perform_autodetect()

    def on_path_changed(self, path):
        self.update_button.setEnabled(bool(path))

    def on_browse_button(self):
        dialog = QFileDialog(self, 'Select root path...')
        dialog.setFileMode(QFileDialog.DirectoryOnly)
        if dialog.exec_() == QDialog.Accepted:
            folder = dialog.selectedFiles()[0]
            if self.server.get_anchor():
                for r, d, f in os.walk(folder, topdown=True):
                    if r.count(os.sep) - folder.count(os.sep) == 1:
                        del d[:]
                    if self.server.get_anchor()['file'] in f:
                        folder = os.path.normpath(r)
                        break
            self.root_path.setText(folder)
            self.perform_autodetect()

    def on_update_button(self):
        root_path = index.win_path(str(self.root_path.text()))
        if not os.path.isdir(root_path):
            QMessageBox.critical(self, 'Cannot proceed', 'Please make sure that the root path exists.')
            return
        if index.win_path(self.app_path).lower().startswith(root_path.lower()):
            QMessageBox.critical(self, 'Cannot proceed', 'Flashpoint Updater is found under the root path.\nPlease move it to a different location to proceed.')
            return
        self.update_button.setEnabled(False)
        current = str(self.from_combo_box.currentText())
        target = str(self.to_combo_box.currentText())
        logger.info('Starting update from %s to %s' % (current, target))
        reporter = ProgressReporter()
        self.reporter_thread = ReporterThread(reporter)
        self.reporter_thread.sig_task.connect(self.set_task)
        self.reporter_thread.sig_step.connect(self.step)
        self.reporter_thread.sig_done.connect(self.set_done)
        self.reporter_thread.start()
        self.update_thread = UpdateThread(reporter, root_path, self.server, current, target)
        self.update_thread.sig_exc.connect(self.update_failed)
        self.update_thread.start()

    def showEvent(self, event):
        self.setFixedSize(self.size()) # Make non-resizable
        if self.win_taskbar:
            self.win_taskbar.setWindow(updater.windowHandle())
        event.accept()

    def closeEvent(self, event):
        if self.update_thread and self.update_thread.isRunning():
            self.update_thread.reporter.stop()
            self.update_thread.wait()
            self.reporter_thread.wait()
        event.accept()

os.makedirs('logs', exist_ok=True)
logger = logging.getLogger('reporter')
logger.setLevel(logging.INFO)
logFormatter = logging.Formatter('%(asctime)s [%(levelname)s] %(message)s', '%Y-%m-%d %H:%M:%S')
fileHandler = logging.FileHandler(datetime.now().strftime('logs/update_%Y-%m-%d_%H-%M-%S.log'), delay=True)
fileHandler.setFormatter(logFormatter)
logger.addHandler(fileHandler)

if os.name == 'nt':
    ctypes.windll.shell32.SetCurrentProcessExplicitAppUserModelID('flashpoint.updater')

res_path = getattr(sys, '_MEIPASS', os.path.dirname(os.path.abspath(__file__)))

QApplication.setAttribute(Qt.AA_EnableHighDpiScaling, True)
app = QApplication([sys.argv])
app.setApplicationName('Flashpoint Updater')
app.setWindowIcon(QIcon(os.path.join(res_path, 'icon.png')))

try:
    with open('config.json', 'r') as f:
        config = json.load(f)
except FileNotFoundError:
    logger.critical('Could not find configuration file. Aborted.')
    QMessageBox.critical(None, 'Initialization error', 'Config file not found!')
    sys.exit(0)
except ValueError:
    logger.critical('Could not parse configuration file. Aborted.')
    QMessageBox.critical(None, 'Initialization error', 'Config file cannot be parsed!')
    sys.exit(0)

try:
    server = IndexServer(config['index_endpoint'])
except requests.exceptions.RequestException as e:
    logger.critical('Could not retrieve index metadata: %s' % str(e))
    QMessageBox.critical(None, 'Initialization error', 'Could not retrieve index metadata. Please, check the log file for more details.')
    sys.exit(0)
except ValueError:
    logger.critical('Could not parse index metadata. Aborted.')
    QMessageBox.critical(None, 'Initialization error', 'Index metadata cannot be parsed! Please check your configuration file or contact a server administrator.')
    sys.exit(0)

if not server.available_updates():
    QMessageBox.critical(None, 'No updates available', '\n'.join([
        'No root specified for any index by the remote server.',
        'Please check your configuration file or contact a server administrator.'
    ]))
    sys.exit(0)

updater = Updater(server)
updater.show()
app.exec_()
