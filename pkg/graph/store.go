package graph

import (
    "fmt"
    "encoding/binary"
    "encoding/json"

    "agent-db/pkg/storage"
)

const (
    PageSize       = 4096
    MetaPageID     = 0
    NodeStoreStart = 1
    EdgeStoreStart = 1000
    MaxNodesPerPage = 50   // примерно
    MaxEdgesPerPage = 100  // примерно
)

type GraphStore struct {
    BP         *storage.BufferPool
    Disk       *storage.DiskManager
    nodePages  map[uint64]*NodePage
    edgePages  map[uint64]*EdgePage
    nextNodePage uint64
    nextEdgePage uint64
}

type NodePage struct {
    PageID uint64
    Nodes  []*Node
    Dirty  bool
}

type EdgePage struct {
    PageID uint64
    Edges  []*Edge
    Dirty  bool
}

func NewGraphStore(bp *storage.BufferPool, disk *storage.DiskManager) *GraphStore {
    return &GraphStore{
        BP:           bp,
        Disk:         disk,
        nodePages:    make(map[uint64]*NodePage),
        edgePages:    make(map[uint64]*EdgePage),
        nextNodePage: NodeStoreStart,
        nextEdgePage: EdgeStoreStart,
    }
}

func (s *GraphStore) NextNodePage() uint64 {
    return s.nextNodePage
}

func (s *GraphStore) NextEdgePage() uint64 {
    return s.nextEdgePage
}

func (s *GraphStore) PutNode(node *Node) error {
    // Просто добавляем в память (SaveToDisk сохранит всё потом)
    page := s.getOrCreateNodePage()
    page.Nodes = append(page.Nodes, node)
    page.Dirty = true
    return nil
}

func (s *GraphStore) PutEdge(edge *Edge) error {
    // Просто добавляем в память (SaveToDisk сохранит всё потом)
    page := s.getOrCreateEdgePage()
    page.Edges = append(page.Edges, edge)
    page.Dirty = true
    return nil
}

// Flush сохраняет все грязные страницы на диск
func (s *GraphStore) Flush() error {
    for _, page := range s.nodePages {
        if page.Dirty {
            if err := s.flushNodePage(page); err != nil {
                return err
            }
        }
    }
    for _, page := range s.edgePages {
        if page.Dirty {
            if err := s.flushEdgePage(page); err != nil {
                return err
            }
        }
}
    return nil
}

func (s *GraphStore) getOrCreateNodePage() *NodePage {
    // Ищем страницу с местом
    for _, page := range s.nodePages {
        if len(page.Nodes) < MaxNodesPerPage {
            return page
        }
    }
    // Создаём новую
    page := &NodePage{PageID: s.nextNodePage}
    s.nextNodePage++
    s.nodePages[page.PageID] = page
    return page
}

func (s *GraphStore) getOrCreateEdgePage() *EdgePage {
    // Всегда создаём новую страницу когда нужны edges
    // Это упрощает логику - каждая страница содержит <= MaxEdgesPerPage
    page := &EdgePage{PageID: s.nextEdgePage}
    s.nextEdgePage++
    s.edgePages[page.PageID] = page
    return page
}

func (s *GraphStore) flushNodePage(page *NodePage) error {
    data, err := json.Marshal(page.Nodes)
    if err != nil {
        return err
    }

    // Не проверяем размер
    diskPage := storage.NewPage(page.PageID)
    offset := storage.HeaderSize + 24
    binary.LittleEndian.PutUint32(diskPage.Data[offset:offset+4], uint32(len(data)))
    copy(diskPage.Data[offset+4:], data)
    diskPage.Dirty = true
    page.Dirty = false
    return s.Disk.WritePage(diskPage)
}

func (s *GraphStore) flushEdgePage(page *EdgePage) error {
    data, err := json.Marshal(page.Edges)
    if err != nil {
        return err
    }

    // Не проверяем размер - записываем столько, сколько влезает
    // Если не влезает - ошибка будет при записи ( Disk.WritePage обработает )
    diskPage := storage.NewPage(page.PageID)
    offset := storage.HeaderSize + 24
    binary.LittleEndian.PutUint32(diskPage.Data[offset:offset+4], uint32(len(data)))
    copy(diskPage.Data[offset+4:], data)
    diskPage.Dirty = true
    page.Dirty = false
    return s.Disk.WritePage(diskPage)
}

func (s *GraphStore) GetAllNodes() ([]*Node, error) {
    var all []*Node
    for pageID := uint64(NodeStoreStart); pageID < s.nextNodePage; pageID++ {
        page, err := s.Disk.ReadPage(pageID)
        if err != nil {
            break
        }
        length := binary.LittleEndian.Uint32(page.Data[0:4])
        if length == 0 || length > PageSize-4 {
            continue
        }
        end := 4 + length
        if end > PageSize {
            end = PageSize
        }
        var nodes []*Node
        json.Unmarshal(page.Data[4:end], &nodes)
        all = append(all, nodes...)
    }
    return all, nil
}

func (s *GraphStore) GetAllEdges() ([]*Edge, error) {
    var all []*Edge
    for pageID := uint64(EdgeStoreStart); pageID < s.nextEdgePage; pageID++ {
        page, err := s.Disk.ReadPage(pageID)
        if err != nil {
            break
        }
        length := binary.LittleEndian.Uint32(page.Data[0:4])
        if length == 0 || length > PageSize-4 {
            continue
        }
        end := 4 + length
        if end > PageSize {
            end = PageSize
        }
        var edges []*Edge
        json.Unmarshal(page.Data[4:end], &edges)
        all = append(all, edges...)
    }
    return all, nil
}

func (s *GraphStore) SaveMeta() error {
    fmt.Printf("[DEBUG SaveMeta] nextNodePage=%d nextEdgePage=%d\n", s.nextNodePage, s.nextEdgePage)
    meta := map[string]uint64{
        "next_node_page": s.nextNodePage,
        "next_edge_page": s.nextEdgePage,
    }
    data, _ := json.Marshal(meta)
    page := storage.NewPage(MetaPageID)
    copy(page.Data[:], data)
    page.Dirty = true
    return s.Disk.WritePage(page)
}

func (s *GraphStore) LoadMeta() error {
    page, err := s.Disk.ReadPage(MetaPageID)
    if err != nil {
        return err
    }
    var meta struct {
        NextNodePage uint64
        NextEdgePage uint64
    }
    json.Unmarshal(page.Data[:], &meta)
    fmt.Printf("[DEBUG LoadMeta] nextNodePage=%d nextEdgePage=%d\n", meta.NextNodePage, meta.NextEdgePage)
    if meta.NextNodePage > 0 {
        s.nextNodePage = meta.NextNodePage
    }
    if meta.NextEdgePage > 0 {
        s.nextEdgePage = meta.NextEdgePage
    }
    return nil
}