// static/sql.js — исправленная рабочая версия

async function executeSQL() {
    const sql = document.getElementById('sql-input').value.trim()
    if (!sql) return

    const resultDiv = document.getElementById('sql-result')
    resultDiv.innerHTML = '⏳ Выполняется...'
    resultDiv.classList.add('loading')

    try {
        const data = await post('/api/query', { sql })

        if (data.error) {
            resultDiv.innerHTML = `<div class="sql-error">❌ ${escapeHTML(data.error)}</div>`
            resultDiv.classList.remove('loading')
            return
        }

        if (data.type === 'SELECT') {
            renderSelectResult(data, resultDiv)
        } else if (data.type === 'INSERT') {
            renderInsertResult(data, resultDiv)
        } else if (data.type === 'UPDATE' || data.type === 'DELETE') {
            renderAffectedResult(data, resultDiv)
        } else if (data.type === 'CREATE' || data.type === 'CREATE_INDEX') {
            renderCreateResult(data, resultDiv)
        } else if (data.type === 'ERROR') {
            resultDiv.innerHTML = `<div class="sql-error">❌ ${escapeHTML(data.error)}</div>`
        } else {
            resultDiv.innerHTML = `<div class="sql-result-text"><pre>${escapeHTML(JSON.stringify(data, null, 2))}</pre></div>`
        }
    } catch (err) {
        resultDiv.innerHTML = `<div class="sql-error">❌ Ошибка: ${escapeHTML(err.message)}</div>`
    } finally {
        resultDiv.classList.remove('loading')
    }

    loadTables()
}

function renderSelectResult(data, container) {
    if (!data.rows || data.rows.length === 0) {
        container.innerHTML = '<div class="sql-empty">📭 Нет данных</div>'
        return
    }

    let html = '<div class="sql-table-wrapper"><table class="sql-table">'
    html += '<thead><tr>'
    for (const col of data.columns) {
        html += `<th>${escapeHTML(col)}</th>`
    }
    html += '</tr></thead>'
    html += '<tbody>'
    for (const row of data.rows) {
        html += '<tr>'
        for (const cell of row) {
            const value = cell === null ? '<span class="sql-null">NULL</span>' : escapeHTML(String(cell))
            html += `<td>${value}</td>`
        }
        html += '</tr>'
    }
    html += '</tbody>'
    html += '</table></div>'
    html += `<div class="sql-stats">📊 ${data.rows.length} строк</div>`

    container.innerHTML = html
}

function renderInsertResult(data, container) {
    let html = '<div class="sql-success">✅ Вставка выполнена</div>'
    html += '<div class="sql-stats">'
    if (data.last_insert_id > 0) {
        html += `🆔 Последний ID: ${data.last_insert_id}<br>`
    }
    html += `📝 Затронуто строк: ${data.affected_rows || 1}`
    html += '</div>'
    container.innerHTML = html
}

function renderAffectedResult(data, container) {
    const typeName = data.type === 'UPDATE' ? 'Обновление' : 'Удаление'
    container.innerHTML = `
        <div class="sql-success">✅ ${typeName} выполнено</div>
        <div class="sql-stats">📝 Затронуто строк: ${data.affected_rows || 0}</div>
    `
}

function renderCreateResult(data, container) {
    container.innerHTML = `
        <div class="sql-success">✅ ${data.rows?.[0]?.[0] || 'Таблица создана'}</div>
    `
}

async function loadTables() {
    try {
        const data = await get('/api/tables')
        const list = document.getElementById('tables-list')
        if (data.tables && data.tables.length > 0) {
            list.innerHTML = data.tables.map(t =>
                `<div class="list-item" onclick="showTableData('${escapeHTML(t)}')">📄 ${escapeHTML(t)}</div>`
            ).join('')
        } else {
            list.innerHTML = '<div class="list-item muted">Нет таблиц</div>'
        }
    } catch (err) {
        console.error('Failed to load tables:', err)
    }
}

// ========== ПОКАЗ ДАННЫХ ТАБЛИЦЫ ==========

async function showTableData(tableName) {
    // Закрываем старое модальное окно
    closeTableModal()

    // Создаём модальное окно
    const modal = document.createElement('div')
    modal.className = 'table-modal'
    modal.id = 'table-modal'
    modal.innerHTML = `
        <div class="table-modal-header">
            <span>📋 ${escapeHTML(tableName)}</span>
            <button class="btn-icon" onclick="closeTableModal()">✕</button>
        </div>
        <div class="table-modal-body">
            <div class="loading-text">⏳ Загрузка данных...</div>
        </div>
        <div class="table-modal-footer">
            <button class="btn-sm" onclick="closeTableModal()">Закрыть</button>
            <button class="btn-sm btn-primary" onclick="refreshTableData()">🔄 Обновить</button>
        </div>
    `
    modal.dataset.tableName = tableName
    document.body.appendChild(modal)

    // Загружаем данные
    await loadTableData(tableName)
}

async function loadTableData(tableName) {
    const modal = document.getElementById('table-modal')
    if (!modal) return

    const body = modal.querySelector('.table-modal-body')
    if (!body) return

    try {
        // Пробуем получить данные
        const sql = `SELECT * FROM ${tableName} ORDER BY id DESC LIMIT 10`
        const data = await post('/api/query', { sql })

        if (data.error) {
            body.innerHTML = `<div class="sql-error">❌ ${escapeHTML(data.error)}</div>`
            return
        }

        if (data.type === 'SELECT' && data.rows && data.rows.length > 0) {
            let html = `<div class="sql-table-wrapper"><table class="sql-table">`
            html += '<thead><tr>'
            for (const col of data.columns) {
                html += `<th>${escapeHTML(col)}</th>`
            }
            html += '</tr></thead>'
            html += '<tbody>'
            for (const row of data.rows) {
                html += '<tr>'
                for (const cell of row) {
                    const value = cell === null ? '<span class="sql-null">NULL</span>' : escapeHTML(String(cell))
                    html += `<td>${value}</td>`
                }
                html += '</tr>'
            }
            html += '</tbody>'
            html += '</table></div>'
            html += `<div class="sql-stats">📊 ${data.rows.length} строк (последние 10)</div>`
            body.innerHTML = html
        } else {
            body.innerHTML = '<div class="sql-empty">📭 Таблица пуста</div>'
        }

    } catch (err) {
        body.innerHTML = `<div class="sql-error">❌ Ошибка: ${escapeHTML(err.message)}</div>`
    }
}

function closeTableModal() {
    const modal = document.getElementById('table-modal')
    if (modal) modal.remove()
}

async function refreshTableData() {
    const modal = document.getElementById('table-modal')
    if (!modal) return

    const tableName = modal.dataset.tableName
    if (!tableName) return

    const body = modal.querySelector('.table-modal-body')
    if (body) {
        body.innerHTML = '<div class="loading-text">⏳ Обновление...</div>'
    }

    await loadTableData(tableName)
}

// Глобальные функции
window.showTableData = showTableData
window.closeTableModal = closeTableModal
window.refreshTableData = refreshTableData