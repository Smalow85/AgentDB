let graphData = null
let nodes = {}
let links = []
let draggedNode = null
let dragOffset = {x:0, y:0}
let scale = 1
let pan = {x:0, y:0}
let isPanning = false
let panStart = {x:0, y:0}
let hovered = null
let filterType = 'all'
let searchQuery = ''
let simulationTick = 0

const WIDTH = 900
const HEIGHT = 600

document.addEventListener('DOMContentLoaded', () => {
    document.getElementById('btn-execute').onclick = execute
    document.getElementById('btn-parse').onclick = parseRepo
    
    document.querySelectorAll('.filter-btn').forEach(btn => {
        btn.onclick = () => {
            document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'))
            btn.classList.add('active')
            filterType = btn.dataset.filter
            runSimulation()
        }
    })
    
    document.getElementById('search-input').oninput = e => {
        searchQuery = e.target.value.toLowerCase()
        draw()
    }
    
    loadTables()
    document.querySelectorAll('.tab').forEach(t => t.onclick = () => switchTab(t.dataset.tab))

    const cvs = document.getElementById('graph-canvas')
    
    cvs.addEventListener('mousedown', e => {
        const pos = getPos(e, cvs)
        const node = findNode(pos.x, pos.y)
        
        if (e.button === 0 && node) {
            draggedNode = node
            dragOffset = {x: pos.x - node.x, y: pos.y - node.y}
        } else if (e.button === 2 || (e.button === 0 && !node)) {
            isPanning = true
            panStart = {x: e.clientX - pan.x, y: e.clientY - pan.y}
        }
    })

    cvs.addEventListener('mousemove', e => {
        const pos = getPos(e, cvs)
        
        if (draggedNode) {
            draggedNode.x = pos.x - dragOffset.x
            draggedNode.y = pos.y - dragOffset.y
            draggedNode.vx = 0
            draggedNode.vy = 0
            draw()
        } else if (isPanning) {
            pan.x = e.clientX - panStart.x
            pan.y = e.clientY - panStart.y
            draw()
        } else {
            const node = findNode(pos.x, pos.y)
            if (node !== hovered) {
                hovered = node
                cvs.style.cursor = node ? 'grab' : 'default'
                draw()
            }
        }
    })

    cvs.addEventListener('mouseup', () => { draggedNode = null; isPanning = false })
    cvs.addEventListener('mouseleave', () => { draggedNode = null; isPanning = false })

    cvs.addEventListener('wheel', e => {
        e.preventDefault()
        scale *= e.deltaY > 0 ? 0.9 : 1.1
        scale = Math.max(0.1, Math.min(3, scale))
        draw()
    })
    
    cvs.addEventListener('contextmenu', e => e.preventDefault())
})

function switchTab(name) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'))
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'))
    document.querySelector('.tab[data-tab="'+name+'"]').classList.add('active')
    document.getElementById('tab-'+name).classList.add('active')
    if (name === 'graph' && !graphData) loadGraph()
}

async function loadGraph() {
    graphData = await (await fetch('/api/graph')).json()
    initNodes()
    runSimulation()
    draw()
}

function initNodes() {
    const filtered = getFilteredNodes()
    const ids = new Set(filtered.map(n => n.id))
    links = graphData.edges.filter(e => ids.has(e.from) && ids.has(e.to))
    
    // Init nodes in a grid with some randomness
    filtered.forEach((n, i) => {
        const col = i % 15
        const row = Math.floor(i / 15)
        nodes[n.id] = {
            id: n.id,
            label: n.label,
            type: n.type,
            x: 80 + col * 55 + Math.random() * 20,
            y: 80 + row * 55 + Math.random() * 20,
            vx: 0,
            vy: 0
        }
    })
}

function runSimulation() {
    if (!graphData) return
    
    const filtered = getFilteredNodes()
    const nodeList = Object.values(nodes)
    
    // Run multiple iterations for spread
    for (let iter = 0; iter < 30; iter++) {
        nodeList.forEach(node => {
            if (draggedNode && node.id === draggedNode.id) return
            
            let fx = 0, fy = 0
            
            // Repulsion from all other nodes
            nodeList.forEach(other => {
                if (other.id === node.id) return
                const dx = node.x - other.x
                const dy = node.y - other.y
                const d = Math.sqrt(dx*dx + dy*dy) + 0.1
                if (d < 120) {
                    fx += (dx / d) * 300 / (d * d)
                    fy += (dy / d) * 300 / (d * d)
                }
            })
            
            // Attraction along links
            links.forEach(link => {
                let other = null
                if (link.from === node.id) other = nodes[link.to]
                else if (link.to === node.id) other = nodes[link.from]
                if (!other) return
                
                const dx = other.x - node.x
                const dy = other.y - node.y
                fx += dx * 0.008
                fy += dy * 0.008
            })
            
            // Center gravity
            fx += (WIDTH/2 - node.x) * 0.003
            fy += (HEIGHT/2 - node.y) * 0.003
            
            // Apply
            node.vx = (node.vx + fx) * 0.85
            node.vy = (node.vy + fy) * 0.85
            node.x += node.vx
            node.y += node.vy
            
            // Bounds
            node.x = Math.max(40, Math.min(WIDTH-40, node.x))
            node.y = Math.max(40, Math.min(HEIGHT-40, node.y))
        })
    }
}

