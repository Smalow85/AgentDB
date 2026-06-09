// static/app.js — исправленная версия

let settingsSaveTimeout = null;

document.addEventListener('DOMContentLoaded', () => {
    // Навигация
    document.querySelectorAll('.nav-btn').forEach(btn => {
        btn.onclick = () => switchView(btn.dataset.view);
    });

    // Граф — новая инициализация
    if (document.getElementById('graph-canvas')) {
        initGraph();
        setupFilters();
        setupSearch();
    }

    // Кнопка "Назад" для графа
    const backBtn = document.getElementById('graph-back-btn');
    if (backBtn) {
        backBtn.onclick = () => goBack();
    }

    // SQL
    const sqlInput = document.getElementById('sql-input');
    if (sqlInput) {
        sqlInput.addEventListener('keydown', e => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') executeSQL();
        });
    }

    // Агент — поддержка Enter и кнопки остановки
    const agentInput = document.getElementById('agent-message');
    if (agentInput) {
        agentInput.addEventListener('keydown', e => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                e.stopPropagation();
                sendAgentMessage();
            }
        });
    }

    // Кнопка остановки генерации
    const stopBtn = document.getElementById('agent-stop-btn');
    if (stopBtn) {
        stopBtn.onclick = stopAgentGeneration;
    }

    // Загрузка контекста и данных
    loadContext();
    loadProjects();
    loadModels();
    loadSettings();
    loadTables();

    // Настройка стриминга
    const streamingToggle = document.getElementById('streaming-toggle');
    if (streamingToggle) {
        streamingToggle.onchange = (e) => toggleStreaming(e.target.checked);
    }

    // Сохраняем настройки при изменении сессии
    const sessionInput = document.getElementById('agent-session');
    if (sessionInput) {
        sessionInput.addEventListener('change', saveSettings);
        sessionInput.addEventListener('input', saveSettings);
    }
});

function switchView(name) {
    document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
    document.querySelector(`.nav-btn[data-view="${name}"]`).classList.add('active');
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
    document.getElementById(`view-${name}`).classList.add('active');

    if (name === 'graph') {
        // Обновляем граф при переключении
        setTimeout(() => {
            if (typeof renderCurrentView === 'function') {
                renderCurrentView();
            } else if (typeof loadGraph === 'function') {
                loadGraph();
            }
        }, 100);
    }
    if (name === 'agent') {
        loadContext();
    }
}

function toggleSidebar() {
    const sidebar = document.getElementById('sidebar');
    if (sidebar) sidebar.classList.toggle('collapsed');
}

function toggleContextPanel() {
    const panel = document.getElementById('context-panel');
    if (panel) panel.classList.toggle('collapsed');
}

function saveSettings() {
    if (settingsSaveTimeout) clearTimeout(settingsSaveTimeout);
    settingsSaveTimeout = setTimeout(() => {
        const sessionEl = document.getElementById('agent-session');
        if (sessionEl) {
            const settings = { session: sessionEl.value };
            localStorage.setItem('agentdb_settings', JSON.stringify(settings));
        }
    }, 1000);
}

function loadSettings() {
    const saved = localStorage.getItem('agentdb_settings');
    if (saved) {
        try {
            const settings = JSON.parse(saved);
            const sessionEl = document.getElementById('agent-session');
            if (settings.session && sessionEl) {
                sessionEl.value = settings.session;
            }
        } catch (e) {
            console.error('Failed to load settings:', e);
        }
    }
}

async function parseRepo() {
    const pathInput = document.getElementById('repo-path');
    const path = pathInput?.value;
    const statusSpan = document.getElementById('parse-status');

    if (!path) {
        if (statusSpan) statusSpan.textContent = '❌ Укажите путь';
        return;
    }

    if (statusSpan) statusSpan.textContent = '⏳ Анализируем...';

    try {
        const result = await post('/api/parse', { path });
        if (result.error) {
            if (statusSpan) statusSpan.textContent = '❌ ' + result.error;
        } else {
            if (statusSpan) {
                statusSpan.textContent = `✅ ${result.files} файлов, ${result.classes} классов, ${result.functions} функций (${result.time_ms}ms)`;
            }
            // Перезагружаем граф
            setTimeout(() => {
                if (typeof loadGraph === 'function') {
                    loadGraph();
                }
            }, 500);
        }
    } catch (err) {
        console.error('Parse error:', err);
        if (statusSpan) statusSpan.textContent = '❌ Ошибка: ' + err.message;
    }
}

// Функция для обновления UI при смене представления графа
function updateGraphUI() {
    const backBtn = document.getElementById('graph-back-btn');
    if (backBtn && typeof getNavigationStackLength === 'function') {
        // Можно добавить логику обновления кнопки
    }
}