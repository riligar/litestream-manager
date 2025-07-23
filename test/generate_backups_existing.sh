#!/bin/bash
# Script para gerar backups dos bancos existentes na pasta data/clients

set -e

BUCKET="${1:-your-bucket}"
DATA_DIR="../data/clients"
BACKUP_DURATION=30

echo "🚀 Gerando backups para bancos existentes..."
echo "📁 Diretório: $DATA_DIR"
echo "☁️  S3 Bucket: $BUCKET"
echo "⏱️  Duração do backup: ${BACKUP_DURATION}s"
echo

# Verificar se diretório existe
if [ ! -d "$DATA_DIR" ]; then
    echo "❌ Diretório $DATA_DIR não encontrado!"
    exit 1
fi

# Função para validar GUID
is_valid_guid() {
    local guid="$1"
    if [[ $guid =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
        return 0
    else
        return 1
    fi
}

# Encontrar bancos .db válidos
DATABASES=()
for db_file in "$DATA_DIR"/*.db; do
    if [ -f "$db_file" ]; then
        filename=$(basename "$db_file")
        client_id="${filename%.db}"
        
        if is_valid_guid "$client_id"; then
            DATABASES+=("$db_file:$client_id")
            echo "✅ Banco encontrado: $client_id"
        else
            echo "⚠️  GUID inválido ignorado: $client_id"
        fi
    fi
done

if [ ${#DATABASES[@]} -eq 0 ]; then
    echo "❌ Nenhum banco com GUID válido encontrado em $DATA_DIR"
    exit 1
fi

echo
echo "📊 Total de bancos: ${#DATABASES[@]}"

# Processar cada banco
PIDS=()
for db_entry in "${DATABASES[@]}"; do
    IFS=':' read -r db_path client_id <<< "$db_entry"
    
    echo
    echo "🔄 Processando cliente: $client_id"
    echo "   Banco: $db_path"
    
    # Verificar estado atual do banco
    echo "📈 Estado atual do banco:"
    sqlite3 "$db_path" "
    .tables
    SELECT 'Total tables: ' || COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';
    " 2>/dev/null || echo "   ⚠️  Erro ao ler banco ou banco vazio"
    
    # Criar configuração temporária
    config_file="litestream_${client_id}.yml"
    cat > "$config_file" <<EOF
dbs:
  - path: $db_path
    replicas:
      - type: s3
        bucket: $BUCKET
        path: databases/$client_id
        region: us-east-1
        sync-interval: 1s
        retention: 24h
EOF
    
    echo "🚀 Iniciando backup para $client_id..."
    litestream replicate -config "$config_file" &
    pid=$!
    PIDS+=("$pid:$config_file:$client_id")
    
    echo "   PID: $pid"
done

echo
echo "⏰ Aguardando snapshot inicial (10 segundos)..."
sleep 10

# Fazer modificações em cada banco para gerar WAL segments
echo
echo "📝 Adicionando dados para gerar WAL segments..."

for db_entry in "${DATABASES[@]}"; do
    IFS=':' read -r db_path client_id <<< "$db_entry"
    
    echo "🔨 Modificando banco $client_id..."
    
    # Tentar criar tabela de teste se não existir
    sqlite3 "$db_path" "
    CREATE TABLE IF NOT EXISTS backup_test (
        id INTEGER PRIMARY KEY,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
        action TEXT,
        data TEXT
    );
    " 2>/dev/null || true
    
    # Inserir dados de teste
    sqlite3 "$db_path" "
    INSERT INTO backup_test (action, data) VALUES 
        ('generation_test', 'Initial backup test data'),
        ('wal_segment_1', 'First WAL segment data');
    " 2>/dev/null || true
    
    echo "   ✅ Primeira modificação"
done

echo "⏰ Aguardando sincronização (5 segundos)..."
sleep 5

# Segunda rodada de modificações
echo
echo "📝 Segunda rodada de modificações..."

for db_entry in "${DATABASES[@]}"; do
    IFS=':' read -r db_path client_id <<< "$db_entry"
    
    sqlite3 "$db_path" "
    INSERT INTO backup_test (action, data) VALUES 
        ('wal_segment_2', 'Second WAL segment data'),
        ('update_test', 'Updated data for generation');
    UPDATE backup_test SET data = 'Updated: ' || data WHERE action = 'generation_test';
    " 2>/dev/null || true
    
    echo "   ✅ Segunda modificação para $client_id"
done

echo "⏰ Aguardando sincronização final (10 segundos)..."
sleep 10

# Terceira rodada para mais snapshots
echo
echo "📝 Gerando mais snapshots..."

for db_entry in "${DATABASES[@]}"; do
    IFS=':' read -r db_path client_id <<< "$db_entry"
    
    sqlite3 "$db_path" "
    INSERT INTO backup_test (action, data) VALUES 
        ('final_test', 'Final backup generation'),
        ('snapshot_3', 'Third snapshot data');
    DELETE FROM backup_test WHERE id % 3 = 0;
    " 2>/dev/null || true
    
    echo "   ✅ Modificação final para $client_id"
done

echo "⏰ Aguardando backup final (5 segundos)..."
sleep 5

# Parar todos os processos do Litestream
echo
echo "🛑 Parando processos de backup..."

for pid_entry in "${PIDS[@]}"; do
    IFS=':' read -r pid config_file client_id <<< "$pid_entry"
    
    echo "   Parando $client_id (PID: $pid)..."
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    
    # Limpar arquivo de configuração
    rm -f "$config_file"
    
    echo "   ✅ Parado"
done

echo
echo "🔍 Verificando gerações criadas..."

for db_entry in "${DATABASES[@]}"; do
    IFS=':' read -r db_path client_id <<< "$db_entry"
    
    echo
    echo "📊 Cliente: $client_id"
    echo "   S3 Path: s3://$BUCKET/databases/$client_id/"
    
    # Tentar listar gerações
    if litestream generations "s3://$BUCKET/databases/$client_id" 2>/dev/null; then
        echo "   ✅ Gerações listadas com sucesso"
    else
        echo "   ⚠️  Erro ao listar gerações (normal se bucket não configurado)"
    fi
done

echo
echo "🎉 Processo concluído!"
echo
echo "📋 Resumo:"
echo "   - Bancos processados: ${#DATABASES[@]}"
echo "   - Bucket S3: $BUCKET"
echo "   - Modificações: 3 rodadas por banco"
echo
echo "🌐 Para testar no dashboard:"
echo "   1. ./bin/litestream-manager -bucket $BUCKET -watch-dir $DATA_DIR"
echo "   2. Acesse: http://localhost:8080"
echo "   3. Clique em 'View Backups' nos clientes listados"
echo
echo "🧹 Limpeza automática: arquivos temporários removidos" 