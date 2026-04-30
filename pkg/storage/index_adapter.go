package storage

// IndexDiskAdapter адаптирует storage.DiskManager для index.DiskManager
type IndexDiskAdapter struct {
	DM *DiskManager
}

func (a *IndexDiskAdapter) AllocatePage() uint64 {
	return a.DM.AllocatePage()
}

func (a *IndexDiskAdapter) ReadPage(pageID uint64) ([]byte, error) {
	page, err := a.DM.ReadPage(pageID)
	if err != nil {
		return nil, err
	}
	return page.Data[:], nil
}

func (a *IndexDiskAdapter) WritePage(pageID uint64, data []byte) error {
	page := &Page{
		ID:   pageID,
		Dirty: true,
	}
	copy(page.Data[:], data)
	return a.DM.WritePage(page)
}