# GPFS Quota Management Example

This example demonstrates how to use ArgusSSH for controlled GPFS quota management operations.

## Overview

GPFS quota management allows administrators to control disk space and inode usage at different levels:
- **Fileset quotas**: Limits for independent filesets
- **User quotas**: Per-user limits within a filesystem
- **Group quotas**: Per-group limits within a filesystem

This configuration provides two levels of access:
1. **Read-only access** - View quota information and usage
2. **Admin access** - Set and modify quotas

## Configuration

### Templates

#### gpfs-quota-readonly

Allows users to view quota information without making changes:

```yaml
- name: gpfs-quota-readonly
  commands:
    - "mmlsquota {{.filesystem}}"
    - "mmlsquota -j {{.fileset}} {{.filesystem}}"
    - "mmlsquota -u {{.user}} {{.filesystem}}"
    - "mmlsquota -g {{.group}} {{.filesystem}}"
    - "mmrepquota {{.filesystem}}"
    - "mmrepquota -j {{.fileset}} {{.filesystem}}"
```

#### gpfs-quota-admin

Full quota management capabilities:

```yaml
- name: gpfs-quota-admin
  commands:
    - "mmlsquota {{.filesystem}}"
    - "mmlsquota -j {{.fileset}} {{.filesystem}}"
    - "mmlsquota -u {{.user}} {{.filesystem}}"
    - "mmlsquota -g {{.group}} {{.filesystem}}"
    - "mmrepquota {{.filesystem}}"
    - "mmrepquota -j {{.fileset}} {{.filesystem}}"
    - "mmsetquota {{.filesystem}}:{{.fileset}}"
    - "mmsetquota -u {{.user}} {{.filesystem}}"
    - "mmsetquota -g {{.group}} {{.filesystem}}"
    - "mmedquota -u {{.user}} {{.filesystem}}"
    - "mmedquota -g {{.group}} {{.filesystem}}"
    - "mmcheckquota {{.filesystem}}"
    - "mmcheckquota -j {{.fileset}} {{.filesystem}}"
```

### Users

#### Read-only viewer

```yaml
- username: quota-viewer
  password: quota123
  templates:
    - basic-commands
    - gpfs-quota-readonly
  params:
    filesystem: "gpfs01"
    fileset: "project01"
    user: "john"
    group: "research"
```

This user can:
- List all quotas in `gpfs01`
- View quota for specific fileset `project01`
- Check quota for user `john`
- Check quota for group `research`
- Generate quota reports

#### Administrator

```yaml
- username: quota-admin
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKey quota-admin@storage"
  templates:
    - basic-commands
    - gpfs-quota-admin
  params:
    filesystem: "gpfs01"
    fileset: "project01"
    user: "john"
    group: "research"
```

This user can:
- All read-only operations
- Set fileset quotas
- Set user quotas
- Set group quotas
- Edit quotas interactively
- Check and repair quota consistency

## Usage Examples

### Read-only operations

```bash
# List all quotas in filesystem
ssh -p 2222 quota-viewer@storage "mmlsquota gpfs01"

# View fileset quota
ssh -p 2222 quota-viewer@storage "mmlsquota -j project01 gpfs01"

# Check user quota
ssh -p 2222 quota-viewer@storage "mmlsquota -u john gpfs01"

# Check group quota
ssh -p 2222 quota-viewer@storage "mmlsquota -g research gpfs01"

# Generate quota report
ssh -p 2222 quota-viewer@storage "mmrepquota gpfs01"

# Generate fileset quota report
ssh -p 2222 quota-viewer@storage "mmrepquota -j project01 gpfs01"
```

### Admin operations

#### Fileset Quotas

```bash
# Set fileset quota (10TB block, 1M files)
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota gpfs01:project01 --block 0:10T --files 0:1M"

# Set fileset quota with grace period
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota gpfs01:project01 --block 9T:10T:7d --files 900K:1M:7d"

# Check fileset quota consistency
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmcheckquota -j project01 gpfs01"
```

#### User Quotas

