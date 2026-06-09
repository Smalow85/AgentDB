// static/graph.js — полная версия с группировкой по пакетам и ограниченной анимацией

let graphData = null;
let rawNodes = [];
let rawEdges = [];

let navigationStack = [];
let currentView = {
    level: 'files',
    selectedId: null,
    selectedName: null
};

let canvas = null;
let ctx = null;

let scale = 1;
let pan = { x: 0, y: 0 };
let isPanning = false;
let panStart = { x: 0, y: 0 };

let hoveredNode = null;
let tooltipDiv = null;

let fileTree = new Map();
let simulationTimeout = null;
let isSimulating = false;

// Obsidian цвета
const COLORS = {
    bg: '#1e1e2e',
    file: { main: '#89b4fa', glow: 'rgba(137, 180, 250, 0.4)' },
    class: { main: '#cba6f7', glow: 'rgba(203, 166, 247, 0.4)' },
    function: { main: '#a6e3a1', glow: 'rgba(166, 227, 161, 0.4)' },
    call: { main: '#f9e2af', glow: 'rgba(249, 226, 175, 0.4)' },
    edge: '#313244',
    edgeActive: '#89b4fa'
};

// ========== ИНИЦИАЛИЗАЦИЯ ==========
function initGraph() {
    canvas = document.getElementById('graph-canvas');
    if (!canvas) return;

    ctx = canvas.getContext('2d');
    resizeCanvas();

    window.addEventListener('resize', () => {
        resizeCanvas();
        if (currentView.level === 'files') {
            initForceLayout();
            renderFilesView();
        } else {
            renderCurrentView();
        }
    });

    canvas.addEventListener('click', onCanvasClick);
    canvas.addEventListener('mousemove', onCanvasMouseMove);
    canvas.addEventListener('mouseleave', () => hideTooltip());
    canvas.addEventListener('mousedown', onMouseDown);
    canvas.addEventListener('mouseup', onMouseUp);
    canvas.addEventListener('wheel', onWheel, { passive: false });
    canvas.addEventListener('contextmenu', e => e.preventDefault());

    loadGraph();
}

function resizeCanvas() {
    const container = canvas.parentElement;
    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;
}

// ========== ЗАГРУЗКА ДАННЫХ ==========
async function loadGraph() {
    try {
        const data = await get('/api/graph');
        rawNodes = data.nodes || [];
        rawEdges = data.edges || [];

        console.log('=== GRAPH LOADED ===');
        console.log('Files:', rawNodes.filter(n => n.type === 'file').length);
        console.log('Classes:', rawNodes.filter(n => n.type === 'class').length);
        console.log('Functions:', rawNodes.filter(n => n.type === 'function').length);

        buildFileStructure();
        initForceLayout();
        startSimulation(1500);
        updateStats();
        updateBackButton();
    } catch (e) {
        console.error('Graph load error:', e);
        showError('Не удалось загрузить граф');
    }
}

