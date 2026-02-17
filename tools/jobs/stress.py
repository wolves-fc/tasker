import os
import tempfile
import threading

# Stress CPU, memory, and IO concurrently.
#
# Prints are flushed because this can be run where stdout is not a tty (piped).

def cpu():
    print("stressing cpu", flush=True)
    while True:
        pass

def memory():
    print("stressing memory", flush=True)
    data = []
    while True:
        data.append(b"x" * 1024 * 1024)

def io():
    print("stressing io", flush=True)
    buf = b"x" * 1024 * 1024
    while True:
        fd, path = tempfile.mkstemp()
        try:
            while True:
                os.write(fd, buf)
        except OSError:
            pass
        finally:
            os.close(fd)
            os.unlink(path)

for target in [cpu, memory, io]:
    threading.Thread(target=target, daemon=True).start()

# Block forever
threading.Event().wait()