```bash
# Set user quota (1TB block, 100K files)
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota -u john gpfs01 --block 0:1T --files 0:100K"

# Set user quota with soft/hard limits and grace
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota -u john gpfs01 --block 900G:1T:7d --files 90K:100K:7d"

# Edit user quota interactively (requires terminal)
# Note: mmedquota opens an editor, may not work well over SSH
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmedquota -u john gpfs01"
```

#### Group Quotas

```bash
# Set group quota (5TB block, 500K files)
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota -g research gpfs01 --block 0:5T --files 0:500K"

# Set group quota with soft/hard limits
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmsetquota -g research gpfs01 --block 4.5T:5T:14d --files 450K:500K:14d"

# Edit group quota interactively
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmedquota -g research gpfs01"
```

#### Quota Maintenance

```bash
# Check quota consistency for entire filesystem
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmcheckquota gpfs01"

# Check and repair quota (if needed)
ssh -i ~/.ssh/quota_admin -p 2222 quota-admin@storage \
  "mmcheckquota gpfs01 -u"
```

## Understanding Quota Limits

GPFS supports soft and hard limits with grace periods:

- **Soft limit**: Warning threshold, can be exceeded temporarily
- **Hard limit**: Absolute maximum, cannot be exceeded
- **Grace period**: Time allowed to exceed soft limit before it becomes hard

### Quota Format

```bash
mmsetquota <target> --block <soft>:<hard>:<grace> --files <soft>:<hard>:<grace>
```

Examples:
- `--block 0:10T` - No soft limit, 10TB hard limit
- `--block 9T:10T:7d` - 9TB soft, 10TB hard, 7 days grace
- `--files 0:1M` - No soft limit, 1M files hard limit
- `--files 900K:1M:7d` - 900K soft, 1M hard, 7 days grace

## Monitoring Quota Usage

### Check if quota is exceeded

```bash
# View quota status (shows if over quota)
ssh -p 2222 quota-viewer@storage "mmlsquota -u john gpfs01"

# Output example:
#                          Block Limits                    |     File Limits
# Filesystem type    KB      quota      limit  in_doubt  grace |  files   quota    limit in_doubt  grace
# gpfs01     USR  950000000 1000000000 1100000000      0   6d  |  95000  100000  110000       0    6d
#                   ^^^^^^^^ ^^^^^^^^^^                  ^^^^
#                   usage    soft limit                  grace remaining
```

### Generate reports

```bash
# Summary report for all users
ssh -p 2222 quota-viewer@storage "mmrepquota gpfs01"

# Detailed report for fileset
ssh -p 2222 quota-viewer@storage "mmrepquota -j project01 gpfs01"

# Report for specific user
ssh -p 2222 quota-viewer@storage "mmlsquota -u john gpfs01 -v"
```

## Security Considerations

1. **Use public key authentication for admin users** - Quota changes are privileged operations

2. **Separate read-only and admin roles** - Most users only need to view quotas

3. **Parameterize targets** - Template parameters ensure users can only operate on assigned filesystem/fileset/user/group

4. **Audit logging** - All quota operations are logged by ArgusSSH

5. **Grace periods** - Use appropriate grace periods to give users time to clean up

6. **Regular monitoring** - Set up automated quota reports to catch issues early

## Multi-tenant Quota Management

For multi-tenant environments, create separate quota admins per tenant:

```yaml
# Tenant A quota admin
- username: quota-admin-tenantA
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3... admin@tenantA"
  templates:
    - gpfs-quota-admin
  params:
    filesystem: "gpfs01"
    fileset: "tenantA"
    user: "tenantA_*"  # Note: * is literal, not wildcard
    group: "tenantA"

# Tenant B quota admin
- username: quota-admin-tenantB
  password: ""
  authorized_keys:
    - "ssh-ed25519 AAAAC3... admin@tenantB"
  templates:
    - gpfs-quota-admin
  params:
    filesystem: "gpfs01"
    fileset: "tenantB"
    user: "tenantB_*"
    group: "tenantB"
```

## Automation Examples

### Python: Set quota for new user