// ========== ПОСТРОЕНИЕ СТРУКТУРЫ ==========
function buildFileStructure() {
    fileTree.clear();

    const nodeToFile = new Map();

    rawEdges.forEach(edge => {
        if (edge.type === 'contains') {
            const fromNode = rawNodes.find(n => n.id === edge.from);
            const toNode = rawNodes.find(n => n.id === edge.to);

            if (fromNode?.type === 'file') nodeToFile.set(edge.to, edge.from);
            if (toNode?.type === 'file') nodeToFile.set(edge.from, edge.to);
        }
    });

    // Создаём файлы с определением пакета
    rawNodes.forEach(node => {
        if (node.type === 'file') {
            const filePath = node.props?.path || node.label;
            const pathParts = filePath.split('/');
            let packageName = 'root';
            if (pathParts.length >= 2) {
                packageName = pathParts.slice(0, 2).join('/');
            } else if (pathParts.length === 1) {
                packageName = pathParts[0];
            }

            fileTree.set(node.id, {
                id: node.id,
                name: filePath.split('/').pop(),
                fullPath: filePath,
                package: packageName,
                type: 'file',
                classes: [],
                functions: [],
                calls: [],
                x: 0, y: 0,
                vx: 0, vy: 0,
                targetX: 0, targetY: 0
            });
        }
    });

    // Распределяем классы и функции
    rawNodes.forEach(node => {
        if (node.type === 'file') return;

        const fileId = nodeToFile.get(node.id);
        const targetFile = fileTree.get(fileId);

        if (targetFile) {
            const nodeName = node.label || node.props?.name || `node_${node.id}`;

            if (node.type === 'class') {
                if (!targetFile.classes.find(c => c.id === node.id)) {
                    targetFile.classes.push({
                        id: node.id,
                        name: nodeName,
                        type: 'class',
                        x: 0, y: 0
                    });
                }
            } else if (node.type === 'function') {
                if (!targetFile.functions.find(f => f.id === node.id)) {
                    targetFile.functions.push({
                        id: node.id,
                        name: nodeName,
                        type: 'function',
                        calls: [],
                        callers: [],
                        x: 0, y: 0
                    });
                }
            }
        }
    });

    // Связываем вызовы
    rawEdges.forEach(edge => {
        if (edge.type !== 'call') return;

        for (const file of fileTree.values()) {
            const fromFunc = file.functions.find(f => f.id === edge.from);
            if (fromFunc) {
                const toNode = rawNodes.find(n => n.id === edge.to);
                const toName = toNode?.label || `node_${edge.to}`;
                if (!fromFunc.calls.find(c => c.id === edge.to)) {
                    fromFunc.calls.push({ id: edge.to, name: toName });
                }
            }

            const toFunc = file.functions.find(f => f.id === edge.to);
            if (toFunc) {
                const fromNode = rawNodes.find(n => n.id === edge.from);
                const fromName = fromNode?.label || `node_${edge.from}`;
                if (!toFunc.callers.find(c => c.id === edge.from)) {
                    toFunc.callers.push({ id: edge.from, name: fromName });
                }
            }
        }
    });

    // Удаляем пустые файлы
    for (const [id, file] of fileTree) {
        if (file.classes.length === 0 && file.functions.length === 0) {
            fileTree.delete(id);
        }
    }

    updateStats();
}

// ========== ГРУППИРОВКА ПО ПАКЕТАМ ==========
function initForceLayout() {
    const files = Array.from(fileTree.values());
    if (files.length === 0) return;

    const packages = new Map();
    files.forEach(file => {
        if (!packages.has(file.package)) {
            packages.set(file.package, []);
        }
        packages.get(file.package).push(file);
    });

    const packageCount = packages.size;
    const centerX = canvas.width / 2;
    const centerY = canvas.height / 2;
    const packageRadius = Math.min(canvas.width, canvas.height) * 0.4;

    let packageIndex = 0;
    for (const [pkgName, pkgFiles] of packages) {
        const packageAngle = (packageIndex / packageCount) * Math.PI * 2;
        const packageCenterX = centerX + Math.cos(packageAngle) * packageRadius;
        const packageCenterY = centerY + Math.sin(packageAngle) * packageRadius;

        const fileRadius = Math.min(70, 30 + pkgFiles.length * 3);
        pkgFiles.forEach((file, i) => {
            const fileAngle = (i / pkgFiles.length) * Math.PI * 2;
            const x = packageCenterX + Math.cos(fileAngle) * fileRadius;
            const y = packageCenterY + Math.sin(fileAngle) * fileRadius;

            file.targetX = x;
            file.targetY = y;
            file.x = x + (Math.random() - 0.5) * 30;
            file.y = y + (Math.random() - 0.5) * 30;
            file.vx = 0;
            file.vy = 0;
        });

        packageIndex++;
    }
}

function startSimulation(duration = 1500) {
    if (simulationTimeout) clearTimeout(simulationTimeout);
    if (isSimulating) return;

    isSimulating = true;
    const startTime = performance.now();

    function animate(currentTime) {
        const elapsed = currentTime - startTime;
        const progress = Math.min(1, elapsed / duration);

        // Плавное затухание силы
        const damping = 1 - progress * 0.3;
        updateForceSimulation(damping);
        renderFilesView();

        if (progress < 1) {
            requestAnimationFrame(animate);
        } else {
            isSimulating = false;
            relaxNodes();
            renderFilesView();
        }
    }

    requestAnimationFrame(animate);
}

