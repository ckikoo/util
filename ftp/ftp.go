package main

import (
        "io"
        "log"
        "os"
        "path/filepath"

        "github.com/goftp/server"
)

// MyFileInfo 实现了 server.FileInfo 接口
type MyFileInfo struct {
        os.FileInfo
}

func (fi MyFileInfo) Owner() string {
        return "owner"
}

func (fi MyFileInfo) Group() string {
        return "group"
}

// MyDriver 实现了 server.Driver 接口
type MyDriver struct {
        rootPath string
}

func (d *MyDriver) Init(conn *server.Conn) {
        log.Println("New connection:", conn.PublicIp())
}

func (d *MyDriver) Stat(path string) (server.FileInfo, error) {
        fullPath := filepath.Join(d.rootPath, path)
        info, err := os.Stat(fullPath)
        if err != nil {
                return nil, err
        }
        return MyFileInfo{info}, nil
}

func (d *MyDriver) ListDir(path string, callback func(server.FileInfo) error) error {
        fullPath := filepath.Join(d.rootPath, path)
        entries, err := os.ReadDir(fullPath)
        if err != nil {
                return err
        }
        for _, entry := range entries {
                info, err := entry.Info()
                if err != nil {
                        return err
                }
                if err := callback(MyFileInfo{info}); err != nil {
                        return err
                }
        }
        return nil
}

func (d *MyDriver) DeleteDir(path string) error {
        fullPath := filepath.Join(d.rootPath, path)
        return os.Remove(fullPath)
}

func (d *MyDriver) DeleteFile(path string) error {
        fullPath := filepath.Join(d.rootPath, path)
        return os.Remove(fullPath)
}

func (d *MyDriver) Rename(fromPath string, toPath string) error {
        fullFromPath := filepath.Join(d.rootPath, fromPath)
        fullToPath := filepath.Join(d.rootPath, toPath)
        return os.Rename(fullFromPath, fullToPath)
}

func (d *MyDriver) MakeDir(path string) error {
        fullPath := filepath.Join(d.rootPath, path)
        return os.Mkdir(fullPath, os.ModePerm)
}

func (d *MyDriver) GetFile(path string, offset int64) (int64, io.ReadCloser, error) {
        fullPath := filepath.Join(d.rootPath, path)
        file, err := os.Open(fullPath)
        if err != nil {
                return 0, nil, err
        }
        stat, err := file.Stat()
        if err != nil {
                return 0, nil, err
        }
        if offset > 0 {
                if _, err := file.Seek(offset, io.SeekStart); err != nil {
                        return 0, nil, err
                }
        }
        return stat.Size(), file, nil
}

func (d *MyDriver) PutFile(destPath string, data io.Reader, appendData bool) (int64, error) {
        fullPath := filepath.Join(d.rootPath, destPath)
        var file *os.File
        var err error
        if appendData {
                file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, os.ModePerm)
        } else {
                file, err = os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
        }
        if err != nil {
                return 0, err
        }
        defer file.Close()
        written, err := io.Copy(file, data)
        return written, err
}

func (d *MyDriver) ChangeDir(path string) error {
        fullPath := filepath.Join(d.rootPath, path)
        info, err := os.Stat(fullPath)
        if err != nil {
                return err
        }
        if !info.IsDir() {
                return os.ErrInvalid
        }
        return nil
}

type MyDriverFactory struct {
        rootPath string
}

func (f *MyDriverFactory) NewDriver() (server.Driver, error) {
        return &MyDriver{rootPath: f.rootPath}, nil
}

func main() {
        factory := &MyDriverFactory{rootPath: ""} // 监听哪个路径
        auth := &server.SimpleAuth{ 
                Name:     "cg",                    // 用户名
                Password: "6666",                  // 密码
        }

        opts := &server.ServerOpts{
                Factory: factory,
                Auth:    auth,
                Port:    2121,
        }

        ftpServer := server.NewServer(opts)
        log.Println("Starting FTP server on port 2121...")
        if err := ftpServer.ListenAndServe(); err != nil {
                log.Fatal("Error starting server:", err)
        }
}
