# Incrementing counter every second.
#
# Prints are flushed because this can be run where stdout is not a tty (piped).
import itertools
import time

for i in itertools.count():
    print(i, flush=True)
    time.sleep(1)
