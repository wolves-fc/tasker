import os

# Fork bomb for testing cgroup process containment.
#
# Prints are flushed because this can be run where stdout is not a tty (piped).
while True:
    pid = os.fork()
    if pid == 0:
        print(f"forked (pid={os.getpid()})", flush=True)
