# Litestream Manager

Automatic SQLite backup system with multi-client support based on GUID.

## 🔄 How It Works

**Server Workflow:**

1. **Initialization**: Validates directories and starts file watcher
2. **Discovery**: Scans existing `.db` files with valid GUID
3. **Configuration**: For each detected database:
   - Creates unique Litestream configuration
   - **If S3 empty**: Starts full initial backup
   - **If S3 exists**: Syncs with existing backup (continues from where it left off)
   - Starts continuous backup (WAL streaming)
   - Registers client in system (O(1) lookup)
4. **Monitoring**: File watcher detects real-time changes:
   - **CREATE**: New `.db` → auto-adds client
   - **DELETE**: Remove `.db` → stops backup and cleans records
   - **MODIFY**: Updates size statistics
5. **Dashboard**: Real-time web interface updates
6. **S3 Backup**: Litestream continuously replicates to `s3://bucket/databases/{clientID}/`

**Optimized Flow**: Sub-millisecond detection → Automatic backup → Real-time dashboard


## 🛠️ Build for Production

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o litestream-manager-linux main.go

# Windows  
GOOS=windows GOARCH=amd64 go build -o litestream-manager.exe main.go

# macOS
go build -o litestream-manager main.go
```

## 🚀 Quick Start

```bash
# Build
go build -o litestream-manager main.go

# Configure AWS
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key

# Create directory and start
mkdir -p data/clients
./litestream-manager -watch-dir "data/clients" -bucket "your-s3-bucket"

# Dashboard: http://localhost:8080
```

## ⚙️ Usage

### Parameters

| Flag | Description | Default |
|------|-------------|---------|
| `-watch-dir` | Directories to watch (comma-separated) | **required** |
| `-bucket` | S3 bucket for backups | **required** |
| `-port` | Web server port | `8080` |

### Client Management

```bash
# Add client (GUID required: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
touch data/clients/12345678-1234-5678-9abc-123456789012.db

# Remove client
rm data/clients/12345678-1234-5678-9abc-123456789012.db

# Multiple environments
./litestream-manager -watch-dir "data/prod" -bucket "prod-backups"
./litestream-manager -watch-dir "data/staging" -bucket "staging-backups" -port 8081
```

## 📊 Structure

### Local
```
data/clients/
├── 12345678-1234-5678-9abc-123456789012.db
├── 98765432-4321-8765-cba9-876543210987.db
└── abcdef01-2345-6789-abcd-ef0123456789.db
```

### S3
```
s3://bucket/databases/
├── 12345678-1234-5678-9abc-123456789012/
├── 98765432-4321-8765-cba9-876543210987/
└── abcdef01-2345-6789-abcd-ef0123456789/
```

## 🔧 Restore

```bash
litestream restore \
  -o "restore/client.db" \
  s3://bucket/databases/12345678-1234-5678-9abc-123456789012
```

## ⚡ Performance

- **Clients**: ~1000 per instance (1:1 client:database)
- **Lookup**: O(1) for all operations
- **Memory**: 30-150MB optimized
- **File Watcher**: Native fsnotify (sub-millisecond)

**Production-ready SaaS system with automatic backup!** 🚀