function updateForceSimulation(damping = 1) {
    const files = Array.from(fileTree.values());
    if (files.length === 0) return;

    // Пружинная сила к целевой позиции
    files.forEach(file => {
        if (file.targetX !== undefined) {
            const dx = file.targetX - file.x;
            const dy = file.targetY - file.y;
            file.vx += dx * 0.08 * damping;
            file.vy += dy * 0.08 * damping;
        }
    });

    // Отталкивание между файлами
    for (let i = 0; i < files.length; i++) {
        for (let j = i + 1; j < files.length; j++) {
            const dx = files[i].x - files[j].x;
            const dy = files[i].y - files[j].y;
            const dist = Math.hypot(dx, dy) || 1;
            const minDist = 100;

            if (dist < minDist) {
                const force = (minDist - dist) * 0.04 * damping;
                const angle = Math.atan2(dy, dx);
                files[i].vx += Math.cos(angle) * force;
                files[i].vy += Math.sin(angle) * force;
                files[j].vx -= Math.cos(angle) * force;
                files[j].vy -= Math.sin(angle) * force;
            }
        }
    }

    // Затухание и обновление позиций
    files.forEach(file => {
        file.vx *= 0.92;
        file.vy *= 0.92;

        const maxSpeed = 3;
        file.vx = Math.min(maxSpeed, Math.max(-maxSpeed, file.vx));
        file.vy = Math.min(maxSpeed, Math.max(-maxSpeed, file.vy));

        file.x += file.vx;
        file.y += file.vy;

        const margin = 60;
        file.x = Math.max(margin, Math.min(canvas.width - margin, file.x));
        file.y = Math.max(margin, Math.min(canvas.height - margin, file.y));
    });
}

function relaxNodes() {
    const files = Array.from(fileTree.values());
    if (files.length === 0) return;

    for (let iter = 0; iter < 50; iter++) {
        for (let i = 0; i < files.length; i++) {
            for (let j = i + 1; j < files.length; j++) {
                const dx = files[i].x - files[j].x;
                const dy = files[i].y - files[j].y;
                const dist = Math.hypot(dx, dy) || 1;
                const minDist = 110;
                if (dist < minDist) {
                    const angle = Math.atan2(dy, dx);
                    const move = (minDist - dist) * 0.2;
                    files[i].x += Math.cos(angle) * move;
                    files[i].y += Math.sin(angle) * move;
                    files[j].x -= Math.cos(angle) * move;
                    files[j].y -= Math.sin(angle) * move;
                }
            }
        }
    }
}

// ========== ОТРИСОВКА УРОВНЯ ФАЙЛОВ ==========
function renderFilesView() {
    if (!ctx || !canvas) return;

    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.save();
    ctx.translate(pan.x, pan.y);
    ctx.scale(scale, scale);

    drawGrid();
    drawPackageConnections();
    drawFileDependencies();

    const files = Array.from(fileTree.values());
    files.forEach(file => drawObsidianNode(file));

    ctx.restore();
}

function drawGrid() {
    const step = 50;
    ctx.strokeStyle = '#313244';
    ctx.lineWidth = 0.5;
    ctx.globalAlpha = 0.3;

    for (let x = 0; x < canvas.width; x += step) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, canvas.height);
        ctx.stroke();
    }

    for (let y = 0; y < canvas.height; y += step) {
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(canvas.width, y);
        ctx.stroke();
    }

    ctx.globalAlpha = 1;
}

