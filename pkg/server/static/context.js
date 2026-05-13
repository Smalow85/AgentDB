async function loadContext() {
    const session = parseInt(document.getElementById('agent-session').value)
    const d = await get(`/api/context/current?session_id=${session}`)

    updateSection('metaspace-list', d.metaspace, 'meta-count')
    updateSection('instructions-list', d.instructions, 'instr-count')
    updateSection('buffer-list', d.buffer, 'buffer-count')

    // Статус
    const dot = document.getElementById('status-dot')
    const text = document.getElementById('status-text')
    if (d.metaspace || d.instructions) {
        dot.classList.add('online')
        text.textContent = 'Контекст загружен'
    } else {
        dot.classList.remove('online')
        text.textContent = 'Контекст пуст'
    }

    // Инструменты
    initTools()
}

function updateSection(listId, data, badgeId) {
    const list = document.getElementById(listId)
    if (!list) return
    list.innerHTML = ''
    if (!data) return
    data.split('\n').filter(Boolean).forEach(line => {
        const div = document.createElement('div')
        div.className = 'context-item'
        div.textContent = line.slice(0, 80)
        list.appendChild(div)
    })
    if (badgeId) {
        document.getElementById(badgeId).textContent = list.children.length
    }
}

function initTools() {
    const tools = [
        'read_file', 'write_file', 'edit_file', 'delete_file',
        'list_dir', 'run_command', 'search_code',
        'get_class', 'find_callers', 'find_callees', 'find_call_path'
    ]
    const grid = document.getElementById('tools-list')
    grid.innerHTML = tools.map(t => `<div class="tool-chip">${t}</div>`).join('')
}

async function rollback() {
    const session = parseInt(document.getElementById('agent-session').value)
    await post('/api/context/rollback', { session_id: session, steps: 1 })
    loadContext()
}

async function runGC() {
    const session = parseInt(document.getElementById('agent-session').value)
    await post('/api/context/gc', { session_id: session, gc_type: 'minor' })
    loadContext()
}