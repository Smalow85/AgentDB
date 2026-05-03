// Дожидаемся загрузки DOM
document.addEventListener('DOMContentLoaded', function () {
    console.log('AgentDB UI initialized');

    // Привязываем кнопки
    document.getElementById('btn-execute').addEventListener('click', executeQuery);
    document.getElementById('btn-clear').addEventListener('click', clearResult);

    // Загружаем таблицы
    loadTables();

    // Горячие клавиши
    document.addEventListener('keydown', function (e) {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            executeQuery();
        }
    });
});

let queryHistory = [];

// Загрузка списка таблиц
async function loadTables() {
    try {
        console.log('Загружаем таблицы...');
        const response = await fetch('/api/tables');
        const data = await response.json();
        console.log('Таблицы:', data);
        renderTables(data.tables || []);
    } catch (err) {
        console.error('Ошибка загрузки таблиц:', err);
    }
}

// Отрисовка таблиц
function renderTables(tables) {
    const list = document.getElementById('tables-list');
    if (tables.length === 0) {
        list.innerHTML = '<div class="empty-state">Нет таблиц</div>';
        return;
    }
    list.innerHTML = tables.map(function (t) {
        return '<div class="table-item" onclick="showSchema(\'' + t + '\')">📄 ' + t + '</div>';
    }).join('');
}

// Показать схему таблицы
async function showSchema(tableName) {
    document.querySelectorAll('.table-item').forEach(function (el) {
        el.classList.remove('active');
    });
    event.target.classList.add('active');

    try {
        const response = await fetch('/api/schema?table=' + tableName);
        const data = await response.json();
        document.getElementById('schema-view').innerHTML = data.error || data.schema || 'Нет данных';
    } catch (err) {
        console.error('Ошибка загрузки схемы:', err);
    }
}

// Выполнить запрос
async function executeQuery() {
    const sqlInput = document.getElementById('sql-input');
    const sql = sqlInput.value.trim();

    console.log('Выполняем запрос:', sql);

    if (!sql) {
        alert('Введите SQL-запрос');
        return;
    }

    const status = document.getElementById('result-status');
    const output = document.getElementById('result-output');

    status.textContent = '⏳ Выполняется...';
    status.className = 'result-status';

    try {
        const response = await fetch('/api/query', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ sql: sql })
        });

        const data = await response.json();
        console.log('Результат:', data);

        if (data.error) {
            output.textContent = '❌ ' + data.error;
            status.textContent = 'Ошибка выполнения';
            status.className = 'result-status error';
        } else {
            output.textContent = data.result;
            status.textContent = '✅ Успешно';
            status.className = 'result-status success';
        }

        // История
        addToHistory(sql);

        // Обновляем таблицы если нужно
        if (sql.toUpperCase().includes('CREATE') || sql.toUpperCase().includes('DROP')) {
            loadTables();
        }
    } catch (err) {
        console.error('Ошибка:', err);
        output.textContent = '❌ Ошибка сети: ' + err.message;
        status.textContent = 'Ошибка соединения';
        status.className = 'result-status error';
    }
}

// История
function addToHistory(sql) {
    queryHistory = queryHistory.filter(function (h) { return h !== sql; });
    queryHistory.unshift(sql);
    if (queryHistory.length > 20) queryHistory.pop();
    renderHistory();
}

function renderHistory() {
    const list = document.getElementById('history-list');
    list.innerHTML = queryHistory.map(function (sql) {
        return '<div class="history-item" onclick="useQuery(\'' + escapeHtml(sql) + '\')">' + escapeHtml(sql) + '</div>';
    }).join('');
}

function useQuery(sql) {
    document.getElementById('sql-input').value = sql;
    executeQuery();
}

// Очистка
function clearResult() {
    document.getElementById('result-output').textContent = '';
    document.getElementById('result-status').textContent = '';
    document.getElementById('result-status').className = 'result-status';
}

// Экранирование
function escapeHtml(text) {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}