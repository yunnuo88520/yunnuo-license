"""Regression: terminate descendants holding stdout open after their parent exits."""
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest

WRAPPER = pathlib.Path(__file__).with_name('check-command.py')


class CheckCommandTest(unittest.TestCase):
    def invoke(self, code, *options):
        return subprocess.run([sys.executable, str(WRAPPER), *options, 'fixture', sys.executable, '-c', code],
                              text=True, capture_output=True, timeout=8)

    def test_success_and_original_failure(self):
        success = self.invoke("print('passed')")
        self.assertEqual(success.returncode, 0)
        failure = self.invoke("print('specific failure'); raise SystemExit(7)")
        self.assertEqual(failure.returncode, 7)
        self.assertIn('specific failure', failure.stdout)
        self.assertIn('::error title=fixture', failure.stdout)

    def test_child_ignoring_term_cannot_hold_stdout_open(self):
        with tempfile.TemporaryDirectory() as directory:
            pidfile = pathlib.Path(directory) / 'child.pid'
            child = "import os,signal,time,pathlib; signal.signal(signal.SIGTERM,signal.SIG_IGN); pathlib.Path(" + repr(str(pidfile)) + ").write_text(str(os.getpid())); time.sleep(60)"
            parent = "import subprocess,sys,time,pathlib; subprocess.Popen([sys.executable,'-c'," + repr(child) + "]);\nwhile not pathlib.Path(" + repr(str(pidfile)) + ").exists(): time.sleep(.01)\n"
            started = time.monotonic()
            result = self.invoke(parent, '--timeout', '0.5', '--kill-after', '0.2')
            self.assertEqual(result.returncode, 124, result.stdout + result.stderr)
            self.assertLess(time.monotonic() - started, 4)
            pid = int(pidfile.read_text())
            stat = pathlib.Path('/proc') / str(pid) / 'stat'
            if stat.exists():
                self.assertEqual(stat.read_text().split(') ', 1)[1].split()[0], 'Z')

    def test_retry_retains_log(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = pathlib.Path(directory) / 'marker'
            log = pathlib.Path(directory) / 'setup.log'
            code = "import pathlib; p=pathlib.Path(" + repr(str(marker)) + "); previous=p.exists(); p.touch(); print('retried' if previous else 'temporary failure'); raise SystemExit(0 if previous else 1)"
            result = self.invoke(code, '--attempts', '2', '--log', str(log))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('temporary failure', log.read_text())
            self.assertIn('retried', log.read_text())


if __name__ == '__main__':
    unittest.main()
