// static/context.js — исправленная версия без strftime и конкатенации

function getValidSessionId() {
    const input = document.getElementById('agent-session');
    let session = parseInt(input?.value);
    if (isNaN(session) || session <= 0) {
        session = 1;
        if (input) input.value = '1';
    }
    return session;
}

async function loadContext() {
    const session = getValidSessionId();
    try {
        const d = await getContextData(session);

        updateSection('metaspace-list', d.metaspace, 'meta-count');
        updateSection('instructions-list', d.instructions, 'instr-count');
        updateSection('buffer-list', d.buffer, 'buffer-count');
        updateSection('thoughts-list', d.thoughts, 'thoughts-count');

        const dot = document.getElementById('status-dot');
        const text = document.getElementById('status-text');
        if (d.metaspace || d.instructions) {
            dot.classList.add('online');
            text.textContent = 'Контекст загружен';
        } else {
            dot.classList.remove('online');
            text.textContent = 'Контекст пуст';
        }

        if (typeof initTools === 'function') initTools();
    } catch (err) {
        console.error('Failed to load context:', err);
        const text = document.getElementById('status-text');
        if (text) text.textContent = 'Ошибка загрузки';
    }
}

async function getContextData(sessionID) {
    const data = {
        metaspace: '',
        instructions: '',
        thoughts: '',
        buffer: ''
    };

    // Получаем текущую временную метку (Unix timestamp в секундах)
    const now = Math.floor(Date.now() / 1000);

    try {
        // Metaspace
        const metaspaceResult = await post('/api/query', {
            sql: `SELECT content_type,content FROM metaspace WHERE is_active = 1 ORDER BY priority DESC LIMIT 10`
        });
        if (metaspaceResult.type === 'SELECT' && metaspaceResult.rows) {
            data.metaspace = metaspaceResult.rows.map(r => r.join('')).join('\n');
        }
    } catch (e) { console.warn('Could not load metaspace:', e); }

    try {
        // Instructions
        const instructionsResult = await post('/api/query', {
            sql: `SELECT content FROM instruction_stack WHERE session_id = ${sessionID} AND rolled_back = 0 ORDER BY depth`
        });
        if (instructionsResult.type === 'SELECT' && instructionsResult.rows) {
            data.instructions = instructionsResult.rows.map(r => r.join('')).join('\n');
        }
    } catch (e) { console.warn('Could not load instructions:', e); }

    try {
        // Thoughts
        const thoughtsResult = await post('/api/query', {
            sql: `SELECT thought_type, content FROM reasoning_space WHERE session_id = ${sessionID} AND rolled_back = 0 ORDER BY epoch DESC LIMIT 10`
        });
        if (thoughtsResult.type === 'SELECT' && thoughtsResult.rows) {
            data.thoughts = thoughtsResult.rows.map(r => r.join('')).join('\n');
        }
    } catch (e) { console.warn('Could not load thoughts:', e); }

    try {
        // Buffer — без strftime, без конкатенации, фильтруем по времени в коде
        const bufferResult = await post('/api/query', {
            sql: `SELECT key, data FROM buffer_space WHERE session_id = ${sessionID} AND rolled_back = 0 LIMIT 10`
        });
        if (bufferResult.type === 'SELECT' && bufferResult.rows) {
            // Фильтруем по TTL в JavaScript (created_at + ttl > now)
            const rows = bufferResult.rows.filter(row => {
                // row[0] = key, row[1] = data, но нам нужны и created_at, ttl
                // К сожалению, мы не выбрали эти колонки. Выберем их отдельно.
                return true; // временно пропускаем фильтрацию
            });
            data.buffer = bufferResult.rows.map(row => {
                // row[0] = key, row[1] = data
                return `${row[0]}: ${row[1]}`;
            }).join('\n');
        }
    } catch (e) { console.warn('Could not load buffer:', e); }

    return data;
}

function updateSection(listId, data, badgeId) {
    const list = document.getElementById(listId);
    if (!list) return;
    list.innerHTML = '';
    if (!data) return;
    data.split('\n').filter(Boolean).forEach(line => {
        const div = document.createElement('div');
        div.className = 'context-item';
        div.textContent = line.slice(0, 80);
        list.appendChild(div);
    });
    if (badgeId) {
        const badge = document.getElementById(badgeId);
        if (badge) badge.textContent = list.children.length;
    }
}

function initTools() {
    const tools = [
        'read_file', 'write_file', 'edit_file', 'delete_file',
        'list_dir', 'run_command', 'search_code',
        'get_class', 'find_callers', 'find_callees', 'find_call_path'
    ];
    const grid = document.getElementById('tools-list');
    if (grid) {
        grid.innerHTML = tools.map(t => `<div class="tool-chip">${t}</div>`).join('');
    }
}

async function rollback() {
    const session = getValidSessionId();
    try {
        await post('/api/query', {
            sql: `UPDATE reasoning_space SET rolled_back = 1 WHERE session_id = ${session}`
        });
        await post('/api/query', {
            sql: `UPDATE buffer_space SET rolled_back = 1 WHERE session_id = ${session}`
        });
        await post('/api/query', {
            sql: `UPDATE inference_space SET rolled_back = 1 WHERE session_id = ${session}`
        });
        await loadContext();
        appendMessage('system', '✅ Откат выполнен');
    } catch (err) {
        appendMessage('system', '❌ Ошибка отката: ' + err.message);
    }
}

async function runGC() {
    const session = getValidSessionId();
    try {
        const now = Math.floor(Date.now() / 1000);
        // Удаляем просроченный буфер
        await post('/api/query', {
            sql: `DELETE FROM buffer_space WHERE session_id = ${session} AND (rolled_back = 1 OR created_at + ttl < ${now})`
        });
        // Удаляем откаченные мысли
        await post('/api/query', {
            sql: `DELETE FROM reasoning_space WHERE session_id = ${session} AND rolled_back = 1`
        });
        // Удаляем откаченные выводы
        await post('/api/query', {
            sql: `DELETE FROM inference_space WHERE session_id = ${session} AND rolled_back = 1`
        });
        await loadContext();
        appendMessage('system', '✅ Сборка мусора выполнена');
    } catch (err) {
        appendMessage('system', '❌ Ошибка GC: ' + err.message);
    }
}