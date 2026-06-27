// static/graph.js — Исправленная версия с использованием путей как идентификаторов файлов
// ============================================================================

// ==================== КОНФИГУРАЦИЯ ====================
const CONFIG = {
    colors: {
        bg: '#1e1e2e',
        file: '#89b4fa',
        class: '#cba6f7',
        function: '#a6e3a1',
        text: '#cdd6f4',
        edge: 'rgba(69, 71, 90, 0.4)',
        edgeHighlight: '#89b4fa',
        dimOpacity: 0.08
    },
    physics: {
        repulsion: 800,          // было 200 – сильно увеличиваем
        linkDist: 150,           // было 100 – увеличиваем, чтобы узлы не слипались
        damping: 0.85,           // оставляем
        centerForce: 0.005       // было 0.05 – сильно уменьшаем, чтобы не тянуло к центру
    }
};

// ==================== ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ ====================
let canvas, ctx, tooltip;
let width, height;
let transform = { x: 0, y: 0, k: 1 };
let currentMode = 'files';
let activeFileId = null;
let nodes = [];
let links = [];
let hoveredNode = null;
let draggedNode = null;
let animationId = null;

// Данные с сервера
let graphData = { nodes: [], edges: [] };
let filters = {
    file: true,
    class: true,
    function: true,
    call: true
};
let currentDetailNodes = [];
let currentDetailEdges = [];


function applyFilters() {
    if (currentMode === 'files') {
        // В файловом режиме показываем только файлы и file-call рёбра
        // Фильтры на типы узлов не влияют, но мы можем показать/скрыть файлы? 
        // Лучше оставить без изменений.
        return;
    }
    if (currentMode === 'detail') {
        // Фильтруем узлы по типу
        const filteredNodes = currentDetailNodes.filter(n => filters[n.type] !== false);
        // Для рёбер фильтруем: оставляем только те, у которых оба конца присутствуют в отфильтрованных узлах
        const filteredNodeIds = new Set(filteredNodes.map(n => n.id));
        const filteredEdges = currentDetailEdges.filter(e =>
            filteredNodeIds.has(e.source) && filteredNodeIds.has(e.target)
        );
        buildGraph(filteredNodes, filteredEdges);
    }
}

// Обновить фильтры из UI
function updateFilters() {
    document.querySelectorAll('#filter-tags .tag').forEach(el => {
        const type = el.dataset.type;
        filters[type] = el.classList.contains('active');
    });
    applyFilters();
}

// ==================== ИНИЦИАЛИЗАЦИЯ ====================
function initGraph() {
    canvas = document.getElementById('graph-canvas');
    if (!canvas) {
        console.error('Canvas #graph-canvas not found');
        return;
    }
    ctx = canvas.getContext('2d');
    tooltip = document.getElementById('tooltip');
    if (!tooltip) {
        tooltip = document.createElement('div');
        tooltip.id = 'tooltip';
        tooltip.style.cssText = 'position:fixed;display:none;background:#313244;color:#cdd6f4;padding:8px 12px;border-radius:6px;font-size:13px;pointer-events:none;z-index:1000;border:1px solid #45475a;';
        document.body.appendChild(tooltip);
    }

    canvas.addEventListener('mousemove', onMouseMove);
    canvas.addEventListener('mousedown', onMouseDown);
    canvas.addEventListener('mouseup', onMouseUp);
    canvas.addEventListener('click', onClick);
    canvas.addEventListener('wheel', onWheel, { passive: false });
    window.addEventListener('resize', resize);

    resize();

    if (!animationId) {
        tick();
    }

    loadGraph();
}

