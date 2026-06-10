// static/sql.js — исправленная версия

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

        // Отображаем в зависимости от типа
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

    // Строим HTML-таблицу
    let html = '<div class="sql-table-wrapper"><table class="sql-table">'

    // Заголовки
    html += '<thead><tr>'
    for (const col of data.columns) {
        html += `<th>${escapeHTML(col)}</th>`
    }
    html += '</tr></thead>'

    // Тело
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
            list.innerHTML = data.tables.map(t => `<div class="list-item" onclick="loadTableSchema('${escapeHTML(t)}')">📄 ${escapeHTML(t)}</div>`).join('')
        } else {
            list.innerHTML = '<div class="list-item muted">Нет таблиц</div>'
        }
    } catch (err) {
        console.error('Failed to load tables:', err)
    }
}

async function loadTableSchema(tableName) {
    const sql = `SELECT * FROM ${tableName} LIMIT 0`
    try {
        const data = await post('/api/query', { sql })
        if (data.type === 'SELECT' && data.columns) {
            let html = `<div class="schema-modal">`
            html += `<div class="schema-header">📋 Схема таблицы ${escapeHTML(tableName)}</div>`
            html += `<div class="schema-body">`
            for (const col of data.columns) {
                html += `<div class="schema-col">📎 ${escapeHTML(col)}</div>`
            }
            html += `</div>`
            html += `<button class="btn-sm" onclick="document.querySelector('.schema-modal')?.remove()">Закрыть</button>`
            html += `</div>`

            // Добавляем модальное окно
            const existing = document.querySelector('.schema-modal')
            if (existing) existing.remove()
            document.body.appendChild(createElementFromHTML(html))
        }
    } catch (err) {
        console.error('Failed to load schema:', err)
    }
}

function createElementFromHTML(htmlString) {
    const div = document.createElement('div')
    div.innerHTML = htmlString.trim()
    return div.firstChild
}