#!/usr/bin/env python3
"""Preserve a failing check's exit code and expose its diagnostic tail in Actions annotations."""
import collections
import subprocess
import sys

title, *command = sys.argv[1:]
tail = collections.deque(maxlen=60)
with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True) as process:
    for line in process.stdout:
        print(line, end="", flush=True)
        tail.append(line)
    result = process.wait()
if result:
    message = "".join(tail)[-10000:]
    message = message.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    print(f"::error title={title}::{message}", flush=True)
sys.exit(result)