// ==================== ЗАГРУЗКА ДАННЫХ С СЕРВЕРА ====================
async function loadGraph() {
    console.log('loadGraph() called');
    try {
        const response = await fetch('/api/graph');
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const data = await response.json();
        if (data.nodes && data.edges) {
            // Приводим рёбра к формату { source, target, type }
            const edges = data.edges.map(e => ({
                source: e.from !== undefined ? e.from : e.source,
                target: e.to !== undefined ? e.to : e.target,
                type: e.type || 'call'
            }));
            graphData.nodes = data.nodes;
            graphData.edges = edges;
            console.log('Данные загружены с сервера:', graphData.nodes.length, 'узлов,', graphData.edges.length, 'связей');
        } else {
            console.warn('Ответ сервера не содержит nodes/edges');
            graphData.nodes = [];
            graphData.edges = [];
        }
    } catch (err) {
        console.error('Ошибка загрузки графа:', err.message);
        graphData.nodes = [];
        graphData.edges = [];
    }

    enterFilesView();
}

// ==================== АГРЕГАЦИЯ СВЯЗЕЙ МЕЖДУ ФАЙЛАМИ ====================
function buildFileGraph() {
    // Собираем файловые узлы и маппинг путь -> id
    const fileMap = new Map(); // путь -> id
    const fileNodes = [];

    graphData.nodes.forEach(n => {
        if (n.type === 'file') {
            const path = n.props?.path;
            if (!path) return;
            fileMap.set(path, n.id);
            fileNodes.push(createNode(n));
        }
    });

    // Маппинг путь файла -> массив id узлов (классов/функций)
    const fileToNodes = new Map();
    graphData.nodes.forEach(n => {
        if (n.type !== 'file') {
            const filePath = n.props?.file;
            if (!filePath) return;
            if (!fileToNodes.has(filePath)) fileToNodes.set(filePath, []);
            fileToNodes.get(filePath).push(n.id);
        }
    });

    // Агрегируем рёбра между файлами (игнорируем contains)
    const fileEdgesMap = new Map();

    graphData.edges.forEach(e => {
        if (e.type === 'contains') return;

        const sourceNode = graphData.nodes.find(n => n.id === e.source);
        const targetNode = graphData.nodes.find(n => n.id === e.target);
        if (!sourceNode || !targetNode) return;

        let srcFilePath, tgtFilePath;
        if (sourceNode.type === 'file') {
            srcFilePath = sourceNode.props?.path;
        } else {
            srcFilePath = sourceNode.props?.file;
        }
        if (targetNode.type === 'file') {
            tgtFilePath = targetNode.props?.path;
        } else {
            tgtFilePath = targetNode.props?.file;
        }

        if (!srcFilePath || !tgtFilePath) return;
        if (srcFilePath === tgtFilePath) return;

        if (!fileMap.has(srcFilePath) || !fileMap.has(tgtFilePath)) return;

        const key = srcFilePath < tgtFilePath ? `${srcFilePath}|${tgtFilePath}` : `${tgtFilePath}|${srcFilePath}`;
        if (!fileEdgesMap.has(key)) {
            fileEdgesMap.set(key, {
                source: fileMap.get(srcFilePath),
                target: fileMap.get(tgtFilePath),
                type: 'file-call'
            });
        }
    });

    const fileEdges = Array.from(fileEdgesMap.values());
    buildGraph(fileNodes, fileEdges);
    console.log('Построен файловый граф: узлов', nodes.length, 'связей', links.length);
}

// ==================== ПЕРЕКЛЮЧЕНИЕ РЕЖИМОВ ====================
function enterFilesView() {
    console.log('enterFilesView()');
    currentMode = 'files';
    activeFileId = null;
    transform = { x: width / 2, y: height / 2, k: 1 };
    const backBtn = document.getElementById('graph-back-btn');
    if (backBtn) backBtn.style.display = 'none';
    buildFileGraph();
}

