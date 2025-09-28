package controllers

type Controllers interface {
}

type controllersImpl struct {
}

var _ Controllers = &controllersImpl{}

func New() Controllers {
	return &controllersImpl{}
}
