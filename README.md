Litestream as Library
=====================

This repository is an example of embedding Litestream as a library in a Go
application. The Litestream API is not stable so you may need to update your
code in the future when you upgrade.


## Install

To install, run:

```sh
go install .
```

You should now have a `litestream-library-example` in `$GOPATH/bin`.


## Usage

This example application uses AWS S3 and only provides a `-bucket` configuration
flag. It will pull AWS credentials from environment variables so you will need
to set those:

```sh
export AWS_ACCESS_KEY_ID=xxxxxxxxxxxxxxxxxxxx
export AWS_SECRET_ACCESS_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

You'll need to setup an S3 bucket and use that name when running the app.

```sh
litestream-library-example -dsn /path/to/db -bucket YOURBUCKETNAME
```

  ```sh
  # Single database mode (legacy)
  go run main.go -dsn data/2025-07-23.db -bucket applications-backups-prod
  
  # Directory watching mode (recommended)
  mkdir -p data/clients
  go run main.go -watch-dir "data/clients" -bucket applications-backups-prod
  ```

On your first run, it will see that there is no snapshot available so the
application will create a new database. If you restart the application then
it will see the local database and use that.

If you remove the database:

```
rm /path/to/db* /path/to/.db-litestream
```

Then when you restart the application, it will fetch the latest snapshot and
replay all WAL files up to the latest position.


## Synchronous replication

This repository provides an example of confirming that the replica syncs to S3
before returning to the caller. Replicating to S3 can be slow so you may end 
up waiting several hundred milliseconds before the sync returns.

## Database Organization

The current implementation includes support for organizing databases in S3 by name:

- Individual databases are stored in separate folders: `databases/{db-name}/`
- Automatic name extraction from DSN path
- Custom naming via `-db-name` flag
- See `docs/database-organization.md` for detailed usage

## GUID-Based Organization & Directory Watching

The system now supports **automatic multi-client management** for SaaS scenarios:

### **🆕 Directory Mode (Recommended)**
- **Watch entire directories**: `-watch-dir /data/clients/`
- **Auto-detect GUID clients**: Monitors `*.db` files with GUID names
- **Zero configuration**: New clients automatically detected and backed up
- **Real-time monitoring**: File system events trigger instant registration

### **📊 Web Dashboard**
- **Live status**: `http://localhost:8080` shows all active clients
- **API endpoint**: `/api/status` for programmatic access
- **Visual interface**: Monitor clients, S3 paths, and system health

### **🔄 Usage Examples**
```bash
# Monitor directory for multiple clients
mkdir -p data/clients
./litestream-example -watch-dir "data/clients" -bucket "saas-backups"

# Monitor multiple directories  
mkdir -p data/clients data/prod
./litestream-example -watch-dir "data/clients,data/prod" -bucket "backups"

# Legacy single database mode
./litestream-example -dsn "/data/single.db" -bucket "backups"
```

See `docs/guid-implementation-summary.md` for complete details.

## Multitenant Architecture

For scenarios requiring multiple dynamic SQLite databases (SaaS, multi-tenant applications), see the comprehensive architecture documentation:

- `docs/directory-watching.md` - **NEW**: Directory monitoring and multi-client mode
- `docs/guid-implementation-summary.md` - GUID-based organization and usage
- `docs/multitenant-architecture.md` - Complete multitenant design  
- `docs/system-comparison.md` - Comparison with single-database system
- `docs/implementation-guide.md` - Practical implementation guide
- Support for dynamic database detection, API management, and tenant isolation