function enterFileView(fileNode) {
    currentMode = 'detail';
    activeFileId = fileNode.id;
    transform = { x: width / 2, y: height / 2, k: 1.2 };
    const backBtn = document.getElementById('graph-back-btn');
    if (backBtn) backBtn.style.display = 'block';

    const filePath = fileNode.props?.path;
    if (!filePath) {
        console.warn('Файл без пути');
        return;
    }

    // 1. Получаем все классы и функции этого файла
    const classAndFuncNodes = graphData.nodes
        .filter(n => n.props?.file === filePath && (n.type === 'class' || n.type === 'function'))
        .map(n => createNode(n));

    // 2. Корневой узел файла
    const root = createNode({ ...fileNode });
    const detailNodes = [root, ...classAndFuncNodes];

    // 3. Связи "содержит" от файла к каждому дочернему узлу
    const containmentEdges = classAndFuncNodes.map(c => ({
        source: root.id,
        target: c.id,
        type: 'contains'
    }));

    // 4. Построение связей между функциями (на основе вызовов)
    // Создаём маппинг: id узла -> его путь (для функций/классов)
    const nodeMap = new Map();
    graphData.nodes.forEach(n => {
        if (n.type === 'function' || n.type === 'class') {
            nodeMap.set(n.id, n);
        }
    });

    // Для каждой функции находим call-узлы внутри неё
    const functionCalls = new Map(); // functionId -> Set of targetFunctionId

    // Сначала собираем все call-узлы, которые принадлежат функциям
    graphData.edges.forEach(e => {
        if (e.type !== 'contains') return;
        const parent = graphData.nodes.find(n => n.id === e.from);
        const child = graphData.nodes.find(n => n.id === e.to);
        if (!parent || !child) return;
        if (parent.type === 'function' && child.type === 'call') {
            // Это call-узел внутри функции
            if (!functionCalls.has(parent.id)) functionCalls.set(parent.id, new Set());
            // Теперь нужно найти, куда ведёт этот call
            const callEdge = graphData.edges.find(ce => ce.from === child.id && ce.type === 'call');
            if (callEdge) {
                const targetNode = graphData.nodes.find(n => n.id === callEdge.to);
                if (targetNode && (targetNode.type === 'function' || targetNode.type === 'class')) {
                    functionCalls.get(parent.id).add(targetNode.id);
                }
            }
        }
    });

    // Создаём рёбра между функциями/классами
    const callEdges = [];
    functionCalls.forEach((targets, sourceId) => {
        targets.forEach(targetId => {
            // Проверяем, что оба узла есть в detailNodes (чтобы не добавлять связи к внешним узлам)
            const sourceInDetail = detailNodes.some(n => n.id === sourceId);
            const targetInDetail = detailNodes.some(n => n.id === targetId);
            if (sourceInDetail && targetInDetail) {
                callEdges.push({
                    source: sourceId,
                    target: targetId,
                    type: 'calls'
                });
            }
        });
    });

    // 5. Объединяем все рёбра
    const allEdges = [...containmentEdges, ...callEdges];
    // Также можно добавить оригинальные рёбра, которые напрямую соединяют функции (если есть)
    // Но в ваших данных таких нет, все через call-узлы.
    currentDetailNodes = detailNodes;
    currentDetailEdges = allEdges;
    applyFilters();
    console.log('Детальный граф: узлов', nodes.length, 'связей', links.length);
}

function exitFileView() {
    enterFilesView();
}

function goBack() {
    exitFileView();
}

// ==================== ЯДРО ГРАФА ====================
function createNode(raw) {
    return {
        id: raw.id,
        label: raw.label,
        type: raw.type,
        props: raw.props || {},
        x: (Math.random() - 0.5) * 100,
        y: (Math.random() - 0.5) * 100,
        vx: 0, vy: 0,
        neighbors: new Set(),
        links: []
    };
}

function buildGraph(newNodes, newEdges) {
    nodes = newNodes;
    links = [];
    const map = new Map(nodes.map(n => [n.id, n]));

    newEdges.forEach(e => {
        const s = map.get(e.source);
        const t = map.get(e.target);
        if (s && t) {
            const link = { source: s, target: t, type: e.type };
            links.push(link);
            s.neighbors.add(t.id);
            t.neighbors.add(s.id);
            s.links.push(link);
            t.links.push(link);
        } else {
            console.warn('Связь пропущена: неизвестный узел', e);
        }
    });

    nodes.forEach(n => { n.vx = 0; n.vy = 0; });
}