function drawPackageConnections() {
    const files = Array.from(fileTree.values());
    const packages = new Map();

    files.forEach(file => {
        if (!packages.has(file.package)) {
            packages.set(file.package, []);
        }
        packages.get(file.package).push(file);
    });

    for (const [pkgName, pkgFiles] of packages) {
        // Связи между файлами пакета
        if (pkgFiles.length >= 2) {
            for (let i = 0; i < pkgFiles.length; i++) {
                for (let j = i + 1; j < pkgFiles.length; j++) {
                    ctx.beginPath();
                    ctx.moveTo(pkgFiles[i].x, pkgFiles[i].y);
                    ctx.lineTo(pkgFiles[j].x, pkgFiles[j].y);
                    ctx.strokeStyle = '#45475a';
                    ctx.lineWidth = 0.8;
                    ctx.globalAlpha = 0.25;
                    ctx.stroke();
                }
            }
        }

        // Область пакета
        if (pkgFiles.length >= 2) {
            let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
            pkgFiles.forEach(f => {
                minX = Math.min(minX, f.x);
                minY = Math.min(minY, f.y);
                maxX = Math.max(maxX, f.x);
                maxY = Math.max(maxY, f.y);
            });

            const padding = 45;
            minX -= padding;
            minY -= padding;
            maxX += padding;
            maxY += padding;

            ctx.beginPath();
            ctx.roundRect(minX, minY, maxX - minX, maxY - minY, 12);
            ctx.fillStyle = `rgba(137, 180, 250, 0.04)`;
            ctx.fill();
            ctx.strokeStyle = `rgba(137, 180, 250, 0.12)`;
            ctx.lineWidth = 1;
            ctx.stroke();

            ctx.font = '10px monospace';
            ctx.fillStyle = '#6c7086';
            ctx.fillText(pkgName, minX + 8, minY + 16);
        }
    }
    ctx.globalAlpha = 1;
}

function drawFileDependencies() {
    const files = Array.from(fileTree.values());
    const fileMap = new Map(files.map(f => [f.id, f]));
    const dependencies = new Map();

    rawEdges.forEach(edge => {
        if (edge.type !== 'call') return;

        const fromNode = rawNodes.find(n => n.id === edge.from);
        const toNode = rawNodes.find(n => n.id === edge.to);

        if (fromNode && toNode) {
            let fromFile = null, toFile = null;
            for (const file of files) {
                if (file.functions.find(f => f.id === edge.from)) fromFile = file;
                if (file.functions.find(f => f.id === edge.to)) toFile = file;
            }
            if (fromFile && toFile && fromFile.id !== toFile.id) {
                const key = `${fromFile.id}|${toFile.id}`;
                dependencies.set(key, (dependencies.get(key) || 0) + 1);
            }
        }
    });

    for (const [key, weight] of dependencies) {
        const [fromId, toId] = key.split('|');
        const fromFile = fileMap.get(Number(fromId));
        const toFile = fileMap.get(Number(toId));

        if (fromFile && toFile) {
            const opacity = Math.min(0.3, 0.08 + weight * 0.015);
            ctx.beginPath();
            ctx.moveTo(fromFile.x, fromFile.y);
            ctx.lineTo(toFile.x, toFile.y);
            ctx.strokeStyle = COLORS.edge;
            ctx.lineWidth = 1 + weight * 0.2;
            ctx.globalAlpha = opacity;
            ctx.stroke();
        }
    }
    ctx.globalAlpha = 1;
}

function drawObsidianNode(node) {
    const size = Math.min(80, 30 + (node.classes.length + node.functions.length) * 1.5);
    const isHovered = hoveredNode?.id === node.id;
    const color = COLORS.file;

    if (isHovered) {
        ctx.shadowBlur = 20;
        ctx.shadowColor = color.glow;
    }

    ctx.beginPath();
    ctx.arc(node.x, node.y, size / 2, 0, Math.PI * 2);
    ctx.fillStyle = color.main;
    ctx.fill();

    ctx.strokeStyle = isHovered ? '#ffffff' : '#45475a';
    ctx.lineWidth = isHovered ? 3 : 1.5;
    ctx.stroke();

    ctx.shadowBlur = 0;

    ctx.font = `${Math.floor(size * 0.3)}px "Segoe UI Emoji"`;
    ctx.fillStyle = '#1e1e2e';
    ctx.fillText('📄', node.x - size / 4, node.y + size / 6);

    ctx.font = `${Math.max(10, Math.floor(size * 0.18))}px "Inter", sans-serif`;
    ctx.fillStyle = '#1e1e2e';
    ctx.textAlign = 'center';
    let displayName = node.name;
    if (displayName.length > 12) displayName = displayName.slice(0, 9) + '...';
    ctx.fillText(displayName, node.x + 5, node.y + 5);

    if (node.classes.length + node.functions.length > 0) {
        ctx.font = '9px sans-serif';
        ctx.fillStyle = '#1e1e2e';
        ctx.fillText(`● ${node.classes.length + node.functions.length}`, node.x + 5, node.y + 22);
    }
}

