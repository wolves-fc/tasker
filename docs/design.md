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
    - [Lifecycle](#lifecycle)
        - [Creation](#creation)
        - [Output](#output)
        - [End](#end)
    - [Authorization](#authorization)
- [CLI](#cli)

## Dependencies

Tasker will use the current (at this time) Go revision [v1.25.7](https://go.dev/dl/). These are the planned dependencies broken down into direct Go libraries and tools:

### Libraries

| Lib         | Description             | Repo                                                          |
| ----------- | ----------------------- | ------------------------------------------------------------- |
| grpc-go     | gRPC implementation     | [github link](https://github.com/grpc/grpc-go)                |
| protobuf-go | protobuf implementation | [github link](https://github.com/protocolbuffers/protobuf-go) |

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

### Lifecycle

#### Creation

All jobs will be put under a cgroup. A user can also add cpu/memory/io limits to the cgroup in their `StartJob` request.

Each subprocess of the main process will be assigned the same process group id.

#### Output

Job output is a combination of the stdout and stderr to a pipe and will be stored as raw bytes. There will be an async read on the process that will store the read data into the job's output buffer and then attempt to write the read data to a channel.

When an `AttachJob` stream is initialized, the job first sends its output buffer to catch the client up (recalling).

After that, the attach is spawned into an async process that will listen to the job output channel and send anything that comes across it to the `AttachJob` stream. There can be many attach processes that listen on the same job output channel.

#### End

When a stop is triggered, a sigterm will be sent to the process group. This will be followed up by a cgroup kill if needed.

### Authorization

Each job will be owned by a user (extracted from the cert CN).

After a job is started it can be managed (stop, archive, and attach). All users will be able to see every job. There will be two avenues for managing jobs based on user roles (extracted from the cert OU).

- **user:** can only manage jobs they started.
- **admin:** can manage any job.

## CLI

The client cli will provide these options that abstract the underlying rpc calls:

- **start:** send a `StartJob` unary
- **stop:** send a `StopJob` unary
- **attach:** start an `AttachJob` stream to view output (with recall)
- **detach:** close an `AttachJob` stream
- **get job:** send a `GetJob` unary