// ==================== ФИЗИКА И ОТРИСОВКА ====================
function tick() {
    if (!ctx) return;

    if (nodes.length > 0) {
        for (let i = 0; i < nodes.length; i++) {
            for (let j = i + 1; j < nodes.length; j++) {
                const a = nodes[i], b = nodes[j];
                let dx = b.x - a.x, dy = b.y - a.y;
                let dist = Math.sqrt(dx * dx + dy * dy) || 1;
                const rep = currentMode === 'detail'
                    ? CONFIG.physics.repulsion * (1 + nodes.length / 20) 
                    : CONFIG.physics.repulsion;
                const f = rep / (dist * dist);
                const fx = (dx / dist) * f, fy = (dy / dist) * f;
                a.vx -= fx; a.vy -= fy;
                b.vx += fx; b.vy += fy;
            }
        }

        links.forEach(l => {
            let dx = l.target.x - l.source.x;
            let dy = l.target.y - l.source.y;
            let dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const idealLen = l.type === 'contains' ? 80 : CONFIG.physics.linkDist;
            const stiffness = l.type === 'contains' ? 0.1 : 0.05;

            const disp = dist - idealLen;
            const f = disp * stiffness;
            const fx = (dx / dist) * f, fy = (dy / dist) * f;
            l.source.vx += fx; l.source.vy += fy;
            l.target.vx -= fx; l.target.vy -= fy;
        });

        nodes.forEach(n => {
            if (n === draggedNode) return;
            n.vx -= n.x * CONFIG.physics.centerForce;
            n.vy -= n.y * CONFIG.physics.centerForce;
            n.vx *= CONFIG.physics.damping;
            n.vy *= CONFIG.physics.damping;
            n.x += n.vx; n.y += n.vy;
        });
    }

    draw();
    animationId = requestAnimationFrame(tick);
}

function draw() {
    ctx.fillStyle = CONFIG.colors.bg;
    ctx.fillRect(0, 0, width, height);

    ctx.save();
    ctx.translate(transform.x, transform.y);
    ctx.scale(transform.k, transform.k);

    const isHL = !!hoveredNode;
    const hlSet = new Set();
    if (isHL) {
        hlSet.add(hoveredNode.id);
        hoveredNode.neighbors.forEach(id => hlSet.add(id));
    }

    links.forEach(l => {
        const related = isHL && (l.source.id === hoveredNode.id || l.target.id === hoveredNode.id);
        ctx.beginPath();
        ctx.moveTo(l.source.x, l.source.y);
        ctx.lineTo(l.target.x, l.target.y);

        if (isHL) {
            ctx.strokeStyle = related ? CONFIG.colors.edgeHighlight : CONFIG.colors.edge;
            ctx.globalAlpha = related ? 1 : CONFIG.colors.dimOpacity;
            ctx.lineWidth = related ? 2 : 1;
        } else {
            const isCall = l.type !== 'contains';
            ctx.strokeStyle = isCall ? 'rgba(137, 180, 250, 0.5)' : 'rgba(255,255,255,0.05)';
            ctx.globalAlpha = isCall ? 0.7 : 0.3;
            ctx.lineWidth = isCall ? 1.5 : 1;
        }
        ctx.stroke();

        if (l.type !== 'contains' && ctx.globalAlpha > 0.2) {
            drawArrow(l.source, l.target);
        }
    });

    nodes.forEach(n => {
        const related = !isHL || hlSet.has(n.id);
        ctx.globalAlpha = related ? 1 : CONFIG.colors.dimOpacity;

        let color = CONFIG.colors.file;
        let radius = 4;
        if (n.type === 'class') { color = CONFIG.colors.class; radius = 6; }
        if (n.type === 'function') { color = CONFIG.colors.function; radius = 3; }
        if (n === hoveredNode) { color = '#fff'; radius += 2; }

        ctx.beginPath();
        ctx.arc(n.x, n.y, radius, 0, Math.PI * 2);
        ctx.fillStyle = color;
        ctx.fill();

        ctx.fillStyle = CONFIG.colors.text;
        ctx.font = `${n.type === 'file' ? 'bold ' : ''}12px sans-serif`;
        ctx.textAlign = 'left';
        ctx.textBaseline = 'middle';
        ctx.fillText(n.label, n.x + radius + 6, n.y);
    });

    ctx.restore();
}

