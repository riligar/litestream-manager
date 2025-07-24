#!/bin/bash
# Script rápido para modificar bancos existentes e gerar atividade

DATA_DIR="../data"

echo "⚡ Modificação rápida dos bancos existentes..."
echo "📁 Diretório: $DATA_DIR"
echo

# Função para validar GUID
is_valid_guid() {
    local guid="$1"
    if [[ $guid =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
        return 0
    else
        return 1
    fi
}

# Encontrar bancos válidos
BANKS_MODIFIED=0

for db_file in "$DATA_DIR"/*.db; do
    if [ -f "$db_file" ]; then
        filename=$(basename "$db_file")
        client_id="${filename%.db}"
        
        if is_valid_guid "$client_id"; then
            echo "🔨 Modificando: $client_id"
            
            # Criar tabela de atividade se não existir
            sqlite3 "$db_file" "
            CREATE TABLE IF NOT EXISTS activity_log (
                id INTEGER PRIMARY KEY,
                timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
                activity_type TEXT,
                description TEXT
            );
            " 2>/dev/null || true
            
            # Adicionar atividade atual
            TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
            sqlite3 "$db_file" "
            INSERT INTO activity_log (activity_type, description) VALUES 
                ('manual_test', 'Script execution at $TIMESTAMP'),
                ('data_modification', 'Adding test data for backup verification'),
                ('wal_generation', 'Generating WAL activity for Litestream');
            " 2>/dev/null && echo "   ✅ Dados adicionados" || echo "   ⚠️  Erro ao modificar"
            
            # Fazer update para gerar mais atividade WAL
            sqlite3 "$db_file" "
            UPDATE activity_log 
            SET description = description || ' [Updated at $TIMESTAMP]' 
            WHERE id IN (SELECT id FROM activity_log ORDER BY id DESC LIMIT 1);
            " 2>/dev/null || true
            
            # Executar SELECT e mostrar resultados na tela
            echo "   📊 Últimas atividades registradas:"
            sqlite3 "$db_file" "
            SELECT 
                id,
                datetime(timestamp, 'localtime') as local_time,
                activity_type,
                description
            FROM activity_log 
            ORDER BY timestamp DESC 
            LIMIT 3;
            " 2>/dev/null | while IFS='|' read -r id timestamp activity_type description; do
                if [ -n "$id" ]; then
                    echo "      ID: $id | Time: $timestamp"
                    echo "      Type: $activity_type"
                    echo "      Description: $description"
                    echo "      ---"
                fi
            done
            
            BANKS_MODIFIED=$((BANKS_MODIFIED + 1))
        else
            echo "⚠️  GUID inválido: $client_id"
        fi
    fi
done

echo
if [ $BANKS_MODIFIED -gt 0 ]; then
    echo "✅ $BANKS_MODIFIED bancos modificados com sucesso!"
    echo
    echo "📊 Para ver as modificações:"
    echo "   sqlite3 $DATA_DIR/CLIENT_ID.db \"SELECT * FROM activity_log ORDER BY timestamp DESC LIMIT 5;\""
    echo
    echo "🔄 Se o Litestream estiver rodando, as modificações serão automaticamente sincronizadas."
else
    echo "❌ Nenhum banco válido encontrado para modificar"
fi 