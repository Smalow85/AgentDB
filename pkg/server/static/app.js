document.addEventListener('DOMContentLoaded', () => {
    // Навигация
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.onclick = () => switchView(btn.dataset.view)
    })

    // Граф
    initGraph()

    // SQL
    document.getElementById('sql-input').addEventListener('keydown', e => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') executeSQL()
    })

    // Агент
    document.getElementById('agent-message').addEventListener('keydown', e => {
        if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendAgentMessage() }
    })

    // Контекст — загружаем при старте
    loadContext()

    // Сайдбар
    loadTables()
})

function switchView(name) {
    document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'))
    document.querySelector(`.nav-btn[data-view="${name}"]`).classList.add('active')
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'))
    document.getElementById(`view-${name}`).classList.add('active')

    if (name === 'graph') renderGraph()
    if (name === 'agent') loadContext()
}

function toggleSidebar() {
    document.getElementById('sidebar').classList.toggle('collapsed')
}

function toggleContextPanel() {
    document.getElementById('context-panel').classList.toggle('collapsed')
}