function getFilteredNodes() {
    if (!graphData || !graphData.nodes) return []
    return graphData.nodes.filter(n => {
        if (filterType !== 'all' && n.type !== filterType) return false
        if (searchQuery && !n.label.toLowerCase().includes(searchQuery)) return false
        return true
    })
}

function draw() {
    const cvs = document.getElementById('graph-canvas')
    const ctx = cvs.getContext('2d')
    ctx.clearRect(0, 0, WIDTH, HEIGHT)
    
    ctx.save()
    ctx.translate(pan.x, pan.y)
    ctx.scale(scale, scale)

    const nodeList = Object.values(nodes)
    const filteredIds = new Set(getFilteredNodes().map(n => n.id))
    
    // Status
    ctx.fillStyle = '#707090'
    ctx.font = '11px sans-serif'
    ctx.fillText(nodeList.length + ' nodes | ' + links.length + ' links', 10, 18)

    // Links
    links.forEach(link => {
        const from = nodes[link.from]
        const to = nodes[link.to]
        if (!from || !to) return
        
        ctx.beginPath()
        ctx.strokeStyle = link.type === 'calls' ? '#944' : '#446'
        ctx.lineWidth = 0.8
        ctx.moveTo(from.x, from.y)
        ctx.lineTo(to.x, to.y)
        ctx.stroke()
    })

    // Nodes
    nodeList.forEach(node => {
        const isHover = hovered && hovered.id === node.id
        const isSearch = searchQuery && node.label.toLowerCase().includes(searchQuery)
        
        ctx.beginPath()
        ctx.fillStyle = node.type === 'class' ? '#c42' : '#38c'
        ctx.strokeStyle = isHover ? '#f64' : (isSearch ? '#fa0' : '#e54')
        ctx.lineWidth = isHover ? 2.5 : 1.5
        ctx.arc(node.x, node.y, isHover ? 10 : 7, 0, Math.PI * 2)
        ctx.fill()
        ctx.stroke()
        
        // Label
        ctx.fillStyle = isHover ? '#fff' : '#aaa'
        ctx.font = '9px sans-serif'
        ctx.textAlign = 'center'
        const label = node.label.length > 14 ? node.label.slice(0,11)+'...' : node.label
        ctx.fillText(label, node.x, node.y + 18)
    })

    ctx.restore()
    
    // Footer
    ctx.fillStyle = '#556'
    ctx.font = '10px sans-serif'
    ctx.fillText('Drag nodes to move | Scroll: zoom | Right-drag: pan | Filter: class/function', 8, HEIGHT - 10)
}

function getPos(e, cvs) {
    const r = cvs.getBoundingClientRect()
    return {
        x: (e.clientX - r.left - pan.x) / scale,
        y: (e.clientY - r.top - pan.y) / scale
    }
}

function findNode(x, y) {
    const list = Object.values(nodes)
    for (const n of list) {
        const dx = x - n.x
        const dy = y - n.y
        if (dx*dx + dy*dy < 150) return n
    }
    return null
}

async function parseRepo() {
    const status = document.getElementById('parse-status')
    status.textContent = 'Analyzing...'
    const d = await (await fetch('/api/parse', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({path: document.getElementById('repo-path').value})
    })).json()
    
    if (d.error) status.textContent = 'Error: ' + d.error
    else { status.textContent = d.files + ' files, ' + d.classes + ' classes' ; loadGraph() }
}

async function execute() {
    const sql = document.getElementById('sql-input').value.trim()
    if (!sql) return
    const d = await (await fetch('/api/query', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({sql})
    })).json()
    document.getElementById('result-output').textContent = d.result || d.error
}

async function loadTables() {
    const d = await (await fetch('/api/tables')).json()
    document.getElementById('tables-list').innerHTML = (d.tables || []).map(t => '<div>'+t+'</div>').join('')
}