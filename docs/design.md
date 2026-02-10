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
- [Taskerctl](#taskerctl)
    - [Usage](#usage)
    - [Start](#start)
    - [Stop](#stop)
    - [Get](#get)
    - [Attach](#attach)
    - [Cert](#cert)

## Dependencies

Tasker will use the current (at this time) Go revision [v1.25.7](https://go.dev/dl/). These are the planned dependencies broken down into direct Go libraries and tools:

### Libraries

| Lib         | Description             | Repo                                                          |
| ----------- | ----------------------- | ------------------------------------------------------------- |
| grpc-go     | gRPC implementation     | [GitHub link](https://github.com/grpc/grpc-go)                |
| protobuf-go | protobuf implementation | [GitHub link](https://github.com/protocolbuffers/protobuf-go) |
| uuid        | uuid generation         | [GitHub link](github.com/google/uuid)                         |
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

Each job will be assigned a uuid for their id.

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
    sync.Cond.broadcast

attach:
    defer sync.Cond.activate
    watcher loop

watcher loop:
    chunk, ok = watcher.nextChunk(ctx)
    if !ok then return (job exited or client disconnected)
    stream.send(chunk)

nextChunk:
    while pos >= job.output.len
        if cancelled or job done return nil, false
        sync.Cond.wait

    chunk = job.output[pos]
    pos++
    return chunk, true
```

### Cleanup

When a stop is triggered, a cgroup kill will happen.

## Taskerctl

Taskerctl will provide commands to generate Tasker certs and manage jobs.

### Usage

```
CLI client for Tasker job service

Usage:
  taskerctl [command]

Available Commands:
  attach      Attach to a job's output
  cert        Manage TLS certificates
  get         Get a job's status
  help        Help about any command
  start       Start a new job
  stop        Stop a running job

Flags:
  -h, --help   help for taskerctl

Use "taskerctl [command] --help" for more information about a command.
```

### Start

Start a new job on the server. Resource limits are optional.

```
$ taskerctl start -u wolf -a localhost:50051 /usr/bin/sleep 60
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: running
```

With resource limits:

```
$ taskerctl start -u wolf -a localhost:50051 -c 0.5 -m 512 -d /dev/sda -r 100 -w 50 /usr/bin/my-app
id: a1b2c3d4-e5f6-7890-abcd-ef1234567890
owner: wolf
command: /usr/bin/my-app
args: []
phase: running
cpu limit: 0.50 cores
memory limit: 512 MB
io device: /dev/sda
io read limit: 100 MB/s
io write limit: 50 MB/s
```

### Stop

Stop a running job. Stopping a stopped/completed job is idempotent (it will return the job details but no error).

```
$ taskerctl stop -u wolf -a localhost:50051 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: stopped
```

### Get

Get a job's current status.

```
$ taskerctl get -u wolf -a localhost:50051 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: completed
```

### Attach

Attach to a job's output stream. Streams stdout and stderr (with recall of past data).

```
$ taskerctl attach -u wolf -a localhost:50051 a1b2c3d4-e5f6-7890-abcd-ef1234567890
<data stream>
```

### Cert

Generate TLS certificates for Tasker.

#### Cert CA

```
taskerctl cert ca
```

#### Cert Client

```
# admin role
taskerctl cert client -u wolf -r admin
# user role
taskerctl cert client -u wolfjr -r user
```

#### Cert Server

```
taskerctl cert server -n tasker1 -H localhost,192.168.1.1
```
