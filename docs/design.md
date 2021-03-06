---
authors: sterling
state: draft
---

# Job Service RFD

## What

Implement a grpc service that provides the following features:
* Start a process
	* Provide resource control (CPU, Mem, Disk IO) via Cgroups
* Stop a running process
* Get the status of a process
* Stream the output of a running process
	* Concurrent clients should be supported
	* Clients that connect stream from the beginning of the output. (potential issue - see below)

Write a CLI tool to interact with the service above

## Details

### APIs

#### client <-> service

CLI client will interact with the service via GRPC

Below is a proposal for the GRPC service and associated message structs

```proto
message Job {
	int32 id = 1;
	repeated string cmd = 2;
	string status = 3;
}

message GetRequest {
	int32 id = 1;
}

message StartRequest {
	repeated string cmd = 1;
	int32 num_cpu = 2;
	int64 max_disk_io = 3;
	int32 max_mem_use = 4;
}

message StopRequest {
	int32 id = 1;
}

message StopResponse {
	int32 code = 1;
	string message = 2;
}

message StreamRequest {
	int32 id = 1;
}

message StreamResponse {
	bytes stream = 1;
}

service JobService {
	rpc Get(GetRequest) returns (Job);
	rpc Start(StartRequest) returns(Job);
	rpc Stop(StopRequest) returns(StopResponse);
	rpc Stream(StreamRequest) returns(stream StreamResponse);
}
```

Error information will be transmitted using grpc codes and error message.

#### service <-> lib
Initial design of the job lib api is represented in the interface below

```go
type Job struct {
	ID int32
	Cmd *exec.Cmd
	Status string
}

type JobService interface {
		Start(cmd []string, limits ResourceLimit) (*Job, error)
		Get(jobID int32) (*Job, error)
		Stop(jobID int32) error
		Stream(jobID int32, writer io.Writer) error
}

// different command statuses
var (
	StatusRunning 	= "running" 	// process is currently running
	StatusStopped 	= "stopped" 	// process was stopped via the Stop command
	StatusFinished 	= "finished" 	// process exit normally
)
```

State Transitions:

`running` can be transitioned to `stop` and `finished`.  
`stopped` and `finished` are terminal states, they cannot change after.

We will store the jobs in an in-memory map data structure like below

```go
type Store struct {
	data map[jobID]*Job
}

```

#### lib <-> linux kernel

Cgroup requires interaction between the lib and the linux kernel. Below is a proposal for the API between lib and the OS

```go
type ResourceLimit struct {
	CPU int
	Mem int
	IO int
}

// Constructor for Cgroup
func NewCgroup(name string, limit ResourceLimit) Cgroup {
	// return Cgroup
}

type Cgroup interface {
	AddProcess(pid int) error
	Close() error
}
```

### Process Execution Lifecycle

For both starting and stopping a process, this library will use Go's std lib package `os/exec` to interact with
commands.

#### Starting a process

The steps to start a process and it's Cgroup are:

1. Request is received via GRPC server
2. userID is parsed from the Subject attribute in x509 cert.
3. Check if the User has admin role
4. Get new job ID
5. Create a new Cgroup hiearchy using the job ID.
6. Add the parent (main) process's PID to the `cgroup.procs` file.
7. Create the process using `Cmd.Start()`
8. Move the parent process's PID back to the root Cgroup
9. Use `cmd.Wait()` in a separate goroutine to ensure long-running processes don't block the grpc request.

#### Stopping a process

The steps to stop a process and it's Cgroups are:

1. Request is received via GRPC server
2. userID is parsed from the Subject attribute in x509 cert.
3. Check if the User has admin role
4. Check if the jobID exists -> exit and returns grpc error code NotFound if jobID is not found
5. Run os.Process.Kill() to kill the process
6. Clean up Cgroup resources
7. Return exit code and message after process is killed. An error otherwise.

#### Cgroup teardown

The following steps will be used to clean up unneeded Cgroup resources after a process is stopped or finished running. 
1. After cmd.Wait has returned (See 'Starting a process #7'), use `rmdir` to remove the Cgroup hiearchy created for the process.

