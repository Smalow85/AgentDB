// static/app.js
document.addEventListener('DOMContentLoaded', () => {
    // Навигация
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.onclick = () => switchView(btn.dataset.view)
    })

    // Граф
    initGraph()
    setupFilters()
    setupSearch()

    // SQL
    document.getElementById('sql-input').addEventListener('keydown', e => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') executeSQL()
    })

    // Агент — поддержка Enter и кнопки остановки
    const agentInput = document.getElementById('agent-message')
    agentInput.addEventListener('keydown', e => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            e.stopPropagation()
            sendAgentMessage()
        }
    })

    // Кнопка остановки генерации
    const stopBtn = document.getElementById('agent-stop-btn')
    if (stopBtn) {
        stopBtn.onclick = stopAgentGeneration
    }

    loadContext()

    // ===== НОВЫЕ ВЫЗОВЫ =====
    loadProjects()    // ← ЗАГРУЖАЕМ ПРОЕКТЫ
    loadModels()      // ← ЗАГРУЖАЕМ МОДЕЛИ
    loadSettings()    // ← ЗАГРУЖАЕМ НАСТРОЙКИ

    // Сайдбар
    loadTables()

    // Настройка стриминга
    const streamingToggle = document.getElementById('streaming-toggle')
    if (streamingToggle) {
        streamingToggle.onchange = (e) => toggleStreaming(e.target.checked)
    }
})

function switchView(name) {
    document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'))
    document.querySelector(`.nav-btn[data-view="${name}"]`).classList.add('active')
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'))
    document.getElementById(`view-${name}`).classList.add('active')

    if (name === 'graph') {
        setTimeout(() => renderGraph(), 100)
    }
    if (name === 'agent') {
        loadContext()
    }
}

function toggleSidebar() {
    document.getElementById('sidebar').classList.toggle('collapsed')
}

function toggleContextPanel() {
    document.getElementById('context-panel').classList.toggle('collapsed')
}

function loadSettings() {
    const saved = localStorage.getItem('agentdb_settings')
    if (saved) {
        try {
            const settings = JSON.parse(saved)
            if (settings.session) document.getElementById('agent-session').value = settings.session
        } catch (e) { }
    }
}

function saveSettings() {
    const sessionEl = document.getElementById('agent-session')
    const settings = {
        session: sessionEl ? sessionEl.value : '1'
    }
    localStorage.setItem('agentdb_settings', JSON.stringify(settings))
}

// Автосохранение настроек
setInterval(saveSettings, 5000)

async function parseRepo() {
    const path = document.getElementById('repo-path').value
    const statusSpan = document.getElementById('parse-status')

    if (!path) {
        statusSpan.textContent = '❌ Укажите путь'
        return
    }

    statusSpan.textContent = '⏳ Анализируем...'

    try {
        const result = await post('/api/parse', { path })
        if (result.error) {
            statusSpan.textContent = '❌ ' + result.error
        } else {
            statusSpan.textContent = `✅ ${result.files} файлов, ${result.classes} классов, ${result.functions} функций (${result.time_ms}ms)`
            setTimeout(() => loadGraph(), 1000)
        }
    } catch (err) {
        statusSpan.textContent = '❌ Ошибка: ' + err.message
    }
}