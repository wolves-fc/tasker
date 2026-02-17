# Tasker

Remote job execution service built on gRPC.

- cgroup v2 isolation with configurable CPU, memory, and IO limits
- Mutual TLS authentication with role-based access

See [docs/design.md](docs/design.md) for more details.

## Requirements

- Linux with cgroup v2
- Go 1.26+
- [Task](https://taskfile.dev) (task runner)

## Build

```
task build
```

Binaries go to `bin/`.

## TLS

All `taskerctl` commands use `-C` to set the certificates directory (default: `certs/`).

```
# Create a CA
taskerctl cert ca

# Create a server cert
taskerctl cert server -n wolfpack1 -H localhost,127.0.0.1

# Create client certs
taskerctl cert client -u wolf -r admin
```

## Usage

Start the server:

```
taskerctl server -n wolfpack1 -a :50051
```

Start a job:

```
taskerctl job -u wolf -a localhost:50051 start -- sleep 60
```

With resource limits:

```
taskerctl job -u wolf -a localhost:50051 start -c 0.5 -m 512 -d /dev/sda -r 100 -w 50 -- python3 ./tools/jobs/counter.py
```

Check on it:

```
taskerctl job -u wolf -a localhost:50051 get <id>
```

Attach to its output:

```
taskerctl job -u wolf -a localhost:50051 attach <id>
```

Stop it:

```
taskerctl job -u wolf -a localhost:50051 stop <id>
```