// ========== УРОВЕНЬ: ФАЙЛ И ЕГО СОДЕРЖИМОЕ ==========
function renderClassesView(fileId) {
    const file = fileTree.get(fileId);
    if (!file) {
        showEmptyState('Файл не найден');
        return;
    }

    currentView = { level: 'classes', selectedId: fileId, selectedName: file.name };
    navigationStack.push({ level: 'files', selectedId: null });
    updateBackButton();

    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.save();
    ctx.translate(pan.x, pan.y);
    ctx.scale(scale, scale);

    drawGrid();

    const centerX = (canvas.width / scale) / 2;
    const centerY = (canvas.height / scale) / 2;
    const radius = Math.min(canvas.width / scale, canvas.height / scale) * 0.3;

    // Центральный узел — файл
    drawFileCenterNode(centerX, centerY, file);

    const allItems = [...file.classes, ...file.functions];

    if (allItems.length === 0) {
        showEmptyState('Нет классов или функций в этом файле');
        ctx.restore();
        return;
    }

    allItems.forEach((item, i) => {
        const angle = (i / allItems.length) * Math.PI * 2;
        const x = centerX + Math.cos(angle) * radius;
        const y = centerY + Math.sin(angle) * radius;
        item.x = x;
        item.y = y;

        drawObsidianItemNode(item);

        ctx.beginPath();
        ctx.moveTo(centerX, centerY);
        ctx.lineTo(x, y);
        ctx.strokeStyle = COLORS.edge;
        ctx.lineWidth = 1;
        ctx.setLineDash([5, 5]);
        ctx.stroke();
        ctx.setLineDash([]);
    });

    ctx.restore();
}

function drawFileCenterNode(x, y, file) {
    const size = 70;
    const isHovered = hoveredNode?.id === file.id;

    if (isHovered) {
        ctx.shadowBlur = 20;
        ctx.shadowColor = COLORS.file.glow;
    }

    ctx.beginPath();
    ctx.arc(x, y, size / 2, 0, Math.PI * 2);
    ctx.fillStyle = COLORS.file.main;
    ctx.fill();

    ctx.strokeStyle = isHovered ? '#ffffff' : '#45475a';
    ctx.lineWidth = isHovered ? 3 : 2;
    ctx.stroke();

    ctx.shadowBlur = 0;

    ctx.font = '24px "Segoe UI Emoji"';
    ctx.fillStyle = '#1e1e2e';
    ctx.fillText('📁', x - 12, y + 8);

    ctx.font = 'bold 12px "Inter", sans-serif';
    ctx.fillStyle = '#1e1e2e';
    ctx.textAlign = 'center';
    let displayName = file.name;
    if (displayName.length > 15) displayName = displayName.slice(0, 12) + '...';
    ctx.fillText(displayName, x + 15, y + 5);

    ctx.font = '9px sans-serif';
    ctx.fillStyle = '#1e1e2e';
    ctx.fillText(`📦${file.classes.length} 🔧${file.functions.length}`, x + 15, y + 22);
}

function drawObsidianItemNode(item) {
    const isClass = item.type === 'class';
    const isHovered = hoveredNode?.id === item.id;
    const size = isClass ? 55 : 45;
    const color = isClass ? COLORS.class : COLORS.function;

    if (isHovered) {
        ctx.shadowBlur = 15;
        ctx.shadowColor = color.glow;
    }

    ctx.beginPath();
    ctx.arc(item.x, item.y, size / 2, 0, Math.PI * 2);
    ctx.fillStyle = color.main;
    ctx.fill();

    ctx.strokeStyle = isHovered ? '#ffffff' : '#45475a';
    ctx.lineWidth = isHovered ? 2 : 1;
    ctx.stroke();

    ctx.shadowBlur = 0;

    ctx.font = `${Math.floor(size * 0.3)}px "Segoe UI Emoji"`;
    ctx.fillStyle = '#1e1e2e';
    ctx.fillText(isClass ? '📦' : '⚡', item.x - size / 4, item.y + size / 6);

    ctx.font = `${Math.max(9, Math.floor(size * 0.18))}px "Inter", sans-serif`;
    ctx.fillStyle = '#1e1e2e';
    ctx.textAlign = 'center';
    let displayName = item.name;
    if (displayName.length > 12) displayName = displayName.slice(0, 9) + '...';
    ctx.fillText(displayName, item.x + 5, item.y + 5);
}

