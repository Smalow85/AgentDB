async function executeSQL() {
    const sql = document.getElementById('sql-input').value.trim()
    if (!sql) return

    const result = document.getElementById('sql-result')
    result.textContent = '⏳ Выполняется...'

    const d = await post('/api/query', { sql })
    result.textContent = d.result || d.error || 'OK'
    loadTables()
}

async function loadTables() {
    const d = await get('/api/tables')
    const list = document.getElementById('tables-list')
    list.innerHTML = (d.tables || []).map(t => `<div class="list-item">📄 ${t}</div>`).join('')
}