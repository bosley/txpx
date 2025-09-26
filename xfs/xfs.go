package xfs

type FileStore interface {
}

type xfsImpl struct {
	location string
}

var _ FileStore = &xfsImpl{}

func NewFileStore(location string) FileStore {
	return &xfsImpl{
		location: location,
	}
}
