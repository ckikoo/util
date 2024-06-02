package model

import (
	"os"
	"path/filepath"
	"sync"
)

// ReadDir 读取目录中的所有文件路径，并将其发送到通道
func ReadDir(dir string, pathChan chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			pathChan <- path
		}
		return nil
	})
	close(pathChan)
}
