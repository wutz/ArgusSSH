# ArgusSSH

ArgusSSH is a secure SSH server that validates commands against configurable templates before execution. It provides fine-grained access control by allowing administrators to define command templates with parameters, and assign them to users.

## Features

- **Command Template System**: Define allowed commands as templates with parameter substitution
- **User-based Access Control**: Each user gets specific command templates with custom parameters
- **Password Authentication**: Simple password-based SSH authentication
- **Command Validation**: Commands are validated against allowed patterns before execution
- **Template Rendering**: Support for Go template syntax in command definitions

## Quick Start

### 1. Build

```bash
go build -o argusssh ./cmd/argusssh
```

### 2. Create Configuration

Copy the example config and customize it:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` to define your command templates and users.

### 3. Run

```bash
./argusssh -config config.yaml
```

The server will listen on the configured port (default `:2222`).

### 4. Connect

```bash
ssh -p 2222 alice@localhost
# Then run allowed commands like:
# echo hello
# date
# ls -la
```

Or execute commands directly:

```bash
ssh -p 2222 alice@localhost "echo hello world"
```

## Configuration

### Server Section

```yaml
server:
  listen: ":2222"           # Address to listen on
  host_key: "host_key.pem"  # Path to host key (leave empty for ephemeral key)
```

### Templates Section

Define command templates with optional Go template parameters:

```yaml
templates:
  - name: basic-commands
    commands:
      - "echo"
      - "date"
      - "uptime"

  - name: docker-ops
    commands:
      - "docker ps"
      - "docker logs {{.container}}"
      - "docker exec {{.container}}"
```

### Users Section

Assign templates to users with custom parameters:

```yaml
users:
  - username: alice
    password: alice123
    templates:
      - basic-commands
    params: {}

  - username: devops
    password: devops789
    templates:
      - basic-commands
      - docker-ops
    params:
      container: "myapp"
```

In this example:
- `alice` can run `echo`, `date`, `uptime` with any arguments
- `devops` can run those plus `docker logs myapp`, `docker exec myapp`, etc.

## Command Matching

Commands are matched by prefix. If a template defines `docker logs`, then:
- ✅ `docker logs myapp` - allowed
- ✅ `docker logs myapp --tail 100` - allowed
- ❌ `docker ps` - not allowed (different command)
- ❌ `docker` - not allowed (too short)

## Deployment

### Systemd Service

Create `/etc/systemd/system/argusssh.service`:

```ini
[Unit]
Description=ArgusSSH Server
After=network.target

[Service]
Type=simple
User=argusssh
Group=argusssh
WorkingDirectory=/opt/argusssh
ExecStart=/opt/argusssh/argusssh -config /opt/argusssh/config.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable argusssh
sudo systemctl start argusssh
```

### Docker

Create `Dockerfile`:

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o argusssh ./cmd/argusssh

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/argusssh /usr/local/bin/
COPY config.yaml /etc/argusssh/config.yaml
EXPOSE 2222
CMD ["argusssh", "-config", "/etc/argusssh/config.yaml"]
```

Build and run:

```bash
docker build -t argusssh .
docker run -d -p 2222:2222 -v $(pwd)/config.yaml:/etc/argusssh/config.yaml argusssh
```

### Kubernetes

Create `deployment.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argusssh-config
data:
  config.yaml: |
    server:
      listen: ":2222"
      host_key: ""
    templates:
      - name: basic
        commands:
          - "echo"
          - "date"
    users:
      - username: admin
        password: changeme
        templates:
          - basic
        params: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: argusssh
spec:
  replicas: 1
  selector:
    matchLabels:
      app: argusssh
  template:
    metadata:
      labels:
        app: argusssh
    spec:
      containers:
      - name: argusssh
        image: argusssh:latest
        ports:
        - containerPort: 2222
        volumeMounts:
        - name: config
          mountPath: /etc/argusssh
      volumes:
      - name: config
        configMap:
          name: argusssh-config
---
apiVersion: v1
kind: Service
metadata:
  name: argusssh
spec:
  type: LoadBalancer
  ports:
  - port: 2222
    targetPort: 2222
  selector:
    app: argusssh
```

Deploy:

```bash
kubectl apply -f deployment.yaml
```

## Security Considerations

1. **Use Strong Passwords**: The example uses simple passwords for demonstration. Use strong passwords in production.

2. **Persistent Host Key**: Generate a persistent host key to avoid "host key changed" warnings:
   ```bash
   ssh-keygen -t ed25519 -f host_key -N ""
   ```
   Then set `host_key: "host_key"` in config.

3. **Principle of Least Privilege**: Only grant users the minimum commands they need.

4. **Command Validation**: Remember that command matching is prefix-based. Be specific in your templates.

5. **Network Security**: Use firewall rules to restrict SSH access to trusted networks.

6. **Audit Logging**: ArgusSSH logs all connection attempts and command executions. Monitor these logs.

## Testing

Run unit tests:

```bash
go test ./...
```

Run integration tests:

```bash
go test -v ./cmd/argusssh
```

## License

Apache License 2.0