function drawArrow(s, t) {
    const head = 8;
    const dx = t.x - s.x, dy = t.y - s.y;
    const ang = Math.atan2(dy, dx);
    const dist = Math.sqrt(dx * dx + dy * dy);
    if (dist < 10) return;
    const ex = t.x - (dx / dist) * 6, ey = t.y - (dy / dist) * 6;

    ctx.beginPath();
    ctx.moveTo(ex, ey);
    ctx.lineTo(ex - head * Math.cos(ang - Math.PI / 6), ey - head * Math.sin(ang - Math.PI / 6));
    ctx.lineTo(ex - head * Math.cos(ang + Math.PI / 6), ey - head * Math.sin(ang + Math.PI / 6));
    ctx.closePath();
    ctx.fillStyle = ctx.strokeStyle || 'rgba(137, 180, 250, 0.9)';
    ctx.fill();
}

// ==================== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ====================
function resize() {
    if (!canvas) return;
    const container = canvas.parentElement;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    width = rect.width;
    height = rect.height;
    canvas.width = width;
    canvas.height = height;
    if (currentMode === 'files') {
        transform = { x: width / 2, y: height / 2, k: 1 };
    }
}

function screenToWorld(x, y) {
    return { x: (x - transform.x) / transform.k, y: (y - transform.y) / transform.k };
}

function findNode(wx, wy) {
    for (let i = nodes.length - 1; i >= 0; i--) {
        const n = nodes[i];
        const r = (n.type === 'class' ? 6 : n.type === 'file' ? 4 : 3) * 2;
        if ((n.x - wx) ** 2 + (n.y - wy) ** 2 < r * r) return n;
    }
    return null;
}

// ==================== ОБРАБОТЧИКИ СОБЫТИЙ ====================
function onMouseMove(e) {
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rect.left, e.clientY - rect.top);

    if (draggedNode) {
        draggedNode.x = w.x; draggedNode.y = w.y;
        draggedNode.vx = 0; draggedNode.vy = 0;
        return;
    }

    const found = findNode(w.x, w.y);
    if (found !== hoveredNode) {
        hoveredNode = found;
        canvas.style.cursor = found ? 'pointer' : 'default';
        if (tooltip) {
            tooltip.style.display = found ? 'block' : 'none';
            if (found) tooltip.innerHTML = `<b>${found.label}</b><br><span style="color:#6c7086">${found.type}</span>`;
        }
    }
    if (hoveredNode && tooltip) {
        tooltip.style.left = e.clientX + 15 + 'px';
        tooltip.style.top = e.clientY + 15 + 'px';
    }
}

function onMouseDown(e) {
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rect.left, e.clientY - rect.top);
    const found = findNode(w.x, w.y);
    if (found) {
        draggedNode = found;
    }
}

function onMouseUp() {
    draggedNode = null;
}

function onClick(e) {
    if (draggedNode) return;
    if (!hoveredNode) return;

    console.log('Click on node:', hoveredNode.type, hoveredNode.label);

    if (currentMode === 'files' && hoveredNode.type === 'file') {
        enterFileView(hoveredNode);
        return;
    }

    if (hoveredNode.type === 'class' || hoveredNode.type === 'function') {
        showCodeModal(hoveredNode);
        return;
    }

    // Для call-узлов, если есть данные
    if (hoveredNode.type === 'call' && (hoveredNode.props?.start_byte !== undefined)) {
        showCodeModal(hoveredNode);
    }
}