// ========== УРОВЕНЬ: ГРАФ ВЫЗОВОВ ==========
function renderCallGraph(functionId) {
    let targetFunc = null;
    let parentFile = null;

    for (const file of fileTree.values()) {
        const func = file.functions.find(f => f.id === functionId);
        if (func) {
            targetFunc = func;
            parentFile = file;
            break;
        }
    }

    if (!targetFunc) {
        showError('Функция не найдена');
        return;
    }

    currentView = { level: 'calls', selectedId: functionId, selectedName: targetFunc.name };
    navigationStack.push({ level: 'classes', selectedId: parentFile.id });
    updateBackButton();

    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.save();
    ctx.translate(pan.x, pan.y);
    ctx.scale(scale, scale);

    drawGrid();

    const centerX = (canvas.width / scale) / 2;
    const centerY = (canvas.height / scale) / 2;
    const radius = 150;

    ctx.beginPath();
    ctx.arc(centerX, centerY, 35, 0, Math.PI * 2);
    ctx.fillStyle = COLORS.call.main;
    ctx.fill();
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 2;
    ctx.stroke();

    ctx.fillStyle = '#1e1e2e';
    ctx.font = 'bold 11px monospace';
    ctx.textAlign = 'center';
    let displayName = targetFunc.name;
    if (displayName.length > 20) displayName = displayName.slice(0, 17) + '...';
    ctx.fillText(displayName, centerX, centerY + 3);

    ctx.font = '9px sans-serif';
    ctx.fillStyle = COLORS.text;
    ctx.fillText(parentFile.name, centerX, centerY + 22);

    const callees = targetFunc.calls || [];
    callees.forEach((callee, i) => {
        const angle = Math.PI / 2 + (i - callees.length / 2) * (Math.PI / Math.max(callees.length, 1));
        const x = centerX + Math.cos(angle) * radius;
        const y = centerY + Math.sin(angle) * radius + 60;

        drawCallNode(x, y, callee.name, COLORS.function);
        drawBezierArrow(centerX, centerY + 20, x, y - 20, COLORS.function.main);
    });

    const callers = targetFunc.callers || [];
    callers.forEach((caller, i) => {
        const angle = -Math.PI / 2 + (i - callers.length / 2) * (Math.PI / Math.max(callers.length, 1));
        const x = centerX + Math.cos(angle) * radius;
        const y = centerY + Math.sin(angle) * radius - 60;

        drawCallNode(x, y, caller.name, COLORS.class);
        drawBezierArrow(x, y + 20, centerX, centerY - 20, COLORS.class.main);
    });

    ctx.fillStyle = COLORS.text;
    ctx.font = '10px sans-serif';
    ctx.globalAlpha = 0.7;
    ctx.fillText(`📞 Вызывает: ${callees.length} | 📞 Вызывается: ${callers.length}`,
        centerX, (canvas.height / scale) - 30);
    ctx.globalAlpha = 1;

    ctx.restore();
}

function drawCallNode(x, y, name, color) {
    const isHovered = hoveredNode?.name === name;

    if (isHovered) {
        ctx.shadowBlur = 15;
        ctx.shadowColor = color.glow;
    }

    ctx.beginPath();
    ctx.arc(x, y, 25, 0, Math.PI * 2);
    ctx.fillStyle = color.main;
    ctx.fill();
    ctx.strokeStyle = isHovered ? '#ffffff' : '#45475a';
    ctx.stroke();

    ctx.shadowBlur = 0;

    ctx.fillStyle = '#1e1e2e';
    ctx.font = '9px monospace';
    ctx.textAlign = 'center';
    let displayName = name;
    if (displayName.length > 15) displayName = displayName.slice(0, 12) + '...';
    ctx.fillText(displayName, x, y + 3);
}

