# Tasker Design

This is a high level design document for a Linux job framework called Tasker.

#### Content

- [Description](#description)
- [Dependencies](#dependencies)
    - [Libraries](#libraries)
    - [Tools](#tools)
- [Authentication](#authentication)
    - [TLS Configuration](#tls-configuration)
    - [TLS CA and Cert Generation](#tls-ca-and-cert-generation)
- [Jobs](#jobs)
    - [Lifecycle](#lifecycle)
    - [Authorization](#authorization)
- [Foreman](#foreman)
    - [Button Control](#button-control)
    - [Emulated Terminal](#emulated-terminal)
    - [Flow](#flow)
- [Crew](#crew)

## Description

Tasker will be designed to be a many clients to many servers architecture. Going forward in this doc:

- framework -> **Tasker**
- client -> **Foreman**
- server -> **Crew**

## Dependencies

Tasker will use the current (at this time) Go revision [v1.25.7](https://go.dev/dl/). These are the planned dependencies broken down into direct Go libraries and tools:

### Libraries

| Lib         | Description               | Repo                                                          |
| ----------- | ------------------------- | ------------------------------------------------------------- |
| grpc-go     | gRPC implementation       | [github link](https://github.com/grpc/grpc-go)                |
| protobuf-go | protobuf implementation   | [github link](https://github.com/protocolbuffers/protobuf-go) |
| tview       | TUI toolkit (wraps tcell) | [github link](https://github.com/rivo/tview)                  |
| tcell/v2    | TUI library               | [github link](https://github.com/gdamore/tcell/tree/v2)       |

### Tools

| Tool | Description                           | Install                                                         |
| ---- | ------------------------------------- | --------------------------------------------------------------- |
| Task | Optional task runner for dev commands | [taskfile.dev/installation](https://taskfile.dev/installation/) |

## Authentication

Mutual TLS will be used to authenticate Foreman and Crew interactions.

### TLS Configuration

- **Version:** v1.3
- **Ciphers:** AES-128-GCM, AES-256-GCM, ChaCha20-Poly1305 (selected by Go TLS)
- **Keys:** ECDSA P-256

### TLS CA and Cert Generation

There will be an internal tool provided to generate a Tasker CA and Foreman/Crew certs. For simplicity, all of these will be written to/read from `/etc/tasker`.

The cert generator will add in a CN, O, and OU:

- **CN:** Foreman user name or Crew name
- **O:** Tasker
- **OU:** user, admin or crew

## Jobs

Jobs are just Linux commands wrapped in a cgroup (v2) and owned by a user. There will also be authorization per job.

### Lifecycle

- **Creation:** user will be able to add cpu/memory/io limits to the cgroup.
- **Output:** combining the stdout and stderr into a pipe. output history will also be stored to allow for recalling.
- **Input:** stdin pipe
- **End:** user will send a stop which will trigger a sigterm to all processes under the cgroup. this will be followed up by a kill if needed.

### Authorization

Each job will be owned by a user (extracted from the cert CN). This user must be an enabled user on the linux machine(s) the Foreman and Crew are running on. This is because the certs will be chown'd to the user and the job will be assigned to the user.

After a job is started it can be managed (stop, archive, and attach). All users will be able to see every job. There will be two avenues for managing jobs based on user roles (extracted from the cert OU).

- **user:** can only manage jobs they started.
- **admin:** can manage any job.

## Foreman

The Foreman is how you will interact with a Crew(s). It will abstract the underlying rpc calls into buttons and a simulated terminal. Any output from the job will be displayed in the terminal.

### Button Control

```
job-1 [stop] [attach]
job-2 [stop] [attach]
```

### Emulated Terminal

```
:attach job-1
<output>
...
:stop job-1
```

### Flow

- **Boot:** repeat attempts to open a `WatchJobs` stream to the Crew(s)
- **Jobs:** once a `WatchJobs` stream is established the jobs are displayed for that Crew
- **Start:** send a `StartJob` unary
- **Attach:** start a `AttachJob` stream to view output (with recall) and allow input
- **Detach:** close `AttachJob` stream
- **Stop:** send a `StopJob` unary
- **Archive:** send a `ArchiveJob` unary to move a job to the Crew's archive (archived jobs will not be sent in the `WatchJobs` stream)
- **End:** detach from all streams and cleanup. started jobs still keep running.

## Crew

The Crew is in charge of keeping track of jobs. For simplicity, each run will not persist jobs or data to the next run. We could add on a store for (sqlite, etc).
