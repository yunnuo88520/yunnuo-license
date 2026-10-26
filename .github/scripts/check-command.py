#!/usr/bin/env python3
"""Stream diagnostics and bound the lifetime of a command and its child processes."""
import argparse
import collections
import contextlib
import os
import pathlib
import signal
import subprocess
import sys
import threading
import time


def run(command, timeout, grace, tail, log):
    expired = threading.Event()
    finished = threading.Event()
    with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                          text=True, encoding='utf-8', errors='replace', start_new_session=True) as process:
        def kill_group(sig):
            try:
                os.killpg(process.pid, sig)
            except ProcessLookupError:
                pass

        def expire():
            if finished.is_set():
                return
            expired.set()
            print(f'Time limit {timeout}s reached; terminating the command process group.', flush=True)
            kill_group(signal.SIGTERM)
            if not finished.wait(grace):
                kill_group(signal.SIGKILL)

        timer = threading.Timer(timeout, expire) if timeout else None
        if timer:
            timer.daemon = True
            timer.start()
        try:
            # The deadline remains active while draining stdout: a child can
            # keep this pipe open even after the original process exits.
            for line in process.stdout:
                print(line, end='', flush=True)
                tail.append(line)
                if log:
                    log.write(line)
                    log.flush()
            result = process.wait()
        finally:
            finished.set()
            if timer:
                timer.cancel()
            if expired.is_set() or process.poll() is None:
                kill_group(signal.SIGKILL)
                process.wait()
        if expired.is_set():
            tail.append(f'Time limit {timeout}s exceeded; command and descendants were stopped.\n')
            return 124
        return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--timeout', type=float)
    parser.add_argument('--kill-after', type=float, default=10)
    parser.add_argument('--attempts', type=int, default=1)
    parser.add_argument('--log', type=pathlib.Path)
    parser.add_argument('title')
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if not args.command or args.attempts < 1 or args.kill_after <= 0 or (args.timeout is not None and args.timeout <= 0):
        parser.error('provide a command and positive timeout/attempt values')
    if args.log:
        args.log.parent.mkdir(parents=True, exist_ok=True)
    with (args.log.open('w', encoding='utf-8') if args.log else contextlib.nullcontext(None)) as log:
        for attempt in range(1, args.attempts + 1):
            tail = collections.deque(maxlen=60)
            result = run(args.command, args.timeout, args.kill_after, tail, log)
            if result == 0:
                return 0
            if attempt < args.attempts:
                print(f'{args.title}: attempt {attempt} failed with exit {result}; retrying.', flush=True)
                time.sleep(2)
    # Keep the final error visible in GitHub's truncated annotation view.
    message = ''.join(tail)[-3000:]
    message = message.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
    print(f'::error title={args.title}::{message}', flush=True)
    return result


if __name__ == '__main__':
    sys.exit(main())
