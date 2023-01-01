package blockdevice

import (
	"bufio"
	"fmt"
	"math"

	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	KB float64 = 2
	MB float64 = 1024 * 2
	GB float64 = 1024 * 1024 * 2
)

func AllDevicePath() []string {
	pathList := []string{}
	root := "/sys/dev/block/"
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		if info.Mode().Type() == fs.ModeSymlink {
			pathList = append(pathList, path)
		}

		return nil

	})
	return pathList

}

func CollectBlockDevice(path string) (deviceName, deviceType, sizeRow string, err error) {
	// 块设备的设备名称和设备类型在 uevent文件
	ueventPath := filepath.Join(path, "uevent")
	f, err := os.Open(ueventPath)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// 逐行扫描 uevent 文件读取到的值
	for scanner.Scan() {
		var str = scanner.Text()
		var index = strings.Index(str, "=")
		var key = str[0:index]
		var value = str[index+1:]
		value = strings.Replace(value, "\n", "", -1)
		if key == "DEVNAME" {
			deviceName = value
		} else if key == "DEVTYPE" {
			deviceType = value
		}
	}
	// 单独处理 lvm类型的设备
	dtype := strings.Split(deviceName, "-")
	if len(dtype) > 1 {
		typePath := filepath.Join(path, dtype[0])
		dir, err := os.Stat(typePath)
		if err == nil && dir.IsDir() {
			namePath := filepath.Join(typePath, "name")
			d, err := ioutil.ReadFile(namePath)
			if err != nil {
				return "", "", "", err
			}

			deviceName = strings.Replace(string(d), "\n", "", -1)
			deviceType = "lvm"

		}
	}
	// size 文件表示的 是 设备有多少个扇区，一个扇区等于 512Bytes
	sizePath := filepath.Join(path, "size")
	sizeData, err := ioutil.ReadFile(sizePath)
	if err != nil {
		return
	}
	sector := strings.TrimSpace(string(sizeData))
	size, err := strconv.Atoi(sector)
	if err != nil {
		return
	}
	switch {
	case size == 0:
		sizeRow = fmt.Sprintf("%d", size)
	case size < int(MB):
		sizeRow = fmt.Sprintf("%dKB", int(math.Ceil(float64(size)/KB)))
	case size < int(GB):
		sizeRow = fmt.Sprintf("%dMB", int(math.Ceil(float64(size)/MB)))
	default:
		sizeRow = fmt.Sprintf("%dGB", int(math.Ceil(float64(size)/GB)))
	}
	return

}