// ==================== ПОКАЗ КОДА ====================

// Кэш для кода
const codeCache = new Map();

/**
 * Показывает код для узла графа
 */
async function showCodeModal(node) {
    console.log('showCodeModal called with node:', node);

    // Проверяем, что узел существует и имеет props
    if (!node || typeof node !== 'object') {
        console.warn('Node is invalid', node);
        return;
    }

    // Определяем файл
    const file = node.props?.file || node.props?.path || node.file;
    if (!file) {
        console.warn('No file path found in node', node);
        return;
    }

    // Определяем позицию
    const startByte = node.props?.start_byte ?? node.start_byte;
    const endByte = node.props?.end_byte ?? node.end_byte;

    if (startByte === undefined || endByte === undefined) {
        console.warn('No byte range found for node', node);
        // Попробуем найти через родительский узел? Но пока просто выходим.
        return;
    }

    const cacheKey = `${file}:${startByte}:${endByte}`;
    let data = codeCache.get(cacheKey);

    if (!data) {
        try {
            const url = `/api/code?file=${encodeURIComponent(file)}&start_byte=${startByte}&end_byte=${endByte}`;
            console.log('Fetching code from:', url);
            const response = await fetch(url);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            data = await response.json();
            if (data.error) {
                alert('Ошибка загрузки кода: ' + data.error);
                return;
            }
            codeCache.set(cacheKey, data);
        } catch (err) {
            console.error('Error loading code:', err);
            alert('Не удалось загрузить код: ' + err.message);
            return;
        }
    }

    // Проверяем, что данные содержат код
    if (!data.code) {
        console.warn('No code in data', data);
        return;
    }

    renderCodeModal(data);
}

/**
 * Отрисовывает модальное окно с кодом
 */
function renderCodeModal(data) {
    // Удаляем старую модалку
    const existing = document.getElementById('code-modal');
    if (existing) {
        existing.style.opacity = '0';
        setTimeout(() => existing.remove(), 200);
    }

    // Определяем язык по расширению
    const ext = (data.file || '').split('.').pop().toLowerCase();
    const langMap = {
        'go': 'go',
        'js': 'javascript',
        'ts': 'typescript',
        'py': 'python',
        'java': 'java',
        'c': 'c',
        'cpp': 'cpp',
        'rs': 'rust',
        'rb': 'ruby',
        'php': 'php',
        'html': 'html',
        'css': 'css',
        'json': 'json',
        'yaml': 'yaml',
        'toml': 'toml',
        'md': 'markdown',
        'sh': 'bash',
        'sql': 'sql'
    };
    const lang = langMap[ext] || 'plaintext';

    // Подготовка кода
    const code = data.code || '';
    const lines = code.split('\n');
    const startLine = data.start_line || 1;
    const endLine = data.end_line || lines.length;
    const lineNumbers = lines.map((_, i) => i + startLine).join('\n');

    // Подсветка синтаксиса
    let highlighted = code;
    if (typeof hljs !== 'undefined') {
        try {
            highlighted = hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
        } catch (e) {
            console.warn('Highlight.js error:', e);
            highlighted = code;
        }
    } else {
        console.warn('Highlight.js not loaded');
    }

    // Создаём модалку
    const modal = document.createElement('div');
    modal.id = 'code-modal';
    modal.className = 'code-modal';
    modal.innerHTML = `
        <div class="code-modal-content">
            <div class="code-modal-header">
                <span class="code-modal-filename">📄 ${escapeHTML(data.file || '')}</span>
                <span class="code-modal-lines">Строки ${startLine}–${endLine}</span>
                <button class="code-modal-close" onclick="this.closest('.code-modal').remove()">✕</button>
            </div>
            <div class="code-modal-body">
                <div class="code-with-lines">
                    <div class="code-line-numbers">${escapeHTML(lineNumbers)}</div>
                    <pre><code class="hljs language-${lang}">${highlighted}</code></pre>
                </div>
            </div>
        </div>
    `;

    document.body.appendChild(modal);

    // Анимация появления
    requestAnimationFrame(() => {
        modal.style.opacity = '0';
        modal.style.transform = 'scale(0.95)';
        modal.style.transition = 'opacity 0.2s ease, transform 0.2s ease';
        requestAnimationFrame(() => {
            modal.style.opacity = '1';
            modal.style.transform = 'scale(1)';
        });
    });

    // Закрытие по клику на фон
    modal.addEventListener('click', (e) => {
        if (e.target === modal) {
            modal.style.opacity = '0';
            modal.style.transform = 'scale(0.95)';
            setTimeout(() => modal.remove(), 200);
        }
    });

    // Закрытие по Escape
    const onKeyDown = (e) => {
        if (e.key === 'Escape') {
            modal.style.opacity = '0';
            modal.style.transform = 'scale(0.95)';
            setTimeout(() => modal.remove(), 200);
            document.removeEventListener('keydown', onKeyDown);
        }
    };
    document.addEventListener('keydown', onKeyDown);
}

