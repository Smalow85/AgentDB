package storage

import "fmt"

type RID struct {
	PageID uint64
	SlotID uint16
}

func (r RID) String() string {
	return fmt.Sprintf("(%d,%d)", r.PageID, r.SlotID)
}

var InvalidRID = RID{0, 0}