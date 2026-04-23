# GPFS Fileset Management Example

This example demonstrates how to use ArgusSSH for controlled GPFS fileset management operations.

## Overview

GPFS (IBM Spectrum Scale) filesets are independent file namespaces within a filesystem. This configuration provides two levels of access:

1. **Read-only access** - View fileset information and quotas
2. **Admin access** - Create, modify, delete filesets and manage quotas

## Configuration

### Templates

#### gpfs-fileset-readonly

Allows users to view fileset information without making changes:

```yaml
- name: gpfs-fileset-readonly
  commands:
    - "mmlsfileset {{.filesystem}}"
    - "mmlsfileset {{.filesystem}} {{.fileset}}"
    - "mmlsquota -j {{.fileset}} {{.filesystem}}"
    - "mmdf {{.filesystem}}"
    - "mmlsfs {{.filesystem}}"
```

#### gpfs-fileset-admin

Full fileset management capabilities:

```yaml
- name: gpfs-fileset-admin
  commands:
    - "mmlsfileset {{.filesystem}}"
    - "mmlsfileset {{.filesystem}} {{.fileset}}"
    - "mmcrfileset {{.filesystem}} {{.fileset}}"
    - "mmlinkfileset {{.filesystem}} {{.fileset}}"
    - "mmunlinkfileset {{.filesystem}} {{.fileset}}"
    - "mmdelfileset {{.filesystem}} {{.fileset}}"
    - "mmchfileset {{.filesystem}} {{.fileset}}"
    - "mmlsquota -j {{.fileset}} {{.filesystem}}"
    - "mmsetquota {{.filesystem}}:{{.fileset}}"
    - "mmdf {{.filesystem}}"
    - "mmlsfs {{.filesystem}}"
```

### Users

#### Read-only viewer

```yaml
- username: gpfs-viewer
  password: viewer123
  templates:
    - basic-commands
    - gpfs-fileset-readonly
  params:
    filesystem: "gpfs01"
    fileset: "project01"
```

This user can:
- List filesets in `gpfs01`
- View details of `project01` fileset
- Check quota usage for `project01`
- View filesystem information

#### Administrator

```yaml
- username: gpfs-admin
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKey gpfs-admin@storage"
  templates:
    - basic-commands
    - gpfs-fileset-admin
  params:
    filesystem: "gpfs01"
    fileset: "project01"
```

This user can:
- All read-only operations
- Create new filesets
- Link/unlink filesets to directories
- Delete filesets
- Modify fileset properties
- Set quotas

## Usage Examples

### Read-only operations

```bash
# List all filesets
ssh -p 2222 gpfs-viewer@storage "mmlsfileset gpfs01"

# View specific fileset details
ssh -p 2222 gpfs-viewer@storage "mmlsfileset gpfs01 project01"

# Check quota usage
ssh -p 2222 gpfs-viewer@storage "mmlsquota -j project01 gpfs01"

# Check filesystem space
ssh -p 2222 gpfs-viewer@storage "mmdf gpfs01"
```

### Admin operations

```bash
# Create a new fileset
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmcrfileset gpfs01 project02"

# Link fileset to directory
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmlinkfileset gpfs01 project02 -J /gpfs01/projects/project02"

# Set quota (10TB block, 1M files)
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmsetquota gpfs01:project02 --block 0:10T --files 0:1M"

# Modify fileset properties
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmchfileset gpfs01 project02 --inode-space new"

# Unlink fileset
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmunlinkfileset gpfs01 project02"

# Delete fileset
ssh -i ~/.ssh/gpfs_admin -p 2222 gpfs-admin@storage "mmdelfileset gpfs01 project02 -f"
```

## Security Considerations

1. **Use public key authentication for admin users** - More secure than passwords for privileged operations

2. **Separate read-only and admin roles** - Don't give admin access unless necessary

3. **Parameterize filesystem and fileset names** - The template parameters ensure users can only operate on their assigned filesystem/fileset

4. **Command prefix matching** - Users can pass additional flags to commands (e.g., `mmlsfileset gpfs01 -L` works if `mmlsfileset gpfs01` is allowed)

5. **Audit logging** - All GPFS operations are logged by ArgusSSH for audit trails

6. **Network restrictions** - Limit SSH access to storage management networks only

## Multi-filesystem Support

To support multiple filesystems, create users with different parameters:

```yaml
- username: gpfs-admin-fs01
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3... admin@host"
  templates:
    - gpfs-fileset-admin
  params:
    filesystem: "gpfs01"
    fileset: "*"  # Note: * is literal, not a wildcard

- username: gpfs-admin-fs02
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3... admin@host"
  templates:
    - gpfs-fileset-admin
  params:
    filesystem: "gpfs02"
    fileset: "*"
```

## Integration with Automation

ArgusSSH can be integrated with automation tools:

```python
# Python example using paramiko
import paramiko

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('storage', port=2222, username='gpfs-admin', key_filename='/path/to/key')

# Create fileset
stdin, stdout, stderr = client.exec_command('mmcrfileset gpfs01 project03')
print(stdout.read().decode())

# Set quota
stdin, stdout, stderr = client.exec_command('mmsetquota gpfs01:project03 --block 0:5T')
print(stdout.read().decode())

client.close()
```

```bash
# Bash script example
#!/bin/bash
STORAGE_HOST="storage"
STORAGE_PORT="2222"
SSH_KEY="/path/to/gpfs_admin_key"

create_fileset() {
    local fs=$1
    local fileset=$2
    ssh -i "$SSH_KEY" -p "$STORAGE_PORT" gpfs-admin@"$STORAGE_HOST" \
        "mmcrfileset $fs $fileset"
}

set_quota() {
    local fs=$1
    local fileset=$2
    local size=$3
    ssh -i "$SSH_KEY" -p "$STORAGE_PORT" gpfs-admin@"$STORAGE_HOST" \
        "mmsetquota $fs:$fileset --block 0:$size"
}

# Usage
create_fileset gpfs01 project04
set_quota gpfs01 project04 20T
```

## Troubleshooting

### Command rejected

If a command is rejected, check:
1. The command prefix matches the template
2. The filesystem/fileset parameters match the user's configuration
3. The user has the correct template assigned

### Permission denied on GPFS side

ArgusSSH only controls SSH access. GPFS commands still require:
1. Proper GPFS cluster membership
2. Root or admin privileges on the GPFS node
3. Correct GPFS roles assigned to the user

Consider running ArgusSSH on a dedicated GPFS admin node with appropriate privileges.
