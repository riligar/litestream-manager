# Litestream Manager

Automatic SQLite backup system with multi-client support based on GUID, powered by [Litestream](https://github.com/benbjohnson/litestream).

## 🔄 How It Works

**Server Workflow:**

1. **Initialization:** Validate directories and start the file watcher.
2. **Discovery:** Scan for existing `.db` files with valid GUIDs.
3. **Configuration:** For each detected database:
   - Create a unique Litestream configuration.
   - **If S3 is empty:** Start a full initial backup.
   - **If S3 contains data:** Sync with the existing backup (continue from where it left off).
   - Start continuous backup (WAL streaming).
   - Register the client in the system (O(1) lookup).
4. **Monitoring:** File watcher detects real-time changes:
   - **CREATE:** New `.db` → Automatically add client.
   - **DELETE:** Remove `.db` → Stop backup and clean records.
   - **MODIFY:** Update size statistics.
5. **Dashboard:** Real-time web interface updates.
6. **S3 Backup:** Litestream continuously replicates to `s3://bucket/databases/{clientID}/`.

**Optimized Flow:** Sub-millisecond detection → Automatic backup → Real-time dashboard.

## 📁 Project Structure

```
litestream-manager/
├── bin/                 # Compiled binaries (standalone)
├── src/
│   ├── main.go          # Main application code
│   └── template.html    # Dashboard template (embedded in binary)
├── data/                # Database files directory
├── go.mod               # Go module definition
├── go.sum               # Go dependencies
└── README.md            # This file
```

## 🛠️ Build for Production

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o bin/litestream-manager-linux src/main.go

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o bin/litestream-manager.exe src/main.go

# Build for macOS
go build -o bin/litestream-manager src/main.go
```

**📦 Standalone Binary:** The template HTML is embedded—no external files needed.

## 🚀 Quick Start

```bash
# Build the Litestream Manager binary
go build -o bin/litestream-manager src/main.go

# Configure AWS credentials
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key

# Create the data directory and start the server
mkdir -p data
./bin/litestream-manager -watch-dir "data" -bucket "applications-backups-prod"

# Access the dashboard at: http://localhost:8080
```

## ⚙️ Usage

### Parameters

| Flag         | Description                             | Default      |
|--------------|-----------------------------------------|--------------|
| `-watch-dir` | Directories to watch (comma-separated)  | **Required** |
| `-bucket`    | S3 bucket for backups                   | **Required** |
| `-port`      | Web server port                         | `8080`       |

### Client Management

```bash
# Add a client (GUID required: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)
touch data/12345678-1234-5678-9abc-123456789012.db

# Remove a client
rm data/12345678-1234-5678-9abc-123456789012.db

# Run with multiple environments
./bin/litestream-manager -watch-dir "data/prod" -bucket "prod-backups"
./bin/litestream-manager -watch-dir "data/staging" -bucket "staging-backups" -port 8081
```

## 📊 Structure

### Local
```
data/
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

**Production-ready SaaS system with automatic backup.** 🚀

