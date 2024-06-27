package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"

	"github.com/knadh/go-pop3"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

type MailClient struct {
	client *pop3.Client
	conn   *pop3.Conn
}

func NewMailClient(server, username, password string) (*MailClient, error) {
	opt := pop3.Opt{
		Host:       server,
		Port:       995,
		TLSEnabled: true,
	}

	client := pop3.New(opt)
	conn, err := client.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %v", err)
	}

	err = conn.Auth(username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %v", err)
	}

	return &MailClient{client: client, conn: conn}, nil
}

func (mc *MailClient) Stat() (int, error) {
	stat, _, err := mc.conn.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to get STAT: %v", err)
	}
	return stat, nil
}

func (mc *MailClient) ListMessages(count int) ([]pop3.MessageID, error) {
	msgs, err := mc.conn.List(count)
	if err != nil {
		return nil, fmt.Errorf("failed to get LIST: %v", err)
	}
	return msgs, nil
}

func (mc *MailClient) RetrieveMessage(msgID int) (*mail.Message, error) {
	f, err := mc.conn.Retr(msgID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve message %d: %v", msgID, err)
	}
	content, err := io.ReadAll(f.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read message body: %v", err)
	}

	return &mail.Message{
		Header: mail.Header(f.Header.Map()),
		Body:   strings.NewReader(string(content)),
	}, nil
}

func (mc *MailClient) Quit() error {
	return mc.conn.Quit()
}

// decodeBase64 decodes a base64 encoded string
func decodeBase64(encoded string) ([]byte, error) {
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	return io.ReadAll(decoder)
}

// decodeCharset decodes a string with the given charset
func decodeCharset(charset string, input []byte) (string, error) {
	var decoded string
	var err error

	switch strings.ToLower(charset) {
	case "gbk":
		decoder := simplifiedchinese.GBK.NewDecoder()
		decoded, _, err = transform.String(decoder, string(input))
	case "gb18030":
		decoder := simplifiedchinese.GB18030.NewDecoder()
		decoded, _, err = transform.String(decoder, string(input))
	case "hz-gb2312":
		decoder := simplifiedchinese.HZGB2312.NewDecoder()
		decoded, _, err = transform.String(decoder, string(input))
	case "big5":
		decoder := traditionalchinese.Big5.NewDecoder()
		decoded, _, err = transform.String(decoder, string(input))
	case "iso-8859-1":
		decoder := charmap.ISO8859_1.NewDecoder()
		decoded, _, err = transform.String(decoder, string(input))
	default:
		decoded = string(input)
	}

	return decoded, err
}

// 具体可以打印正文
// 存在问题，不适配google，163 邮箱。
func (mc *MailClient) ParseMessage(msg *mail.Message) {
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		log.Fatal(err)
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}

			slurp, err := io.ReadAll(p)
			if err != nil {
				log.Fatal(err)
			}

			// 根据 Content-Transfer-Encoding 头信息解码内容
			encoding := p.Header.Get("Content-Transfer-Encoding")
			var decoded []byte
			if strings.ToLower(encoding) == "base64" {
				decoded, err = decodeBase64(string(slurp))
				if err != nil {
					log.Fatalf("Failed to decode base64 content: %v", err)
				}
			} else {
				decoded = slurp
			}

			// 根据 Content-Type 头信息解码字符集
			contentType := p.Header.Get("Content-Type")
			_, params, _ := mime.ParseMediaType(contentType)
			charset := params["charset"]
			decodedStr, err := decodeCharset(charset, decoded)
			if err != nil {
				log.Fatalf("Failed to decode charset: %v", err)
			}

			fmt.Printf("Part %q: %q\n", p.Header, decodedStr)
		}
	} else {
		content, _ := io.ReadAll(msg.Body)
		fmt.Printf("Single part message: %s\n", content)
	}
}

func main() {
	server := "pop.xx.com"
	username := "1@xx.com"
	password := ""

	mc, err := NewMailClient(server, username, password)
	if err != nil {
		log.Fatalf("Failed to create mail client: %v", err)
	}
	defer mc.Quit()

	stat, err := mc.Stat()
	if err != nil {
		log.Fatalf("Failed to get mail stat: %v", err)
	}
	fmt.Printf("Number of messages: %d\n", stat)

	msgs, err := mc.ListMessages(2)
	if err != nil {
		log.Fatalf("Failed to list messages: %v", err)
	}

	for _, msg := range msgs {
		fmt.Printf("Message ID: %d, Size: %d bytes\n", msg.ID, msg.Size)
		mailMsg, err := mc.RetrieveMessage(msg.ID)
		if err != nil {
			log.Fatalf("Failed to retrieve message %d: %v", msg.ID, err)
		}
		mc.ParseMessage(mailMsg)
	}
}
