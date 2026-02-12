# Tasker Design

This is a high level design document for a Linux job service called Tasker.

#### Content

- [Dependencies](#dependencies)
    - [Libraries](#libraries)
    - [Tools](#tools)
- [Authentication](#authentication)
    - [TLS Configuration](#tls-configuration)
    - [TLS CA and Certs](#tls-ca-and-certs)
        - [Certs Directory](#certs-directory)
- [Jobs](#jobs)
    - [Resource Limits](#resource-limits)
        - [CPU](#cpu)
        - [Memory](#memory)
        - [IO](#io)
        - [PIDS](#pids)
    - [Creation](#creation)
    - [Authorization](#authorization)
    - [Output](#output)
    - [Cleanup](#cleanup)
- [Taskerctl](#taskerctl)
    - [Usage](#usage)
    - [Cert](#cert)
    - [Job](#job)
        - [Attach](#attach)
        - [Get](#get)
        - [Start](#start)
        - [Stop](#stop)
    - [Server](#server)

## Dependencies

Tasker will use the current (at this time) Go revision [v1.25.7](https://go.dev/dl/). These are the planned dependencies broken down into direct Go libraries and tools:

### Libraries

| Lib         | Description             | Repo                                                          |
| ----------- | ----------------------- | ------------------------------------------------------------- |
| sys         | syscall replacement     | [Google Git link](https://go.googlesource.com/sys)            |
| grpc-go     | gRPC implementation     | [GitHub link](https://github.com/grpc/grpc-go)                |
| protobuf-go | protobuf implementation | [GitHub link](https://github.com/protocolbuffers/protobuf-go) |
| uuid        | uuid generation         | [GitHub link](https://github.com/google/uuid)                 |
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

### TLS CA and Certs

There will be sample CA, client and server certs provided in the `certs` directory on the repo. By default [Taskerctl](#taskerctl) uses this directory to look for certs.

Along with that, there will be a tool to generate Tasker CA and client/server certs: [Taskerctl Cert](#cert).

A Tasker Client cert needs to contain CN, O, and OU:

- **CN:** user name
- **O:** `Tasker`
- **OU:** `user` or `admin`

A Tasker Server cert needs to contain CN, O, and OU:

- **CN:** server name
- **O:** `Tasker`
- **OU:** `server`

#### Certs Directory

By default `certs` is the directory where certs are looked for.

- The CA will be looked for at `certs/ca.<crt|key>`.
- When a user does `taskerctl job stop -u wolf...` this will look for `certs/client/wolf.<crt|key>`.
- When a user does `taskerctl server -n wolfpack1...` this will look for `certs/server/wolfpack1.<crt|key>`.

Optionally you can change the certs directory when using [Taskerctl](#taskerctl).

## Jobs

Jobs are just Linux commands wrapped in a cgroup (v2) and owned by a user. There will also be authorization per job.

For simplicity, each server run will not persist jobs or data to the next run.

### Resource Limits

A user can add cpu, memory, and/or io limits to the cgroup in their [Start](#start) command. If a limit is not provided then the cgroup defaults to max for that resource type. There also will be a hard limit of 1000 concurrent processes for a job.

`cpu`, `memory`, `io`, `pids` controllers will be enabled on the cgroups root and Tasker subtree.

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

#### PIDS

```
pids.max = 1000
```

### Creation

All jobs will be put under a cgroup. Each subprocess of the main process will be assigned the same process group id. Each job will be assigned a uuid for their id. Creating a job under a cgroup looks similar to this:

```
j := Job{...}
cgFD := createCgroup(j.id, j.limits)

j.cmd = exec.Command(j.command, j.args...)
j.cmd.Stdout = j.output
j.cmd.Stderr = j.output
j.cmd.SysProcAttr = &unix.SysProcAttr{
	Setpgid:     true,
	UseCgroupFD: true,
	CgroupFD:    cgFD,
}

j.cmd.Start()
```

### Authorization

Each job will be owned by a user (extracted from the cert CN).

After a job is started it can be managed (stop, get, and attach). There will be two avenues for managing jobs based on user roles (extracted from the cert OU).

- **user:** can only manage jobs they started.
- **admin:** can manage any job.

### Output

Job output is a combination of the stdout and stderr and will be stored as raw bytes.

NOTE: there is no cap on the output buffer size.

When an [Attach](#attach) command is initialized, the job will return a `io.Reader` that will start reading at the beginning of the job's output. The reader will keep track of its offset in the job's output so data will never be lost. This is a simplified flow of how it will work:

```
// Job writes output to a byte buffer and notifies readers via a channel.
outputBuffer.Write(data):
    add data to buf
    close(notifyChan)
    notifyChan = make(chan)

outputBuffer.Close():
    closed = true
    close(notifyChan)

// Each client gets a reader with its own offset.
AttachJob(stream):
    reader = job.NewReader(stream.Context())
    // 4KB chunks
    buf := make([]byte, 4096)
    for {
        count, err = reader.Read(buf)
        if count > 0: stream.Send(buf[:count])
        if err: return
    }

outputReader.Read(buf):
    for {
        // new data available
        if offset < len(outputBuffer.buf):
            copy into buf, advance offset
            return count, nil

        if ob.closed:
            return 0, EOF

        select {
        case <-notifyChan:
            // notified so loop back and find out why
        case <-ctx.Done():
            // client disconnected
            return 0, ctx.Err()
        }
    }
```

### Cleanup

When a [Stop](#stop) command is triggered the process group receives a SIGTERM followed up by a cgroup kill.

On server shutdown, all running jobs go through the same SIGTERM then cgroup kill flow in parallel.

## Taskerctl

Taskerctl will provide commands to generate Tasker certs, manage jobs and start a Tasker server.

### Usage

```
taskerctl manages the Tasker job service

Usage:
  taskerctl [command]

Available Commands:
  cert        Manage TLS certificates
  help        Help about any command
  job         Manage jobs
  server      Start the Tasker server

Flags:
  -C, --certs-dir string   Certificate directory (default "certs")
  -h, --help               help for taskerctl

Use "taskerctl [command] --help" for more information about a command.
```

### Cert

```
Manage TLS certificates

Usage:
  taskerctl cert [command]

Available Commands:
  ca          Generate a new CA
  client      Generate a client certificate
  server      Generate a server certificate

Flags:
  -h, --help   help for cert

Global Flags:
  -C, --certs-dir string   Certificate directory (default "certs")

Use "taskerctl cert [command] --help" for more information about a command.
```

### Job

```
Manage jobs

Usage:
  taskerctl job [command]

Available Commands:
  attach      Attach to a job's output
  get         Get a job's status
  start       Start a new job
  stop        Stop a running job

Flags:
  -a, --addr string   Server address (e.g. localhost:50051)
  -h, --help          help for job
  -u, --user string   User name

Global Flags:
  -C, --certs-dir string   Certificate directory (default "certs")

Use "taskerctl job [command] --help" for more information about a command.
```

#### Attach

```
Attach to a job's output

Usage:
  taskerctl job attach <id> [flags]

Flags:
  -h, --help   help for attach

Global Flags:
  -a, --addr string        Server address (e.g. localhost:50051)
  -C, --certs-dir string   Certificate directory (default "certs")
  -u, --user string        User name
```

Example:

```
$ taskerctl job attach -u wolf -a localhost:50051 a1b2c3d4-e5f6-7890-abcd-ef1234567890
<data stream>
```

#### Get

```
Get a job's status

Usage:
  taskerctl job get <id> [flags]

Flags:
  -h, --help   help for get

Global Flags:
  -a, --addr string        Server address (e.g. localhost:50051)
  -C, --certs-dir string   Certificate directory (default "certs")
  -u, --user string        User name
```

Example:

```
$ taskerctl job get -u wolf -a localhost:50051 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: completed
```

#### Start

```
Start a new job

Usage:
  taskerctl job start [flags] <command> [args...]

Flags:
  -c, --cpu float32     CPU limit in cores (e.g. 0.5)
  -d, --device string   Block device for IO limits (e.g. /dev/sda)
  -h, --help            help for start
  -m, --memory uint32   Memory limit in MB (e.g. 512)
  -r, --read uint32     IO read limit in MB/s (requires -d)
  -w, --write uint32    IO write limit in MB/s (requires -d)

Global Flags:
  -a, --addr string        Server address (e.g. localhost:50051)
  -C, --certs-dir string   Certificate directory (default "certs")
  -u, --user string        User name
```

Example:

```
$ taskerctl job start -u wolf -a localhost:50051 /usr/bin/sleep 60
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: running
```

With resource limits:

```
$ taskerctl job start -u wolf -a localhost:50051 -c 0.5 -m 512 -d /dev/sda -r 100 -w 50 /usr/bin/my-app
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

#### Stop

Stopping a stopped/completed job is idempotent (it will return the job details but no error).

```
Stop a running job

Usage:
  taskerctl job stop <id> [flags]

Flags:
  -h, --help   help for stop

Global Flags:
  -a, --addr string        Server address (e.g. localhost:50051)
  -C, --certs-dir string   Certificate directory (default "certs")
  -u, --user string        User name
```

Example:

```
$ taskerctl job stop -u wolf -a localhost:50051 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
id: 3f8a1b2c-9d4e-4f5a-b6c7-8d9e0f1a2b3c
owner: wolf
command: /usr/bin/sleep
args: [60]
phase: stopped
```

### Server

```
Start the Tasker server

Usage:
  taskerctl server [flags]

Flags:
  -a, --addr string   Listen address (e.g. :8080) (default ":50051")
  -h, --help          help for server
  -n, --name string   Server name (cert name) (default "wolfpack1")

Global Flags:
  -C, --certs-dir string   Certificate directory (default "certs")
```