function onWheel(e) {
    e.preventDefault();
    if (!canvas) return;
    const d = e.deltaY > 0 ? 0.9 : 1.1;
    const nk = Math.max(0.1, Math.min(5, transform.k * d));
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    transform.x = mx - (mx - transform.x) * (nk / transform.k);
    transform.y = my - (my - transform.y) * (nk / transform.k);
    transform.k = nk;
}

// ==================== УПРАВЛЕНИЕ ВИДОМ ====================

function fitGraph() {
    if (nodes.length === 0) return;

    // Находим bounding box всех узлов
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const n of nodes) {
        if (n.x < minX) minX = n.x;
        if (n.x > maxX) maxX = n.x;
        if (n.y < minY) minY = n.y;
        if (n.y > maxY) maxY = n.y;
    }

    // Добавляем отступы (padding)
    const padding = 50;
    const bboxWidth = maxX - minX + padding * 2;
    const bboxHeight = maxY - minY + padding * 2;

    if (bboxWidth === 0 || bboxHeight === 0) {
        // Все узлы в одной точке – просто центрируем
        transform.x = width / 2;
        transform.y = height / 2;
        transform.k = 1;
        return;
    }

    // Вычисляем масштаб так, чтобы bbox влез в видимую область
    const scaleX = width / bboxWidth;
    const scaleY = height / bboxHeight;
    const scale = Math.min(scaleX, scaleY, 5); // ограничиваем максимальный zoom

    // Вычисляем центр bbox в мировых координатах
    const centerX = (minX + maxX) / 2;
    const centerY = (minY + maxY) / 2;

    // Устанавливаем трансформацию так, чтобы центр bbox оказался в центре экрана
    transform.x = width / 2 - centerX * scale;
    transform.y = height / 2 - centerY * scale;
    transform.k = scale;
}

function resetGraph() {
    // Сбрасываем трансформацию в центр с масштабом 1
    transform.x = width / 2;
    transform.y = height / 2;
    transform.k = 1;
}

// ==================== ЭКСПОРТ В ГЛОБАЛЬНУЮ ОБЛАСТЬ ====================
window.loadGraph = loadGraph;
window.renderCurrentView = draw;
window.enterFilesView = enterFilesView;
window.enterFileView = enterFileView;
window.exitFileView = exitFileView;
window.goBack = goBack;
window.fitGraph = fitGraph;
window.resetGraph = resetGraph;

// ==================== АВТОЗАПУСК ====================
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initGraph);
    document.addEventListener('DOMContentLoaded', () => {
        document.querySelectorAll('#filter-tags .tag').forEach(el => {
            el.addEventListener('click', function () {
                this.classList.toggle('active');
                updateFilters();
            });
        });
    });
} else {
    initGraph();
}