function drawBezierArrow(fromX, fromY, toX, toY, color) {
    const cp1x = fromX + (toX - fromX) * 0.25;
    const cp1y = fromY - 30;
    const cp2x = toX - (toX - fromX) * 0.25;
    const cp2y = toY - 30;

    ctx.beginPath();
    ctx.moveTo(fromX, fromY);
    ctx.bezierCurveTo(cp1x, cp1y, cp2x, cp2y, toX, toY);
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    const angle = Math.atan2(toY - cp2y, toX - cp2x);
    const headSize = 6;

    ctx.beginPath();
    ctx.moveTo(toX, toY);
    ctx.lineTo(toX - headSize * Math.cos(angle - Math.PI / 6), toY - headSize * Math.sin(angle - Math.PI / 6));
    ctx.lineTo(toX - headSize * Math.cos(angle + Math.PI / 6), toY - headSize * Math.sin(angle + Math.PI / 6));
    ctx.fillStyle = color;
    ctx.fill();
}

// ========== ОБРАБОТЧИКИ СОБЫТИЙ ==========
function onMouseDown(e) {
    if (e.button === 1 || e.button === 2) {
        isPanning = true;
        panStart = { x: e.clientX - pan.x, y: e.clientY - pan.y };
        canvas.style.cursor = 'grabbing';
        e.preventDefault();
    }
}

function onMouseUp() {
    isPanning = false;
    canvas.style.cursor = 'grab';
}

function onWheel(e) {
    e.preventDefault();
    const delta = e.deltaY > 0 ? 0.95 : 1.05;
    scale = Math.max(0.3, Math.min(2, scale * delta));
    renderCurrentView();
}

function onCanvasMouseMove(e) {
    const rect = canvas.getBoundingClientRect();
    const canvasX = (e.clientX - rect.left) * (canvas.width / rect.width);
    const canvasY = (e.clientY - rect.top) * (canvas.height / rect.height);
    const x = (canvasX - pan.x) / scale;
    const y = (canvasY - pan.y) / scale;

    let node = null;

    if (currentView.level === 'files') {
        for (const file of fileTree.values()) {
            const size = Math.min(80, 30 + (file.classes.length + file.functions.length) * 1.5);
            const dx = x - file.x;
            const dy = y - file.y;
            if (Math.hypot(dx, dy) < size / 2) {
                node = { id: file.id, name: file.name, type: 'file', data: file };
                break;
            }
        }
    } else if (currentView.level === 'classes') {
        const file = fileTree.get(currentView.selectedId);
        if (file) {
            const centerX = (canvas.width / scale) / 2;
            const centerY = (canvas.height / scale) / 2;
            if (Math.hypot(x - centerX, y - centerY) < 40) {
                node = { id: file.id, name: file.name, type: 'file', data: file };
            }
            if (!node) {
                const allItems = [...file.classes, ...file.functions];
                for (const item of allItems) {
                    const size = item.type === 'class' ? 55 : 45;
                    const dx = x - item.x;
                    const dy = y - item.y;
                    if (Math.hypot(dx, dy) < size / 2) {
                        node = { id: item.id, name: item.name, type: item.type, data: item };
                        break;
                    }
                }
            }
        }
    }

    if (node) {
        hoveredNode = node;
        canvas.style.cursor = 'pointer';
        const tooltipContent = `
            <strong>${node.type === 'file' ? '📁' : node.type === 'class' ? '📦' : '⚡'} ${escapeHTML(node.name)}</strong><br>
            ${node.type === 'file' ?
                `Классов: ${node.data?.classes?.length || 0}<br>Функций: ${node.data?.functions?.length || 0}` :
                node.type === 'class' ?
                    `Методов: ${node.data?.methods?.length || 0}` :
                    `Вызовов: ${node.data?.calls?.length || 0}`
            }
        `;
        showTooltip(tooltipContent, e.clientX, e.clientY);
        renderCurrentView();
    } else {
        hideTooltip();
        canvas.style.cursor = isPanning ? 'grabbing' : 'grab';
        renderCurrentView();
    }
}

