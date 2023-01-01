package blockdevice

import "os"

const (
	Online  = "online"
	OffLine = "offline"
)

var NodeName string

type Device struct {
	Name string
	Type string
	Size string
	Path string
}

var BlockDeviceManager map[string]Device

func init() {

	NodeName = os.Getenv("NodeName")
	BlockDeviceManager = make(map[string]Device)
}