```python
import paramiko

def set_user_quota(username, block_limit, file_limit):
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect('storage', port=2222, username='quota-admin', 
                   key_filename='/path/to/key')
    
    cmd = f'mmsetquota -u {username} gpfs01 --block 0:{block_limit} --files 0:{file_limit}'
    stdin, stdout, stderr = client.exec_command(cmd)
    
    result = stdout.read().decode()
    error = stderr.read().decode()
    
    client.close()
    
    if error:
        raise Exception(f"Quota set failed: {error}")
    return result

# Usage
set_user_quota('john', '1T', '100K')
```

### Bash: Quota monitoring script

```bash
#!/bin/bash
STORAGE_HOST="storage"
STORAGE_PORT="2222"
SSH_USER="quota-viewer"
SSH_PASS="quota123"

check_quota_usage() {
    local filesystem=$1
    local threshold=90  # Alert if over 90%
    
    # Get quota report
    sshpass -p "$SSH_PASS" ssh -p "$STORAGE_PORT" "$SSH_USER@$STORAGE_HOST" \
        "mmrepquota $filesystem" | while read line; do
        
        # Parse usage percentage (simplified)
        usage=$(echo "$line" | awk '{print $3}')
        limit=$(echo "$line" | awk '{print $4}')
        
        if [ -n "$usage" ] && [ -n "$limit" ]; then
            percent=$((usage * 100 / limit))
            if [ $percent -gt $threshold ]; then
                echo "WARNING: Quota usage at ${percent}% for $line"
            fi
        fi
    done
}

# Run check
check_quota_usage gpfs01
```

### Automated quota enforcement

```bash
#!/bin/bash
# Set quotas for all users in a group

QUOTA_ADMIN_KEY="/path/to/quota_admin_key"
STORAGE="storage"
PORT="2222"

set_group_member_quotas() {
    local group=$1
    local block_quota=$2
    local file_quota=$3
    
    # Get group members (assumes you have this list)
    for user in $(getent group "$group" | cut -d: -f4 | tr ',' ' '); do
        echo "Setting quota for $user..."
        ssh -i "$QUOTA_ADMIN_KEY" -p "$PORT" quota-admin@"$STORAGE" \
            "mmsetquota -u $user gpfs01 --block 0:$block_quota --files 0:$file_quota"
    done
}

# Set 1TB/100K quota for all research group members
set_group_member_quotas research 1T 100K
```

## Troubleshooting

### Quota not enforced

If quotas are set but not enforced:
1. Check if quotas are enabled on the filesystem: `mmlsfs gpfs01 -Q`
2. Enable quotas if needed: `mmchfs gpfs01 -Q yes`
3. Check quota consistency: `mmcheckquota gpfs01`

### Quota commands rejected by ArgusSSH

Check:
1. Command prefix matches the template
2. Filesystem/fileset/user/group parameters match user's configuration
3. User has the correct template assigned

### Quota commands fail on GPFS side

ArgusSSH only controls SSH access. GPFS quota commands require:
1. Proper GPFS cluster membership
2. Root or admin privileges
3. Quotas enabled on the filesystem

## Integration with Monitoring

### Prometheus Exporter

Create a quota metrics exporter:

```python
from prometheus_client import start_http_server, Gauge
import paramiko
import time

quota_usage = Gauge('gpfs_quota_usage_bytes', 'GPFS quota usage', 
                    ['filesystem', 'type', 'name'])
quota_limit = Gauge('gpfs_quota_limit_bytes', 'GPFS quota limit',
                    ['filesystem', 'type', 'name'])

def collect_quotas():
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect('storage', port=2222, username='quota-viewer',
                   password='quota123')
    
    stdin, stdout, stderr = client.exec_command('mmlsquota gpfs01')
    # Parse output and update metrics
    # ... (parsing logic)
    
    client.close()

if __name__ == '__main__':
    start_http_server(8000)
    while True:
        collect_quotas()
        time.sleep(60)
```

## Best Practices

1. **Set soft limits at 90% of hard limits** - Gives users warning before hitting hard limit

2. **Use appropriate grace periods** - 7 days for users, 14 days for groups

3. **Regular quota reports** - Generate weekly reports to identify trends

4. **Automate quota setting** - Use scripts to set consistent quotas for new users

5. **Monitor quota usage** - Alert when users approach limits

6. **Document quota policies** - Make quota limits clear to users

7. **Separate fileset and user quotas** - Use fileset quotas for project limits, user quotas for individual limits
