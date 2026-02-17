# Accept input and flip the chars
#
# Prints are flushed because this can be run where stdout is not a tty (piped).
while True:
    print("> ", end="", flush=True)

    line = input()
    if not line or line.strip() == "exit":
        break

    text = line.strip()
    print(text + " -> " + text[::-1], flush=True)
