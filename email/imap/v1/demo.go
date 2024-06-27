package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/emersion/go-imap"
	id "github.com/emersion/go-imap-id"
	"github.com/emersion/go-imap/client"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/golang-module/carbon"
)

func main() {

	err, result := emailListByUid1("imap.xx.com:993", "@xx.com", "1")
	fmt.Println(err)
	// 正则表达式解析
	for _, value := range result {
		fmt.Println(value)
	}
}

func emailListByUid1(Eserver, UserName, Password string) (err error, result []string) {
	c, err := loginEmail(Eserver, UserName, Password)
	if err != nil {
		fmt.Println(err)
		return
	}
	idClient := id.NewClient(c)
	idClient.ID(
		id.ID{
			id.FieldName:    "IMAPClient",
			id.FieldVersion: "2.1.0",
		},
	)

	defer c.Close()

	mailboxes := make(chan *imap.MailboxInfo, 10)
	mailboxeDone := make(chan error, 1)
	go func() {
		mailboxeDone <- c.List("", "*", mailboxes)
	}()
	for box := range mailboxes {
		if box.Name != "INBOX" {
			continue
		}
		fmt.Println("切换目录:", box.Name)
		mbox, err := c.Select(box.Name, false)
		// 选择收件箱
		if err != nil {
			fmt.Println("select inbox err: ", err)
			continue
		}
		if mbox.Messages == 0 {
			continue
		}

		// 选择收取邮件的时间段
		criteria := imap.NewSearchCriteria()
		// 收取7天之内的邮件
		location, _ := time.LoadLocation(carbon.Shanghai)
		format := time.Now().Format("2006-01-02 15:04:05")
		inLocation, _ := time.ParseInLocation("2006-01-02 15:04:05", format, location)
		criteria.Since = inLocation.Add(-1 * time.Minute * 15)
		fmt.Println(criteria.Since.Unix())
		// 按条件查询邮件
		ids, err := c.UidSearch(criteria)
		fmt.Println(len(ids))
		if err != nil {
			continue
		}
		if len(ids) == 0 {
			continue
		}
		seqset := new(imap.SeqSet)
		seqset.AddNum(ids...)
		sect := &imap.BodySectionName{Peek: true}

		messages := make(chan *imap.Message, 100)
		messageDone := make(chan error, 1)

		go func() {
			messageDone <- c.UidFetch(seqset, []imap.FetchItem{sect.FetchItem()}, messages)
		}()
		for msg := range messages {
			r := msg.GetBody(sect)
			mr, err := mail.CreateReader(r)
			if err != nil {
				fmt.Println(err)
				continue
			}
			/*header := mr.Header
			fmt.Println(header.Subject())*/
			_, fileName, results := parseEmail1(mr)
			result = append(result, results...)
			for k, _ := range fileName {
				fmt.Println("收取到附件:", k)
			}
		}
	}
	return
}

func parseEmail1(mr *mail.Reader) (body []byte, fileMap map[string][]byte, results []string) {
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			break
		}
		if p != nil {
			switch p.Header.(type) {
			case *mail.InlineHeader:
				body, err = ioutil.ReadAll(p.Body)
				if err != nil {
					fmt.Println("read body err:", err.Error())
				}
				//fmt.Println(string(body))
				//fmt.Println("---------------------------------------------------------")
				results = append(results, string(body))

				/*case *mail.AttachmentHeader:
				fileName, _ := h.Filename()
				fileContent, _ := ioutil.ReadAll(p.Body)
				fileMap[fileName] = fileContent*/
			}
		}
	}
	return
}

func loginEmail(Eserver, UserName, Password string) (*client.Client, error) {
	dial := new(net.Dialer)
	dial.Timeout = time.Duration(3) * time.Second
	c, err := client.DialWithDialerTLS(dial, Eserver, nil)
	if err != nil {
		c, err = client.DialWithDialer(dial, Eserver) // 非加密登录
	}
	if err != nil {
		return nil, err
	}
	// 登陆
	if err = c.Login(UserName, Password); err != nil {
		return nil, err
	}
	return c, nil
}