### Streaming output

There are 3 functional behaviors for streaming the output
For clients to get a continuous stream of data, we will use grpc's server streaming rpc.

1. clients get a continuous stream of data
2. clients that connect start streaming from the beginning
3. multiple concurrent client can connect to the stream

A stream can be gracefully stopped by 2 events:
1. The caller cancels the stream. This can be controlled by context cancellation. A cancel can be propagated from the CLI.
2. The command exits and there is nothing left to stream. An `io.EOF` can be returned to signal the close of the server stream, which can then propagate to the client.

```proto
message StreamResponse { 
	bytes data = 1;
}

service Streamer {
	rpc Stream(StreamRequest) returns(stream StreamResponse)
}
```

To accomplish bullets 2 and 3, we will need to write a data structure that does the following
1. stores the complete output of the process
2. support 1 writer
3. support multiple readers

A potential implementation may look like the following:

```go
type Buf struct {
	sync.RWMutex
	buf []byte
}

// Process's stdout will write to buf
// expecting only a single writer.
func (b *Buf) Write(p []byte) (int, error) { 
	b.Lock()
	defer b.Unlock()
  // logic
}

// GetReader returns a io.Reader than can safely read from buf
// can handle multiple readers
func (b *Buf) GetReader() io.Reader {
	// each concurrent reader has its own closure. the variable offset represents the offset that it reads from buf.
	var offset int
	return func(p []byte) (int, error) {
		b.RLock()
		defer b.RUnlock()
		// use offset here
	}
}
```

Single Writer, multiple readers.
Readers take a `RLock`, writers take a `Lock`.

Another possible implementation that may avoid this issue is avoiding locks altogether and leveraging the fact that the
len of the buffer is always increasing. The length of the buffer can be written to a field that's atomically read by
readers and written by the single writer.

### Resource Limits via cgroups

There are 2 versions of Cgroups, v1 and v2.

One of the major differences is the hierarchy structure when mounting cgroups to `/sys/fs/cgroups`

To put it briefly, in v1, individual resource controllers are the top level concept, and groups are added underneath it.
If you needed a `cpu` and `mem` controller for a group `baz`, the structure would be
```
/sys/fs/cgroup/cpu/baz
/sys/fs/cgroup/mem/baz
```

In v2, the group is the top level concept and all available resource controllers are available based on the group.
The fs would be:
```
/sys/fs/cgroup/baz/{cpu.max, mem.max}
```
where `cpu.weight` and `mem.max` are files that control the cpu and mem resource. These limits are applied to all processes 
added to the group.

We will choose V2 for the simplicity in the file system.

We will limit CPU using `cpu.weight`, max memory allowed by `mem.max`, and IO limits by `io.max`.

What I'm not sure about is how to choose the correct $MAJ:$MIN device numbers.

Special considerations:
There is a brief window of time between the start of a process and adding the PID into the cgroup where
the target process escapes resource limits set forth by the cgroup and more importantly, has the chance to
create child processes that are not in the appropriate cgroup. In order to prevent this "escape" from occurring,
we create a parent `sh` process and place that process into a cgroup first. Then, we will create the target process as a child 
of the `sh` process. This ensures that when the target process is started, it will be executed already in the cgroup.
	

### Authorization

The identity of the user will be based off of the `Subject` field in the x509 certificate that is presented (because mTLS is required). Other considerations were
made, such as basing identity off of the certificate, but if attributes other than identity changes, then that would be
considered a new user. The obvious downside of this is if the `Subject` field changes, that would be considered a new
"user" in the system.

We will use a simple RBAC scheme, with the following roles and their access.
* viewer - can only run Get and Stream commands
* admin - can use all the exposed API's

Below are simple data models that can implement the basic RBAC scheme.