function onCanvasClick(e) {
    if (!hoveredNode) return;

    if (currentView.level === 'files') {
        renderClassesView(hoveredNode.id);
    } else if (currentView.level === 'classes' && hoveredNode.type === 'function') {
        renderCallGraph(hoveredNode.id);
    }
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========
function showTooltip(content, x, y) {
    if (!tooltipDiv) {
        tooltipDiv = document.createElement('div');
        tooltipDiv.className = 'node-tooltip';
        document.body.appendChild(tooltipDiv);
    }
    tooltipDiv.innerHTML = content;
    tooltipDiv.style.left = x + 15 + 'px';
    tooltipDiv.style.top = y + 15 + 'px';
    tooltipDiv.style.display = 'block';
}

function hideTooltip() {
    if (tooltipDiv) tooltipDiv.style.display = 'none';
    hoveredNode = null;
}

function showEmptyState(message) {
    ctx.fillStyle = '#cdd6f4';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText(message || 'Нет данных', canvas.width / 2, canvas.height / 2);
}

function showError(message) {
    ctx.fillStyle = '#f38ba8';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('❌ ' + message, canvas.width / 2, canvas.height / 2);
}

function renderCurrentView() {
    if (currentView.level === 'files') {
        renderFilesView();
    } else if (currentView.level === 'classes') {
        renderClassesView(currentView.selectedId);
    } else if (currentView.level === 'calls') {
        renderCallGraph(currentView.selectedId);
    }
}

function goBack() {
    if (navigationStack.length === 0) {
        currentView = { level: 'files', selectedId: null, selectedName: null };
        updateBackButton();
        renderFilesView();
    } else {
        const lastView = navigationStack.pop();
        if (lastView.level === 'files') {
            currentView = { level: 'files', selectedId: null, selectedName: null };
            renderFilesView();
        } else if (lastView.level === 'classes') {
            renderClassesView(lastView.selectedId);
        }
        updateBackButton();
    }
}

function updateBackButton() {
    const backBtn = document.getElementById('graph-back-btn');
    if (backBtn) {
        backBtn.style.display = navigationStack.length > 0 ? 'inline-flex' : 'none';
    }
}

function updateStats() {
    const statsDiv = document.getElementById('graph-stats');
    if (statsDiv) {
        let totalClasses = 0, totalFunctions = 0;
        for (const file of fileTree.values()) {
            totalClasses += file.classes.length;
            totalFunctions += file.functions.length;
        }
        statsDiv.innerHTML = `📁 ${fileTree.size} | 📦 ${totalClasses} | 🔧 ${totalFunctions}`;
    }
}

function fitGraph() {
    scale = 0.8;
    pan = { x: 50, y: 50 };
    renderCurrentView();
}

function resetGraph() {
    scale = 1;
    pan = { x: 0, y: 0 };
    navigationStack = [];
    currentView = { level: 'files', selectedId: null, selectedName: null };
    initForceLayout();
    renderFilesView();
    updateBackButton();
}

function setupFilters() { }
function setupSearch() { }

// ========== ЭКСПОРТ ==========
window.initGraph = initGraph;
window.goBack = goBack;
window.fitGraph = fitGraph;
window.resetGraph = resetGraph;
window.setupFilters = setupFilters;
window.setupSearch = setupSearch;

// Добавляем roundRect если его нет
if (!CanvasRenderingContext2D.prototype.roundRect) {
    CanvasRenderingContext2D.prototype.roundRect = function (x, y, w, h, r) {
        if (w < 2 * r) r = w / 2;
        if (h < 2 * r) r = h / 2;
        this.moveTo(x + r, y);
        this.lineTo(x + w - r, y);
        this.quadraticCurveTo(x + w, y, x + w, y + r);
        this.lineTo(x + w, y + h - r);
        this.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
        this.lineTo(x + r, y + h);
        this.quadraticCurveTo(x, y + h, x, y + h - r);
        this.lineTo(x, y + r);
        this.quadraticCurveTo(x, y, x + r, y);
        return this;
    };
}