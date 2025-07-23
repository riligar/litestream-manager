# 🚀 Quick Start Guide

## ⚡ Immediate Setup (5 minutes)

### **1. Create Local Directory Structure**
```bash
# Create a local directory for your client databases
mkdir -p data/clients

# Add some sample GUID database files
touch data/clients/12345678-1234-5678-9abc-123456789012.db
touch data/clients/87654321-4321-8765-dcba-210987654321.db
```

### **2. Run the Program**
```bash
# Start monitoring the directory
go run main.go -watch-dir "data/clients" -bucket "your-s3-bucket"
```

### **3. View Dashboard**
Open your browser and visit: **http://localhost:8080**

### **4. Test Dynamic Client Creation**
```bash
# In another terminal, create a new client
touch data/clients/fedcba98-7654-3210-fedc-ba9876543210.db

# Watch the logs and dashboard update automatically!
```

## 🚨 Common Issues & Solutions

### **❌ Permission Denied / Read-only Filesystem**
```bash
# Problem: Using system directories like /data/
go run main.go -watch-dir "/data/" -bucket "backups"
# ❌ Failed to watch directory /data/: directory does not exist: /data/ (please create it first)

# Solution: Use local directories
mkdir -p data/clients
go run main.go -watch-dir "data/clients" -bucket "backups"
# ✅ Works perfectly!
```

### **❌ Directory Not Found**
```bash
# The program will tell you exactly what to do:
go run main.go -watch-dir "nonexistent" -bucket "backups"
# ❌ Failed to watch directory nonexistent: directory does not exist: nonexistent (please create it first)

# Just create the directory first:
mkdir -p nonexistent
go run main.go -watch-dir "nonexistent" -bucket "backups"
# ✅ Now it works!
```

## 📋 Best Practices

### **Directory Structure**
```
your-project/
├── data/
│   ├── clients/          # For production clients
│   ├── staging/          # For staging environment  
│   └── testing/          # For development
├── main.go
└── docs/
```

### **Production Setup**
```bash
# Create environment-specific directories
mkdir -p data/{production,staging,development}

# Run for production
go run main.go -watch-dir "data/production" -bucket "prod-backups"

# Run for multiple environments
go run main.go -watch-dir "data/production,data/staging" -bucket "multi-env-backups"
```

### **GUID File Naming**
```bash
# ✅ Valid GUID format
12345678-1234-5678-9abc-123456789012.db

# ❌ Invalid formats (will be ignored)
client-123.db
user_data.db
random-name.db
```

## 🎯 Next Steps

1. **Production Setup**: Read [Directory Watching Guide](directory-watching.md)
2. **Multi-tenant Architecture**: See [Architecture Documentation](architecture.md)  
3. **S3 Configuration**: Check [Database Organization](database-organization.md)

---
💡 **Pro Tip**: Always test with local directories first, then move to production paths once everything works! 