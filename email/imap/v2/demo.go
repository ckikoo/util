package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/emersion/go-imap"
	id "github.com/emersion/go-imap-id"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/pkg/errors"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func init() {
	// 注册GBK字符集
	charset.RegisterEncoding("GBK", simplifiedchinese.GBK)
	charset.RegisterEncoding("GB18030", simplifiedchinese.GB18030)
	charset.RegisterEncoding("HZ-GB2312", simplifiedchinese.HZGB2312)
}

type IMAPClient struct {
	server   string
	username string
	password string
	client   *client.Client
}

func NewIMAPClient(server, username, password string) *IMAPClient {
	return &IMAPClient{
		server:   server,
		username: username,
		password: password,
	}
}

func (c *IMAPClient) Connect() error {
	cl, err := client.DialTLS(c.server, nil)
	if err != nil {
		return err
	}

	err = cl.Login(c.username, c.password)
	if err != nil {
		return err
	}

	// 发送客户端ID信息
	idClient := id.NewClient(cl)
	idClient.ID(
		id.ID{
			id.FieldName:    "IMAPClient",
			id.FieldVersion: "2.1.0",
		},
	)

	c.client = cl
	return nil
}

func (c *IMAPClient) Disconnect() error {
	if c.client != nil {
		return c.client.Logout()
	}
	return nil
}

func (c *IMAPClient) GetMessages(numMessages uint32) ([]*imap.Message, error) {
	mbox, err := c.client.Select("INBOX", false)
	if err != nil {
		return nil, err
	}

	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > numMessages && numMessages != 0 {
		from = mbox.Messages - numMessages + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	// 获取邮件信封、RFC822 内容和 UID
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchRFC822}
	messages := make(chan *imap.Message, numMessages)
	done := make(chan error, 1)
	go func() {
		done <- c.client.Fetch(seqset, items, messages)
	}()

	var msgs []*imap.Message
	for msg := range messages {
		msgs = append(msgs, msg)
	}

	if err := <-done; err != nil {
		return nil, err
	}
	return msgs, nil
}

func (client *IMAPClient) SetSeen(uid uint32) error {
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	done := make(chan error, 1)

	go func() {
		// 设置邮件为 SeenFlag（已读）
		err := client.client.Store(seqSet, imap.AddFlags, []interface{}{imap.SeenFlag}, nil)
		if err != nil {
			done <- errors.Wrap(err, "Store failed")
			return
		}

		done <- nil
	}()

	return <-done
}

func (client *IMAPClient) SetDelete(pos int) error {
	seenSet, err := imap.ParseSeqSet(strconv.Itoa(pos))
	if err != nil {
		return errors.New("ParseSeqSet failed: " + err.Error())
	}

	done := make(chan error, 1)

	go func() {
		// 设置邮件为 DeletedFlag
		err := client.client.Store(seenSet, imap.AddFlags, []interface{}{imap.DeletedFlag}, nil)
		if err != nil {
			done <- errors.Wrap(err, "Store failed")
			return
		}

		// 执行 EXPUNGE 操作来彻底删除邮件
		err = client.client.Expunge(nil) // nil 表示不需要接收被删除邮件的 UID
		if err != nil {
			done <- errors.Wrap(err, "Expunge failed")
			return
		}

		done <- nil
	}()

	return <-done
}

func (c *IMAPClient) ParseMessages(messages []*imap.Message, body chan string, filePaths chan string) {
	defer close(body)
	defer close(filePaths)
	var wg sync.WaitGroup

	// 控制同时运行的协程数量
	concurrency := 5
	sem := make(chan struct{}, concurrency)

	for _, msg := range messages {
		if msg.Body == nil {
			log.Println("Body is nil")
			continue
		}

		wg.Add(1)
		sem <- struct{}{} // 协程信号量控制

		go func(msg *imap.Message) {
			defer func() {
				<-sem
				wg.Done()
			}()
			if msg == nil {
				return
			}
			section := imap.BodySectionName{}
			r := msg.GetBody(&section)
			if r == nil {
				log.Println("Server didn't return message body")
				return
			}

			// 解析邮件内容
			mr, err := mail.CreateReader(r)
			if err != nil {
				log.Printf("Error creating mail reader: %v\n", err)
				return
			}

			// 打印邮件主体内容
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Printf("Error reading message part: %v\n", err)
					continue
				}

				switch h := p.Header.(type) {
				case *mail.InlineHeader:
					b, err := io.ReadAll(p.Body)
					if err != nil {
						log.Printf("Error reading inline part: %v\n", err)
						continue
					}
					log.Printf("Got text: %s\n", b)
					body <- string(b)

				case *mail.AttachmentHeader:
					filename, err := h.Filename()
					if err != nil {
						log.Printf("Error getting attachment filename: %v\n", err)
						continue
					}
					log.Printf("Got attachment: %s\n", filename)

					// 生成本地文件路径
					localPath := "./attachments/" + filename // 示例存放在当前目录下的 attachments 文件夹中

					// 创建本地文件
					file, err := os.Create(localPath)
					if err != nil {
						log.Printf("Error creating file: %v\n", err)
						continue
					}

					// 将附件内容写入文件
					_, err = io.Copy(file, p.Body)
					if err != nil {
						log.Printf("Error writing attachment to file: %v\n", err)
						file.Close()
						continue
					}
					file.Close()

					// 将文件路径发送到 filePaths 通道
					filePaths <- localPath
				}
			}
		}(msg)
	}

	wg.Wait()
}

func main() {

	client := NewIMAPClient("imap.xx.com:993", "yy@xx.com", "密钥")
	err := client.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect()

	fmt.Println("登录成功")

	messages, err := client.GetMessages(1)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("收件箱中有 %d 封邮件\n", len(messages))

	body := make(chan string)
	filePath := make(chan string)

	var wg sync.WaitGroup

	// 启动协程处理邮件解析和附件保存
	wg.Add(1)
	go func() {
		defer wg.Done()
		client.ParseMessages(messages, body, filePath)
	}()

	// 启动协程处理接收到的邮件内容
	wg.Add(1)
	go func() {

		defer wg.Done()

		for msg := range body {
			fmt.Println("Received body:", msg)
		}
	}()

	// 启动协程处理接收到的附件文件路径
	wg.Add(1)
	go func() {
		defer wg.Done()

		for path := range filePath {
			fmt.Println("Saved attachment at:", path)
		}
	}()

	// 等待所有协程完成
	wg.Wait()
}
