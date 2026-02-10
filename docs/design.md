# Tasker Design

This is a high level design document for a Linux job service called Tasker.

#### Content

- [Dependencies](#dependencies)
    - [Libraries](#libraries)
    - [Tools](#tools)
- [Authentication](#authentication)
    - [TLS Configuration](#tls-configuration)
    - [TLS CA and Cert Generation](#tls-ca-and-cert-generation)
- [Jobs](#jobs)
    - [Resource Limits](#resource-limits)
        - [CPU](#cpu)
        - [Memory](#memory)
        - [IO](#io)
    - [Creation](#creation)
    - [Authorization](#authorization)
    - [Output](#output)
    - [Cleanup](#cleanup)
- [CLI](#cli)

## Dependencies

Tasker will use the current (at this time) Go revision [v1.25.7](https://go.dev/dl/). These are the planned dependencies broken down into direct Go libraries and tools:

### Libraries

| Lib         | Description             | Repo                                                          |
| ----------- | ----------------------- | ------------------------------------------------------------- |
| grpc-go     | gRPC implementation     | [GitHub link](https://github.com/grpc/grpc-go)                |
| protobuf-go | protobuf implementation | [GitHub link](https://github.com/protocolbuffers/protobuf-go) |
| cobra       | cli flag toolkit        | [GitHub link](https://github.com/spf13/cobra)                 |

### Tools

| Tool | Description                           | Install                                                         |
| ---- | ------------------------------------- | --------------------------------------------------------------- |
| Task | Optional task runner for dev commands | [taskfile.dev/installation](https://taskfile.dev/installation/) |

## Authentication

Mutual TLS will be used to authenticate client and server interactions.

### TLS Configuration

- **Version:** v1.3
- **Ciphers:** AES-128-GCM, AES-256-GCM, ChaCha20-Poly1305 (selected by Go TLS)
- **Keys:** ECDSA P-256

### TLS CA and Cert Generation

There will be an internal tool provided to generate a Tasker CA and client/server certs. For simplicity, all of these will be written to/read from `/etc/tasker`.

The cert generator will add in a CN, O, and OU for a client:

- **CN:** user name
- **O:** Tasker
- **OU:** user or admin

## Jobs

Jobs are just Linux commands wrapped in a cgroup (v2) and owned by a user. There will also be authorization per job.

For simplicity, each server run will not persist jobs or data to the next run.

### Resource Limits

A user can add cpu/memory/io limits to the cgroup in their `StartJob` request. If a limit is not provided then the cgroup defaults to max for that resource type.

#### CPU

```
cores = user specified float
cpu period = 100k usec
cpu quota = <cores> * <period>
cpu.max = <quota> <period>
```

#### Memory

```
memory = user specified int in MB
// convert to MB -> bytes
memory.max = <memory> * 1024 * 1024
```

#### IO

```
device = user provided device
read = user specified int in MB/s
write = user specified int in MB/s
// convert to MB -> bytes
rbps = <read> * 1024 * 1024
wbps = <write> * 1024 * 1024
io.max = <device> rbps=<rbps> wbps=<wbps>
```

### Creation

All jobs will be put under a cgroup. Each subprocess of the main process will be assigned the same process group id. Here's some pseudocode of creating a process in a cgroup:

```
// cg := create c group (optional limits)
...
// cgFD := open fd to c group directory
...
cmd = exec.Command(command, args...)
// set pgid (allows sigterm to whole group id)
// set c group fd so the process gets added to the c group
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid:     true,
	UseCgroupFD: true,
	CgroupFD:    cgFD,
}
// handle piping stdout and such
...
// start the command
cmd.Start()
```

Each job will be assigned a 16 byte uuid in a `4-2-2-2-6` bytes format e.g. `a1b2c3d4-e5f6-7890-abcd-ef1234567890`

### Authorization

Each job will be owned by a user (extracted from the cert CN).

After a job is started it can be managed (stop, get, and attach). There will be two avenues for managing jobs based on user roles (extracted from the cert OU).

- **user:** can only manage jobs they started.
- **admin:** can manage any job.

### Output

Job output is a combination of the stdout and stderr to a pipe and will be stored as raw bytes. There will be an async read on the process that will store the read data into the job's output buffer.

NOTE: there is no cap on the output buffer size.

When an `AttachJob` stream is initialized, the job will add a watcher for that client stream. The watcher will keep track of where it has read on the job's output buffer. The watcher will have a `watcher.nextChunk()` method that it waits on. Here is some pseudocode of the watcher:

```
job:
    append chunk to output
    broadcast activate to all watchers

attach:
    defer cancel watchers (activate)
    watcher loop

watcher loop:
    chunk, ok = watcher.nextChunk(ctx)
    if !ok then return (job exited or client disconnected)
    stream.send(line)

nextChunk:
    while pos >= job.output.len
        if cancelled or job done return nil, false
        wait for watcher activate

    line = job.output[pos]
    pos++
    return line, true
```

### Cleanup

When a stop is triggered, a sigterm will be sent to the process group. This will be followed up by a cgroup kill if needed.

## CLI

The client CLI will provide these commands that abstract the underlying RPC calls:

- **start:** send a `StartJob` unary
- **stop:** send a `StopJob` unary
- **get:** send a `GetJob` unary
- **attach:** start an `AttachJob` stream to view output (with recall)