```go
type Role struct {
	Name string
	Actions []string // a list of allowable actions. in this exercise, they will be the methods as strings ("Get", "Stream", "Stop", "Start")
}

type User struct {
	ID string
	Roles []Role // a list of roles associated with the user
}

type Authorizer interface {
	// Determines if `user` can perform `action`. Returns access as a boolean and an error
	HasAccess(userID string, action string) (bool, error)
}
```

In our exercise, we will preload a two users, one with a `viewer` role and another with an `admin` role.

### Authentication

In order to authenticate and secure communication between server and client, mutual-TLS will be used.
TLS 1.0 and 1.1 have been deprecated, thus the minimum TLS versions required by the server will be TLS 1.2 and 1.3.

As long as the pair of TLS version and cipher suite chosen is robust and secure enough to secure communication for the
forseeable future, I don't have a strong opinion on the details of either one.

However, if I were to consider some trade offs, it would be the following
* TLS 1.3 removes insecure cipher suits available in 1.2. Thus it would be harder to select an insecure suite. There is
	also some performance benefits because it reduces round trips.
* TLS 1.2 has support for a wider range of clients. 

If choosing TLS 1.2, I would ensure that the cipher suite chosen has no known vulnerabilities or security issues. KEX
generates a ephemeral pub-key pair (uses ECDHE) for forward secrecy. Good performance in terms of processing the keys
(generally speaking RSA with higher bit size requires more processing, than ECDSA keys). Hashing algorithm free from
collision attack (no SHA1).

A strong cipher suite for TLS 1.2 that addresses the above concerns is `ECDHE-ECDSA-AES256-GCM-SHA384`.

For this exercise, because the client will just be the CLI, we will not need to provide support for a wider range of
clients, thus we will choose TLS 1.3.

### Security Considerations / Limitations

We need to consider safeguards against the types of commands/processes that can be created through this API. This api
currently has no safeguards to prevent the user from running commands that may be deleterious to the machine hosting
this service. There are no access controls on what the user can and cannot run in terms of commands.

The machine has no network ingress/egress safeguards. A user can curl a bash script and run it. A user can probably
exfiltrate data as well. 

There is currently no rate limiting on the service - ie there are no application level bounds on the number of
concurrent clients that can be streaming an output. At some point, it will reach a system level upper limit (likely
running out of fds).

In this exercise, I use int32 as the jobID and do a naive auto increment and auth.ID is just string of the Identity in
the cert. Using int is prone to possible attacks by attempting to query a job id that is +n/-n.
Those could be of type UUID reduce the risk, but for this exercise, I think auto increment is simple enough, but would
like to call it our here.

### Performance Considerations

This is not a HA system. An improvements can been to load balance requests and run multiple nodes of this application 
in the back (loadbalancing considerations will be routing requests to certain nodes because data is kept in memory).

### UX - CLI Experience

This section provides some examples CLI commands for each of the different supported APIs.

> urfave/cli library will be used to simplify the code for this exercise

Preliminary design of the CLI UX

```
$ client --help

USAGE:
   client command [command options] [arguments...]

COMMANDS:
   get      
   start    
   stop     
   stream   

GLOBAL OPTIONS:
   --ca-cert value      path to the CA certificate file
   --client-cert value  path to client certificate file 
   --client-key value   path to client key file 
   --target value       target address of server 
```

To run the command `ls -al`

```
$ client start ls -al
> { "id": 1, "status": "running" }
```


To stop a job with job id 1

```
$ client stop --id=1
> error message (if error)
```

To get a job with job id 1

```
$ client get --id=1
> { "id": 1, "status": "string" }

```

To stream a job with job id 1

```
$ client stream --id=1
> ..
> ..
```

The actual CLI output format may not be JSON, but the data presented returned be the same.

## Open Questions
* The ability to stream the data starting from the beginning of the process creation means that the process's output
	must be stored/buffered in-memory to provide good performance. An application issue that needs to be addressed is
	limited system memory. If a long running process generates enough data to use up all the available memory of the
	system, the program will fail. A possible solution is to set a maximum limit on the size that is stored for a given
	process. The data type could just be a byte slice but act as a ring buffer, where if data reaches that maximum size,
	new data will overwrite the oldest data.
